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
	if w.pre {
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

// emitWord appends one word, breaking the line when it doesn't fit and
// hard-splitting words wider than the line.
func (w *walker) emitWord(word string, st style.Style) {
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

// emitPre appends verbatim text: newlines break lines, no wrapping,
// overflow clipped at width.
func (w *walker) emitPre(text string, st style.Style) {
	for i, seg := range strings.Split(text, "\n") {
		if i > 0 {
			w.startLine() // materialize even blank pre lines
			w.flushLine()
		}
		for _, r := range seg {
			rw := runewidth.RuneWidth(r)
			if w.col+rw > w.width && w.started {
				break // clip, don't wrap
			}
			w.putRune(r, st)
		}
	}
}

// putRune appends one styled cell to the current line. It never wraps;
// callers manage line breaks.
func (w *walker) putRune(r rune, st style.Style) {
	if !w.started {
		w.startLine()
	}
	if w.linkURL != "" && w.linkOpen < 0 {
		w.linkOpen = w.col
	}
	w.line = append(w.line, styledCell(r, st))
	w.col += runewidth.RuneWidth(r)
}

// startLine begins a new output line: materializes a pending blank
// separator and writes the blockquote indent.
func (w *walker) startLine() {
	if w.pendingBlank && len(w.doc.Lines) > 0 {
		w.doc.Lines = append(w.doc.Lines, nil)
	}
	w.pendingBlank = false
	w.started = true
	for i := 0; i < 2*w.quote; i++ {
		w.line = append(w.line, cell.Cell{Rune: ' ', Dim: true})
		w.col++
	}
	w.lineBase = w.col
}

// flushLine closes any open link range and appends the current line.
func (w *walker) flushLine() {
	if !w.started {
		return
	}
	w.closeLinkRange()
	w.doc.Lines = append(w.doc.Lines, w.line)
	w.line = nil
	w.col = 0
	w.lineBase = 0
	w.started = false
	w.pendingSpace = false
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
