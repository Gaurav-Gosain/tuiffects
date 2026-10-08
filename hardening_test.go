package tuiffects

import (
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"
)

// assertOnlySGR fails unless every escape in frame is a complete SGR sequence
// and the frame holds no other control character. A frame is written straight
// to a terminal, so a stray BEL, ESC or C1 control from the input would be
// read by that terminal as the start of a command.
func assertOnlySGR(t *testing.T, label, frame string) {
	t.Helper()
	for i := 0; i < len(frame); {
		b := frame[i]
		switch {
		case b == 0x1b:
			j := i + 1
			if j >= len(frame) || frame[j] != '[' {
				t.Fatalf("%s: ESC at byte %d does not start a CSI: %q", label, i, around(frame, i))
			}
			j++
			for j < len(frame) && (frame[j] == ';' || (frame[j] >= '0' && frame[j] <= '9')) {
				j++
			}
			if j >= len(frame) || frame[j] != 'm' {
				t.Fatalf("%s: CSI at byte %d is not an SGR: %q", label, i, around(frame, i))
			}
			i = j + 1
		case b == '\n':
			i++
		case b < 0x20 || b == 0x7f:
			t.Fatalf("%s: control byte %#x at byte %d: %q", label, b, i, around(frame, i))
		case b >= 0x80:
			r, size := utf8.DecodeRuneInString(frame[i:])
			// A terminal that reads 8-bit controls takes a raw byte such as
			// 0x9b as CSI, so a byte that is not valid UTF-8 fails too.
			if r == utf8.RuneError && size == 1 {
				t.Fatalf("%s: invalid UTF-8 byte %#x at byte %d: %q", label, b, i, around(frame, i))
			}
			if r >= 0x80 && r <= 0x9f {
				t.Fatalf("%s: C1 control U+%04X at byte %d: %q", label, r, i, around(frame, i))
			}
			i += size
		default:
			i++
		}
	}
}

func around(s string, i int) string {
	return s[max(0, i-24):min(len(s), i+24)]
}

// TestTextInputDropsControlCharacters covers text that carries terminal
// control characters, as piped or captured output does.
//
// Each control used to become a character of its own and was written back out
// in the frame. The per-cell SGR wrapping broke a sequence apart by accident,
// so no whole OSC reached the terminal, but a raw BEL and ESC did, and the
// line took a column per control. Writing SGR only on a style change would
// have put the sequence back together.
//
// Negative control: with preprocessLines keeping controls, the frame carries
// the raw ESC and BEL and the line is 17 characters, not 14.
func TestTextInputDropsControlCharacters(t *testing.T) {
	const input = "ok\x07\x1b]0;pwned\x07done"
	const visible = "ok]0;pwneddone"
	for _, name := range []string{"decrypt", "print", "wipe"} {
		descriptor, _ := Lookup(name)
		term := NewTerminalFromText(input, TerminalConfig{
			Width: 30, Height: 2, MakeFillCharacters: descriptor.NeedsFillCharacters,
		})
		var got strings.Builder
		for _, ch := range term.InputCharacters {
			got.WriteString(ch.InputSymbol)
		}
		if got.String() != visible {
			t.Errorf("%s: input characters spell %q, want %q", name, got.String(), visible)
		}
		engine := NewEngine(term, NewRng(1))
		frames, err := Run(descriptor.New(), engine, 5000)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		for i, frame := range frames {
			assertOnlySGR(t, name+" frame "+strconv.Itoa(i), frame)
		}
	}

	// C1 controls, DEL and a lone carriage return go too.
	term := NewTerminalFromText("a\u009b31mb\x7fc\rd\x00e", TerminalConfig{Width: 20, Height: 1})
	var got strings.Builder
	for _, ch := range term.InputCharacters {
		got.WriteString(ch.InputSymbol)
	}
	if got.String() != "a31mbcde" {
		t.Errorf("input characters spell %q, want %q", got.String(), "a31mbcde")
	}
}

// TestCellInputDropsControlCharacters is the same guard for a captured grid.
// A host that hands over its own cells is trusted to have parsed them, but a
// cell symbol is still a string from someone else's output.
//
// Negative control: without the sanitising in NewTerminalFromCells the ESC
// cell reaches the frame as a raw ESC. Without the UTF-8 check before it, the
// raw 0x9b byte and the split U+009D reach the frame.
func TestCellInputDropsControlCharacters(t *testing.T) {
	grid := [][]InputCell{{
		{Symbol: "a"},
		{Symbol: "\x1b"},
		{Symbol: "b\x07"},
		{Symbol: "\u009d", Bg: RGB(1, 2, 3), HasBg: true},
		{Symbol: "c"},
	}}
	term := NewTerminalFromCells(grid, TerminalConfig{Width: 5, Height: 1})
	var got []string
	for _, ch := range term.InputCharacters {
		got = append(got, ch.InputSymbol)
	}
	// The ESC cell is blank with no background, so it is dropped. The C1 cell
	// keeps its background, so it stays as a blank that carries it.
	if want := []string{"a", "b", " ", "c"}; strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("input characters = %q, want %q", got, want)
	}
	for _, ch := range term.InputCharacters {
		term.SetCharacterVisibility(ch, true)
	}
	assertOnlySGR(t, "cells", term.Frame())

	// Bytes that are not valid UTF-8 go too. A raw 0x9b is an 8-bit CSI to a
	// terminal that reads 8-bit controls. A lone 0xc2 in one cell and 0x9d at
	// the start of the next join in the frame into U+009D, the OSC
	// introducer, though neither cell holds a control on its own.
	grid = [][]InputCell{{
		{Symbol: "a"},
		{Symbol: "\x9b31m"},
		{Symbol: "\xc2"},
		{Symbol: "\x9dx"},
		{Symbol: "b"},
	}}
	term = NewTerminalFromCells(grid, TerminalConfig{Width: 5, Height: 1})
	got = got[:0]
	for _, ch := range term.InputCharacters {
		got = append(got, ch.InputSymbol)
	}
	// The 0xc2 cell is empty once the byte goes, so it is dropped.
	if want := []string{"a", "31m", "x", "b"}; strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("invalid UTF-8: input characters = %q, want %q", got, want)
	}
	for _, ch := range term.InputCharacters {
		term.SetCharacterVisibility(ch, true)
	}
	assertOnlySGR(t, "invalid UTF-8 cells", term.Frame())
}

// TestZeroSizeConfigTakesTheGridSize covers the fallback the doc of
// NewTerminalFromCells relies on: a config with no size uses the grid's.
//
// Negative control: with the fallback after normalizeConfig, the canvas is
// 1x1 and keeps 1 of the 180 characters.
func TestZeroSizeConfigTakesTheGridSize(t *testing.T) {
	term := NewTerminalFromCells(fullScreenGrid(30, 6, 4), TerminalConfig{})
	if term.Canvas.Right != 30 || term.Canvas.Top != 6 {
		t.Errorf("canvas is %dx%d, want 30x6", term.Canvas.Right, term.Canvas.Top)
	}
	if got := len(term.InputCharacters); got != 180 {
		t.Errorf("input characters = %d, want 180", got)
	}
}

// TestFrameRowsReusesItsSlices holds FrameRows to its doc, which tells the
// caller the slices are reused between calls.
//
// Negative control: allocating the rows on each call costs 51 allocations at
// 200x50, one per row plus the outer slice. Without the full slice
// expression, the capacity of the first row is 10,000, the whole grid.
func TestFrameRowsReusesItsSlices(t *testing.T) {
	const cols, rows = 200, 50
	term := NewTerminalFromCells(fullScreenGrid(cols, rows, 16), TerminalConfig{Width: cols, Height: rows})
	for _, ch := range term.InputCharacters {
		term.SetCharacterVisibility(ch, true)
	}
	first := term.FrameRows()
	if len(first) != rows || len(first[0]) != cols {
		t.Fatalf("FrameRows is %dx%d, want %dx%d", len(first[0]), len(first), cols, rows)
	}
	// Each row is capped at its own width, so an append to one row cannot
	// write into the next.
	if got := cap(first[0]); got != cols {
		t.Errorf("cap of a row = %d, want %d", got, cols)
	}
	if allocs := testing.AllocsPerRun(20, func() { _ = term.FrameRows() }); allocs != 0 {
		t.Errorf("FrameRows allocates %.0f times per call, want 0", allocs)
	}
	// A cell that empties must read nil on the next call, not keep the
	// visual from the call before.
	term.SetCharacterVisibility(term.InputCharacters[0], false)
	top := term.InputCharacters[0].InputCoord
	again := term.FrameRows()
	if v := again[rows-top.Row][top.Column-1]; v != nil {
		t.Errorf("a hidden cell still reads %q", v.Symbol)
	}
}

// TestEveryEffectAcceptsAnEmptyScreen covers a host that runs an effect over
// a blank capture. A screen saver over a cleared pane does that.
//
// Negative control: swarm returns "the input has no characters to swarm".
func TestEveryEffectAcceptsAnEmptyScreen(t *testing.T) {
	for _, descriptor := range Descriptors() {
		term := NewTerminalFromCells(make([][]InputCell, 5), TerminalConfig{
			Width: 20, Height: 5,
			MakeFillCharacters:    descriptor.NeedsFillCharacters,
			ExistingColorHandling: DynamicExistingColors,
		})
		engine := NewEngine(term, NewRng(1))
		if _, err := Run(descriptor.New(), engine, 20000); err != nil {
			t.Errorf("%s: %v", descriptor.Name, err)
		}
	}
}
