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

// Render lays out doc at the given content width.
func Render(d *dom.Doc, width int, images ImageFetcher) *Document {
	if width < 1 {
		width = 1
	}
	out := &Document{Anchors: map[string]int{}}
	out.Title = findTitle(d.Root)
	w := &walker{doc: out, src: d, width: width, images: images, linkOpen: -1}
	w.renderNode(d.Root, style.Style{})
	w.flushLine()
	return out
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

	pendingBlank bool // emit one blank line before the next content
	pendingSpace bool // a collapsed space is owed before the next word

	pre   bool // inside <pre>: verbatim text, no wrap
	quote int  // blockquote nesting depth (2 cols each)
	lists []listFrame

	linkURL  string // open link URL, "" when none
	linkOpen int    // column where the open link's range began on this line; -1 none
}

var blockTags = map[string]bool{
	"p": true, "div": true, "section": true, "article": true,
	"header": true, "footer": true, "main": true, "nav": true, "aside": true,
	"h1": true, "h2": true, "h3": true, "h4": true, "h5": true, "h6": true,
	"table": true, "tr": true, "figure": true, "figcaption": true,
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
	st = style.ApplyInline(style.ForTag(tag, st), n)

	switch {
	case tag == "br":
		w.recordAnchor(n)
		w.flushLine()
	case tag == "hr":
		w.flushLine()
		w.pendingBlank = true
		w.recordAnchor(n)
		for i := 0; i < w.width; i++ {
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
		nested := len(w.lists) > 0
		w.flushLine()
		if !nested {
			w.pendingBlank = true
		}
		w.recordAnchor(n)
		w.lists = append(w.lists, listFrame{ordered: tag == "ol"})
		w.walkChildren(n, st)
		w.lists = w.lists[:len(w.lists)-1]
		w.flushLine()
		if !nested {
			w.pendingBlank = true
		}
	case tag == "li":
		w.flushLine()
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
		w.walkChildren(n, st)
		w.flushLine()
	case tag == "a":
		w.recordAnchor(n)
		href := dom.Attr(n, "href")
		if href == "" || w.linkURL != "" { // no target, or nested link: plain
			w.walkChildren(n, st)
			return
		}
		w.linkURL = w.src.Resolve(href)
		w.walkChildren(n, st)
		w.closeLinkRange()
		w.linkURL = ""
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
		w.blockStart()
		w.recordAnchor(n)
		w.walkChildren(n, st)
		w.blockEnd()
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

func (w *walker) blockStart() {
	w.flushLine()
	w.pendingBlank = true
}

func (w *walker) blockEnd() {
	w.flushLine()
	w.pendingBlank = true
}

// emitImage renders an <img>. Without a fetcher (or in this task, always)
// it emits an [alt] placeholder; Task 6 slots pixel rendering in here.
func (w *walker) emitImage(n *dom.Node, st style.Style) {
	alt := strings.TrimSpace(dom.Attr(n, "alt"))
	text := "[image]"
	if alt != "" {
		text = "[" + alt + "]"
	}
	st.Dim = true
	w.emitText(text, st)
}
