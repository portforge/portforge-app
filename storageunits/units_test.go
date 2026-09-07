package storageunits

import (
	"os"
	"path/filepath"
	"testing"
)

// The store resolves its directory through os.UserConfigDir, which honours
// XDG_CONFIG_HOME on Linux, so a test can redirect it wholesale.
func testManager(t *testing.T) *Manager {
	t.Helper()
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	m, err := NewManager()
	if err != nil {
		t.Fatal(err)
	}
	return m
}

func dirs(t *testing.T, n int) []string {
	t.Helper()
	var out []string
	for i := 0; i < n; i++ {
		d := t.TempDir()
		out = append(out, d)
	}
	return out
}

func ids(units []Unit) []string {
	out := make([]string, len(units))
	for i, u := range units {
		out[i] = u.ID
	}
	return out
}

// New units go to the bottom. Adding a location must never silently redirect
// where the next output lands.
func TestAddAppendsAsLowestPriority(t *testing.T) {
	m := testManager(t)
	d := dirs(t, 3)
	for _, p := range d {
		if _, err := m.Add(p); err != nil {
			t.Fatal(err)
		}
	}
	got, err := m.List()
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("got %d units, want 3", len(got))
	}
	for i, p := range d {
		if got[i].Path != p {
			t.Errorf("position %d is %q, want %q — order is priority", i, got[i].Path, p)
		}
	}
}

// One folder is one unit. Adding it twice is the user picking somewhere they
// already have, and is told so rather than silently accepted.
func TestAddRejectsADuplicate(t *testing.T) {
	m := testManager(t)
	d := t.TempDir()
	if _, err := m.Add(d); err != nil {
		t.Fatal(err)
	}
	if _, err := m.Add(d); err == nil {
		t.Error("expected a rejection adding the same folder twice")
	}
	list, _ := m.List()
	if len(list) != 1 {
		t.Errorf("got %d units after adding the same path twice, want 1", len(list))
	}
}

func TestAddRejectsAnUnreadableFolder(t *testing.T) {
	m := testManager(t)
	if _, err := m.Add(filepath.Join(t.TempDir(), "does-not-exist")); err == nil {
		t.Error("expected an error adding a folder that cannot be read")
	}
}

// Reorder takes the whole id set or nothing: a client working from a stale list
// must not be able to drop or invent a unit.
func TestReorderRequiresTheExactIDSet(t *testing.T) {
	m := testManager(t)
	for _, p := range dirs(t, 3) {
		if _, err := m.Add(p); err != nil {
			t.Fatal(err)
		}
	}
	list, _ := m.List()
	all := ids(list)

	for _, c := range []struct {
		name string
		ids  []string
	}{
		{"too few", all[:2]},
		{"unknown id", []string{all[0], all[1], "deadbeef"}},
		{"a repeat", []string{all[0], all[0], all[1]}},
	} {
		if err := m.Reorder(c.ids); err == nil {
			t.Errorf("%s: expected a rejection", c.name)
		}
	}

	// The rejected calls must have left the order untouched.
	after, _ := m.List()
	for i := range all {
		if after[i].ID != all[i] {
			t.Fatalf("a rejected reorder changed the list at %d", i)
		}
	}

	reversed := []string{all[2], all[1], all[0]}
	if err := m.Reorder(reversed); err != nil {
		t.Fatalf("a complete id set should be accepted: %v", err)
	}
	after, _ = m.List()
	for i, want := range reversed {
		if after[i].ID != want {
			t.Errorf("position %d is %s, want %s", i, after[i].ID, want)
		}
	}
}

// Remove forgets a location; it must not touch what is stored there.
func TestRemoveLeavesContentsAlone(t *testing.T) {
	m := testManager(t)
	d := t.TempDir()
	marker := filepath.Join(d, "installed-game.txt")
	if err := os.WriteFile(marker, []byte("mine"), 0644); err != nil {
		t.Fatal(err)
	}
	u, err := m.Add(d)
	if err != nil {
		t.Fatal(err)
	}
	if err := m.Remove(u.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(marker); err != nil {
		t.Errorf("Remove deleted content: %v", err)
	}
	if err := m.Remove(u.ID); err == nil {
		t.Error("removing an unknown id should error")
	}
}

func TestDestination(t *testing.T) {
	m := testManager(t)
	if _, err := m.Destination(0); err == nil {
		t.Error("an empty list must be the same error as no unit qualifying")
	}
	d := dirs(t, 2)
	for _, p := range d {
		if _, err := m.Add(p); err != nil {
			t.Fatal(err)
		}
	}
	got, err := m.Destination(0)
	if err != nil {
		t.Fatal(err)
	}
	if got.Path != d[0] {
		t.Errorf("need==0 gave %q, want the top unit %q", got.Path, d[0])
	}

	// Inclusive comparison: a unit with exactly `need` free qualifies. Asking for
	// exactly what the top unit reports must therefore still choose it.
	list, _ := m.List()
	if _, err := m.Destination(list[0].FreeBytes); err != nil {
		t.Errorf("freeBytes == need should qualify: %v", err)
	}
	if _, err := m.Destination(^uint64(0)); err == nil {
		t.Error("expected an error when nothing has room")
	}
}
