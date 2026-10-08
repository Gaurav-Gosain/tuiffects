package tuiffects

import (
	"sort"
	"strings"
	"unicode/utf8"
)

// TerminalConfig sets up the canvas an effect draws on.
type TerminalConfig struct {
	// Width and Height are the canvas size in cells. Both must be above zero.
	Width  int
	Height int
	// TabWidth is how many columns a tab expands to. Zero means four.
	TabWidth int
	// ExistingColorHandling decides what happens to colours the input carried.
	ExistingColorHandling ExistingColorHandling
	// AnchorText pins the input block inside the canvas.
	AnchorText Anchor
	// MakeFillCharacters populates every empty canvas cell with a space
	// character that effects can animate. Effects that only touch input
	// characters do not need it, and it costs one character per empty cell.
	MakeFillCharacters bool
}

// InputCell is one cell of a screen capture handed to the engine.
type InputCell struct {
	// Symbol is the grapheme cluster in the cell. An empty string is a blank.
	Symbol string
	Fg     Color
	HasFg  bool
	Bg     Color
	HasBg  bool
	Bold   bool
}

// CharacterFilter selects which populations a character query returns.
type CharacterFilter struct {
	Input     bool
	InnerFill bool
	OuterFill bool
	Added     bool
}

// InputOnly is the default filter: the characters that came from the input.
func InputOnly() CharacterFilter { return CharacterFilter{Input: true} }

// CharacterSort orders a character query.
type CharacterSort int

// The character sorts.
const (
	SortTopToBottomLeftToRight CharacterSort = iota
	SortTopToBottomRightToLeft
	SortBottomToTopLeftToRight
	SortBottomToTopRightToLeft
	SortOutsideRowToMiddle
	SortMiddleRowToOutside
	SortRandom
)

// CharacterGroup buckets a character query into ordered groups.
type CharacterGroup int

// The character groupings.
const (
	GroupColumnLeftToRight CharacterGroup = iota
	GroupColumnRightToLeft
	GroupRowTopToBottom
	GroupRowBottomToTop
	GroupDiagonalBottomLeftToTopRight
	GroupDiagonalTopRightToBottomLeft
	GroupDiagonalTopLeftToBottomRight
	GroupDiagonalBottomRightToTopLeft
	GroupCenterToOutside
	GroupOutsideToCenter
)

// Terminal owns every character and paints the frame.
type Terminal struct {
	Config TerminalConfig
	Canvas *Canvas

	// Characters is every character, in allocation order.
	Characters []*Character

	InputCharacters     []*Character
	InnerFillCharacters []*Character
	OuterFillCharacters []*Character
	AddedCharacters     []*Character

	byInputCoord map[Coord]*Character

	visible []*Character

	// renderCells holds the winning character per cell, row-major from the
	// bottom row. It is reused across frames.
	renderCells []*Character
	// frameBuffer is reused across frames. It is a byte slice rather than a
	// strings.Builder because Builder.Reset drops the buffer: a 28 KB frame
	// then regrew from nothing sixty times a second and allocated 827 KB to
	// produce 28 KB of output.
	frameBuffer []byte
	// rows and rowCells back FrameRows, and are reused across calls the same
	// way frameBuffer is.
	rows     [][]*CharacterVisual
	rowCells []*CharacterVisual

	// visuals is shared by every character in this run. See visualCache.
	visuals *visualCache
}

// NewTerminalFromText builds a terminal from plain text. Tabs expand, trailing
// blank space is dropped, and each remaining rune becomes a character.
func NewTerminalFromText(input string, cfg TerminalConfig) *Terminal {
	cfg = normalizeConfig(cfg)
	lines := preprocessLines(input, cfg.TabWidth)
	t := newTerminal(cfg)

	// Rows count up from the bottom, so the last line of the input is row 1.
	height := len(lines)
	var placed []*Character
	for lineIndex, line := range lines {
		row := height - lineIndex
		column := 0
		for _, r := range line {
			column++
			if r == ' ' {
				// Plain spaces become fill characters rather than input ones,
				// so effects that animate "the text" do not animate its gaps.
				continue
			}
			ch := t.allocate(string(r), C(column, row))
			placed = append(placed, ch)
		}
	}
	t.finishInput(placed)
	return t
}

// NewTerminalFromCells builds a terminal from a captured cell grid. Row zero
// of the grid is the top of the screen, matching how screen captures are
// stored; the canvas flips it so row 1 is the bottom.
//
// A cell's own colours become the character's input colours, so an effect run
// with DynamicExistingColors resolves the screen back to how it looked.
func NewTerminalFromCells(grid [][]InputCell, cfg TerminalConfig) *Terminal {
	// The fallback to the grid's size has to come before normalizeConfig,
	// which raises a zero size to 1.
	if cfg.Width == 0 && len(grid) > 0 {
		cfg.Width = len(grid[0])
	}
	if cfg.Height == 0 {
		cfg.Height = len(grid)
	}
	cfg = normalizeConfig(cfg)
	t := newTerminal(cfg)

	height := len(grid)
	var placed []*Character
	for y, row := range grid {
		canvasRow := height - y
		for x, cell := range row {
			// A control character in a symbol would reach the frame raw, and
			// the host's terminal would read it as a command.
			symbol := stripControls(cell.Symbol)
			blank := symbol == "" || symbol == " "
			// A blank cell that carries its own background is still worth
			// animating: on a captured screen that is the window chrome, the
			// dock and every filled panel, and dropping them would resolve the
			// screen back as bare text on nothing.
			if blank && !cell.HasBg {
				continue
			}
			if symbol == "" {
				symbol = " "
			}
			ch := t.allocate(symbol, C(x+1, canvasRow))
			ch.UsesInputColors = cell.HasFg || cell.HasBg
			ch.Animation.InputColors = ColorPair{
				Fg: cell.Fg, HasFg: cell.HasFg,
				Bg: cell.Bg, HasBg: cell.HasBg,
			}
			ch.Animation.InputBold = cell.Bold
			placed = append(placed, ch)
		}
	}
	t.finishInput(placed)
	return t
}

func normalizeConfig(cfg TerminalConfig) TerminalConfig {
	if cfg.TabWidth <= 0 {
		cfg.TabWidth = 4
	}
	if cfg.Width < 1 {
		cfg.Width = 1
	}
	if cfg.Height < 1 {
		cfg.Height = 1
	}
	return cfg
}

func newTerminal(cfg TerminalConfig) *Terminal {
	return &Terminal{
		Config:       cfg,
		Canvas:       NewCanvas(cfg.Width, cfg.Height),
		byInputCoord: make(map[Coord]*Character),
		visuals:      newVisualCache(),
	}
}

func (t *Terminal) allocate(symbol string, coord Coord) *Character {
	ch := newCharacter(len(t.Characters), symbol, coord, t.visuals)
	ch.index = len(t.Characters)
	ch.Animation.ExistingColorHandling = t.Config.ExistingColorHandling
	t.Characters = append(t.Characters, ch)
	return ch
}

func (t *Terminal) finishInput(placed []*Character) {
	kept := t.Canvas.anchorText(placed, t.Config.AnchorText)
	if len(kept) == 0 {
		// Nothing survived the anchor. Leave the text extents covering the
		// whole canvas so gradient mappings still resolve.
		t.Canvas.TextLeft, t.Canvas.TextRight = t.Canvas.Left, t.Canvas.Right
		t.Canvas.TextBottom, t.Canvas.TextTop = t.Canvas.Bottom, t.Canvas.Top
		t.Canvas.TextWidth, t.Canvas.TextHeight = t.Canvas.Width, t.Canvas.Height
		t.Canvas.TextCenter = t.Canvas.Center
		t.Canvas.TextCenterColumn, t.Canvas.TextCenterRow = t.Canvas.CenterColumn, t.Canvas.CenterRow
	}
	// Drop the characters the anchor pushed off-canvas, then renumber so ids
	// stay dense and in reading order.
	keptSet := make(map[*Character]struct{}, len(kept))
	for _, ch := range kept {
		keptSet[ch] = struct{}{}
	}
	survivors := t.Characters[:0]
	for _, ch := range t.Characters {
		if _, ok := keptSet[ch]; ok {
			survivors = append(survivors, ch)
		}
	}
	t.Characters = survivors
	for i, ch := range t.Characters {
		ch.ID = i
		ch.index = i
		t.byInputCoord[ch.InputCoord] = ch
	}
	t.InputCharacters = append([]*Character(nil), t.Characters...)
	if t.Config.MakeFillCharacters {
		t.makeFillCharacters()
	}
}

// makeFillCharacters gives every empty canvas cell a space character, split by
// whether it falls inside the text block.
func (t *Terminal) makeFillCharacters() {
	for row := 1; row <= t.Canvas.Top; row++ {
		for column := 1; column <= t.Canvas.Right; column++ {
			coord := C(column, row)
			if _, taken := t.byInputCoord[coord]; taken {
				continue
			}
			ch := t.allocate(" ", coord)
			ch.IsFill = true
			t.byInputCoord[coord] = ch
			if t.Canvas.CoordIsInText(coord) {
				t.InnerFillCharacters = append(t.InnerFillCharacters, ch)
			} else {
				t.OuterFillCharacters = append(t.OuterFillCharacters, ch)
			}
		}
	}
}

// AddCharacter creates a character that was not in the input. It joins the
// added population only, so ordinary queries do not see it.
func (t *Terminal) AddCharacter(symbol string, coord Coord) *Character {
	ch := t.allocate(symbol, coord)
	t.AddedCharacters = append(t.AddedCharacters, ch)
	return ch
}

// CharacterAtInputCoord finds the character that started at a coordinate.
func (t *Terminal) CharacterAtInputCoord(coord Coord) *Character {
	return t.byInputCoord[coord]
}

// Neighbors returns the characters directly north, east, south and west of a
// character, in that order, skipping any of the four cells that holds none.
// Diagonals are not neighbours.
//
// Upstream snapshots these four slots onto every character when the terminal
// is built. This looks them up on demand instead. The two are equivalent: the
// snapshot is taken from the same input-coordinate table this reads, and the
// table never changes afterwards because AddCharacter deliberately stays out
// of it. Looking them up saves four pointers per character over a full screen.
//
// A character created with AddCharacter has no neighbours, because it has no
// entry in the table. That matches upstream.
func (t *Terminal) Neighbors(ch *Character) []*Character {
	return t.appendNeighbors(nil, ch)
}

// appendNeighbors is Neighbors with a caller-supplied buffer, so a spanning
// tree walking thousands of cells does not allocate a slice per step.
func (t *Terminal) appendNeighbors(dst []*Character, ch *Character) []*Character {
	c := ch.InputCoord
	for _, coord := range [4]Coord{
		{Column: c.Column, Row: c.Row + 1},
		{Column: c.Column + 1, Row: c.Row},
		{Column: c.Column, Row: c.Row - 1},
		{Column: c.Column - 1, Row: c.Row},
	} {
		if neighbor := t.byInputCoord[coord]; neighbor != nil {
			dst = append(dst, neighbor)
		}
	}
	return dst
}

// SetCharacterVisibility shows or hides a character.
func (t *Terminal) SetCharacterVisibility(ch *Character, visible bool) {
	if ch.IsVisible == visible {
		return
	}
	ch.IsVisible = visible
	if visible {
		t.visible = append(t.visible, ch)
		return
	}
	for i, other := range t.visible {
		if other == ch {
			t.visible[i] = t.visible[len(t.visible)-1]
			t.visible = t.visible[:len(t.visible)-1]
			return
		}
	}
}

// CollectCharacters returns the selected populations in allocation order.
func (t *Terminal) CollectCharacters(filter CharacterFilter) []*Character {
	var all []*Character
	if filter.Input {
		all = append(all, t.InputCharacters...)
	}
	if filter.InnerFill {
		all = append(all, t.InnerFillCharacters...)
	}
	if filter.OuterFill {
		all = append(all, t.OuterFillCharacters...)
	}
	if filter.Added {
		all = append(all, t.AddedCharacters...)
	}
	return all
}

// GetCharacters returns the selected populations in the requested order.
func (t *Terminal) GetCharacters(rng *Rng, filter CharacterFilter, order CharacterSort) []*Character {
	all := t.CollectCharacters(filter)
	// Every sort starts from reading order: top row first, left to right.
	sort.SliceStable(all, func(i, j int) bool {
		a, b := all[i].InputCoord, all[j].InputCoord
		if a.Row != b.Row {
			return a.Row > b.Row
		}
		return a.Column < b.Column
	})
	switch order {
	case SortTopToBottomLeftToRight:
	case SortBottomToTopRightToLeft:
		reverseCharacters(all)
	case SortBottomToTopLeftToRight, SortTopToBottomRightToLeft:
		sort.SliceStable(all, func(i, j int) bool {
			a, b := all[i].InputCoord, all[j].InputCoord
			if a.Row != b.Row {
				return a.Row < b.Row
			}
			return a.Column < b.Column
		})
		if order == SortTopToBottomRightToLeft {
			reverseCharacters(all)
		}
	case SortOutsideRowToMiddle, SortMiddleRowToOutside:
		interleaved := make([]*Character, 0, len(all))
		front, back := 0, len(all)-1
		fromFront := true
		for front <= back {
			if fromFront {
				interleaved = append(interleaved, all[front])
				front++
			} else {
				interleaved = append(interleaved, all[back])
				back--
			}
			fromFront = !fromFront
		}
		all = interleaved
		if order == SortMiddleRowToOutside {
			reverseCharacters(all)
		}
	case SortRandom:
		Shuffle(rng, all)
	}
	return all
}

func reverseCharacters(list []*Character) {
	for i, j := 0, len(list)-1; i < j; i, j = i+1, j-1 {
		list[i], list[j] = list[j], list[i]
	}
}

// GetCharactersGrouped buckets the selected populations into ordered groups.
func (t *Terminal) GetCharactersGrouped(filter CharacterFilter, grouping CharacterGroup) [][]*Character {
	all := t.CollectCharacters(filter)
	sort.SliceStable(all, func(i, j int) bool {
		a, b := all[i].InputCoord, all[j].InputCoord
		if a.Row != b.Row {
			return a.Row < b.Row
		}
		return a.Column < b.Column
	})

	var groups [][]*Character
	switch grouping {
	case GroupColumnLeftToRight, GroupColumnRightToLeft:
		groups = orderedBuckets(all, 0, t.Canvas.Right, func(ch *Character) int { return ch.InputCoord.Column })
		if grouping == GroupColumnRightToLeft {
			reverseGroups(groups)
		}
	case GroupRowBottomToTop, GroupRowTopToBottom:
		groups = orderedBuckets(all, 0, t.Canvas.Top, func(ch *Character) int { return ch.InputCoord.Row })
		if grouping == GroupRowTopToBottom {
			reverseGroups(groups)
		}
	case GroupDiagonalBottomLeftToTopRight, GroupDiagonalTopRightToBottomLeft:
		groups = orderedBuckets(all, 0, t.Canvas.Top+t.Canvas.Right, func(ch *Character) int {
			return ch.InputCoord.Row + ch.InputCoord.Column
		})
		if grouping == GroupDiagonalTopRightToBottomLeft {
			reverseGroups(groups)
		}
	case GroupDiagonalTopLeftToBottomRight, GroupDiagonalBottomRightToTopLeft:
		groups = orderedBuckets(all, t.Canvas.Left-t.Canvas.Top, t.Canvas.Right-t.Canvas.Bottom, func(ch *Character) int {
			return ch.InputCoord.Column - ch.InputCoord.Row
		})
		if grouping == GroupDiagonalBottomRightToTopLeft {
			reverseGroups(groups)
		}
	case GroupCenterToOutside, GroupOutsideToCenter:
		maxDistance := 0
		for _, ch := range all {
			maxDistance = max(maxDistance, abs(ch.InputCoord.Column-t.Canvas.TextCenter.Column)+
				abs(ch.InputCoord.Row-t.Canvas.TextCenter.Row))
		}
		groups = orderedBuckets(all, 0, maxDistance, func(ch *Character) int {
			return abs(ch.InputCoord.Column-t.Canvas.TextCenter.Column) +
				abs(ch.InputCoord.Row-t.Canvas.TextCenter.Row)
		})
		if grouping == GroupOutsideToCenter {
			reverseGroups(groups)
		}
	}
	return groups
}

func orderedBuckets(characters []*Character, firstKey, lastKey int, key func(*Character) int) [][]*Character {
	if firstKey > lastKey {
		return nil
	}
	buckets := make([][]*Character, lastKey-firstKey+1)
	for _, ch := range characters {
		k := key(ch)
		if k < firstKey || k > lastKey {
			continue
		}
		buckets[k-firstKey] = append(buckets[k-firstKey], ch)
	}
	out := make([][]*Character, 0, len(buckets))
	for _, bucket := range buckets {
		if len(bucket) > 0 {
			out = append(out, bucket)
		}
	}
	return out
}

func reverseGroups(groups [][]*Character) {
	for i, j := 0, len(groups)-1; i < j; i, j = i+1, j-1 {
		groups[i], groups[j] = groups[j], groups[i]
	}
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

// paint fills renderCells with the winning character for each cell. Where two
// characters share a cell the higher layer wins, and within a layer the later
// character does.
func (t *Terminal) paint() (width, height int) {
	width, height = t.Canvas.Right, t.Canvas.Top
	needed := width * height
	if cap(t.renderCells) < needed {
		t.renderCells = make([]*Character, needed)
	}
	t.renderCells = t.renderCells[:needed]
	for i := range t.renderCells {
		t.renderCells[i] = nil
	}
	for _, ch := range t.visible {
		coord := ch.Motion.CurrentCoord
		if coord.Row < 1 || coord.Row > height || coord.Column < 1 || coord.Column > width {
			continue
		}
		index := (coord.Row-1)*width + (coord.Column - 1)
		painted := t.renderCells[index]
		if painted == nil || ch.Layer > painted.Layer || (ch.Layer == painted.Layer && ch.ID > painted.ID) {
			t.renderCells[index] = ch
		}
	}
	return width, height
}

// FrameRows returns the current frame as rows of visuals, top row first. A nil
// entry is an empty cell. The slices are reused between calls, so a caller
// that keeps them must copy.
func (t *Terminal) FrameRows() [][]*CharacterVisual {
	width, height := t.paint()
	if cap(t.rowCells) < width*height {
		t.rowCells = make([]*CharacterVisual, width*height)
	}
	if cap(t.rows) < height {
		t.rows = make([][]*CharacterVisual, height)
	}
	cells, rows := t.rowCells[:width*height], t.rows[:height]
	for i := range rows {
		// The full slice expression caps each row at its own width, so a
		// caller that appends to one row cannot write into the next.
		line := cells[i*width : (i+1)*width : (i+1)*width]
		row := height - 1 - i
		for column := range line {
			line[column] = nil
			if ch := t.renderCells[row*width+column]; ch != nil {
				line[column] = ch.Animation.currentVisual
			}
		}
		rows[i] = line
	}
	return rows
}

// Frame returns the current frame as an ANSI string, top row first, with rows
// separated by newlines and no trailing newline. Every row ends with no style
// left on, so a host can write the rows anywhere.
//
// A style is written once at the start of each run of cells that share it,
// not around every cell. The host parses this string again every time it
// draws, and a settled full screen is mostly long runs of one colour: at
// 200x50 that is about 20 KB where a sequence per cell was 220 KB.
func (t *Terminal) Frame() string {
	width, height := t.paint()
	buffer := t.frameBuffer[:0]
	if cap(buffer) < width*height+height {
		buffer = make([]byte, 0, width*height+height)
	}
	for row := height - 1; row >= 0; row-- {
		if row+1 < height {
			buffer = append(buffer, '\n')
		}
		// run is the visual whose style is on, or nil when none is.
		var run *CharacterVisual
		for column := 0; column < width; column++ {
			ch := t.renderCells[row*width+column]
			if ch == nil {
				if run != nil {
					buffer = append(buffer, sgrReset...)
					run = nil
				}
				buffer = append(buffer, ' ')
				continue
			}
			visual := ch.Animation.currentVisual
			if run == nil || !run.sameStyle(visual) {
				if run != nil {
					buffer = append(buffer, sgrReset...)
					run = nil
				}
				if visual.sgr != "" {
					buffer = append(buffer, visual.sgr...)
					run = visual
				}
			}
			buffer = append(buffer, visual.Symbol...)
		}
		if run != nil {
			buffer = append(buffer, sgrReset...)
		}
	}
	t.frameBuffer = buffer
	return string(buffer)
}

// preprocessLines expands tabs, drops carriage returns and every other control
// character, and trims trailing blank lines so the canvas is not padded by
// them.
func preprocessLines(input string, tabWidth int) []string {
	if input == "" {
		return []string{"No input."}
	}
	input = strings.ReplaceAll(input, "\r\n", "\n")
	input = strings.ReplaceAll(input, "\t", strings.Repeat(" ", tabWidth))
	lines := strings.Split(input, "\n")
	for i, line := range lines {
		if !utf8.ValidString(line) {
			line = strings.ToValidUTF8(line, "")
		}
		lines[i] = stripControls(line)
	}
	for len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
		lines = lines[:len(lines)-1]
	}
	if len(lines) == 0 {
		return []string{"No input."}
	}
	return lines
}

// isControl reports whether r is a C0 control, DEL or a C1 control. A
// terminal reads any of them as part of a command, never as text.
func isControl(r rune) bool {
	return r < 0x20 || r == 0x7f || (r >= 0x80 && r <= 0x9f)
}

// stripControls drops every control character from s. Input text and captured
// cells come from other programs' output, and a control kept as a character
// would be written straight back to the host's terminal in the frame.
func stripControls(s string) string {
	if !strings.ContainsFunc(s, isControl) {
		return s
	}
	return strings.Map(func(r rune) rune {
		if isControl(r) {
			return -1
		}
		return r
	}, s)
}
