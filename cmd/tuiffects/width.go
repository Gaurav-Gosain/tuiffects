package main

import (
	"sort"
	"unicode"
)

// Cell width, in terminal columns.
//
// The engine puts one character in one column, so a glyph that the terminal
// draws two columns wide would shear the rest of its row one column left. The
// grid therefore has to agree with the terminal about how wide every rune is,
// and this is that agreement: two columns for East Asian Wide and Fullwidth
// and for the emoji that are drawn wide, zero for combining marks and format
// characters, one for the rest.
//
// It is a table rather than a dependency because this module has none, and a
// CLI in a library's repo that adds one makes every importer of the library
// carry it. The table is the Unicode 13 wide set, which is what the terminals
// this runs in were built against.

// runeWidth is how many columns a rune occupies.
func runeWidth(r rune) int {
	if r == 0 {
		return 0
	}
	if r < 0x20 || (r >= 0x7f && r < 0xa0) {
		// C0 and C1 controls never reach the grid; the parser drops them.
		return 0
	}
	if r < 0x300 {
		// The whole of Latin-1 and ASCII is one column, and it is almost all
		// of every input, so it never reaches the tables.
		return 1
	}
	if isZeroWidth(r) {
		return 0
	}
	if inRanges(r, wideRanges) {
		return 2
	}
	return 1
}

// isZeroWidth reports whether a rune joins the cell before it rather than
// taking one of its own: combining marks, enclosing marks, variation
// selectors, the zero width joiner and the other format characters.
func isZeroWidth(r rune) bool {
	if r >= 0x200b && r <= 0x200f {
		return true
	}
	return unicode.In(r, unicode.Mn, unicode.Me, unicode.Cf)
}

// runeRange is one inclusive span of the table.
type runeRange struct{ lo, hi rune }

// inRanges binary searches a sorted, non-overlapping range table.
func inRanges(r rune, table []runeRange) bool {
	i := sort.Search(len(table), func(i int) bool { return table[i].hi >= r })
	return i < len(table) && r >= table[i].lo
}

// wideRanges is every rune a terminal draws two columns wide. It must stay
// sorted and non-overlapping; TestWideRangesAreSortedAndDisjoint says so.
var wideRanges = []runeRange{
	{0x1100, 0x115f},
	{0x231a, 0x231b},
	{0x2329, 0x232a},
	{0x23e9, 0x23ec},
	{0x23f0, 0x23f0},
	{0x23f3, 0x23f3},
	{0x25fd, 0x25fe},
	{0x2614, 0x2615},
	{0x2648, 0x2653},
	{0x267f, 0x267f},
	{0x2693, 0x2693},
	{0x26a1, 0x26a1},
	{0x26aa, 0x26ab},
	{0x26bd, 0x26be},
	{0x26c4, 0x26c5},
	{0x26ce, 0x26ce},
	{0x26d4, 0x26d4},
	{0x26ea, 0x26ea},
	{0x26f2, 0x26f3},
	{0x26f5, 0x26f5},
	{0x26fa, 0x26fa},
	{0x26fd, 0x26fd},
	{0x2705, 0x2705},
	{0x270a, 0x270b},
	{0x2728, 0x2728},
	{0x274c, 0x274c},
	{0x274e, 0x274e},
	{0x2753, 0x2755},
	{0x2757, 0x2757},
	{0x2795, 0x2797},
	{0x27b0, 0x27b0},
	{0x27bf, 0x27bf},
	{0x2b1b, 0x2b1c},
	{0x2b50, 0x2b50},
	{0x2b55, 0x2b55},
	{0x2e80, 0x303e},
	{0x3041, 0x33ff},
	{0x3400, 0x4dbf},
	{0x4e00, 0x9fff},
	{0xa000, 0xa4cf},
	{0xa960, 0xa97f},
	{0xac00, 0xd7a3},
	{0xf900, 0xfaff},
	{0xfe10, 0xfe19},
	{0xfe30, 0xfe6f},
	{0xff00, 0xff60},
	{0xffe0, 0xffe6},
	{0x16fe0, 0x16fe3},
	{0x17000, 0x187f7},
	{0x18800, 0x18cd5},
	{0x1b000, 0x1b12f},
	{0x1b150, 0x1b152},
	{0x1b164, 0x1b167},
	{0x1b170, 0x1b2fb},
	{0x1f004, 0x1f004},
	{0x1f0cf, 0x1f0cf},
	{0x1f18e, 0x1f18e},
	{0x1f191, 0x1f19a},
	{0x1f200, 0x1f320},
	{0x1f32d, 0x1f335},
	{0x1f337, 0x1f37c},
	{0x1f37e, 0x1f393},
	{0x1f3a0, 0x1f3ca},
	{0x1f3cf, 0x1f3d3},
	{0x1f3e0, 0x1f3f0},
	{0x1f3f4, 0x1f3f4},
	{0x1f3f8, 0x1f43e},
	{0x1f440, 0x1f440},
	{0x1f442, 0x1f4fc},
	{0x1f4ff, 0x1f53d},
	{0x1f54b, 0x1f54e},
	{0x1f550, 0x1f567},
	{0x1f57a, 0x1f57a},
	{0x1f595, 0x1f596},
	{0x1f5a4, 0x1f5a4},
	{0x1f5fb, 0x1f64f},
	{0x1f680, 0x1f6c5},
	{0x1f6cc, 0x1f6cc},
	{0x1f6d0, 0x1f6d2},
	{0x1f6d5, 0x1f6d7},
	{0x1f6eb, 0x1f6ec},
	{0x1f6f4, 0x1f6fc},
	{0x1f7e0, 0x1f7eb},
	{0x1f90c, 0x1f93a},
	{0x1f93c, 0x1f945},
	{0x1f947, 0x1f978},
	{0x1f97a, 0x1f9cb},
	{0x1f9cd, 0x1f9ff},
	{0x1fa70, 0x1fa74},
	{0x1fa78, 0x1fa7a},
	{0x1fa80, 0x1fa86},
	{0x1fa90, 0x1faa8},
	{0x1fab0, 0x1fab6},
	{0x1fac0, 0x1fac2},
	{0x1fad0, 0x1fad6},
	{0x20000, 0x3fffd},
}
