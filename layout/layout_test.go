package layout

import (
	"image"
	"image/color"
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
		if c.Continuation {
			continue
		}
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

func TestTableCaption(t *testing.T) {
	d := doc(t, "<table><caption>Monthly Fees</caption><tr><td>aa</td><td>bb</td></tr></table>", 30)
	txt := allText(d)
	if !strings.Contains(txt, "Monthly Fees") {
		t.Errorf("caption content missing:\n%s", txt)
	}
	if !strings.Contains(txt, "aa") || !strings.Contains(txt, "bb") {
		t.Errorf("row content missing:\n%s", txt)
	}
	if strings.Index(txt, "Monthly Fees") > strings.Index(txt, "aa") {
		t.Errorf("caption must precede rows:\n%s", txt)
	}
}

func TestTableManyColumnsClipped(t *testing.T) {
	var b strings.Builder
	b.WriteString("<table><tr>")
	for i := 0; i < 30; i++ {
		b.WriteString("<td>x</td>")
	}
	b.WriteString("</tr></table>")
	d := doc(t, b.String(), 20)
	for i, ln := range d.Lines {
		if len(ln) > 20 {
			t.Errorf("line %d has %d cells, want <= 20", i, len(ln))
		}
	}
	if !strings.Contains(allText(d), "x") {
		t.Errorf("row content entirely missing:\n%s", allText(d))
	}
	for _, l := range d.Links {
		if l.End > 20 || l.Start >= 20 {
			t.Errorf("link range beyond width: %+v", l)
		}
	}
}
