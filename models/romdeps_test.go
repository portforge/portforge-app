package models

import (
	"encoding/json"
	"testing"
)

// romDependencies is a bare array of requirements, each satisfied by any one of
// its options. This pins the wire shape the catalog and the frontend both rely
// on: requirements at the top level, options nested, and no wrapper object.
func TestROMDependenciesShape(t *testing.T) {
	const in = `{"_itemType":"VideoGameFanPort","romDependencies":[
	  {"name":"Disc 1","required":true,"options":[
	    {"_itemType":"Xbox360DiscImage","title":"BD (USA) (Disc 1)"},
	    {"_itemType":"Xbox360DiscImage","title":"BD (Japan) (Disc 1)"}]},
	  {"name":"Shuffle Dungeon","required":false,"options":[
	    {"_itemType":"Xbox360DiscImage","title":"BD DLC"}]}]}`

	var v VideoGameVersion
	if err := json.Unmarshal([]byte(in), &v); err != nil {
		t.Fatal(err)
	}
	if len(v.ROMDependencies) != 2 {
		t.Fatalf("got %d requirements, want 2", len(v.ROMDependencies))
	}
	first := v.ROMDependencies[0]
	if first.Name != "Disc 1" || !first.Required || len(first.Options) != 2 {
		t.Errorf("first requirement decoded wrong: %+v", first)
	}
	if v.ROMDependencies[1].Required {
		t.Error("the DLC requirement should be optional")
	}
	if n := len(v.ROMDependencies.AllOptions()); n != 3 {
		t.Errorf("AllOptions returned %d options across both requirements, want 3", n)
	}

	// What we emit has to be what we accept: the frontend reads this struct back
	// out through Wails, so a shape we cannot re-read is a shape it cannot read.
	out, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var back VideoGameVersion
	if err := json.Unmarshal(out, &back); err != nil {
		t.Fatalf("re-reading our own output: %v", err)
	}
	if len(back.ROMDependencies) != 2 || back.ROMDependencies[0].Name != "Disc 1" {
		t.Errorf("round trip lost a requirement: %s", out)
	}
}

// A version with no ROM requirements at all is normal — plenty of ports need
// nothing — and must not decode into a phantom requirement.
func TestROMDependenciesAbsent(t *testing.T) {
	var v VideoGameVersion
	if err := json.Unmarshal([]byte(`{"_itemType":"VideoGameFanPort"}`), &v); err != nil {
		t.Fatal(err)
	}
	if len(v.ROMDependencies) != 0 {
		t.Errorf("got %d requirements from an item that declares none", len(v.ROMDependencies))
	}
	if v.ROMDependencies.AllOptions() != nil {
		t.Error("AllOptions should be empty when there are no requirements")
	}
}
