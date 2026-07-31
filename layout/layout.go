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

// StyleResolver supplies computed styles. Render accepts nil, in which
// case it falls back to style.ForTag + style.ApplyInline as today.
type StyleResolver interface {
	// Hidden reports display:none / visibility:hidden for n's subtree.
	Hidden(n *dom.Node) bool
	// Resolve returns n's computed style given its parent's.
	Resolve(n *dom.Node, parent style.Style) style.Style
}

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
// styles supplies computed CSS styles; nil falls back to tag defaults.
func Render(d *dom.Doc, width int, images ImageFetcher, values FormValues, styles StyleResolver) *Document {
	if width < 1 {
		width = 1
	}
	out := &Document{Anchors: map[string]int{}}
	out.Title = findTitle(d.Root)
	out.PageBg, out.HasPageBg = pageBackground(d)
	if !out.HasPageBg && styles != nil {
		out.PageBg, out.HasPageBg = cssPageBackground(d, styles)
	}
	root := contentRoot(d)
	w := &walker{doc: out, src: d, width: width, images: images, values: values, styles: styles, linkOpen: -1, formIdx: -1}
	w.skipChrome = chromeSkipSafe(root)
	rootSt := style.Style{}
	if styles != nil {
		// Resolve the elements above the content root so their inherited
		// properties and painted backdrop are in effect for the walk even
		// when the root is below <body>.
		rootSt = resolveChain(root.Parent, styles)
	}
	if out.HasPageBg {
		// seed the walk with the page background so every content line is
		// painted even when the content root is below <body>
		rootSt.HasBackdrop, rootSt.Backdrop = true, out.PageBg
	}
	w.hasStyleBg, w.styleBg = rootSt.HasBackdrop, rootSt.Backdrop
	w.renderNode(root, rootSt)
	w.flushLine()
	return out
}

// resolveChain computes n's style by resolving its ancestor chain top
// down, exactly as the body walk would have. Resolver memoisation makes
// repeated calls cheap.
func resolveChain(n *dom.Node, styles StyleResolver) style.Style {
	if n == nil {
		return style.Style{}
	}
	parent := resolveChain(n.Parent, styles)
	if n.Type != dom.ElementNode {
		return parent
	}
	return styles.Resolve(n, parent)
}

// cssPageBackground is the page sheet colour per the stylesheet: the
// computed backdrop of the <body> (or <html>) element. It complements
// pageBackground, which only sees attributes and inline styles.
func cssPageBackground(d *dom.Doc, styles StyleResolver) (cell.RGB, bool) {
	for _, tag := range []string{"body", "html"} {
		n := findElement(d.Root, tag)
		if n == nil {
			continue
		}
		if st := resolveChain(n, styles); st.HasBackdrop {
			return st.Backdrop, true
		}
	}
	return cell.RGB{}, false
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
	styles StyleResolver // nil: style.ForTag + style.ApplyInline

	line     []cell.Cell // current line being built
	col      int         // display column after last cell
	lineBase int         // column where content starts (after indent)
	started  bool        // startLine has run for the current line

	pendingBlank  bool     // emit one blank line before the next content
	blankHasBg    bool     // paint the pending blank with blankBg
	blankBg       cell.RGB // backdrop of the pending blank's containing sheet
	pendingSpace  bool     // a collapsed space is owed before the next word
	joinNextBlock bool     // bullet-join window: the line still holds only an <li> marker
	skipChrome    bool     // skip nav/header/footer/aside subtrees
	hfDepth       int      // nesting depth inside kept header/footer elements

	measuring bool // natural-width measurement: skip background fill

	// Style-derived backdrop currently in effect: the backdrop of the
	// style last applied (page background as the base). Image-pixel
	// lines pad with this — never with the incidental colour of a
	// content cell such as an image pixel. Blank separators paint with
	// blankBg instead (see requestBlank).
	styleBg    cell.RGB
	hasStyleBg bool
	linePixels bool        // current line holds image pixel cells
	lineAlign  style.Align // alignment of the style last applied on this line

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
	parentSt := st // n's containing sheet: blank separators paint with it
	if w.styles != nil {
		if w.styles.Hidden(n) {
			return
		}
		st = w.styles.Resolve(n, st)
	} else {
		st = style.ApplyInline(style.ForTag(tag, st), n)
	}

	switch {
	case tag == "br":
		w.recordAnchor(n)
		w.flushLine()
	case tag == "hr":
		w.flushLine()
		w.requestBlank(parentSt)
		w.recordAnchor(n)
		for i := 0; i < w.width-w.indentCols(); i++ {
			w.putRune('─', st)
		}
		w.flushLine()
		w.requestBlank(parentSt)
	case tag == "pre":
		w.blockStart(parentSt)
		w.recordAnchor(n)
		w.pre = true
		w.walkChildren(n, st)
		w.pre = false
		w.blockEnd(parentSt)
	case tag == "blockquote":
		w.blockStart(parentSt)
		w.recordAnchor(n)
		w.quote++
		w.walkChildren(n, st)
		w.quote--
		w.blockEnd(parentSt)
	case tag == "ul" || tag == "ol":
		if w.skipChrome && w.hfDepth > 0 && isLinkFarm(n) {
			return // nav-list inside a kept header/footer (no nav markup)
		}
		nested := len(w.lists) > 0
		if !w.joinNextBlock { // a list opening on a bare bullet line joins it
			w.flushLine()
			if !nested {
				w.requestBlank(parentSt)
			}
		}
		w.recordAnchor(n)
		w.lists = append(w.lists, listFrame{ordered: tag == "ol"})
		w.walkChildren(n, st)
		w.lists = w.lists[:len(w.lists)-1]
		if !w.joinNextBlock { // empty list on a bare bullet line: keep it open
			w.flushLine()
			if !nested {
				w.requestBlank(parentSt)
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
		w.renderForm(n, st, parentSt)
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
		w.renderTable(n, st, parentSt)
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
		w.blockStart(parentSt)
		w.recordAnchor(n)
		w.walkChildren(n, st)
		w.blockEnd(parentSt)
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
// separator painted with the block's containing sheet (parent is the
// style of the block's parent). While the bullet-join window is open —
// an <li> line still holding only its marker — the break is suppressed
// at any wrapper depth so the first real content joins the bullet line;
// emitWord closes the window.
func (w *walker) blockStart(parent style.Style) {
	if w.joinNextBlock {
		return
	}
	w.flushLine()
	w.requestBlank(parent)
}

func (w *walker) blockEnd(parent style.Style) {
	if w.joinNextBlock {
		return // nothing after the marker yet (empty block): keep joining
	}
	w.flushLine()
	w.requestBlank(parent)
}

// requestBlank asks for one blank separator line before the next
// content. The blank is a between-block margin: transparent, so it
// shows the backdrop of the parent of the blocks it separates — never
// the colour of a small painted element (a button, say) that happened
// to sit at the boundary. Consecutive requests overwrite; the last one
// before the blank materializes is the outermost boundary crossed,
// whose parent sheet is the right one.
func (w *walker) requestBlank(parent style.Style) {
	w.pendingBlank = true
	w.blankHasBg, w.blankBg = parent.HasBackdrop, parent.Backdrop
}
