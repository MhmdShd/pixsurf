package layout

import (
	"net/url"
	"strconv"
	"strings"

	"github.com/mattn/go-runewidth"

	"github.com/MhmdShd/pixsurf/dom"
	"github.com/MhmdShd/pixsurf/style"
)

// Text-field box content width bounds (excluding the [ ] brackets).
const (
	defaultBoxWidth = 20
	maxBoxWidth     = 30
)

// Field is a rendered text-input box on one line.
type Field struct {
	Name    string
	Value   string // current text (prefilled from value attr or values map)
	Line    int    // box region for hit-testing
	Start   int
	End     int // exclusive
	FormIdx int
}

// Form is an extracted <form>.
type Form struct {
	Action                             string     // resolved absolute URL; page URL when no action attr
	Method                             string     // "get" or "post" (lowercased); only get is submittable
	Hidden                             url.Values // name→value pairs from <input type=hidden>
	SubmitLine, SubmitStart, SubmitEnd int        // -1s when no visible submit button
}

// FormValues holds user-entered field values, keyed by ValuesKey.
type FormValues map[string]string

// ValuesKey is the FormValues key for a field of a form.
func ValuesKey(action, name string) string {
	return action + "\x00" + name
}

// FieldAt returns the index into Fields of the field box covering
// (line, col), if any.
func (d *Document) FieldAt(line, col int) (int, bool) {
	for i, f := range d.Fields {
		if f.Line == line && col >= f.Start && col < f.End {
			return i, true
		}
	}
	return 0, false
}

// SubmitAt returns the index into Forms of the form whose submit button
// covers (line, col), if any.
func (d *Document) SubmitAt(line, col int) (int, bool) {
	for i, f := range d.Forms {
		if f.SubmitLine == line && col >= f.SubmitStart && col < f.SubmitEnd {
			return i, true
		}
	}
	return 0, false
}

// renderForm extracts a <form> and flows its children with the form open.
func (w *walker) renderForm(n *dom.Node, st style.Style) {
	w.blockStart()
	w.recordAnchor(n)
	f := Form{
		Method:      "get",
		Hidden:      url.Values{},
		SubmitLine:  -1,
		SubmitStart: -1,
		SubmitEnd:   -1,
	}
	if a := strings.TrimSpace(dom.Attr(n, "action")); a != "" {
		f.Action = w.src.Resolve(a)
	} else {
		f.Action = w.src.Base.String()
	}
	if m := strings.TrimSpace(dom.Attr(n, "method")); m != "" {
		f.Method = strings.ToLower(m)
	}
	w.doc.Forms = append(w.doc.Forms, f)
	prev := w.formIdx
	w.formIdx = len(w.doc.Forms) - 1
	w.walkChildren(n, st)
	w.formIdx = prev
	w.blockEnd()
}

// emitInput dispatches one <input> by type. Inputs outside any <form>
// are skipped (no orphan fields in v0.3).
func (w *walker) emitInput(n *dom.Node, st style.Style) {
	if w.formIdx < 0 {
		return
	}
	name := dom.Attr(n, "name")
	switch strings.ToLower(strings.TrimSpace(dom.Attr(n, "type"))) {
	case "", "text", "search":
		w.emitFieldBox(n, name, st)
	case "submit":
		label := strings.TrimSpace(dom.Attr(n, "value"))
		if label == "" {
			label = "Submit"
		}
		w.emitSubmit(label, st)
	case "hidden":
		if name != "" {
			w.doc.Forms[w.formIdx].Hidden.Add(name, dom.Attr(n, "value"))
		}
	default:
		// checkbox/radio/password/etc.: not rendered in v0.3
	}
}

// emitButton renders a <button> as a submit button labeled with its text.
func (w *walker) emitButton(n *dom.Node, st style.Style) {
	if w.formIdx < 0 {
		return
	}
	var b strings.Builder
	dom.Walk(n, func(c *dom.Node) {
		if c.Type == dom.TextNode {
			b.WriteString(c.Data)
		}
	})
	label := strings.Join(strings.Fields(b.String()), " ")
	if label == "" {
		label = "Submit"
	}
	w.emitSubmit(label, st)
}

// emitFieldBox draws the box for one text-style <input>. The values map
// (via ValuesKey) wins over the value attribute.
func (w *walker) emitFieldBox(n *dom.Node, name string, st style.Style) {
	form := &w.doc.Forms[w.formIdx]
	val := dom.Attr(n, "value")
	if v, ok := w.values[ValuesKey(form.Action, name)]; ok {
		val = v
	}
	w.emitFieldValueBox(name, val, w.boxWidth(n, "size"), st)
}

// emitTextarea renders a <textarea> as a single-line field box: visible,
// clickable, fillable, and submitted like a text input (no multi-line
// editing in v0.5). Its initial value is its text content, which is
// consumed here and never re-emitted as page text. Textareas outside any
// <form> are skipped, matching emitInput.
func (w *walker) emitTextarea(n *dom.Node, st style.Style) {
	if w.formIdx < 0 {
		return
	}
	name := dom.Attr(n, "name")
	var b strings.Builder
	dom.Walk(n, func(c *dom.Node) {
		if c.Type == dom.TextNode {
			b.WriteString(c.Data)
		}
	})
	val := strings.TrimSpace(b.String())
	form := &w.doc.Forms[w.formIdx]
	if v, ok := w.values[ValuesKey(form.Action, name)]; ok {
		val = v
	}
	w.emitFieldValueBox(name, val, w.boxWidth(n, "cols"), st)
}

// boxWidth resolves a field box's content width from the given attribute
// (input size / textarea cols), clamped to the line and the global cap.
func (w *walker) boxWidth(n *dom.Node, attr string) int {
	boxW := defaultBoxWidth
	if s := strings.TrimSpace(dom.Attr(n, attr)); s != "" {
		if sz, err := strconv.Atoi(s); err == nil && sz > 0 {
			boxW = sz
		}
	}
	if boxW > maxBoxWidth {
		boxW = maxBoxWidth
	}
	if m := w.width - w.indentCols() - 2; boxW > m {
		boxW = m
	}
	if boxW < 1 {
		boxW = 1
	}
	return boxW
}

// emitFieldValueBox draws a reverse-video [value_____] box and records
// the field with its hit-testable region; a too-long value shows its tail.
func (w *walker) emitFieldValueBox(name, val string, boxW int, st style.Style) {
	w.fitRun(boxW+2, st)
	st.Reverse = true
	w.putRune('[', st)
	line := len(w.doc.Lines)
	start := w.col - 1
	shown := boxW
	for _, r := range tailClip(val, boxW) {
		w.putRune(r, st)
		shown -= runewidth.RuneWidth(r)
	}
	for ; shown > 0; shown-- {
		w.putRune('_', st)
	}
	w.putRune(']', st)

	w.doc.Fields = append(w.doc.Fields, Field{
		Name:    name,
		Value:   val,
		Line:    line,
		Start:   start,
		End:     w.col,
		FormIdx: w.formIdx,
	})
}

// emitSubmit draws a bold [ label ] button and records it as its form's
// submit region (first one wins).
func (w *walker) emitSubmit(label string, st style.Style) {
	form := &w.doc.Forms[w.formIdx]
	text := "[ " + label + " ]"
	st.Bold = true
	w.fitRun(runewidth.StringWidth(text), st)
	line, start := -1, 0
	for _, r := range text {
		w.putRune(r, st)
		if line < 0 {
			line = len(w.doc.Lines)
			start = w.col - runewidth.RuneWidth(r)
		}
	}
	if form.SubmitLine < 0 && line >= 0 {
		form.SubmitLine = line
		form.SubmitStart = start
		form.SubmitEnd = w.col
	}
}

// fitRun handles word-style spacing for an unbreakable run of the given
// display width: emits the owed separator space and breaks the line when
// the run would not fit.
func (w *walker) fitRun(width int, st style.Style) {
	sp := 0
	if w.pendingSpace && w.hasContent() {
		sp = 1
	}
	w.pendingSpace = false
	if w.hasContent() && w.col+sp+width > w.width {
		w.flushLine()
		sp = 0
	}
	if sp == 1 {
		// the separator space carries only the surrounding background so a
		// filled block stays solid; other attributes stay off as before
		w.putRune(' ', style.Style{Bg: st.Bg, HasBg: st.HasBg})
	}
}

// tailClip returns the suffix of s whose display width fits in max.
func tailClip(s string, max int) string {
	if runewidth.StringWidth(s) <= max {
		return s
	}
	rs := []rune(s)
	width := 0
	for i := len(rs) - 1; i >= 0; i-- {
		width += runewidth.RuneWidth(rs[i])
		if width > max {
			return string(rs[i+1:])
		}
	}
	return s
}
