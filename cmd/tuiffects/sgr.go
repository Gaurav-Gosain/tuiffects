package main

import (
	"strconv"
	"strings"

	tfx "github.com/Gaurav-Gosain/tuiffects"
)

// The SGR reader.
//
// A screen that arrives down a pipe carries its colours in SGR escapes, and
// those colours are the whole reason this program is worth running: the
// library resolves every character back to the colour it came in with, so what
// dissolves and re-forms is your own `ls`, not a recolouring of it. Dropping
// the escapes would leave a grey animation of the right letters.
//
// So this reads SGR, and only SGR. Every other escape sequence is recognised
// far enough to be skipped, because a sequence half-read prints its own tail as
// text and one stray `[0m` in the middle of the picture is worse than no
// colour at all. Cursor movement is not honoured: this is a filter over a
// stream of lines, not a terminal, and a program that positions its cursor is
// drawing something this cannot reconstruct anyway.

// sgrState is the pen: the colours and the weight the next character is drawn
// with. Its zero value is the terminal's own default, which is what a cell
// gets when the input never coloured it.
type sgrState struct {
	fg    tfx.Color
	hasFg bool
	// fgBase is which of the eight base colours the foreground came from, or
	// minus one when it came from anywhere else. See cell.
	fgBase int
	bg     tfx.Color
	hasBg  bool
	bold   bool
	invert bool
}

// newSGRState is the pen a line starts with: the terminal's own defaults.
func newSGRState() sgrState { return sgrState{fgBase: -1} }

// cell resolves the pen to one cell of the grid.
//
// Bold over one of the eight base colours resolves to that colour's bright
// twin, which is what a terminal does with it and what every tool that colours
// its output is counting on: `ls --color` writes a directory as bold blue and
// means bright blue, and the library carries a colour into the animation but
// not a weight, so a pen that kept the dark blue would dissolve a screen that
// was never on the screen.
func (s sgrState) cell(symbol string) tfx.InputCell {
	c := tfx.InputCell{Symbol: symbol, Bold: s.bold}
	fg, hasFg, bg, hasBg := s.fg, s.hasFg, s.bg, s.hasBg
	if s.bold && s.fgBase >= 0 {
		fg = ansi16[s.fgBase+8]
	}
	if s.invert {
		fg, hasFg, bg, hasBg = bg, hasBg, fg, hasFg
	}
	c.Fg, c.HasFg, c.Bg, c.HasBg = fg, hasFg, bg, hasBg
	return c
}

// colored reports whether the pen is holding anything the effect would not
// have chosen for itself.
func (s sgrState) colored() bool { return s.hasFg || s.hasBg }

// escape writes the sequence that puts a terminal back into this state. It is
// used for the rows that did not fit on the canvas: they are printed under the
// animation as ordinary text, and without this they would print in whatever
// colour the last animated row happened to leave behind.
func (s sgrState) escape() string {
	var b strings.Builder
	b.WriteString("\x1b[0m")
	if !s.bold && !s.invert && !s.hasFg && !s.hasBg {
		return b.String()
	}
	if s.bold {
		b.WriteString("\x1b[1m")
	}
	if s.invert {
		b.WriteString("\x1b[7m")
	}
	if s.hasFg {
		fg := s.fg
		if s.bold && s.fgBase >= 0 {
			fg = ansi16[s.fgBase+8]
		}
		b.WriteString(trueColorEscape(38, fg))
	}
	if s.hasBg {
		b.WriteString(trueColorEscape(48, s.bg))
	}
	return b.String()
}

func trueColorEscape(prefix int, c tfx.Color) string {
	return "\x1b[" + strconv.Itoa(prefix) + ";2;" +
		strconv.Itoa(int(c.R)) + ";" +
		strconv.Itoa(int(c.G)) + ";" +
		strconv.Itoa(int(c.B)) + "m"
}

// applySGR runs one CSI ... m sequence over the pen. params is the text
// between the CSI and the m.
//
// An unknown parameter is skipped rather than treated as an error. SGR is an
// open set and terminals have always ignored what they do not know; a parser
// that gave up on the first unrecognised code would lose the rest of a
// perfectly good sequence.
func (s *sgrState) applySGR(params string) {
	codes := sgrParams(params)
	for i := 0; i < len(codes); i++ {
		switch n := codes[i]; {
		case n == 0:
			*s = newSGRState()
		case n == 1:
			s.bold = true
		case n == 22:
			s.bold = false
		case n == 7:
			s.invert = true
		case n == 27:
			s.invert = false
		case n >= 30 && n <= 37:
			s.fg, s.hasFg, s.fgBase = ansi16[n-30], true, n-30
		case n == 39:
			s.fg, s.hasFg, s.fgBase = tfx.Color{}, false, -1
		case n >= 40 && n <= 47:
			s.bg, s.hasBg = ansi16[n-40], true
		case n == 49:
			s.bg, s.hasBg = tfx.Color{}, false
		case n >= 90 && n <= 97:
			s.fg, s.hasFg, s.fgBase = ansi16[n-90+8], true, -1
		case n >= 100 && n <= 107:
			s.bg, s.hasBg = ansi16[n-100+8], true
		case n == 38 || n == 48:
			c, used, ok := extendedColor(codes[i:])
			i += used - 1
			if !ok {
				continue
			}
			if n == 38 {
				s.fg, s.hasFg, s.fgBase = c, true, -1
			} else {
				s.bg, s.hasBg = c, true
			}
		}
	}
}

// extendedColor reads a 38 or 48 that carries its colour in the parameters
// that follow it: `5;n` for a palette index and `2;r;g;b` for a direct one. It
// returns how many parameters it consumed, including the 38 or 48 itself, so
// the caller can step past them whether or not the colour parsed.
func extendedColor(codes []int) (tfx.Color, int, bool) {
	if len(codes) < 2 {
		return tfx.Color{}, len(codes), false
	}
	switch codes[1] {
	case 5:
		if len(codes) < 3 {
			return tfx.Color{}, len(codes), false
		}
		return palette256(codes[2]), 3, true
	case 2:
		if len(codes) < 5 {
			return tfx.Color{}, len(codes), false
		}
		return tfx.RGB(channel(codes[2]), channel(codes[3]), channel(codes[4])), 5, true
	}
	return tfx.Color{}, 2, false
}

func channel(v int) uint8 {
	if v < 0 {
		return 0
	}
	if v > 255 {
		return 255
	}
	return uint8(v)
}

// sgrParams splits the body of an SGR sequence into its numbers.
//
// Parameters are separated by semicolons, and a single parameter may carry
// sub-parameters separated by colons: a terminal that writes `38;2;r;g;b` and
// one that writes `38:2::r:g:b` mean the same colour. Flattening both on both
// separators reads either, and the empty field the second form puts where a
// colour space would go is dropped rather than read as a zero, which would
// otherwise shift the red channel into the green.
func sgrParams(params string) []int {
	if params == "" {
		// A bare `\x1b[m` is `\x1b[0m`.
		return []int{0}
	}
	fields := strings.FieldsFunc(params, func(r rune) bool { return r == ';' || r == ':' })
	codes := make([]int, 0, len(fields))
	for _, f := range fields {
		n, err := strconv.Atoi(f)
		if err != nil {
			continue
		}
		codes = append(codes, n)
	}
	if len(codes) == 0 {
		return []int{0}
	}
	return codes
}

// ansi16 is the first sixteen palette entries, in the values xterm uses.
var ansi16 = [16]tfx.Color{
	tfx.RGB(0, 0, 0), tfx.RGB(205, 0, 0), tfx.RGB(0, 205, 0), tfx.RGB(205, 205, 0),
	tfx.RGB(0, 0, 238), tfx.RGB(205, 0, 205), tfx.RGB(0, 205, 205), tfx.RGB(229, 229, 229),
	tfx.RGB(127, 127, 127), tfx.RGB(255, 0, 0), tfx.RGB(0, 255, 0), tfx.RGB(255, 255, 0),
	tfx.RGB(92, 92, 255), tfx.RGB(255, 0, 255), tfx.RGB(0, 255, 255), tfx.RGB(255, 255, 255),
}

// palette256 resolves an xterm palette index: sixteen named colours, a six by
// six by six cube, and twenty four greys.
func palette256(index int) tfx.Color {
	switch {
	case index < 0 || index > 255:
		return tfx.Color{}
	case index < 16:
		return ansi16[index]
	case index < 232:
		n := index - 16
		steps := [6]uint8{0, 95, 135, 175, 215, 255}
		return tfx.RGB(steps[n/36], steps[(n/6)%6], steps[n%6])
	default:
		v := uint8(8 + (index-232)*10)
		return tfx.RGB(v, v, v)
	}
}
