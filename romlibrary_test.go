package main

import (
	"os"
	"path/filepath"
	"testing"

	"portforge/metadata"

	"github.com/zamiba/forge/engine"
)

// GetROMLibrary runs against the real catalog, without a store: the presence
// map is then all-false, which is exactly the state a fresh install is in.
func newCatalogApp(t *testing.T) *App {
	t.Helper()
	if _, err := os.Stat(filepath.Join("mediaitems", metadata.PortItemType)); err != nil {
		t.Skip("mediaitems submodule not checked out")
	}
	return &App{metadataPath: "mediaitems"}
}

func TestROMLibraryCoversEveryPortThatNeedsAROM(t *testing.T) {
	app := newCatalogApp(t)

	lib, err := app.GetROMLibrary()
	if err != nil {
		t.Fatal(err)
	}
	if len(lib.Ports) == 0 {
		t.Fatal("no ports returned")
	}

	// Whatever the catalog holds, every port listed must carry requirements and
	// every requirement must offer something that can satisfy it. A requirement
	// with no options can never be met and would sit in the UI as a permanent
	// red mark with no way to act on it.
	for _, p := range lib.Ports {
		if p.ItemTitle == "" {
			t.Error("a port was returned without an item title")
		}
		if len(p.Requirements) == 0 {
			t.Errorf("%s: listed with no requirements", p.ItemTitle)
		}
		for _, req := range p.Requirements {
			if len(req.Options) == 0 {
				t.Errorf("%s: requirement %q has no options", p.ItemTitle, req.Name)
			}
		}
	}
}

// The presence map is what the frontend looks checksums up in, so a format
// whose MD5 is absent from it reads as "unknown" forever — the row never
// resolves to present or missing.
func TestROMLibraryStatusCoversEveryFormat(t *testing.T) {
	app := newCatalogApp(t)

	lib, err := app.GetROMLibrary()
	if err != nil {
		t.Fatal(err)
	}

	formats := 0
	for _, p := range lib.Ports {
		for _, opt := range p.Requirements.AllOptions() {
			// An option with no formats cannot be matched against a file on
			// disk: its checksums live in the standalone ROM MediaItem, so an
			// empty list means hydration failed to find it.
			if len(opt.Formats) == 0 {
				t.Errorf("%s: option %q hydrated no formats", p.ItemTitle, opt.Title)
				continue
			}
			for _, f := range opt.Formats {
				if f.Checksums.MD5 == "" {
					t.Errorf("%s: option %q has a format with no MD5", p.ItemTitle, opt.Title)
					continue
				}
				formats++
				if _, ok := lib.Status[f.Checksums.MD5]; !ok {
					t.Errorf("%s: MD5 %s is missing from the status map", p.ItemTitle, f.Checksums.MD5)
				}
			}
		}
	}
	if formats == 0 {
		t.Fatal("no formats were checked")
	}

	// Without a store nothing can be present, so anything true here would mean
	// presence was being decided somewhere other than the ROM index.
	for md5, present := range lib.Status {
		if present {
			t.Errorf("MD5 %s reported present with no store attached", md5)
		}
	}
}

// A dump two ports both accept is one entry, not two. The library view counts
// distinct dumps to tell the user how much of the catalog they hold, and a
// duplicated key would inflate both the total and the held count.
func TestROMLibraryStatusDeduplicatesSharedDumps(t *testing.T) {
	app := newCatalogApp(t)

	lib, err := app.GetROMLibrary()
	if err != nil {
		t.Fatal(err)
	}

	seen := map[string]int{}
	for _, p := range lib.Ports {
		for _, opt := range p.Requirements.AllOptions() {
			for _, f := range opt.Formats {
				seen[f.Checksums.MD5]++
			}
		}
	}
	shared := 0
	for md5, n := range seen {
		if n > 1 {
			shared++
			if _, ok := lib.Status[md5]; !ok {
				t.Errorf("shared MD5 %s missing from the status map", md5)
			}
		}
	}
	if shared == 0 {
		t.Skip("no dump is currently shared between ports")
	}
	if len(lib.Status) != len(seen) {
		t.Errorf("status map has %d entries for %d distinct checksums", len(lib.Status), len(seen))
	}
}

// A spec that interpolates a name it never declares fails before anything is
// downloaded or compiled, and says which name. The alternative is what Render96
// did: build for six steps, then hand make a literal "$romVersion" and fail
// with extract_assets.py's usage text.
func TestInstallRejectsUndeclaredSpecVariables(t *testing.T) {
	spec := &engine.Spec{
		Args: map[string]engine.ArgSpec{"textureMod": {Type: "string", Label: "Texture mod"}},
		Steps: []engine.Step{
			{Step: "make", Args: []string{"VERSION=$romVersion", "MOD=${textureMod}"}},
		},
	}

	missing := engine.UndeclaredArgs(spec)
	if len(missing) != 1 || missing[0] != "romVersion" {
		t.Fatalf("UndeclaredArgs = %v, want [romVersion]", missing)
	}

	msg := joinArgNames(missing)
	if msg != "$romVersion" {
		t.Errorf("joinArgNames = %q, want %q", msg, "$romVersion")
	}
}

// The message names every offending variable: fixing one and rediscovering the
// next on the following attempt is the slow way to repair a spec.
func TestJoinArgNamesListsAllOfThem(t *testing.T) {
	for _, tc := range []struct {
		names []string
		want  string
	}{
		{[]string{"a"}, "$a"},
		{[]string{"a", "b"}, "$a and $b"},
		{[]string{"a", "b", "c"}, "$a, $b and $c"},
	} {
		if got := joinArgNames(tc.names); got != tc.want {
			t.Errorf("joinArgNames(%v) = %q, want %q", tc.names, got, tc.want)
		}
	}
}

// Every spec in the catalog must pass the check the installer now applies, or
// the port simply cannot be installed.
func TestCatalogSpecsPassTheInstallerCheck(t *testing.T) {
	app := newCatalogApp(t)

	versions, err := app.GetVersions()
	if err != nil {
		t.Fatal(err)
	}
	checked := 0
	for _, v := range versions {
		file, err := metadata.LoadSpecFile(app.metadataPath, v.ItemTitle)
		if err != nil || file == nil {
			continue
		}
		for i := range file.Specs {
			checked++
			if missing := engine.UndeclaredArgs(&file.Specs[i]); len(missing) > 0 {
				t.Errorf("%s build %d uses %s, which it does not declare", v.ItemTitle, i, joinArgNames(missing))
			}
		}
	}
	if checked == 0 {
		t.Fatal("no specs were checked")
	}
	t.Logf("%d builds checked", checked)
}
