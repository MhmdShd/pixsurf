# pixsurf v1 — Design

Date: 2026-07-31
Repo: git@github.com:MhmdShd/pixsurf.git

## Overview

`pixsurf` is a terminal web browser. The user runs `pixsurf <url>` and the page
renders as colored pixels inside the terminal. Pages are rendered by a headless
Chrome/Chromium instance; pixsurf captures screenshots and draws them using
truecolor ANSI half-block characters.

- Language: Go (single static binary)
- Browser engine: headless Chrome/Chromium via `chromedp`
- Terminal UI: `tcell` (raw input, mouse events, truecolor output)
- Requirement on user machine: Chrome or Chromium installed

## v1 Scope

In scope:
- Open a URL (CLI argument or in-app URL bar)
- Render page as truecolor half-block pixels
- Scroll (arrow keys, PgUp/PgDn)
- Click links with the mouse (terminal cell mapped to page coordinates)
- Back / forward / reload
- Handle terminal resize

Out of scope for v1:
- Typing into forms / login pages
- Tabs, bookmarks, history persistence
- Sixel / Kitty graphics protocols (half-blocks only; upgrade path later)

## Architecture

Four units:

### browser/ — page driver
Wraps chromedp. Interface:
- `Open(url)` — normalize URL (prefix `https://` when scheme missing),
  navigate, wait for readiness (load event + 500ms settle, 15s hard timeout)
- `Screenshot() (image.Image, error)` — capture current viewport
- `ScrollBy(dy)` — scroll page by dy page pixels
- `ClickAt(x, y)` — dispatch click at page coordinates
- `Back()`, `Forward()`, `Reload()`
- `SetViewport(w, h)` — set page viewport in page pixels (see Rendering scale)
- `CurrentURL() string`

Chrome launch: headless; retry with `--no-sandbox` when running as root or
when the sandbox fails to start.

Depends on: chromedp. Knows nothing about terminals.

### Rendering scale

The page is NOT rendered at terminal-pixel size (80×24 cells → 160×48 px
would destroy page layout). Instead:

- Page viewport width is fixed at 1280 page pixels.
- Viewport height preserves the terminal grid's aspect ratio:
  `1280 * (gridRows*2) / (gridCols*2)` where the grid is the terminal minus
  the status bar row (grid rows = term rows − 1).
- The screenshot (1280×H) is downscaled (box filter / nearest area average)
  to `gridCols × gridRows*2` pixels for half-block output.
- Scale factor `s = 1280 / gridCols` converts terminal cell coordinates to
  page coordinates in `CellToPage`; scroll steps are also expressed in page
  pixels via `s`.

### render/ — image to terminal cells
Pure functions:
- `ToCells(img, cols, rows) [][]Cell` — downscale image to cols × rows*2
  pixels; each cell is `▀` with foreground = top pixel color, background =
  bottom pixel color.
- `CellToPage(cellX, cellY, scale) (pageX, pageY)` — map a clicked terminal
  cell back to page pixel coordinates (cell center × scale) for ClickAt.

Depends on: stdlib image only. Fully unit-testable.

### ui/ — terminal frontend
tcell-based event loop:
- Draw cell grid + one-line status bar (URL, load state, errors)
- URL input prompt (`g` key)
- Key/mouse event capture

Depends on: tcell. Knows nothing about Chrome.

### main.go — glue
Event loop: input event → browser action → screenshot → render → draw.
Debounce resize events (re-viewport + re-capture) and rapid scroll keypresses
(coalesce into one scroll + one recapture, ~50ms window) so held-down arrow
keys stay responsive.

## Controls

| Key | Action |
|-----|--------|
| ↑ / ↓ | scroll line |
| PgUp / PgDn | scroll page |
| mouse click | click link at that spot |
| g | open URL bar |
| b / f | back / forward |
| r | reload |
| q / Ctrl-C | quit |

## Error Handling

- Chrome not found → exit with clear message and install hint per OS.
- Navigation error / timeout → error shown in status bar; current page stays.
- Resize → viewport update, re-capture, re-render.

## Testing

- Unit tests: render math (image→cells colors, downscale dimensions,
  cell→page coordinate mapping, resize edge cases).
- Manual smoke tests against real sites (example.com, wikipedia.org).
- CI: GitHub Actions — `go build`, `go vet`, `go test` on push.

## Distribution

- GitHub repo MhmdShd/pixsurf, MIT license.
- README: what it is, install (`go install`), controls, demo GIF placeholder,
  requirement note (Chrome/Chromium).
- Later (post-v1): GoReleaser prebuilt binaries, sixel/kitty renderers,
  form input.
