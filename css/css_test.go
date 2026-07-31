package css_test

import (
	"testing"

	"github.com/MhmdShd/pixsurf/cell"
	"github.com/MhmdShd/pixsurf/css"
	"github.com/MhmdShd/pixsurf/dom"
	"github.com/MhmdShd/pixsurf/style"
)

var (
	red   = cell.RGB{R: 255, G: 0, B: 0}
	lime  = cell.RGB{R: 0, G: 255, B: 0}
	blue  = cell.RGB{R: 0, G: 0, B: 255}
	green = cell.RGB{R: 0, G: 128, B: 0}
)

func mustDoc(t *testing.T, body string) *dom.Doc {
	t.Helper()
	d, err := dom.Parse("<html><body>"+body+"</body></html>", "http://x/")
	if err != nil {
		t.Fatal(err)
	}
	return d
}

// first returns the first element with the given tag name.
func first(root *dom.Node, tag string) *dom.Node {
	var out *dom.Node
	dom.Walk(root, func(n *dom.Node) {
		if out == nil && n.Type == dom.ElementNode && n.Data == tag {
			out = n
		}
	})
	return out
}

// resolve computes n's style by resolving the whole ancestor chain, as
// layout would during its top-down walk.
func resolve(e *css.Engine, n *dom.Node) style.Style {
	var walk func(*dom.Node) style.Style
	walk = func(m *dom.Node) style.Style {
		if m == nil || m.Parent == nil {
			return style.Style{}
		}
		ps := walk(m.Parent)
		if m.Type != dom.ElementNode {
			return ps
		}
		return e.Resolve(m, ps)
	}
	return walk(n)
}

func fgOf(t *testing.T, sheets []string, body, tag string) (style.Style, cell.RGB) {
	t.Helper()
	d := mustDoc(t, body)
	e := css.New(sheets)
	s := resolve(e, first(d.Root, tag))
	return s, s.Fg
}

func TestSpecificityOrdering(t *testing.T) {
	tests := []struct {
		name  string
		sheet string
		want  cell.RGB
	}{
		{"id beats class beats tag",
			"#i { color: blue } .c { color: green } span { color: red }", blue},
		{"class beats tag regardless of order",
			".c { color: green } span { color: red }", green},
		{"equal specificity, later wins",
			"span { color: red } span { color: lime }", lime},
		{"equal class specificity, later wins",
			".c { color: red } .c { color: lime }", lime},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, fg := fgOf(t, []string{tt.sheet}, `<span id="i" class="c">x</span>`, "span")
			if !s.HasFg || fg != tt.want {
				t.Errorf("got %+v HasFg=%v, want %+v", fg, s.HasFg, tt.want)
			}
		})
	}
}

func TestLaterSheetWins(t *testing.T) {
	s, fg := fgOf(t, []string{"p { color: red }", "p { color: lime }"}, "<p>x</p>", "p")
	if !s.HasFg || fg != lime {
		t.Errorf("got %+v, want lime", fg)
	}
}

func TestInlineBeatsSheet(t *testing.T) {
	s, fg := fgOf(t, []string{"#i { color: red }"},
		`<span id="i" style="color: blue">x</span>`, "span")
	if !s.HasFg || fg != blue {
		t.Errorf("got %+v, want blue", fg)
	}
}

func TestImportantBeatsInline(t *testing.T) {
	s, fg := fgOf(t, []string{"span { color: red !important }"},
		`<span style="color: blue">x</span>`, "span")
	if !s.HasFg || fg != red {
		t.Errorf("got %+v, want red", fg)
	}
}

func TestInlineImportantBeatsSheetImportant(t *testing.T) {
	s, fg := fgOf(t, []string{"span { color: red !important }"},
		`<span style="color: blue !important">x</span>`, "span")
	if !s.HasFg || fg != blue {
		t.Errorf("got %+v, want blue", fg)
	}
}

func TestColorInheritsBackgroundDoesNot(t *testing.T) {
	d := mustDoc(t, "<div><span>x</span></div>")
	e := css.New([]string{"div { color: red; background-color: blue }"})
	div := resolve(e, first(d.Root, "div"))
	if !div.HasFg || div.Fg != red || !div.HasBg || div.Bg != blue {
		t.Fatalf("div style wrong: %+v", div)
	}
	span := resolve(e, first(d.Root, "span"))
	if !span.HasFg || span.Fg != red {
		t.Errorf("color did not inherit: %+v", span)
	}
	if span.HasBg {
		t.Errorf("background-color inherited but must not: %+v", span)
	}
}

func TestHidden(t *testing.T) {
	body := `<div id="a">x</div>
		<div id="b">x</div>
		<div id="c"><p>child</p></div>
		<div id="d"><p style="visibility: visible">child</p></div>
		<div id="e">x</div>`
	d := mustDoc(t, body)
	e := css.New([]string{
		"#a { display: none }",
		"#b { visibility: hidden }",
		"#c { display: none }",
		"#d { visibility: hidden }",
	})
	byID := map[string]*dom.Node{}
	dom.Walk(d.Root, func(n *dom.Node) {
		if n.Type == dom.ElementNode {
			if id := dom.Attr(n, "id"); id != "" {
				byID[id] = n
			}
		}
	})
	if !e.Hidden(byID["a"]) {
		t.Error("display:none not hidden")
	}
	if !e.Hidden(byID["b"]) {
		t.Error("visibility:hidden not hidden")
	}
	if !e.Hidden(first(byID["c"], "p")) {
		t.Error("child of display:none parent not hidden")
	}
	if e.Hidden(first(byID["d"], "p")) {
		t.Error("visibility:visible child of hidden parent should un-hide")
	}
	if e.Hidden(byID["e"]) {
		t.Error("unstyled element reported hidden")
	}
}

func TestMalformedCSSSkipped(t *testing.T) {
	sheets := []string{
		"p { color: } ?!?bad@@ { color: red } em { color: lime }",
		"div { color: red", // unclosed brace at EOF
		"span::selection { color: red } b { font-weight: normal; color: lime }",
	}
	d := mustDoc(t, "<p>x</p><em>y</em><b>z</b>")
	e := css.New(sheets) // must not panic
	if s := resolve(e, first(d.Root, "p")); s.HasFg {
		t.Errorf("empty color value applied: %+v", s)
	}
	if s := resolve(e, first(d.Root, "em")); !s.HasFg || s.Fg != lime {
		t.Errorf("valid rule after malformed ones dropped: %+v", s)
	}
	if s := resolve(e, first(d.Root, "b")); !s.HasFg || s.Fg != lime || s.Bold {
		t.Errorf("valid rule after pseudo-element rule dropped: %+v", s)
	}
}

func TestRGBColors(t *testing.T) {
	tests := []struct {
		val  string
		want cell.RGB
	}{
		{"rgb(255, 0, 0)", red},
		{"rgb(0,128,0)", green},
		{"rgba(0, 0, 255, 0.5)", blue},
		{"rgba(255,0,0,1)", red},
		{"rgb(0 128 0 / 50%)", green},
		{"rgb(100%, 0%, 0%)", red},
	}
	for _, tt := range tests {
		t.Run(tt.val, func(t *testing.T) {
			s, fg := fgOf(t, []string{"p { color: " + tt.val + " }"}, "<p>x</p>", "p")
			if !s.HasFg || fg != tt.want {
				t.Errorf("got %+v HasFg=%v, want %+v", fg, s.HasFg, tt.want)
			}
		})
	}
}

func TestPropertyMapping(t *testing.T) {
	tests := []struct {
		name  string
		sheet string
		check func(style.Style) bool
	}{
		{"color", "p { color: red }",
			func(s style.Style) bool { return s.HasFg && s.Fg == red }},
		{"background-color", "p { background-color: blue }",
			func(s style.Style) bool { return s.HasBg && s.Bg == blue }},
		{"background shorthand colour", "p { background: url(x.png) no-repeat green }",
			func(s style.Style) bool { return s.HasBg && s.Bg == green }},
		{"font-weight bold", "p { font-weight: bold }",
			func(s style.Style) bool { return s.Bold }},
		{"font-weight 700", "p { font-weight: 700 }",
			func(s style.Style) bool { return s.Bold }},
		{"font-weight 400 clears", "p { font-weight: bold } p { font-weight: 400 }",
			func(s style.Style) bool { return !s.Bold }},
		{"font-style italic", "p { font-style: italic }",
			func(s style.Style) bool { return s.Italic }},
		{"text-decoration underline", "p { text-decoration: underline }",
			func(s style.Style) bool { return s.Underline }},
		{"text-decoration-line line-through", "p { text-decoration-line: line-through }",
			func(s style.Style) bool { return s.Strike }},
		{"text-decoration none clears", "p { text-decoration: underline line-through } p { text-decoration: none }",
			func(s style.Style) bool { return !s.Underline && !s.Strike }},
		{"text-align center", "p { text-align: center }",
			func(s style.Style) bool { return s.Align == style.AlignCenter }},
		{"text-align right", "p { text-align: right }",
			func(s style.Style) bool { return s.Align == style.AlignRight }},
		{"text-transform uppercase", "p { text-transform: uppercase }",
			func(s style.Style) bool { return s.Transform == style.TransformUpper }},
		{"text-transform capitalize", "p { text-transform: capitalize }",
			func(s style.Style) bool { return s.Transform == style.TransformCapitalize }},
		{"white-space pre", "p { white-space: pre }",
			func(s style.Style) bool { return s.Pre }},
		{"white-space pre-wrap", "p { white-space: pre-wrap }",
			func(s style.Style) bool { return s.Pre }},
		{"white-space normal clears", "p { white-space: pre } p { white-space: normal }",
			func(s style.Style) bool { return !s.Pre }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s, _ := fgOf(t, []string{tt.sheet}, "<p>x</p>", "p")
			if !tt.check(s) {
				t.Errorf("style %+v fails check", s)
			}
		})
	}
}

func TestIndexClassAndTagBothApply(t *testing.T) {
	d := mustDoc(t, `<p class="x">t</p>`)
	e := css.New([]string{"p { color: red } .x { font-weight: bold }"})
	s := resolve(e, first(d.Root, "p"))
	if !s.HasFg || s.Fg != red {
		t.Errorf("tag-indexed rule missed: %+v", s)
	}
	if !s.Bold {
		t.Errorf("class-indexed rule missed: %+v", s)
	}
}

func TestMediaQueries(t *testing.T) {
	sheets := []string{
		"@media print { p { color: red } }",
		"@media screen { p { font-weight: bold } }",
		"@media (max-width: 99999px) { p { font-style: italic } }",
		"@supports (display: grid) { p { text-decoration: underline } }",
	}
	s, _ := fgOf(t, sheets, "<p>x</p>", "p")
	if s.HasFg {
		t.Errorf("print-only rule applied: %+v", s)
	}
	if !s.Bold {
		t.Errorf("screen rule dropped: %+v", s)
	}
	if !s.Italic {
		t.Errorf("bare non-print media rule dropped: %+v", s)
	}
	if s.Underline {
		t.Errorf("@supports block should be skipped: %+v", s)
	}
}

func TestDescendantSelector(t *testing.T) {
	d := mustDoc(t, `<div class="wrap"><span>in</span></div><span>out</span>`)
	e := css.New([]string{".wrap span { color: red }"})
	in := resolve(e, first(d.Root, "span"))
	if !in.HasFg || in.Fg != red {
		t.Errorf("descendant rule missed: %+v", in)
	}
	var out *dom.Node
	dom.Walk(d.Root, func(n *dom.Node) {
		if n.Type == dom.ElementNode && n.Data == "span" && n.Parent != nil && n.Parent.Data == "body" {
			out = n
		}
	})
	if s := resolve(e, out); s.HasFg {
		t.Errorf("descendant rule wrongly matched outside span: %+v", s)
	}
}
