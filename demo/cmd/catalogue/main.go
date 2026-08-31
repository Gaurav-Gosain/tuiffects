// catalogue prints every registered effect as JSON, for the demo page's
// list. It is run at build time so the page never has to load the WASM
// binary just to know what is in it.
package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/Gaurav-Gosain/tuiffects"
)

// Entry is one row of the page's catalogue.
type Entry struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	// Origin is set for an effect original to this package and empty for a
	// port. The page shows a tag for one and a link upstream for the other.
	Origin string `json:"origin,omitempty"`
}

// Entries lists every registered effect, sorted by name.
func Entries() []Entry {
	descriptors := tuiffects.Descriptors()
	entries := make([]Entry, 0, len(descriptors))
	for _, d := range descriptors {
		entries = append(entries, Entry{Name: d.Name, Description: d.Description, Origin: d.Origin})
	}
	return entries
}

func main() {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	if err := enc.Encode(Entries()); err != nil {
		fmt.Fprintln(os.Stderr, "catalogue:", err)
		os.Exit(1)
	}
}
