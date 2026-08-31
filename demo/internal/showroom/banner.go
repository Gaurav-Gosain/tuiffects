package showroom

import (
	"strings"
	"unicode/utf8"
)

const (
	tagline = "terminal text effects for Go"
	credit  = "TerminalTextEffects by ChrisBuilds"
)

const blockLetters = ` _             _    __    __                 _
| |_   _   _  (_)  / _|  / _|   ___    ___  | |_   ___
| __| | | | | | | | |_  | |_   / _ \  / __| | __| / __|
| |_  | |_| | | | |  _| |  _| |  __/ | (__  | |_  \__ \
 \__|  \__,_| |_| |_|   |_|    \___|  \___|  \__| |___/`

// variants are the banner at three widths, widest first.
var variants = []string{
	blockLetters + "\n\n" + tagline + "\n" + credit,
	"tuiffects\n\n" + tagline + "\n" + credit,
	"tuiffects",
}

// Banner is the text every effect animates: the widest variant whose longest
// line fits in width, so a tiny terminal still animates something.
func Banner(width int) string {
	for _, v := range variants {
		if longestLine(v) <= width {
			return v
		}
	}
	return variants[len(variants)-1]
}

func longestLine(s string) int {
	longest := 0
	for _, line := range strings.Split(s, "\n") {
		if n := utf8.RuneCountInString(line); n > longest {
			longest = n
		}
	}
	return longest
}
