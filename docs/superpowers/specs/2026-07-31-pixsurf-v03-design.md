# pixsurf v0.3 — Search Forms, Async Images, Cross-Page Anchors

Date: 2026-07-31
Builds on `2026-07-31-pixsurf-v2-engine-design.md`.

## Scope

1. GET search forms (text/search inputs + submit).
2. Async image loading (instant first paint, images fill in).
3. Cross-page anchor jumps (`otherpage#section`).

Out of scope: POST forms, checkboxes/radios/selects/textareas, logins,
anchor-stable scroll during image arrival, sixel/kitty.

## 1. Forms

### Data model (layout package)

```go
type Field struct {
    Name    string
    Value   string // current text (prefilled from value attr or values map)
    Line    int    // box region for hit-testing
    Start   int
    End     int
    FormIdx int
}

type Form struct {
    Action string // resolved absolute URL; defaults to page URL when no action attr
    Method string // "get" or "post" (lowercased); only get is submittable
    Hidden url.Values // name→value pairs from <input type=hidden>
    SubmitLine, SubmitStart, SubmitEnd int // -1s when no visible submit button
}

// Document gains:
Forms  []Form
Fields []Field
// plus lookups:
func (d *Document) FieldAt(line, col int) (int, bool)  // index into Fields
func (d *Document) SubmitAt(line, col int) (int, bool) // index into Forms
```

`layout.Render` gains a values parameter: `Render(d *dom.Doc, width int,
images ImageFetcher, values FormValues)` where `FormValues =
map[string]string` keyed `action + "\x00" + fieldName`. Values survive
re-layout because main owns the map.

### Rendering

- `<input type=text|search>` (and type absent): box `[value_____]`,
  width = min(size attr, 30, content width − 2), default 20; value clipped to
  show the tail when longer. Reverse-video style so the box is visible.
- `<input type=submit value=X>` / `<button>text</button>`: `[ X ]` (default
  label "Submit"), bold. Recorded as the form's submit region (first wins).
- `<input type=hidden>`: not rendered; name/value into `Form.Hidden`.
- Inputs with no `name`: rendered but excluded from submission.
- Other input types (checkbox/radio/password/etc.): not rendered in v0.3.
- Forms with method POST: fields render, submit shows status error.

### Interaction (main + ui)

- ui gains a generalized prompt: `Prompt(label, initial string)` starts input
  mode with a label and pre-filled text; Enter emits new `InputEvent{Text}`,
  Esc emits `InputCancelEvent{}`. The URL bar (`g`) becomes
  `Prompt("URL: ", "")` and its Enter maps to the existing URLEvent semantics
  at the main layer (main tracks what the prompt was for).
- Click on a field box → `Prompt("<name>: ", currentValue)`; on submit-Enter,
  main stores the value in FormValues, re-renders (cheap, local — page DOM is
  cached, see Async), and immediately submits that field's form? No —
  Enter in a field stores the value AND submits the form (search-box
  behavior). Esc stores nothing.
- Click on `[ Submit ]` region → submit that form with current FormValues.
- Submit (GET only): `url = action stripped of query + "?" +
  urlencode(Hidden merged with named field values)`; navigate normally
  (history push). POST → status "unsupported form (POST)".

### DOM caching

To make field editing + re-layout cheap and to support async images, main
caches the parsed `*dom.Doc` of the current page. Re-layout = layout.Render
on the cached DOM (no re-fetch). Resize also uses the cache now (removes the
v2 re-fetch-on-resize behavior).

## 2. Async images

- `layout.Render` is called with an ImageFetcher that only hits an in-memory
  cache: hit → image, miss → records the URL in a wanted-set and returns
  error (placeholder rendered).
- After each render, main hands the wanted-set to a fetch pool (4 worker
  goroutines, per-image caps from fetch package). Each completed fetch stores
  into the cache and signals an arrivals channel.
- Main's event loop is a select over ui events, arrivals, and a 200ms
  debounce timer: arrivals mark dirty; timer fires → re-layout from cached
  DOM + redraw, preserving scroll offset (small jumps when images land above
  the viewport are accepted, documented).
- Page generation counter: incremented on navigate; workers carry the
  generation they were spawned for; stale arrivals are dropped and their
  in-flight requests canceled via context on navigate.
- Redraws are deferred while the status-bar prompt is active (typing);
  applied when the prompt closes.
- Image cache is per-page (cleared on navigate) to bound memory; capped at
  50 images per page — beyond that, placeholders stay.
- `--no-images`: no fetcher, no workers, placeholders only (unchanged).

## 3. Cross-page anchors

- `click`: for a non-same-page link with a fragment, split URL: navigate to
  the base part, then after successful load jump to `Anchors[frag]` (offset
  = line, clamped); missing anchor → status note. Fragment survives
  redirects (applied after final load).
- Same-page fragment behavior unchanged from v2.

## Errors

- Form with unparseable action → status error on submit.
- Image worker errors → placeholder stays; no status noise.
- All existing v2 error behavior unchanged.

## Testing

- layout: field/submit region extraction + hit-tests; hidden collection;
  box rendering with value clipping; values map prefill; POST flag.
- main-level pure helpers unit-tested: submit URL building (query replace,
  encoding, hidden merge), generation filtering logic if factored pure.
- Async: cache-backed fetcher (miss records + placeholder, hit renders)
  tested with fake images; debounce/generation logic covered via helper
  tests where pure.
- Smoke: DuckDuckGo (`html.duckduckgo.com/html`) search flow — render form,
  fill query, build submit URL, fetch results, assert result links present.
  Wikipedia page with images: first render has placeholders, cache-warmed
  second render has pixel lines.

## Ship

v0.3.0 tag → GoReleaser binaries. README: controls unchanged; add "site
search supported" line and async-images note.
