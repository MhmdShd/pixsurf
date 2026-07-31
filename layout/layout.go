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

	// ContrastFixes counts cells whose declared foreground was replaced
	// by the contrast safety net because it was too close to the cell's
	// backdrop to read (see ensureContrast).
	ContrastFixes int
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
	w.hasBlockBg, w.blockBg = rootSt.HasBackdrop, rootSt.Backdrop
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
// pageBackground, which only sees attributes and inline styles. When
// body and html declare no background, sites often paint the sheet on a
// wrapper element instead (e.g. a page-container div holding all the
// content), so the search descends from <body> through the chain of
// dominant wrappers — each holding essentially all of the page's text —
// and takes the highest one that declares a background.
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
	body := findElement(d.Root, "body")
	if body == nil {
		body = d.Root
	}
	st := resolveChain(body, styles)
	const maxWrapperDepth = 8
	for n, depth := body, 0; depth < maxWrapperDepth; depth++ {
		c := dominantChild(n)
		if c == nil {
			break
		}
		st = styles.Resolve(c, st)
		if st.HasBackdrop {
			return st.Backdrop, true
		}
		n = c
	}
	return cell.RGB{}, false
}

// dominantWrapperShare: a child is the page's wrapper when it holds at
// least this share of its parent's visible text.
const dominantWrapperShare = 0.9

// dominantChild returns the element child of n holding essentially all
// of n's visible text — the wrapper pattern — or nil when none does.
func dominantChild(n *dom.Node) *dom.Node {
	total := visibleTextLen(n)
	if total == 0 {
		return nil
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type != dom.ElementNode {
			continue
		}
		if float64(visibleTextLen(c)) >= dominantWrapperShare*float64(total) {
			return c
		}
	}
	return nil
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

	pendingBlank  bool        // emit one blank line before the next content
	blankHasBg    bool        // paint the pending blank with blankBg
	blankBg       cell.RGB    // backdrop of the pending blank's containing sheet
	pendingSpace  bool        // a collapsed space is owed before the next word
	spaceSt       style.Style // style the pending space arose in (its backdrop paints the space)
	joinNextBlock bool        // bullet-join window: the line still holds only an <li> marker
	skipChrome    bool        // skip nav/header/footer/aside subtrees
	hfDepth       int         // nesting depth inside kept header/footer elements

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

	// Block-level background currently in effect: the page sheet or the
	// own background of the innermost enclosing block-level element.
	// Only this fills whole lines (fillBackground); an inline element's
	// background stays behind its own glyphs.
	blockBg    cell.RGB
	hasBlockBg bool

	// flexRow is set while walking the immediate children of a flex or
	// grid container: each child lays out inline regardless of its own
	// block-ness, separated by single spaces.
	flexRow bool

	// alignForce overrides line alignment while an auto-margin block is
	// open (margin: 0 auto centres, margin-left: auto right-aligns).
	alignForce style.Align

	// inlineBox counts open inline-block / inline-flex boxes; while > 0,
	// descendant blocks do not break the line (they break inside the box
	// in CSS, which a flat flow cannot represent).
	inlineBox int

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
	"dt": true, "dd": true, "center": true,
}

// extraBlockish are tags with dedicated handlers that are nonetheless
// block-level for display resolution and block-background purposes.
var extraBlockish = map[string]bool{
	"pre": true, "blockquote": true, "ul": true, "ol": true, "li": true,
	"table": true, "hr": true, "td": true, "th": true,
}

// blockishTag reports a tag's default block-ness absent any CSS display
// declaration.
func blockishTag(tag string) bool { return blockTags[tag] || extraBlockish[tag] }

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

	// This element is a flex item when its parent is a flex/grid
	// container; the flag applies to immediate children only, so clear
	// it for this element's own descent and restore it for siblings.
	flexItem := w.flexRow
	w.flexRow = false
	defer func() { w.flexRow = flexItem }()

	// Effective display: the tag default overridden by any CSS display
	// declaration; flex items always lay out inline.
	tagBlock := blockishTag(tag)
	isBlock := tagBlock
	switch st.Display {
	case style.DisplayInline, style.DisplayInlineBlock, style.DisplayInlineFlex:
		isBlock = false
	case style.DisplayBlock, style.DisplayFlex:
		isBlock = true
	}
	if flexItem || w.inlineBox > 0 {
		// Flex items lay out inline; and a block inside an open
		// inline-block box breaks inside that box in CSS, which a flat
		// character flow approximates by not breaking at all.
		isBlock = false
	}
	flexC := st.Display == style.DisplayFlex || st.Display == style.DisplayInlineFlex
	inlineBoxEl := st.Display == style.DisplayInlineBlock || st.Display == style.DisplayInlineFlex
	if inlineBoxEl {
		w.inlineBox++
		defer func() { w.inlineBox-- }()
	}

	// Auto horizontal margins centre (or right-shift) a block's lines.
	// Applied as a walker-level override rather than by mutating st:
	// resolver memoisation means children never see a mutated parent.
	if isBlock && st.MarginLeftAuto {
		saved := w.alignForce
		if st.MarginRightAuto {
			w.alignForce = style.AlignCenter
		} else {
			w.alignForce = style.AlignRight
		}
		defer func() { w.alignForce = saved }()
	}

	// justify-content on a flex/grid container aligns the container's
	// inline row via the same alignForce override auto margins use, so
	// link/field/submit/anchor ranges shift with the line. Set after the
	// auto-margin override: on one element, justify-content is the more
	// specific signal for where the container's own row sits, so it wins;
	// the deferred restores unwind in reverse, putting the outer value
	// back once the container closes so siblings are unaffected.
	if flexC && st.Justify != style.AlignNone {
		saved := w.alignForce
		w.alignForce = st.Justify
		defer func() { w.alignForce = saved }()
	}

	// A block-level element's own background fills whole lines while it
	// is open; an inline element's background never does.
	if isBlock && st.OwnBg && st.HasBackdrop {
		savedHas, savedBg := w.hasBlockBg, w.blockBg
		w.hasBlockBg, w.blockBg = true, st.Backdrop
		defer func() { w.hasBlockBg, w.blockBg = savedHas, savedBg }()
	}

	// An element laid inline against its block nature (a flex item, or
	// display:inline on a block tag) gets a single separating space on
	// each side so adjacent items never run together; pendingSpace
	// collapses, so no space doubles or lands at a line edge.
	if flexItem || inlineBoxEl || (tagBlock && !isBlock) {
		w.pendingSpace, w.spaceSt = true, parentSt
		defer func() { w.pendingSpace, w.spaceSt = true, parentSt }()
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
	case (tag == "ul" || tag == "ol") && isBlock && !flexC:
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
	case tag == "li" && isBlock:
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
		if p := prevRenderedSibling(n); p != nil && p.Type == dom.ElementNode && strings.EqualFold(p.Data, "a") {
			// Directly adjacent links (</a><a>, no whitespace between):
			// browsers keep them apart with padding; on a character grid
			// a single separating space keeps them readable and
			// individually clickable.
			w.pendingSpace, w.spaceSt = true, parentSt
		}
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
	case isBlock || flexC:
		hf := tag == "header" || tag == "footer"
		if hf {
			w.hfDepth++
		}
		if isBlock {
			w.blockStart(parentSt)
		}
		w.recordAnchor(n)
		if flexC {
			w.flexRow = true
		}
		w.walkChildren(n, st)
		w.flexRow = false
		if isBlock {
			w.blockEnd(parentSt)
		}
		if hf {
			w.hfDepth--
		}
	default: // inline or unknown: flow children in place
		w.recordAnchor(n)
		w.walkChildren(n, st)
	}
}

// prevRenderedSibling returns n's previous sibling, skipping nodes that
// render nothing (empty text, comments), so adjacency checks see the
// element that actually precedes n in the output.
func prevRenderedSibling(n *dom.Node) *dom.Node {
	for p := n.PrevSibling; p != nil; p = p.PrevSibling {
		if p.Type == dom.TextNode && p.Data == "" {
			continue
		}
		if p.Type != dom.TextNode && p.Type != dom.ElementNode {
			continue
		}
		return p
	}
	return nil
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
