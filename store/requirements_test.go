package store

import (
	"os"
	"testing"
)

// Requirements AND together while the options inside one OR, so a three-disc
// game is ready only when every disc is present — and any regional dump of a
// given disc will do. The old flat model could not say this: it reported ready
// on the first disc alone.
func TestReadinessAndsAcrossRequirements(t *testing.T) {
	const catalog = "../mediaitems"
	if _, err := os.Stat(catalog); err != nil {
		t.Skip("mediaitems submodule not checked out")
	}
	s, err := Open(t.TempDir() + "/library.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.RebuildCatalog(catalog); err != nil {
		t.Fatal(err)
	}

	const port = "reBlue · 2026"
	discMD5 := func(disc string) string {
		var md5 string
		err := s.db.QueryRow(`
			SELECT rf.md5 FROM version_rom_deps vd
			JOIN rom_formats rf ON rf.item_title = vd.rom_item_title
			WHERE vd.version_item_title = ? AND vd.slot_name = ?
			ORDER BY vd.rom_item_title LIMIT 1
		`, port, disc).Scan(&md5)
		if err != nil {
			t.Fatalf("no format indexed for %s: %v", disc, err)
		}
		return md5
	}
	mark := func(md5 string) {
		if _, err := s.db.Exec(`UPDATE rom_formats SET local_path = ? WHERE md5 = ?`, "/tmp/x.iso", md5); err != nil {
			t.Fatal(err)
		}
	}
	ready := func() bool {
		m, err := s.GetVersionROMReadiness()
		if err != nil {
			t.Fatal(err)
		}
		return m[port]
	}

	if ready() {
		t.Error("ready with no discs at all")
	}
	mark(discMD5("Disc 1"))
	if ready() {
		t.Error("ready with only disc 1 — requirements must AND")
	}
	mark(discMD5("Disc 2"))
	if ready() {
		t.Error("ready with only discs 1 and 2")
	}
	mark(discMD5("Disc 3"))
	if !ready() {
		t.Error("not ready with all three discs present")
	}
}

// A port whose requirements are all optional installs and launches without any
// of them, so it must never be held back. Gen1Recomp extracts its assets on
// first run; the start-menu wall is the port's business, not PortForge's.
func TestReadinessIgnoresOptionalRequirements(t *testing.T) {
	const catalog = "../mediaitems"
	if _, err := os.Stat(catalog); err != nil {
		t.Skip("mediaitems submodule not checked out")
	}
	s, err := Open(t.TempDir() + "/library.db")
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.RebuildCatalog(catalog); err != nil {
		t.Fatal(err)
	}
	m, err := s.GetVersionROMReadiness()
	if err != nil {
		t.Fatal(err)
	}
	if got, ok := m["Gen1Recomp · 2026"]; !ok || !got {
		t.Errorf("all-optional port reported ready=%v present=%v; want ready with nothing imported", got, ok)
	}
	// A single-requirement port with nothing present is still not ready.
	if m["Ship of Harkinian · 2022"] {
		t.Error("a required requirement was satisfied by nothing")
	}
}
