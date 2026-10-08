package tuiffects

import (
	"strconv"
	"strings"
	"testing"
	"unicode/utf8"
)

// sgrStyle is the part of a cell's look that SGR sets.
type sgrStyle struct {
	bold, italic, underline bool
	colors                  ColorPair
}

func styleOf(v *CharacterVisual) sgrStyle {
	if v == nil {
		return sgrStyle{}
	}
	return sgrStyle{bold: v.Bold, italic: v.Italic, underline: v.Underline, colors: v.Colors}
}

// applySGR updates style from the parameters of one SGR sequence, the way a
// terminal does. It knows only the parameters the frame writer uses.
func applySGR(t *testing.T, style *sgrStyle, params string) {
	t.Helper()
	if params == "" {
		*style = sgrStyle{}
		return
	}
	fields := strings.Split(params, ";")
	for i := 0; i < len(fields); i++ {
		code, err := strconv.Atoi(fields[i])
		if err != nil {
			t.Fatalf("SGR parameter %q in %q", fields[i], params)
		}
		switch code {
		case 0:
			*style = sgrStyle{}
		case 1:
			style.bold = true
		case 3:
			style.italic = true
		case 4:
			style.underline = true
		case 38, 48:
			if i+4 >= len(fields) || fields[i+1] != "2" {
				t.Fatalf("SGR %q: colour %d is not truecolor", params, code)
			}
			var rgb [3]uint8
			for k := range rgb {
				n, err := strconv.Atoi(fields[i+2+k])
				if err != nil || n < 0 || n > 255 {
					t.Fatalf("SGR %q: channel %q", params, fields[i+2+k])
				}
				rgb[k] = uint8(n)
			}
			c := RGB(rgb[0], rgb[1], rgb[2])
			if code == 38 {
				style.colors.Fg, style.colors.HasFg = c, true
			} else {
				style.colors.Bg, style.colors.HasBg = c, true
			}
			i += 4
		default:
			t.Fatalf("SGR %q: unexpected parameter %d", params, code)
		}
	}
}

// frameStyles reads frame as a terminal would and returns the style each
// printed rune is drawn in, row by row. It treats every rune as one cell,
// which holds for frames whose symbols are single runes.
func frameStyles(t *testing.T, frame string) [][]sgrStyle {
	t.Helper()
	var out [][]sgrStyle
	for _, line := range strings.Split(frame, "\n") {
		var style sgrStyle
		var row []sgrStyle
		for i := 0; i < len(line); {
			if strings.HasPrefix(line[i:], "\x1b[") {
				end := strings.IndexByte(line[i:], 'm')
				if end < 0 {
					t.Fatalf("unterminated CSI: %q", around(line, i))
				}
				applySGR(t, &style, line[i+2:i+end])
				i += end + 1
				continue
			}
			_, size := utf8.DecodeRuneInString(line[i:])
			row = append(row, style)
			i += size
		}
		out = append(out, row)
	}
	return out
}

// countFg counts the cells of frame drawn with fg as their foreground.
func countFg(t *testing.T, frame string, fg Color) int {
	t.Helper()
	n := 0
	for _, row := range frameStyles(t, frame) {
		for _, style := range row {
			if style.colors.HasFg && style.colors.Fg == fg {
				n++
			}
		}
	}
	return n
}

// assertFrameDrawsRows reads frame as a terminal would and checks that every
// cell shows the symbol and the style FrameRows gives for it. An empty cell
// must be a space with no style. Each row must end with no style left on, so
// a host can write the rows anywhere.
func assertFrameDrawsRows(t *testing.T, label, frame string, rows [][]*CharacterVisual) {
	t.Helper()
	pos := 0
	var style sgrStyle
	readSGR := func() {
		for strings.HasPrefix(frame[pos:], "\x1b[") {
			end := strings.IndexByte(frame[pos:], 'm')
			if end < 0 {
				t.Fatalf("%s: unterminated CSI at byte %d", label, pos)
			}
			applySGR(t, &style, frame[pos+2:pos+end])
			pos += end + 1
		}
	}
	for y, row := range rows {
		if y > 0 {
			if pos >= len(frame) || frame[pos] != '\n' {
				t.Fatalf("%s: row %d does not start with a newline at byte %d: %q", label, y, pos, around(frame, pos))
			}
			pos++
		}
		for x, visual := range row {
			readSGR()
			symbol := " "
			if visual != nil {
				symbol = visual.Symbol
			}
			if !strings.HasPrefix(frame[pos:], symbol) {
				t.Fatalf("%s: cell %d,%d: want symbol %q at %q", label, x, y, symbol, around(frame, pos))
			}
			if want := styleOf(visual); style != want {
				t.Fatalf("%s: cell %d,%d %q: style %+v, want %+v", label, x, y, symbol, style, want)
			}
			pos += len(symbol)
		}
		readSGR()
		if style != (sgrStyle{}) {
			t.Fatalf("%s: row %d ends with style %+v still on", label, y, style)
		}
	}
	if pos != len(frame) {
		t.Fatalf("%s: %d bytes left after the last row: %q", label, len(frame)-pos, around(frame, pos))
	}
}

// themedScreenGrid is a dense capture that looks like a real screen: 16
// colours, each held for a run of 24 cells, as a theme colours a prompt, a
// file name or a status line.
func themedScreenGrid(cols, rows int) [][]InputCell {
	grid := make([][]InputCell, rows)
	for y := range grid {
		row := make([]InputCell, cols)
		for x := range row {
			shade := ((y*cols + x) / 24) % 16
			row[x] = InputCell{
				Symbol: string(rune('a' + (x+y)%26)),
				Fg:     RGB(uint8(shade*7), uint8(shade*11), 200),
				HasFg:  true,
			}
		}
		grid[y] = row
	}
	return grid
}

// fullScreenEngine builds a registered effect over a dense capture, the way
// a screen saver host does.
func fullScreenEngine(t *testing.T, descriptor Descriptor, grid [][]InputCell) (*Engine, Effect) {
	t.Helper()
	cols, rows := len(grid[0]), len(grid)
	term := NewTerminalFromCells(grid, TerminalConfig{
		Width: cols, Height: rows,
		ExistingColorHandling: DynamicExistingColors,
		MakeFillCharacters:    descriptor.NeedsFillCharacters,
		AnchorText:            AnchorSW,
	})
	engine := NewEngine(term, NewRng(1))
	effect := descriptor.New()
	if err := effect.Build(engine); err != nil {
		t.Fatalf("%s: Build: %v", descriptor.Name, err)
	}
	return engine, effect
}

// TestFrameDrawsTheSameCellsAsFrameRows checks that the ANSI frame, read the
// way a terminal reads it, shows every cell FrameRows reports, for every
// effect over a full screen. It is the guard on the frame writer emitting SGR
// only when the style changes: a run that skips a change, or a reset a host
// relied on, draws a cell in the wrong colour.
//
// Negative control: dropping Colors from sameStyle, so a colour change
// inside a run is not written, fails at binarypath frame 1.
func TestFrameDrawsTheSameCellsAsFrameRows(t *testing.T) {
	// 80x24 keeps this quick. The demo module runs the same check at 200x50
	// through Ultraviolet, the parser Bubble Tea hosts use.
	const cols, rows = 80, 24
	checkpoints := map[int]bool{0: true, 1: true, 30: true, 120: true, 300: true}
	for _, descriptor := range Descriptors() {
		// Every cell differs in colour from its neighbour, so every run
		// boundary the writer can get wrong is on the screen.
		engine, effect := fullScreenEngine(t, descriptor, fullScreenGrid(cols, rows, 16))
		for frame := 0; frame <= 300; frame++ {
			if frame > 0 && !effect.Advance(engine) {
				break
			}
			if checkpoints[frame] {
				label := descriptor.Name + " frame " + strconv.Itoa(frame)
				assertFrameDrawsRows(t, label, engine.Frame(), engine.FrameRows())
			}
		}
	}
}

// TestSettledFrameIsSmall is the budget for a full-screen frame. The frame is
// handed to a host that parses it again every time it is drawn, so its size
// is the host's cost. tuios draws its screen saver this way.
//
// A cell in a run of the same style needs only its symbol. Wrapping every
// cell in its own SGR and reset made a settled 200x50 highlight frame 222,513
// bytes, where 19,609 draw the same cells.
//
// Negative control: writing each visual's own SGR and reset per cell takes
// this frame to 222,513 bytes.
func TestSettledFrameIsSmall(t *testing.T) {
	descriptor, _ := Lookup("highlight")
	engine, effect := fullScreenEngine(t, descriptor, themedScreenGrid(200, 50))
	for i := 0; i < 300; i++ {
		if !effect.Advance(engine) {
			break
		}
	}
	frame := engine.Frame()
	t.Logf("highlight at 200x50 after 300 frames: %d bytes", len(frame))
	const budget = 25_000
	if len(frame) > budget {
		t.Errorf("frame is %d bytes, want under %d", len(frame), budget)
	}
}

// TestFrameWritesEachAttributeChange covers the attribute half of sameStyle.
// No shipped effect sets italic or underline, and bold comes only from the
// input, so the full-screen check above never puts two cells side by side
// that differ only in an attribute.
//
// Negative control: with only Colors compared in sameStyle, the bold cell is
// drawn plain.
func TestFrameWritesEachAttributeChange(t *testing.T) {
	colors := ColorPair{Fg: RGB(200, 100, 50), HasFg: true}
	params := []VisualParams{
		{Colors: colors},
		{Colors: colors, Bold: true},
		{Colors: colors, Bold: true},
		{Colors: colors, Italic: true},
		{Colors: colors, Underline: true},
		{Colors: colors},
	}
	row := make([]InputCell, len(params))
	for x := range row {
		row[x] = InputCell{Symbol: string(rune('a' + x))}
	}
	term := NewTerminalFromCells([][]InputCell{row}, TerminalConfig{Width: len(row), Height: 1})
	for i, ch := range term.InputCharacters {
		term.SetCharacterVisibility(ch, true)
		ch.Animation.currentVisual = NewCharacterVisual(ch.InputSymbol, params[i])
	}
	assertFrameDrawsRows(t, "attributes", term.Frame(), term.FrameRows())
}
