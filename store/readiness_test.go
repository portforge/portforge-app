package store

import (
	"os"
	"testing"
)

// Within a single requirement the options are alternatives, not a checklist —
// the regional and revision dumps of one game are interchangeable, and only one
// is ever needed. That is easy to regress into an AND, so this builds an index
// from the real catalog, marks a single dump present, and checks it satisfies
// the whole requirement without leaking to other versions.
//
// The AND lives one level up, across requirements; TestReadinessAndsAcrossRequirements
// covers that side.
func TestGetVersionROMReadiness(t *testing.T) {
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

	ready, err := s.GetVersionROMReadiness()
	if err != nil {
		t.Fatal(err)
	}
	if len(ready) == 0 {
		t.Fatal("no version reported any ROM dependencies")
	}
	// Ports whose requirements are all optional are ready with nothing present,
	// by design — they install and launch without a ROM.
	optional := map[string]bool{}
	rows, err := s.db.Query(`
		SELECT version_item_title FROM version_rom_deps
		GROUP BY version_item_title HAVING MAX(slot_required) = 0
	`)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var title string
		if err := rows.Scan(&title); err != nil {
			t.Fatal(err)
		}
		optional[title] = true
	}
	rows.Close()
	for title, isReady := range ready {
		if isReady && !optional[title] {
			t.Errorf("%q reported ready before any file was marked present", title)
		}
	}

	// Pick the port with the most alternatives inside a single requirement, and
	// whose requirements are exactly that one — the case where an AND/OR mixup
	// inside a requirement shows up most clearly.
	var target string
	var depCount int
	if err := s.db.QueryRow(`
		SELECT version_item_title, COUNT(*) FROM version_rom_deps
		GROUP BY version_item_title
		HAVING COUNT(DISTINCT slot) = 1 AND MAX(slot_required) = 1
		ORDER BY COUNT(*) DESC LIMIT 1
	`).Scan(&target, &depCount); err != nil {
		t.Fatal(err)
	}
	if depCount < 2 {
		t.Skipf("no version in the catalog has multiple ROM alternatives (max %d)", depCount)
	}

	res, err := s.db.Exec(`
		UPDATE rom_formats SET local_path = '/tmp/dump.bin'
		WHERE id = (
			SELECT rf.id FROM rom_formats rf
			JOIN version_rom_deps vd ON vd.rom_item_title = rf.item_title
			WHERE vd.version_item_title = ?
			LIMIT 1
		)`, target)
	if err != nil {
		t.Fatal(err)
	}
	if n, _ := res.RowsAffected(); n != 1 {
		t.Fatalf("expected to mark 1 format present, marked %d", n)
	}

	ready, err = s.GetVersionROMReadiness()
	if err != nil {
		t.Fatal(err)
	}
	if !ready[target] {
		t.Errorf("one present dump did not satisfy %q's %d alternatives", target, depCount)
	}
	for title, isReady := range ready {
		if isReady && title != target && !optional[title] {
			t.Errorf("marking a dump for %q also made %q ready", target, title)
		}
	}
}
