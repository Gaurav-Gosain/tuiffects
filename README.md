# tuiffects

Terminal text effects as a Go library. Feed it text or a captured cell grid,
pick an effect, and pull frames off it one at a time.

## Credit where it is due

**This is a port of a port, and it originates none of the art.**

[TerminalTextEffects][tte] by [ChrisBuilds][chrisbuilds] is the original.
Every effect in this package, and the architecture of the engine that runs
them, are that project's design. [ttfx][ttfx] by omacom-io translated that work
to Rust and says the same thing about itself. This package translates ttfx to
Go.

All three are MIT. All three copyrights are preserved in [LICENSE](LICENSE),
and [NOTICE](NOTICE) maps every file here to both the ttfx source and the
TerminalTextEffects source it came from.

One effect, `tuffbaby`, is the exception: it is original to this package and
has no upstream. It is written against ChrisBuilds' engine like every other
effect here. It is not in NOTICE, because NOTICE records what was translated
and from where; it declares where its material came from in its own
`Descriptor.Origin` instead. `catalogue_test.go` holds every effect to one or
the other and refuses both, so an original cannot quietly claim an upstream
and a port cannot quietly lose one.

If you like what you see, star [the original][tte]. Effect ideas belong
upstream, where they were invented.

[tte]: https://github.com/ChrisBuilds/terminaltexteffects
[chrisbuilds]: https://github.com/ChrisBuilds
[ttfx]: https://github.com/omacom-io/ttfx

## Using it

```go
import "github.com/Gaurav-Gosain/tuiffects"
```

```go
terminal := tuiffects.NewTerminalFromText("hello", tuiffects.TerminalConfig{
    Width: 80, Height: 24,
})
engine := tuiffects.NewEngine(terminal, tuiffects.NewRng(1))

effect := tuiffects.NewDecrypt(tuiffects.DefaultDecryptConfig())
if err := effect.Build(engine); err != nil {
    return err
}
for effect.Advance(engine) {
    fmt.Print("\x1b[H", engine.Frame())
}
```

`Advance` does not return the frame. Read it with `engine.Frame()` for an ANSI
string, or `engine.FrameRows()` for rows of visuals you can style yourself.
That is the one place this port deliberately differs in shape from ttfx, which
writes to a tty it owns. It is what lets this drive a pane, a widget or a
screen saver rather than a terminal.

To animate a screen rather than a string, hand it a cell grid:

```go
terminal := tuiffects.NewTerminalFromCells(cells, tuiffects.TerminalConfig{
    Width:                 cols,
    Height:                rows,
    ExistingColorHandling: tuiffects.DynamicExistingColors,
})
```

`DynamicExistingColors` makes every character resolve back to the colour it
arrived with, so the screen reassembles as itself rather than in the effect's
own palette. It is the mode to use when the input is a picture that was
already on the screen.

## Demo

Every effect runs live at
[neomantra.github.io/tuiffects](https://neomantra.github.io/tuiffects/): the
whole catalogue in a real terminal emulator, compiled to WebAssembly with
[go-booba](https://github.com/NimbleMarkets/go-booba). The same program runs
in your own terminal:

```sh
cd demo && go run ./cmd/tuiffects-demo rain
```

`←`/`→` step through the catalogue, `r` replays, `space` pauses, `q` quits.
The demo lives in its own module under `demo/` so the library stays free of
dependencies. To build and serve the page locally, `task serve` (needs
[Task](https://taskfile.dev) and `npx`), then open http://localhost:3000/.

## What is here

The engine, in the shape ttfx found it:

| Piece | What it is |
| --- | --- |
| `Coord`, `geometry.go` | 1-based grid coordinates, origin bottom left, lines and bezier curves |
| `Color`, `Gradient` | colour ramps and the coordinate mappings effects paint across the canvas |
| `Easing` | the thirty-one standard curves |
| `Waypoint`, `Path`, `Motion` | where a character goes and how fast |
| `Frame`, `Scene`, `Animation` | what a character looks like over time |
| `Event`, `Action` | how a scene or path hands off to the next one |
| `Character` | one cell: its animation, its motion, its handlers |
| `Canvas`, `Terminal` | the grid, the character populations, and the frame painter |
| `ParticlePool` | recycles the short-lived characters an effect throws off |
| `PrimsSimple` and the other spanning trees | join the canvas into a tree, and give an effect its running order |
| `Clock` | seconds, for the effects written in seconds rather than frames |
| `Engine` | the stepping loop that ties all of it together |

Thirty-five effects, every one ttfx ships except `beams` and `colorshift`:

| Effect | What it does | What it shows |
| --- | --- | --- |
| `binarypath` | every character breaks into the binary digits of its code point, which travel the canvas and collapse back into it | added characters, right-angled paths, a group released one member a frame, and a two-phase run ending in a diagonal wipe |
| `blackhole` | the text scatters into a starfield, a ring of stars eats it, then the singularity explodes and it drifts home | five phases, looping ring paths, distance-synced scenes, layers, and a colour ramp on both foreground and background |
| `bouncyballs` | balls fall in from above the screen and bounce into place | motion that starts off the canvas, a non-monotonic easing on a path, and a row-by-row release from the bottom up |
| `bubbles` | groups of characters ride a bubble down the screen, which pops and drops them into place | an added anchor character stepped by hand, a rigid ring redrawn around it each frame, and paths chained through a burst |
| `burn` | fire spreads through the text and each character cools into its colour | a spanning tree as the running order, a recycled particle pool for the smoke, and a background carried through a dynamic run |
| `crumble` | the text dims, falls to the floor as dust, is vacuumed out the top, then flies home and re-forms | four stages over one shared set of paths and scenes, a distance-synced dust animation, and a layer change mid-fall |
| `decrypt` | types out ciphertext, then decrypts it | per-character scenes and scene-to-scene chaining, no motion at all |
| `errorcorrect` | some characters start in each other's places and swap back | a scene handing off to a path and back, layer changes while a character is in flight, and a queue released on a delay |
| `expand` | the text starts piled on one cell and grows out of it | eased paths out of a single point, distance-synced colour ramps, and layer swaps in flight |
| `fireworks` | characters climb as shells, burst apart, and fall into place | three chained paths per character, bezier arcs, a looping scene and a step-synced one |
| `highlight` | a band of light sweeps over the text | an eased sequence releasing character groups, brightness-derived gradients, no motion |
| `laseretch` | a laser beam cuts the text on, one character at a time | a spanning tree for the etch order, a particle pool for the sparks |
| `matrix` | green rain falls down the screen and the text resolves out of it | the engine clock, columns cut from the whole canvas including the fill, and drawing by appearance instead of scenes |
| `middleout` | text collapses onto the centre, spreads along one axis, then expands out | two paths per character run in phases the effect drives itself, and a colour ramp that opens on a fixed starting colour |
| `orbittingvolley` | four launchers circle the canvas and fire the text into place | one moving character driving three others' positions, layered paths, and a per-frame launch queue |
| `overflow` | rows of the text scroll up past the screen out of order, then the real picture scrolls in from the bottom | copies of the input as extra characters, whole-canvas row groups, and a scroll that only lands correctly with fill characters |
| `pour` | characters pour in from one edge and fill the canvas from the near side first | row and column groups released in alternating order, one path and one colour ramp per character |
| `print` | types the canvas out one line at a time on the bottom row and scrolls the page up under it | a character of its own as the print head, a path rebuilt per line, row groups over the whole canvas, and one scene per cell |
| `rain` | characters fall in and settle | paths, easing, and a path completion handing off to an animation |
| `randomsequence` | fades the text back in one character at a time, in a random order | a shuffled reveal order, a per-character colour ramp with no motion, paths or events at all |
| `rings` | text gathers into spinning rings, scatters, and goes home | many chained looping paths per character, phase timers, and rings that turn opposite ways |
| `scattered` | characters start in random places and gather into the text | paths from random start coordinates, a distance-synced colour ramp, and layer swaps in flight |
| `slice` | the picture is cut in two and the halves slide back in from opposite edges | eased paths over the fill characters as well as the input, and two halves shearing past each other |
| `slide` | rows, columns or diagonals push in from off screen | groups released on a gap timer, one character per group per frame, each on its own eased path |
| `smoke` | smoke seeps out from one cell and colours the text as it passes | a weighted spanning tree, a breadth-first walk of it one layer per frame, scene-to-scene handover |
| `spotlights` | beams of light search the screen, meet in the middle, then widen until everything is lit | direct appearance changes with no scenes at all, chained looping paths, and a distance-based falloff |
| `spray` | characters shoot out of one point on the edge and fly into place | per-character path speed, a layer lifted for the flight and dropped on arrival, and a burst release sized by the character count |
| `swarm` | groups of characters fly between gathering points, then land | grouped characters, chained paths, and one group member pulling the rest along |
| `sweep` | two bands cross the canvas, the first uncovering the characters in grey and the second colouring them | one eased sequence run twice over different groupings, and fill characters so the whole canvas shimmers |
| `synthgrid` | a grid draws itself across the screen, fills its blocks in a few at a time, then takes itself back down | added characters on a layer above the text, fill characters, and phases driven by a per-block completion count |
| `thunderstorm` | the text dims, rain crosses it, and lightning strikes and leaves it glowing | two particle pools, the seconds clock, and characters the effect adds to the terminal itself |
| `unstable` | the screen scrambles, shakes itself apart, and flies back together | whole-screen coordinate shoves, two eased flights, and a three-phase run |
| `vhstape` | rows slip and the picture is redrawn | paths driving synced scenes, row groups, and several phases |
| `waves` | a band of blocks sweeps across | eased scenes released in bands, a sweep with no motion at all |
| `wipe` | a line crosses the screen and the text appears behind it | an easing curve deciding which character groups are released, and taking them back when it reverses |

And one that is not a port:

| Effect | What it does | What it shows |
| --- | --- | --- |
| `tuffbaby` | the text on screen gathers into a picture, a short clip plays in it a tone at a time, and everything goes home | a deflated frame sequence decoded once and scaled to the canvas, characters appended when the screen has too few and swept off the nearest edge when it has too many, and animation by repaint rather than by motion |

`tuffbaby` is the one effect here nobody upstream wrote; see
[Credit where it is due](#credit-where-it-is-due) and its `Origin`, which
names where its frames came from. It
takes whatever is already on the screen and arranges it into the picture, so
what the picture is drawn out of is your own text. The cells are the union of
every frame of the clip, which works out at a bit over half the canvas at any
size: a denser screen parks its surplus off the edges, a sparser one has the
rest appended, recycling the glyphs that were there.

It is the one effect that carries data: the frames are 18KB of deflated base64
in `tuffbaby_frames.go`, decoded once on first use, which is about half the
size of the largest hand-written effect here. And it is a continuous-tone
photograph rendered in text, so it is softer than the shapes the other effects
draw: a glyph fills about a third of its cell and how much varies per glyph,
which is louder than a five step ramp. The light end is drawn bold to claw
some of that back. It reads best on a wide canvas.

## Adding an effect

Write one file. Implement `Build` (set up scenes and paths on every character)
and `Advance` (release a few characters, call `engine.Update()`, say whether
you are done), and call `Register` from an `init`. The ones here are 160 to 1000
lines each and the engine does the rest.

[PORTING.md](PORTING.md) is the full guide for bringing one across from ttfx:
the call-for-call mapping, the quirks that are wrong on purpose, what the
colour policy does, and what a finished port has to include.

## Differences from ttfx

* No parity with the Python original, and no Mersenne Twister clone. The same
  effect will not produce the same frames as either upstream. `NewRng(seed)`
  makes a run reproducible within this package, which is what the tests need.
* Time is virtual by default. The engine's clock advances one frame's worth per
  `Update` rather than reading the machine, so an effect written in seconds
  runs to the same number of frames every time. Set `Engine.Clock` to the rate
  the host really paints at: `NewEngine` assumes sixty, and every effect
  written in seconds runs at the wrong speed on a host that paints at anything
  else. `NewRealClock` is there for a host that would rather have wall time.
* No command line, no tty writer, no resize handling. The host owns the screen.
* Thirty-five effects rather than thirty-seven. `beams` and `colorshift` are
  not ported.
* Rounding quirks that change how effects look **are** kept: half-to-even
  rounding on coordinates, floor division on gradient channel steps, and the
  bezier arc-length estimate that stops at t=0.9. Removing them would retune
  every effect by a little, silently.
* Several effects behave differently under `DynamicExistingColors`, because
  upstream is written for piped text and that mode means the input was already
  on the screen. Backgrounds a captured cell carried survive the run rather
  than blinking out; anything an effect throws across the screen carries the
  background of the cell it is over rather than punching a hole through it; a
  ramp that closes on a background starts from that background rather than
  flushing the bar white first; and an effect whose subject is a colour change
  is given a neutral foreground to work with on a cell that arrived with none.
  Each is commented where it is made, and the default behaviour is unchanged.

## Licence

MIT. See [LICENSE](LICENSE) and [NOTICE](NOTICE).
