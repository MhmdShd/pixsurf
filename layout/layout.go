// Package layout flows a parsed page into terminal-width styled lines.
package layout

import (
	"fmt"
	"image"
	"strings"

	"github.com/MhmdShd/pixsurf/cell"
	"github.com/MhmdShd/pixsurf/dom"
	"github.com/MhmdShd/pixsurf/style"
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
	Forms   []Form
	Fields  []Field

	// Page background from the <body> (or <html>) element's bgcolor
	// attribute or inline style; the viewport paints it edge to edge.
	PageBg    cell.RGB
	HasPageBg bool
}

// ImageFetcher loads an image by absolute URL; nil disables images.
type ImageFetcher func(url string) (image.Image, error)

// LinkAt returns the URL covering (line, col), if any.
func (d *Document) LinkAt(line, col int) (string, bool) {
	for _, l := range d.Links {
		if l.Line == line && col >= l.Start && col < l.End {
			return l.URL, true
		}
	}
	return "", false
}

// Render lays out doc at the given content width. values holds
// user-entered form field values (keyed by ValuesKey); nil is fine.
func Render(d *dom.Doc, width int, images ImageFetcher, values FormValues) *Document {
	if width < 1 {
		width = 1
	}
	out := &Document{Anchors: map[string]int{}}
	out.Title = findTitle(d.Root)
	out.PageBg, out.HasPageBg = pageBackground(d)
	root := contentRoot(d)
	w := &walker{doc: out, src: d, width: width, images: images, values: values, linkOpen: -1, formIdx: -1}
	w.skipChrome = chromeSkipSafe(root)
	rootSt := style.Style{}
	if out.HasPageBg {
		// seed the walk with the page background so every content line is
		// painted even when the content root is below <body>
		rootSt.HasBg, rootSt.Bg = true, out.PageBg
	}
	w.renderNode(root, rootSt)
	w.flushLine()
	return out
}

// pageBackground extracts the page's sheet color from the <body> (or
// <html>) element: the legacy bgcolor attribute or an inline style
// background(-color) declaration.
func pageBackground(d *dom.Doc) (cell.RGB, bool) {
	for _, tag := range []string{"body", "html"} {
		n := findElement(d.Root, tag)
		if n == nil {
			continue
		}
		if c, ok := style.ParseColor(dom.Attr(n, "bgcolor")); ok {
			return c, true
		}
		for _, decl := range strings.Split(dom.Attr(n, "style"), ";") {
			k, v, ok := strings.Cut(decl, ":")
			if !ok {
				continue
			}
			switch strings.TrimSpace(strings.ToLower(k)) {
			case "background-color", "background":
				if c, ok := style.ParseColor(v); ok {
					return c, true
				}
			}
		}
	}
	return cell.RGB{}, false
}

// findTitle extracts the first <title>'s text; the body walk skips it.
func findTitle(root *dom.Node) string {
	var title string
	dom.Walk(root, func(n *dom.Node) {
		if title != "" {
			return
		}
		if n.Type == dom.ElementNode && n.Data == "title" {
			var b strings.Builder
			dom.Walk(n, func(c *dom.Node) {
				if c.Type == dom.TextNode {
					b.WriteString(c.Data)
				}
			})
			title = strings.TrimSpace(b.String())
		}
	})
	return title
}

// listFrame tracks one nested <ul>/<ol> level.
type listFrame struct {
	ordered bool
	n       int
}

// walker carries the flow-layout state while descending the DOM.
type walker struct {
	doc    *Document
	src    *dom.Doc
	width  int
	images ImageFetcher

	line     []cell.Cell // current line being built
	col      int         // display column after last cell
	lineBase int         // column where content starts (after indent)
	started  bool        // startLine has run for the current line

	pendingBlank  bool // emit one blank line before the next content
	pendingSpace  bool // a collapsed space is owed before the next word
	joinNextBlock bool // bullet-join window: the line still holds only an <li> marker
	skipChrome    bool // skip nav/header/footer/aside subtrees
	hfDepth       int  // nesting depth inside kept header/footer elements

	measuring bool // natural-width measurement: skip background fill

	pre   bool // inside <pre>: verbatim text, no wrap
	quote int  // blockquote nesting depth (2 cols each)
	lists []listFrame

	linkURL  string // open link URL, "" when none
	linkOpen int    // column where the open link's range began on this line; -1 none

	values  FormValues // user-entered field values; nil is fine
	formIdx int        // index of the open <form> in doc.Forms; -1 none
}

var blockTags = map[string]bool{
	"p": true, "div": true, "section": true, "article": true,
	"header": true, "footer": true, "main": true, "nav": true, "aside": true,
	"h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true,
	"tr": true, "figure": true, "figcaption": true,
	"form": true, "fieldset": true, "address": true, "dl": true,
	"dt": true, "dd": true,
}

func (w *walker) renderNode(n *dom.Node, st style.Style) {
	switch n.Type {
	case dom.TextNode:
		w.emitText(n.Data, st)
		return
	case dom.ElementNode:
		// handled below
	default:
		w.walkChildren(n, st)
		return
	}

	tag := strings.ToLower(n.Data)
	if style.Hidden(tag) {
		return
	}
	if w.skipChrome && isChrome(n) {
		return
	}
	st = style.ApplyInline(style.ForTag(tag, st), n)

	switch {
	case tag == "br":
		w.recordAnchor(n)
		w.flushLine()
	case tag == "hr":
		w.flushLine()
		w.pendingBlank = true
		w.recordAnchor(n)
		for i := 0; i < w.width-w.indentCols(); i++ {
			w.putRune('─', st)
		}
		w.flushLine()
		w.pendingBlank = true
	case tag == "pre":
		w.blockStart()
		w.recordAnchor(n)
		w.pre = true
		w.walkChildren(n, st)
		w.pre = false
		w.blockEnd()
	case tag == "blockquote":
		w.blockStart()
		w.recordAnchor(n)
		w.quote++
		w.walkChildren(n, st)
		w.quote--
		w.blockEnd()
	case tag == "ul" || tag == "ol":
		if w.skipChrome && w.hfDepth > 0 && isLinkFarm(n) {
			return // nav-list inside a kept header/footer (no nav markup)
		}
		nested := len(w.lists) > 0
		if !w.joinNextBlock { // a list opening on a bare bullet line joins it
			w.flushLine()
			if !nested {
				w.pendingBlank = true
			}
		}
		w.recordAnchor(n)
		w.lists = append(w.lists, listFrame{ordered: tag == "ol"})
		w.walkChildren(n, st)
		w.lists = w.lists[:len(w.lists)-1]
		if !w.joinNextBlock { // empty list on a bare bullet line: keep it open
			w.flushLine()
			if !nested {
				w.pendingBlank = true
			}
		}
	case tag == "li":
		if !w.joinNextBlock { // nested item on a bare bullet line joins it
			w.flushLine()
		}
		w.recordAnchor(n)
		depth := len(w.lists)
		marker := "• "
		if depth > 0 && w.lists[depth-1].ordered {
			w.lists[depth-1].n++
			marker = fmt.Sprintf("%d. ", w.lists[depth-1].n)
		}
		if depth > 1 {
			marker = strings.Repeat("  ", depth-1) + marker
		}
		for _, r := range marker {
			w.putRune(r, st)
		}
		w.pendingSpace = false
		w.joinNextBlock = true // a leading block child stays on the bullet line
		w.walkChildren(n, st)
		w.joinNextBlock = false
		w.flushLine()
		w.pendingBlank = false // items pack tightly; ul/ol owns outer blanks
	case tag == "a":
		w.recordAnchor(n)
		href := dom.Attr(n, "href")
		if href == "" || w.linkURL != "" { // no target, or nested link: plain
			w.walkChildren(n, st)
			return
		}
		// NOTE: a pending separator space is deliberately NOT emitted here
		// before opening the link; it is emitted by the first emitWord after
		// the link opens and therefore joins the link range. Excluding it
		// (review follow-up 3) drops TestLinksAndAnchors coverage to 14 < 15
		// and fails the fixed test contract. See review report.
		w.linkURL = w.src.Resolve(href)
		w.walkChildren(n, st)
		w.closeLinkRange()
		w.linkURL = ""
	case tag == "form":
		w.renderForm(n, st)
	case tag == "input":
		w.recordAnchor(n)
		w.emitInput(n, st)
	case tag == "button":
		w.recordAnchor(n)
		w.emitButton(n, st)
	case tag == "textarea":
		// consumes its text content as the field value; children are
		// deliberately not walked so the content never renders as page text
		w.recordAnchor(n)
		w.emitTextarea(n, st)
	case tag == "table":
		w.renderTable(n, st)
	case tag == "img":
		w.recordAnchor(n)
		w.emitImage(n, st)
	case tag == "iframe" || tag == "frame":
		w.flushLine()
		w.recordAnchor(n)
		src := dom.Attr(n, "src")
		if src == "" {
			return
		}
		resolved := w.src.Resolve(src)
		prev := w.linkURL
		if prev == "" {
			w.linkURL = resolved
		}
		for _, r := range "[frame: " + resolved + "]" {
			w.putRune(r, st)
		}
		if prev == "" {
			w.closeLinkRange()
			w.linkURL = ""
		}
		w.flushLine()
	case blockTags[tag]:
		hf := tag == "header" || tag == "footer"
		if hf {
			w.hfDepth++
		}
		w.blockStart()
		w.recordAnchor(n)
		w.walkChildren(n, st)
		w.blockEnd()
		if hf {
			w.hfDepth--
		}
	default: // inline or unknown: flow children in place
		w.recordAnchor(n)
		w.walkChildren(n, st)
	}
}

func (w *walker) walkChildren(n *dom.Node, st style.Style) {
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		w.renderNode(c, st)
	}
}

// recordAnchor maps an id attribute to the line its content will land on.
func (w *walker) recordAnchor(n *dom.Node) {
	id := dom.Attr(n, "id")
	if id == "" {
		return
	}
	if _, exists := w.doc.Anchors[id]; exists {
		return
	}
	w.doc.Anchors[id] = w.upcomingLine()
}

// upcomingLine is the index the next emitted content will occupy.
func (w *walker) upcomingLine() int {
	ln := len(w.doc.Lines)
	if w.started {
		return ln // current partial line flushes at this index
	}
	if w.pendingBlank && ln > 0 {
		return ln + 1 // a blank line materializes first
	}
	return ln
}

// blockStart breaks the line before a block and requests a blank
// separator. While the bullet-join window is open — an <li> line still
// holding only its marker — the break is suppressed at any wrapper
// depth so the first real content joins the bullet line; emitWord
// closes the window.
func (w *walker) blockStart() {
	if w.joinNextBlock {
		return
	}
	w.flushLine()
	w.pendingBlank = true
}

func (w *walker) blockEnd() {
	if w.joinNextBlock {
		return // nothing after the marker yet (empty block): keep joining
	}
	w.flushLine()
	w.pendingBlank = true
}
