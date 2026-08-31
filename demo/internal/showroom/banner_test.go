package showroom

import (
	"strings"
	"testing"
	"unicode/utf8"
)

func TestBannerFitsTheWidthItIsGiven(t *testing.T) {
	for _, width := range []int{9, 20, 40, 55, 100} {
		for i, line := range strings.Split(Banner(width), "\n") {
			if n := utf8.RuneCountInString(line); n > width {
				t.Errorf("Banner(%d) line %d is %d wide: %q", width, i, n, line)
			}
		}
	}
}

func TestBannerPicksTheWidestVariantThatFits(t *testing.T) {
	const blockLetterTell = `\__,_|` // only the block letters contain this
	if !strings.Contains(Banner(100), blockLetterTell) {
		t.Error("Banner(100) should be the block letters")
	}
	if !strings.Contains(Banner(55), blockLetterTell) {
		t.Error("Banner(55) should still be the block letters; they are 55 wide")
	}
	narrow := Banner(40)
	if strings.Contains(narrow, blockLetterTell) {
		t.Error("Banner(40) should not be the block letters")
	}
	if !strings.HasPrefix(narrow, "tuiffects\n") || !strings.Contains(narrow, tagline) {
		t.Errorf("Banner(40) should be the word over the tagline, got %q", narrow)
	}
	if got := Banner(9); got != "tuiffects" {
		t.Errorf("Banner(9) = %q, want just the word", got)
	}
}
