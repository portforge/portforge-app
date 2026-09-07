package metadata

import (
	"os"
	"testing"
)

// Artwork declared in the JSON and artwork present only on disk both have to
// surface: catalogue entries list some types explicitly, and new ones are added
// by dropping files into .artwork/ without touching the JSON.
func TestLoadOneVersionMergesArtwork(t *testing.T) {
	const catalog = "../mediaitems"
	if _, err := os.Stat(catalog); err != nil {
		t.Skip("mediaitems submodule not checked out")
	}

	v, err := LoadOneVersion(catalog, "Ship of Harkinian · 2022")
	if err != nil || v == nil {
		t.Fatalf("LoadOneVersion = %v, %v", v, err)
	}

	found := map[string]int{}
	for _, a := range v.Artwork {
		found[a.ArtworkType]++
	}
	for _, want := range []string{"Cover", "Hero", "Screenshot"} {
		if found[want] == 0 {
			t.Errorf("no %q artwork returned; got %v", want, found)
		}
	}

	// The declared Cover must not be duplicated by the directory scan.
	if found["Cover"] != 1 {
		t.Errorf("Cover appeared %d times, want 1 — declared and scanned entries were not deduplicated", found["Cover"])
	}
}

// The versions array carries release notes whose titles are the join key to the
// version field in the spec file.
func TestLoadOneVersionReadsVersions(t *testing.T) {
	const catalog = "../mediaitems"
	if _, err := os.Stat(catalog); err != nil {
		t.Skip("mediaitems submodule not checked out")
	}

	v, err := LoadOneVersion(catalog, "Ship of Harkinian · 2022")
	if err != nil || v == nil {
		t.Fatalf("LoadOneVersion = %v, %v", v, err)
	}
	if len(v.Versions) == 0 {
		t.Fatal("no versions parsed from the versions array")
	}
	for _, sv := range v.Versions {
		if sv.Title == "" {
			t.Errorf("version entry with no title: %+v", sv)
		}
	}
}
