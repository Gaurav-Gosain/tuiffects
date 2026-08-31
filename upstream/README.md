# Prepared for ghostty-web, not yet sent

Nothing here has been posted. These are the files for one upstream submission.

## What it is

`ghostty-web`'s Canvas renderer reads the whole terminal grid once for every
row it paints. Reading it once per frame instead makes `render()` three to four
times cheaper on the tuiffects page, and nine times cheaper on a screen where
every row changes.

## Where it goes

`coder/ghostty-web`, branch `main`.

`NimbleMarkets/ghostty-web` is a fork of it, and go-booba vendors the fork.
Coder is still the right target:

- `getLine()` is byte for byte identical in both repos, and so is the row loop
  in `render()` apart from one extra disjunct the fork added.
- The fork has issues disabled, still declares `"author": "Coder"` in its
  `package.json`, and points its bug tracker at coder.
- The patch applies to the fork as well, because the function is unmodified
  there.

## Files

| file | what it is |
| --- | --- |
| `0001-perf-renderer-read-the-viewport-once-per-frame.patch` | the diff, against `coder/ghostty-web` at `1858a594` |
| `PR.md` | the pull request text |
| `viewport-repro.html` | the reproduction, also included in the patch as `demo/viewport-repro.html` |

## House rules that apply

- `coder/ghostty-web` checks pull request titles against Conventional Commits
  in `.github/workflows/pr-title.yml`. The title above is `perf(renderer): ...`,
  which passes, and release-please turns it into the changelog line.
- There is no CONTRIBUTING.md and no issue template. Issues are open, pull
  requests are the normal flow.
- MIT, Copyright (c) 2025 Coder.

## Before sending it

- Check the author line on the patch. It is set to the tuiffects maintainer.
- Nothing here has been posted. No issue, no pull request, no fork pushed.

## To apply it

```
git clone https://github.com/coder/ghostty-web
cd ghostty-web
git am < 0001-perf-renderer-read-the-viewport-once-per-frame.patch
bun install && bun run dev      # then open /demo/viewport-repro.html
```
