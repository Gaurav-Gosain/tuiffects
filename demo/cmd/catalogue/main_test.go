package main

import (
	"encoding/json"
	"reflect"
	"testing"

	"github.com/Gaurav-Gosain/tuiffects"
)

func TestEntriesCoverTheRegistryInOrder(t *testing.T) {
	entries := Entries()
	names := tuiffects.Names()
	if len(entries) != len(names) {
		t.Fatalf("got %d entries for %d registered effects", len(entries), len(names))
	}
	for i, entry := range entries {
		if entry.Name != names[i] {
			t.Errorf("entry %d is %q, want %q", i, entry.Name, names[i])
		}
		if entry.Description == "" {
			t.Errorf("entry %q has no description", entry.Name)
		}
	}
}

func TestEntriesRoundTripThroughJSON(t *testing.T) {
	data, err := json.Marshal(Entries())
	if err != nil {
		t.Fatal(err)
	}
	var back []Entry
	if err := json.Unmarshal(data, &back); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(back, Entries()) {
		t.Fatal("entries changed on the way through JSON")
	}
}

// The page shows an "original" tag for an effect with an Origin and a link
// upstream for one without, so both kinds have to come through.
func TestOriginDistinguishesOriginalsFromPorts(t *testing.T) {
	var sawOriginal, sawPort bool
	for _, entry := range Entries() {
		if entry.Origin != "" {
			sawOriginal = true
		} else {
			sawPort = true
		}
	}
	if !sawOriginal {
		t.Error("no entry carries an Origin; tuffbaby should")
	}
	if !sawPort {
		t.Error("every entry carries an Origin; the ports should not")
	}
}
