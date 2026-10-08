// Package framecheck holds the check that a host drawing tuiffects frames with
// Ultraviolet, as Bubble Tea and tuios do, gets the cells the engine painted.
// It lives in the demo module because the library has no dependencies.
package framecheck

import (
	"strings"
	"testing"

	tfx "github.com/Gaurav-Gosain/tuiffects"
	uv "github.com/charmbracelet/ultraviolet"
)

// denseGrid is a capture with every cell filled and the colour changing on
// every cell, so every style change the frame writer could skip is on the
// screen.
func denseGrid(cols, rows int) [][]tfx.InputCell {
	grid := make([][]tfx.InputCell, rows)
	for y := range grid {
		row := make([]tfx.InputCell, cols)
		for x := range row {
			shade := (y*cols + x) % 16
			row[x] = tfx.InputCell{
				Symbol: string(rune('a' + (x+y)%26)),
				Fg:     tfx.RGB(uint8(shade*7), uint8(shade*11), 200),
				HasFg:  true,
			}
			if (x/7+y)%5 == 0 {
				row[x].Bg, row[x].HasBg = tfx.RGB(20, uint8(shade*9), 40), true
			}
		}
		grid[y] = row
	}
	return grid
}

// perCellFrame writes the frame the way tuiffects v0.7.0 did: every cell in
// its own SGR and reset. Its cells are the reference.
func perCellFrame(rows [][]*tfx.CharacterVisual) string {
	var b strings.Builder
	for y, row := range rows {
		if y > 0 {
			b.WriteByte('\n')
		}
		for _, visual := range row {
			if visual == nil {
				b.WriteByte(' ')
			} else {
				b.WriteString(visual.Formatted())
			}
		}
	}
	return b.String()
}

func draw(frame string, cols, rows int) uv.ScreenBuffer {
	buf := uv.NewScreenBuffer(cols, rows)
	uv.NewStyledString(frame).Draw(buf, buf.Bounds())
	return buf
}

// TestFrameDrawsLikeOneSGRPerCell draws every effect's frame with Ultraviolet
// and checks each cell against the same frame written one SGR per cell.
//
// Negative control: dropping Colors from the library's sameStyle, so a colour
// change inside a run is not written, fails at binarypath frame 1.
func TestFrameDrawsLikeOneSGRPerCell(t *testing.T) {
	cols, rows := 200, 50
	if testing.Short() {
		cols, rows = 80, 24
	}
	grid := denseGrid(cols, rows)
	checkpoints := map[int]bool{0: true, 1: true, 30: true, 120: true, 300: true}
	for _, descriptor := range tfx.Descriptors() {
		term := tfx.NewTerminalFromCells(grid, tfx.TerminalConfig{
			Width: cols, Height: rows,
			ExistingColorHandling: tfx.DynamicExistingColors,
			MakeFillCharacters:    descriptor.NeedsFillCharacters,
			AnchorText:            tfx.AnchorSW,
		})
		engine := tfx.NewEngine(term, tfx.NewRng(1))
		effect := descriptor.New()
		if err := effect.Build(engine); err != nil {
			t.Fatalf("%s: Build: %v", descriptor.Name, err)
		}
		for frame := 0; frame <= 300; frame++ {
			if frame > 0 && !effect.Advance(engine) {
				break
			}
			if !checkpoints[frame] {
				continue
			}
			got := draw(engine.Frame(), cols, rows)
			want := draw(perCellFrame(engine.FrameRows()), cols, rows)
			for y := 0; y < rows; y++ {
				for x := 0; x < cols; x++ {
					g, w := got.CellAt(x, y), want.CellAt(x, y)
					if !g.Equal(w) {
						t.Fatalf("%s frame %d: cell %d,%d is %q in %+v, want %q in %+v",
							descriptor.Name, frame, x, y, g.Content, g.Style, w.Content, w.Style)
					}
				}
			}
		}
	}
}
