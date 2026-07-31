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
	return docV(t, src, width, nil)
}

func docV(t *testing.T, src string, width int, values FormValues) *Document {
	t.Helper()
	d, err := dom.Parse(src, "https://example.org/page")
	if err != nil {
		t.Fatal(err)
	}
	return Render(d, width, nil, values)
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
	out := Render(d, 20, fetch, nil)
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

func TestFormExtraction(t *testing.T) {
	d := doc(t, `<form action="/search" method="GET">
		<input type="hidden" name="lang" value="en">
		<input type="text" name="q" value="cats">
		<input type="submit" value="Go">
	</form>`, 60)
	if len(d.Forms) != 1 {
		t.Fatalf("forms = %d, want 1", len(d.Forms))
	}
	f := d.Forms[0]
	if f.Action != "https://example.org/search" {
		t.Errorf("action = %q", f.Action)
	}
	if f.Method != "get" {
		t.Errorf("method = %q, want lowercased get", f.Method)
	}
	if got := f.Hidden.Get("lang"); got != "en" {
		t.Errorf("hidden lang = %q", got)
	}
	if len(d.Fields) != 1 {
		t.Fatalf("fields = %d, want 1 (hidden/submit are not Fields)", len(d.Fields))
	}
	fl := d.Fields[0]
	if fl.Name != "q" || fl.Value != "cats" || fl.FormIdx != 0 {
		t.Errorf("field = %+v", fl)
	}
	if f.SubmitLine < 0 || f.SubmitEnd <= f.SubmitStart {
		t.Fatalf("submit region unset: %+v", f)
	}
	if !strings.Contains(lineText(d, f.SubmitLine), "[ Go ]") {
		t.Errorf("submit line = %q, want [ Go ]", lineText(d, f.SubmitLine))
	}
	if idx, ok := d.SubmitAt(f.SubmitLine, f.SubmitStart); !ok || idx != 0 {
		t.Errorf("SubmitAt(start) = %d,%v", idx, ok)
	}
	if idx, ok := d.SubmitAt(f.SubmitLine, f.SubmitEnd-1); !ok || idx != 0 {
		t.Errorf("SubmitAt(end-1) = %d,%v", idx, ok)
	}
	if _, ok := d.SubmitAt(f.SubmitLine, f.SubmitEnd); ok {
		t.Error("SubmitAt(end) should miss (End exclusive)")
	}
}

func TestFormDefaults(t *testing.T) {
	d := doc(t, `<form><input name="a"><input type="text"></form>`, 60)
	if len(d.Forms) != 1 {
		t.Fatalf("forms = %d, want 1", len(d.Forms))
	}
	f := d.Forms[0]
	if f.Action != "https://example.org/page" {
		t.Errorf("default action = %q, want page URL", f.Action)
	}
	if f.Method != "get" {
		t.Errorf("default method = %q, want get", f.Method)
	}
	if f.SubmitLine != -1 || f.SubmitStart != -1 || f.SubmitEnd != -1 {
		t.Errorf("no submit button: region should be -1s, got %+v", f)
	}
	// input without type → text field; nameless input rendered with Name ""
	if len(d.Fields) != 2 {
		t.Fatalf("fields = %d, want 2", len(d.Fields))
	}
	if d.Fields[0].Name != "a" {
		t.Errorf("field0 name = %q", d.Fields[0].Name)
	}
	if d.Fields[1].Name != "" {
		t.Errorf("field1 name = %q, want empty (nameless)", d.Fields[1].Name)
	}
	// both boxes rendered
	if got := strings.Count(allText(d), "["); got != 2 {
		t.Errorf("box count = %d, want 2:\n%s", got, allText(d))
	}
}

func TestFieldBoxRendering(t *testing.T) {
	// padded: value shorter than box width
	d := doc(t, `<form><input type="text" name="q" size="5" value="ab"></form>`, 60)
	if !strings.Contains(allText(d), "[ab___]") {
		t.Errorf("padded box missing:\n%s", allText(d))
	}
	fl := d.Fields[0]
	// reverse style inside the box
	c := d.Lines[fl.Line][fl.Start]
	if !c.Reverse {
		t.Error("box cell not reverse-video")
	}
	// hit-test: inside hits, outside misses
	if idx, ok := d.FieldAt(fl.Line, fl.Start); !ok || idx != 0 {
		t.Errorf("FieldAt(start) = %d,%v", idx, ok)
	}
	if idx, ok := d.FieldAt(fl.Line, fl.End-1); !ok || idx != 0 {
		t.Errorf("FieldAt(end-1) = %d,%v", idx, ok)
	}
	if _, ok := d.FieldAt(fl.Line, fl.End); ok {
		t.Error("FieldAt(end) should miss (End exclusive)")
	}
	if fl.Start > 0 {
		if _, ok := d.FieldAt(fl.Line, fl.Start-1); ok {
			t.Error("FieldAt(start-1) should miss")
		}
	}

	// long value shows the tail
	d2 := doc(t, `<form><input type="text" name="q" size="5" value="abcdefgh"></form>`, 60)
	if !strings.Contains(allText(d2), "[defgh]") {
		t.Errorf("long value must clip to tail:\n%s", allText(d2))
	}
	if d2.Fields[0].Value != "abcdefgh" {
		t.Errorf("Field.Value = %q, want full value", d2.Fields[0].Value)
	}
}

func TestFormValuesPrefill(t *testing.T) {
	values := FormValues{ValuesKey("https://example.org/search", "q"): "stored"}
	d := docV(t, `<form action="/search"><input type="search" name="q" value="orig"></form>`, 60, values)
	if !strings.Contains(allText(d), "stored") {
		t.Errorf("stored value not rendered:\n%s", allText(d))
	}
	if strings.Contains(allText(d), "orig") {
		t.Errorf("value attr must lose to values map:\n%s", allText(d))
	}
	if d.Fields[0].Value != "stored" {
		t.Errorf("Field.Value = %q, want stored", d.Fields[0].Value)
	}
}

func TestPostFormFlagged(t *testing.T) {
	d := doc(t, `<form action="/go" method="POST"><input type="text" name="q" value="x"></form>`, 60)
	if len(d.Forms) != 1 {
		t.Fatalf("forms = %d, want 1", len(d.Forms))
	}
	if d.Forms[0].Method != "post" {
		t.Errorf("method = %q, want post", d.Forms[0].Method)
	}
	// fields still extracted and rendered
	if len(d.Fields) != 1 {
		t.Fatalf("fields = %d, want 1", len(d.Fields))
	}
	if !strings.Contains(allText(d), "[x") {
		t.Errorf("field box not rendered:\n%s", allText(d))
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

func TestContentRootPrefersMain(t *testing.T) {
	nav := `<nav><a href="/a">HomeLink</a> <a href="/b">AboutLink</a> <a href="/c">MenuLink</a></nav>`
	para := strings.Repeat("the quick brown fox jumps over the lazy dog ", 10)
	d := doc(t, nav+`<main><p>`+para+`</p></main>`, 100)
	txt := allText(d)
	if !strings.Contains(txt, "quick brown fox") {
		t.Errorf("main paragraph missing:\n%s", txt)
	}
	if strings.Contains(txt, "HomeLink") || strings.Contains(txt, "AboutLink") {
		t.Errorf("nav chrome leaked into output:\n%s", txt)
	}
}

func TestContentRootFallsBackWhenMainTiny(t *testing.T) {
	body := strings.Repeat("plenty of real body prose here ", 20)
	d := doc(t, `<main><p>tiny</p></main><p>`+body+`</p>`, 100)
	txt := allText(d)
	if !strings.Contains(txt, "plenty of real body prose") {
		t.Errorf("body text lost when main is tiny:\n%s", txt)
	}
}

func TestChromeSkipNotAppliedWhenPageIsAllChrome(t *testing.T) {
	d := doc(t, `<nav><a href="/one">first link</a> <a href="/two">second link</a></nav>`, 60)
	txt := allText(d)
	if !strings.Contains(txt, "first link") || !strings.Contains(txt, "second link") {
		t.Errorf("all-chrome page must still render (safety valve):\n%s", txt)
	}
}

func TestNoOrphanBullets(t *testing.T) {
	d := doc(t, "<ul><li><div>alpha</div></li><li><p>beta</p></li></ul>", 40)
	txt := allText(d)
	if !strings.Contains(txt, "• alpha") || !strings.Contains(txt, "• beta") {
		t.Errorf("bullets not joined to block content:\n%s", txt)
	}
	for i := range d.Lines {
		if strings.TrimSpace(lineText(d, i)) == "•" {
			t.Errorf("line %d is a bare bullet:\n%s", i, txt)
		}
	}
}

func TestHeaderInsideMainKept(t *testing.T) {
	body := strings.Repeat("long enough article body text to make main the content root ", 5)
	d := doc(t, `<main><header><h1>The Article Title</h1><p>By Jane</p></header><p>`+body+`</p></main>`, 100)
	txt := allText(d)
	for _, want := range []string{"The Article Title", "By Jane", "long enough article body"} {
		if !strings.Contains(txt, want) {
			t.Errorf("missing %q (header inside main must not be chrome):\n%s", want, txt)
		}
	}
}

func TestLinkFarmInHeaderSkipped(t *testing.T) {
	var farm strings.Builder
	for i := 0; i < 12; i++ {
		farm.WriteString(`<li><a href="/l">FarmLink</a></li>`)
	}
	body := strings.Repeat("long enough article body text to make main the content root ", 5)
	d := doc(t, `<main><header><h1>The Title</h1><p>By Jane</p><ul>`+farm.String()+
		`</ul></header><p>`+body+`</p></main>`, 100)
	txt := allText(d)
	if !strings.Contains(txt, "The Title") || !strings.Contains(txt, "By Jane") {
		t.Errorf("header content lost:\n%s", txt)
	}
	if strings.Contains(txt, "FarmLink") {
		t.Errorf("link-farm list inside header must be skipped:\n%s", txt)
	}
}

func assertNoBareBullets(t *testing.T, d *Document) {
	t.Helper()
	for i := range d.Lines {
		if strings.TrimSpace(lineText(d, i)) == "•" {
			t.Errorf("line %d is a bare bullet:\n%s", i, allText(d))
		}
	}
}

func TestNoOrphanBulletsDeepWrap(t *testing.T) {
	d := doc(t, "<ul><li><div><div>alpha</div></div></li></ul>", 40)
	if !strings.Contains(allText(d), "• alpha") {
		t.Errorf("doubly wrapped block must join bullet:\n%s", allText(d))
	}
	assertNoBareBullets(t, d)
}

func TestNoOrphanBulletNestedList(t *testing.T) {
	d := doc(t, "<ul><li><ul><li>sub</li></ul></li></ul>", 40)
	if !strings.Contains(allText(d), "sub") {
		t.Errorf("nested list content missing:\n%s", allText(d))
	}
	assertNoBareBullets(t, d)
}

func TestNestedBlocksSingleBlank(t *testing.T) {
	d := doc(t, "<div><div><div><p>a</p></div></div></div><p>b</p>", 40)
	if got, want := allText(d), "a\n\nb"; got != want {
		t.Errorf("text = %q, want %q", got, want)
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
