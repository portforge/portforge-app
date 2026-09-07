// Package storageunits implements the suite's shared StorageUnit list: the
// ordered set of folders finished output is written to, agreed between the
// programs in the MediaItem suite and stored in one file every one of them
// reads.
//
// The specification is the StorageUnit Configuration section of the suite
// README. This is a port of Digitalizer's reference implementation rather than a
// second design: the semantics have to match exactly, or a folder would receive
// different output depending on which program placed it.
package storageunits

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
)

// Unit is one folder in the user's ordered list.
//
// Three fields persist. The space figures and Unreachable are read from the
// filesystem on every List and never stored: they go stale in seconds, and two
// units on the same volume correctly report identical numbers.
type Unit struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Path string `json:"path"`

	FreeBytes  uint64 `json:"freeBytes"`
	TotalBytes uint64 `json:"totalBytes"`

	// Unreachable marks a path that cannot be read right now — an unmounted NAS,
	// a pulled USB disk. The unit stays in the list: the user configured it
	// deliberately and it may come back.
	Unreachable bool `json:"unreachable"`
}

// Manager owns the shared list. Every operation reads the file fresh under a
// lock, so correctness never depends on another program telling us it changed.
type Manager struct{ store *Store }

func NewManager() (*Manager, error) {
	s, err := NewStore()
	if err != nil {
		return nil, err
	}
	return &Manager{store: s}, nil
}

// List returns the units in priority order with live space figures attached.
func (m *Manager) List() ([]Unit, error) {
	units, err := m.store.Load()
	if err != nil {
		return nil, err
	}
	return withSpace(units), nil
}

// withSpace fills in the transient fields, leaving the persisted ones untouched.
func withSpace(units []Unit) []Unit {
	out := make([]Unit, len(units))
	for i, u := range units {
		total, free, err := DiskSpace(u.Path)
		if err != nil {
			u.Unreachable = true
		} else {
			u.FreeBytes, u.TotalBytes = free, total
		}
		out[i] = u
	}
	return out
}

// Add appends a folder as the lowest-priority unit. New units go to the bottom,
// never the top — adding a location must not silently redirect where the next
// output lands.
//
// A path already in the list is rejected rather than added twice. It is never
// silently accepted: nothing adds folders on a user's behalf, so a duplicate is
// always someone picking a folder they already have, and saying so is more use
// than doing nothing.
func (m *Manager) Add(path string) (Unit, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return Unit{}, err
	}
	if _, _, err := DiskSpace(abs); err != nil {
		return Unit{}, fmt.Errorf("that folder can't be read: %w", err)
	}

	var added Unit
	err = m.store.Update(func(units []Unit) ([]Unit, error) {
		for _, u := range units {
			if u.Path == abs {
				return nil, fmt.Errorf("%s is already a storage location", u.Name)
			}
		}
		added = Unit{ID: newID(), Name: filepath.Base(abs), Path: abs}
		return append(units, added), nil
	})
	if err != nil {
		return Unit{}, err
	}
	return added, nil
}

// Remove drops a unit by id. It forgets a location; it never touches what is
// stored there. Content on a removed unit is the user's, exactly as it would be
// on a drive they unplugged.
func (m *Manager) Remove(id string) error {
	return m.store.Update(func(units []Unit) ([]Unit, error) {
		out := make([]Unit, 0, len(units))
		found := false
		for _, u := range units {
			if u.ID == id {
				found = true
				continue
			}
			out = append(out, u)
		}
		if !found {
			return nil, errors.New("no such save location")
		}
		return out, nil
	})
}

// Rename changes a unit's display label. The path and id are untouched.
func (m *Manager) Rename(id, name string) error {
	return m.store.Update(func(units []Unit) ([]Unit, error) {
		for i := range units {
			if units[i].ID == id {
				units[i].Name = name
				return units, nil
			}
		}
		return nil, errors.New("no such save location")
	})
}

// Reorder applies a new priority order given as the full list of ids.
//
// It rejects any call whose id set differs from the current one — wrong length,
// unknown id, or a repeat — so a client working from a stale list cannot drop or
// invent a unit through a reorder. The order is replaced whole or not at all.
func (m *Manager) Reorder(ids []string) error {
	return m.store.Update(func(units []Unit) ([]Unit, error) {
		if len(ids) != len(units) {
			return nil, errors.New("reorder must list every save location exactly once")
		}
		byID := make(map[string]Unit, len(units))
		for _, u := range units {
			byID[u.ID] = u
		}
		out := make([]Unit, 0, len(ids))
		for _, id := range ids {
			u, ok := byID[id]
			if !ok {
				return nil, fmt.Errorf("unknown save location %q", id)
			}
			delete(byID, id) // a repeated id fails here on its second appearance
			out = append(out, u)
		}
		return out, nil
	})
}

// Destination picks where output of the given size should go: the
// highest-priority reachable unit with room for it.
//
// need == 0 means "the top reachable unit". The comparison is inclusive, so a
// unit with exactly need bytes free qualifies. An empty list returns the same
// error as a full one, leaving callers a single failure path.
func (m *Manager) Destination(need uint64) (Unit, error) {
	units, err := m.List()
	if err != nil {
		return Unit{}, err
	}
	for _, u := range units {
		if u.Unreachable {
			continue
		}
		if need == 0 || u.FreeBytes >= need {
			return u, nil
		}
	}
	return Unit{}, errors.New("no save location has enough free space")
}

func newID() string {
	b := make([]byte, 6)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}
