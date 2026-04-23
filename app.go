package main

import (
	"archive/zip"
	"bufio"
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"portforge/metadata"
	"portforge/models"
	"regexp"
	"runtime"
	"sort"
	"sync"
	"strings"
	"time"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

var argRe = regexp.MustCompile(`\$\{(\w+)\}|\$(\w+)`)

// httpClient is used for all outbound requests. The download timeout is kept
// generous (30 min) to accommodate large game source archives over slow links,
// while the API timeout is short since those responses are tiny.
var (
	httpClient    = &http.Client{Timeout: 30 * time.Minute}
	httpAPIClient = &http.Client{Timeout: 15 * time.Second}
)

const (
	mediaItemsZipURL  = "https://github.com/portforge/portforge-mediaitems/archive/refs/heads/main.zip"
	mediaItemsAPIURL  = "https://api.github.com/repos/portforge/portforge-mediaitems/commits/main"
	mediaItemsSHAFile = ".portforge-sha"
)

// GetDefaultPaths returns the platform-appropriate default locations for the
// MediaItems library and user library folders.
func (a *App) GetDefaultPaths() map[string]string {
	home, _ := os.UserHomeDir()
	var mediaItems, library string
	switch runtime.GOOS {
	case "windows":
		appData := os.Getenv("APPDATA")
		mediaItems = filepath.Join(appData, "PortForge", "MediaItems")
		library = filepath.Join(appData, "PortForge", "Library")
	case "darwin":
		mediaItems = filepath.Join(home, "Library", "Application Support", "PortForge", "MediaItems")
		library = filepath.Join(home, "Library", "Application Support", "PortForge", "Library")
	default:
		dataHome := os.Getenv("XDG_DATA_HOME")
		if dataHome == "" {
			dataHome = filepath.Join(home, ".local", "share")
		}
		mediaItems = filepath.Join(dataHome, "PortForge", "MediaItems")
		library = filepath.Join(dataHome, "PortForge", "Library")
	}
	return map[string]string{
		"mediaItemsPath": mediaItems,
		"dataPath":       library,
	}
}

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
		if _, err := os.Stat(filepath.Join(a.metadataPath, "VideoGameVersion")); err == nil {
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

// SyncMediaItems downloads the latest MediaItems from GitHub into a temp
// directory, then copies all files over destDir with overwrite, leaving any
// local-only files untouched. Records the commit SHA when done.
func (a *App) SyncMediaItems(destDir string) error {
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

	wailsruntime.EventsEmit(a.ctx, "mediaitems:progress", map[string]interface{}{"phase": "downloading", "percent": 0})
	resp, err := httpClient.Get(mediaItemsZipURL)
	if err != nil {
		tmp.Close()
		return err
	}
	pr := &progressReader{
		r:     resp.Body,
		total: resp.ContentLength,
		onPct: func(pct int) {
			wailsruntime.EventsEmit(a.ctx, "mediaitems:progress", map[string]interface{}{"phase": "downloading", "percent": pct})
		},
	}
	_, err = io.Copy(tmp, pr)
	resp.Body.Close()
	tmp.Close()
	if err != nil {
		return err
	}

	// Extract into a temp directory.
	wailsruntime.EventsEmit(a.ctx, "mediaitems:progress", map[string]interface{}{"phase": "extracting", "percent": 0})
	tmpDir, err := os.MkdirTemp("", "portforge-mediaitems-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmpDir)

	if err := extractZipStrip1(tmpZip, tmpDir); err != nil {
		return fmt.Errorf("extraction failed: %w", err)
	}

	// Copy extracted files over destDir, overwriting existing files.
	wailsruntime.EventsEmit(a.ctx, "mediaitems:progress", map[string]interface{}{"phase": "copying", "percent": 0})
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

	wailsruntime.EventsEmit(a.ctx, "mediaitems:progress", map[string]interface{}{"phase": "done", "percent": 100})
	return nil
}

// copyDirMerge copies all files from src into dst, overwriting existing files.
// Directories in dst that are not in src are left untouched.
func copyDirMerge(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
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
	for _, f := range r.File {
		name := filepath.ToSlash(f.Name)
		name = strings.TrimPrefix(name, prefix)
		if name == "" {
			continue
		}
		destPath := filepath.Join(destDir, filepath.FromSlash(name))
		rel, err := filepath.Rel(destDir, destPath)
		if err != nil || strings.HasPrefix(rel, "..") {
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
	metadataPath string // library: .mediaitem.json, .install.json, artwork (read-only)
	dataPath     string // user data: ROM files, install dirs, .state.json (writable)

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
	MediaItemsPath string `json:"mediaItemsPath"`
	DataPath       string `json:"dataPath"`
}

func settingsFilePath() (string, error) {
	dir, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "PortForge", "settings.json"), nil
}

// GetSettings returns the current app settings.
func (a *App) GetSettings() Settings {
	return Settings{MediaItemsPath: a.metadataPath, DataPath: a.dataPath}
}

// ValidateMediaItemsPath checks both configured paths and returns a human-readable
// warning if anything is off. Returns an empty string when everything looks fine.
func (a *App) ValidateMediaItemsPath() string {
	if a.metadataPath == "" {
		return "No MediaItems library folder has been selected."
	}
	info, err := os.Stat(a.metadataPath)
	if err != nil || !info.IsDir() {
		return fmt.Sprintf("The MediaItems library folder does not exist or is not accessible: %s", a.metadataPath)
	}
	if _, err := os.Stat(filepath.Join(a.metadataPath, "VideoGameVersion")); os.IsNotExist(err) {
		return fmt.Sprintf("The library folder does not contain a VideoGameVersion directory. Check that %q is the right folder.", a.metadataPath)
	}
	if a.dataPath == "" {
		return "No user data folder has been selected."
	}
	if info, err := os.Stat(a.dataPath); err != nil || !info.IsDir() {
		return fmt.Sprintf("The user data folder does not exist or is not accessible: %s", a.dataPath)
	}
	return ""
}

// SaveSettings persists both paths and applies them immediately.
func (a *App) SaveSettings(mediaItemsPath string, dataPath string) error {
	a.metadataPath = mediaItemsPath
	a.dataPath = dataPath
	path, err := settingsFilePath()
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	data, err := json.Marshal(Settings{MediaItemsPath: mediaItemsPath, DataPath: dataPath})
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// SelectFolder opens a native folder picker and returns the chosen path.
func (a *App) SelectFolder() (string, error) {
	return wailsruntime.OpenDirectoryDialog(a.ctx, wailsruntime.OpenDialogOptions{
		Title: "Select Folder",
	})
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	if path, err := settingsFilePath(); err == nil {
		if data, err := os.ReadFile(path); err == nil {
			var s Settings
			if json.Unmarshal(data, &s) == nil {
				a.metadataPath = s.MediaItemsPath
				a.dataPath = s.DataPath
			}
		}
	}
}

// GetPlatform returns the current OS as a platform string matching the schema.
func (a *App) GetPlatform() string {
	switch runtime.GOOS {
	case "windows":
		return "Windows"
	case "darwin":
		return "Mac"
	default:
		return "Linux"
	}
}

// GetGames returns all VideoGame items from the local mediaitems directory.
func (a *App) GetGames() ([]models.VideoGame, error) {
	return metadata.LoadAll(a.metadataPath)
}

// GetVersions returns all VideoGameVersion items from the local mediaitems directory.
func (a *App) GetVersions() ([]models.VideoGameVersion, error) {
	return metadata.LoadAllVersions(a.metadataPath)
}

// GetRoms returns all VideoGameRom items from the local mediaitems directory.
func (a *App) GetRoms() ([]models.VideoGameRom, error) {
	return metadata.LoadAllRoms(a.metadataPath)
}

// GetRomLibraryStatus returns a map of ROM itemTitle → whether a file is present.
func (a *App) GetRomLibraryStatus() (map[string]bool, error) {
	roms, err := metadata.LoadAllRoms(a.metadataPath)
	if err != nil {
		return nil, err
	}
	result := make(map[string]bool)
	for _, rom := range roms {
		romDir := filepath.Join(a.dataPath, "VideoGameRom", rom.ItemTitle)
		hashes, err := metadata.ScanROMs(romDir)
		result[rom.ItemTitle] = err == nil && len(hashes) > 0
	}
	return result, nil
}

// GetInstallState returns the install state for a VideoGameVersion, or nil if not yet installed.
func (a *App) GetInstallState(itemTitle string) (*models.InstallState, error) {
	versionDir := filepath.Join(a.dataPath, "VideoGameVersion", itemTitle)
	return metadata.ReadInstallState(versionDir)
}

// GetPlatformAvailable reports whether the version can be installed on the current platform.
// If a .install.json exists it is the sole source of truth (targetPlatforms).
// Otherwise it falls back to checking the download entries in .mediaitem.json.
func (a *App) GetPlatformAvailable(itemTitle string) (bool, error) {
	specs, err := metadata.LoadInstallationSpecs(a.metadataPath, itemTitle)
	if err != nil {
		return false, err
	}
	return len(specs) > 0 && a.findMatchingSpec(specs) != nil, nil
}

// GetInstallPrompts returns the user-facing arg prompts for a version's .installation.json,
// with ROM readiness pre-populated per option. Returns nil if no spec exists or the spec
// has no args (meaning the install needs no user input).
func (a *App) GetInstallPrompts(itemTitle string) ([]models.ArgPrompt, error) {
	version, err := metadata.LoadOneVersion(a.metadataPath, itemTitle)
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

	romHashes, _ := metadata.ScanROMLibrary(a.dataPath)

	// Build title → present map from romDependencies
	romPresent := make(map[string]bool)
	for _, dep := range version.ROMDependencies {
		for _, f := range dep.Formats {
			if _, ok := romHashes[f.Checksums.MD5]; ok {
				romPresent[dep.Title] = true
				break
			}
		}
	}

	// Sort arg names for deterministic display order
	names := make([]string, 0, len(spec.Args))
	for k := range spec.Args {
		names = append(names, k)
	}
	sort.Strings(names)

	var prompts []models.ArgPrompt
	for _, name := range names {
		argSpec := spec.Args[name]
		prompt := models.ArgPrompt{Name: name, Type: argSpec.Type, Label: argSpec.Label}
		for _, opt := range argSpec.Options {
			ready := opt.ROMTitle == "" || romPresent[opt.ROMTitle]
			prompt.Options = append(prompt.Options, models.ArgOption{
				Value:     opt.Value,
				Label:     opt.Label,
				ROMTitle:  opt.ROMTitle,
				ROMsReady: ready,
			})
		}
		prompts = append(prompts, prompt)
	}
	return prompts, nil
}

// platformMatches returns true if platforms is empty (all platforms) or contains target.
func platformMatches(platforms []string, target string) bool {
	if len(platforms) == 0 {
		return true
	}
	for _, p := range platforms {
		if p == target {
			return true
		}
	}
	return false
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

func (a *App) InstallVersion(itemTitle string, args map[string]string) error {
	installCtx, cancel := context.WithCancel(a.ctx)
	a.installMu.Lock()
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

	wailsruntime.EventsEmit(a.ctx, "install:started", map[string]interface{}{
		"itemTitle": itemTitle,
	})

	version, err := metadata.LoadOneVersion(a.metadataPath, itemTitle)
	if err != nil {
		return err
	}
	if version == nil {
		return fmt.Errorf("version not found: %s", itemTitle)
	}

	versionDir := filepath.Join(a.dataPath, "VideoGameVersion", itemTitle)

	specs, err := metadata.LoadInstallationSpecs(a.metadataPath, itemTitle)
	if err != nil {
		return fmt.Errorf("failed to load installation spec: %w", err)
	}
	spec := a.findMatchingSpec(specs)
	if spec == nil {
		return fmt.Errorf("no install spec available for %s on %s", itemTitle, a.GetPlatform())
	}
	if args == nil {
		args = map[string]string{}
	}
	return a.buildAndInstallSpec(installCtx, version, spec, args, versionDir)
}

// findMatchingSpec returns the first spec in the array whose targetPlatforms includes
// the current platform (or has no platform restriction). Returns nil if none match.
func (a *App) findMatchingSpec(specs []models.InstallationSpec) *models.InstallationSpec {
	platform := a.GetPlatform()
	for i := range specs {
		s := &specs[i]
		if len(s.TargetPlatforms) == 0 {
			return s
		}
		for _, p := range s.TargetPlatforms {
			if p == platform {
				return s
			}
		}
	}
	return nil
}


// buildAndInstallSpec handles the .install.json spec path.
func (a *App) buildAndInstallSpec(ctx context.Context, version *models.VideoGameVersion, spec *models.InstallationSpec, args map[string]string, versionDir string) error {
	// Resolve romTitle for each choice arg and inject it so runBuildSteps can use it.
	enriched := make(map[string]string, len(args))
	for k, v := range args {
		enriched[k] = v
	}
	for argName, argSpec := range spec.Args {
		if argSpec.Type != "choice" {
			continue
		}
		selectedValue := args[argName]
		for _, opt := range argSpec.Options {
			if opt.Value == selectedValue && opt.ROMTitle != "" {
				enriched["__romTitle__"+argName] = opt.ROMTitle
				break
			}
		}
	}
	exes, err := a.runBuildSteps(ctx, spec.Steps, spec.Dependencies, enriched, version, versionDir)
	if err != nil {
		return err
	}
	return a.writeInstallState(versionDir, spec, exes)
}

// runBuildSteps executes a build step sequence and returns the declared executables.
func (a *App) runBuildSteps(ctx context.Context, steps []models.BuildStep, deps []string, args map[string]string, version *models.VideoGameVersion, versionDir string) ([]models.ExecutableEntry, error) {
	if len(deps) > 0 {
		if err := checkDependencies(deps); err != nil {
			return nil, err
		}
	}

	if err := os.MkdirAll(versionDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create version data directory: %w", err)
	}

	romHashes, err := metadata.ScanROMLibrary(a.dataPath)
	if err != nil {
		return nil, fmt.Errorf("failed to scan ROM library: %w", err)
	}

	var msys2Root string
	if runtime.GOOS == "windows" {
		for _, dep := range deps {
			if dep == "msys2" {
				msys2Root = findMSYS2()
				break
			}
		}
	}

	logPath := filepath.Join(versionDir, "install.log")
	logFile, err := os.Create(logPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create install log: %w", err)
	}
	defer logFile.Close()
	logf := func(format string, v ...any) {
		fmt.Fprintf(logFile, format+"\n", v...)
	}

	currentDir := versionDir
	var exes []models.ExecutableEntry

	total := 0
	for _, s := range steps {
		if s.If == "" || evalCondition(s.If, args) {
			total++
		}
	}

	stepNum := 0
	for _, step := range steps {
		if step.If != "" && !evalCondition(step.If, args) {
			logf("[skipped] %s", stepLabel(step))
			continue
		}

		select {
		case <-ctx.Done():
			wailsruntime.EventsEmit(a.ctx, "install:cancelled", nil)
			return nil, fmt.Errorf("install cancelled")
		default:
		}

		label := stepLabel(step)
		logf("[step %d/%d] %s", stepNum+1, total, label)
		a.emitStep(stepNum, total, label)
		stepNum++

		switch step.Step {
		case "cd":
			currentDir = filepath.Clean(filepath.Join(currentDir, interpolate(step.Path, args)))

		case "fetch":
			url := interpolate(step.URL, args)
			dest := filepath.Join(currentDir, interpolate(step.Dest, args))
			if err := fetchFile(url, dest); err != nil {
				a.emitFailed(label, err.Error())
				return nil, fmt.Errorf("build step 'fetch' failed: %w", err)
			}

		case "extract":
			src := filepath.Join(currentDir, interpolate(step.Src, args))
			dest := filepath.Join(currentDir, interpolate(step.Dest, args))
			if err := os.MkdirAll(dest, 0755); err != nil {
				a.emitFailed(label, err.Error())
				return nil, fmt.Errorf("build step 'extract' failed: %w", err)
			}
			var extractErr error
			switch strings.ToLower(filepath.Ext(src)) {
			case ".7z":
				cmd7z := exec.CommandContext(ctx, "7z", "x", "-o"+dest, src)
				cmd7z.Dir = currentDir
				cmd7z.Env = mergeEnv(nil)
				if msys2Root != "" {
					cmd7z.Env = prependMSYS2Path(cmd7z.Env, msys2Root)
				}
				stdout7z, _ := cmd7z.StdoutPipe()
				stderr7z, _ := cmd7z.StderrPipe()
				if err := cmd7z.Start(); err != nil {
					extractErr = err
				} else {
					go streamOutput(stdout7z, logFile)
					go streamOutput(stderr7z, logFile)
					extractErr = cmd7z.Wait()
				}
			default:
				extractErr = extractZip(src, dest)
			}
			if extractErr != nil {
				a.emitFailed(label, extractErr.Error())
				return nil, fmt.Errorf("build step 'extract' failed: %w", extractErr)
			}

		case "copy":
			var srcPath string
			if step.From == "rom" {
				romTitle := interpolate(step.Src, args)
				if romTitle == "" && step.Arg != "" {
					// Legacy format: arg reference — look up pre-resolved romTitle.
					romTitle = args["__romTitle__"+step.Arg]
				}
				var romDeps []models.ROMDependency
				if version != nil {
					romDeps = version.ROMDependencies
				}
				for _, dep := range romDeps {
					if romTitle != "" && dep.Title != romTitle {
						continue
					}
					for _, f := range dep.Formats {
						if p, ok := romHashes[f.Checksums.MD5]; ok {
							srcPath = p
							if romTitle == "" {
								romTitle = dep.Title
							}
							break
						}
					}
					if srcPath != "" {
						break
					}
				}
				if srcPath == "" {
					a.emitFailed(label, fmt.Sprintf("ROM not found: %s", romTitle))
					return nil, fmt.Errorf("build step 'copy': ROM not found: %s", romTitle)
				}
			} else {
				srcPath = filepath.Join(currentDir, interpolate(step.Src, args))
			}
			dest := filepath.Join(currentDir, interpolate(step.Dest, args))
			if err := copyPath(srcPath, dest); err != nil {
				a.emitFailed(label, err.Error())
				return nil, fmt.Errorf("build step 'copy' failed: %w", err)
			}

		case "move":
			src := filepath.Join(currentDir, interpolate(step.Src, args))
			dest := filepath.Join(currentDir, interpolate(step.Dest, args))
			logf("  src:  %s", src)
			logf("  dest: %s", dest)
			if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
				a.emitFailed(label, err.Error())
				return nil, fmt.Errorf("build step 'move' failed: %w", err)
			}
			// os.Rename is atomic but fails across devices; fall back to copy+delete.
			if err := os.Rename(src, dest); err != nil {
				if err2 := copyPath(src, dest); err2 != nil {
					msg := fmt.Sprintf("src=%s dest=%s: %v", src, dest, err2)
					a.emitFailed(label, msg)
					return nil, fmt.Errorf("build step 'move' failed: %s", msg)
				}
				if err2 := os.RemoveAll(src); err2 != nil {
					a.emitFailed(label, err2.Error())
					return nil, fmt.Errorf("build step 'move' (cleanup) failed: %w", err2)
				}
			}

		case "make":
			makeArgs := make([]string, len(step.Args))
			for j, a := range step.Args {
				makeArgs[j] = interpolate(a, args)
			}
			cmd := exec.CommandContext(ctx, "make", makeArgs...)
			cmd.Dir = currentDir
			cmd.Env = mergeEnv(step.Env)
			if msys2Root != "" {
				cmd.Env = prependMSYS2Path(cmd.Env, msys2Root)
			}

			stdout, _ := cmd.StdoutPipe()
			stderr, _ := cmd.StderrPipe()
			if err := cmd.Start(); err != nil {
				a.emitFailed(label, err.Error())
				return nil, fmt.Errorf("build step 'make' failed to start: %w", err)
			}
			go streamOutput(stdout, logFile)
			go streamOutput(stderr, logFile)
			if err := cmd.Wait(); err != nil {
				a.emitFailed(label, err.Error())
				return nil, fmt.Errorf("build step 'make' failed: %w", err)
			}

		case "createDir":
			full := filepath.Join(currentDir, interpolate(step.Path, args))
			if err := os.MkdirAll(full, 0755); err != nil {
				a.emitFailed(label, err.Error())
				return nil, fmt.Errorf("build step 'createDir' failed: %w", err)
			}

		case "touch":
			full := filepath.Join(currentDir, interpolate(step.Path, args))
			if err := os.MkdirAll(filepath.Dir(full), 0755); err != nil {
				a.emitFailed(label, err.Error())
				return nil, fmt.Errorf("build step 'touch' failed: %w", err)
			}
			f, err := os.Create(full)
			if err != nil {
				a.emitFailed(label, err.Error())
				return nil, fmt.Errorf("build step 'touch' failed: %w", err)
			}
			f.Close()

		case "deletePath":
			full := filepath.Join(currentDir, interpolate(step.Path, args))
			if err := os.RemoveAll(full); err != nil {
				a.emitFailed(label, err.Error())
				return nil, fmt.Errorf("build step 'deletePath' failed: %w", err)
			}

		case "defineExecutable":
			exePath := interpolate(step.Executable, args)
			if runtime.GOOS != "windows" {
				_ = os.Chmod(filepath.Join(versionDir, exePath), 0755)
			}
			title := step.Title
			if title == "" {
				title = filepath.Base(exePath)
			}
			exes = append(exes, models.ExecutableEntry{Path: exePath, Title: title})
		}
	}

	if len(exes) == 0 {
		a.emitFailed("defineExecutable", "no defineExecutable steps found")
		return nil, fmt.Errorf("build produced no output: no 'defineExecutable' steps defined")
	}

	return exes, nil
}


// CleanBuildDir removes build artefacts for a version after a failed build.
// It uses the buildPaths array from the matching .install.json spec when available,
// falling back to removing common build directories (.build, build).
func (a *App) CleanBuildDir(itemTitle string) error {
	versionDir := filepath.Join(a.dataPath, "VideoGameVersion", itemTitle)
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

// UninstallVersion runs any uninstallSteps from the .install.json spec, then removes
// the install directory and clears the .state.json. If no uninstallSteps are defined
// it simply removes the install/ subdirectory.
func (a *App) UninstallVersion(itemTitle string) error {
	versionDir := filepath.Join(a.dataPath, "VideoGameVersion", itemTitle)

	var cleanupErr error
	specs, _ := metadata.LoadInstallationSpecs(a.metadataPath, itemTitle)
	if spec := a.findMatchingSpec(specs); spec != nil && len(spec.UninstallSteps) > 0 {
		version, _ := metadata.LoadOneVersion(a.metadataPath, itemTitle)
		_, cleanupErr = a.runBuildSteps(a.ctx, spec.UninstallSteps, nil, map[string]string{}, version, versionDir)
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
	state.ExecutablePath = ""
	state.Executables = nil
	_ = metadata.WriteInstallState(versionDir, state)

	if cleanupErr != nil {
		return fmt.Errorf("uninstall failed: %w", cleanupErr)
	}
	return nil
}

// LaunchVersion launches an installed executable. If executablePath is empty the primary is used.
func (a *App) LaunchVersion(itemTitle string, executablePath string) error {
	versionDir := filepath.Join(a.dataPath, "VideoGameVersion", itemTitle)
	state, err := metadata.ReadInstallState(versionDir)
	if err != nil {
		return err
	}
	if state == nil || !state.Installed {
		return fmt.Errorf("not installed: %s", itemTitle)
	}

	exes := state.Executables
	if len(exes) == 0 && state.ExecutablePath != "" {
		exes = []models.ExecutableEntry{{Path: state.ExecutablePath, Title: "Play"}}
	}
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

	wailsruntime.EventsEmit(a.ctx, "game:started", map[string]interface{}{
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

		wailsruntime.EventsEmit(a.ctx, "game:ended", map[string]interface{}{
			"itemTitle":   itemTitle,
			"playSeconds": playSeconds,
		})
	}()

	return nil
}

// GetROMStatus scans the ROM library for the given version and returns MD5 → found.
func (a *App) GetROMStatus(itemTitle string) (map[string]bool, error) {
	version, err := metadata.LoadOneVersion(a.metadataPath, itemTitle)
	if err != nil {
		return nil, err
	}
	if version == nil {
		return nil, fmt.Errorf("version not found: %s", itemTitle)
	}

	foundHashes, err := metadata.ScanROMLibrary(a.dataPath)
	if err != nil {
		return nil, err
	}

	status := make(map[string]bool)
	for _, rom := range version.ROMDependencies {
		for _, f := range rom.Formats {
			_, found := foundHashes[f.Checksums.MD5]
			status[f.Checksums.MD5] = found
		}
	}

	return status, nil
}

// AddROMFiles copies or moves dropped files into the matching VideoGameRom folder.
// MatchDroppedROMs calculates the MD5 of each dropped file and compares it
// against every VideoGameRom format in the library. Returns matched and unmatched files.
func (a *App) MatchDroppedROMs(paths []string) (*models.ROMDropSummary, error) {
	allRoms, err := metadata.LoadAllRoms(a.metadataPath)
	if err != nil {
		return nil, err
	}

	// Build md5 → (romItemTitle, ext) index from library metadata.
	type romEntry struct {
		romTitle  string
		formatExt string
	}
	index := make(map[string]romEntry)
	for _, rom := range allRoms {
		for _, f := range rom.Formats {
			if f.Checksums.MD5 != "" {
				index[strings.ToLower(f.Checksums.MD5)] = romEntry{rom.ItemTitle, f.Ext}
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
				FilePath:  path,
				FileName:  filepath.Base(path),
				ROMTitle:  entry.romTitle,
				FormatExt: entry.formatExt,
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
		destDir := filepath.Join(a.dataPath, "VideoGameRom", m.ROMTitle)
		if err := os.MkdirAll(destDir, 0755); err != nil {
			return fmt.Errorf("failed to create directory for %s: %w", m.ROMTitle, err)
		}
		dest := filepath.Join(destDir, m.FileName)
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
	return nil
}

func (a *App) AddROMFiles(itemTitle string, paths []string, move bool) ([]string, error) {
	version, err := metadata.LoadOneVersion(a.metadataPath, itemTitle)
	if err != nil {
		return nil, err
	}
	if version == nil {
		return nil, fmt.Errorf("version not found: %s", itemTitle)
	}

	romsByMD5 := make(map[string]string)
	for _, rom := range version.ROMDependencies {
		for _, f := range rom.Formats {
			romsByMD5[f.Checksums.MD5] = rom.Title
		}
	}

	romItemByMD5 := make(map[string]string)
	if allRoms, err := metadata.LoadAllRoms(a.metadataPath); err == nil {
		for _, r := range allRoms {
			for _, f := range r.Formats {
				romItemByMD5[f.Checksums.MD5] = r.ItemTitle
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
		if romItemTitle, ok := romItemByMD5[hash]; ok {
			destDir = filepath.Join(a.dataPath, "VideoGameRom", romItemTitle)
		} else {
			destDir = filepath.Join(a.dataPath, "VideoGameVersion", itemTitle)
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

func (a *App) copyROMs(deps []models.ROMDependency, installDir string) error {
	foundHashes, err := metadata.ScanROMLibrary(a.dataPath)
	if err != nil {
		return fmt.Errorf("failed to scan ROM library: %w", err)
	}
	for _, rom := range deps {
		if rom.InstallPath == "" {
			continue
		}
		var srcPath string
		for _, f := range rom.Formats {
			if p, ok := foundHashes[f.Checksums.MD5]; ok {
				srcPath = p
				break
			}
		}
		if srcPath == "" {
			return fmt.Errorf("required ROM not found: %s", rom.Title)
		}
		destPath := filepath.Join(installDir, rom.InstallPath)
		if err := os.MkdirAll(filepath.Dir(destPath), 0755); err != nil {
			return err
		}
		if err := copyFile(srcPath, destPath); err != nil {
			return fmt.Errorf("failed to copy ROM %s: %w", rom.Title, err)
		}
	}
	return nil
}

func (a *App) writeInstallState(versionDir string, spec *models.InstallationSpec, exes []models.ExecutableEntry) error {
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
		ExecutablePath:   primaryPath,
		Executables:      exes,
		ActiveMods:       []string{},
		InstalledAt:      time.Now().UTC().Format(time.RFC3339),
	}
	if err := metadata.WriteInstallState(versionDir, state); err != nil {
		return fmt.Errorf("failed to save install state: %w", err)
	}
	a.emitProgress("done", 100)
	return nil
}

// evalCondition evaluates a step's "if" expression after interpolating args.
// Supports "lhs != rhs" and "lhs == rhs"; bare values are truthy when non-empty and not "false".
func evalCondition(expr string, args map[string]string) bool {
	expr = strings.TrimSpace(interpolate(expr, args))
	if idx := strings.Index(expr, "!="); idx >= 0 {
		return strings.TrimSpace(expr[:idx]) != strings.TrimSpace(expr[idx+2:])
	}
	if idx := strings.Index(expr, "=="); idx >= 0 {
		return strings.TrimSpace(expr[:idx]) == strings.TrimSpace(expr[idx+2:])
	}
	return expr != "" && expr != "false" && expr != "0"
}

// interpolate replaces $varName and ${varName} with values from args.
func interpolate(s string, args map[string]string) string {
	return argRe.ReplaceAllStringFunc(s, func(match string) string {
		var key string
		if strings.HasPrefix(match, "${") {
			key = match[2 : len(match)-1]
		} else {
			key = match[1:]
		}
		if v, ok := args[key]; ok {
			return v
		}
		return match
	})
}

func checkDependencies(deps []string) error {
	var missing []string
	for _, dep := range deps {
		if dep == "msys2" {
			if runtime.GOOS == "windows" && findMSYS2() == "" {
				missing = append(missing, "msys2 (install from https://www.msys2.org)")
			}
			continue
		}
		if _, err := exec.LookPath(dep); err != nil {
			missing = append(missing, dep)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing build dependencies: %s", strings.Join(missing, ", "))
	}
	return nil
}

// findMSYS2 returns the root of the MSYS2 installation on Windows, or "" if not found.
func findMSYS2() string {
	candidates := []string{
		`C:\msys64`,
		`C:\msys2`,
		filepath.Join(os.Getenv("USERPROFILE"), "msys64"),
		filepath.Join(os.Getenv("USERPROFILE"), "msys2"),
	}
	for _, root := range candidates {
		if info, err := os.Stat(filepath.Join(root, "usr", "bin")); err == nil && info.IsDir() {
			return root
		}
	}
	return ""
}

// prependMSYS2Path returns a copy of env with the MSYS2 bin directories prepended to PATH.
func prependMSYS2Path(env []string, msys2Root string) []string {
	extra := strings.Join([]string{
		filepath.Join(msys2Root, "mingw64", "bin"),
		filepath.Join(msys2Root, "usr", "local", "bin"),
		filepath.Join(msys2Root, "usr", "bin"),
	}, string(os.PathListSeparator))
	for i, e := range env {
		if strings.HasPrefix(strings.ToUpper(e), "PATH=") {
			env[i] = e[:5] + extra + string(os.PathListSeparator) + e[5:]
			return env
		}
	}
	return append(env, "PATH="+extra)
}

func mergeEnv(extra map[string]string) []string {
	env := os.Environ()
	for k, v := range extra {
		env = append(env, k+"="+v)
	}
	return env
}

func stepLabel(step models.BuildStep) string {
	switch step.Step {
	case "cd":
		return "cd " + step.Path
	case "fetch":
		return "fetch " + step.URL
	case "extract":
		return "extract " + step.Src
	case "copy":
		if step.From == "rom" {
			return "copy rom → " + step.Dest
		}
		return "copy " + step.Src + " → " + step.Dest
	case "move":
		return "move " + step.Src + " → " + step.Dest
	case "make":
		return "make " + strings.Join(step.Args, " ")
	case "createDir":
		return "createDir " + step.Path
	case "touch":
		return "touch " + step.Path
	case "deletePath":
		return "deletePath " + step.Path
	case "defineExecutable":
		return "defineExecutable " + step.Executable
	default:
		return step.Step
	}
}

func fetchFile(url, dest string) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		return err
	}
	resp, err := httpClient.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP %d fetching %s", resp.StatusCode, url)
	}
	f, err := os.Create(dest)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = io.Copy(f, resp.Body)
	return err
}

func streamOutput(r io.Reader, w io.Writer) {
	scanner := bufio.NewScanner(r)
	for scanner.Scan() {
		fmt.Fprintln(w, scanner.Text())
	}
}

func (a *App) emitProgress(phase string, percent int) {
	wailsruntime.EventsEmit(a.ctx, "install:progress", map[string]interface{}{
		"phase":   phase,
		"percent": percent,
	})
}

func (a *App) emitStep(index, total int, label string) {
	wailsruntime.EventsEmit(a.ctx, "install:step", map[string]interface{}{
		"index": index,
		"total": total,
		"label": label,
	})
}

func (a *App) emitFailed(step, errMsg string) {
	wailsruntime.EventsEmit(a.ctx, "install:failed", map[string]interface{}{
		"step":  step,
		"error": errMsg,
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

func (a *App) downloadFile(url string, dest *os.File) error {
	resp, err := httpClient.Get(url)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	pr := &progressReader{
		r:     resp.Body,
		total: resp.ContentLength,
		onPct: func(pct int) { a.emitProgress("downloading", pct) },
	}
	_, err = io.Copy(dest, pr)
	return err
}

func extractZip(src, destDir string) error {
	r, err := zip.OpenReader(src)
	if err != nil {
		return err
	}
	defer r.Close()

	destDir = filepath.Clean(destDir)

	for _, f := range r.File {
		destPath := filepath.Join(destDir, f.Name)
		rel, err := filepath.Rel(destDir, destPath)
		if err != nil || strings.HasPrefix(rel, "..") {
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

// copyPath copies src to dest, handling both files and directories.
// For files, parent directories of dest are created automatically.
// For directories, dest becomes a copy of src (recursive).
func copyPath(src, dest string) error {
	info, err := os.Stat(src)
	if err != nil {
		return err
	}
	if info.IsDir() {
		return copyDirRecursive(src, dest)
	}
	if err := os.MkdirAll(filepath.Dir(dest), 0755); err != nil {
		return err
	}
	return copyFile(src, dest)
}


// copyDirRecursive copies src directory and all its contents into dest.
func copyDirRecursive(src, dest string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dest, rel)
		if info.IsDir() {
			return os.MkdirAll(target, info.Mode())
		}
		if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
			return err
		}
		return copyFile(path, target)
	})
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
