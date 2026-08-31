// One viewport read per painted frame.
//
// WHAT IS SLOW
//
// ghostty-web's Canvas renderer paints a frame one row at a time. For each row
// it calls GhosttyTerminal.getLine(row). getLine is a compatibility shim: it
// calls getViewport(), and getViewport walks every cell in the grid across the
// wasm boundary. So a frame that paints N rows walks the whole grid N times,
// when one walk already holds every row the frame needs.
//
// The library caches the per-row dirty and wrap flags, but not the viewport
// itself, so nothing stops the repeat walks.
//
// WHAT THIS DOES
//
// It wraps two methods on the live objects. render() marks the frame and
// clears the memo. The first getLine of the frame fills the memo with one
// getViewport(), and the rest of the rows are slices of it.
//
// The memo cannot serve stale cells. JavaScript runs the frame to completion,
// so no terminal write can land inside render(). Outside render() the memo is
// empty, so selection and link code still take the original path.
//
// It returns the same value getLine always returned: a fresh copy of the row,
// so nothing holds a reference into the shared cell pool.
//
// WHY IT IS HERE AND NOT IN ghostty-web
//
// tuiffects does not own ghostty-web and does not edit its file. web/ghostty-web/
// is copied in by `go tool booba-assets` and stays exactly as go-booba shipped
// it. This file sits beside it and wraps its public methods at runtime.
//
// The same change is prepared as a patch against ghostty-web itself, in
// upstream/. When ghostty-web reads the viewport once per frame on its own,
// delete this file and its import in index.html.
//
// ghostty-web is MIT. Copyright (c) Coder Technologies, and the NimbleMarkets
// fork of it. No ghostty-web code is copied here. This file is tuiffects' own
// wrapper, MIT, Copyright (c) 2026 Gaurav Gosain.

/**
 * Wrap a live terminal so each painted frame reads the viewport once.
 *
 * Returns a handle with uninstall(), or null if the terminal does not look
 * like the ghostty-web this was written against. A null return is not an
 * error. The page keeps working at the original speed.
 *
 * @param {object} terminal a ghostty-web Terminal that is already open
 */
export function readViewportOncePerFrame(terminal) {
  const vt = terminal && terminal.wasmTerm;
  const renderer = terminal && terminal.renderer;
  if (!vt || !renderer) return null;
  if (typeof vt.getLine !== 'function' || typeof vt.getViewport !== 'function') return null;
  if (typeof renderer.render !== 'function') return null;

  // Patch the prototypes, not the objects. Terminal.reset() builds a new
  // wasmTerm, and a patch on the old object would go with it.
  const vtProto = Object.getPrototypeOf(vt);
  const rendererProto = Object.getPrototypeOf(renderer);
  if (!vtProto || !rendererProto) return null;
  if (!Object.hasOwn(vtProto, 'getLine') || !Object.hasOwn(rendererProto, 'render')) return null;
  if (vtProto.__viewportMemo) return null;

  const originalGetLine = vtProto.getLine;
  const originalRender = rendererProto.render;
  let inFrame = false;
  let viewport = null;

  vtProto.getLine = function (row) {
    if (!inFrame) return originalGetLine.call(this, row);
    const rows = this._rows;
    const cols = this._cols;
    if (typeof rows !== 'number' || typeof cols !== 'number') {
      return originalGetLine.call(this, row);
    }
    if (row < 0 || row >= rows) return null;
    if (viewport === null) viewport = this.getViewport();
    const at = row * cols;
    return viewport.slice(at, at + cols).map((cell) => ({ ...cell }));
  };

  rendererProto.render = function (...args) {
    inFrame = true;
    viewport = null;
    try {
      return originalRender.apply(this, args);
    } finally {
      inFrame = false;
      viewport = null;
    }
  };

  vtProto.__viewportMemo = true;
  return {
    uninstall() {
      vtProto.getLine = originalGetLine;
      rendererProto.render = originalRender;
      delete vtProto.__viewportMemo;
      inFrame = false;
      viewport = null;
    },
  };
}
