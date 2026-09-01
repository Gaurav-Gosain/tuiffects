package main

import (
	"bufio"
	"errors"
	"io"
	"strings"
	"testing"
)

// rowText is the text of one grid row, with trailing blanks removed.
func rowText(g grid, n int) string {
	var b strings.Builder
	for _, c := range g.rows[n] {
		if c.Symbol == "" {
			b.WriteString(" ")
			continue
		}
		b.WriteString(c.Symbol)
	}
	return strings.TrimRight(b.String(), " ")
}

func buildOK(t *testing.T, in string, width, height int) (grid, io.Reader) {
	t.Helper()
	g, tail, err := buildGrid(strings.NewReader(in), width, height)
	if err != nil {
		t.Fatalf("buildGrid: %v", err)
	}
	return g, tail
}

func TestEveryRowIsExactlyTheCanvasWidth(t *testing.T) {
	// The painter writes each row straight to the terminal, so a row that is
	// not the canvas width wraps and tears the block apart.
	g, _ := buildOK(t, "a\nlonger line here\n\nx\n", 12, 20)
	for i, row := range g.rows {
		if len(row) != 12 {
			t.Errorf("row %d is %d cells wide, want 12", i, len(row))
		}
	}
}

func TestLongLinesWrapRatherThanCrop(t *testing.T) {
	g, _ := buildOK(t, "abcdefghij\n", 4, 20)
	if len(g.rows) != 3 {
		t.Fatalf("got %d rows, want 3", len(g.rows))
	}
	for i, want := range []string{"abcd", "efgh", "ij"} {
		if got := rowText(g, i); got != want {
			t.Errorf("row %d is %q, want %q", i, got, want)
		}
	}
}

func TestTheRowsThatDoNotFitAreLeftForTheTail(t *testing.T) {
	in := "one\ntwo\nthree\nfour\nfive\n"
	g, tail := buildOK(t, in, 20, 2)
	if len(g.rows) != 2 {
		t.Fatalf("the canvas took %d rows, want 2", len(g.rows))
	}
	rest, err := io.ReadAll(tail)
	if err != nil {
		t.Fatal(err)
	}
	if string(rest) != "three\nfour\nfive\n" {
		t.Errorf("the tail is %q, want the last three lines", rest)
	}
}

func TestALineTooTallForWhatIsLeftGoesToTheTailWhole(t *testing.T) {
	// The second line wraps to three rows and only one row is free. Animating
	// part of it and printing the rest would draw it twice.
	in := "top\nabcdefghij\n"
	g, tail := buildOK(t, in, 4, 3)
	if len(g.rows) != 1 || rowText(g, 0) != "top" {
		t.Fatalf("the canvas is %d rows starting %q, want one row of top", len(g.rows), rowText(g, 0))
	}
	rest, _ := io.ReadAll(tail)
	if string(rest) != "abcdefghij\n" {
		t.Errorf("the tail is %q, want the whole wrapped line", rest)
	}
}

func TestEmptyInputMakesNoCanvas(t *testing.T) {
	for _, in := range []string{"", "\n", "\n\n\n", "   \n  \n"} {
		g, _ := buildOK(t, in, 20, 10)
		if len(g.rows) != 0 {
			t.Errorf("input %q made %d rows, want none", in, len(g.rows))
		}
	}
}

func TestBlankLinesInsideThePictureAreKept(t *testing.T) {
	g, _ := buildOK(t, "a\n\n\nb\n", 8, 10)
	if len(g.rows) != 4 {
		t.Fatalf("got %d rows, want 4", len(g.rows))
	}
	if rowText(g, 3) != "b" {
		t.Errorf("the last row is %q, want b", rowText(g, 3))
	}
}

func TestCarriageReturnDrawsOverTheLine(t *testing.T) {
	// This is a progress bar: it goes back to the start and writes again.
	g, _ := buildOK(t, "10%\r100%\n", 20, 4)
	if got := rowText(g, 0); got != "100%" {
		t.Errorf("the row is %q, want 100%%", got)
	}
}

func TestBackspaceMovesBackOneColumn(t *testing.T) {
	g, _ := buildOK(t, "abX\bc\n", 20, 4)
	if got := rowText(g, 0); got != "abc" {
		t.Errorf("the row is %q, want abc", got)
	}
}

func TestTabsMoveToTheNextStop(t *testing.T) {
	g, _ := buildOK(t, "a\tb\n", 20, 4)
	if got := rowText(g, 0); got != "a       b" {
		t.Errorf("the row is %q, want a then seven spaces then b", got)
	}
}

func TestBytesThatAreNotUTF8AreDropped(t *testing.T) {
	g, _ := buildOK(t, "a\xffb\x80c\n", 20, 4)
	if got := rowText(g, 0); got != "abc" {
		t.Errorf("the row is %q, want abc", got)
	}
}

func TestControlCharactersDrawNothing(t *testing.T) {
	g, _ := buildOK(t, "a\x07b\x00c\x7fd\n", 20, 4)
	if got := rowText(g, 0); got != "abcd" {
		t.Errorf("the row is %q, want abcd", got)
	}
}

func TestAWideGlyphKeepsItsTwoColumnsAndLosesItsSymbol(t *testing.T) {
	// The engine draws one character in one column. A glyph the terminal draws
	// two columns wide would make the printed row one column too long.
	g, _ := buildOK(t, "a\U0001F600b\n", 10, 4)
	row := g.rows[0]
	if row[0].Symbol != "a" || row[1].Symbol != "" || row[2].Symbol != "" || row[3].Symbol != "b" {
		t.Errorf("the row is %q %q %q %q, want a, two blanks, b",
			row[0].Symbol, row[1].Symbol, row[2].Symbol, row[3].Symbol)
	}
}

func TestACombiningMarkJoinsTheCellBeforeIt(t *testing.T) {
	g, _ := buildOK(t, "éx\n", 10, 4)
	row := g.rows[0]
	if row[0].Symbol != "é" {
		t.Errorf("the first cell is %q, want e with its accent", row[0].Symbol)
	}
	if row[1].Symbol != "x" {
		t.Errorf("the accent took a column of its own: second cell is %q", row[1].Symbol)
	}
}

func TestColoredSaysWhetherTheInputBroughtColours(t *testing.T) {
	plain, _ := buildOK(t, "hello\n", 20, 4)
	if plain.colored {
		t.Error("plain text was read as coloured")
	}
	painted, _ := buildOK(t, "\x1b[31mhello\x1b[0m\n", 20, 4)
	if !painted.colored {
		t.Error("coloured text was read as plain")
	}
}

func TestThePenAtTheCutIsTheOneTheTailStartsIn(t *testing.T) {
	// Green opens on the first line and is never closed. The second line turns
	// red and does not fit, so it goes to the tail whole and is printed from
	// its own start: the pen the tail begins in is the green, not the red the
	// rejected line was read in.
	g, _ := buildOK(t, "\x1b[32mone\n\x1b[31mabcdefghij\n", 4, 2)
	if len(g.rows) != 1 {
		t.Fatalf("the canvas took %d rows, want 1", len(g.rows))
	}
	if !g.pen.hasFg {
		t.Fatal("the pen at the cut carries no colour")
	}
	if !strings.Contains(g.pen.escape(), "38;2;0;205;0") {
		t.Errorf("the pen escape is %q, want the green the canvas ended in", g.pen.escape())
	}
}

func TestReadLineStopsAtTheCap(t *testing.T) {
	// A line with no end would otherwise put the whole input in memory.
	br := bufio.NewReader(strings.NewReader(strings.Repeat("x", 100)))
	line, err := readLine(br, 10)
	if len(line) != 10 {
		t.Errorf("readLine read %d bytes, want the cap of 10", len(line))
	}
	if !errors.Is(err, errLineTooLong) {
		t.Errorf("readLine reported %v, want errLineTooLong", err)
	}
}

func TestAnEnormousLineDoesNotGoOnTheCanvas(t *testing.T) {
	in := strings.Repeat("x", maxLineBytes+10) + "\n"
	g, tail := buildOK(t, in, 80, 5)
	if len(g.rows) != 0 {
		t.Errorf("a line over the cap put %d rows on the canvas", len(g.rows))
	}
	rest, _ := io.ReadAll(tail)
	if len(rest) < maxLineBytes {
		t.Errorf("the tail is %d bytes, want the whole line back", len(rest))
	}
}

func TestWideRangesAreSortedAndDisjoint(t *testing.T) {
	for i := 1; i < len(wideRanges); i++ {
		if wideRanges[i-1].hi >= wideRanges[i].lo {
			t.Fatalf("range %d ends at %#x and range %d starts at %#x",
				i-1, wideRanges[i-1].hi, i, wideRanges[i].lo)
		}
		if wideRanges[i].lo > wideRanges[i].hi {
			t.Fatalf("range %d is backwards", i)
		}
	}
}

func TestRuneWidth(t *testing.T) {
	cases := map[rune]int{
		'a': 1, ' ': 1, 'é': 1, ' ': 1,
		'中': 2, 'ﾃ': 1, 'Ｔ': 2, '\U0001F600': 2,
		'́': 0, '‍': 0, '️': 0,
	}
	for r, want := range cases {
		if got := runeWidth(r); got != want {
			t.Errorf("runeWidth(%#x) is %d, want %d", r, got, want)
		}
	}
}
