package main

import (
	"bufio"
	"bytes"
	"errors"
	"io"
	"unicode/utf8"

	tfx "github.com/Gaurav-Gosain/tuiffects"
)

// tabWidth is how far a tab moves the cursor. Terminals use eight.
const tabWidth = 8

// maxLineBytes caps one line of input. A line longer than this is not a line
// anyone meant to look at, and reading it would put the whole file in memory
// to animate one screen of it. The line and everything after it goes to the
// tail, which streams.
const maxLineBytes = 1 << 20

// grid is the screen the effect will be built over.
type grid struct {
	// rows is the canvas, top row first, every row exactly width wide.
	rows [][]tfx.InputCell
	// colored is true when anything in the input carried a colour of its own.
	colored bool
	// pen is the SGR state at the point the grid stopped reading, so the rows
	// that did not fit can be printed in the colour they were written in.
	pen sgrState
}

// buildGrid reads the input into a canvas width columns wide and at most
// height rows tall.
//
// It stops at the first line that would not fit and leaves it unread, so the
// caller can print it and everything after it as ordinary text. Nothing is
// lost and nothing beyond one screen is held in memory: a log file animates
// its first screenful and then scrolls past like any other output.
//
// The returned reader is the rest of the input. It is never nil.
func buildGrid(src io.Reader, width, height int) (grid, io.Reader, error) {
	br, ok := src.(*bufio.Reader)
	if !ok {
		br = bufio.NewReader(src)
	}
	g := grid{pen: newSGRState()}
	if width < 1 || height < 1 {
		return g, br, nil
	}
	var pending []byte
	var failure error
	for len(g.rows) < height {
		line, err := readLine(br, maxLineBytes)
		if len(line) == 0 && err != nil {
			failure = notEOF(err)
			break
		}
		// The pen carries from line to line, so a rejected line has to be able
		// to put it back the way it found it.
		before := g.pen
		cells, colored := renderLine(trimNewline(line), &g.pen)
		wrapped := wrapCells(cells, width)
		if len(g.rows)+len(wrapped) > height {
			// Half of a wrapped line on the canvas and half of it under the
			// animation would draw the same text twice. The whole line waits.
			g.pen = before
			pending = line
			break
		}
		g.rows = append(g.rows, wrapped...)
		g.colored = g.colored || colored
		if err != nil {
			// The input ran out mid line, or the line was longer than the cap.
			// Either way there is no more canvas.
			failure = notEOF(err)
			break
		}
	}
	if len(pending) == 0 {
		// Trailing blank rows are not worth animating, and a command whose
		// output ends in a newline always makes one. They are only dropped
		// when nothing follows them, because a blank line with text under it
		// is part of the picture.
		g.rows = trimBlankRows(g.rows)
	}
	return g, prefixed(pending, br), failure
}

// notEOF turns the end of the input into no error at all. Running out of input
// is how reading finishes, not how it fails.
func notEOF(err error) error {
	if errors.Is(err, io.EOF) || errors.Is(err, errLineTooLong) {
		return nil
	}
	return err
}

// trimNewline drops the line ending readLine kept. The line keeps it so a line
// that goes to the tail goes there exactly as it arrived.
func trimNewline(line []byte) []byte {
	if n := len(line); n > 0 && line[n-1] == '\n' {
		return line[:n-1]
	}
	return line
}

// prefixed puts bytes already read back in front of the rest of the input.
func prefixed(head []byte, rest io.Reader) io.Reader {
	if len(head) == 0 {
		return rest
	}
	return io.MultiReader(bytes.NewReader(head), rest)
}

// readLine reads up to and including one newline, or up to max bytes.
//
// The newline stays on the line. A line the canvas cannot take is put back in
// front of the input and printed under the animation, and it has to go back
// exactly as it arrived. A line that hits the cap comes back with
// errLineTooLong, which stops the canvas and not the output.
func readLine(br *bufio.Reader, max int) ([]byte, error) {
	line := make([]byte, 0, 128)
	for {
		b, err := br.ReadByte()
		if err != nil {
			return line, err
		}
		line = append(line, b)
		if b == '\n' {
			return line, nil
		}
		if len(line) >= max {
			return line, errLineTooLong
		}
	}
}

var errLineTooLong = errors.New("line is longer than the cap")

// renderLine turns one line of input into cells, moving the pen as it goes.
//
// It is a cursor over a row rather than a walk along a string, because the
// controls a real pipe carries move backwards: a carriage return sends a
// progress bar back to the start of the line to draw over itself, and a
// backspace is how a pager underlines. Reading them as forward text puts the
// bar on the screen four times.
func renderLine(line []byte, pen *sgrState) ([]tfx.InputCell, bool) {
	var row []tfx.InputCell
	colored := false
	column := 0
	// put writes one cell at the cursor, growing the row to reach it.
	put := func(cell tfx.InputCell) {
		for len(row) <= column {
			row = append(row, tfx.InputCell{})
		}
		row[column] = cell
	}
	for i := 0; i < len(line); {
		b := line[i]
		switch {
		case b == 0x1b:
			i += skipEscape(line[i:], pen)
			continue
		case b == '\r':
			column = 0
			i++
			continue
		case b == '\b':
			if column > 0 {
				column--
			}
			i++
			continue
		case b == '\t':
			next := (column/tabWidth + 1) * tabWidth
			for ; column < next; column++ {
				if pen.hasBg {
					put(pen.cell(" "))
				}
			}
			i++
			continue
		case b < 0x20 || b == 0x7f:
			// Every other control character draws nothing. A bell in the
			// middle of a build log is not part of the picture.
			i++
			continue
		}
		r, size := utf8.DecodeRune(line[i:])
		if r == utf8.RuneError && size == 1 {
			// Not UTF-8. Drop the byte rather than draw a replacement
			// character: a stray byte in otherwise good text is noise, and a
			// screen of them is a file that was never text.
			i++
			continue
		}
		symbol := string(line[i : i+size])
		i += size
		w := runeWidth(r)
		if w == 0 {
			// A combining mark, a variation selector or a zero width joiner
			// belongs to the cell before it.
			if column > 0 && column-1 < len(row) && row[column-1].Symbol != "" {
				row[column-1].Symbol += symbol
			}
			continue
		}
		if w == 2 {
			// The engine draws one character in one column. A glyph the
			// terminal draws two columns wide would make every printed row one
			// column too long, and the block would wrap and tear itself apart.
			// The cell keeps its place and its background and loses its glyph.
			put(pen.cell(""))
			column++
			put(pen.cell(""))
			column++
			if pen.colored() {
				colored = true
			}
			continue
		}
		put(pen.cell(symbol))
		column++
		if pen.colored() {
			colored = true
		}
	}
	return row, colored
}

// skipEscape consumes one escape sequence and returns its length in bytes. An
// SGR sequence moves the pen; everything else is skipped.
//
// A sequence that runs off the end of the line takes the rest of the line with
// it. That is the right answer for a truncated stream and the wrong one for an
// OSC that spans a newline, which nothing writing to a pipe does.
func skipEscape(b []byte, pen *sgrState) int {
	if len(b) < 2 {
		return len(b)
	}
	switch b[1] {
	case '[':
		// CSI: parameter and intermediate bytes, then one final byte.
		for i := 2; i < len(b); i++ {
			if b[i] >= 0x40 && b[i] <= 0x7e {
				if b[i] == 'm' {
					pen.applySGR(string(b[2:i]))
				}
				return i + 1
			}
		}
		return len(b)
	case ']', 'P', 'X', '^', '_':
		// OSC and friends: a string terminated by BEL or by ST.
		for i := 2; i < len(b); i++ {
			if b[i] == 0x07 {
				return i + 1
			}
			if b[i] == 0x1b && i+1 < len(b) && b[i+1] == '\\' {
				return i + 2
			}
		}
		return len(b)
	default:
		// An escape with intermediate bytes, such as the ESC ( B that selects
		// the ASCII character set. Two bytes is not the whole of it, and the
		// byte left behind prints as text.
		if b[1] >= 0x20 && b[1] <= 0x2f {
			for i := 2; i < len(b); i++ {
				if b[i] >= 0x30 && b[i] <= 0x7e {
					return i + 1
				}
				if b[i] < 0x20 || b[i] > 0x2f {
					return i
				}
			}
			return len(b)
		}
		return 2
	}
}

// wrapCells cuts a row into rows the canvas is wide, and pads the last one.
//
// Cropping instead would lose the right hand side of every wide line, and a
// piece of ANSI art is mostly right hand side. Wrapping is what the terminal
// would have done with the same text.
func wrapCells(row []tfx.InputCell, width int) [][]tfx.InputCell {
	if len(row) == 0 {
		return [][]tfx.InputCell{make([]tfx.InputCell, width)}
	}
	var out [][]tfx.InputCell
	for start := 0; start < len(row); start += width {
		end := min(start+width, len(row))
		line := make([]tfx.InputCell, width)
		copy(line, row[start:end])
		out = append(out, line)
	}
	return out
}

// trimBlankRows drops empty rows off the bottom of the canvas.
func trimBlankRows(rows [][]tfx.InputCell) [][]tfx.InputCell {
	for len(rows) > 0 && rowIsBlank(rows[len(rows)-1]) {
		rows = rows[:len(rows)-1]
	}
	return rows
}

func rowIsBlank(row []tfx.InputCell) bool {
	for _, c := range row {
		if c.Symbol != "" && c.Symbol != " " {
			return false
		}
		if c.HasBg {
			return false
		}
	}
	return true
}
