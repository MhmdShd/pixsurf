# pixsurf v0.7 — CSS Cascade

Date: 2026-07-31

## Problem

pixsurf ignores CSS entirely. Styling comes from hard-coded per-tag defaults
plus the `color`/`background-color` inline properties. Measured: Wikipedia's
Cat article ships 21 `<style>` blocks, 2 external sheets and 947 inline
`style` attributes; google.com hides 12 elements with `display:none` that we
render anyway. Pages therefore look like pixsurf, not like themselves.

## Goal

Resolve real computed styles from author CSS and apply everything a terminal
can express. Colours and typography should match the site; geometry cannot
and will not.

### In scope (terminal-expressible)
- `color`, `background-color`, `background` (colour component only)
- `font-weight` (bold when >= 600 or `bold`/`bolder`)
- `font-style` (`italic`/`oblique`)
- `text-decoration` / `text-decoration-line` (`underline`, `line-through`,
  `none`)
- `display: none` and `visibility: hidden` → element and subtree skipped
- `text-align` (`left`/`center`/`right`)
- `text-transform` (`uppercase`/`lowercase`/`capitalize`)
- `white-space` (`pre`, `pre-wrap` → verbatim like `<pre>`)

### Explicitly out of scope
Box model, flexbox, grid, floats, positioning, fonts, sizes, borders,
shadows, transitions, media queries beyond `screen`/`all`, pseudo-elements
that generate content, `@import`, and JavaScript-applied styles. A terminal
is a character grid; geometry does not survive the translation.

## Architecture

New package `css`, consumed by `layout` through a narrow interface so the
layout engine keeps working unchanged when no CSS is present.

### Interface (layout depends only on this)

```go
// StyleResolver supplies computed styles. layout.Render accepts nil, in
// which case it falls back to style.ForTag + style.ApplyInline as today.
type StyleResolver interface {
    // Hidden reports display:none / visibility:hidden for n's subtree.
    Hidden(n *dom.Node) bool
    // Resolve returns n's computed style given its parent's.
    Resolve(n *dom.Node, parent style.Style) style.Style
}
```

`style.Style` gains `Align` (none/left/center/right), `Transform`
(none/upper/lower/capitalize) and `Pre bool`.

### css package

- `Rule{Selector cascadia.Selector, Key string, Specificity [3]int, Order int,
  Decls map[string]Value}`; `Value{Text string, Important bool}`.
- `ParseSheet(text string, order int) []Rule` — tokenises with
  `github.com/tdewolff/parse/v2/css`, compiles selectors with
  `github.com/andybalholm/cascadia`, computes specificity, drops rules whose
  selectors fail to compile (pseudo-elements etc.) rather than erroring.
- `Engine{rules []Rule, index map[string][]int, cache map[*dom.Node]style.Style}`.
  The index keys rules by the rightmost simple selector (tag, `.class`, `#id`)
  so each node tests only candidate rules — Wikipedia has thousands of rules
  and a naive all-rules-per-node scan is quadratic.
- Cascade order, lowest to highest: UA defaults (`style.ForTag`), author
  rules sorted by (specificity, source order), inline `style` attribute, then
  any `!important` declarations in the same relative order.
- Inheritance: `color`, `font-*`, `text-align`, `text-transform`,
  `visibility`, `white-space` inherit from the parent's computed style;
  `background-color` and `display` do not.
- Results memoised per node.

### Stylesheet loading

- `fetch.Client.Asset(url) (string, error)` — same caps as pages but 512KB
  and 5s; used for `<link rel="stylesheet">`.
- main collects, in document order: every `<style>` element's text, then each
  `<link rel=stylesheet>` fetched (max 4 sheets, 8s total budget; failures
  are skipped silently and the page still renders).
- Sheets load before first layout. They are small and cached by the origin;
  blocking here is what browsers do and keeps first paint correct.
- `--no-css` flag disables the whole path for debugging and for users who
  prefer pixsurf's own defaults.

## Errors
Malformed CSS is skipped rule-by-rule. A failed stylesheet fetch is ignored.
Neither ever blocks page rendering.

## Testing
- css: specificity ordering, inline-over-sheet, `!important`, inheritance vs
  non-inheritance, index correctness, malformed input, each supported
  property mapping to the right `style.Style` field.
- layout: `display:none` subtree skipped; `text-align: center` centres a line
  within the content width; `text-transform: uppercase` uppercases emitted
  text; resolver == nil reproduces today's output exactly.
- Real pages: google.com search box gets its CSS background rather than our
  invented reverse-video; HN link colours come from its stylesheet;
  Wikipedia keeps its line/link counts and gains correct hiding.

## Ship
v0.7.0 once real-page checks pass.
