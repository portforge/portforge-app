package main

import (
	"archive/zip"
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"portforge/metadata"
	"portforge/models"
	"portforge/storageunits"
	"portforge/store"
	"runtime"
	"strings"
	"sync"
	"time"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
	"github.com/zamiba/forge/engine"
)

// devMetadataOverride is empty in production. The dev build tag (set by
// "wails dev") populates it with the project-local mediaitem folder.
var devMetadataOverride string

// httpClient is used for all outbound requests. The download timeout is kept
// generous (30 min) to accommodate large game source archives over slow links,
// while the API timeout is short since those responses are tiny.
var (
	httpClient    = &http.Client{Timeout: 30 * time.Minute}
	httpAPIClient = &http.Client{Timeout: 15 * time.Second}
)

const (
	mediaItemsZipURL  = "https://github.com/zamiba/portforge-mediaitems/archive/refs/heads/main.zip"
	mediaItemsAPIURL  = "https://api.github.com/repos/zamiba/portforge-mediaitems/commits/main"
	mediaItemsSHAFile = ".portforge-sha"
)

// GetMediaItemsSHA returns the short commit SHA of the currently installed
// MediaItems library, or an empty string if not yet downloaded.
// GetMediaItemsSHA returns the short commit SHA of the synced MediaItems library.
// Returns "unknown" if MediaItems are present but were not synced through PortForge,
// or an empty string if the configured path has no MediaItems at all.
func (a *App) GetMediaItemsSHA() string {
	if a.metadataPath == "" {
		return ""
	}
	data, err := os.ReadFile(filepath.Join(a.metadataPath, mediaItemsSHAFile))
	if err != nil {
		// No SHA file — check whether MediaItems actually exist at the path.
		if _, err := os.Stat(filepath.Join(a.metadataPath, metadata.PortItemType)); err == nil {
			return "unknown"
		}
		return ""
	}
	sha := strings.TrimSpace(string(data))
	if len(sha) > 7 {
		sha = sha[:7]
	}
	return sha
}

// CatalogInfo is the state of the MediaItems catalog, as shown in Settings.
type CatalogInfo struct {
	SHA       string `json:"sha"`       // short commit SHA, "unknown", or "" when absent
	SyncedAt  string `json:"syncedAt"`  // RFC3339; empty when never synced through PortForge
	PortCount int    `json:"portCount"` // ports the catalog offers, synced or not
	DevMode   bool   `json:"devMode"`
}

// GetCatalogInfo summarises the installed catalog for the Settings screen.
func (a *App) GetCatalogInfo() CatalogInfo {
	info := CatalogInfo{SHA: a.GetMediaItemsSHA(), DevMode: devMetadataOverride != ""}
	if a.metadataPath == "" {
		return info
	}
	// The SHA file is written at the end of every sync, so its mtime already is
	// the last-synced time; persisting a second copy of it would only give the
	// two a chance to disagree.
	if st, err := os.Stat(filepath.Join(a.metadataPath, mediaItemsSHAFile)); err == nil {
		info.SyncedAt = st.ModTime().UTC().Format(time.RFC3339)
	}
	entries, err := os.ReadDir(filepath.Join(a.metadataPath, metadata.PortItemType))
	if err != nil {
		return info
	}
	for _, e := range entries {
		if e.IsDir() {
			info.PortCount++
		}
	}
	return info
}

// CheckMediaItemsUpdate fetches the latest commit SHA from GitHub and returns
// true if it differs from the currently synced version.
func (a *App) CheckMediaItemsUpdate() (bool, error) {
	installed, err := os.ReadFile(filepath.Join(a.metadataPath, mediaItemsSHAFile))
	if err != nil {
		return true, nil // no SHA file → always offer a sync
	}
	req, err := http.NewRequest("GET", mediaItemsAPIURL, nil)
	if err != nil {
		return false, err
	}
	req.Header.Set("Accept", "application/vnd.github.sha")
	resp, err := httpAPIClient.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, err
	}
	return strings.TrimSpace(string(installed)) != strings.TrimSpace(string(body)), nil
}

// IsDevMode returns true when the app is running under wails dev, meaning the
// project-local mediaitems folder is used as the catalog.
func (a *App) IsDevMode() bool {
	return devMetadataOverride != ""
}

// RefreshLibraryIndex rebuilds the SQLite index from the current catalog on
// disk and resyncs user ROM presence. Useful in dev mode after adding new
// MediaItem JSON files without running a full sync.
func (a *App) RefreshLibraryIndex() error {
	if a.store == nil {
		return fmt.Errorf("store not initialised")
	}
	if err := a.store.RebuildCatalog(a.metadataPath); err != nil {
		return err
	}
	if a.dataPath != "" {
		_ = a.store.SyncUserROMs(a.dataPath)
		_ = a.store.ScanUserLibraryUpdates(a.metadataPath, a.dataPath)
	}
	return nil
}

// SyncMediaItems downloads the latest MediaItems from GitHub into the catalog
// directory (config dir / mediaitems), then compares the updated catalog with
// the user's local library copies and marks any changed items as having updates.
func (a *App) SyncMediaItems() error {
	if devMetadataOverride != "" {
		return fmt.Errorf("sync is disabled in dev mode")
	}
	destDir := a.metadataPath
	if err := os.MkdirAll(destDir, 0755); err != nil {
		return err
	}

	// Download ZIP to a temp file.
	tmp, err := os.CreateTemp("", "portforge-mediaitems-*.zip")
	if err != nil {
		return err
	}
	tmpZip := tmp.Name()
	defer os.Remove(tmpZip)

	a.emit("mediaitems:progress", map[string]interface{}{"phase": "downloading", "percent": 0})
	resp, err := httpClient.Get(mediaItemsZipURL)
	if err != nil {
		tmp.Close()
		return err
	}
	pr := &progressReader{
		r:     resp.Body,
		total: resp.ContentLength,
		onPct: func(pct int) {
			a.emit("mediaitems:progress", map[string]interface{}{"phase": "downloading", "percent": pct})
		},
	}
	_, err = io.Copy(tmp, pr)
	resp.Body.Close()
	tmp.Close()
	if err != nil {
		return err
	}

	// Extract into a temp directory.
	a.emit("mediaitems:progress", map[string]interface{}{"phase": "extracting", "percent": 0})
	tmpDir, err := os.MkdirTemp("", "portforge-mediaitems-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	if err := extractZipStrip1(tmpZip, tmpDir); err != nil {
		return fmt.Errorf("extraction failed: %w", err)
	}

	// Copy extracted files over destDir, overwriting existing files.
	a.emit("mediaitems:progress", map[string]interface{}{"phase": "copying", "percent": 0})
	if err := copyDirMerge(tmpDir, destDir); err != nil {
		return fmt.Errorf("sync failed: %w", err)
	}

	// Fetch and store the commit SHA.
	if req, err := http.NewRequest("GET", mediaItemsAPIURL, nil); err == nil {
		req.Header.Set("Accept", "application/vnd.github.sha")
		if shaResp, err := http.DefaultClient.Do(req); err == nil {
			body, _ := io.ReadAll(shaResp.Body)
			shaResp.Body.Close()
			_ = os.WriteFile(filepath.Join(destDir, mediaItemsSHAFile), body, 0644)
		}
	}

	// Rebuild the DB index from the updated catalog, then resync user ROMs and
	// update-flags so the UI reflects the new catalog state immediately.
	if a.store != nil {
		_ = a.store.RebuildCatalog(destDir)
		if a.dataPath != "" {
			_ = a.store.SyncUserROMs(a.dataPath)
			_ = a.store.ScanUserLibraryUpdates(destDir, a.dataPath)
		}
	}

	a.emit("mediaitems:progress", map[string]interface{}{"phase": "done", "percent": 100})
	return nil
}

// copyDirMerge copies all files from src into dst, overwriting existing files.
// Directories in dst that are not in src are left untouched.
func copyDirMerge(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		// filepath.Walk uses os.Lstat, so symlinks appear here with
		// ModeSymlink set rather than being followed. Skip them: the
		// subsequent copyFile call uses os.Open which would follow the
		// symlink and potentially read content outside the source tree.
		if info.Mode()&os.ModeSymlink != 0 {
			return nil
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if info.IsDir() {
			return os.MkdirAll(target, 0755)
		}
		return copyFile(path, target)
	})
}

// userLibraryPath returns the root of the user's local MediaItem copies.
func (a *App) userLibraryPath() string {
	return filepath.Join(a.dataPath, "library")
}

// copyToUserLibrary copies a MediaItem folder from the catalog into the user's
// local library, creating the destination tree as needed. Called after install
// or ROM import so the user has a local snapshot for update comparison.
func (a *App) copyToUserLibrary(mediaType, itemTitle string) error {
	src := filepath.Join(a.metadataPath, mediaType, itemTitle)
	dst := filepath.Join(a.userLibraryPath(), mediaType, itemTitle)
	if err := os.MkdirAll(dst, 0755); err != nil {
		return err
	}
	return copyDirMerge(src, dst)
}

// GetItemUpdate returns true when the catalog version of this item differs from
// the user's local library copy (i.e. an update is available).
func (a *App) GetItemUpdate(itemTitle string) bool {
	if a.store == nil {
		return false
	}
	return a.store.GetItemUpdate(itemTitle)
}

// UpdateMediaItem overwrites the user's local library copy with the current
// catalog version and clears the pending-update flag for that item.
func (a *App) UpdateMediaItem(itemTitle string) error {
	if err := a.copyToUserLibrary(metadata.PortItemType, itemTitle); err != nil {
		return err
	}
	if a.store != nil {
		_ = a.store.SetItemUpdate(itemTitle, false)
	}
	return nil
}

// extractZipStrip1 extracts a ZIP archive into destDir, stripping the single
// top-level directory that GitHub adds to repo archives (e.g. "repo-main/").
func extractZipStrip1(src, destDir string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer r.Close()

	// Determine the common top-level prefix to strip.
	prefix := ""
	if len(r.File) > 0 {
		parts := strings.SplitN(filepath.ToSlash(r.File[0].Name), "/", 2)
		if len(parts) > 1 {
			prefix = parts[0] + "/"
		}
	}

	destDir = filepath.Clean(destDir)
	// Pre-compute the canonical prefix used for containment checks below.
	// We append the separator so that a destDir of "/foo/bar" cannot be
	// confused with "/foo/barbaz".
	destDirSlash := destDir + string(os.PathSeparator)

	for _, f := range r.File {
		// Skip symlink entries: on some ZIP implementations the symlink
		// target is stored as the file body. Extracting symlinks could
		// allow a malicious archive to point outside the destination tree
		// and affect subsequent file operations.
		if f.Mode()&os.ModeSymlink != 0 {
			continue
		}

		name := filepath.ToSlash(f.Name)
		name = strings.TrimPrefix(name, prefix)
		if name == "" {
			continue
		}
		destPath := filepath.Join(destDir, filepath.FromSlash(name))

		// Guard against path traversal using a HasPrefix check on the
		// cleaned path. filepath.Rel is not used here because on Windows
		// it can return non-".." results for paths on different drives
		// that still escape the destination directory.
		clean := filepath.Clean(destPath)
		if clean != destDir && !strings.HasPrefix(clean, destDirSlash) {
			return fmt.Errorf("invalid path in zip: %s", f.Name)
		}

		if f.FileInfo().IsDir() {
			if err := os.MkdirAll(destPath, 0755); err != nil {
				return err
			}
			continue
		}
		if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			return err
		}
		out, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, f.Mode())
		if err != nil {
			rc.Close()
			return err
		}
		_, err = io.Copy(out, rc)
		out.Close()
		rc.Close()
		if err != nil {
			return err
		}
	}
	return nil
}

type App struct {
	ctx          context.Context
	metadataPath string // catalog: MediaItem JSON + artwork, stored in config dir (read-only)
	dataPath     string // user data: ROM files, install dirs, .state.json (writable)
	store        *store.Store

	units *storageunits.Manager

	// events is nil in the desktop build, where emit falls through to the Wails
	// runtime. Under -server it is set to the SSE hub's broadcast, and it doubles
	// as the "no native window is attached" flag that the file dialogs check.
	events func(name string, data interface{})

	installMu     sync.RWMutex
	installingFor string
	installCancel context.CancelFunc // non-nil while an install is running
}

// GetActiveInstall returns the item title currently being installed, or "" if idle.
func (a *App) GetActiveInstall() string {
	a.installMu.RLock()
	defer a.installMu.RUnlock()
	return a.installingFor
}

func NewApp() *App {
	return &App{}
}

type Settings struct {
	DataPath string `json:"dataPath"`
}

func configDir() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "PortForge"), nil
}

// GetSettings reports the current settings.
//
// DataPath is derived from the shared StorageUnit list rather than stored: the
// list is the single place a storage location is configured, and a private copy
// here would be a second source of truth that goes stale the moment another
// suite program edits the list.
func (a *App) GetSettings() Settings {
	a.syncDataPath()
	return Settings{DataPath: a.dataPath}
}

// ValidateMediaItemsPath checks the configured paths and returns a human-readable
// warning if anything is off. Returns an empty string when everything looks fine.
func (a *App) ValidateMediaItemsPath() string {
	if a.dataPath == "" {
		return "No user library folder has been selected."
	}
	if info, err := os.Stat(a.dataPath); err != nil || !info.IsDir() {
		return fmt.Sprintf("The user library folder does not exist or is not accessible: %s", a.dataPath)
	}
	if _, err := os.Stat(filepath.Join(a.metadataPath, metadata.PortItemType)); os.IsNotExist(err) {
		return "The MediaItems catalog has not been synced yet. Go to Settings to sync it."
	}
	return ""
}

// SelectROMFiles opens a native multi-file picker for adding ROM files to the
// library. The chosen files are matched by checksum, not by name, so no
// extension filter is applied.
func (a *App) SelectROMFiles() ([]string, error) {
	if err := a.requireNativeDialogs("Choosing files"); err != nil {
		return nil, err
	}
	return wailsruntime.OpenMultipleFilesDialog(a.ctx, wailsruntime.OpenDialogOptions{
		Title: "Select ROM Files",
	})
}

// requireNativeDialogs reports whether a native file dialog can be opened. Under
// -server there is no window to parent one to, and a browser cannot hand back a
// filesystem path: the File System Access API yields opaque handles, and Firefox
// does not implement it at all. Server mode therefore needs its own way to name
// a path, which is a UI decision rather than something to fake here.
func (a *App) requireNativeDialogs(what string) error {
	if a.events == nil {
		return nil
	}
	return fmt.Errorf("%s needs the desktop window; in server mode a browser cannot return a file path", what)
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx

	// In dev mode the project-local mediaitem folder is used; otherwise the
	// catalog lives in the OS config directory and is never user-configurable.
	if devMetadataOverride != "" {
		a.metadataPath = devMetadataOverride
	} else if dir, err := configDir(); err == nil {
		a.metadataPath = filepath.Join(dir, "mediaitems")
		_ = os.MkdirAll(a.metadataPath, 0755)
	}

	// The StorageUnit list is shared with every other program in the suite. Nothing
	// is contributed to it here: storage locations are the user's to choose, and a
	// folder one program added silently becomes a destination every other program
	// will write to — a choice the user never made, surfacing in an app they may
	// not have opened.
	if m, err := storageunits.NewManager(); err == nil {
		a.units = m
	}
	a.migrate(legacySettingsPath())

	// Open the library index database. On first run (empty DB) rebuild from the
	// catalog, then index any ROM files already in the user library.
	if dir, err := configDir(); err == nil {
		if st, err := store.Open(filepath.Join(dir, "library.db")); err == nil {
			a.store = st
			// A schema change can discard the derived dependency index, which
			// only a rebuild restores — an empty index would otherwise read as
			// "no port needs a ROM".
			if (st.IsEmpty() || st.NeedsCatalogRebuild()) && a.metadataPath != "" {
				_ = st.RebuildCatalog(a.metadataPath)
			}
			if a.dataPath != "" {
				_ = st.SyncUserROMs(a.dataPath)
				_ = st.ScanUserLibraryUpdates(a.metadataPath, a.dataPath)
			}
		}
	}
}

// GetPlatform returns the host as a platform string with its architecture
// appended: "Linux-x64", "Mac-arm64", "Windows-x64".
//
// Architecture rides in the platform string rather than sitting on its own axis
// because it only matters for the few ports that publish one build per
// architecture — SpaghettiKart ships separate mac-arm64 and mac-intel-x64
// archives — while every other port publishes one build per OS.
//
// A catalog spec may therefore target either form: "Mac" matches any Mac host,
// "Mac-arm64" only Apple Silicon. resolveTargetPlatform does that matching and
// returns the string as the spec declared it, so what reaches engine.Select,
// $platform and the recorded install state is unchanged for every existing
// spec — a step reading `$platform == Windows` still sees "Windows".
func (a *App) GetPlatform() string {
	osName := "Linux"
	switch runtime.GOOS {
	case "windows":
		osName = "Windows"
	case "darwin":
		osName = "Mac"
	}
	arch := runtime.GOARCH
	switch arch {
	case "amd64":
		arch = "x64"
	case "386":
		arch = "x86"
	}
	return osName + "-" + arch
}

// platformBase strips the architecture suffix: "Mac-arm64" → "Mac".
func platformBase(platform string) string {
	if i := strings.Index(platform, "-"); i >= 0 {
		return platform[:i]
	}
	return platform
}

// resolveTargetPlatform picks which platform to build for when the user has not
// chosen one, and returns it spelled the way the spec spells it.
//
// An exact architecture match wins over the bare OS name, so a port shipping
// separate arm64 and x64 builds gets the right one; a port declaring only "Mac"
// still matches any Mac. When nothing matches, the bare OS name is returned so
// the caller reports "no spec for Linux" rather than "no spec for Linux-x64",
// and so a spec declaring no platforms at all — which targets everything — is
// still selected.
func (a *App) resolveTargetPlatform(specs []engine.Spec) string {
	return resolvePlatform(specs, a.GetPlatform())
}

func resolvePlatform(specs []engine.Spec, host string) string {
	base := platformBase(host)
	fallback := ""
	for i := range specs {
		for _, p := range specs[i].TargetPlatforms {
			if p == host {
				return p
			}
			if p == base && fallback == "" {
				fallback = p
			}
		}
	}
	if fallback != "" {
		return fallback
	}
	return base
}

// GetGames returns all VideoGame items from the local mediaitems directory.
func (a *App) GetGames() ([]models.VideoGame, error) {
	return metadata.LoadAll(a.metadataPath)
}

// resolveRomDeps populates ROMDependency.Formats from the catalog for any
// dependency whose Formats array is empty. This allows romDependencies in
// .mediaitem.json to declare only title and _itemType, with format/checksum
// data coming from the standalone ROM MediaItem rather than being duplicated.
func (a *App) resolveRomDeps(v *models.VideoGameVersion) {
	if len(v.ROMDependencies) == 0 {
		return
	}
	if a.store != nil {
		byKey, _ := a.store.GetROMFormatsForVersion(v.ItemTitle)
		for i := range v.ROMDependencies {
			for j := range v.ROMDependencies[i].Options {
				opt := &v.ROMDependencies[i].Options[j]
				if len(opt.Formats) == 0 {
					opt.Formats = byKey[opt.ItemType+"\x00"+opt.Title]
				}
			}
		}
		return
	}
	for i := range v.ROMDependencies {
		for j := range v.ROMDependencies[i].Options {
			opt := &v.ROMDependencies[i].Options[j]
			if len(opt.Formats) > 0 {
				continue
			}
			if rom, _ := metadata.FindRomByTitle(a.metadataPath, opt.ItemType, opt.Title); rom != nil {
				opt.Formats = rom.Formats
			}
		}
	}
}

// loadVersion loads a VideoGameVersion and resolves its romDependencies'
// format data from the ROM catalog.
func (a *App) loadVersion(itemTitle string) (*models.VideoGameVersion, error) {
	v, err := metadata.LoadOneVersion(a.metadataPath, itemTitle)
	if err != nil || v == nil {
		return v, err
	}
	a.resolveRomDeps(v)
	return v, nil
}

func (a *App) GetVersion(itemTitle string) (*models.VideoGameVersion, error) {
	return a.loadVersion(itemTitle)
}

func (a *App) GetVersions() ([]models.VideoGameVersion, error) {
	if a.store != nil {
		return a.store.GetVersions()
	}
	return metadata.LoadAllVersions(a.metadataPath)
}

// GetInstallState returns the install state for a VideoGameVersion, or nil if not yet installed.
func (a *App) GetInstallState(itemTitle string) (*models.InstallState, error) {
	versionDir := filepath.Join(a.dataPath, metadata.PortItemType, itemTitle)
	return metadata.ReadInstallState(versionDir)
}

// GetInstallPrompts returns the user-facing arg prompts for a version's .installation.json,
// with ROM readiness pre-populated per option. Returns nil if no spec exists or the spec
// has no args (meaning the install needs no user input).
func (a *App) GetInstallPrompts(itemTitle string) ([]models.ArgPrompt, error) {
	version, err := a.loadVersion(itemTitle)
	if err != nil || version == nil {
		return nil, err
	}
	specs, err := metadata.LoadInstallationSpecs(a.metadataPath, itemTitle)
	if err != nil {
		return nil, err
	}
	spec := a.findMatchingSpec(specs)
	if spec == nil || len(spec.Args) == 0 {
		return nil, nil
	}

	// The engine orders prompts deterministically; map them onto the UI's own
	// shape so the frontend contract doesn't follow the engine's.
	var prompts []models.ArgPrompt
	for _, p := range spec.Prompts() {
		prompt := models.ArgPrompt{Name: p.Name, Type: p.Type, Label: p.Label}
		for _, opt := range p.Options {
			prompt.Options = append(prompt.Options, models.ArgOption{
				Value: opt.Value,
				Label: opt.Label,
			})
		}
		prompts = append(prompts, prompt)
	}
	return prompts, nil
}

// CancelInstall cancels a running install. The install goroutine will stop at the
// next step boundary or when the current command exits.
func (a *App) CancelInstall() {
	a.installMu.Lock()
	defer a.installMu.Unlock()
	if a.installCancel != nil {
		a.installCancel()
	}
}

func (a *App) InstallVersion(itemTitle string, args map[string]string, specVersion, targetPlatform string) error {
	installCtx, cancel := context.WithCancel(a.ctx)
	// Only one install may run at a time. Claiming the slot and recording the
	// cancel func has to happen under the same lock, or a second caller can
	// overwrite the first one's cancel func and leave it unstoppable.
	a.installMu.Lock()
	if a.installingFor != "" {
		busy := a.installingFor
		a.installMu.Unlock()
		cancel()
		return fmt.Errorf("already installing %s", busy)
	}
	a.installingFor = itemTitle
	a.installCancel = cancel
	a.installMu.Unlock()
	defer func() {
		a.installMu.Lock()
		a.installingFor = ""
		a.installCancel = nil
		a.installMu.Unlock()
		cancel()
	}()

	a.emit("install:started", map[string]interface{}{
		"itemTitle": itemTitle,
	})

	version, err := a.loadVersion(itemTitle)
	if err != nil {
		return err
	}
	if version == nil {
		return fmt.Errorf("version not found: %s", itemTitle)
	}

	versionDir := filepath.Join(a.dataPath, metadata.PortItemType, itemTitle)

	specFile, err := metadata.LoadSpecFile(a.metadataPath, itemTitle)
	if err != nil {
		return fmt.Errorf("failed to load installation spec: %w", err)
	}
	if specFile == nil {
		return fmt.Errorf("no install spec for %s", itemTitle)
	}
	specs := specFile.Specs
	order := engine.VersionOrder(specs)

	// An empty target platform means "this machine". An empty version is
	// ambiguous once a build can span several, so it is resolved here rather
	// than left to Select: the file's stated default, else the newest declared.
	if targetPlatform == "" {
		targetPlatform = a.resolveTargetPlatform(specs)
	}
	if specVersion == "" {
		specVersion = specFile.DefaultVersion
	}
	if specVersion == "" && len(order) > 0 {
		specVersion = order[len(order)-1] // the order is oldest first
	}

	spec := engine.Select(specs, targetPlatform, specVersion)
	if spec == nil {
		return fmt.Errorf("no install spec for %s at version %q on %s", itemTitle, specVersion, targetPlatform)
	}
	resolvedArgs, err := spec.ResolveArgs(args)
	if err != nil {
		return err
	}

	// A $name nothing declares is left in the string verbatim by the engine, so
	// a spec with a typo builds for as long as it takes to reach the step that
	// carries it and then fails with whatever the invoked tool made of the
	// literal text. Catalog specs are synced rather than written here, so this
	// is checked before anything is downloaded or compiled.
	//
	// Only the install path checks this. Uninstalling deliberately does not:
	// refusing to remove a port because its uninstallSteps have a typo would
	// strand the user with something they cannot get rid of.
	if missing := engine.UndeclaredArgs(spec); len(missing) > 0 {
		return fmt.Errorf("the install spec for %s uses %s, which it does not declare — this is a problem with the spec, not with your setup",
			itemTitle, joinArgNames(missing))
	}

	exes, err := a.runSpec(installCtx, spec, spec.Steps, resolvedArgs, version, versionDir, engine.Options{
		RequireExecutable: true,
		Platform:          targetPlatform,
		Version:           specVersion,
		VersionOrder:      order,
	})
	if err != nil {
		return err
	}
	if err := a.writeInstallState(versionDir, spec, exes, resolvedArgs, targetPlatform); err != nil {
		return err
	}
	// Snapshot the catalog MediaItem into the user library so we can detect
	// future updates by comparing the two copies.
	_ = a.copyToUserLibrary(metadata.PortItemType, itemTitle)
	// Resync ROM index so any ROMs staged during the build are reflected.
	if a.store != nil {
		_ = a.store.SyncUserROMs(a.dataPath)
	}
	return nil
}

// findMatchingSpec returns the first spec in the array whose targetPlatforms includes
// the current platform (or has no platform restriction). Returns nil if none match.
// Used where no specific version was asked for — status checks and prompts.
func (a *App) findMatchingSpec(specs []engine.Spec) *engine.Spec {
	return engine.Select(specs, a.resolveTargetPlatform(specs), "")
}

// GetSpecVersions returns the versions a port declares, each with the platforms
// it can be built for and one of them marked as the default to offer. Every
// version is returned regardless of host platform: choosing a build target is the
// user's, via the platform picker, since PortForge can build for platforms it does
// not run on.
//
// The spec file declares versions oldest first, because that order is the
// hierarchy that ordered `if` conditions compare against. This reverses it, since
// a picker should lead with the newest release.
func (a *App) GetSpecVersions(itemTitle string) ([]engine.SpecVersion, error) {
	file, err := metadata.LoadSpecFile(a.metadataPath, itemTitle)
	if err != nil || file == nil {
		return nil, err
	}
	declared := file.Versions()
	out := make([]engine.SpecVersion, len(declared))
	for i, v := range declared {
		out[len(declared)-1-i] = v
	}
	return out, nil
}

// CleanBuildDir removes build artefacts for a version after a failed build.
// It uses the buildPaths array from the matching spec when available,
// falling back to removing common build directories (.build, build).
func (a *App) CleanBuildDir(itemTitle string) error {
	versionDir := filepath.Join(a.dataPath, metadata.PortItemType, itemTitle)
	specs, _ := metadata.LoadInstallationSpecs(a.metadataPath, itemTitle)
	if spec := a.findMatchingSpec(specs); spec != nil && len(spec.BuildPaths) > 0 {
		for _, p := range spec.BuildPaths {
			if err := os.RemoveAll(filepath.Join(versionDir, p)); err != nil {
				return err
			}
		}
		return nil
	}
	_ = os.RemoveAll(filepath.Join(versionDir, ".build"))
	_ = os.RemoveAll(filepath.Join(versionDir, "build"))
	return nil
}

// UninstallVersion runs any uninstallSteps from the spec file, then removes
// the install directory and clears the .state.json. If no uninstallSteps are defined
// it simply removes the install/ subdirectory.
func (a *App) UninstallVersion(itemTitle string) error {
	versionDir := filepath.Join(a.dataPath, metadata.PortItemType, itemTitle)

	var cleanupErr error
	specs, _ := metadata.LoadInstallationSpecs(a.metadataPath, itemTitle)
	// Uninstall with the steps of the version that was installed, not whichever
	// spec happens to match first — they can differ between versions.
	installed, _ := metadata.ReadInstallState(versionDir)
	wantVersion, wantPlatform := "", a.resolveTargetPlatform(specs)
	if installed != nil {
		wantVersion = installed.InstalledVersion
		if installed.TargetPlatform != "" {
			wantPlatform = installed.TargetPlatform
		}
	}
	spec := engine.Select(specs, wantPlatform, wantVersion)
	if spec == nil {
		spec = a.findMatchingSpec(specs)
	}
	if spec != nil && len(spec.UninstallSteps) > 0 {
		version, _ := a.loadVersion(itemTitle)
		// The tools that built this item may no longer be installed, so the
		// dependency check is skipped — but they stay on the run allowlist.
		// The uninstall steps get the same $platform/$version the build did, so
		// a spec that branched on them tears down what it actually created.
		_, cleanupErr = a.runSpec(a.ctx, spec, spec.UninstallSteps, map[string]string{}, version, versionDir, engine.Options{
			SkipDependencyCheck: true,
			Platform:            wantPlatform,
			Version:             wantVersion,
			VersionOrder:        engine.VersionOrder(specs),
		})
	} else {
		cleanupErr = os.RemoveAll(filepath.Join(versionDir, "install"))
	}

	// Always clear the installed flag, even if cleanup steps partially failed.
	state, err := metadata.ReadInstallState(versionDir)
	if err != nil || state == nil {
		state = &models.InstallState{}
	}
	state.Installed = false
	state.InstalledVersion = ""
	state.InstallDir = ""
	state.Executables = nil
	_ = metadata.WriteInstallState(versionDir, state)

	if cleanupErr != nil {
		return fmt.Errorf("uninstall failed: %w", cleanupErr)
	}
	return nil
}

// LaunchVersion launches an installed executable. If executablePath is empty the primary is used.
func (a *App) LaunchVersion(itemTitle string, executablePath string) error {
	versionDir := filepath.Join(a.dataPath, metadata.PortItemType, itemTitle)
	state, err := metadata.ReadInstallState(versionDir)
	if err != nil {
		return err
	}
	if state == nil || !state.Installed {
		return fmt.Errorf("not installed: %s", itemTitle)
	}

	exes := state.Executables
	if len(exes) == 0 {
		return fmt.Errorf("no executable path configured for this version — add \"executablePath\" to the download entry in the .mediaitem.json and reinstall")
	}

	targetPath := executablePath
	if targetPath == "" {
		targetPath = exes[0].Path
	}

	absPath, err := filepath.Abs(filepath.Join(versionDir, targetPath))
	if err != nil {
		return err
	}

	if runtime.GOOS != "windows" {
		_ = os.Chmod(absPath, 0755)
	}

	cmd := newCommand(absPath)
	cmd.Dir = filepath.Dir(absPath)
	if err := cmd.Start(); err != nil {
		return err
	}

	a.emit("game:started", map[string]interface{}{
		"itemTitle": itemTitle,
	})

	startTime := time.Now()
	go func() {
		cmd.Wait()
		playSeconds := int64(time.Since(startTime).Seconds())

		if s, err := metadata.ReadInstallState(versionDir); err == nil && s != nil {
			s.TotalPlaySeconds += playSeconds
			s.LastPlayedAt = time.Now().UTC().Format(time.RFC3339)
			metadata.WriteInstallState(versionDir, s)
		}

		a.emit("game:ended", map[string]interface{}{
			"itemTitle":   itemTitle,
			"playSeconds": playSeconds,
		})
	}()

	return nil
}

// GetInstallSize returns the total bytes occupied by a version's data directory,
// which includes the built game plus any source and build artefacts left behind.
// Returns 0 when the directory does not exist.
func (a *App) GetInstallSize(itemTitle string) (int64, error) {
	dir := filepath.Join(a.dataPath, metadata.PortItemType, itemTitle)
	var total int64
	err := filepath.WalkDir(dir, func(_ string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil //nolint:nilerr // an unreadable entry should not fail the total
		}
		if info, err := d.Info(); err == nil {
			total += info.Size()
		}
		return nil
	})
	if os.IsNotExist(err) {
		return 0, nil
	}
	return total, err
}

// LibraryStatus is everything the library grid needs to draw one card's badge and
// decide which filter chips it belongs to.
type LibraryStatus struct {
	Installed        bool   `json:"installed"`
	InstalledVersion string `json:"installedVersion,omitempty"`
	LatestVersion    string `json:"latestVersion,omitempty"`
	HasUpdate        bool   `json:"hasUpdate"`
	Buildable        bool   `json:"buildable"` // the port declares an install spec at all
	HasROMDeps       bool   `json:"hasRomDeps"`
	ROMsReady        bool   `json:"romsReady"`
	LastPlayedAt     string `json:"lastPlayedAt,omitempty"`
}

// GetLibraryStatus returns the status of every version in the catalog, keyed by
// item title. The grid needs this for all items at once, so it is gathered in a
// single call rather than four per card.
func (a *App) GetLibraryStatus() (map[string]LibraryStatus, error) {
	versions, err := a.GetVersions()
	if err != nil {
		return nil, err
	}

	// The listing rows from the store carry no romDependencies of their own, so
	// readiness comes from the index in a single query rather than from versions.
	var romReady map[string]bool
	if a.store != nil {
		romReady, _ = a.store.GetVersionROMReadiness()
	} else {
		romReady = a.romReadinessFromDisk(versions)
	}

	out := make(map[string]LibraryStatus, len(versions))
	for i := range versions {
		v := &versions[i]
		st := LibraryStatus{HasUpdate: a.GetItemUpdate(v.ItemTitle)}

		// Buildable means the port declares an install spec, not that one targets
		// this host: PortForge builds for platforms it does not run on, so the host
		// is the platform picker's default rather than a filter.
		//
		// Declaration order is chronological, oldest first, so the newest version
		// is the last one declared — not the first, which is where a file's
		// defaultVersion often sits when the newest release is a pre-release.
		if specs, err := metadata.LoadInstallationSpecs(a.metadataPath, v.ItemTitle); err == nil && len(specs) > 0 {
			st.Buildable = true
			if order := engine.VersionOrder(specs); len(order) > 0 {
				st.LatestVersion = order[len(order)-1]
			}
		}

		versionDir := filepath.Join(a.dataPath, metadata.PortItemType, v.ItemTitle)
		if state, err := metadata.ReadInstallState(versionDir); err == nil && state != nil {
			st.Installed = state.Installed
			st.InstalledVersion = state.InstalledVersion
			st.LastPlayedAt = state.LastPlayedAt
		}

		ready, hasDeps := romReady[v.ItemTitle]
		st.HasROMDeps = hasDeps
		st.ROMsReady = !hasDeps || ready

		out[v.ItemTitle] = st
	}
	return out, nil
}

// romReadinessFromDisk is the fallback for when the SQLite index is unavailable:
// it reads each version's dependencies from the catalog and checks the ROM library
// on disk. Versions with no dependencies are omitted, matching the index query.
func (a *App) romReadinessFromDisk(versions []models.VideoGameVersion) map[string]bool {
	hashes, err := metadata.ScanROMLibrary(a.dataPath)
	if err != nil {
		return nil
	}
	out := make(map[string]bool)
	for i := range versions {
		v, err := a.loadVersion(versions[i].ItemTitle)
		if err != nil || v == nil || len(v.ROMDependencies) == 0 {
			continue
		}
		// Every required requirement must be met by one of its options; the
		// optional ones add content and never hold a port back.
		ready := true
		for _, req := range v.ROMDependencies {
			if !req.Required {
				continue
			}
			met := false
			for _, opt := range req.Options {
				for _, f := range opt.Formats {
					if f.Checksums.MD5 == "" {
						continue
					}
					if _, ok := hashes[f.Checksums.MD5]; ok {
						met = true
					}
				}
			}
			if !met {
				ready = false
			}
		}
		out[v.ItemTitle] = ready
	}
	return out
}

// GetROMStatus returns MD5 → present for every format needed by a version's ROM dependencies.
func (a *App) GetROMStatus(itemTitle string) (map[string]bool, error) {
	version, err := a.loadVersion(itemTitle)
	if err != nil {
		return nil, err
	}
	if version == nil {
		return nil, fmt.Errorf("version not found: %s", itemTitle)
	}

	status := make(map[string]bool)
	for _, rom := range version.ROMDependencies.AllOptions() {
		for _, f := range rom.Formats {
			if f.Checksums.MD5 == "" {
				continue
			}
			if a.store != nil {
				status[f.Checksums.MD5] = a.store.IsROMPresent(f.Checksums.MD5)
			} else {
				status[f.Checksums.MD5] = false
			}
		}
	}
	return status, nil
}

// GetROMLibrary returns every ROM requirement declared by every port, together
// with one presence map covering all of them.
//
// It exists so the ROM library is a single call rather than one GetROMStatus per
// port: the presence lookups collapse to one pass, and a dump two ports both
// accept is resolved once. Ports declaring no requirements are omitted — the
// view is about ROMs, and an empty heading says nothing.
func (a *App) GetROMLibrary() (*models.ROMLibrary, error) {
	versions, err := a.GetVersions()
	if err != nil {
		return nil, err
	}

	// Both fields are non-nil even when nothing matches, so the frontend never
	// has to distinguish an empty library from a missing one.
	lib := &models.ROMLibrary{Ports: []models.ROMLibraryPort{}, Status: map[string]bool{}}
	for _, v := range versions {
		// loadVersion is what hydrates an option's formats from the standalone
		// ROM MediaItem. Going through it rather than reading the summary rows
		// keeps this view on the same data the game page sees.
		full, err := a.loadVersion(v.ItemTitle)
		if err != nil || full == nil || len(full.ROMDependencies) == 0 {
			continue
		}
		lib.Ports = append(lib.Ports, models.ROMLibraryPort{
			ItemTitle:    full.ItemTitle,
			Title:        full.Title,
			Requirements: full.ROMDependencies,
		})
		for _, opt := range full.ROMDependencies.AllOptions() {
			for _, f := range opt.Formats {
				if f.Checksums.MD5 == "" {
					continue
				}
				if _, seen := lib.Status[f.Checksums.MD5]; seen {
					continue
				}
				lib.Status[f.Checksums.MD5] = a.store != nil && a.store.IsROMPresent(f.Checksums.MD5)
			}
		}
	}
	return lib, nil
}

// AddROMFiles copies or moves dropped files into the matching VideoGameRom folder.
// MatchDroppedROMs calculates the MD5 of each dropped file and compares it
// against every VideoGameRom format in the library. Returns matched and unmatched files.
func (a *App) MatchDroppedROMs(paths []string) (*models.ROMDropSummary, error) {
	// Build md5 → {itemTitle, ext, itemType, filename} index from the DB when available.
	type romEntry struct {
		romTitle  string
		romType   string
		formatExt string
		filename  string
	}
	index := make(map[string]romEntry)

	if a.store != nil {
		dbIndex, err := a.store.GetROMCatalogIndex()
		if err != nil {
			return nil, err
		}
		for md5, e := range dbIndex {
			index[md5] = romEntry{e.ItemTitle, e.ItemType, e.Ext, e.Filename}
		}
	} else {
		allRoms, err := metadata.LoadAllRoms(a.metadataPath)
		if err != nil {
			return nil, err
		}
		for _, rom := range allRoms {
			for _, f := range rom.Formats {
				if f.Checksums.MD5 != "" {
					index[strings.ToLower(f.Checksums.MD5)] = romEntry{rom.ItemTitle, rom.ItemType, f.Ext, f.Filename}
				}
			}
		}
	}

	result := &models.ROMDropSummary{}
	for _, path := range paths {
		hash, err := fileMD5(path)
		if err != nil {
			result.Unmatched = append(result.Unmatched, filepath.Base(path))
			continue
		}
		if entry, ok := index[strings.ToLower(hash)]; ok {
			result.Matched = append(result.Matched, models.ROMFileMatch{
				FilePath:       path,
				FileName:       filepath.Base(path),
				ROMTitle:       entry.romTitle,
				ROMType:        entry.romType,
				FormatExt:      entry.formatExt,
				FormatFilename: entry.filename,
			})
		} else {
			result.Unmatched = append(result.Unmatched, filepath.Base(path))
		}
	}
	return result, nil
}

// ImportROMs copies or moves previously matched ROM files into the user data directory.
func (a *App) ImportROMs(matches []models.ROMFileMatch, move bool) error {
	for _, m := range matches {
		romType := m.ROMType
		if romType == "" {
			romType = "VideoGameRom"
		}
		destDir := filepath.Join(a.dataPath, romType, m.ROMTitle)
		if err := os.MkdirAll(destDir, 0755); err != nil {
			return fmt.Errorf("failed to create directory for %s: %w", m.ROMTitle, err)
		}

		// Rename to the catalog's canonical filename, but keep the dropped
		// file's actual extension in case it differs from the catalog entry.
		destName := m.FileName
		if m.FormatFilename != "" {
			ext := filepath.Ext(m.FileName)
			stem := strings.TrimSuffix(m.FormatFilename, filepath.Ext(m.FormatFilename))
			destName = stem + ext
		}

		dest := filepath.Join(destDir, destName)
		if move {
			if err := os.Rename(m.FilePath, dest); err != nil {
				if err2 := copyFile(m.FilePath, dest); err2 != nil {
					return fmt.Errorf("failed to move %s: %w", m.FileName, err2)
				}
				os.Remove(m.FilePath)
			}
		} else {
			if err := copyFile(m.FilePath, dest); err != nil {
				return fmt.Errorf("failed to copy %s: %w", m.FileName, err)
			}
		}
	}
	if a.store != nil {
		_ = a.store.SyncUserROMs(a.dataPath)
	}
	return nil
}

func (a *App) AddROMFiles(itemTitle string, paths []string, move bool) ([]string, error) {
	version, err := a.loadVersion(itemTitle)
	if err != nil {
		return nil, err
	}
	if version == nil {
		return nil, fmt.Errorf("version not found: %s", itemTitle)
	}

	romsByMD5 := make(map[string]string)
	for _, rom := range version.ROMDependencies.AllOptions() {
		for _, f := range rom.Formats {
			romsByMD5[f.Checksums.MD5] = rom.Title
		}
	}

	type romItemEntry struct {
		title    string
		itemType string
	}
	romItemByMD5 := make(map[string]romItemEntry)
	if a.store != nil {
		if idx, err := a.store.GetROMCatalogIndex(); err == nil {
			for md5, e := range idx {
				romItemByMD5[md5] = romItemEntry{e.ItemTitle, e.ItemType}
			}
		}
	} else if allRoms, err := metadata.LoadAllRoms(a.metadataPath); err == nil {
		for _, r := range allRoms {
			for _, f := range r.Formats {
				romItemByMD5[f.Checksums.MD5] = romItemEntry{r.ItemTitle, r.ItemType}
			}
		}
	}

	var matched []string
	for _, src := range paths {
		hash, err := fileMD5(src)
		if err != nil {
			continue
		}
		title, ok := romsByMD5[hash]
		if !ok {
			continue
		}

		var destDir string
		if entry, ok := romItemByMD5[hash]; ok {
			romType := entry.itemType
			if romType == "" {
				romType = "VideoGameRom"
			}
			destDir = filepath.Join(a.dataPath, romType, entry.title)
		} else {
			destDir = filepath.Join(a.dataPath, metadata.PortItemType, itemTitle)
		}
		if err := os.MkdirAll(destDir, 0755); err != nil {
			return matched, fmt.Errorf("failed to create destination directory: %w", err)
		}

		dest := filepath.Join(destDir, filepath.Base(src))
		if move {
			if err := os.Rename(src, dest); err != nil {
				if err2 := copyFile(src, dest); err2 != nil {
					return matched, fmt.Errorf("failed to move %s: %w", filepath.Base(src), err2)
				}
				os.Remove(src)
			}
		} else {
			if err := copyFile(src, dest); err != nil {
				return matched, fmt.Errorf("failed to copy %s: %w", filepath.Base(src), err)
			}
		}
		matched = append(matched, title)
	}

	return matched, nil
}

// ── helpers ──────────────────────────────────────────────────────────────────

func (a *App) writeInstallState(versionDir string, spec *engine.Spec, exes []models.ExecutableEntry, args map[string]string, targetPlatform string) error {
	primaryPath := ""
	if len(exes) > 0 {
		primaryPath = exes[0].Path
	}
	version := ""
	if spec != nil {
		version = spec.Version
	}
	state := &models.InstallState{
		Installed:        true,
		InstalledVersion: version,
		InstallDir:       filepath.Dir(primaryPath),
		Executables:      exes,
		ActiveMods:       []string{},
		Args:             args,
		TargetPlatform:   targetPlatform,
		InstalledAt:      time.Now().UTC().Format(time.RFC3339),
	}
	if err := metadata.WriteInstallState(versionDir, state); err != nil {
		return fmt.Errorf("failed to save install state: %w", err)
	}
	a.emitProgress("done", 100)
	return nil
}

func (a *App) emitProgress(phase string, percent int) {
	a.emit("install:progress", map[string]interface{}{
		"itemTitle": a.GetActiveInstall(),
		"phase":     phase,
		"percent":   percent,
	})
}

type progressReader struct {
	r       io.Reader
	total   int64
	read    int64
	onPct   func(int)
	lastPct int
}

func (pr *progressReader) Read(p []byte) (n int, err error) {
	n, err = pr.r.Read(p)
	pr.read += int64(n)
	if pr.total > 0 {
		pct := int(pr.read * 100 / pr.total)
		if pct != pr.lastPct {
			pr.lastPct = pct
			pr.onPct(pct)
		}
	}
	return
}

func fileMD5(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := md5.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()
	_, err = io.Copy(out, in)
	return err
}

// ── StorageUnits ─────────────────────────────────────────────────────────────
// The shared, ordered list of folders finished output is written to. The file
// behind these is read and written by every program in the suite, so the
// behaviour lives in the storageunits package and matches the suite README
// rather than being reimplemented here.

// syncDataPath points this program's storage root at the highest-priority unit.
//
// By priority, deliberately not by reachability. Falling through to the next unit
// when the top one is unplugged would silently relocate the library: installs
// recorded under one folder would be looked for in another, and the user would be
// told their games are missing rather than that a drive is disconnected. An
// unreachable top unit is reported as unavailable and stays the root.
//
// This is a single-unit reading of a multi-unit list, and it is deliberate.
// Placing across several units needs a lookup that survives a unit being offline —
// the shared item index — and until that exists PortForge uses one.
func (a *App) syncDataPath() {
	if a.units == nil {
		return
	}
	units, err := a.units.List()
	if err != nil || len(units) == 0 {
		a.dataPath = ""
		return
	}
	a.dataPath = units[0].Path
}

// GetStorageUnits returns the units in priority order with live space figures.
func (a *App) GetStorageUnits() ([]storageunits.Unit, error) {
	if a.units == nil {
		return nil, fmt.Errorf("the shared storage settings could not be opened")
	}
	return a.units.List()
}

// AddStorageUnit prompts for a folder and appends it as the lowest-priority
// unit. An empty return means the user cancelled, which is not an error.
func (a *App) AddStorageUnit() (*storageunits.Unit, error) {
	if a.units == nil {
		return nil, fmt.Errorf("the shared storage settings could not be opened")
	}
	if err := a.requireNativeDialogs("Choosing a folder"); err != nil {
		return nil, err
	}
	path, err := wailsruntime.OpenDirectoryDialog(a.ctx, wailsruntime.OpenDialogOptions{
		Title: "Choose a folder to store media in",
	})
	if err != nil || path == "" {
		return nil, err
	}
	u, err := a.units.Add(path)
	if err != nil {
		return nil, err
	}
	a.syncDataPath()
	return &u, nil
}

// RemoveStorageUnit forgets a location. It never touches what is stored there:
// content on a removed unit is the user's, exactly as on a drive they unplugged.
func (a *App) RemoveStorageUnit(id string) error {
	if a.units == nil {
		return fmt.Errorf("the shared storage settings could not be opened")
	}
	if err := a.units.Remove(id); err != nil {
		return err
	}
	a.syncDataPath()
	return nil
}

// RenameStorageUnit changes a unit's display label only.
func (a *App) RenameStorageUnit(id, name string) error {
	if a.units == nil {
		return fmt.Errorf("the shared storage settings could not be opened")
	}
	return a.units.Rename(id, name)
}

// ReorderStorageUnits sets the priority order. It takes every id exactly once,
// so a stale UI cannot drop or invent a unit by reordering.
func (a *App) ReorderStorageUnits(ids []string) error {
	if a.units == nil {
		return fmt.Errorf("the shared storage settings could not be opened")
	}
	if err := a.units.Reorder(ids); err != nil {
		return err
	}
	a.syncDataPath()
	return nil
}

// OpenStorageUnit shows a unit's folder in the desktop file manager.
func (a *App) OpenStorageUnit(id string) error {
	units, err := a.GetStorageUnits()
	if err != nil {
		return err
	}
	for _, u := range units {
		if u.ID != id {
			continue
		}
		if u.Unreachable {
			return fmt.Errorf("%s can't be opened — it isn't connected right now", u.Name)
		}
		return openInFileManager(u.Path)
	}
	return fmt.Errorf("no such save location")
}
