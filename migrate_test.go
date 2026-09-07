package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"portforge/storageunits"
	"testing"
)

// A user's imported files live in folders named after their ItemType, so the
// renames strand anything already imported unless the folders come along.
func TestMigrateLegacyTypeDirs(t *testing.T) {
	root := t.TempDir()
	mk := func(parts ...string) string {
		p := filepath.Join(append([]string{root}, parts...)...)
		if err := os.MkdirAll(p, 0755); err != nil {
			t.Fatal(err)
		}
		return p
	}
	write := func(dir, name, content string) {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0644); err != nil {
			t.Fatal(err)
		}
	}
	exists := func(parts ...string) bool {
		_, err := os.Stat(filepath.Join(append([]string{root}, parts...)...))
		return err == nil
	}

	// A plain rename: only the superseded name is present.
	write(mk("N64Rom", "Banjo-Kazooie (USA) · N64"), "banjo.z64", "rom")
	// The ports folder, renamed with everything installed under it.
	write(mk("VideoGameVersion", "Ship of Harkinian · 2022", "install"), "soh.appimage", "build")
	// Two superseded names converging on one current name.
	write(mk("PS1Rom", "From Bare"), "a.bin", "rom")
	write(mk("PS1GameRom", "From GameRom"), "b.bin", "rom")
	// Both names already present: merge, and never overwrite what is there.
	write(mk("Xbox360GameRom", "Only In Old"), "old.iso", "old")
	write(mk("Xbox360GameRom", "In Both"), "disc.iso", "the old copy")
	write(mk("Xbox360DiscImage", "In Both"), "disc.iso", "the new copy")

	migrateLegacyTypeDirs(root)

	if !exists("N64CartRom", "Banjo-Kazooie (USA) · N64", "banjo.z64") {
		t.Error("a plain rename did not carry the ROM across")
	}
	if exists("N64Rom") {
		t.Error("the superseded directory should be gone after a plain rename")
	}
	if !exists("VideoGameFanPort", "Ship of Harkinian · 2022", "install", "soh.appimage") {
		t.Error("an installed port did not survive the ports-folder rename")
	}
	if !exists("PS1DiscImage", "From Bare", "a.bin") || !exists("PS1DiscImage", "From GameRom", "b.bin") {
		t.Error("converging names did not both land in PS1DiscImage")
	}
	if !exists("Xbox360DiscImage", "Only In Old", "old.iso") {
		t.Error("merge did not move the item present only under the old name")
	}
	// The pre-existing copy wins and the old one is left in place, never deleted.
	data, err := os.ReadFile(filepath.Join(root, "Xbox360DiscImage", "In Both", "disc.iso"))
	if err != nil || string(data) != "the new copy" {
		t.Errorf("a colliding item was overwritten: %q %v", data, err)
	}
	if !exists("Xbox360GameRom", "In Both", "disc.iso") {
		t.Error("the colliding item should be left behind rather than deleted")
	}

	// Running again changes nothing.
	migrateLegacyTypeDirs(root)
	if !exists("N64CartRom", "Banjo-Kazooie (USA) · N64", "banjo.z64") {
		t.Error("the second run was not idempotent")
	}
	if !exists("PS1DiscImage", "From Bare", "a.bin") {
		t.Error("the second run disturbed a converged migration")
	}
}

func writeLegacySettings(t *testing.T, path, dataPath string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(legacySettingsFile{DataPath: dataPath})
	if err := os.WriteFile(path, b, 0644); err != nil {
		t.Fatal(err)
	}
}

// A folder the user chose in an earlier version has to survive the move to the
// shared list, or an upgrade shows the first-run screen to someone who already
// configured the program.
func TestMigrateLibraryPath(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	m, err := storageUnitsForTest()
	if err != nil {
		t.Fatal(err)
	}
	library := t.TempDir()
	settings := filepath.Join(t.TempDir(), "settings.json")
	writeLegacySettings(t, settings, library)

	migrateLibraryPath(m, settings)

	units, err := m.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(units) != 1 || units[0].Path != library {
		t.Fatalf("the library folder did not become a storage unit: %+v", units)
	}
	if _, err := os.Stat(settings); err == nil {
		t.Error("the old settings file should have been retired after migrating")
	}
	if _, err := os.Stat(settings + ".migrated"); err != nil {
		t.Error("the retired settings file should be kept, renamed, not deleted")
	}

	// The user removes the unit. A later launch must not put it back: there is
	// nothing left to migrate, which is what makes "empty is deliberate" hold.
	if err := m.Remove(units[0].ID); err != nil {
		t.Fatal(err)
	}
	migrateLibraryPath(m, settings)
	if units, _ = m.List(); len(units) != 0 {
		t.Errorf("migration re-added a folder the user had removed: %+v", units)
	}
}

// A library folder that is not mounted right now must be retried rather than
// silently dropped — the settings file is the only record that it existed.
func TestMigrateLibraryPathRetriesWhenTheFolderIsAbsent(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	m, err := storageUnitsForTest()
	if err != nil {
		t.Fatal(err)
	}
	gone := filepath.Join(t.TempDir(), "unplugged-drive")
	settings := filepath.Join(t.TempDir(), "settings.json")
	writeLegacySettings(t, settings, gone)

	migrateLibraryPath(m, settings)

	if units, _ := m.List(); len(units) != 0 {
		t.Errorf("an unreadable folder should not have been added: %+v", units)
	}
	if _, err := os.Stat(settings); err != nil {
		t.Error("the settings file must survive so the migration can be retried")
	}

	// The drive comes back.
	if err := os.MkdirAll(gone, 0755); err != nil {
		t.Fatal(err)
	}
	migrateLibraryPath(m, settings)
	units, _ := m.List()
	if len(units) != 1 || units[0].Path != gone {
		t.Errorf("the retry did not migrate the folder once it was readable: %+v", units)
	}
}

// storageUnitsForTest builds a manager against the redirected config directory.
func storageUnitsForTest() (*storageunits.Manager, error) { return storageunits.NewManager() }

// The two migrations are ordered, and the order is load-bearing: the type folders
// live under the storage root, so the library path has to become a unit before
// there is a root to migrate them in. This mirrors what startup does.
func TestAnOldInstallMigratesEndToEnd(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	m, err := storageUnitsForTest()
	if err != nil {
		t.Fatal(err)
	}

	// An install from before either rename: a chosen library folder recorded in
	// the private settings file, with content filed under the old type names —
	// including the update-detection snapshots under library/, which mirror the
	// catalog's layout and carry the same folder names a second time.
	library := t.TempDir()
	for _, p := range []string{
		filepath.Join("VideoGameVersion", "Ship of Harkinian · 2022", "install"),
		filepath.Join("N64Rom", "Legend of Zelda, The - Ocarina of Time (USA) · N64"),
		filepath.Join("library", "VideoGameVersion", "Ship of Harkinian · 2022"),
	} {
		if err := os.MkdirAll(filepath.Join(library, p), 0755); err != nil {
			t.Fatal(err)
		}
	}
	rom := filepath.Join(library, "N64Rom", "Legend of Zelda, The - Ocarina of Time (USA) · N64", "oot.z64")
	if err := os.WriteFile(rom, []byte("rom"), 0644); err != nil {
		t.Fatal(err)
	}
	settings := filepath.Join(t.TempDir(), "settings.json")
	writeLegacySettings(t, settings, library)

	// The real sequence, not a copy of it — App.migrate is what startup calls.
	a := &App{units: m}
	a.migrate(settings)

	if a.dataPath != library {
		t.Fatalf("the storage root is %q, want the migrated library %q", a.dataPath, library)
	}
	for _, want := range []string{
		filepath.Join("VideoGameFanPort", "Ship of Harkinian · 2022", "install"),
		filepath.Join("N64CartRom", "Legend of Zelda, The - Ocarina of Time (USA) · N64", "oot.z64"),
		// Left behind, ScanUserLibraryUpdates never finds this port again and it
		// silently stops reporting catalog updates.
		filepath.Join("library", "VideoGameFanPort", "Ship of Harkinian · 2022"),
	} {
		if _, err := os.Stat(filepath.Join(library, want)); err != nil {
			t.Errorf("%s did not survive the migration: %v", want, err)
		}
	}
	for _, gone := range []string{"VideoGameVersion", "N64Rom", filepath.Join("library", "VideoGameVersion")} {
		if _, err := os.Stat(filepath.Join(library, gone)); err == nil {
			t.Errorf("%s should have been renamed away", gone)
		}
	}
}
