package storageunits

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// FileVersion is the shared file's format version. Bump it only alongside the
// suite README, since every program in the suite reads this file.
const FileVersion = 1

const (
	dirName  = "MediaItem"
	fileName = "storage-units.json"
	lockName = "storage-units.lock"
)

// persistedUnit is a unit as it appears in the shared file: the three fields that
// persist, and nothing else.
//
// It is deliberately a separate type from Unit rather than tag-juggling on one.
// Unit is also the shape sent to the UI, which *does* want the live space figures
// — so a single type cannot serve both without one of the two silently getting
// the wrong fields. Writing freeBytes to a file every suite program reads would
// publish a number stale within seconds.
type persistedUnit struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Path string `json:"path"`
}

// file is the on-disk shape.
type file struct {
	Version      int             `json:"version"`
	StorageUnits []persistedUnit `json:"storageUnits"`
}

// Store reads and writes the shared list.
//
// It is shared with every other suite program, so writes are serialised by an
// advisory lock and land atomically. The lock is taken on a sibling lockfile
// rather than on the JSON itself: the write replaces the JSON's inode, so a lock
// held on the target would, by the time the write completed, be a lock on a file
// that no longer exists.
type Store struct{ dir string }

// NewStore resolves the shared config directory, creating it if needed. Both
// programs resolve it identically through os.UserConfigDir rather than
// hard-coding ~/.config.
func NewStore() (*Store, error) {
	base, err := os.UserConfigDir()
	if err != nil {
		return nil, err
	}
	dir := filepath.Join(base, dirName)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, err
	}
	return &Store{dir: dir}, nil
}

// Path is the shared file's location, for display and for opening it.
func (s *Store) Path() string { return filepath.Join(s.dir, fileName) }

// Load reads the list. A missing file is an empty list, not an error: the file
// is created on first write.
func (s *Store) Load() ([]Unit, error) {
	data, err := os.ReadFile(s.Path())
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return decode(data)
}

func decode(data []byte) ([]Unit, error) {
	var f file
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("%s is not readable: %w", fileName, err)
	}
	if f.Version > FileVersion {
		return nil, fmt.Errorf("%s is version %d, but this program understands %d — update it",
			fileName, f.Version, FileVersion)
	}
	units := make([]Unit, len(f.StorageUnits))
	for i, p := range f.StorageUnits {
		units[i] = Unit{ID: p.ID, Name: p.Name, Path: p.Path}
	}
	return units, nil
}

// Update applies a change under the lock: read the current list, transform it,
// write it back. The whole sequence is locked because every mutation is a
// read-modify-write, and two interleaved reads would each write a list missing
// the other's change — losing an Add silently rather than merely tearing a read.
func (s *Store) Update(fn func([]Unit) ([]Unit, error)) error {
	lf, err := os.OpenFile(filepath.Join(s.dir, lockName), os.O_CREATE|os.O_RDWR, 0644)
	if err != nil {
		return err
	}
	defer lf.Close()
	unlock, err := lockFile(lf)
	if err != nil {
		return err
	}
	defer unlock()

	current, err := s.Load()
	if err != nil {
		return err
	}
	next, err := fn(current)
	if err != nil {
		return err
	}
	return s.write(next)
}

// write serialises to a temporary file in the same directory and renames it over
// the target, so a concurrent reader sees either the old file or the new one and
// never a half-written one.
func (s *Store) write(units []Unit) error {
	// An empty list must serialise as [], never null: it is a deliberate statement
	// that the user removed every unit, and a reader has to tell it from a fresh file.
	persisted := make([]persistedUnit, 0, len(units))
	for _, u := range units {
		persisted = append(persisted, persistedUnit{ID: u.ID, Name: u.Name, Path: u.Path})
	}
	data, err := json.MarshalIndent(file{Version: FileVersion, StorageUnits: persisted}, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')

	tmp, err := os.CreateTemp(s.dir, fileName+".*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	defer func() {
		tmp.Close()
		os.Remove(tmpName) // a no-op once the rename below has succeeded
	}()

	if _, err := tmp.Write(data); err != nil {
		return err
	}
	// Flush to disk before the rename: the rename is atomic with respect to other
	// readers, but not with respect to power loss.
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	return os.Rename(tmpName, s.Path())
}
