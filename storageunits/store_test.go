package storageunits

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// The file is read and written by every program in the suite, so its shape is a
// contract, not an implementation detail. This pins it against the suite
// README's StorageUnit Configuration section.
func TestFileShapeMatchesTheAgreedSpec(t *testing.T) {
	m := testManager(t)
	d := t.TempDir()
	if _, err := m.Add(d); err != nil {
		t.Fatal(err)
	}

	raw, err := os.ReadFile(m.store.Path())
	if err != nil {
		t.Fatal(err)
	}
	if base := filepath.Base(m.store.Path()); base != "storage-units.json" {
		t.Errorf("file is named %q, want storage-units.json", base)
	}
	if dir := filepath.Base(filepath.Dir(m.store.Path())); dir != "MediaItem" {
		t.Errorf("file sits in %q, want the shared MediaItem directory", dir)
	}

	var top map[string]json.RawMessage
	if err := json.Unmarshal(raw, &top); err != nil {
		t.Fatal(err)
	}
	if _, ok := top["storageUnits"]; !ok {
		t.Error(`top level has no "storageUnits" array`)
	}
	if _, ok := top["version"]; !ok {
		t.Error(`top level has no "version"`)
	}

	var entries []map[string]json.RawMessage
	if err := json.Unmarshal(top["storageUnits"], &entries); err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("got %d entries, want 1", len(entries))
	}
	// Only the three persisted fields. Space figures go stale in seconds and two
	// units on one volume report the same numbers, so writing them is wrong.
	want := map[string]bool{"id": true, "name": true, "path": true}
	for k := range entries[0] {
		if !want[k] {
			t.Errorf("persisted an unexpected field %q", k)
		}
		delete(want, k)
	}
	for k := range want {
		t.Errorf("missing persisted field %q", k)
	}
}

// An empty list is a deliberate statement — "I removed them all" — and must
// serialise as [] rather than null, or a reader cannot tell it from a fresh file.
func TestEmptyListSerialisesAsAnArray(t *testing.T) {
	m := testManager(t)
	d := t.TempDir()
	u, err := m.Add(d)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Remove(u.ID); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(m.store.Path())
	if err != nil {
		t.Fatal(err)
	}
	var f struct {
		StorageUnits []Unit `json:"storageUnits"`
	}
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatal(err)
	}
	if f.StorageUnits == nil {
		t.Errorf("an emptied list wrote null, not []: %s", raw)
	}
}

// A file written by a newer suite program must be refused rather than silently
// truncated back to what this version understands.
func TestRefusesAFutureFileVersion(t *testing.T) {
	m := testManager(t)
	future := []byte(`{"version":99,"storageUnits":[{"id":"a","name":"n","path":"/tmp"}]}`)
	if err := os.WriteFile(m.store.Path(), future, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := m.List(); err == nil {
		t.Error("expected a refusal for a file version this program does not understand")
	}
}

// A missing file is an empty list, not an error: it is created on first write.
func TestMissingFileReadsAsEmpty(t *testing.T) {
	m := testManager(t)
	units, err := m.List()
	if err != nil {
		t.Fatalf("a missing file should not be an error: %v", err)
	}
	if len(units) != 0 {
		t.Errorf("got %d units from a missing file", len(units))
	}
}
