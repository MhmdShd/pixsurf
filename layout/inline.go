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
			w.pendingSpace, w.spaceSt = true, st
		}
		return
	}
	if w.hasContent() && startsWithSpace(text) {
		w.pendingSpace, w.spaceSt = true, st
	}
	words := strings.Fields(text)
	for i, word := range words {
		if i > 0 {
			w.pendingSpace, w.spaceSt = true, st
		}
		w.emitWord(word, st)
	}
	if endsWithSpace(text) {
		w.pendingSpace, w.spaceSt = true, st
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
		// The separator paints with the style it arose in, so a space
		// preceding an inline-background element (a small button, say)
		// is not painted with that element's background.
		w.putRune(' ', w.spaceSt)
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
	st = w.ensureContrast(st)
	w.hasStyleBg, w.styleBg = st.HasBackdrop, st.Backdrop
	w.lineAlign = st.Align
	if w.alignForce != style.AlignNone {
		w.lineAlign = w.alignForce
	}
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

// appendBlank materializes one blank separator line. When the blank's
// containing sheet carries a backdrop (recorded by requestBlank) — a
// painted block or the page background — the blank is painted full
// content width with that colour so a background sheet has no dark
// gaps; otherwise it stays a zero-length line exactly as before. The
// fill deliberately never comes from a content cell: neither an image's
// pixel colours nor a painted button at the block boundary must smear
// into the separator. Skipped styling while measuring: painting would
// inflate natural cell widths.
func (w *walker) appendBlank() {
	if !w.measuring && w.blankHasBg {
		ln := make([]cell.Cell, w.width)
		for i := range ln {
			ln[i] = cell.Cell{Rune: ' ', HasBg: true, Bg: w.blankBg}
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

// fillBackground makes a line inside a painted block render as a solid
// band: cells without a background of their own (indent, alignment
// padding) take the block background, and the line is padded out to the
// content width with it. Only a BLOCK-level background fills the line —
// the page sheet or a block element's own background, tracked in
// w.blockBg — so an inline element's background (a small button, say)
// stays behind its own glyphs and never smears across the line. Lines
// with no block background in effect are left exactly as built.
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
	if !w.hasBlockBg {
		return
	}
	for i := range w.line {
		if !w.line[i].HasBg {
			w.line[i].HasBg = true
			w.line[i].Bg = w.blockBg
		}
	}
	for w.col < w.width {
		w.line = append(w.line, cell.Cell{Rune: ' ', HasBg: true, Bg: w.blockBg})
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

// minLumDiff is the smallest relative-luminance difference between a
// glyph's foreground and its backdrop that still counts as readable.
const minLumDiff = 0.30

// relLuminance is a simple relative luminance in [0,1] (ITU-R BT.709
// coefficients on 8-bit channels; no gamma — a threshold is all we need).
func relLuminance(c cell.RGB) float64 {
	return (0.2126*float64(c.R) + 0.7152*float64(c.G) + 0.0722*float64(c.B)) / 255
}

// readableOn is a guaranteed-readable foreground for a backdrop:
// near-black on light sheets, near-white on dark ones.
func readableOn(bg cell.RGB) cell.RGB {
	if relLuminance(bg) >= 0.5 {
		return cell.RGB{R: 0x11, G: 0x11, B: 0x11}
	}
	return cell.RGB{R: 0xee, G: 0xee, B: 0xee}
}

// ensureContrast is the final safety net before a glyph cell is built:
// no text may ever be painted in a colour close to its own backdrop. A
// declared foreground within minLumDiff of the backdrop is replaced with
// a readable one derived from it (and counted in doc.ContrastFixes); a
// cell with a backdrop but no declared foreground gets the derived
// foreground too, since the terminal's default foreground has unknown
// contrast against a page-painted sheet. Cells without a known backdrop
// are left alone — both colours are the terminal's own defaults.
func (w *walker) ensureContrast(st style.Style) style.Style {
	if !st.HasBackdrop {
		return st
	}
	if !st.HasFg {
		st.HasFg, st.Fg = true, readableOn(st.Backdrop)
		return st
	}
	d := relLuminance(st.Fg) - relLuminance(st.Backdrop)
	if d < 0 {
		d = -d
	}
	if d < minLumDiff {
		st.Fg = readableOn(st.Backdrop)
		w.doc.ContrastFixes++
	}
	return st
}

// styledCell builds one display cell. The cell's painted background is
// the style's backdrop — the element's own background when declared,
// else the sheet showing through from an ancestor — never the raw
// cascade value, so transparent elements sit on the right colour.
func styledCell(r rune, st style.Style) cell.Cell {
	return cell.Cell{
		Rune:      r,
		Fg:        st.Fg,
		Bg:        st.Backdrop,
		HasFg:     st.HasFg,
		HasBg:     st.HasBackdrop,
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
