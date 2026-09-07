package main

import (
	"os"
	"path/filepath"
	"testing"

	"portforge/metadata"
)

func writeSized(t *testing.T, path string, n int) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, make([]byte, n), 0644); err != nil {
		t.Fatal(err)
	}
}

func TestDirSize(t *testing.T) {
	root := t.TempDir()
	writeSized(t, filepath.Join(root, "a.bin"), 100)
	writeSized(t, filepath.Join(root, "nested", "b.bin"), 250)
	writeSized(t, filepath.Join(root, "nested", "deep", "c.bin"), 7)
	if err := os.MkdirAll(filepath.Join(root, "empty"), 0755); err != nil {
		t.Fatal(err)
	}

	if got := dirSize(root); got != 357 {
		t.Errorf("dirSize = %d, want 357", got)
	}
}

func TestDirSizeMissingRoot(t *testing.T) {
	// A library folder that has gone away sizes to zero rather than panicking.
	if got := dirSize(filepath.Join(t.TempDir(), "nope")); got != 0 {
		t.Errorf("dirSize of missing root = %d, want 0", got)
	}
}

func TestGetLibraryStorageUnconfigured(t *testing.T) {
	for name, path := range map[string]string{
		"no path":      "",
		"missing path": filepath.Join(t.TempDir(), "nope"),
	} {
		t.Run(name, func(t *testing.T) {
			s := (&App{dataPath: path}).GetLibraryStorage()
			if s.Available {
				t.Error("Available = true, want false")
			}
		})
	}
}

func TestGetLibraryStorageMeasuresDevice(t *testing.T) {
	root := t.TempDir()
	writeSized(t, filepath.Join(root, "roms", "game.z64"), 4096)

	s := (&App{dataPath: root}).GetLibraryStorage()
	if !s.Available {
		t.Fatal("Available = false, want true for a real directory")
	}
	if s.Path != root {
		t.Errorf("Path = %q, want %q", s.Path, root)
	}
	if s.TotalBytes == 0 {
		t.Error("TotalBytes = 0")
	}
	if s.FreeBytes > s.TotalBytes {
		t.Errorf("FreeBytes %d exceeds TotalBytes %d", s.FreeBytes, s.TotalBytes)
	}
	if s.LibraryBytes != 4096 {
		t.Errorf("LibraryBytes = %d, want 4096", s.LibraryBytes)
	}
}

func TestGetCatalogInfoCountsPorts(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"Ship of Harkinian · 2024", "Render96 · 2021"} {
		if err := os.MkdirAll(filepath.Join(root, metadata.PortItemType, name), 0755); err != nil {
			t.Fatal(err)
		}
	}
	// A stray file alongside the item directories must not be counted.
	writeSized(t, filepath.Join(root, metadata.PortItemType, "README.md"), 10)
	writeSized(t, filepath.Join(root, mediaItemsSHAFile), 40)

	info := (&App{metadataPath: root}).GetCatalogInfo()
	if info.PortCount != 2 {
		t.Errorf("PortCount = %d, want 2", info.PortCount)
	}
	if info.SyncedAt == "" {
		t.Error("SyncedAt is empty; want the SHA file's mtime")
	}
}

func TestGetCatalogInfoNeverSynced(t *testing.T) {
	info := (&App{metadataPath: t.TempDir()}).GetCatalogInfo()
	if info.SHA != "" {
		t.Errorf("SHA = %q, want empty", info.SHA)
	}
	if info.SyncedAt != "" {
		t.Errorf("SyncedAt = %q, want empty", info.SyncedAt)
	}
	if info.PortCount != 0 {
		t.Errorf("PortCount = %d, want 0", info.PortCount)
	}
}
