package main

import (
	"io/fs"
	"os"
	"path/filepath"
)

// LibraryStorage describes the device holding the user library folder, for the
// storage bar in Settings.
//
// Unmeasurable locations — a network share that reports no size, a path that has
// gone away — come back with Available false rather than as an error, because the
// screen degrades to "size unavailable" instead of failing.
type LibraryStorage struct {
	Path      string `json:"path"`
	Available bool   `json:"available"`

	TotalBytes uint64 `json:"totalBytes"`
	FreeBytes  uint64 `json:"freeBytes"`

	// What the library folder itself occupies. Counted separately from
	// TotalBytes-FreeBytes, which is everything on the device.
	LibraryBytes uint64 `json:"libraryBytes"`
}

// GetLibraryStorage measures the device holding the user library folder.
//
// It walks the library to size it, so it is not free — call it when the Settings
// screen opens, not on a timer.
func (a *App) GetLibraryStorage() LibraryStorage {
	s := LibraryStorage{Path: a.dataPath}
	if a.dataPath == "" {
		return s
	}
	if info, err := os.Stat(a.dataPath); err != nil || !info.IsDir() {
		return s
	}

	total, free, err := diskSpace(a.dataPath)
	if err != nil || total == 0 {
		return s
	}
	s.Available = true
	s.TotalBytes = total
	s.FreeBytes = free
	s.LibraryBytes = dirSize(a.dataPath)
	return s
}

// dirSize totals the apparent size of every regular file under root.
//
// Apparent size, not blocks consumed: sparse files over-count and hard links
// count once per name. Neither matters for a library of ROMs and build outputs,
// and avoiding both would mean giving up filepath.WalkDir. Unreadable entries are
// skipped rather than aborting the walk — a partial figure beats none.
func dirSize(root string) uint64 {
	var total uint64
	_ = filepath.WalkDir(root, func(_ string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if err != nil || !info.Mode().IsRegular() {
			return nil
		}
		total += uint64(info.Size())
		return nil
	})
	return total
}
