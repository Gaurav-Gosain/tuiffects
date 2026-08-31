package showroom

import (
	"strings"
	"testing"

	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"

	"github.com/Gaurav-Gosain/tuiffects"
)

// sized sends a WindowSizeMsg and returns the model and the command it gave back.
func sized(t *testing.T, m Model, width, height int) (Model, tea.Cmd) {
	t.Helper()
	next, cmd := m.Update(tea.WindowSizeMsg{Width: width, Height: height})
	return next.(Model), cmd
}

func TestStartsOnTheNamedEffect(t *testing.T) {
	if got := New(Options{Effect: "rain"}).Current(); got != "rain" {
		t.Errorf("Current() = %q, want rain", got)
	}
	first := tuiffects.Names()[0]
	if got := New(Options{Effect: "no-such-effect"}).Current(); got != first {
		t.Errorf("unknown name: Current() = %q, want %q", got, first)
	}
	if got := New(Options{}).Current(); got != first {
		t.Errorf("no name: Current() = %q, want %q", got, first)
	}
}

func TestSizingBuildsACanvasOfRowsMinusOne(t *testing.T) {
	m, cmd := sized(t, New(Options{Effect: "rain", Seed: 1}), 40, 12)
	if cmd == nil {
		t.Fatal("the first size should start the tick chain")
	}
	if m.err != nil {
		t.Fatalf("build: %v", m.err)
	}
	v := m.View()
	if v.WindowTitle != "rain" {
		t.Errorf("WindowTitle = %q, want rain", v.WindowTitle)
	}
	if !v.AltScreen {
		t.Error("the view should use the alternate screen")
	}
	lines := strings.Split(v.Content, "\n")
	if len(lines) != 12 {
		t.Fatalf("view has %d lines, want 12 (11 canvas + status)", len(lines))
	}
	for i, line := range lines[:11] {
		if w := lipgloss.Width(line); w != 40 {
			t.Errorf("canvas line %d is %d cells wide, want 40", i, w)
		}
	}
	if w := lipgloss.Width(lines[11]); w > 40 {
		t.Errorf("status line is %d cells wide, wider than the terminal", w)
	}
	if !strings.Contains(lines[11], "rain") {
		t.Errorf("status line should name the effect: %q", lines[11])
	}
}

func TestResizeRebuildsAndDoesNotStartASecondTickChain(t *testing.T) {
	m, _ := sized(t, New(Options{Effect: "rain", Seed: 1}), 40, 12)
	first := m.engine
	m, cmd := sized(t, m, 50, 14)
	if cmd != nil {
		t.Fatal("a second size started another tick chain")
	}
	if m.engine == first {
		t.Fatal("resize did not rebuild the effect")
	}
	if got := len(strings.Split(m.View().Content, "\n")); got != 14 {
		t.Errorf("view has %d lines after resize, want 14", got)
	}
}

func TestViewBeforeSizingIsHarmless(t *testing.T) {
	v := New(Options{Effect: "rain"}).View()
	if v.WindowTitle != "rain" {
		t.Errorf("WindowTitle = %q before sizing, want rain", v.WindowTitle)
	}
}

func ticked(t *testing.T, m Model, n int) Model {
	t.Helper()
	for i := 0; i < n; i++ {
		next, cmd := m.Update(TickMsg{})
		if cmd == nil {
			t.Fatal("a tick must schedule the next tick")
		}
		m = next.(Model)
	}
	return m
}

func pressed(t *testing.T, m Model, key tea.KeyPressMsg) (Model, tea.Cmd) {
	t.Helper()
	next, cmd := m.Update(key)
	return next.(Model), cmd
}

func TestTicksAdvanceTheEffect(t *testing.T) {
	m, _ := sized(t, New(Options{Effect: "randomsequence", Seed: 1}), 40, 12)
	before := m.frame
	if m = ticked(t, m, 60); m.frame == before {
		t.Fatal("sixty ticks changed nothing on screen")
	}
}

func TestFinishedEffectHoldsThenReplays(t *testing.T) {
	m, _ := sized(t, New(Options{Effect: "wipe", Seed: 1, FPS: 10}), 20, 5)
	first := m.engine
	var n int
	for n = 0; n < 5000 && m.mode != modeHolding; n++ {
		m = ticked(t, m, 1)
	}
	if m.mode != modeHolding {
		t.Fatalf("wipe on a 20x5 canvas did not finish within %d ticks", n)
	}
	if m.engine != first {
		t.Fatal("holding should keep the finished engine on screen")
	}
	m = ticked(t, m, holdSeconds*10-1)
	if m.mode != modeHolding {
		t.Fatal("the hold ended a tick early")
	}
	m = ticked(t, m, 1)
	if m.mode != modePlaying {
		t.Fatalf("after the hold the mode is %v, want playing", m.mode)
	}
	if m.engine == first {
		t.Fatal("the replay did not rebuild the effect")
	}
}

func TestArrowsStepThroughTheCatalogueAndWrap(t *testing.T) {
	names := tuiffects.Names()
	last := names[len(names)-1]
	m, _ := sized(t, New(Options{Effect: names[0], Seed: 1}), 40, 12)

	m, _ = pressed(t, m, tea.KeyPressMsg{Code: tea.KeyLeft})
	if m.Current() != last {
		t.Errorf("left from the first effect gave %q, want %q", m.Current(), last)
	}
	m, _ = pressed(t, m, tea.KeyPressMsg{Code: tea.KeyRight})
	if m.Current() != names[0] {
		t.Errorf("right from the last effect gave %q, want %q", m.Current(), names[0])
	}
	m, _ = pressed(t, m, tea.KeyPressMsg{Code: ']', Text: "]"})
	if m.Current() != names[1] {
		t.Errorf("] gave %q, want %q", m.Current(), names[1])
	}
	m, _ = pressed(t, m, tea.KeyPressMsg{Code: 'h', Text: "h"})
	if m.Current() != names[0] {
		t.Errorf("h gave %q, want %q", m.Current(), names[0])
	}
	m, _ = pressed(t, m, tea.KeyPressMsg{Code: 'l', Text: "l"})
	m, _ = pressed(t, m, tea.KeyPressMsg{Code: '[', Text: "["})
	if m.Current() != names[0] {
		t.Errorf("l then [ gave %q, want %q", m.Current(), names[0])
	}
	if title := m.View().WindowTitle; title != names[0] {
		t.Errorf("WindowTitle = %q, want %q", title, names[0])
	}
}

func TestSelectMsgSwitchesToKnownNamesOnly(t *testing.T) {
	m, _ := sized(t, New(Options{Effect: "rain", Seed: 1}), 40, 12)
	next, _ := m.Update(SelectMsg{Name: "decrypt"})
	m = next.(Model)
	if m.Current() != "decrypt" {
		t.Fatalf("Current() = %q after selecting decrypt", m.Current())
	}
	next, _ = m.Update(SelectMsg{Name: "no-such-effect"})
	m = next.(Model)
	if m.Current() != "decrypt" {
		t.Fatalf("an unknown name moved the showroom to %q", m.Current())
	}
}

func TestReplayRebuildsFromTheStart(t *testing.T) {
	for _, key := range []tea.KeyPressMsg{{Code: 'r', Text: "r"}, {Code: tea.KeyEnter}} {
		m, _ := sized(t, New(Options{Effect: "rain", Seed: 1}), 40, 12)
		m = ticked(t, m, 10)
		before := m.engine
		m, _ = pressed(t, m, key)
		if m.engine == before {
			t.Errorf("%s did not rebuild", key)
		}
		if m.mode != modePlaying {
			t.Errorf("%s left the mode at %v", key, m.mode)
		}
	}
}

func TestSpacePausesAndResumes(t *testing.T) {
	m, _ := sized(t, New(Options{Effect: "randomsequence", Seed: 1}), 40, 12)
	m = ticked(t, m, 5)
	m, _ = pressed(t, m, tea.KeyPressMsg{Code: tea.KeySpace})
	if m.mode != modePaused {
		t.Fatalf("space left the mode at %v, want paused", m.mode)
	}
	frozen := m.frame
	if m = ticked(t, m, 60); m.frame != frozen {
		t.Fatal("a paused showroom kept animating")
	}
	if !strings.Contains(m.View().Content, "paused") {
		t.Error("the status line should say it is paused")
	}
	m, _ = pressed(t, m, tea.KeyPressMsg{Code: tea.KeySpace})
	if m.mode != modePlaying {
		t.Fatalf("second space left the mode at %v, want playing", m.mode)
	}
	if m = ticked(t, m, 60); m.frame == frozen {
		t.Fatal("a resumed showroom did not animate")
	}
}

func TestSpaceDuringTheHoldReplaysAtOnce(t *testing.T) {
	m, _ := sized(t, New(Options{Effect: "wipe", Seed: 1, FPS: 10}), 20, 5)
	for n := 0; n < 5000 && m.mode != modeHolding; n++ {
		m = ticked(t, m, 1)
	}
	first := m.engine
	m, _ = pressed(t, m, tea.KeyPressMsg{Code: tea.KeySpace})
	if m.mode != modePlaying || m.engine == first {
		t.Fatal("space during the hold should replay immediately")
	}
}

func TestResizeKeepsThePause(t *testing.T) {
	m, _ := sized(t, New(Options{Effect: "randomsequence", Seed: 1}), 40, 12)
	m = ticked(t, m, 5)
	m, _ = pressed(t, m, tea.KeyPressMsg{Code: tea.KeySpace})
	m, _ = sized(t, m, 60, 20)
	if m.mode != modePaused {
		t.Fatalf("resize left the mode at %v, want paused", m.mode)
	}
	frozen := m.frame
	if m = ticked(t, m, 30); m.frame != frozen {
		t.Fatal("a paused showroom animated after a resize")
	}
}

func TestQuitKeysOnlyWhenHonoured(t *testing.T) {
	quitKeys := []tea.KeyPressMsg{
		{Code: 'q', Text: "q"},
		{Code: tea.KeyEscape},
		{Code: 'c', Mod: tea.ModCtrl},
	}
	native, _ := sized(t, New(Options{Effect: "rain", Seed: 1, QuitKeys: true}), 40, 12)
	for _, key := range quitKeys {
		_, cmd := pressed(t, native, key)
		if cmd == nil {
			t.Errorf("%s did not quit", key)
			continue
		}
		if _, ok := cmd().(tea.QuitMsg); !ok {
			t.Errorf("%s returned %T, want tea.QuitMsg", key, cmd())
		}
	}
	browser, _ := sized(t, New(Options{Effect: "rain", Seed: 1}), 40, 12)
	for _, key := range quitKeys {
		if _, cmd := pressed(t, browser, key); cmd != nil {
			t.Errorf("%s quit with quit keys off", key)
		}
	}
}
