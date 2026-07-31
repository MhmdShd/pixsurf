package layout

import (
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
