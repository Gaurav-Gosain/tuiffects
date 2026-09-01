package main

import (
	"bufio"
	"io"
	"strconv"
	"strings"
)

// The painter.
//
// The animation is drawn in place, in the flow of the shell, not on the
// alternate screen. `ls | tuiffects` is meant to leave you looking at your ls
// output, and the alternate screen takes the whole thing away again the moment
// it ends. So the program opens a block of blank lines, repaints it, and
// leaves the finished picture on the screen where the command's own output
// would have been.
//
// Everything it changes about the terminal, it changes back. There is one exit
// path, restore, and the signal handler and the end of the animation both go
// through it.

const (
	hideCursor = "\x1b[?25l"
	showCursor = "\x1b[?25h"
	clearToEOL = "\x1b[K"
)

// screen owns the block of lines the animation is drawn in.
type screen struct {
	out    *bufio.Writer
	height int
	// opened is true once the blank lines are on the screen and the cursor is
	// back at the top of them. It is what makes restore safe to call twice and
	// safe to call before anything was drawn.
	opened bool
}

func newScreen(w io.Writer, height int) *screen {
	return &screen{out: bufio.NewWriterSize(w, 64<<10), height: height}
}

// open reserves height lines under the cursor and parks the cursor on the
// first of them.
//
// The lines are printed rather than jumped to. Printing is what scrolls the
// terminal, so a block opened at the bottom of the window pushes the earlier
// output up exactly as any command's output would; moving the cursor down
// would run it off the bottom edge and draw over whatever is there.
func (s *screen) open() {
	if s.opened || s.height < 1 {
		return
	}
	s.out.WriteString(hideCursor)
	s.out.WriteString(strings.Repeat("\n", s.height))
	s.cursorUp(s.height)
	s.opened = true
	s.out.Flush()
}

// paint draws one frame. The frame is the engine's, rows separated by
// newlines, and it is exactly height rows of exactly the canvas width.
func (s *screen) paint(frame string) error {
	if !s.opened {
		return nil
	}
	rows := strings.Split(frame, "\n")
	for i, row := range rows {
		if i > 0 {
			s.out.WriteByte('\n')
		}
		s.out.WriteByte('\r')
		s.out.WriteString(row)
		// The row is written full width every frame, so nothing is left over
		// from the frame before it. The clear is for the row that is short
		// because the terminal was resized under us.
		s.out.WriteString(clearToEOL)
	}
	s.cursorUp(len(rows) - 1)
	return s.out.Flush()
}

// restore puts the cursor under the block and brings it back. It is safe to
// call more than once and safe to call on a block that was never opened.
func (s *screen) restore() {
	if s.opened {
		s.cursorDown(s.height - 1)
		s.out.WriteString("\r\n")
		s.opened = false
	}
	s.out.WriteString(showCursor)
	s.out.Flush()
}

// write puts text on the screen under the block, for the rows that did not fit
// on the canvas.
func (s *screen) write(text string) error {
	_, err := s.out.WriteString(text)
	return err
}

func (s *screen) flush() error { return s.out.Flush() }

func (s *screen) cursorUp(n int) {
	if n > 0 {
		s.out.WriteString("\x1b[" + strconv.Itoa(n) + "A")
	}
}

func (s *screen) cursorDown(n int) {
	if n > 0 {
		s.out.WriteString("\x1b[" + strconv.Itoa(n) + "B")
	}
}

// Write makes the screen an io.Writer, for the rows that did not fit.
func (s *screen) Write(p []byte) (int, error) { return s.out.Write(p) }
