package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"portforge/metadata"
	"portforge/models"
	"strings"

	"github.com/zamiba/forge/engine"
)

// runSpec executes a build step sequence through the forge engine, wiring in
// PortForge's ROM library as a provider and forwarding progress to the frontend.
// overrides carries the per-call engine options that differ between installing
// and uninstalling; everything else is the same for both.
func (a *App) runSpec(
	ctx context.Context,
	spec *engine.Spec,
	steps []engine.Step,
	args map[string]string,
	version *models.VideoGameVersion,
	versionDir string,
	overrides engine.Options,
) ([]models.ExecutableEntry, error) {
	if err := os.MkdirAll(versionDir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create version data directory: %w", err)
	}

	logFile, err := os.Create(filepath.Join(versionDir, "install.log"))
	if err != nil {
		return nil, fmt.Errorf("failed to create install log: %w", err)
	}
	defer logFile.Close()

	provider, err := a.romProvider(version)
	if err != nil {
		return nil, err
	}

	opts := overrides
	opts.Steps = steps
	opts.Dependencies = spec.Dependencies
	opts.Args = args
	opts.RootDir = versionDir
	opts.Providers = map[string]engine.Provider{"rom": provider}
	opts.Events = a.forwardEngineEvent
	opts.Log = logFile
	opts.HTTPClient = httpClient

	res, err := engine.Run(ctx, opts)
	if err != nil {
		return nil, err
	}

	exes := make([]models.ExecutableEntry, 0, len(res.Executables))
	for _, e := range res.Executables {
		exes = append(exes, models.ExecutableEntry{Path: e.Path, Title: e.Title})
	}
	return exes, nil
}

// forwardEngineEvent translates engine events into the Wails events the
// frontend already listens for.
func (a *App) forwardEngineEvent(e engine.Event) {
	// Every install event carries the item it belongs to so the frontend can
	// ignore anything that isn't the install it is currently showing.
	item := a.GetActiveInstall()
	switch e.Kind {
	case engine.EventStepStart:
		a.emit("install:step", map[string]interface{}{
			"itemTitle": item,
			"index":     e.Index,
			"total":     e.Total,
			"label":     e.Label,
		})
	case engine.EventProgress:
		a.emit("install:progress", map[string]interface{}{
			"itemTitle": item,
			"phase":     "downloading",
			"percent":   e.Percent,
		})
	case engine.EventLog:
		a.emit("install:log", map[string]interface{}{
			"itemTitle": item,
			"line":      e.Line,
			"stream":    e.Stream,
		})
	case engine.EventRunFailed:
		// A cancelled run reports through its own event so the UI can tell the
		// difference between "you stopped it" and "it broke".
		if a.ctx.Err() == nil && isCancellation(e.Error) {
			a.emit("install:cancelled", map[string]interface{}{
				"itemTitle": item,
			})
			return
		}
		a.emit("install:failed", map[string]interface{}{
			"itemTitle": item,
			"step":      e.Label,
			"error":     e.Error,
		})
	}
}

func isCancellation(msg string) bool {
	return msg == context.Canceled.Error()
}

// romProvider resolves `copy from:"rom"` steps against the user's ROM library.
// The step's src is the title of one of the version's romDependencies; an empty
// src means "any dependency whose file is present locally".
func (a *App) romProvider(version *models.VideoGameVersion) (engine.Provider, error) {
	var romHashes map[string]string
	if a.store != nil {
		romHashes, _ = a.store.GetROMLocalPaths()
	} else {
		var err error
		romHashes, err = metadata.ScanROMLibrary(a.dataPath)
		if err != nil {
			return nil, fmt.Errorf("failed to scan ROM library: %w", err)
		}
	}

	var reqs []models.ROMRequirement
	if version != nil {
		reqs = version.ROMDependencies
	}

	// present returns the local path of the first option in req that the user
	// actually has.
	present := func(r models.ROMRequirement) (string, bool) {
		for _, opt := range r.Options {
			for _, f := range opt.Formats {
				if p, ok := romHashes[f.Checksums.MD5]; ok {
					return p, true
				}
			}
		}
		return "", false
	}

	return engine.ProviderFunc(func(_ context.Context, req engine.ProviderRequest) (string, error) {
		// An empty src means "any dump this version accepts", which is what every
		// single-ROM port wants. A non-empty src names a requirement — "Disc 2" —
		// so a multi-disc spec can place each disc without naming a particular
		// region's dump. An option title is still accepted so a spec can pin one
		// exact dump when it has to.
		want := req.Src
		for _, r := range reqs {
			if want != "" && r.Name != want {
				continue
			}
			if p, ok := present(r); ok {
				return p, nil
			}
		}
		if want != "" {
			for _, r := range reqs {
				for _, opt := range r.Options {
					if opt.Title != want {
						continue
					}
					for _, f := range opt.Formats {
						if p, ok := romHashes[f.Checksums.MD5]; ok {
							return p, nil
						}
					}
				}
			}
			return "", fmt.Errorf("ROM not found for %q: no dump this port accepts for it is in your library", want)
		}
		return "", fmt.Errorf("no ROM available: none of this version's %d ROM requirements are met by your library", len(reqs))
	}), nil
}

// joinArgNames renders undeclared variable names the way they appear in the
// spec, so the message can be searched for in the file it is about.
func joinArgNames(names []string) string {
	quoted := make([]string, len(names))
	for i, n := range names {
		quoted[i] = "$" + n
	}
	if len(quoted) == 1 {
		return quoted[0]
	}
	return strings.Join(quoted[:len(quoted)-1], ", ") + " and " + quoted[len(quoted)-1]
}
