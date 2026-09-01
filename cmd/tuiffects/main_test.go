package main

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	tfx "github.com/Gaurav-Gosain/tuiffects"
)

// runToFile runs the command with standard output on a file, which is never a
// terminal, and returns what was written.
func runToFile(t *testing.T, args []string, stdin io.Reader) (int, string, string) {
	t.Helper()
	dir := t.TempDir()
	out, err := os.Create(filepath.Join(dir, "out"))
	if err != nil {
		t.Fatal(err)
	}
	defer out.Close()
	errFile, err := os.Create(filepath.Join(dir, "err"))
	if err != nil {
		t.Fatal(err)
	}
	defer errFile.Close()
	code := run(args, stdin, out, errFile)
	outBytes, _ := os.ReadFile(filepath.Join(dir, "out"))
	errBytes, _ := os.ReadFile(filepath.Join(dir, "err"))
	return code, string(outBytes), string(errBytes)
}

// TestOutputThatIsNotATerminalGetsTheInputBack is the rule that keeps this
// usable in a pipeline. `ls | tuiffects | less` has nowhere to animate, and a
// screenful of cursor movement in a pager, or in a file, is worse than no
// program at all.
func TestOutputThatIsNotATerminalGetsTheInputBack(t *testing.T) {
	in := "\x1b[31mred\x1b[0m\nplain\n"
	code, out, _ := runToFile(t, nil, strings.NewReader(in))
	if code != exitOK {
		t.Errorf("the exit code is %d, want %d", code, exitOK)
	}
	if out != in {
		t.Errorf("the output is %q, want the input back byte for byte", out)
	}
	if strings.Contains(out, "\x1b[?25l") {
		t.Error("the output carries the hide-cursor escape, so it animated")
	}
}

func TestThePassThroughIsSilentWhenNobodyCanReadTheNote(t *testing.T) {
	// The note goes to standard error only when standard error is a terminal.
	// A file or a pipe there is somebody capturing the output.
	_, _, errText := runToFile(t, nil, strings.NewReader("hello\n"))
	if errText != "" {
		t.Errorf("standard error carries %q, want nothing", errText)
	}
}

func TestAnArgumentBeatsAPipe(t *testing.T) {
	_, out, _ := runToFile(t, []string{"typed"}, strings.NewReader("piped\n"))
	if strings.TrimSpace(out) != "typed" {
		t.Errorf("the output is %q, want the argument", out)
	}
}

func TestOneArgumentThatNamesAFileIsReadAsThatFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "note.txt")
	if err := os.WriteFile(path, []byte("from the file\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, out, _ := runToFile(t, []string{path}, strings.NewReader("piped\n"))
	if out != "from the file\n" {
		t.Errorf("the output is %q, want the file", out)
	}
}

func TestSeveralArgumentsBecomeOneLineOfText(t *testing.T) {
	_, out, _ := runToFile(t, []string{"hello", "there"}, strings.NewReader(""))
	if out != "hello there\n" {
		t.Errorf("the output is %q, want the arguments joined", out)
	}
}

func TestAnUnknownEffectIsRefusedWhereverTheOutputGoes(t *testing.T) {
	// The output here is a file, so nothing is ever animated. A typo in
	// --effect still has to be a refusal, or `ls | tuiffects -e nope | cat`
	// says nothing and exits zero.
	code, out, errText := runToFile(t, []string{"-e", "nope", "hello"}, strings.NewReader(""))
	if code != exitUsage {
		t.Errorf("an unknown effect exited %d, want %d", code, exitUsage)
	}
	if out != "" {
		t.Errorf("an unknown effect still wrote %q", out)
	}
	if !strings.Contains(errText, "there is no effect named") {
		t.Errorf("the refusal reads %q", errText)
	}
}

func TestPickEffect(t *testing.T) {
	if _, ok := pickEffect("nosucheffect"); ok {
		t.Fatal("an effect that does not exist was found")
	}
	if _, ok := pickEffect("matrix"); !ok {
		t.Fatal("matrix was not found")
	}
	for range 200 {
		if _, ok := pickEffect("random"); !ok {
			t.Fatal("random did not resolve to an effect")
		}
	}
}

func TestBadFlagsAreRefusedWithTheUsageCode(t *testing.T) {
	cases := [][]string{
		{"--fps", "0"},
		{"--fps", "5000"},
		{"--seconds", "-1"},
		{"--colors", "purple"},
		{"--effect", "nosucheffect"},
	}
	for _, args := range cases {
		code, _, _ := runToFile(t, args, strings.NewReader("x\n"))
		if code != exitUsage {
			t.Errorf("%v exited %d, want %d", args, code, exitUsage)
		}
	}
}

func TestHelpExitsCleanly(t *testing.T) {
	code, _, errText := runToFile(t, []string{"--help"}, strings.NewReader(""))
	if code != exitOK {
		t.Errorf("--help exited %d, want %d", code, exitOK)
	}
	if !strings.Contains(errText, "tuiffects animates text") {
		t.Errorf("--help printed %q", errText)
	}
}

func TestListNamesEveryEffect(t *testing.T) {
	_, out, _ := runToFile(t, []string{"--list"}, strings.NewReader(""))
	for _, name := range tfx.Names() {
		if !strings.Contains(out, name) {
			t.Errorf("--list does not name %s", name)
		}
	}
}

// TestColorAutoFollowsTheInput is the difference between the two things people
// pipe in.
func TestColorAutoFollowsTheInput(t *testing.T) {
	if got := colorHandling("auto", true); got != tfx.DynamicExistingColors {
		t.Errorf("coloured input under auto got policy %v, want dynamic", got)
	}
	if got := colorHandling("auto", false); got != tfx.IgnoreExistingColors {
		t.Errorf("plain input under auto got policy %v, want ignore", got)
	}
	if got := colorHandling("dynamic", false); got != tfx.DynamicExistingColors {
		t.Errorf("an explicit dynamic was overridden to %v", got)
	}
}

// TestTheScreenIsPutBackTheWayItWasFound covers the ordinary end of a run.
func TestTheScreenIsPutBackTheWayItWasFound(t *testing.T) {
	var buf bytes.Buffer
	scr := newScreen(&buf, 3)
	scr.open()
	if err := scr.paint("abc\ndef\nghi"); err != nil {
		t.Fatal(err)
	}
	scr.restore()
	out := buf.String()
	if !strings.HasPrefix(out, hideCursor) {
		t.Error("the run did not hide the cursor")
	}
	if !strings.HasSuffix(out, showCursor) {
		t.Errorf("the run did not show the cursor again; it ends %q", tailOf(out, 12))
	}
	if strings.Count(out, showCursor) != 1 {
		t.Errorf("the cursor was shown %d times, want once", strings.Count(out, showCursor))
	}
	// Three blank lines open the block, then the cursor walks back to the top
	// of them, and at the end it walks back down to the bottom.
	if !strings.Contains(out, "\n\n\n\x1b[3A") {
		t.Error("the block was not opened by printing its lines and going back up")
	}
	if !strings.Contains(out, "\x1b[2B\r\n") {
		t.Error("the cursor was not left under the block")
	}
}

func TestRestoreIsSafeTwiceAndSafeOnABlockNeverOpened(t *testing.T) {
	var buf bytes.Buffer
	scr := newScreen(&buf, 3)
	scr.restore()
	scr.restore()
	if strings.Contains(buf.String(), "\x1b[2B") {
		t.Error("a block that was never opened moved the cursor")
	}
	buf.Reset()
	scr = newScreen(&buf, 2)
	scr.open()
	scr.restore()
	scr.restore()
	if n := strings.Count(buf.String(), "\x1b[1B"); n != 1 {
		t.Errorf("the cursor walked down %d times, want once", n)
	}
}

// TestASignalStopsTheRunAndTheScreenIsStillPutBack is the Ctrl-C path. A CLI
// that exits into a terminal with no cursor is worse than no CLI.
func TestASignalStopsTheRunAndTheScreenIsStillPutBack(t *testing.T) {
	g, _, err := buildGrid(strings.NewReader(strings.Repeat("hello there\n", 6)), 30, 6)
	if err != nil {
		t.Fatal(err)
	}
	d, _ := tfx.Lookup("matrix")
	engine := newEngineOver(g, d, options{fps: 60, seed: 3, colors: "auto"})
	effect := d.New()
	if err := effect.Build(engine); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	scr := newScreen(&buf, len(g.rows))
	scr.open()
	stop := make(chan os.Signal, 1)
	go func() {
		time.Sleep(30 * time.Millisecond)
		stop <- os.Interrupt
	}()
	start := time.Now()
	// No time limit, so only the signal can end this: matrix over six rows
	// runs for the better part of half a minute.
	interrupted := play(scr, effect, engine, options{fps: 60, seconds: 0}, stop)
	elapsed := time.Since(start)
	scr.restore()
	if !interrupted {
		t.Fatal("the signal did not stop the run")
	}
	if elapsed > 3*time.Second {
		t.Errorf("the signal took %v to stop the run", elapsed)
	}
	if !strings.HasSuffix(buf.String(), showCursor) {
		t.Error("the interrupted run did not show the cursor again")
	}
}

// TestTheTimeLimitEndsOnTheFinishedPicture is what stops swarm holding the
// terminal for the better part of a minute.
func TestTheTimeLimitEndsOnTheFinishedPicture(t *testing.T) {
	text := strings.Repeat("the quick brown fox jumps\n", 8)
	g, _, err := buildGrid(strings.NewReader(text), 40, 8)
	if err != nil {
		t.Fatal(err)
	}
	d, _ := tfx.Lookup("swarm")
	opts := options{fps: 60, seed: 5, colors: "auto", seconds: 0.2}
	engine := newEngineOver(g, d, opts)
	effect := d.New()
	if err := effect.Build(engine); err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	scr := newScreen(&buf, len(g.rows))
	scr.open()
	start := time.Now()
	if play(scr, effect, engine, opts, make(chan os.Signal)) {
		t.Fatal("the run reported a signal it never got")
	}
	elapsed := time.Since(start)
	// Read the frames before the restore, which is not one.
	painted := buf.String()
	scr.restore()
	if elapsed > 5*time.Second {
		t.Fatalf("a 0.2 second limit took %v", elapsed)
	}
	// The limit cuts the animation short and never the picture. The last frame
	// has to be the one the effect meant to finish on.
	last := plainText(lastFrameOf(painted))
	if !strings.Contains(last, "the quick brown fox jumps") {
		t.Errorf("the run ended on an unfinished picture:\n%s", last)
	}
}

// lastFrameOf pulls the final painted frame out of a capture. Each frame ends
// where the painter walks the cursor back to the top of the block.
func lastFrameOf(out string) string {
	parts := cursorUpPattern.Split(out, -1)
	for i := len(parts) - 1; i >= 0; i-- {
		frame := strings.ReplaceAll(parts[i], "\x1b[K", "")
		frame = strings.ReplaceAll(frame, "\r", "")
		if strings.TrimSpace(frame) != "" {
			return frame
		}
	}
	return ""
}

var cursorUpPattern = regexp.MustCompile(`\x1b\[\d+A`)

// plainText drops the colour, so a test can read what is on the screen.
func plainText(s string) string { return sgrPattern.ReplaceAllString(s, "") }

var sgrPattern = regexp.MustCompile(`\x1b\[[0-9;]*m`)

func tailOf(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[len(s)-n:]
}

// TestTheCanvasKeepsTheRowEveryCellCameInOn is why the anchor is south west.
//
// Every other anchor measures the block the input fills and slides it to a
// corner, so input whose first rows are blank is lifted and the animation opens
// by jolting the picture upwards. The grid already carries the row of every
// cell and south west is the anchor that keeps them.
func TestTheCanvasKeepsTheRowEveryCellCameInOn(t *testing.T) {
	g, _, err := buildGrid(strings.NewReader("\n\nhi\n"), 10, 6)
	if err != nil {
		t.Fatal(err)
	}
	if len(g.rows) != 3 {
		t.Fatalf("the canvas is %d rows, want 3", len(g.rows))
	}
	// highlight never moves a character, so the first frame is the input.
	d, _ := tfx.Lookup("highlight")
	engine := newEngineOver(g, d, options{fps: 60, seed: 1, colors: "auto"})
	effect := d.New()
	if err := effect.Build(engine); err != nil {
		t.Fatal(err)
	}
	rows := strings.Split(plainText(engine.Frame()), "\n")
	if len(rows) != 3 {
		t.Fatalf("the frame is %d rows, want 3", len(rows))
	}
	if strings.TrimSpace(rows[0]) != "" || strings.TrimSpace(rows[1]) != "" {
		t.Errorf("the two blank rows did not stay blank: %q and %q", rows[0], rows[1])
	}
	if strings.TrimSpace(rows[2]) != "hi" {
		t.Errorf("the third row is %q, want hi. The text was moved off the row it came in on.", rows[2])
	}
}

func TestTheTailIsPrintedInTheColourItWasWrittenIn(t *testing.T) {
	var buf bytes.Buffer
	scr := newScreen(&buf, 1)
	pen := newSGRState()
	pen.applySGR("32")
	if err := writeTail(scr, pen, strings.NewReader("rest of it\n")); err != nil {
		t.Fatal(err)
	}
	out := buf.String()
	if !strings.HasPrefix(out, pen.escape()) {
		t.Errorf("the tail does not open in the pen it was cut in: %q", out)
	}
	if !strings.Contains(out, "rest of it\n") {
		t.Errorf("the tail is missing: %q", out)
	}
	if !strings.HasSuffix(out, "\x1b[0m") {
		t.Errorf("the tail does not put the colour away: %q", out)
	}
}

func TestAnEmptyTailWritesNothing(t *testing.T) {
	var buf bytes.Buffer
	scr := newScreen(&buf, 1)
	pen := newSGRState()
	pen.applySGR("32")
	if err := writeTail(scr, pen, strings.NewReader("")); err != nil {
		t.Fatal(err)
	}
	if buf.String() != "" {
		t.Errorf("an empty tail wrote %q", buf.String())
	}
}

func TestFlagsCanComeAfterTheText(t *testing.T) {
	cases := []struct {
		args []string
		text string
		fps  int
	}{
		{[]string{"hello", "-f", "30"}, "hello", 30},
		{[]string{"-f", "30", "hello"}, "hello", 30},
		{[]string{"one", "-f", "30", "two"}, "one two", 30},
		{[]string{"notes.txt", "--fps=25"}, "notes.txt", 25},
		{[]string{"-l", "hello"}, "hello", defaultFPS},
		{[]string{"--", "-e", "matrix"}, "-e matrix", defaultFPS},
	}
	for _, c := range cases {
		opts, rest, _, _ := parseArgs(c.args, io.Discard)
		if got := strings.Join(rest, " "); got != c.text {
			t.Errorf("%v left the text %q, want %q", c.args, got, c.text)
		}
		if opts.fps != c.fps {
			t.Errorf("%v gave %d frames a second, want %d", c.args, opts.fps, c.fps)
		}
	}
}

// TestDevNullIsNotATerminal is the difference between a file's mode and the
// kernel's answer. /dev/null is a character device, so a mode check calls it a
// terminal and `tuiffects > /dev/null` animates for twelve seconds into
// nothing instead of copying its input and stopping.
func TestDevNullIsNotATerminal(t *testing.T) {
	f, err := os.OpenFile(os.DevNull, os.O_WRONLY, 0)
	if err != nil {
		t.Skip("no /dev/null here")
	}
	defer f.Close()
	if isTerminal(f) {
		t.Error("/dev/null was read as a terminal")
	}
	if _, _, ok := terminalSize(f); ok {
		t.Error("/dev/null reported a window size")
	}
}

func TestAPipeAndAFileAreNotTerminals(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	defer w.Close()
	if isTerminal(w) {
		t.Error("a pipe was read as a terminal")
	}
	f, err := os.Create(filepath.Join(t.TempDir(), "f"))
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	if isTerminal(f) {
		t.Error("a regular file was read as a terminal")
	}
}

func TestAWindowSizeFromTheEnvironmentIsCapped(t *testing.T) {
	t.Setenv("COLUMNS", "99999999")
	t.Setenv("LINES", "99999999")
	w, h := envSize()
	if w != 99999999 || h != 99999999 {
		t.Fatalf("envSize read %d by %d, want the numbers it was given", w, h)
	}
	// canvasSize is what the canvas is allocated from, and it is the one that
	// has to refuse them.
	f, err := os.Open(os.DevNull)
	if err != nil {
		t.Skip("no /dev/null here")
	}
	defer f.Close()
	w, h = canvasSize(f)
	if w > maxCanvas || h > maxCanvas {
		t.Errorf("canvasSize gave %d by %d, want no more than %d either way", w, h, maxCanvas)
	}
}
