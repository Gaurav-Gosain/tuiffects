# perf(renderer): read the viewport once per frame

Target: `coder/ghostty-web`, branch `main`, at `1858a594`.

## The mechanism

`CanvasRenderer.render()` paints a frame one row at a time:

```ts
for (let y = 0; y < dims.rows; y++) {
  if (!rowsToRender.has(y)) continue;
  line = buffer.getLine(y);
  if (line) this.renderLine(line, y, dims.cols);
}
```

`getLine()` is a compatibility shim. It builds the whole viewport and returns
one row of it:

```ts
getLine(y: number): GhosttyCell[] | null {
  if (y < 0 || y >= this._rows) return null;
  this.update();
  const viewport = this.getViewport();          // reads cols * rows cells
  const start = y * this._cols;
  return viewport.slice(start, start + this._cols).map((cell) => ({ ...cell }));
}
```

So a frame that paints R rows reads the whole grid R times. The read it repeats
is the expensive one: `getViewport()` fills the buffer through
`ghostty_render_state_get_viewport` and then parses `cols * rows` cells into the
pool. Every one of those reads but the first is discarded.

The row-level caching that exists does not cover this. `isRowDirty` and, in
forks that have it, `isRowWrapped`, are cached per row. The viewport cell pool
is the one read that is not.

The tell that this is a read problem and not a drawing problem: after the
change the renderer issues **more** 2D operations per second, because it is no
longer starved, and it is still several times faster. Drawing was never the
bottleneck.

## The change

Read the viewport once at the top of `render()` and slice each row out of it.

The memo is a local variable inside `render()`. It cannot outlive the call, so
there is no cache to invalidate and no staleness question: `render()` runs to
completion before any write can land, and every caller outside `render()`,
`SelectionManager` included, keeps taking the original `getLine()` path.

Scrolled frames keep the old path, because those rows come from the scrollback
provider rather than the viewport.

`getViewport()` is added to `IRenderable` as an **optional** method, so any
buffer that does not implement it behaves exactly as before.

The patch keeps the per-cell copy that `getLine()` makes, so callers still
never hold a reference into the reused pool. That leaves a second, larger win
on the table: inside one frame the pool is stable, so the copy is not needed
there. I left it out to keep this diff about one variable.

## Reproduction

`demo/viewport-repro.html` is in the patch. Run `bun run dev` and open
`/demo/viewport-repro.html`. It writes a full screen of changing text at 60 Hz,
counts `getViewport()` calls per frame and times `render()`, with and without
the change, in one page.

114x42 grid, Chromium 4 s per arm, NVIDIA RTX 3070:

| arm | grid reads / frame | render() mean | render() p95 | frames measured |
| --- | --- | --- | --- | --- |
| stock, `getLine()` per row | 22.0 | 66.48 ms | 131.98 ms | 58 |
| one viewport read per frame | 1.0 | 7.41 ms | 7.63 ms | 240 |

## A second measurement, on a real page

tuiffects (https://github.com/neomantra/tuiffects) is a terminal animation
catalogue compiled to wasm and drawn through ghostty-web. It repaints most of a
114x42 grid every frame, which is the load this path is worst at.

Nine paired rounds per effect, interleaved so machine drift cancels, 2.5 s per
round, load gated below 1-minute loadavg 4, all 72 rounds accepted, same
RTX 3070:

| effect | render() mean, stock | one read per frame | median of per-round ratios |
| --- | --- | --- | --- |
| matrix | 13.16 ms | 3.25 ms | 4.08x |
| blackhole | 10.54 ms | 2.69 ms | 3.91x |
| print | 8.46 ms | 2.49 ms | 3.37x |
| decrypt | 7.48 ms | 2.25 ms | 3.29x |

Whole browser process tree CPU, seconds per wall second, matrix: 0.76 to 0.42.
Full-grid reads per frame, matrix: 5.3 to 0.9.

## Notes

- No behaviour change is intended. The rows handed to `renderLine()` are the
  same objects with the same contents.
- A stale-row probe was written specifically to try to break this. It compares
  `getLine()` against a fresh read at a moment between frames, and it does
  catch a memo that outlives `render()`: such a memo fails 40 of 40 trials.
  The change in this patch passes 40 of 40, as does stock.
- The deeper version of this fix is a viewport memo on `GhosttyTerminal`
  invalidated by writes and resizes, which would also skip the read on idle
  frames. That needs every mutation path enumerated to stay correct. This patch
  does not, which is why it is shaped this way.
