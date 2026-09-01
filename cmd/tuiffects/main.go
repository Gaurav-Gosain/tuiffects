// Command tuiffects animates text in your terminal.
//
// It takes an argument, a file or a pipe, turns it into a grid of coloured
// cells, and runs one of the library's effects over it. Colours the input
// carried are kept, so what dissolves and re-forms on the screen is your own
// output rather than a recolouring of it.
//
//	echo hello | tuiffects
//	ls --color=always | tuiffects
//	tuiffects "shipped"
//	tuiffects capture.ans
//
// The command lives under cmd so it can be installed on its own, and it adds
// no dependency to the module: everything it needs beyond the library is the
// standard library. Importing github.com/Gaurav-Gosain/tuiffects costs the
// same as it did before this existed.
package main

import (
	"bufio"
	"errors"
	"flag"
	"fmt"
	"io"
	"math/rand/v2"
	"os"
	"os/signal"
	"runtime/debug"
	"strings"
	"syscall"
	"time"

	tfx "github.com/Gaurav-Gosain/tuiffects"
)

// Exit codes.
const (
	exitOK       = 0
	exitError    = 1
	exitUsage    = 2
	exitSignal   = 130
	frameCeiling = 500000
)

// defaults.
const (
	defaultFPS     = 60
	defaultSeconds = 12.0
	fallbackWidth  = 80
	fallbackHeight = 24
	// maxCanvas caps a window size that came from the environment rather than
	// from the kernel.
	maxCanvas = 4096
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

// options is the whole flag surface.
type options struct {
	effect  string
	fps     int
	colors  string
	seed    uint64
	seconds float64
	list    bool
	version bool
}

func run(args []string, stdin io.Reader, stdout, stderr *os.File) int {
	opts, rest, code, done := parseArgs(args, stderr)
	if done {
		return code
	}

	if opts.list {
		listEffects(stdout)
		return exitOK
	}
	if opts.version {
		fmt.Fprintln(stdout, version())
		return exitOK
	}

	source, err := openInput(rest, stdin)
	if err != nil {
		fmt.Fprintln(stderr, "tuiffects: "+err.Error())
		return exitError
	}
	defer source.Close()
	if source.reader == nil {
		usage(stderr)
		return exitUsage
	}

	// Standard output is not a terminal, so there is nothing to animate on.
	// Copy the input straight through: `ls | tuiffects | less` then shows the
	// same thing `ls | less` would, and `ls | tuiffects > out` writes the same
	// bytes `ls > out` would. Refusing would break the pipeline instead.
	if !isTerminal(stdout) {
		if isTerminal(stderr) {
			fmt.Fprintln(stderr, "tuiffects: the output is not a terminal. The input was copied and not animated.")
		}
		if _, err := io.Copy(stdout, source.reader); err != nil {
			fmt.Fprintln(stderr, "tuiffects: "+err.Error())
			return exitError
		}
		return exitOK
	}

	return animate(opts, source.reader, stdout, stderr)
}

func animate(opts options, src io.Reader, stdout, stderr *os.File) int {
	width, height := canvasSize(stdout)
	g, tail, err := buildGrid(src, width, height)
	if err != nil {
		fmt.Fprintln(stderr, "tuiffects: "+err.Error())
		return exitError
	}
	if len(g.rows) == 0 {
		// Nothing to animate. Whatever is left is still the user's output.
		out := bufio.NewWriter(stdout)
		_, _ = io.Copy(out, tail)
		_ = out.Flush()
		return exitOK
	}

	descriptor, ok := pickEffect(opts.effect)
	if !ok {
		fmt.Fprintf(stderr, "tuiffects: there is no effect named %q. Run tuiffects --list.\n", opts.effect)
		return exitError
	}
	engine := newEngineOver(g, descriptor, opts)
	effect := descriptor.New()

	scr := newScreen(stdout, len(g.rows))
	if err := effect.Build(engine); err != nil {
		// The effect could not be set up over this screen. Print the input
		// rather than swallow it.
		fmt.Fprintf(stderr, "tuiffects: %s could not run here: %v\n", descriptor.Name, err)
		_ = scr.write(engine.Frame() + "\n")
		_ = scr.flush()
		_, _ = io.Copy(stdout, tail)
		return exitError
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	defer signal.Stop(stop)

	scr.open()
	interrupted := play(scr, effect, engine, opts, stop)
	scr.restore()
	if interrupted {
		return exitSignal
	}

	// The rows that did not fit follow the animation as ordinary output, in
	// the colour they were written in.
	if err := writeTail(scr, g.pen, tail); err != nil {
		fmt.Fprintln(stderr, "tuiffects: "+err.Error())
		return exitError
	}
	return exitOK
}

// play runs the animation and reports whether a signal cut it short.
//
// It paints on a wall clock deadline rather than a frame count. Past the
// deadline it stops painting and stops sleeping and runs the effect out at
// full speed, so the picture always arrives at the state the effect meant to
// leave it in. A capped run is a short animation, never a half-finished one.
func play(scr *screen, effect tfx.Effect, engine *tfx.Engine, opts options, stop <-chan os.Signal) bool {
	interval := time.Second / time.Duration(opts.fps)
	start := time.Now()
	budget := time.Duration(opts.seconds * float64(time.Second))
	_ = scr.paint(engine.Frame())

	drained := false
	for frame := 1; frame <= frameCeiling; frame++ {
		if !effect.Advance(engine) {
			break
		}
		if drained {
			select {
			case <-stop:
				return true
			default:
			}
			continue
		}
		if err := scr.paint(engine.Frame()); err != nil {
			return false
		}
		if budget > 0 && time.Since(start) >= budget {
			drained = true
			continue
		}
		select {
		case <-stop:
			return true
		case <-time.After(time.Until(start.Add(interval * time.Duration(frame+1)))):
		}
	}
	if drained {
		_ = scr.paint(engine.Frame())
	}
	return false
}

// newEngineOver builds the engine the effect runs on.
func newEngineOver(g grid, d tfx.Descriptor, opts options) *tfx.Engine {
	width := len(g.rows[0])
	height := len(g.rows)
	terminal := tfx.NewTerminalFromCells(g.rows, tfx.TerminalConfig{
		Width:              width,
		Height:             height,
		MakeFillCharacters: d.NeedsFillCharacters,
		// South west is the one anchor that moves nothing. Every other one
		// measures the block the input fills and slides it into a corner, so a
		// capture whose first rows are blank starts by jumping upwards. The
		// grid already carries the coordinate of every cell and this is the
		// anchor that keeps them.
		AnchorText:            tfx.AnchorSW,
		ExistingColorHandling: colorHandling(opts.colors, g.colored),
	})
	engine := tfx.NewEngine(terminal, tfx.NewRng(opts.seed))
	// The clock has to be the rate this program really paints at. NewEngine
	// installs one that steps a sixtieth of a second per frame, so an effect
	// written in seconds, matrix and thunderstorm and tuffbaby, runs at the
	// wrong speed on any other frame rate. At thirty frames a second and the
	// default clock, matrix rains for half as long as it should.
	engine.Clock = tfx.NewVirtualClock(opts.fps)
	return engine
}

// colorHandling picks the library's input-colour policy.
//
// auto is the default, and it is the difference between the two things people
// pipe in. Output that carried its own colours is a picture of a screen, and
// resolving every character back to the colour it arrived with reassembles
// that screen as itself. Plain text carried no colours at all, so dynamic
// would resolve it to nothing and the animation would run in the terminal's
// default foreground from the first frame to the last. There the effect's own
// gradient is the whole show.
func colorHandling(name string, inputHasColor bool) tfx.ExistingColorHandling {
	switch name {
	case "ignore":
		return tfx.IgnoreExistingColors
	case "always":
		return tfx.AlwaysExistingColors
	case "dynamic":
		return tfx.DynamicExistingColors
	default:
		if inputHasColor {
			return tfx.DynamicExistingColors
		}
		return tfx.IgnoreExistingColors
	}
}

// pickEffect resolves a name, or picks one at random.
func pickEffect(name string) (tfx.Descriptor, bool) {
	if name == "random" || name == "" {
		names := tfx.Names()
		if len(names) == 0 {
			return tfx.Descriptor{}, false
		}
		name = names[rand.IntN(len(names))]
	}
	return tfx.Lookup(name)
}

// writeTail prints the rows that did not fit on the canvas.
func writeTail(scr *screen, pen sgrState, tail io.Reader) error {
	br := bufio.NewReader(tail)
	if _, err := br.Peek(1); err != nil {
		// Nothing followed the canvas.
		return scr.flush()
	}
	// The animation reset the terminal's colours after every cell it drew, so
	// the tail would start in the default colour whatever the input said. Put
	// the pen back first, and put it away again at the end.
	if err := scr.write(pen.escape()); err != nil {
		return err
	}
	if _, err := io.Copy(scr, br); err != nil {
		return err
	}
	if err := scr.write("\x1b[0m"); err != nil {
		return err
	}
	return scr.flush()
}

// canvasSize is how much of the window the animation may use.
//
// The canvas is as wide as the window, because the effects that need room need
// it sideways, and as tall as the input, because a program in a pipeline has
// no business taking twenty rows of scrollback to animate one line. One row is
// held back so the shell prompt has somewhere to land without scrolling the
// top of the picture away.
func canvasSize(out *os.File) (width, height int) {
	width, height, ok := terminalSize(out)
	if !ok {
		width, height = envSize()
	}
	height--
	if width < 1 {
		width = fallbackWidth
	}
	if height < 1 {
		height = 1
	}
	// A window this size does not exist. The numbers can only come from
	// COLUMNS and LINES, which anyone can set to anything, and the canvas is
	// allocated from them.
	width = min(width, maxCanvas)
	height = min(height, maxCanvas)
	return width, height
}

func envSize() (int, int) {
	width, height := fallbackWidth, fallbackHeight
	if v := atoiOr(os.Getenv("COLUMNS"), 0); v > 0 {
		width = v
	}
	if v := atoiOr(os.Getenv("LINES"), 0); v > 0 {
		height = v
	}
	return width, height
}

func atoiOr(s string, fallback int) int {
	n := 0
	if s == "" {
		return fallback
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return fallback
		}
		n = n*10 + int(r-'0')
	}
	return n
}

func listEffects(out io.Writer) {
	w := bufio.NewWriter(out)
	defer w.Flush()
	for _, d := range tfx.Descriptors() {
		fmt.Fprintf(w, "%-16s %s\n", d.Name, d.Description)
	}
}

func version() string {
	info, ok := debug.ReadBuildInfo()
	if !ok || info.Main.Version == "" {
		return "tuiffects (unknown version)"
	}
	return "tuiffects " + info.Main.Version
}

func parseArgs(args []string, stderr io.Writer) (options, []string, int, bool) {
	var opts options
	fs := flag.NewFlagSet("tuiffects", flag.ContinueOnError)
	fs.SetOutput(stderr)
	fs.Usage = func() { usage(stderr) }

	stringVar(fs, &opts.effect, "random", "effect", "e")
	intVar(fs, &opts.fps, defaultFPS, "fps", "f")
	stringVar(fs, &opts.colors, "auto", "colors", "c")
	uint64Var(fs, &opts.seed, 0, "seed", "s")
	float64Var(fs, &opts.seconds, defaultSeconds, "seconds", "t")
	boolVar(fs, &opts.list, false, "list", "l")
	boolVar(fs, &opts.version, false, "version", "V")

	if err := fs.Parse(reorderArgs(args)); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return opts, nil, exitOK, true
		}
		return opts, nil, exitUsage, true
	}
	if opts.fps < 1 || opts.fps > 1000 {
		fmt.Fprintln(stderr, "tuiffects: the frame rate must be between 1 and 1000.")
		return opts, nil, exitUsage, true
	}
	if opts.seconds < 0 {
		fmt.Fprintln(stderr, "tuiffects: the time limit cannot be negative.")
		return opts, nil, exitUsage, true
	}
	switch opts.colors {
	case "auto", "dynamic", "ignore", "always":
	default:
		fmt.Fprintln(stderr, "tuiffects: --colors must be auto, dynamic, ignore or always.")
		return opts, nil, exitUsage, true
	}
	// The name is checked here rather than where the effect is built, because
	// the output may not be a terminal and that path never builds one. A typo
	// in --effect should be a refusal wherever the output goes.
	if opts.effect != "random" && opts.effect != "" {
		if _, ok := tfx.Lookup(opts.effect); !ok {
			fmt.Fprintf(stderr, "tuiffects: there is no effect named %q. Run tuiffects --list.\n", opts.effect)
			return opts, nil, exitUsage, true
		}
	}
	if opts.seed == 0 {
		opts.seed = rand.Uint64()
	}
	return opts, fs.Args(), exitOK, false
}

// valuelessFlags are the flags that take nothing after them. reorderArgs needs
// to know, because every other flag swallows the argument that follows it.
var valuelessFlags = map[string]bool{
	"list": true, "l": true,
	"version": true, "V": true,
	"help": true, "h": true,
}

// reorderArgs moves the flags in front of the text.
//
// The flag package stops reading flags at the first argument that is not one,
// so `tuiffects notes.txt -e matrix` reads the whole line as text and animates
// the words "notes.txt -e matrix". Nobody types it the other way round, so the
// flags are gathered up and the rest is left in the order it was given.
//
// A lone `--` ends it: everything after is text, however much it looks like a
// flag. That is how to animate a line that starts with a dash.
func reorderArgs(args []string) []string {
	var flags, operands []string
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--" {
			operands = append(operands, args[i+1:]...)
			break
		}
		if len(arg) < 2 || arg[0] != '-' {
			operands = append(operands, arg)
			continue
		}
		flags = append(flags, arg)
		name := strings.TrimLeft(arg, "-")
		if strings.ContainsRune(name, '=') || valuelessFlags[name] {
			continue
		}
		if i+1 < len(args) {
			i++
			flags = append(flags, args[i])
		}
	}
	// The marker goes back in between the two halves. Without it the flag
	// package would read a line of text that starts with a dash as a flag,
	// which is the thing `--` was typed to prevent.
	return append(append(flags, "--"), operands...)
}

// The flag package has no short and long pair, so each flag is registered
// under both names against the same variable.
func stringVar(fs *flag.FlagSet, p *string, def string, long, short string) {
	fs.StringVar(p, long, def, "")
	fs.StringVar(p, short, def, "")
}

func intVar(fs *flag.FlagSet, p *int, def int, long, short string) {
	fs.IntVar(p, long, def, "")
	fs.IntVar(p, short, def, "")
}

func uint64Var(fs *flag.FlagSet, p *uint64, def uint64, long, short string) {
	fs.Uint64Var(p, long, def, "")
	fs.Uint64Var(p, short, def, "")
}

func float64Var(fs *flag.FlagSet, p *float64, def float64, long, short string) {
	fs.Float64Var(p, long, def, "")
	fs.Float64Var(p, short, def, "")
}

func boolVar(fs *flag.FlagSet, p *bool, def bool, long, short string) {
	fs.BoolVar(p, long, def, "")
	fs.BoolVar(p, short, def, "")
}

const usageText = `tuiffects animates text in your terminal.

Usage:
  tuiffects [flags] [text]
  tuiffects [flags] FILE
  command | tuiffects [flags]

Input:
  An argument wins over a pipe, because an argument is what you typed.
  One argument that names a file is read as a file. Anything else is text.
  With no argument, tuiffects reads standard input.
  Flags can come before or after the text. Put -- first to animate a line
  that starts with a dash.

Flags:
  -e, --effect NAME   Which effect to run, or random. Default random.
  -f, --fps N         Frames per second. Default 60.
  -c, --colors MODE   auto, dynamic, ignore or always. Default auto.
  -s, --seed N        Seed for the random numbers. Default random.
  -t, --seconds S     Stop the animation after S seconds and jump to the end.
                      Default 12. Use 0 for no limit.
  -l, --list          List the effects and exit.
  -V, --version       Print the version and exit.
  -h, --help          Print this help.

Colours:
  auto     dynamic if the input carried colours, ignore if it did not.
  dynamic  Each character resolves back to the colour it came in with.
  ignore   The effect uses its own colours.
  always   Every frame keeps the input colour, so only the shape moves.

Notes:
  If the output is not a terminal, tuiffects copies the input and animates
  nothing. Pipe it to less or to a file and you get your text back.
  The canvas is as wide as the window and as tall as the input. Lines that
  are too long wrap. Lines below the window print after the animation.
`

func usage(out io.Writer) { fmt.Fprint(out, usageText) }

// inputSource is where the text came from.
type inputSource struct {
	reader io.Reader
	file   *os.File
}

func (s *inputSource) Close() {
	if s.file != nil {
		_ = s.file.Close()
	}
}

// openInput decides what to animate.
//
// An argument beats standard input. A pipe can be inherited by accident, a
// shell function or a wrapper script can leave one attached, and an argument
// is the one thing on the command line the user certainly typed. So when both
// are there, the argument is the instruction and the pipe is the accident.
//
// A single argument that names a file is read as that file, because
// `tuiffects notes.txt` means the file every time anyone types it.
func openInput(args []string, stdin io.Reader) (*inputSource, error) {
	if len(args) == 1 {
		if info, err := os.Stat(args[0]); err == nil && info.Mode().IsRegular() {
			f, err := os.Open(args[0])
			if err != nil {
				return nil, err
			}
			return &inputSource{reader: f, file: f}, nil
		}
	}
	if len(args) > 0 {
		return &inputSource{reader: strings.NewReader(strings.Join(args, " ") + "\n")}, nil
	}
	if f, ok := stdin.(*os.File); ok && isTerminal(f) {
		// Standard input is the keyboard and no argument was given. There is
		// nothing to animate and waiting for the user to type would look like
		// a hang.
		return &inputSource{}, nil
	}
	return &inputSource{reader: stdin}, nil
}
