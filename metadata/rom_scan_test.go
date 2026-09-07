package metadata

import (
	"os"
	"path/filepath"
	"testing"
)

// A user's ROMs are filed under a folder per ROM item type, so a scan that
// looked only at VideoGameRom found nothing for anyone whose ROMs had been
// imported under a platform-specific type — which is all of them, since
// MatchDroppedROMs files imports by the matched ROM's own type.
func TestScanROMLibraryCoversEveryROMItemType(t *testing.T) {
	base := t.TempDir()

	// One ROM per item type, each with distinct contents so the MD5s differ.
	want := map[string]string{}
	for i, itemType := range RomItemTypes {
		dir := filepath.Join(base, itemType, "Some Game · 1996")
		if err := os.MkdirAll(dir, 0755); err != nil {
			t.Fatal(err)
		}
		file := filepath.Join(dir, "game.z64")
		body := []byte{byte(i), 'r', 'o', 'm'}
		if err := os.WriteFile(file, body, 0644); err != nil {
			t.Fatal(err)
		}
		want[itemType] = file
	}

	got, err := ScanROMLibrary(base)
	if err != nil {
		t.Fatalf("ScanROMLibrary: %v", err)
	}

	found := map[string]bool{}
	for _, path := range got {
		for itemType, wantPath := range want {
			if path == wantPath {
				found[itemType] = true
			}
		}
	}
	for _, itemType := range RomItemTypes {
		if !found[itemType] {
			t.Errorf("no ROM found under %s/; scan returned %d entries", itemType, len(got))
		}
	}
}

// A library with none of the ROM directories present is the normal state before
// a user imports anything, and must read as empty rather than as an error.
func TestScanROMLibraryEmptyWhenNoROMDirs(t *testing.T) {
	got, err := ScanROMLibrary(t.TempDir())
	if err != nil {
		t.Fatalf("ScanROMLibrary on an empty library: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("expected no ROMs, got %d", len(got))
	}
}
