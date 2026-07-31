# pixsurf v2 Engine Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers-extended-cc:subagent-driven-development (recommended) or superpowers-extended-cc:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace headless Chrome with a pure-Go fetch → parse → style → layout pipeline; pixsurf runs anywhere with zero external programs.

**Architecture:** New leaf package `cell` (display cells) shared by `ui` and `layout`. Pipeline packages `fetch` (HTTP), `dom` (x/net/html wrapper), `style` (semantic tag styles), `layout` (flow layout → Document of styled lines + link/anchor/image metadata). `render` is kept for image→half-block math. `browser/` (chromedp) is deleted. `main.go` becomes a local event loop: scroll = offset into Document lines.

**Tech Stack:** Go, `golang.org/x/net/html` (+`/charset`), `golang.org/x/image/webp`, `github.com/mattn/go-runewidth`, existing tcell.

Spec: `docs/superpowers/specs/2026-07-31-pixsurf-v2-engine-design.md`

**Note on reference code:** Interfaces, types, and test files in this plan are FIXED — implementers must not change them. Function bodies marked "reference implementation" may be refined (clearer/safer equivalents) as long as all tests pass and the public API is untouched.

---

### Task 1: cell package + ui generalization

**Goal:** Shared display-cell type; ui draws arbitrary styled text, not only pixel cells.

**Files:**
- Create: `cell/cell.go`
- Modify: `ui/ui.go` (Draw + imports only; events/API otherwise unchanged)

**Acceptance Criteria:**
- [ ] `cell.Cell` carries rune, optional fg/bg RGB, and Bold/Italic/Underline/Strike/Reverse/Dim flags
- [ ] `ui.Draw(view [][]cell.Cell, status string)` renders runes with styles; unset fg/bg = terminal default
- [ ] `go build ./... && go vet ./...` exit 0

**Verify:** `go build ./... && go vet ./...` → exit 0

**Steps:**

- [ ] **Step 1: Create `cell/cell.go`**

```go
// Package cell defines the terminal display cell shared by layout and ui.
package cell

// RGB is an 8-bit color.
type RGB struct{ R, G, B uint8 }

// Cell is one terminal character cell with optional colors and attributes.
// HasFg/HasBg false means "terminal default color".
type Cell struct {
	Rune         rune
	Fg, Bg       RGB
	HasFg, HasBg bool
	Bold, Italic, Underline, Strike, Reverse, Dim bool
}
```

- [ ] **Step 2: Update `ui/ui.go`**

Replace the `render` import with `cell`, change `Draw(cells [][]render.Cell, status string)` to `Draw(view [][]cell.Cell, status string)`, and replace the cell-drawing loop body with:

```go
for y, row := range view {
	for x, c := range row {
		st := tcell.StyleDefault
		if c.HasFg {
			st = st.Foreground(tcell.NewRGBColor(int32(c.Fg.R), int32(c.Fg.G), int32(c.Fg.B)))
		}
		if c.HasBg {
			st = st.Background(tcell.NewRGBColor(int32(c.Bg.R), int32(c.Bg.G), int32(c.Bg.B)))
		}
		st = st.Bold(c.Bold).Italic(c.Italic).Underline(c.Underline).
			StrikeThrough(c.Strike).Reverse(c.Reverse).Dim(c.Dim)
		r := c.Rune
		if r == 0 {
			r = ' '
		}
		u.screen.SetContent(x, y, r, nil, st)
	}
}
```

`main.go` still references the old signature — expected to break the build until Task 7. To keep the module compiling for tests, temporarily adjust `main.go`'s `refresh()`/Draw call sites minimally (convert `render.Cell` grid to `cell.Cell` grid with `Rune: '▀', HasFg: true, HasBg: true`) — this shim is deleted in Task 7.

- [ ] **Step 3: Verify + commit**

Run: `go build ./... && go vet ./... && go test ./...` → exit 0

```bash
git add cell/ ui/ main.go
git commit -m "feat: shared cell package; ui draws styled text cells"
```

---

### Task 2: fetch package (TDD)

**Goal:** HTTP client with cookies, UA, caps, charset decoding.

**Files:**
- Create: `fetch/fetch.go`
- Test: `fetch/fetch_test.go`

**Acceptance Criteria:**
- [ ] Sends `User-Agent: pixsurf/0.2 (+https://github.com/MhmdShd/pixsurf)`
- [ ] Follows redirects; returns final URL
- [ ] Page body capped at 5MB with truncated=true
- [ ] Non-UTF8 pages (e.g. ISO-8859-1 via Content-Type) decoded to UTF-8
- [ ] Cookies persist across requests within a session
- [ ] `Image` rejects bodies over 2MB and respects 5s timeout

**Verify:** `go test ./fetch/ -v` → all PASS

**Steps:**

- [ ] **Step 1: Write failing tests `fetch/fetch_test.go`**

```go
package fetch

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestUserAgentAndRedirectAndFinalURL(t *testing.T) {
	var gotUA string
	mux := http.NewServeMux()
	mux.HandleFunc("/start", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/end", http.StatusFound)
	})
	mux.HandleFunc("/end", func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		fmt.Fprint(w, "<html><body>hi</body></html>")
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := New()
	body, finalURL, truncated, err := c.Page(srv.URL + "/start")
	if err != nil {
		t.Fatal(err)
	}
	if truncated {
		t.Error("unexpected truncation")
	}
	if !strings.Contains(body, "hi") {
		t.Errorf("body = %q", body)
	}
	if finalURL != srv.URL+"/end" {
		t.Errorf("finalURL = %q, want %q", finalURL, srv.URL+"/end")
	}
	if !strings.HasPrefix(gotUA, "pixsurf/") {
		t.Errorf("User-Agent = %q", gotUA)
	}
}

func TestPageCap(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(make([]byte, 6<<20)) // 6MB of zero bytes
	}))
	defer srv.Close()
	c := New()
	body, _, truncated, err := c.Page(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if !truncated {
		t.Error("want truncated=true for 6MB body")
	}
	if len(body) > maxPageBytes {
		t.Errorf("body len %d exceeds cap %d", len(body), maxPageBytes)
	}
}

func TestCharsetDecoding(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=iso-8859-1")
		w.Write([]byte{'c', 'a', 'f', 0xE9}) // "café" in latin-1
	}))
	defer srv.Close()
	c := New()
	body, _, _, err := c.Page(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(body, "café") {
		t.Errorf("body = %q, want café decoded", body)
	}
}

func TestCookiesPersist(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/set", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "sid", Value: "42"})
		fmt.Fprint(w, "ok")
	})
	mux.HandleFunc("/check", func(w http.ResponseWriter, r *http.Request) {
		if c, err := r.Cookie("sid"); err == nil && c.Value == "42" {
			fmt.Fprint(w, "have-cookie")
		} else {
			fmt.Fprint(w, "no-cookie")
		}
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	c := New()
	if _, _, _, err := c.Page(srv.URL + "/set"); err != nil {
		t.Fatal(err)
	}
	body, _, _, err := c.Page(srv.URL + "/check")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(body, "have-cookie") {
		t.Errorf("cookie not persisted, body = %q", body)
	}
}

func TestImageCap(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(make([]byte, 3<<20)) // 3MB, over the 2MB image cap
	}))
	defer srv.Close()
	c := New()
	if _, err := c.Image(srv.URL); err == nil {
		t.Error("want error for 3MB image, got nil")
	}
}
```

- [ ] **Step 2: Run, confirm FAIL** (`undefined: New` etc.)

- [ ] **Step 3: Implement `fetch/fetch.go`** (reference implementation)

```go
// Package fetch retrieves pages and images over HTTP with safety caps.
package fetch

import (
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"net/http/cookiejar"
	"time"

	"golang.org/x/net/html/charset"
	_ "golang.org/x/image/webp"
)

const (
	userAgent    = "pixsurf/0.2 (+https://github.com/MhmdShd/pixsurf)"
	pageTimeout  = 15 * time.Second
	imageTimeout = 5 * time.Second
	maxPageBytes  = 5 << 20
	maxImageBytes = 2 << 20
)

// Client is an HTTP session with an in-memory cookie jar.
type Client struct {
	hc    *http.Client
	imgHC *http.Client
}

// New creates a Client. Cookies live for the process lifetime only.
func New() *Client {
	jar, _ := cookiejar.New(nil)
	return &Client{
		hc:    &http.Client{Jar: jar, Timeout: pageTimeout},
		imgHC: &http.Client{Jar: jar, Timeout: imageTimeout},
	}
}

func (c *Client) get(hc *http.Client, rawURL string) (*http.Response, error) {
	req, err := http.NewRequest("GET", rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := hc.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		resp.Body.Close()
		return nil, fmt.Errorf("HTTP %s", resp.Status)
	}
	return resp, nil
}

// Page fetches a page, decodes it to UTF-8, and caps it at 5MB.
// finalURL is the URL after redirects (base for relative resolution).
func (c *Client) Page(rawURL string) (body, finalURL string, truncated bool, err error) {
	resp, err := c.get(c.hc, rawURL)
	if err != nil {
		return "", "", false, err
	}
	defer resp.Body.Close()

	utf8Reader, err := charset.NewReader(resp.Body, resp.Header.Get("Content-Type"))
	if err != nil {
		return "", "", false, err
	}
	data, err := io.ReadAll(io.LimitReader(utf8Reader, maxPageBytes+1))
	if err != nil {
		return "", "", false, err
	}
	if len(data) > maxPageBytes {
		data = data[:maxPageBytes]
		truncated = true
	}
	return string(data), resp.Request.URL.String(), truncated, nil
}

// Image fetches and decodes an image (png/jpeg/gif/webp), capped at 2MB.
func (c *Client) Image(rawURL string) (image.Image, error) {
	resp, err := c.get(c.imgHC, rawURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxImageBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxImageBytes {
		return nil, fmt.Errorf("image exceeds %d bytes", maxImageBytes)
	}
	img, _, err := image.Decode(bytesReader(data))
	return img, err
}
```

(`bytesReader` = `bytes.NewReader`; import `bytes` and use it directly.)

- [ ] **Step 4:** `go mod tidy && go test ./fetch/ -v` → all PASS

- [ ] **Step 5: Commit**

```bash
git add fetch/ go.mod go.sum
git commit -m "feat: fetch package — capped HTTP client with cookies and charset decoding"
```

---

### Task 3: dom package (TDD)

**Goal:** Parse HTML; resolve relative URLs; index ids.

**Files:**
- Create: `dom/dom.go`
- Test: `dom/dom_test.go`

**Acceptance Criteria:**
- [ ] `Parse` builds tree from broken HTML without error
- [ ] `Resolve` handles absolute, relative, and `<base href>` cases
- [ ] `Attr` returns attribute values case-insensitively

**Verify:** `go test ./dom/ -v` → all PASS

**Steps:**

- [ ] **Step 1: Write failing tests `dom/dom_test.go`**

```go
package dom

import (
	"strings"
	"testing"
)

func TestParseAndResolve(t *testing.T) {
	d, err := Parse("<p><a href=/wiki/Go>go</a>", "https://en.example.org/wiki/Home")
	if err != nil {
		t.Fatal(err)
	}
	if got := d.Resolve("/wiki/Go"); got != "https://en.example.org/wiki/Go" {
		t.Errorf("Resolve = %q", got)
	}
	if got := d.Resolve("Sub"); got != "https://en.example.org/wiki/Sub" {
		t.Errorf("relative Resolve = %q", got)
	}
	if got := d.Resolve("https://other.org/x"); got != "https://other.org/x" {
		t.Errorf("absolute Resolve = %q", got)
	}
}

func TestBaseHref(t *testing.T) {
	d, err := Parse(`<head><base href="https://cdn.example.org/app/"></head><body><a href=img>x</a></body>`, "https://example.org/page")
	if err != nil {
		t.Fatal(err)
	}
	if got := d.Resolve("img"); got != "https://cdn.example.org/app/img" {
		t.Errorf("Resolve with base = %q", got)
	}
}

func TestAttr(t *testing.T) {
	d, err := Parse(`<img SRC="a.png" alt="pic">`, "https://example.org/")
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	Walk(d.Root, func(n *Node) {
		if n.Type == ElementNode && n.Data == "img" {
			found = true
			if Attr(n, "src") != "a.png" {
				t.Errorf("Attr src = %q", Attr(n, "src"))
			}
		}
	})
	if !found {
		t.Error("img node not found")
	}
	_ = strings.TrimSpace // keep strings import
}
```

- [ ] **Step 2: Run, confirm FAIL**

- [ ] **Step 3: Implement `dom/dom.go`** (reference implementation)

```go
// Package dom parses HTML and resolves URLs against the page base.
package dom

import (
	"net/url"
	"strings"

	"golang.org/x/net/html"
)

// Node and ElementNode alias x/net/html types so callers avoid the import.
type Node = html.Node

const ElementNode = html.ElementNode
const TextNode = html.TextNode

// Doc is a parsed page.
type Doc struct {
	Root *Node
	Base *url.URL
}

// Parse parses src (already UTF-8) with pageURL as the default base.
// A <base href> tag overrides the base.
func Parse(src, pageURL string) (*Doc, error) {
	root, err := html.Parse(strings.NewReader(src))
	if err != nil {
		return nil, err
	}
	base, err := url.Parse(pageURL)
	if err != nil {
		return nil, err
	}
	Walk(root, func(n *Node) {
		if n.Type == ElementNode && n.Data == "base" {
			if href := Attr(n, "href"); href != "" {
				if u, err := base.Parse(href); err == nil {
					base = u
				}
			}
		}
	})
	return &Doc{Root: root, Base: base}, nil
}

// Resolve turns a possibly-relative href into an absolute URL string.
func (d *Doc) Resolve(href string) string {
	u, err := d.Base.Parse(strings.TrimSpace(href))
	if err != nil {
		return href
	}
	return u.String()
}

// Attr returns the value of the named attribute (case-insensitive), or "".
func Attr(n *Node, key string) string {
	for _, a := range n.Attr {
		if strings.EqualFold(a.Key, key) {
			return a.Val
		}
	}
	return ""
}

// Walk calls fn for every node in depth-first order.
func Walk(n *Node, fn func(*Node)) {
	fn(n)
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		Walk(c, fn)
	}
}
```

- [ ] **Step 4:** `go test ./dom/ -v` → all PASS

- [ ] **Step 5: Commit**

```bash
git add dom/
git commit -m "feat: dom package — HTML parsing, base URL resolution"
```

---

### Task 4: style package (TDD)

**Goal:** Semantic per-tag styles + inline color parsing.

**Files:**
- Create: `style/style.go`
- Test: `style/style_test.go`

**Acceptance Criteria:**
- [ ] Tag defaults per spec (headings bold, links blue+underline, code reverse, blockquote dim, etc.)
- [ ] `ParseColor` handles `#rgb`, `#rrggbb`, and the 16 basic CSS named colors
- [ ] `style="color: red; background-color: #002b36"` and legacy `color`/`bgcolor` attrs applied
- [ ] `Hidden` true for script/style/head/noscript/template

**Verify:** `go test ./style/ -v` → all PASS

**Steps:**

- [ ] **Step 1: Write failing tests `style/style_test.go`**

```go
package style

import (
	"strings"
	"testing"

	"golang.org/x/net/html"

	"github.com/MhmdShd/pixsurf/cell"
)

func node(t *testing.T, src, tag string) *html.Node {
	t.Helper()
	root, err := html.Parse(strings.NewReader(src))
	if err != nil {
		t.Fatal(err)
	}
	var found *html.Node
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == tag {
			found = n
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(root)
	if found == nil {
		t.Fatalf("tag %q not found", tag)
	}
	return found
}

func TestTagDefaults(t *testing.T) {
	base := Style{}
	if s := ForTag("h1", base); !s.Bold || !s.Underline {
		t.Errorf("h1 = %+v, want bold+underline", s)
	}
	if s := ForTag("a", base); !s.Underline || !s.HasFg {
		t.Errorf("a = %+v, want underline + fg color", s)
	}
	if s := ForTag("code", base); !s.Reverse {
		t.Errorf("code = %+v, want reverse", s)
	}
	if s := ForTag("blockquote", base); !s.Dim {
		t.Errorf("blockquote = %+v, want dim", s)
	}
	if s := ForTag("em", base); !s.Italic {
		t.Errorf("em = %+v, want italic", s)
	}
	// inheritance: bold parent stays bold in unknown child
	bold := Style{Bold: true}
	if s := ForTag("span", bold); !s.Bold {
		t.Errorf("span under bold = %+v, want bold inherited", s)
	}
}

func TestParseColor(t *testing.T) {
	cases := map[string]cell.RGB{
		"#fff":    {255, 255, 255},
		"#002b36": {0, 43, 54},
		"red":     {255, 0, 0},
		"navy":    {0, 0, 128},
	}
	for in, want := range cases {
		got, ok := ParseColor(in)
		if !ok || got != want {
			t.Errorf("ParseColor(%q) = %v,%v want %v", in, got, ok, want)
		}
	}
	if _, ok := ParseColor("notacolor"); ok {
		t.Error("ParseColor accepted garbage")
	}
}

func TestApplyInline(t *testing.T) {
	n := node(t, `<p style="color: red; background-color: #002b36">x</p>`, "p")
	s := ApplyInline(Style{}, n)
	if !s.HasFg || s.Fg != (cell.RGB{255, 0, 0}) {
		t.Errorf("fg = %+v", s)
	}
	if !s.HasBg || s.Bg != (cell.RGB{0, 43, 54}) {
		t.Errorf("bg = %+v", s)
	}
	legacy := node(t, `<font color="navy">x</font>`, "font")
	s2 := ApplyInline(Style{}, legacy)
	if !s2.HasFg || s2.Fg != (cell.RGB{0, 0, 128}) {
		t.Errorf("legacy color = %+v", s2)
	}
}

func TestHidden(t *testing.T) {
	for _, tag := range []string{"script", "style", "head", "noscript", "template"} {
		if !Hidden(tag) {
			t.Errorf("Hidden(%q) = false", tag)
		}
	}
	if Hidden("p") {
		t.Error("Hidden(p) = true")
	}
}
```

- [ ] **Step 2: Run, confirm FAIL**

- [ ] **Step 3: Implement `style/style.go`** (reference implementation)

```go
// Package style maps HTML tags and inline attributes to display styles.
package style

import (
	"strings"

	"golang.org/x/net/html"

	"github.com/MhmdShd/pixsurf/cell"
)

// Style is the resolved display style for a DOM subtree.
type Style struct {
	Fg, Bg       cell.RGB
	HasFg, HasBg bool
	Bold, Italic, Underline, Strike, Reverse, Dim bool
}

var linkColor = cell.RGB{R: 95, G: 175, B: 255}

// ForTag returns parent adjusted by tag defaults (inheritance preserved).
func ForTag(tag string, parent Style) Style {
	s := parent
	switch tag {
	case "h1", "h2":
		s.Bold, s.Underline = true, true
	case "h3", "h4", "h5", "h6":
		s.Bold = true
	case "a":
		s.Underline, s.HasFg, s.Fg = true, true, linkColor
	case "b", "strong", "th":
		s.Bold = true
	case "i", "em":
		s.Italic = true
	case "u":
		s.Underline = true
	case "s", "del", "strike":
		s.Strike = true
	case "code", "kbd":
		s.Reverse = true
	case "pre":
		s.Reverse = false
	case "blockquote":
		s.Dim = true
	}
	return s
}

var hidden = map[string]bool{
	"script": true, "style": true, "head": true,
	"noscript": true, "template": true, "title": true,
}

// Hidden reports whether a tag's subtree renders nothing.
func Hidden(tag string) bool { return hidden[tag] }

var named = map[string]cell.RGB{
	"black": {0, 0, 0}, "silver": {192, 192, 192}, "gray": {128, 128, 128},
	"white": {255, 255, 255}, "maroon": {128, 0, 0}, "red": {255, 0, 0},
	"purple": {128, 0, 128}, "fuchsia": {255, 0, 255}, "green": {0, 128, 0},
	"lime": {0, 255, 0}, "olive": {128, 128, 0}, "yellow": {255, 255, 0},
	"navy": {0, 0, 128}, "blue": {0, 0, 255}, "teal": {0, 128, 128},
	"aqua": {0, 255, 255},
}

// ParseColor parses #rgb, #rrggbb, or a basic named color.
func ParseColor(v string) (cell.RGB, bool) {
	v = strings.ToLower(strings.TrimSpace(v))
	if c, ok := named[v]; ok {
		return c, true
	}
	if strings.HasPrefix(v, "#") {
		hex := v[1:]
		if len(hex) == 3 {
			hex = string([]byte{hex[0], hex[0], hex[1], hex[1], hex[2], hex[2]})
		}
		if len(hex) == 6 {
			var out [3]uint8
			for i := 0; i < 3; i++ {
				n, ok := hexByte(hex[i*2], hex[i*2+1])
				if !ok {
					return cell.RGB{}, false
				}
				out[i] = n
			}
			return cell.RGB{R: out[0], G: out[1], B: out[2]}, true
		}
	}
	return cell.RGB{}, false
}

func hexByte(a, b byte) (uint8, bool) {
	hi, ok1 := hexNibble(a)
	lo, ok2 := hexNibble(b)
	return hi<<4 | lo, ok1 && ok2
}

func hexNibble(c byte) (uint8, bool) {
	switch {
	case c >= '0' && c <= '9':
		return c - '0', true
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10, true
	}
	return 0, false
}

// ApplyInline overlays style="" declarations and legacy color/bgcolor attrs.
func ApplyInline(s Style, n *html.Node) Style {
	attr := func(key string) string {
		for _, a := range n.Attr {
			if strings.EqualFold(a.Key, key) {
				return a.Val
			}
		}
		return ""
	}
	if c, ok := ParseColor(attr("color")); ok {
		s.HasFg, s.Fg = true, c
	}
	if c, ok := ParseColor(attr("bgcolor")); ok {
		s.HasBg, s.Bg = true, c
	}
	for _, decl := range strings.Split(attr("style"), ";") {
		k, v, ok := strings.Cut(decl, ":")
		if !ok {
			continue
		}
		switch strings.TrimSpace(strings.ToLower(k)) {
		case "color":
			if c, ok := ParseColor(v); ok {
				s.HasFg, s.Fg = true, c
			}
		case "background-color", "background":
			if c, ok := ParseColor(v); ok {
				s.HasBg, s.Bg = true, c
			}
		}
	}
	return s
}
```

- [ ] **Step 4:** `go test ./style/ -v` → all PASS

- [ ] **Step 5: Commit**

```bash
git add style/
git commit -m "feat: style package — semantic tag styles and inline colors"
```

---

### Task 5: layout package — flow core (TDD)

**Goal:** DOM → Document: wrapped styled lines, links, anchors, lists, pre, hr.

**Files:**
- Create: `layout/layout.go`, `layout/inline.go`
- Test: `layout/layout_test.go`

**Acceptance Criteria:**
- [ ] Paragraph text word-wraps at width; words never split mid-word (unless longer than width)
- [ ] Wide runes (CJK) counted as 2 columns
- [ ] Block elements separated by exactly one blank line; consecutive blanks collapsed
- [ ] Links produce per-line ranges retrievable via `LinkAt`; multi-line links = multiple ranges
- [ ] `id` attributes recorded in `Anchors` with correct line numbers
- [ ] `<li>` bullets/numbers, nested list indent 2 per level
- [ ] `<pre>` lines verbatim (no wrap; clipped at width)
- [ ] `<hr>` full-width `─` line
- [ ] script/style/head content absent from output

**Verify:** `go test ./layout/ -v` → all PASS

**Steps:**

- [ ] **Step 1: Write failing tests `layout/layout_test.go`**

```go
package layout

import (
	"strings"
	"testing"

	"github.com/MhmdShd/pixsurf/dom"
)

func doc(t *testing.T, src string, width int) *Document {
	t.Helper()
	d, err := dom.Parse(src, "https://example.org/page")
	if err != nil {
		t.Fatal(err)
	}
	return Render(d, width, nil)
}

func lineText(d *Document, i int) string {
	if i >= len(d.Lines) {
		return ""
	}
	var b strings.Builder
	for _, c := range d.Lines[i] {
		if c.Rune != 0 {
			b.WriteRune(c.Rune)
		} else {
			b.WriteRune(' ')
		}
	}
	return strings.TrimRight(b.String(), " ")
}

func allText(d *Document) string {
	var parts []string
	for i := range d.Lines {
		parts = append(parts, lineText(d, i))
	}
	return strings.Join(parts, "\n")
}

func TestWordWrap(t *testing.T) {
	d := doc(t, "<p>alpha beta gamma delta</p>", 11)
	if got := lineText(d, 0); got != "alpha beta" {
		t.Errorf("line0 = %q", got)
	}
	if got := lineText(d, 1); got != "gamma delta" {
		t.Errorf("line1 = %q", got)
	}
}

func TestWideRunes(t *testing.T) {
	// each CJK char is 2 cols; width 4 fits two chars per line
	d := doc(t, "<p>日本語テ</p>", 4)
	if got := lineText(d, 0); got != "日本" {
		t.Errorf("line0 = %q", got)
	}
	if got := lineText(d, 1); got != "語テ" {
		t.Errorf("line1 = %q", got)
	}
}

func TestBlocksAndBlankCollapse(t *testing.T) {
	d := doc(t, "<h1>Title</h1><p>one</p><div><p>two</p></div>", 40)
	txt := allText(d)
	if strings.Contains(txt, "\n\n\n") {
		t.Errorf("multiple consecutive blank lines:\n%q", txt)
	}
	want := "Title\n\none\n\ntwo"
	if txt != want {
		t.Errorf("text = %q, want %q", txt, want)
	}
}

func TestLinksAndAnchors(t *testing.T) {
	d := doc(t, `<p id="top">go to <a href="/wiki/Go">the go page now</a></p>`, 14)
	if ln, ok := d.Anchors["top"]; !ok || ln != 0 {
		t.Errorf("anchor top = %d,%v", ln, ok)
	}
	// link text wraps across lines; both halves must resolve
	var hits int
	for line := 0; line < len(d.Lines); line++ {
		for col := 0; col < 14; col++ {
			if u, ok := d.LinkAt(line, col); ok {
				if u != "https://example.org/wiki/Go" {
					t.Fatalf("link url = %q", u)
				}
				hits++
			}
		}
	}
	if hits < len("the go page now") {
		t.Errorf("link coverage %d cols, want >= %d", hits, len("the go page now"))
	}
}

func TestLists(t *testing.T) {
	d := doc(t, "<ul><li>one</li><li>two<ul><li>sub</li></ul></li></ul><ol><li>first</li></ol>", 40)
	txt := allText(d)
	for _, want := range []string{"• one", "• two", "  • sub", "1. first"} {
		if !strings.Contains(txt, want) {
			t.Errorf("missing %q in:\n%s", want, txt)
		}
	}
}

func TestPreAndHr(t *testing.T) {
	d := doc(t, "<pre>a  b\n  indented</pre><hr>", 20)
	txt := allText(d)
	if !strings.Contains(txt, "a  b") || !strings.Contains(txt, "  indented") {
		t.Errorf("pre not verbatim:\n%s", txt)
	}
	if !strings.Contains(txt, strings.Repeat("─", 20)) {
		t.Errorf("hr missing:\n%s", txt)
	}
}

func TestHiddenElements(t *testing.T) {
	d := doc(t, "<head><title>T</title><style>p{}</style></head><body><script>var x=1;</script><p>visible</p></body>", 40)
	txt := allText(d)
	if strings.Contains(txt, "var x") || strings.Contains(txt, "p{}") || strings.Contains(txt, "T\n") {
		t.Errorf("hidden content leaked:\n%s", txt)
	}
	if !strings.Contains(txt, "visible") {
		t.Errorf("visible content missing:\n%s", txt)
	}
}
```

- [ ] **Step 2: Run, confirm FAIL**

- [ ] **Step 3: Implement.** Public API (FIXED):

```go
// Package layout flows a parsed page into terminal-width styled lines.
package layout

import (
	"image"

	"github.com/MhmdShd/pixsurf/cell"
	"github.com/MhmdShd/pixsurf/dom"
)

// Link is a clickable range on one line.
type Link struct {
	Line, Start, End int // End exclusive
	URL              string
}

// Document is a laid-out page.
type Document struct {
	Lines   [][]cell.Cell
	Links   []Link
	Anchors map[string]int
	Title   string
}

// ImageFetcher loads an image by absolute URL; nil disables images.
type ImageFetcher func(url string) (image.Image, error)

// Render lays out doc at the given content width.
func Render(d *dom.Doc, width int, images ImageFetcher) *Document

// LinkAt returns the URL covering (line, col), if any.
func (d *Document) LinkAt(line, col int) (string, bool)
```

Reference implementation structure (implementer writes bodies; tests are the contract):

- `walker` struct: current `Document`, line builder (`[]cell.Cell` + current column), current width, list depth / ordered counters stack, pre flag, pending-blank flag, current link URL (propagated to emitted cells' ranges via an open-range tracker).
- `emitText(text string, st style.Style)`: split on whitespace (unless pre), append words with `runewidth.RuneWidth` accounting; break line when word doesn't fit; hard-split words wider than width; record link ranges as cells are emitted while a link is open.
- `flushLine()`: close open link range at current column, append line, reset builder, reopen link range at col 0 (or list indent) if a link is still open.
- `block(n)`: `flushLine`, request blank separator (`pendingBlank=true`, collapsed naturally since it only materializes when the next non-empty line arrives), then walk children, then `flushLine` + request blank.
- Element dispatch in `renderNode(n *dom.Node, st style.Style)`:
  - hidden tags (`style.Hidden`) → return
  - `id` attr → `Anchors[id] = upcoming line index` (record before children)
  - text nodes → collapse whitespace (unless pre) → `emitText`
  - `br` → `flushLine`
  - `hr` → blank, line of `─` × width, blank
  - `p`, `div`, `section`, `article`, `header`, `footer`, `main`, `h1`–`h6`, `blockquote` (with indent), `table`, `tr` → block treatment
  - `ul`/`ol` → push list frame; `li` → `flushLine`, indent 2×(depth−1), bullet `• ` or `N. `, walk children
  - `pre` → block; text emitted line-by-line verbatim, clipped at width
  - `a` with `href` → resolve via `d.Resolve`, open link, walk children, close link
  - `img` → Task 6 (emit alt text placeholder for now)
  - `iframe`/`frame` → emit link to resolved `src` with text `[frame: <src>]`
  - default → walk children
- `blockquote` indent: prefix each flushed line with 2 spaces while depth > 0 (dim style comes from the style package).
- Title: first `<title>` text into `Document.Title` (not rendered as body).

- [ ] **Step 4:** `go mod tidy && go test ./layout/ -v` → all PASS (go-runewidth becomes a direct dependency)

- [ ] **Step 5: Commit**

```bash
git add layout/ go.mod go.sum
git commit -m "feat: layout package — flow layout with links, lists, anchors"
```

---

### Task 6: layout — tables and images (TDD)

**Goal:** Basic tables; inline images via render.ToCells.

**Files:**
- Modify: `layout/layout.go` (or new `layout/table.go`, `layout/image.go`)
- Test: append to `layout/layout_test.go`

**Acceptance Criteria:**
- [ ] Table cells in a row share the width equally, one row per `<tr>` (tall cells wrap within their column)
- [ ] Nested tables flattened (inner table rendered as sequential blocks)
- [ ] `<img>` with working fetcher → pixel lines (`▀` cells, HasFg+HasBg) scaled to fit width, max 15 rows
- [ ] `<img>` with nil fetcher or fetch error → `[alt text]` or `[image]` placeholder
- [ ] Images taller than 15 rows are capped at 15

**Verify:** `go test ./layout/ -v` → all PASS

**Steps:**

- [ ] **Step 1: Add failing tests**

```go
func TestTable(t *testing.T) {
	d := doc(t, "<table><tr><td>aa</td><td>bb</td></tr><tr><td>cc</td><td>dd</td></tr></table>", 20)
	txt := allText(d)
	l0 := lineText(d, 0)
	if !strings.Contains(l0, "aa") || !strings.Contains(l0, "bb") {
		t.Errorf("row0 = %q, want aa and bb on same line\nfull:\n%s", l0, txt)
	}
	if !strings.Contains(allText(d), "cc") {
		t.Errorf("row1 missing cc:\n%s", txt)
	}
}

func TestImagePlaceholder(t *testing.T) {
	d := doc(t, `<p><img src="x.png" alt="my cat"></p>`, 40) // nil fetcher
	if !strings.Contains(allText(d), "[my cat]") {
		t.Errorf("alt placeholder missing:\n%s", allText(d))
	}
}

func TestImagePixels(t *testing.T) {
	fetch := func(url string) (image.Image, error) {
		img := image.NewRGBA(image.Rect(0, 0, 40, 20))
		for y := 0; y < 20; y++ {
			for x := 0; x < 40; x++ {
				img.Set(x, y, color.RGBA{R: 200, A: 255})
			}
		}
		return img, nil
	}
	d, err := dom.Parse(`<img src="/x.png">`, "https://example.org/")
	if err != nil {
		t.Fatal(err)
	}
	out := Render(d, 20, fetch)
	var pixelLines int
	for _, ln := range out.Lines {
		if len(ln) > 0 && ln[0].Rune == '▀' && ln[0].HasFg && ln[0].HasBg {
			pixelLines++
		}
	}
	if pixelLines == 0 {
		t.Error("no pixel lines rendered for image")
	}
	if pixelLines > 15 {
		t.Errorf("pixelLines = %d, exceeds 15-row cap", pixelLines)
	}
}
```

(add `"image"` and `"image/color"` imports to the test file)

- [ ] **Step 2: Run, confirm FAIL**

- [ ] **Step 3: Implement.**
- Tables: collect `<td>`/`<th>` per `<tr>`; colWidth = (width − ncols + 1) / ncols (1-space gutters); lay out each cell's inline content at colWidth via a nested mini-layout; pad cell lines to equal height; merge columns side by side into output lines. `colspan` ignored (cell takes one column). A `<table>` inside a `<td>` is rendered by the nested mini-layout as sequential blocks (flattening falls out naturally).
- Images: resolve src, call fetcher; on success scale with existing `render.ToCells(img, imgCols, imgRows)` where `imgCols = min(width, natural width in cells)` and `imgRows = min(15, proportional)`; convert `render.Cell` → `cell.Cell{Rune: '▀', HasFg: true, HasBg: true, Fg: top, Bg: bottom}`; emit as standalone lines. On nil fetcher/error: emit `[alt]` (or `[image]` when alt empty) in dim style.

- [ ] **Step 4:** `go test ./layout/ -v` → all PASS

- [ ] **Step 5: Commit**

```bash
git add layout/
git commit -m "feat: layout tables and inline half-block images"
```

---

### Task 7: main.go rewrite, delete browser/, README

**Goal:** Wire the new engine; remove Chrome completely.

**Files:**
- Modify: `main.go` (rewrite), `README.md` (rewrite)
- Delete: `browser/` (entire package)

**Acceptance Criteria:**
- [ ] `pixsurf <url>` fetches, lays out, draws; `--no-images` flag works
- [ ] Scroll instant/local; resize re-layouts and keeps position ratio; click hit-tests links; `#frag`/`mailto:`/`javascript:` handled per spec
- [ ] Back/forward via history stack; `g`/`r`/`q` as before
- [ ] chromedp gone from go.mod after `go mod tidy`
- [ ] `go build ./... && go vet ./... && go test ./...` exit 0

**Verify:** `go build -o pixsurf . && go vet ./... && go test ./...` → exit 0; grep chromedp go.mod → no match

**Steps:**

- [ ] **Step 1: Delete `browser/`**, rewrite `main.go`:

```go
// Command pixsurf is a terminal web browser with its own pure-Go engine.
package main

import (
	"flag"
	"fmt"
	"image"
	"os"
	"strings"

	"github.com/MhmdShd/pixsurf/cell"
	"github.com/MhmdShd/pixsurf/dom"
	"github.com/MhmdShd/pixsurf/fetch"
	"github.com/MhmdShd/pixsurf/layout"
	"github.com/MhmdShd/pixsurf/ui"
)

func main() {
	noImages := flag.Bool("no-images", false, "skip fetching and rendering images")
	flag.Parse()

	startURL := "https://example.com"
	if flag.NArg() > 0 {
		startURL = flag.Arg(0)
	}

	u, err := ui.New()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer u.Close()

	a := &app{u: u, client: fetch.New(), noImages: *noImages}
	a.cols, a.rows = u.GridSize()
	a.navigate(startURL, true)
	a.run()
}

type app struct {
	u        *ui.UI
	client   *fetch.Client
	noImages bool

	cols, rows int
	doc        *layout.Document
	offset     int
	url        string
	lastErr    string

	history []string
	histPos int // index of current page in history
}

func (a *app) fetcher() layout.ImageFetcher {
	if a.noImages {
		return nil
	}
	return func(u string) (image.Image, error) { return a.client.Image(u) }
}

// navigate fetches and lays out rawURL. push records it in history.
func (a *app) navigate(rawURL string, push bool) {
	a.lastErr = ""
	target := rawURL
	if !strings.Contains(target, "://") {
		target = "https://" + strings.TrimSpace(target)
	}

	body, finalURL, truncated, err := a.client.Page(target)
	if err != nil {
		a.lastErr = err.Error()
		a.draw()
		return
	}
	d, err := dom.Parse(body, finalURL)
	if err != nil {
		a.lastErr = err.Error()
		a.draw()
		return
	}
	a.doc = layout.Render(d, a.cols, a.fetcher())
	a.url = finalURL
	a.offset = 0
	if truncated {
		a.lastErr = "page truncated at 5MB"
	}
	if push {
		a.history = append(a.history[:a.histPos], finalURL)
		a.histPos = len(a.history) - 1
	}
	a.draw()
}

// relayout re-flows the current document at the current width, keeping the
// scroll position proportionally.
func (a *app) relayout() {
	if a.doc == nil {
		return
	}
	ratio := 0.0
	if len(a.doc.Lines) > 0 {
		ratio = float64(a.offset) / float64(len(a.doc.Lines))
	}
	// re-fetch-free reflow requires the parsed DOM; simplest correct path:
	// re-fetch via navigate without pushing history. Cheap pages, local net
	// cost accepted for v2.
	saved := a.url
	a.navigate(saved, false)
	a.offset = int(ratio * float64(len(a.doc.Lines)))
	a.clampOffset()
	a.draw()
}

func (a *app) clampOffset() {
	max := 0
	if a.doc != nil {
		max = len(a.doc.Lines) - a.rows
	}
	if max < 0 {
		max = 0
	}
	if a.offset > max {
		a.offset = max
	}
	if a.offset < 0 {
		a.offset = 0
	}
}

// view returns the visible slice of the document, padded to rows x cols.
func (a *app) view() [][]cell.Cell {
	out := make([][]cell.Cell, a.rows)
	for i := 0; i < a.rows; i++ {
		out[i] = make([]cell.Cell, a.cols)
		if a.doc != nil && a.offset+i < len(a.doc.Lines) {
			copy(out[i], a.doc.Lines[a.offset+i])
		}
	}
	return out
}

func (a *app) draw() {
	status := a.url + "   [g:url b:back f:fwd r:reload q:quit]"
	if a.lastErr != "" {
		status = "error: " + a.lastErr
	}
	a.u.Draw(a.view(), status)
}

func (a *app) scroll(dy int) {
	a.offset += dy
	a.clampOffset()
	a.draw()
}

func (a *app) click(x, y int) {
	if a.doc == nil {
		return
	}
	href, ok := a.doc.LinkAt(a.offset+y, x)
	if !ok {
		return
	}
	switch {
	case strings.HasPrefix(href, "mailto:"), strings.HasPrefix(href, "javascript:"):
		a.lastErr = "unsupported link: " + href
		a.draw()
	case strings.Contains(href, "#") && strings.Split(href, "#")[0] == strings.Split(a.url, "#")[0]:
		frag := strings.SplitN(href, "#", 2)[1]
		if ln, ok := a.doc.Anchors[frag]; ok {
			a.offset = ln
			a.clampOffset()
		} else {
			a.lastErr = "anchor not found: #" + frag
		}
		a.draw()
	default:
		a.navigate(href, true)
	}
}

func (a *app) back() {
	if a.histPos > 0 {
		a.histPos--
		a.navigate(a.history[a.histPos], false)
	}
}

func (a *app) forward() {
	if a.histPos < len(a.history)-1 {
		a.histPos++
		a.navigate(a.history[a.histPos], false)
	}
}

func (a *app) run() {
	for ev := range a.u.Events() {
		switch e := ev.(type) {
		case ui.ActionEvent:
			switch e.Kind {
			case ui.Quit:
				return
			case ui.ScrollUp:
				a.scroll(-1)
			case ui.ScrollDown:
				a.scroll(1)
			case ui.PageUp:
				a.scroll(-(a.rows - 1))
			case ui.PageDown:
				a.scroll(a.rows - 1)
			case ui.Back:
				a.back()
			case ui.Forward:
				a.forward()
			case ui.Reload:
				a.navigate(a.url, false)
			}
		case ui.ClickEvent:
			a.click(e.X, e.Y)
		case ui.ResizeEvent:
			cols, rows := a.u.GridSize()
			if cols == a.cols && rows == a.rows {
				a.draw() // redraw request (URL bar typing)
				continue
			}
			if cols < 1 || rows < 1 {
				continue
			}
			a.cols, a.rows = cols, rows
			a.relayout()
		case ui.URLEvent:
			a.navigate(e.URL, true)
		}
	}
}
```

Design note baked in: `relayout()` re-fetches the current URL — simple and correct; a cached-DOM reflow is a future optimization. History truncation on new navigation (`a.history[:a.histPos]` then append) intentionally forks history like real browsers... verify slice bounds: on first navigate history is empty and histPos 0 — `a.history[:0]` is valid. After first push history=[url], histPos=0. Second push: `history[:0]`+append = [url2] — WRONG, loses first entry. Fix in implementation: push as `a.history = append(a.history[:a.histPos+1], finalURL)` when history is non-empty, plain append when empty:

```go
if push {
	if len(a.history) == 0 {
		a.history = []string{finalURL}
	} else {
		a.history = append(a.history[:a.histPos+1], finalURL)
	}
	a.histPos = len(a.history) - 1
}
```

Use this corrected version, not the earlier snippet.

- [ ] **Step 2:** `go mod tidy` (chromedp drops out), then `go build -o pixsurf . && go vet ./... && go test ./...` → exit 0; `grep chromedp go.mod` → no match. Delete the binary.

- [ ] **Step 3: Rewrite `README.md`:**

```markdown
# pixsurf

Surf the web in your terminal — no browser install, no JavaScript bloat.

pixsurf is a terminal web browser with its own pure-Go rendering engine:
it fetches pages, lays them out with colors and styles, draws images as
truecolor half-block pixels, and makes links clickable — all in a single
static binary. No Chrome, no Chromium, no dependencies. VPS-friendly:
starts instantly, uses a few dozen MB of RAM.

No JavaScript by design: content sites (news, docs, wikis, blogs) render
great; app-style sites that require JS will show their static content only.

## Requirements

- A terminal with truecolor support (most modern terminals)

That's it.

## Install

```sh
go install github.com/MhmdShd/pixsurf@latest
```

## Usage

```sh
pixsurf wikipedia.org
pixsurf --no-images news.ycombinator.com   # text only, minimal bandwidth
```

## Controls

| Key | Action |
|-----|--------|
| ↑ / ↓ | scroll |
| PgUp / PgDn | scroll a page |
| mouse click | follow link |
| g | enter a URL |
| b / f | back / forward |
| r | reload |
| q | quit |

## How it works

Pure-Go pipeline: HTTP fetch (capped, cookie-aware) → HTML parse
(`x/net/html`) → semantic styling → flow layout at your terminal width →
styled cells drawn with tcell. Images are downscaled to `▀` half-blocks
with 24-bit color.

## License

MIT
```

- [ ] **Step 4: Commit**

```bash
git add -A
git commit -m "feat!: replace Chrome with pure-Go rendering engine

pixsurf no longer requires Chrome/Chromium. Own fetch/parse/style/layout
pipeline; instant local scrolling; --no-images flag; VPS-friendly."
```

---

### Task 8: Smoke test + push

**Goal:** Verify against real sites, push.

**Steps:**

- [ ] **Step 1:** Temp `smoke/main.go`: fetch.New → Page("https://example.com") → dom.Parse → layout.Render(width 80, nil fetcher) → assert >3 non-empty lines, ≥1 link on wikipedia.org main page (second fetch), print OK + line/link counts. Run `go run ./smoke`, paste output, then `rm -rf smoke/`.
- [ ] **Step 2:** Full check: `go build ./... && go vet ./... && go test ./...` → exit 0.
- [ ] **Step 3:** `git push origin main`. Confirm CI green.

---

## Self-Review Notes

- Spec coverage: fetch caps/UA/cookies/charset (Task 2), base URL + parsing (Task 3), tag styles + inline colors + hidden (Task 4), wrap/wide-runes/links/anchors/lists/pre/hr (Task 5), tables/images/alt (Task 6), app loop incl. fragment/mailto/history/resize-ratio/--no-images + Chrome removal + README (Task 7), smoke+push (Task 8).
- Known simplifications called out in code: relayout re-fetches; colspan ignored; pre clipped.
- Type consistency: `cell.Cell`/`cell.RGB` shared by ui/layout; `layout.Render(d *dom.Doc, width int, images ImageFetcher)`; `Document.LinkAt(line, col)` — consistent across Tasks 5–7.
- History push bug in first draft corrected inline in Task 7 Step 1.
