package showroom

import (
	"testing"

	"github.com/Gaurav-Gosain/tuiffects"
)

// A browser hands the showroom whatever size the page's terminal frame
// happens to be, which the library's own tests never exercised. Every
// effect must build cleanly at a typical size and must not panic at a tiny
// one; a tiny canvas is allowed to refuse with an error, which the status
// line shows.
func TestEveryEffectRunsAtBrowserSizes(t *testing.T) {
	for _, name := range tuiffects.Names() {
		t.Run(name, func(t *testing.T) {
			typical, _ := sized(t, New(Options{Effect: name, Seed: 1}), 100, 30)
			if typical.err != nil {
				t.Fatalf("build at 100x30: %v", typical.err)
			}
			typical = ticked(t, typical, 5)
			if typical.frame == "" {
				t.Fatal("no frame after five ticks")
			}

			tiny, _ := sized(t, New(Options{Effect: name, Seed: 1}), 20, 5)
			ticked(t, tiny, 5)
		})
	}
}
