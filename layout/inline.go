package layout

import (
	"strings"
	"unicode"

	"github.com/mattn/go-runewidth"

	"github.com/MhmdShd/pixsurf/cell"
	"github.com/MhmdShd/pixsurf/style"
)

// emitText flows a text run into the current line(s). Outside <pre>, any run
// of whitespace collapses to a single pending space; inside <pre>, text is
// emitted verbatim line-by-line and clipped at width.
func (w *walker) emitText(text string, st style.Style) {
	text = transformText(text, st.Transform)
	if w.pre || st.Pre {
		w.emitPre(text, st)
		return
	}
	if strings.TrimSpace(text) == "" {
		if text != "" && w.hasContent() {
			w.pendingSpace = true
		}
		return
	}
	if w.hasContent() && startsWithSpace(text) {
		w.pendingSpace = true
	}
	words := strings.Fields(text)
	for i, word := range words {
		if i > 0 {
			w.pendingSpace = true
		}
		w.emitWord(word, st)
	}
	if endsWithSpace(text) {
		w.pendingSpace = true
	}
}

// transformText applies a CSS text-transform to a text run before it is
// split into words, so wrapping and width measurement see the final runes.
func transformText(text string, t style.Transform) string {
	switch t {
	case style.TransformUpper:
		return strings.ToUpper(text)
	case style.TransformLower:
		return strings.ToLower(text)
	case style.TransformCapitalize:
		prevLetter := false
		return strings.Map(func(r rune) rune {
			isLetter := unicode.IsLetter(r) || unicode.IsDigit(r)
			if isLetter && !prevLetter {
				r = unicode.ToUpper(r)
			}
			prevLetter = isLetter
			return r
		}, text)
	}
	return text
}

// emitWord appends one word, breaking the line when it doesn't fit and
// hard-splitting words wider than the line.
func (w *walker) emitWord(word string, st style.Style) {
	w.joinNextBlock = false // inline text ends any bullet-joining window
	wl := runewidth.StringWidth(word)
	sp := 0
	if w.pendingSpace && w.hasContent() {
		sp = 1
	}
	w.pendingSpace = false
	if w.hasContent() && w.col+sp+wl > w.width {
		w.flushLine()
		sp = 0
	}
	if sp == 1 {
		w.putRune(' ', st)
	}
	for _, r := range word {
		rw := runewidth.RuneWidth(r)
		if w.hasContent() && w.col+rw > w.width {
			w.flushLine() // word wider than the line: hard split
		}
		w.putRune(r, st)
	}
}

// emitPre appends verbatim text: newlines break lines, tabs expand to the
// next 4-column stop, no wrapping, overflow clipped at width.
func (w *walker) emitPre(text string, st style.Style) {
	for i, seg := range strings.Split(text, "\n") {
		if i > 0 {
			w.startLine() // materialize even blank pre lines
			w.flushLine()
		}
		for _, r := range seg {
			if r == '\t' {
				if !w.started {
					w.startLine()
				}
				for next := (w.col-w.lineBase)/4*4 + 4 + w.lineBase; w.col < next && w.col < w.width; {
					w.putRune(' ', st)
				}
				continue
			}
			rw := runewidth.RuneWidth(r)
			if w.started && w.col+rw > w.width {
				break // clip, don't wrap
			}
			w.putRune(r, st)
		}
	}
}

// putRune appends one styled cell to the current line. It never wraps;
// callers manage line breaks. Wide runes are followed by explicit
// continuation cells so slice index == display column holds everywhere.
// Zero-width runes (combining marks) are dropped to keep that invariant.
func (w *walker) putRune(r rune, st style.Style) {
	rw := runewidth.RuneWidth(r)
	if rw == 0 {
		return
	}
	if !w.started {
		w.startLine()
	}
	w.hasStyleBg, w.styleBg = st.HasBg, st.Bg
	w.lineAlign = st.Align
	if w.linkURL != "" && w.linkOpen < 0 {
		w.linkOpen = w.col
	}
	w.line = append(w.line, styledCell(r, st))
	for i := 1; i < rw; i++ {
		c := styledCell(0, st)
		c.Continuation = true
		w.line = append(w.line, c)
	}
	w.col += rw
}

// appendBlank materializes one blank separator line. When a style-derived
// background is in effect — a painted block or the page background — the
// blank is painted full content width with that color so a background
// sheet has no dark gaps; otherwise it stays a zero-length line exactly
// as before. The fill deliberately never comes from a content cell: an
// image's pixel colours must not smear into the separator. Skipped
// styling while measuring: painting would inflate natural cell widths.
func (w *walker) appendBlank() {
	if !w.measuring && w.hasStyleBg {
		ln := make([]cell.Cell, w.width)
		for i := range ln {
			ln[i] = cell.Cell{Rune: ' ', HasBg: true, Bg: w.styleBg}
		}
		w.doc.Lines = append(w.doc.Lines, ln)
		return
	}
	w.doc.Lines = append(w.doc.Lines, nil)
}

// startLine begins a new output line: materializes a pending blank
// separator and writes the blockquote indent.
func (w *walker) startLine() {
	if w.pendingBlank && len(w.doc.Lines) > 0 {
		w.appendBlank()
	}
	w.pendingBlank = false
	w.started = true
	for i := 0; i < w.indentCols(); i++ {
		w.line = append(w.line, cell.Cell{Rune: ' ', Dim: true})
		w.col++
	}
	w.lineBase = w.col
}

// indentCols is the blockquote indent, clamped to leave at least one
// content column.
func (w *walker) indentCols() int {
	ind := 2 * w.quote
	if ind > w.width-1 {
		ind = w.width - 1
	}
	if ind < 0 {
		ind = 0
	}
	return ind
}

// flushLine aligns the line, closes any open link range and appends it.
func (w *walker) flushLine() {
	if !w.started {
		return
	}
	w.alignLine()
	w.closeLinkRange()
	w.fillBackground()
	w.doc.Lines = append(w.doc.Lines, w.line)
	w.line = nil
	w.linePixels = false
	w.lineAlign = style.AlignNone
	w.col = 0
	w.lineBase = 0
	w.started = false
	w.pendingSpace = false
}

// alignLine shifts a centred or right-aligned line within the content
// width by padding on the left after the indent. Every range already
// recorded on this line — links, field boxes, submit regions, and the
// still-open link start — moves by the same shift so hit-testing stays
// correct. Inserted cells are plain spaces (index == column holds);
// fillBackground later paints them when the line carries a background.
// Skipped while measuring: the pad would inflate natural cell widths.
func (w *walker) alignLine() {
	if w.measuring {
		return
	}
	var shift int
	switch w.lineAlign {
	case style.AlignCenter:
		shift = (w.width - w.col) / 2
	case style.AlignRight:
		shift = w.width - w.col
	default:
		return
	}
	if shift <= 0 {
		return
	}
	pad := make([]cell.Cell, shift)
	for i := range pad {
		pad[i] = cell.Cell{Rune: ' '}
	}
	w.line = append(w.line[:w.lineBase], append(pad, w.line[w.lineBase:]...)...)
	w.col += shift
	if w.linkOpen >= 0 {
		w.linkOpen += shift
	}
	ln := len(w.doc.Lines)
	for i := range w.doc.Links {
		if w.doc.Links[i].Line == ln {
			w.doc.Links[i].Start += shift
			w.doc.Links[i].End += shift
		}
	}
	for i := range w.doc.Fields {
		if w.doc.Fields[i].Line == ln {
			w.doc.Fields[i].Start += shift
			w.doc.Fields[i].End += shift
		}
	}
	for i := range w.doc.Forms {
		if w.doc.Forms[i].SubmitLine == ln {
			w.doc.Forms[i].SubmitStart += shift
			w.doc.Forms[i].SubmitEnd += shift
		}
	}
}

// fillBackground makes a line that carries a background color render as a
// solid band: leading indent cells take the first content background, and
// the line is padded out to the content width with the background active
// at its end. Lines with no background cell are left exactly as built.
// Skipped while measuring natural cell widths: the padding would make
// every background line look content-width wide.
func (w *walker) fillBackground() {
	if w.measuring {
		return
	}
	if w.linePixels {
		// Image pixel cells carry per-pixel colours, not a style
		// background: they are excluded from consideration entirely.
		// Pad (and paint the leading indent) only with the style
		// background in effect; with none, the line stays as built.
		if !w.hasStyleBg {
			return
		}
		for i := range w.line {
			if !w.line[i].HasBg {
				w.line[i].HasBg = true
				w.line[i].Bg = w.styleBg
			}
		}
		for w.col < w.width {
			w.line = append(w.line, cell.Cell{Rune: ' ', HasBg: true, Bg: w.styleBg})
			w.col++
		}
		return
	}
	first, last := -1, -1
	for i, c := range w.line {
		if c.HasBg {
			if first < 0 {
				first = i
			}
			last = i
		}
	}
	if first < 0 {
		return
	}
	for i := 0; i < first; i++ {
		if !w.line[i].HasBg {
			w.line[i].HasBg = true
			w.line[i].Bg = w.line[first].Bg
		}
	}
	bg := w.line[last].Bg
	for w.col < w.width {
		w.line = append(w.line, cell.Cell{Rune: ' ', HasBg: true, Bg: bg})
		w.col++
	}
}

// closeLinkRange records the open link's range on the current line, if any.
// The range reopens lazily at the next emitted cell while the link is open.
func (w *walker) closeLinkRange() {
	if w.linkOpen >= 0 && w.col > w.linkOpen {
		w.doc.Links = append(w.doc.Links, Link{
			Line:  len(w.doc.Lines),
			Start: w.linkOpen,
			End:   w.col,
			URL:   w.linkURL,
		})
	}
	w.linkOpen = -1
}

// hasContent reports whether the current line holds content beyond indent.
func (w *walker) hasContent() bool {
	return w.started && w.col > w.lineBase
}

func styledCell(r rune, st style.Style) cell.Cell {
	return cell.Cell{
		Rune:      r,
		Fg:        st.Fg,
		Bg:        st.Bg,
		HasFg:     st.HasFg,
		HasBg:     st.HasBg,
		Bold:      st.Bold,
		Italic:    st.Italic,
		Underline: st.Underline,
		Strike:    st.Strike,
		Reverse:   st.Reverse,
		Dim:       st.Dim,
	}
}

func startsWithSpace(s string) bool {
	for _, r := range s {
		return unicode.IsSpace(r)
	}
	return false
}

func endsWithSpace(s string) bool {
	rs := []rune(s)
	return len(rs) > 0 && unicode.IsSpace(rs[len(rs)-1])
}
