package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"portforge/metadata"

	"github.com/zamiba/forge/engine"
)

// Every (version, platform) pair a spec declares has to be reachable: the
// version picker offers exactly these combinations, so one that Select cannot
// resolve is a dead entry in the UI. A misspelled platform name is invisible
// until someone on that platform tries to install.
func TestCatalogSpecsAreSelectable(t *testing.T) {
	const catalog = "mediaitems"
	dirs, err := os.ReadDir(filepath.Join(catalog, metadata.PortItemType))
	if err != nil {
		t.Skip("mediaitems submodule not checked out")
	}

	known := map[string]bool{
		"Linux": true, "Windows": true, "Mac": true,
		"Linux-x64": true, "Linux-arm64": true,
		"Windows-x64": true, "Windows-arm64": true,
		"Mac-x64": true, "Mac-arm64": true,
	}

	for _, d := range dirs {
		if !d.IsDir() {
			continue
		}
		t.Run(d.Name(), func(t *testing.T) {
			file, err := metadata.LoadSpecFile(catalog, d.Name())
			if err != nil {
				t.Fatalf("LoadSpecFile: %v", err)
			}
			if file == nil {
				t.Skip("no spec file")
			}
			declared := engine.Versions(file.Specs)
			if len(declared) == 0 {
				t.Errorf("declares no versions, so the picker has nothing to offer")
			}
			for _, v := range declared {
				if v.Version == "" {
					t.Errorf("a build declares no version")
				}
				for _, p := range v.Platforms {
					if !known[p] {
						t.Errorf("version %q targets unknown platform %q", v.Version, p)
					}
					spec := engine.Select(file.Specs, p, v.Version)
					if spec == nil {
						t.Errorf("version %q platform %q resolves to no spec", v.Version, p)
						continue
					}
					var exe bool
					for _, s := range spec.Steps {
						if s.Step == "defineExecutable" {
							exe = true
						}
					}
					if !exe {
						t.Errorf("version %q platform %q builds nothing launchable", v.Version, p)
					}
				}
			}
			if dv := file.DefaultVersion; dv != "" {
				found := false
				for _, v := range declared {
					if v.Version == dv {
						found = true
					}
				}
				if !found {
					t.Errorf("defaultVersion %q is not one of the declared versions", dv)
				}
			}
		})
	}
}

// The host a user is actually on has to reach a build. This is the check that
// would have caught the catalog being Linux-only.
func TestCatalogCoversHostPlatforms(t *testing.T) {
	const catalog = "mediaitems"
	dirs, err := os.ReadDir(filepath.Join(catalog, metadata.PortItemType))
	if err != nil {
		t.Skip("mediaitems submodule not checked out")
	}

	hosts := []string{"Linux-x64", "Windows-x64", "Mac-arm64", "Mac-x64"}
	coverage := map[string][]string{}

	for _, d := range dirs {
		if !d.IsDir() {
			continue
		}
		file, err := metadata.LoadSpecFile(catalog, d.Name())
		if err != nil || file == nil {
			continue
		}
		for _, host := range hosts {
			p := resolvePlatform(file.Specs, host)
			if engine.Select(file.Specs, p, "") != nil {
				coverage[host] = append(coverage[host], d.Name())
			}
		}
	}
	for _, host := range hosts {
		t.Logf("%-12s %d ports installable", host, len(coverage[host]))
	}
	if len(coverage["Linux-x64"]) == 0 {
		t.Error("no port is installable on Linux")
	}
}

// A $name that no arg declares is left in the string verbatim by the engine,
// on the reasoning that a visibly wrong path beats a silently truncated one.
// That is right for the engine and useless for a catalog: nothing fails until
// the step runs, and what surfaces is the tool's own error about a nonsensical
// argument rather than anything pointing at the spec.
//
// Render96 shipped "VERSION=$romVersion" with only textureMod declared. The
// build ran for six steps, then make passed the literal string to
// extract_assets.py, which answered with its usage text.
func TestCatalogSpecsDeclareEveryVariableTheyUse(t *testing.T) {
	const catalog = "mediaitems"
	dirs, err := os.ReadDir(filepath.Join(catalog, metadata.PortItemType))
	if err != nil {
		t.Skip("mediaitems submodule not checked out")
	}

	// Injected by the engine for every run, so a spec may use them without
	// declaring them.
	reserved := map[string]bool{"platform": true, "version": true}
	ref := regexp.MustCompile(`\$\{(\w+)\}|\$(\w+)`)

	// The spec file is walked as plain JSON rather than through the Step struct:
	// a host-registered step type carries its own fields, and interpolation
	// applies to all of them.
	var refsIn func(v any, out map[string]bool)
	refsIn = func(v any, out map[string]bool) {
		switch t := v.(type) {
		case string:
			for _, m := range ref.FindAllStringSubmatch(t, -1) {
				out[m[1]+m[2]] = true
			}
		case []any:
			for _, e := range t {
				refsIn(e, out)
			}
		case map[string]any:
			for _, e := range t {
				refsIn(e, out)
			}
		}
	}

	for _, d := range dirs {
		if !d.IsDir() {
			continue
		}
		t.Run(d.Name(), func(t *testing.T) {
			raw, err := os.ReadFile(filepath.Join(catalog, metadata.PortItemType, d.Name(), ".forge.json"))
			if os.IsNotExist(err) {
				t.Skip("no .forge.json")
			}
			if err != nil {
				t.Fatal(err)
			}

			var doc any
			if err := json.Unmarshal(raw, &doc); err != nil {
				t.Fatalf("parse .forge.json: %v", err)
			}

			// Both file forms: a bare array of builds, or an object whose args
			// are the default for every build that does not override them.
			builds, _ := doc.([]any)
			declared := map[string]bool{}
			if obj, ok := doc.(map[string]any); ok {
				builds, _ = obj["builds"].([]any)
				if args, ok := obj["args"].(map[string]any); ok {
					for name := range args {
						declared[name] = true
					}
				}
			}

			for i, b := range builds {
				build, ok := b.(map[string]any)
				if !ok {
					continue
				}
				// A build's own args replace the header's rather than adding to
				// them, which is how the engine resolves them.
				scope := declared
				if args, ok := build["args"].(map[string]any); ok {
					scope = map[string]bool{}
					for name := range args {
						scope[name] = true
					}
				}

				used := map[string]bool{}
				refsIn(build["steps"], used)
				refsIn(build["uninstallSteps"], used)
				for name := range used {
					if !scope[name] && !reserved[name] {
						t.Errorf("build %d uses $%s, which no args entry declares", i, name)
					}
				}
			}
		})
	}
}
