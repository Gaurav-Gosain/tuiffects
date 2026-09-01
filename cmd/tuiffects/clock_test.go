package main

import (
	"math"
	"strings"
	"testing"

	tfx "github.com/Gaurav-Gosain/tuiffects"
)

// framesAt runs an effect to its end and reports how many frames it took.
func framesAt(t *testing.T, name string, fps int) int {
	t.Helper()
	g, _, err := buildGrid(strings.NewReader(strings.Repeat("the quick brown fox\n", 10)), 40, 10)
	if err != nil {
		t.Fatal(err)
	}
	d, ok := tfx.Lookup(name)
	if !ok {
		t.Fatalf("there is no effect named %q", name)
	}
	engine := newEngineOver(g, d, options{fps: fps, seed: 99, colors: "auto"})
	effect := d.New()
	if err := effect.Build(engine); err != nil {
		t.Fatalf("build %s: %v", name, err)
	}
	frames := 0
	for frames < frameCeiling && effect.Advance(engine) {
		frames++
	}
	return frames
}

// TestAnEffectWrittenInSecondsLastsTheSameTimeAtAnyFrameRate is the whole
// reason the engine's clock is set from the frame rate.
//
// matrix rains for a number of seconds rather than a number of frames, and it
// reads those seconds off the engine's clock. The clock only knows what a
// second is if it is told the rate the host paints at. Left at the default
// sixty on a host painting at thirty, the rain lasts half as long as it should.
//
// An effect is part seconds and part frames, so its frame count is
// seconds x rate + frames: the clock-driven part grows with the frame rate and
// the frame-driven part does not. Two frame rates give the seconds, and the
// test is that two different pairs of rates give the same seconds. A clock left
// at the default makes the frame count the same at every rate, so the seconds
// come out zero, which is what the last check here refuses.
//
// thunderstorm is the third effect written in seconds and it is not in the
// list. Its lightning is scheduled on clock intervals, so the number of strikes
// a run fits in changes by one when the rate changes, and its frame count is
// therefore not a straight line. It is right for the same reason and this test
// cannot say so.
func TestAnEffectWrittenInSecondsLastsTheSameTimeAtAnyFrameRate(t *testing.T) {
	for _, name := range []string{"matrix", "tuffbaby"} {
		t.Run(name, func(t *testing.T) {
			at30 := framesAt(t, name, 30)
			at60 := framesAt(t, name, 60)
			at120 := framesAt(t, name, 120)
			slow := float64(at60-at30) / 30
			fast := float64(at120-at60) / 60
			t.Logf("%s: %d frames at 30fps, %d at 60fps, %d at 120fps. "+
				"The clock-driven part is %.2fs by the first pair and %.2fs by the second.",
				name, at30, at60, at120, slow, fast)
			if slow <= 0.5 {
				t.Fatalf("%s spends %.2f seconds on the clock. "+
					"Either it is not clock driven or the engine clock was never set from the frame rate.",
					name, slow)
			}
			if drift := math.Abs(fast-slow) / slow; drift > 0.02 {
				t.Errorf("the clock-driven part is %.2fs between 30 and 60fps and %.2fs between 60 and 120fps, a drift of %.0f%%",
					slow, fast, drift*100)
			}
			// And the wait itself: seconds on the clock, plus a fixed number of
			// frames, divided by the rate.
			fixed := float64(at60) - slow*60
			for _, at := range []struct {
				fps    int
				frames int
			}{{30, at30}, {120, at120}} {
				want := slow + fixed/float64(at.fps)
				got := float64(at.frames) / float64(at.fps)
				if math.Abs(got-want) > 0.05 {
					t.Errorf("at %dfps the run takes %.2fs, want %.2fs", at.fps, got, want)
				}
			}
		})
	}
}

// TestTheClockIsTheOneTheFrameRateAsksFor pins the wiring the test above
// depends on, so a clock that was never installed fails as a wrong clock
// rather than as a wrong duration.
func TestTheClockIsTheOneTheFrameRateAsksFor(t *testing.T) {
	g, _, err := buildGrid(strings.NewReader("hello\n"), 20, 4)
	if err != nil {
		t.Fatal(err)
	}
	d, _ := tfx.Lookup("wipe")
	engine := newEngineOver(g, d, options{fps: 30, seed: 1, colors: "auto"})
	// Thirty frames of a thirty frame clock is one second.
	for range 30 {
		engine.Clock.AdvanceFrame()
	}
	if got := engine.Clock.Elapsed(); math.Abs(got-1) > 1e-9 {
		t.Errorf("thirty frames moved the clock %.4f seconds, want 1", got)
	}
}
