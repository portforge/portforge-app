package main

import (
	"os"
	"path/filepath"
	"portforge/store"
	"testing"

	"portforge/metadata"
)

// The frontend builds an artwork URL from the item's own _itemType and
// _itemTitle, and the handler resolves it against the catalog root. This walks
// the real catalog and checks every declared artwork file is where that URL says
// it is.
//
// It exists because a hardcoded type name in the URL builder survived two
// renames of the ItemType folders and broke every image in the application at
// once — which presents as an artwork bug rather than as a rename that missed a
// file, and so is looked for in the wrong place.
func TestDeclaredArtworkResolvesAtItsURL(t *testing.T) {
	const catalog = "mediaitems"
	if _, err := os.Stat(catalog); err != nil {
		t.Skip("mediaitems submodule not checked out")
	}

	versions, err := metadata.LoadAllVersions(catalog)
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) == 0 {
		t.Fatal("no versions loaded from the catalog")
	}

	checked := 0
	for _, v := range versions {
		if v.ItemType == "" {
			t.Errorf("%q has no _itemType, so no artwork URL can be built for it", v.ItemTitle)
			continue
		}
		for _, art := range v.Artwork {
			if art.FileName == "" {
				continue
			}
			// Exactly the path lib/artwork.js builds, resolved the way
			// artworkHandler resolves it.
			p := filepath.Join(catalog, v.ItemType, v.ItemTitle, ".artwork", art.FileName)
			if _, err := os.Stat(p); err != nil {
				t.Errorf("%s declares artwork %q, which does not exist at %s",
					v.ItemTitle, art.FileName, p)
			}
			checked++
		}
	}
	if checked == 0 {
		t.Error("no artwork was checked; the catalog declares none, so this proves nothing")
	}
	t.Logf("%d artwork files checked across %d ports", checked, len(versions))
}

// The store is the path the running app actually uses, and it has to return the
// item type or the frontend cannot build a URL at all.
func TestStoreReturnsTheItemType(t *testing.T) {
	const catalog = "mediaitems"
	if _, err := os.Stat(catalog); err != nil {
		t.Skip("mediaitems submodule not checked out")
	}
	s, err := openTestStore(t)
	if err != nil {
		t.Fatal(err)
	}
	defer s.Close()
	if err := s.RebuildCatalog(catalog); err != nil {
		t.Fatal(err)
	}
	versions, err := s.GetVersions()
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) == 0 {
		t.Fatal("the store returned no versions")
	}
	for _, v := range versions {
		if v.ItemType != metadata.PortItemType {
			t.Errorf("%q came back with _itemType %q, want %q",
				v.ItemTitle, v.ItemType, metadata.PortItemType)
		}
	}
}

func openTestStore(t *testing.T) (*store.Store, error) {
	t.Helper()
	return store.Open(t.TempDir() + "/library.db")
}
