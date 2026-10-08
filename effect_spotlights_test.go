package tuiffects

import (
	"strconv"
	"strings"
	"testing"
)

// spotlightsTestConfig is the default spotlights with a short search. The
// default wanders for 550 frames before anything converges, which is most of a
// test run and none of what a test is looking at.
func spotlightsTestConfig() SpotlightsConfig {
	config := DefaultSpotlightsConfig()
	config.SearchDuration = 20
	return config
}

// spotlightsGrid builds a capture where every cell holds the same symbol and
// the same colours, so a difference between two cells can only be the beam.
func spotlightsGrid(cols, rows int, fg Color) [][]InputCell {
	grid := make([][]InputCell, rows)
	for y := range grid {
		row := make([]InputCell, cols)
		for x := range row {
			row[x] = InputCell{Symbol: "#", Fg: fg, HasFg: true}
		}
		grid[y] = row
	}
	return grid
}

// TestSpotlightsSettlesIntoTheInputText runs the effect to completion. The
// beams only recolour, so the text has to come out as it went in.
//
// Negative control: passing a fixed symbol to SetAppearance in illuminateChars
// instead of ch.InputSymbol. Run and watched to fail with a final frame of
// "xxxxxxxxxx".
func TestSpotlightsSettlesIntoTheInputText(t *testing.T) {
	const input = "spotlights"
	term := NewTerminalFromText(input, TerminalConfig{Width: 30, Height: 8})
	engine := NewEngine(term, NewRng(4))

	frames, err := Run(NewSpotlights(spotlightsTestConfig()), engine, 4000)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(frames) == 0 {
		t.Fatal("the effect produced no frames")
	}
	if len(frames) >= 4000 {
		t.Fatal("the effect never finished within the frame cap")
	}
	rows := nonBlank(frames[len(frames)-1])
	if len(rows) != 1 || rows[0] != input {
		t.Errorf("the final frame reads %q, want %q", rows, input)
	}
}

// TestSpotlightsStartsDarkAndEndsLit checks the shape of the whole run: the
// screen begins dimmed, and it ends at full brightness with nothing left in
// the dark.
//
// The first frame is not entirely dark. The beams spawn one cell outside the
// canvas and are already the width they search at, so a beam sitting past an
// edge lights a strip of the screen before it has moved. What matters is that
// the screen is mostly dark and that the run finishes with all of it lit.
//
// Negative controls, both run and watched to fail. Dropping the ActivatePath
// in Build so the beams never move: the run hits the frame cap, because a beam
// the engine dropped from the active set never finishes its run to the middle
// and the widening never starts. Dropping settle, which is this port's one
// deviation from upstream: the final frame carries the lit colour 406 times
// against the wanted 480, the missing cells being the vignette upstream leaves
// around the edge of the last beam.
func TestSpotlightsStartsDarkAndEndsLit(t *testing.T) {
	const cols, rows = 40, 12
	fg := RGB(200, 180, 60)
	term := NewTerminalFromCells(spotlightsGrid(cols, rows, fg), TerminalConfig{
		Width: cols, Height: rows, ExistingColorHandling: DynamicExistingColors,
	})
	engine := NewEngine(term, NewRng(5))

	frames, err := Run(NewSpotlights(spotlightsTestConfig()), engine, 4000)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(frames) == 0 {
		t.Fatal("the effect produced no frames")
	}
	if len(frames) >= 4000 {
		t.Fatal("the effect never finished within the frame cap")
	}

	dark := AdjustColorBrightness(fg, 0.2)
	if got := countFg(t, frames[0], dark); got < cols*rows/2 {
		t.Errorf("the first frame is dark in %d of its %d cells, want most of the screen dark",
			got, cols*rows)
	}
	last := frames[len(frames)-1]
	if got := countFg(t, last, fg); got != cols*rows {
		t.Errorf("the final frame carries the lit colour %d times, want every one of the %d cells",
			got, cols*rows)
	}
	if countFg(t, last, dark) > 0 {
		t.Error("the final frame still carries the dark the run started in")
	}
}

// TestSpotlightsPassesOverTheScreen is the check on which kind of effect this
// is. Spotlights travels over a picture that is already on the screen; it does
// not assemble one. So under DynamicExistingColors the whole picture, its
// backgrounds included, is on the first frame, dimmed rather than absent.
//
// An effect that got this backwards would still resolve correctly and would
// still pass a final-frame check, which is why this is tested on frame one.
//
// Negative control: hiding every character in Build and showing it when a beam
// first reaches it. Run and watched to fail with a first frame of "a " against
// the wanted "ab".
func TestSpotlightsPassesOverTheScreen(t *testing.T) {
	red, blue := RGB(255, 0, 0), RGB(0, 0, 255)
	grid := [][]InputCell{{
		{Symbol: "a", Fg: red, HasFg: true, Bg: blue, HasBg: true},
		{Symbol: "b", Fg: red, HasFg: true, Bg: blue, HasBg: true},
	}}
	term := NewTerminalFromCells(grid, TerminalConfig{
		Width: 2, Height: 1, ExistingColorHandling: DynamicExistingColors,
	})
	engine := NewEngine(term, NewRng(3))

	frames, err := Run(NewSpotlights(spotlightsTestConfig()), engine, 4000)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(frames) == 0 {
		t.Fatal("the effect produced no frames")
	}
	if got := plain(frames[0]); got != "ab" {
		t.Errorf("the first frame reads %q, want the whole picture %q", got, "ab")
	}
	// A dimmed background is still a background. Losing it would blank the
	// panel a captured cell came from for the length of the run.
	for i, frame := range frames {
		if !strings.Contains(frame, "\x1b[48;2;") {
			t.Fatalf("frame %d lost the input's background: %q", i, frame)
		}
	}
	last := frames[len(frames)-1]
	if got := plain(last); got != "ab" {
		t.Errorf("the final frame reads %q, want %q", got, "ab")
	}
	if !strings.Contains(last, sgrForFg(red)) {
		t.Errorf("the final frame does not resolve to the input's red: %q", last)
	}
	if !strings.Contains(last, sgrForBg(blue)) {
		t.Errorf("the final frame does not resolve to the input's blue background: %q", last)
	}
}

// TestSpotlightsBeamIsAWideSoftPoolOfLight is the check on the thing the
// effect is named for. A spotlight is a pool of light: wider than it is tall,
// because a cell is about twice as tall as it is wide, and brightest in the
// middle with a rim that fades off.
//
// The beam is parked in the middle of the canvas so the shape can be measured
// without waiting for a path to put it somewhere.
//
// Negative control: dropping the falloff branch in illuminateChars so every
// lit character gets the full colour. Run and watched to fail with "the rim of
// the beam is as bright as its middle".
func TestSpotlightsBeamIsAWideSoftPoolOfLight(t *testing.T) {
	const cols, rows = 41, 21
	const radius = 8
	fg := RGB(200, 200, 200)
	term := NewTerminalFromCells(spotlightsGrid(cols, rows, fg), TerminalConfig{
		Width: cols, Height: rows, ExistingColorHandling: DynamicExistingColors,
	})
	engine := NewEngine(term, NewRng(2))
	effect := NewSpotlights(spotlightsTestConfig())
	if err := effect.Build(engine); err != nil {
		t.Fatalf("Build: %v", err)
	}

	center := term.Canvas.Center
	effect.spotlights = effect.spotlights[:1]
	effect.spotlights[0].Motion.SetCoordinate(center)
	effect.illuminateChars(engine, radius)

	if len(effect.illuminated) == 0 {
		t.Fatal("the beam lit nothing")
	}
	minCol, maxCol := cols+1, 0
	minRow, maxRow := rows+1, 0
	for ch := range effect.illuminated {
		minCol = min(minCol, ch.InputCoord.Column)
		maxCol = max(maxCol, ch.InputCoord.Column)
		minRow = min(minRow, ch.InputCoord.Row)
		maxRow = max(maxRow, ch.InputCoord.Row)
	}
	if width, height := maxCol-minCol+1, maxRow-minRow+1; width <= height {
		t.Errorf("the lit area is %d by %d, want it wider than it is tall", width, height)
	}
	if got := maxCol - center.Column; got != radius {
		t.Errorf("the beam reaches %d columns from its middle, want the radius %d", got, radius)
	}
	if got := maxRow - center.Row; got != radius/2 {
		t.Errorf("the beam reaches %d rows from its middle, want half the radius, %d", got, radius/2)
	}

	// The soft edge is measured one cell in from the outermost column. The
	// outermost column itself sits on the floor the falloff is clamped to,
	// which is the same brightness as the unlit screen.
	middle := term.CharacterAtInputCoord(center)
	rim := term.CharacterAtInputCoord(C(center.Column+radius-1, center.Row))
	if middle == nil || rim == nil {
		t.Fatal("the canvas is missing the cells the beam was measured on")
	}
	middleFg := middle.Animation.CurrentVisual().Colors
	rimFg := rim.Animation.CurrentVisual().Colors
	if !middleFg.HasFg || middleFg.Fg != fg {
		t.Errorf("the middle of the beam shows %v, want the input colour %v", middleFg, fg)
	}
	if !rimFg.HasFg {
		t.Fatal("the rim of the beam shows no colour at all")
	}
	if channelSum(rimFg.Fg) >= channelSum(middleFg.Fg) {
		t.Errorf("the rim of the beam is as bright as its middle: rim %v, middle %v", rimFg.Fg, middleFg.Fg)
	}
	// Outside the beam the screen is still in the dark it started in.
	outside := term.CharacterAtInputCoord(C(1, 1))
	if outside == nil {
		t.Fatal("the canvas is missing its corner cell")
	}
	if _, lit := effect.illuminated[outside]; lit {
		t.Error("the corner of the canvas is inside a beam parked in the middle")
	}
	if got := outside.Animation.CurrentVisual().Colors; channelSum(got.Fg) >= channelSum(rimFg.Fg) {
		t.Errorf("the unlit corner shows %v, want it darker than the beam's rim %v", got.Fg, rimFg.Fg)
	}
}

// TestSpotlightsDropsTheExtraBeamsOnConvergence pins the handover from the
// search to the widening: the beams meet, all but one are dropped, and the
// survivor grows a step per frame until it is past the canvas.
//
// Negative control: leaving illuminateRange alone in the widening branch. Run
// and watched to fail on the frame cap, because a beam that never widens never
// reaches the limit that ends the effect.
func TestSpotlightsDropsTheExtraBeamsOnConvergence(t *testing.T) {
	const cols, rows = 40, 12
	term := NewTerminalFromCells(spotlightsGrid(cols, rows, RGB(200, 180, 60)), TerminalConfig{
		Width: cols, Height: rows, ExistingColorHandling: DynamicExistingColors,
	})
	engine := NewEngine(term, NewRng(6))
	config := spotlightsTestConfig()
	config.SpotlightCount = 3
	effect := NewSpotlights(config)
	if err := effect.Build(engine); err != nil {
		t.Fatalf("Build: %v", err)
	}
	if got := len(effect.spotlights); got != 3 {
		t.Fatalf("the effect built %d beams, want 3", got)
	}
	// Every beam starts off the canvas and the search paths bring them in.
	for _, spotlight := range effect.spotlights {
		if term.Canvas.CoordIsInCanvas(spotlight.Motion.CurrentCoord) {
			t.Errorf("a beam started at %v, which is inside the canvas", spotlight.Motion.CurrentCoord)
		}
	}

	widths := []int{}
	for i := 0; i < 4000; i++ {
		if !effect.Advance(engine) {
			break
		}
		if effect.expanding {
			widths = append(widths, effect.illuminateRange)
		}
	}
	if !effect.complete {
		t.Fatal("the effect never finished within the frame cap")
	}
	if got := len(effect.spotlights); got != 1 {
		t.Errorf("%d beams survived the convergence, want 1", got)
	}
	if len(widths) < 2 {
		t.Fatalf("the widening ran for %d frames, want more than one", len(widths))
	}
	for i := 1; i < len(widths); i++ {
		if widths[i] != widths[i-1]+1 {
			t.Fatalf("the beam went from %d to %d wide, want one step at a time", widths[i-1], widths[i])
		}
	}
	// max(40, 12) / 1.5 is 26, so the last width recorded is the first one
	// past it.
	if got := widths[len(widths)-1]; got != 27 {
		t.Errorf("the widening stopped at %d, want 27", got)
	}
}

// sgrForFg is the escape a foreground colour is written as.
func sgrForFg(c Color) string {
	return "\x1b[38;2;" + strconv.Itoa(int(c.R)) + ";" + strconv.Itoa(int(c.G)) + ";" + strconv.Itoa(int(c.B)) + "m"
}

// sgrForBg is the escape a background colour is written as.
func sgrForBg(c Color) string {
	return "\x1b[48;2;" + strconv.Itoa(int(c.R)) + ";" + strconv.Itoa(int(c.G)) + ";" + strconv.Itoa(int(c.B)) + "m"
}
