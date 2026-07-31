# pixsurf v2 — Own Rendering Engine (No Chrome)

Date: 2026-07-31
Supersedes the Chrome-based pipeline from `2026-07-31-pixsurf-design.md`.

## Goal

Remove the Chrome/Chromium dependency entirely. pixsurf becomes a pure-Go
terminal browser with its own fetch → parse → style → layout → draw pipeline.
Target: runs on any VPS, < 30MB RAM, instant rendering, single static binary.

Trade-off accepted: no JavaScript. Content-oriented sites (news, docs, wikis,
blogs) render well; SPA-only sites show their static shell.

## Architecture

Pipeline: `fetch → dom → style+layout → document → ui`

### fetch/ — HTTP client
- stdlib `net/http` with in-memory cookie jar (no persistence).
- `User-Agent: pixsurf/0.2 (+https://github.com/MhmdShd/pixsurf)`.
- Follows redirects (stdlib limit, 10). Page request timeout 15s.
- Response body capped at 5MB (truncate beyond, note in status).
- Charset: decode via `golang.org/x/net/html/charset` (handles meta charset
  and Content-Type).
- `Get(url) (body io.Reader, finalURL string, err error)` — finalURL after
  redirects becomes the base for relative resolution.
- Image fetching: same client; per-image cap 2MB, per-image timeout 5s,
  total image budget per page 10s. `--no-images` flag skips all image fetches.

### dom/ — parsing
- `golang.org/x/net/html` parses (tolerant of broken HTML, handles entities).
- Thin wrapper exposing the node tree plus helpers (attr lookup, id index for
  fragment jumps, `<base href>` extraction).

### style/ — semantic default styles (no CSS engine in v2)
Per-tag defaults:
- `h1`–`h6`: bold (h1/h2 additionally underlined)
- `a`: blue + underline; visited state not tracked
- `b`/`strong`: bold; `i`/`em`: italic; `u`: underline; `s`/`del`: strikethrough
- `code`/`kbd`: reverse video; `pre`: verbatim block, reverse off
- `blockquote`: 2-cell indent + dim
- `li`: bullet `• ` (ordered lists numbered)
- `hr`: full-width line of `─`
Inline overrides honored: `style="color: …"`, `style="background-color: …"`
(named colors + #hex), and legacy `bgcolor`/`color` attributes.
Hidden entirely: `script`, `style`, `head`, `noscript`, `template`, comments.
Unknown tags: render children with inherited style.

### layout/ — flow layout to terminal width
- Input: DOM + styles + content width (terminal cols). Output: `Document`.
- `Document`: slice of `Line`s (styled spans), `Links` (per-line column ranges
  → URL), `Anchors` (id → line index), `Images` (line slot → image ref).
- Block/inline model: block tags break lines; inline text word-wraps at
  content width. Rune display width via `go-runewidth` (CJK/emoji = 2 cells).
- `<pre>`: no wrap (long lines clipped at width in v2).
- Tables: simple column split across width; `colspan` collapsed; nested
  tables flattened to sequential blocks.
- `<img>`: decoded (png/jpeg/gif stdlib + webp via `x/image/webp`; svg
  skipped), downscaled with existing `render.ToCells` to fit content width
  (max height 15 rows), embedded as pixel lines. `alt` text shown when image
  missing/failed/disabled.
- `<iframe>`/`<frame>`: rendered as a link to their `src`.

### render/ — kept as-is
Image → half-block cell math (`ToCells`). `CellToPage` becomes unused by the
app (kept in package; harmless pure function).

### ui/ — generalized cells
- Cell becomes `{Rune rune, Fg, Bg RGB, Bold, Italic, Underline, Strike,
  Reverse, Dim bool}`; pixel cells are just `▀` with Fg/Bg set.
- Draw(grid, status) unchanged in spirit; events unchanged.
- ui stays engine-ignorant.

### main.go — app loop
- Navigation: fetch → parse → layout → store `Document` + scroll offset.
- Scroll: local offset into Document lines. Instant. Scroll debounce deleted.
- Click: hit-test `Links` at (line=offset+y, col=x) → navigate (resolve
  relative against page base URL).
- Special links: `#frag` → jump to `Anchors[frag]` if present else no-op with
  status note; `mailto:`/`javascript:` → status bar message, no navigation.
- History: back/forward as in-memory stack of URLs (re-fetch on revisit).
- Resize: re-run layout at new width; restore scroll by line ratio.
- Reload: re-fetch current URL.
- URL bar (`g`), keys, status bar: unchanged from v1.

## Errors
- Fetch/HTTP errors, timeouts, non-HTML content types → status bar message,
  previous page stays.
- Broken/oversized images → alt text placeholder, page still renders.
- Truncated page (5MB cap) → status bar note.

## Removal ripple
- Delete `browser/`; drop chromedp from go.mod.
- README rewritten: "no browser install needed" is the headline; Chrome
  requirement section deleted; `--no-images` documented.
- CI unchanged. v1 spec/plan docs remain for history.

## Testing
- layout/: table-driven tests — fixed HTML in, expected styled lines out
  (wrap, wide runes, lists, pre, links ranges, anchors).
- style/: tag → style mapping, inline color parsing.
- fetch/: `httptest` server — redirects, charset, caps, cookies, UA.
- dom/: base-href + id-index helpers.
- Smoke: real sites (example.com, wikipedia article) render non-empty
  documents with links.

## Out of scope (future)
- JavaScript, full CSS engine, forms, async image loading, sixel/kitty,
  scrollable `<pre>`, frames content inlining, visited-link tracking.

## Ship
Version v0.2.0. GoReleaser prebuilt binaries follow immediately after
(separate task, already approved).
