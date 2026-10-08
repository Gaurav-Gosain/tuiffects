package tuiffects

import (
	"fmt"
	"strings"
)

// swarm, ported from ttfx src/effects/swarm.rs, which ports
// TerminalTextEffects effects/effect_swarm.py by ChrisBuilds.

func init() {
	Register(Descriptor{
		Name:        "swarm",
		Description: "Characters fly in groups between a few gathering points, then settle into place",
		New:         func() Effect { return NewSwarm(DefaultSwarmConfig()) },
	})
}

// swarmAreaMarker is in every swarm-area path id. The effect reads a path id
// back in Advance to tell a gathering flight from an inner wander.
const swarmAreaMarker = "swarm_area"

// swarmInnerPathsPerArea is how many short hops a character makes inside a
// gathering area before it moves on to the next one.
const swarmInnerPathsPerArea = 2

// SwarmConfig tunes the swarm effect.
type SwarmConfig struct {
	// BaseColors are the swarm body colours. Each swarm picks one.
	BaseColors []Color
	// FlashColor is what a character wears at the top of every flight.
	FlashColor Color
	// SwarmSize is the share of all characters in one swarm, from 0 to 1.
	SwarmSize float64
	// SwarmCoordination is the chance that a character follows the first of
	// its swarm to reach the next gathering area, from 0 to 1.
	SwarmCoordination float64
	// SwarmAreaCountLow and SwarmAreaCountHigh bound how many gathering areas
	// a swarm visits.
	SwarmAreaCountLow  int
	SwarmAreaCountHigh int
	// FinalGradientStops colour the text once it lands. They are ignored when
	// the engine is set to resolve to the input's own colours.
	FinalGradientStops     []Color
	FinalGradientSteps     []int
	FinalGradientDirection GradientDirection
}

// DefaultSwarmConfig is upstream's default swarm.
func DefaultSwarmConfig() SwarmConfig {
	return SwarmConfig{
		BaseColors:         []Color{MustParseColor("31a0d4")},
		FlashColor:         MustParseColor("f2ea79"),
		SwarmSize:          0.1,
		SwarmCoordination:  0.80,
		SwarmAreaCountLow:  2,
		SwarmAreaCountHigh: 4,
		FinalGradientStops: []Color{
			MustParseColor("31b900"), MustParseColor("f0ff65"),
		},
		FinalGradientSteps:     []int{12},
		FinalGradientDirection: Horizontal,
	}
}

// Swarm splits the text into groups, launches one group at a time from a point
// off the canvas, and flies each group between a few gathering areas before
// its members break away to their own cells.
//
// This effect assembles the screen rather than passing over it, so under
// DynamicExistingColors it still starts from an empty canvas: a character
// becomes visible only when its swarm launches. Showing every character from
// the first frame, which is what a sweeping effect has to do, would leave the
// finished picture on screen with the swarms crawling over the top of it.
type Swarm struct {
	config SwarmConfig

	// swarms holds the groups still to launch. Advance pops from the end.
	swarms [][]*Character
	// callNext is set when the current swarm has lost enough members to the
	// landing path that the next one should go.
	callNext bool
	// activeSwarmArea is the furthest gathering area the current swarm has
	// reached. It only ever moves forward.
	activeSwarmArea string
	currentSwarm    []*Character
}

// NewSwarm builds the effect.
func NewSwarm(config SwarmConfig) *Swarm {
	return &Swarm{
		config:          config,
		callNext:        true,
		activeSwarmArea: swarmAreaName(0),
	}
}

func swarmAreaName(index int) string {
	return fmt.Sprintf("%d_%s", index, swarmAreaMarker)
}

// swarmAreaIndex reproduces effect_swarm.py's int(path_id[0]): only the first
// character of the id is read, so a tenth gathering area reads as the first
// one and the swarm never advances past it. Every id this effect makes starts
// with a digit, so there is nothing here that can fail to parse.
func swarmAreaIndex(id string) int {
	if id == "" || id[0] < '0' || id[0] > '9' {
		return -1
	}
	return int(id[0] - '0')
}

// swarmArea is one gathering point and the cells a character may pick inside
// it. Upstream keys a dict by the focus coord, so a repeated focus coord
// overwrites its cells in place and keeps its original position; this slice
// does the same.
type swarmArea struct {
	focus  Coord
	coords []Coord
}

// makeSwarms cuts the text into groups of swarmSize, taken from the end of a
// bottom-to-top right-to-left ordering. A last group smaller than half a swarm
// is folded into the one before it.
func (s *Swarm) makeSwarms(e *Engine, swarmSize int) error {
	unswarmed := e.Terminal.GetCharacters(e.Rng, InputOnly(), SortBottomToTopRightToLeft)
	for len(unswarmed) > 0 {
		newSwarm := make([]*Character, 0, swarmSize)
		for i := 0; i < swarmSize && len(unswarmed) > 0; i++ {
			newSwarm = append(newSwarm, unswarmed[len(unswarmed)-1])
			unswarmed = unswarmed[:len(unswarmed)-1]
		}
		s.swarms = append(s.swarms, newSwarm)
	}
	if len(s.swarms) == 0 {
		// Nothing to swarm, so Advance ends the effect on its first call,
		// as every other effect does over an empty screen.
		return nil
	}
	final := s.swarms[len(s.swarms)-1]
	s.swarms = s.swarms[:len(s.swarms)-1]
	if len(final) < floorDiv(swarmSize, 2) {
		if len(s.swarms) == 0 {
			// Upstream raises IndexError here. It is unreachable with a swarm
			// size taken from a share of the character count, because that
			// share is never more than twice the count.
			return fmt.Errorf("swarm: no preceding swarm to merge the last one into")
		}
		s.swarms[len(s.swarms)-1] = append(s.swarms[len(s.swarms)-1], final...)
	} else {
		s.swarms = append(s.swarms, final)
	}
	return nil
}

// Build cuts the text into swarms and gives every character its flight plan:
// a flash scene for the moves, a path per gathering area with two short hops
// inside it, and a landing path back to its own cell.
func (s *Swarm) Build(e *Engine) error {
	// SwarmIterator.DYNAMIC_CLEAR_COLOR
	dynamicClearColor := MustParseColor("ffffff")

	characters := e.Terminal.GetCharacters(e.Rng, InputOnly(), SortTopToBottomLeftToRight)
	swarmSize := max(roundHalfEven(float64(len(characters))*s.config.SwarmSize), 1)
	if err := s.makeSwarms(e, swarmSize); err != nil {
		return err
	}

	gradient, err := NewGradient(s.config.FinalGradientStops, s.config.FinalGradientSteps, false)
	if err != nil {
		return err
	}
	canvas := e.Terminal.Canvas
	mapping, err := gradient.BuildCoordinateColorMapping(
		canvas.TextBottom, canvas.TextTop, canvas.TextLeft, canvas.TextRight,
		s.config.FinalGradientDirection)
	if err != nil {
		return err
	}
	fallback := gradient.Spectrum[0]
	dynamic := e.Terminal.Config.ExistingColorHandling == DynamicExistingColors

	finalColors := make(map[*Character]ColorPair, len(characters))
	for _, ch := range characters {
		if dynamic {
			// A character that carried no colours of its own resolves to an
			// empty pair, which is the clear-to-nothing case below.
			finalColors[ch] = ch.Animation.InputColors
		} else {
			finalColors[ch] = Fg(mapping.At(ch.InputCoord, fallback))
		}
	}

	flashRun := make([]Color, 10)
	for i := range flashRun {
		flashRun[i] = s.config.FlashColor
	}

	// The gathering areas are picked by stepping around a circle from the last
	// focus point. Upstream's find_coords_on_circle is lru_cached and this
	// effect shuffles the returned list in place, so a second visit to the
	// same focus coord gets the previously shuffled list rather than a fresh
	// one. The cache is kept here, and shuffled in place, to reproduce that.
	circleCache := map[Coord][]Coord{}
	radius := max(floorDiv(min(canvas.Right, canvas.Top), 2), 1)
	areaDiameter := max(floorDiv(min(canvas.Right, canvas.Top), 6), 1) * 2

	for _, swarm := range s.swarms {
		base := *Choice(e.Rng, s.config.BaseColors)
		swarmGradient, err := NewGradientSteps([]Color{base, s.config.FlashColor}, 7, false)
		if err != nil {
			return err
		}
		// Out to the flash colour, hold there, and back to the base colour.
		mirror := make([]Color, 0, 2*len(swarmGradient.Spectrum)+len(flashRun))
		mirror = append(mirror, swarmGradient.Spectrum...)
		mirror = append(mirror, flashRun...)
		for i := len(swarmGradient.Spectrum) - 1; i >= 0; i-- {
			mirror = append(mirror, swarmGradient.Spectrum[i])
		}

		var areas []swarmArea
		spawn := canvas.RandomCoord(e.Rng, true, false)
		areaCount := e.Rng.IntBetween(s.config.SwarmAreaCountLow, s.config.SwarmAreaCountHigh)
		lastFocus := spawn
		for placed := 0; placed < areaCount; placed++ {
			candidates, cached := circleCache[lastFocus]
			if !cached {
				candidates = FindCoordsOnCircle(lastFocus, radius, 0, true)
				circleCache[lastFocus] = candidates
			}
			Shuffle(e.Rng, candidates)
			// The fallback is drawn only when no coordinate on the circle is
			// inside the canvas, as it is upstream. Drawing it up front would
			// take two values off the generator on every iteration.
			var nextFocus Coord
			found := false
			for _, coord := range candidates {
				if canvas.CoordIsInCanvas(coord) {
					nextFocus, found = coord, true
					break
				}
			}
			if !found {
				nextFocus = canvas.RandomCoord(e.Rng, false, false)
			}
			areaCoords := FindCoordsInCircle(lastFocus, areaDiameter)
			replaced := false
			for i := range areas {
				if areas[i].focus == lastFocus {
					areas[i].coords = areaCoords
					replaced = true
					break
				}
			}
			if !replaced {
				areas = append(areas, swarmArea{focus: lastFocus, coords: areaCoords})
			}
			lastFocus = nextFocus
		}

		for _, ch := range swarm {
			ch.Motion.SetCoordinate(spawn)
			// The flash is synced to distance, so a character is brightest in
			// the middle of a hop however long that hop turns out to be.
			flash := ch.Animation.NewScene("", SceneOptions{
				Sync:            SyncDistance,
				UsesInputColors: ch.UsesInputColors,
				Frames:          len(mirror),
			})
			for _, step := range mirror {
				// A cell whose only colour was a background is a blank space
				// with a fill: a piece of window chrome, a bar, a panel. It
				// draws nothing at all while it flies unless the flash is put
				// on both channels, and on a captured screen that is a large
				// part of the picture missing for most of the run. Upstream
				// animates piped text, where nothing carries a background, so
				// the default path is exactly upstream's.
				colors := Fg(step)
				if dynamic && ch.Animation.InputColors.HasBg && !ch.Animation.InputColors.HasFg {
					colors = FgBg(step, step)
				}
				if err := flash.AddFrame(ch.InputSymbol, 1, VisualParams{Colors: colors}); err != nil {
					return err
				}
			}

			for areaIndex := range areas {
				areaName := swarmAreaName(areaIndex)
				origin, err := ch.Motion.NewPath(areaName, PathOptions{
					Speed: 0.4, Ease: OutSine, HasEase: true,
				})
				if err != nil {
					return err
				}
				if _, err := origin.NewWaypoint(*Choice(e.Rng, areas[areaIndex].coords), nil, areaName); err != nil {
					return err
				}
				// A character in flight is drawn over one that has landed.
				ch.RegisterEvent(PathActivated, PathCaller(areaName), ActivateScene(flash.ID))
				ch.RegisterEvent(PathActivated, PathCaller(areaName), SetLayer(1))
				ch.RegisterEvent(PathComplete, PathCaller(areaName), DeactivateScene(""))

				for inner := 0; inner < swarmInnerPathsPerArea; inner++ {
					next := *Choice(e.Rng, areas[areaIndex].coords)
					// Upstream names the inner path and its waypoint after the
					// path count, reading it again after the path is added, so
					// the waypoint id is one higher than the path id.
					innerID := fmt.Sprintf("%d", ch.Motion.paths.Len())
					innerPath, err := ch.Motion.NewPath(innerID, PathOptions{
						Speed: 0.18, Ease: InOutSine, HasEase: true,
					})
					if err != nil {
						return err
					}
					waypointID := fmt.Sprintf("%d", ch.Motion.paths.Len())
					if _, err := innerPath.NewWaypoint(next, nil, waypointID); err != nil {
						return err
					}
				}
			}

			landing, err := ch.Motion.NewPath("", PathOptions{
				Speed: 0.45, Ease: InOutQuad, HasEase: true,
			})
			if err != nil {
				return err
			}
			if _, err := landing.NewWaypoint(ch.InputCoord, nil, ""); err != nil {
				return err
			}

			final := finalColors[ch]
			settle := ch.Animation.NewScene("", SceneOptions{UsesInputColors: ch.UsesInputColors})
			switch {
			case dynamic && !final.HasFg && !final.HasBg:
				// Nothing to resolve back to, so the character fades from the
				// flash to white and then to no colour at all.
				clearGradient, err := NewGradientSteps(
					[]Color{s.config.FlashColor, dynamicClearColor}, 10, false)
				if err != nil {
					return err
				}
				for _, step := range clearGradient.Spectrum {
					if err := settle.AddFrame(ch.InputSymbol, 3, VisualParams{Colors: Fg(step)}); err != nil {
						return err
					}
				}
				if err := settle.AddFrame(ch.InputSymbol, 3, VisualParams{}); err != nil {
					return err
				}
			case dynamic:
				// Both halves of the input colour are ramped, so a cell that
				// arrived with a background lands wearing it again.
				var fgGradient, bgGradient *Gradient
				if final.HasFg {
					if fgGradient, err = NewGradientSteps(
						[]Color{s.config.FlashColor, final.Fg}, 10, false); err != nil {
						return err
					}
				}
				if final.HasBg {
					if bgGradient, err = NewGradientSteps(
						[]Color{s.config.FlashColor, final.Bg}, 10, false); err != nil {
						return err
					}
				}
				if err := settle.ApplyGradientToSymbols(
					[]string{ch.InputSymbol}, 3, fgGradient, bgGradient); err != nil {
					return err
				}
			default:
				landingGradient, err := NewGradientSteps(
					[]Color{s.config.FlashColor, final.Fg}, 10, false)
				if err != nil {
					return err
				}
				for _, step := range landingGradient.Spectrum {
					if err := settle.AddFrame(ch.InputSymbol, 3, VisualParams{Colors: Fg(step)}); err != nil {
						return err
					}
				}
			}

			ch.RegisterEvent(PathComplete, PathCaller(landing.ID), ActivateScene(settle.ID))
			ch.RegisterEvent(PathComplete, PathCaller(landing.ID), SetLayer(0))
			ch.RegisterEvent(PathActivated, PathCaller(landing.ID), ActivateScene(flash.ID))

			// Every path in the order it was built: area, two hops, area, two
			// hops, and finally the landing.
			e.ChainPaths(ch, ch.Motion.paths.Keys(), false)
		}
	}

	s.callNext = true
	s.activeSwarmArea = swarmAreaName(0)
	return nil
}

// Advance runs one frame and reports whether the effect is still going.
func (s *Swarm) Advance(e *Engine) bool {
	if len(s.swarms) == 0 && e.ActiveCount() == 0 {
		return false
	}
	if len(s.swarms) > 0 && s.callNext {
		s.callNext = false
		s.currentSwarm = s.swarms[len(s.swarms)-1]
		s.swarms = s.swarms[:len(s.swarms)-1]
		s.activeSwarmArea = swarmAreaName(0)
		for _, ch := range s.currentSwarm {
			e.ActivatePath(ch, s.activeSwarmArea)
			e.Terminal.SetCharacterVisibility(ch, true)
			e.Activate(ch)
		}
	}
	if e.ActiveCount() < len(s.currentSwarm) {
		// Some of the swarm has landed, so the next one can launch.
		s.callNext = true
	}
	// The first character to reach a later gathering area pulls most of its
	// swarm along with it. That is what makes a swarm read as one body rather
	// than as a group of characters on the same route.
	for _, ch := range s.currentSwarm {
		if !ch.Motion.hasActivePath {
			continue
		}
		pathID := ch.Motion.activePath
		if pathID == s.activeSwarmArea || !strings.Contains(pathID, swarmAreaMarker) {
			continue
		}
		if swarmAreaIndex(pathID) <= swarmAreaIndex(s.activeSwarmArea) {
			continue
		}
		s.activeSwarmArea = pathID
		for _, other := range s.currentSwarm {
			if other != ch && e.Rng.Float() < s.config.SwarmCoordination {
				e.ActivatePath(other, s.activeSwarmArea)
			}
		}
		break
	}
	e.Update()
	return true
}
