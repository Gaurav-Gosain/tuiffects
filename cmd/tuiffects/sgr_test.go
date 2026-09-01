package main

import (
	"strings"
	"testing"

	tfx "github.com/Gaurav-Gosain/tuiffects"
)

// cellsOf renders one line with a fresh pen and returns the cells.
func cellsOf(t *testing.T, line string) []tfx.InputCell {
	t.Helper()
	pen := newSGRState()
	cells, _ := renderLine([]byte(line), &pen)
	return cells
}

func TestSGRReadsEveryColourForm(t *testing.T) {
	cases := []struct {
		name string
		line string
		want tfx.Color
	}{
		{"base 30 to 37", "\x1b[31mx", tfx.RGB(205, 0, 0)},
		{"bright 90 to 97", "\x1b[94mx", tfx.RGB(92, 92, 255)},
		{"palette cube", "\x1b[38;5;196mx", tfx.RGB(255, 0, 0)},
		{"palette grey", "\x1b[38;5;244mx", tfx.RGB(128, 128, 128)},
		{"palette named", "\x1b[38;5;2mx", tfx.RGB(0, 205, 0)},
		{"true colour", "\x1b[38;2;17;34;51mx", tfx.RGB(17, 34, 51)},
		{"true colour in colons", "\x1b[38:2::17:34:51mx", tfx.RGB(17, 34, 51)},
		{"palette in colons", "\x1b[38:5:196mx", tfx.RGB(255, 0, 0)},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cells := cellsOf(t, c.line)
			if len(cells) != 1 {
				t.Fatalf("got %d cells, want 1", len(cells))
			}
			if !cells[0].HasFg {
				t.Fatalf("the cell carries no foreground")
			}
			if cells[0].Fg != c.want {
				t.Errorf("foreground is %v, want %v", cells[0].Fg, c.want)
			}
		})
	}
}

func TestSGRReadsBackgroundsAndReset(t *testing.T) {
	cells := cellsOf(t, "\x1b[41ma\x1b[0mb")
	if len(cells) != 2 {
		t.Fatalf("got %d cells, want 2", len(cells))
	}
	if !cells[0].HasBg || cells[0].Bg != tfx.RGB(205, 0, 0) {
		t.Errorf("the first cell has bg %v %v, want red", cells[0].HasBg, cells[0].Bg)
	}
	if cells[1].HasBg {
		t.Errorf("the reset did not clear the background")
	}
}

func TestBoldOverABaseColourResolvesToTheBrightTwin(t *testing.T) {
	// This is what `ls --color` writes for a directory.
	cells := cellsOf(t, "\x1b[0m\x1b[01;34mbin")
	if len(cells) == 0 {
		t.Fatal("no cells")
	}
	if cells[0].Fg != tfx.RGB(92, 92, 255) {
		t.Errorf("foreground is %v, want bright blue 92 92 255", cells[0].Fg)
	}
	// A bold true colour is left exactly as it was asked for.
	cells = cellsOf(t, "\x1b[1;38;2;10;20;30mx")
	if cells[0].Fg != tfx.RGB(10, 20, 30) {
		t.Errorf("bold moved a true colour to %v", cells[0].Fg)
	}
}

func TestInvertSwapsForegroundAndBackground(t *testing.T) {
	cells := cellsOf(t, "\x1b[31;47;7mx")
	if cells[0].Fg != tfx.RGB(229, 229, 229) || cells[0].Bg != tfx.RGB(205, 0, 0) {
		t.Errorf("invert gave fg %v bg %v, want them the other way round", cells[0].Fg, cells[0].Bg)
	}
}

func TestNonSGREscapesDrawNothing(t *testing.T) {
	cases := map[string]string{
		"osc 8 hyperlink":  "\x1b]8;;https://example.com\x07ok\x1b]8;;\x07",
		"cursor movement":  "\x1b[2Kok\x1b[10G",
		"charset select":   "\x1b(Bok",
		"private mode set": "\x1b[?25lok\x1b[?25h",
	}
	for name, line := range cases {
		t.Run(name, func(t *testing.T) {
			cells := cellsOf(t, line)
			var got strings.Builder
			for _, c := range cells {
				got.WriteString(c.Symbol)
			}
			if got.String() != "ok" {
				t.Errorf("the line drew %q, want %q", got.String(), "ok")
			}
		})
	}
}

func TestUnknownSGRParametersDoNotStopTheRest(t *testing.T) {
	// 53 is overline, which this does not implement. The colour after it must
	// still arrive.
	cells := cellsOf(t, "\x1b[53;31mx")
	if !cells[0].HasFg || cells[0].Fg != tfx.RGB(205, 0, 0) {
		t.Errorf("the colour after an unknown parameter was lost: %v", cells[0])
	}
}

func TestPenEscapeRestoresTheState(t *testing.T) {
	pen := newSGRState()
	pen.applySGR("1;31;44")
	got := pen.escape()
	for _, want := range []string{"\x1b[0m", "\x1b[1m", "38;2;255;0;0", "48;2;0;0;238"} {
		if !strings.Contains(got, want) {
			t.Errorf("the escape %q does not contain %q", got, want)
		}
	}
}
