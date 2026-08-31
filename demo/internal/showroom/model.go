// Package showroom is the Bubble Tea program behind the tuiffects demo. It
// plays one effect at a time on a canvas the size of the terminal, loops it,
// and takes keys and a SelectMsg to change which one.
package showroom

import (
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/Gaurav-Gosain/tuiffects"
)

// Options configure a showroom.
type Options struct {
	// Effect is the name to start on. Unknown or empty means the first
	// effect alphabetically.
	Effect string
	// FPS is the tick rate, and the rate the engine's virtual clock runs
	// at, so a change of rate changes speed on the screen and nothing else.
	// At or below zero means sixty.
	FPS int
	// Seed pins the random source so every replay is the same run. Zero
	// takes a fresh seed from the clock for every run.
	Seed uint64
	// QuitKeys makes q, esc and ctrl+c quit. Off in a browser, where a quit
	// would leave a dead terminal on the page.
	QuitKeys bool
}

// TickMsg is one frame's worth of time.
type TickMsg time.Time

// SelectMsg asks for an effect by name. An unknown name is ignored.
type SelectMsg struct{ Name string }

type mode int

const (
	modePlaying mode = iota
	modeHolding
	modePaused
)

// holdSeconds is how long a finished effect's last frame stays up before
// the replay.
const holdSeconds = 2

// Model is the showroom. It is a value, as Bubble Tea models are.
type Model struct {
	opts  Options
	names []string
	index int

	width, height int

	engine *tuiffects.Engine
	effect tuiffects.Effect
	frame  string
	err    error

	mode    mode
	hold    int
	ticking bool
}

// New builds a showroom that does not yet know its size. Nothing is built
// until the first tea.WindowSizeMsg.
func New(opts Options) Model {
	if opts.FPS <= 0 {
		opts.FPS = 60
	}
	m := Model{opts: opts, names: tuiffects.Names()}
	if i := m.indexOf(opts.Effect); i >= 0 {
		m.index = i
	}
	return m
}

// Current is the name of the effect on screen.
func (m Model) Current() string { return m.names[m.index] }

func (m Model) indexOf(name string) int {
	for i, n := range m.names {
		if n == name {
			return i
		}
	}
	return -1
}

// Init does nothing: the tick chain starts on the first WindowSizeMsg, which
// both a terminal and booba's browser bridge send on their own.
func (m Model) Init() tea.Cmd { return nil }

func (m Model) tick() tea.Cmd {
	return tea.Tick(time.Second/time.Duration(m.opts.FPS), func(t time.Time) tea.Msg {
		return TickMsg(t)
	})
}

// Update is the message loop: sizing, ticks, selection, keys.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width, m.height = msg.Width, msg.Height
		// A resize rebuilds at the new size; it must not undo a pause.
		paused := m.mode == modePaused && m.err == nil
		m.rebuild()
		if paused && m.err == nil {
			m.mode = modePaused
		}
		if m.ticking {
			return m, nil
		}
		m.ticking = true
		return m, m.tick()
	case TickMsg:
		m.step()
		return m, m.tick()
	case SelectMsg:
		if i := m.indexOf(msg.Name); i >= 0 {
			m.index = i
			m.rebuild()
		}
		return m, nil
	case tea.KeyPressMsg:
		return m.key(msg)
	}
	return m, nil
}

// step is one frame: advance while playing, count down while holding, and
// rebuild when the hold runs out. Paused does nothing but keep the chain up.
func (m *Model) step() {
	switch m.mode {
	case modePlaying:
		if m.engine == nil || m.err != nil {
			return
		}
		if !m.effect.Advance(m.engine) {
			m.mode = modeHolding
			m.hold = holdSeconds * m.opts.FPS
		}
		m.frame = m.engine.Frame()
	case modeHolding:
		m.hold--
		if m.hold <= 0 {
			m.rebuild()
		}
	case modePaused:
	}
}

func (m Model) key(msg tea.KeyPressMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "right", "l", "]":
		m.move(1)
	case "left", "h", "[":
		m.move(-1)
	case "r", "enter":
		m.rebuild()
	case "space":
		switch m.mode {
		case modePlaying:
			m.mode = modePaused
		case modePaused:
			if m.err == nil {
				m.mode = modePlaying
			}
		case modeHolding:
			m.rebuild()
		}
	case "q", "esc", "ctrl+c":
		if m.opts.QuitKeys {
			return m, tea.Quit
		}
	}
	return m, nil
}

// move steps through the catalogue, wrapping at both ends.
func (m *Model) move(delta int) {
	n := len(m.names)
	m.index = ((m.index+delta)%n + n) % n
	m.rebuild()
}

func (m *Model) seed() uint64 {
	if m.opts.Seed != 0 {
		return m.opts.Seed
	}
	return uint64(time.Now().UnixNano())
}

// rebuild starts the current effect over on a canvas the size of the
// terminal less the status line. It is a no-op before the first size.
func (m *Model) rebuild() {
	if m.width < 1 || m.height < 1 {
		return
	}
	desc, _ := tuiffects.Lookup(m.Current())
	canvasHeight := m.height - 1
	if canvasHeight < 1 {
		canvasHeight = 1
	}
	terminal := tuiffects.NewTerminalFromText(Banner(m.width), tuiffects.TerminalConfig{
		Width:              m.width,
		Height:             canvasHeight,
		AnchorText:         tuiffects.AnchorC,
		MakeFillCharacters: desc.NeedsFillCharacters,
	})
	engine := tuiffects.NewEngine(terminal, tuiffects.NewRng(m.seed()))
	engine.Clock = tuiffects.NewVirtualClock(m.opts.FPS)
	effect := desc.New()
	m.engine, m.effect = engine, effect
	m.err = effect.Build(engine)
	m.frame = engine.Frame()
	m.hold = 0
	if m.err != nil {
		m.mode = modePaused
		return
	}
	m.mode = modePlaying
}

var (
	nameStyle        = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("212"))
	descriptionStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("250"))
	hintStyle        = lipgloss.NewStyle().Foreground(lipgloss.Color("241"))
	errorStyle       = lipgloss.NewStyle().Foreground(lipgloss.Color("203"))
)

const hints = "←/→ effect · r replay · space pause"

// View is the frame over a one-line status bar. The window title is the
// effect's name, which is how the demo page learns what is playing.
func (m Model) View() tea.View {
	v := tea.NewView(m.frame + "\n" + m.statusLine())
	v.AltScreen = true
	v.WindowTitle = m.Current()
	return v
}

func (m Model) statusLine() string {
	desc, _ := tuiffects.Lookup(m.Current())
	parts := []string{nameStyle.Render(desc.Name)}
	switch {
	case m.err != nil:
		parts = append(parts, errorStyle.Render(m.err.Error()))
	case m.mode == modePaused:
		parts = append(parts, hintStyle.Render("paused"))
	}
	parts = append(parts,
		hintStyle.Render(m.hintText()),
		descriptionStyle.Render(desc.Description))
	line := strings.Join(parts, hintStyle.Render(" · "))
	return lipgloss.NewStyle().MaxWidth(m.width).Render(line)
}

func (m Model) hintText() string {
	if m.opts.QuitKeys {
		return hints + " · q quit"
	}
	return hints
}
