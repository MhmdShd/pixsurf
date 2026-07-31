package layout

import (
	"image"
	"image/color"
	"strings"
	"testing"

	"github.com/MhmdShd/pixsurf/cell"
	"github.com/MhmdShd/pixsurf/css"
	"github.com/MhmdShd/pixsurf/dom"
	"github.com/MhmdShd/pixsurf/style"
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
	return Render(d, width, nil, values, nil)
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
	out := Render(d, 20, fetch, nil, nil)
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

func TestTableProportionalColumns(t *testing.T) {
	long := "the quick brown fox jumps over the lazy dog near the river bank today"
	d := doc(t, "<table><tr><td>ID:</td><td>"+long+"</td></tr></table>", 60)
	l0 := lineText(d, 0)
	if !strings.HasPrefix(l0, "ID:") {
		t.Fatalf("line0 = %q, want label first", l0)
	}
	at := strings.Index(l0, "the quick")
	if at < 0 || at >= 8 {
		t.Errorf("value starts at col %d, want < 8 (no wasted label space): %q", at, l0)
	}
	for i := range d.Lines {
		if len(d.Lines[i]) > 60 {
			t.Errorf("line %d width %d > 60", i, len(d.Lines[i]))
		}
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

// solidImage returns a fetcher serving a solid-color w x h image and a
// pointer to the list of URLs it was asked for.
func solidImage(w, h int) (ImageFetcher, *[]string) {
	var calls []string
	fetch := func(url string) (image.Image, error) {
		calls = append(calls, url)
		img := image.NewRGBA(image.Rect(0, 0, w, h))
		for y := 0; y < h; y++ {
			for x := 0; x < w; x++ {
				img.Set(x, y, color.RGBA{R: 200, A: 255})
			}
		}
		return img, nil
	}
	return fetch, &calls
}

func renderWith(t *testing.T, src string, width int, fetch ImageFetcher) *Document {
	t.Helper()
	d, err := dom.Parse(src, "https://example.org/")
	if err != nil {
		t.Fatal(err)
	}
	return Render(d, width, fetch, nil, nil)
}

func pixelDims(d *Document) (cols, rows int) {
	for _, ln := range d.Lines {
		if len(ln) > 0 && ln[0].Rune == '▀' && ln[0].HasFg && ln[0].HasBg {
			rows++
			n := 0
			for _, c := range ln {
				if c.Rune == '▀' {
					n++
				}
			}
			if n > cols {
				cols = n
			}
		}
	}
	return cols, rows
}

func TestSpacerImageSkipped(t *testing.T) {
	fetch, calls := solidImage(14, 1)
	d := renderWith(t, `<p>a<img src="s.gif" width="14" height="1">b</p>`, 40, fetch)
	if _, rows := pixelDims(d); rows != 0 {
		t.Errorf("spacer image emitted %d pixel rows, want 0", rows)
	}
	txt := allText(d)
	if strings.Contains(txt, "[") {
		t.Errorf("spacer image emitted a placeholder:\n%s", txt)
	}
	if !strings.Contains(txt, "ab") {
		t.Errorf("text around spacer not contiguous, want %q in:\n%s", "ab", txt)
	}
	if len(*calls) != 0 {
		t.Errorf("fetcher was called for spacer image: %v", *calls)
	}
}

func TestTinyDecodedImageSkipped(t *testing.T) {
	fetch, _ := solidImage(2, 2)
	d := renderWith(t, `<img src="/tiny.png">`, 40, fetch)
	if cols, rows := pixelDims(d); cols != 0 || rows != 0 {
		t.Errorf("tiny decoded image emitted %dx%d pixel cells, want none", cols, rows)
	}
}

func TestImageSizedLikeBrowser(t *testing.T) {
	fetch, _ := solidImage(400, 200)
	d := renderWith(t, `<img src="/big.png">`, 100, fetch)
	cols, rows := pixelDims(d)
	if cols < 48 || cols > 52 {
		t.Errorf("400px-wide image = %d cols, want ~50", cols)
	}
	if rows < 10 || rows > 14 {
		t.Errorf("200px-tall image = %d rows, want ~12", rows)
	}
	if rows > 15 {
		t.Errorf("rows = %d exceeds 15-row cap", rows)
	}

	fetch, _ = solidImage(20, 20)
	d = renderWith(t, `<img src="/icon.png">`, 100, fetch)
	cols, rows = pixelDims(d)
	if cols < 2 || cols > 3 {
		t.Errorf("20px icon = %d cols, want 2-3", cols)
	}
	if rows < 1 || rows > 2 {
		t.Errorf("20px icon = %d rows, want 1-2", rows)
	}
}

func TestImageAttrsDriveSize(t *testing.T) {
	fetch, _ := solidImage(800, 800)
	d := renderWith(t, `<img src="/x.png" width="80" height="80">`, 100, fetch)
	cols, rows := pixelDims(d)
	if cols < 8 || cols > 12 {
		t.Errorf("80px-attr image = %d cols, want ~10 (attrs must win over 800px natural)", cols)
	}
	if rows < 4 || rows > 6 {
		t.Errorf("80px-attr image = %d rows, want ~5", rows)
	}
}

// imageAspectOK asserts the rendered cell grid's pixel aspect ratio
// (cols*cellPxW by rows*cellPxH) is within tol of the source aspect w/h.
func imageAspectOK(t *testing.T, cols, rows, w, h int, tol float64) {
	t.Helper()
	got := float64(cols*cellPxW) / float64(rows*cellPxH)
	want := float64(w) / float64(h)
	if got < want*(1-tol) || got > want*(1+tol) {
		t.Errorf("rendered aspect = %.3f (%dx%d cells), want ~%.3f (source %dx%d)",
			got, cols, rows, want, w, h)
	}
}

func TestImageAspectPreservedOnRowCap(t *testing.T) {
	fetch, _ := solidImage(400, 2000)
	d := renderWith(t, `<img src="/tall.png">`, 100, fetch)
	cols, rows := pixelDims(d)
	if rows != maxImageRows {
		t.Errorf("rows = %d, want the %d-row cap", rows, maxImageRows)
	}
	if cols < 7 || cols > 9 {
		t.Errorf("cols = %d, want ~8 (scaled with rows, far below width 100)", cols)
	}
	imageAspectOK(t, cols, rows, 400, 2000, 0.15)
}

func TestImageAspectPreservedOnWidthCap(t *testing.T) {
	fetch, _ := solidImage(2000, 400)
	d := renderWith(t, `<img src="/wide.png">`, 50, fetch)
	cols, rows := pixelDims(d)
	if cols != 50 {
		t.Errorf("cols = %d, want the content width 50", cols)
	}
	if rows < 4 || rows > 6 {
		t.Errorf("rows = %d, want ~5 (scaled with cols)", rows)
	}
	imageAspectOK(t, cols, rows, 2000, 400, 0.15)
}

func TestImageSmallUnchanged(t *testing.T) {
	fetch, _ := solidImage(400, 200)
	d := renderWith(t, `<img src="/photo.png">`, 100, fetch)
	cols, rows := pixelDims(d)
	if cols < 48 || cols > 52 {
		t.Errorf("cols = %d, want ~50 (no clamp binds)", cols)
	}
	if rows < 11 || rows > 14 {
		t.Errorf("rows = %d, want ~12-13 (no clamp binds)", rows)
	}
}

func TestBackgroundFillsLine(t *testing.T) {
	d := doc(t, `<div bgcolor="#ff6600">Hi</div>`, 20)
	if len(d.Lines) == 0 {
		t.Fatal("no lines")
	}
	line := d.Lines[0]
	if len(line) != 20 {
		t.Fatalf("line length = %d, want 20", len(line))
	}
	want := cell.RGB{R: 255, G: 102, B: 0}
	for i, c := range line {
		if !c.HasBg || c.Bg != want {
			t.Errorf("cell %d: HasBg=%v Bg=%v, want HasBg with %v", i, c.HasBg, c.Bg, want)
		}
	}
	if line[0].Rune != 'H' || line[1].Rune != 'i' {
		t.Errorf("glyphs = %q %q, want 'H' 'i'", line[0].Rune, line[1].Rune)
	}
}

func TestNoBackgroundNoPadding(t *testing.T) {
	d := doc(t, `<p>Hi</p>`, 20)
	if len(d.Lines) == 0 {
		t.Fatal("no lines")
	}
	line := d.Lines[0]
	if len(line) != 2 {
		t.Fatalf("line length = %d, want 2 (no trailing padding)", len(line))
	}
	for i, c := range line {
		if c.HasBg {
			t.Errorf("cell %d has HasBg, want none", i)
		}
	}
}

func TestNestedBackgroundInherits(t *testing.T) {
	d := doc(t, `<div bgcolor="#ff6600"><span>Hi</span></div>`, 20)
	if len(d.Lines) == 0 {
		t.Fatal("no lines")
	}
	line := d.Lines[0]
	if len(line) != 20 {
		t.Fatalf("line length = %d, want 20", len(line))
	}
	want := cell.RGB{R: 255, G: 102, B: 0}
	for i, c := range line {
		if !c.HasBg || c.Bg != want {
			t.Errorf("cell %d: HasBg=%v Bg=%v, want outer background %v", i, c.HasBg, c.Bg, want)
		}
	}
}

func TestTextareaRendersAsField(t *testing.T) {
	d := doc(t, `<form action="/s"><textarea name="q">hello</textarea></form>`, 60)
	if len(d.Fields) != 1 {
		t.Fatalf("fields = %d, want 1", len(d.Fields))
	}
	fl := d.Fields[0]
	if fl.Name != "q" || fl.Value != "hello" || fl.FormIdx != 0 {
		t.Errorf("field = %+v, want name q value hello", fl)
	}
	if idx, ok := d.FieldAt(fl.Line, fl.Start); !ok || idx != 0 {
		t.Errorf("FieldAt(start) = %d,%v", idx, ok)
	}
	if idx, ok := d.FieldAt(fl.Line, fl.End-1); !ok || idx != 0 {
		t.Errorf("FieldAt(end-1) = %d,%v", idx, ok)
	}
	if _, ok := d.FieldAt(fl.Line, fl.End); ok {
		t.Error("FieldAt(end) should miss (End exclusive)")
	}
	// the content shows once inside the box and never again as page text
	if got := strings.Count(allText(d), "hello"); got != 1 {
		t.Errorf("%q appears %d times, want exactly 1 (inside the box):\n%s", "hello", got, allText(d))
	}
	if !strings.Contains(allText(d), "[hello") {
		t.Errorf("textarea box missing:\n%s", allText(d))
	}
	if !d.Lines[fl.Line][fl.Start].Reverse {
		t.Error("box cell not reverse-video")
	}
}

func TestTextareaSubmits(t *testing.T) {
	values := FormValues{ValuesKey("https://example.org/s", "q"): "typed"}
	d := docV(t, `<form action="/s"><textarea name="q" cols="8">hello</textarea></form>`, 60, values)
	if len(d.Fields) != 1 {
		t.Fatalf("fields = %d, want 1", len(d.Fields))
	}
	if d.Fields[0].Value != "typed" {
		t.Errorf("Field.Value = %q, want values map to win", d.Fields[0].Value)
	}
	if !strings.Contains(allText(d), "[typed") {
		t.Errorf("typed value not shown in box:\n%s", allText(d))
	}
	if strings.Contains(allText(d), "hello") {
		t.Errorf("content value must lose to values map:\n%s", allText(d))
	}
}

func TestFormControlsInsideTableCells(t *testing.T) {
	d := doc(t, `<form action="/s"><table><tr>
		<td><input type="hidden" name="lang" value="en"><input type="text" name="q" value="cats" size="8"></td>
		<td><input type="submit" value="Go"></td>
	</tr></table></form>`, 60)
	if len(d.Forms) != 1 {
		t.Fatalf("forms = %d, want 1", len(d.Forms))
	}
	f := d.Forms[0]
	if got := f.Hidden.Get("lang"); got != "en" {
		t.Errorf("hidden lang = %q, want merged from cell", got)
	}
	if len(d.Fields) != 1 {
		t.Fatalf("fields = %d, want 1 (cell field must reach the page)", len(d.Fields))
	}
	fl := d.Fields[0]
	if fl.Name != "q" || fl.Value != "cats" || fl.FormIdx != 0 {
		t.Errorf("field = %+v", fl)
	}
	if idx, ok := d.FieldAt(fl.Line, fl.Start); !ok || idx != 0 {
		t.Errorf("FieldAt(start) = %d,%v", idx, ok)
	}
	if f.SubmitLine < 0 || f.SubmitEnd <= f.SubmitStart {
		t.Fatalf("submit region unset: %+v", f)
	}
	if idx, ok := d.SubmitAt(f.SubmitLine, f.SubmitStart); !ok || idx != 0 {
		t.Errorf("SubmitAt(start) = %d,%v", idx, ok)
	}
	if !strings.Contains(lineText(d, f.SubmitLine), "[ Go ]") {
		t.Errorf("submit line = %q, want [ Go ]", lineText(d, f.SubmitLine))
	}
}

func TestPageBackgroundCaptured(t *testing.T) {
	d := doc(t, `<body bgcolor="#ffffff"><p>hi</p></body>`, 20)
	if !d.HasPageBg {
		t.Fatal("HasPageBg = false, want true")
	}
	if want := (cell.RGB{R: 255, G: 255, B: 255}); d.PageBg != want {
		t.Errorf("PageBg = %v, want %v", d.PageBg, want)
	}
}

func TestBlankLinesPaintedWithBackground(t *testing.T) {
	d := doc(t, `<body bgcolor="#ffffff"><p>one</p><p>two</p></body>`, 20)
	if len(d.Lines) != 3 {
		t.Fatalf("lines = %d, want 3 (one, blank, two):\n%s", len(d.Lines), allText(d))
	}
	blank := d.Lines[1]
	if len(blank) != 20 {
		t.Fatalf("blank separator width = %d, want full content width 20", len(blank))
	}
	want := cell.RGB{R: 255, G: 255, B: 255}
	for i, c := range blank {
		if !c.HasBg || c.Bg != want {
			t.Errorf("blank cell %d: HasBg=%v Bg=%v, want HasBg with %v", i, c.HasBg, c.Bg, want)
		}
	}
}

func TestNoPageBackgroundUnchanged(t *testing.T) {
	d := doc(t, `<p>one</p><p>two</p>`, 20)
	if d.HasPageBg {
		t.Error("HasPageBg = true, want false")
	}
	if len(d.Lines) != 3 {
		t.Fatalf("lines = %d, want 3:\n%s", len(d.Lines), allText(d))
	}
	if len(d.Lines[1]) != 0 {
		t.Errorf("blank separator length = %d, want 0 (unpainted)", len(d.Lines[1]))
	}
	for i, ln := range d.Lines {
		for j, c := range ln {
			if c.HasBg {
				t.Errorf("line %d cell %d has HasBg, want none", i, j)
			}
		}
	}
}

func TestTableGuttersCarryBackground(t *testing.T) {
	d := doc(t, `<div bgcolor="#f6f6ef"><table><tr><td>aa</td><td>bb</td></tr></table></div>`, 20)
	want := cell.RGB{R: 246, G: 246, B: 239}
	var row []cell.Cell
	for i := range d.Lines {
		if strings.Contains(lineText(d, i), "aa") {
			row = d.Lines[i]
			break
		}
	}
	if row == nil {
		t.Fatal("table row not found")
	}
	// gutter is the cell between the two columns: first space after "aa"
	gut := -1
	for i, c := range row {
		if c.Rune == ' ' && i > 0 && row[i-1].Rune == 'a' {
			gut = i
			break
		}
	}
	if gut < 0 {
		t.Fatal("gutter cell not found")
	}
	if !row[gut].HasBg || row[gut].Bg != want {
		t.Errorf("gutter cell: HasBg=%v Bg=%v, want HasBg with %v", row[gut].HasBg, row[gut].Bg, want)
	}
}

// pixelLines returns the indexes of lines that contain image pixel cells.
func pixelLines(d *Document) []int {
	var idx []int
	for i, ln := range d.Lines {
		for _, c := range ln {
			if c.Rune == '▀' && c.HasFg && c.HasBg {
				idx = append(idx, i)
				break
			}
		}
	}
	return idx
}

func TestImagePixelsDoNotSmearPadding(t *testing.T) {
	fetch, _ := solidImage(64, 32) // solid red {200,0,0}
	d := renderWith(t, `<body bgcolor="#ffffff"><img src="/x.png"></body>`, 60, fetch)
	white := cell.RGB{R: 255, G: 255, B: 255}
	red := cell.RGB{R: 200}
	lines := pixelLines(d)
	if len(lines) == 0 {
		t.Fatal("no pixel lines rendered")
	}
	for _, i := range lines {
		ln := d.Lines[i]
		if len(ln) != 60 {
			t.Fatalf("line %d length = %d, want 60 (padded full width)", i, len(ln))
		}
		for j, c := range ln {
			if c.Rune == '▀' {
				if c.Bg != red || c.Fg != red {
					t.Errorf("line %d pixel %d = Fg %v Bg %v, want red %v", i, j, c.Fg, c.Bg, red)
				}
				continue
			}
			if !c.HasBg || c.Bg != white {
				t.Errorf("line %d padding cell %d: HasBg=%v Bg=%v, want white %v", i, j, c.HasBg, c.Bg, white)
			}
		}
	}
}

func TestImageLineNoBackgroundNoPadding(t *testing.T) {
	fetch, _ := solidImage(64, 32)
	d := renderWith(t, `<img src="/x.png">`, 60, fetch)
	lines := pixelLines(d)
	if len(lines) == 0 {
		t.Fatal("no pixel lines rendered")
	}
	for _, i := range lines {
		for j, c := range d.Lines[i] {
			if c.Rune != '▀' {
				t.Errorf("line %d cell %d = %q, want only pixel cells (no padding)", i, j, c.Rune)
			}
		}
	}
}

func TestBlankSeparatorUsesStyleBackground(t *testing.T) {
	fetch, _ := solidImage(64, 32)
	d := renderWith(t, `<body bgcolor="#ffffff"><p><img src="/x.png"></p><p>after</p></body>`, 60, fetch)
	lines := pixelLines(d)
	if len(lines) == 0 {
		t.Fatal("no pixel lines rendered")
	}
	blank := lines[len(lines)-1] + 1
	if blank >= len(d.Lines) {
		t.Fatal("no line after the image")
	}
	ln := d.Lines[blank]
	if len(ln) != 60 {
		t.Fatalf("blank separator length = %d, want 60 (painted)", len(ln))
	}
	white := cell.RGB{R: 255, G: 255, B: 255}
	for j, c := range ln {
		if !c.HasBg || c.Bg != white {
			t.Errorf("blank cell %d: HasBg=%v Bg=%v, want white %v", j, c.HasBg, c.Bg, white)
		}
	}
}

// stubResolver overlays test behaviour on the default tag styling.
type stubResolver struct {
	hidden  func(n *dom.Node) bool
	resolve func(n *dom.Node, parent style.Style) style.Style
}

func (s stubResolver) Hidden(n *dom.Node) bool {
	return s.hidden != nil && s.hidden(n)
}

func (s stubResolver) Resolve(n *dom.Node, parent style.Style) style.Style {
	if s.resolve != nil {
		return s.resolve(n, parent)
	}
	return style.ApplyInline(style.ForTag(strings.ToLower(n.Data), parent), n)
}

func docR(t *testing.T, src string, width int, r StyleResolver) *Document {
	t.Helper()
	d, err := dom.Parse(src, "https://example.org/page")
	if err != nil {
		t.Fatal(err)
	}
	return Render(d, width, nil, nil, r)
}

func TestResolverNilUnchanged(t *testing.T) {
	src := `<h1 id="top">Title</h1><p>go to <a href="/wiki/Go">the page</a></p><ul><li>one</li></ul>`
	d := docR(t, src, 40, nil)
	want := "Title\n\ngo to the page\n\n• one"
	if got := allText(d); got != want {
		t.Errorf("nil resolver text = %q, want %q", got, want)
	}
	if u, ok := d.LinkAt(2, 6); !ok || u != "https://example.org/wiki/Go" {
		t.Errorf("LinkAt(2,6) = %q,%v", u, ok)
	}
	if ln, ok := d.Anchors["top"]; !ok || ln != 0 {
		t.Errorf("anchor top = %d,%v", ln, ok)
	}
}

func TestResolverHidesSubtree(t *testing.T) {
	src := `<p>keep</p><div class="x"><p>secret text</p></div><p>tail</p>`
	r := stubResolver{hidden: func(n *dom.Node) bool {
		return n.Type == dom.ElementNode && n.Data == "div" && dom.Attr(n, "class") == "x"
	}}
	d := docR(t, src, 40, r)
	txt := allText(d)
	if strings.Contains(txt, "secret") {
		t.Errorf("hidden subtree rendered:\n%s", txt)
	}
	for _, want := range []string{"keep", "tail"} {
		if !strings.Contains(txt, want) {
			t.Errorf("missing %q in:\n%s", want, txt)
		}
	}
}

func TestResolverStyles(t *testing.T) {
	src := `<p><span class="hot">word</span></p>`
	r := stubResolver{resolve: func(n *dom.Node, parent style.Style) style.Style {
		s := style.ApplyInline(style.ForTag(strings.ToLower(n.Data), parent), n)
		if dom.Attr(n, "class") == "hot" {
			s.Bold = true
		}
		return s
	}}
	d := docR(t, src, 40, r)
	if got := lineText(d, 0); got != "word" {
		t.Fatalf("line0 = %q", got)
	}
	for i, c := range d.Lines[0][:4] {
		if !c.Bold {
			t.Errorf("cell %d not bold", i)
		}
	}
}

func TestTextTransformUppercase(t *testing.T) {
	src := `<p class="up">hello world</p>`
	r := stubResolver{resolve: func(n *dom.Node, parent style.Style) style.Style {
		s := style.ApplyInline(style.ForTag(strings.ToLower(n.Data), parent), n)
		if dom.Attr(n, "class") == "up" {
			s.Transform = style.TransformUpper
		}
		return s
	}}
	d := docR(t, src, 40, r)
	if got := lineText(d, 0); got != "HELLO WORLD" {
		t.Errorf("line0 = %q, want %q", got, "HELLO WORLD")
	}
}

func TestAlignCenterShiftsLineAndRanges(t *testing.T) {
	src := `<p class="c">go <a href="/x">link</a></p>`
	r := stubResolver{resolve: func(n *dom.Node, parent style.Style) style.Style {
		s := style.ApplyInline(style.ForTag(strings.ToLower(n.Data), parent), n)
		if dom.Attr(n, "class") == "c" {
			s.Align = style.AlignCenter
		}
		return s
	}}
	d := docR(t, src, 21, r)
	// "go link" is 7 cols; shift = (21-7)/2 = 7
	if got := lineText(d, 0); got != "       go link" {
		t.Errorf("line0 = %q, want %q", got, "       go link")
	}
	// old link columns 3..6 no longer hit
	if _, ok := d.LinkAt(0, 3); ok {
		t.Error("LinkAt(0,3) still matches after shift")
	}
	// new columns 10..13 hit
	for _, col := range []int{10, 13} {
		if u, ok := d.LinkAt(0, col); !ok || u != "https://example.org/x" {
			t.Errorf("LinkAt(0,%d) = %q,%v", col, u, ok)
		}
	}
	if _, ok := d.LinkAt(0, 14); ok {
		t.Error("LinkAt(0,14) matches past link end")
	}
}

// docC lays out src with a real CSS engine over the given sheets, as a
// full browser run with CSS enabled would.
func docC(t *testing.T, src string, width int, sheets ...string) *Document {
	t.Helper()
	d, err := dom.Parse(src, "https://example.org/page")
	if err != nil {
		t.Fatal(err)
	}
	return Render(d, width, nil, nil, css.New(sheets))
}

func TestBackdropPropagatesThroughTransparentChildren(t *testing.T) {
	src := `<body bgcolor="#ffffff"><div style="color:#1f1f1f">Gmail</div></body>`
	d := docC(t, src, 20)
	if len(d.Lines) == 0 {
		t.Fatal("no lines")
	}
	white := cell.RGB{R: 255, G: 255, B: 255}
	dark := cell.RGB{R: 0x1f, G: 0x1f, B: 0x1f}
	line := d.Lines[0]
	if len(line) != 20 {
		t.Fatalf("line length = %d, want padded to 20", len(line))
	}
	for i, c := range line {
		if !c.HasBg || c.Bg != white {
			t.Errorf("cell %d: HasBg=%v Bg=%v, want ancestor white backdrop", i, c.HasBg, c.Bg)
		}
	}
	if c := line[0]; !c.HasFg || c.Fg != dark {
		t.Errorf("glyph fg = %v (has %v), want %v", c.Fg, c.HasFg, dark)
	}
	if !d.HasPageBg || d.PageBg != white {
		t.Errorf("PageBg = %v,%v, want white", d.PageBg, d.HasPageBg)
	}
}

func TestBackdropNotPaintedWhenNoBackgroundAnywhere(t *testing.T) {
	d := docC(t, `<p>Hi <b>there</b></p>`, 20, "b { color: #ff0000 }")
	if len(d.Lines) == 0 {
		t.Fatal("no lines")
	}
	line := d.Lines[0]
	if len(line) != 8 { // "Hi there", no trailing padding
		t.Fatalf("line length = %d, want 8 (no padding)", len(line))
	}
	for i, c := range line {
		if c.HasBg {
			t.Errorf("cell %d has HasBg, want terminal default", i)
		}
	}
	if d.HasPageBg {
		t.Error("HasPageBg = true, want false")
	}
}

func TestDeclaredBackgroundOverridesAncestor(t *testing.T) {
	src := `<body bgcolor="#ffffff"><div><span style="background:#000080">own</span></div><div>after</div></body>`
	d := docC(t, src, 20)
	white := cell.RGB{R: 255, G: 255, B: 255}
	navy := cell.RGB{R: 0, G: 0, B: 0x80}
	var ownLine, afterLine = -1, -1
	for i := range d.Lines {
		switch lineText(d, i) {
		case "own":
			ownLine = i
		case "after":
			afterLine = i
		}
	}
	if ownLine < 0 || afterLine < 0 {
		t.Fatalf("lines not found in:\n%s", allText(d))
	}
	for i := 0; i < 3; i++ { // the span's own glyphs use its own background
		if c := d.Lines[ownLine][i]; !c.HasBg || c.Bg != navy {
			t.Errorf("own cell %d: HasBg=%v Bg=%v, want navy", i, c.HasBg, c.Bg)
		}
	}
	for i := 0; i < 5; i++ { // the sibling reverts to the ancestor sheet
		if c := d.Lines[afterLine][i]; !c.HasBg || c.Bg != white {
			t.Errorf("after cell %d: HasBg=%v Bg=%v, want white", i, c.HasBg, c.Bg)
		}
	}
}

func TestFieldUsesCSSColoursWhenAvailable(t *testing.T) {
	src := `<body bgcolor="#ffffff"><form><input type="text" name="q" size="5" value="ab">` +
		`<input type="submit" value="Go"></form></body>`
	d := docC(t, src, 60, "body { color: #202124 }")
	if len(d.Fields) != 1 {
		t.Fatalf("fields = %d, want 1", len(d.Fields))
	}
	white := cell.RGB{R: 255, G: 255, B: 255}
	dark := cell.RGB{R: 0x20, G: 0x21, B: 0x24}
	fl := d.Fields[0]
	for col := fl.Start; col < fl.End; col++ {
		c := d.Lines[fl.Line][col]
		if c.Reverse {
			t.Errorf("field col %d reverse-video, want CSS colours", col)
		}
		if !c.HasFg || c.Fg != dark || !c.HasBg || c.Bg != white {
			t.Errorf("field col %d: fg=%v,%v bg=%v,%v, want dark on white", col, c.HasFg, c.Fg, c.HasBg, c.Bg)
		}
	}
	if d.Lines[fl.Line][fl.Start].Rune != '[' || d.Lines[fl.Line][fl.End-1].Rune != ']' {
		t.Error("bracket affordance missing on CSS-coloured field")
	}
	f := d.Forms[0]
	if f.SubmitLine < 0 {
		t.Fatal("no submit region")
	}
	sc := d.Lines[f.SubmitLine][f.SubmitStart]
	if sc.Reverse {
		t.Error("submit reverse-video, want CSS colours")
	}
	if !sc.Bold || !sc.HasFg || sc.Fg != dark || !sc.HasBg || sc.Bg != white {
		t.Errorf("submit cell = %+v, want bold dark on white", sc)
	}
}

func TestFieldFallsBackToReverse(t *testing.T) {
	// No colours anywhere: the reverse-video box is the only thing
	// keeping the field visible, exactly as before CSS support.
	d := docC(t, `<form><input type="text" name="q" size="5" value="ab"></form>`, 60)
	if len(d.Fields) != 1 {
		t.Fatalf("fields = %d, want 1", len(d.Fields))
	}
	fl := d.Fields[0]
	for col := fl.Start; col < fl.End; col++ {
		c := d.Lines[fl.Line][col]
		if !c.Reverse {
			t.Errorf("field col %d not reverse-video with no usable colours", col)
		}
		if c.HasBg {
			t.Errorf("field col %d has a background from nowhere", col)
		}
	}

	// A page background but no foreground: still reverse, never invisible.
	d2 := docC(t, `<body bgcolor="#ffffff"><form><input type="text" name="q" size="5"></form></body>`, 60)
	fl2 := d2.Fields[0]
	if c := d2.Lines[fl2.Line][fl2.Start]; !c.Reverse {
		t.Error("field with backdrop but no foreground must keep reverse fallback")
	}
}

func TestPageBackgroundFromWrapperElement(t *testing.T) {
	// html and body declare nothing; a wrapper div holding all the
	// content paints the sheet (the Wikipedia .mw-page-container pattern).
	src := `<body><div class="page-container"><p>alpha beta gamma delta content here</p>` +
		`<p>more content lines to fill the page</p></div></body>`
	d := docC(t, src, 40, ".page-container { background-color: #f8f9fa }")
	if !d.HasPageBg {
		t.Fatal("HasPageBg = false, want true from wrapper background")
	}
	if want := (cell.RGB{R: 0xf8, G: 0xf9, B: 0xfa}); d.PageBg != want {
		t.Errorf("PageBg = %v, want %v", d.PageBg, want)
	}
}

func TestTableCellCaptionKeepsBackdrop(t *testing.T) {
	// An image row followed by a text row inside a painted table: the
	// text row's cells must carry the table's backdrop, not terminal
	// default. The row's own inline background must also reach its cells
	// even though the <tr> is not walked directly.
	src := `<table class="box"><tr><td><img src="/x.png" alt="pic"></td></tr>` +
		`<tr style="background-color: rgb(235,235,210)"><td>Various types of cats</td></tr></table>`
	d := docC(t, src, 40, ".box { background-color: #f8f9fa }")
	rowBg := cell.RGB{R: 235, G: 235, B: 210}
	found := false
	for i := range d.Lines {
		if !strings.Contains(lineText(d, i), "Various") {
			continue
		}
		found = true
		for j, c := range d.Lines[i] {
			if c.Rune == 0 || c.Rune == ' ' {
				continue
			}
			if !c.HasBg {
				t.Fatalf("line %d cell %d (%q) has no backdrop, want table/row background", i, j, c.Rune)
			}
			if c.Bg != rowBg {
				t.Errorf("line %d cell %d bg = %v, want row background %v", i, j, c.Bg, rowBg)
			}
		}
	}
	if !found {
		t.Fatalf("caption text not rendered:\n%s", allText(d))
	}
}

func TestContrastSafetyNetDarkOnDark(t *testing.T) {
	// Dark grey text on a near-black backdrop: the safety net must
	// lighten the foreground to a readable value.
	src := `<body><p>invisible text</p></body>`
	d := docC(t, src, 40, "body { background-color: #101418; color: #202122 }")
	if len(d.Lines) == 0 {
		t.Fatal("no lines")
	}
	if d.ContrastFixes == 0 {
		t.Error("ContrastFixes = 0, want the safety net to have fired")
	}
	for j, c := range d.Lines[0] {
		if c.Rune == 0 || c.Rune == ' ' {
			continue
		}
		if !c.HasFg || !c.HasBg {
			t.Fatalf("cell %d missing fg/bg: %+v", j, c)
		}
		diff := relLuminance(c.Fg) - relLuminance(c.Bg)
		if diff < 0 {
			diff = -diff
		}
		if diff < minLumDiff {
			t.Errorf("cell %d (%q): fg %v on bg %v, luminance diff %.3f < %.2f",
				j, c.Rune, c.Fg, c.Bg, diff, minLumDiff)
		}
	}
}

func TestContrastSafetyNetLeavesGoodPairsAlone(t *testing.T) {
	src := `<body><p>readable text</p></body>`
	d := docC(t, src, 40, "body { background-color: #ffffff; color: #202122 }")
	if len(d.Lines) == 0 {
		t.Fatal("no lines")
	}
	if d.ContrastFixes != 0 {
		t.Errorf("ContrastFixes = %d, want 0 for a good pair", d.ContrastFixes)
	}
	dark := cell.RGB{R: 0x20, G: 0x21, B: 0x22}
	white := cell.RGB{R: 255, G: 255, B: 255}
	for j, c := range d.Lines[0] {
		if c.Rune == 0 || c.Rune == ' ' {
			continue
		}
		if c.Fg != dark || c.Bg != white {
			t.Errorf("cell %d = fg %v bg %v, want unchanged dark on white", j, c.Fg, c.Bg)
		}
	}
}

func contentLines(d *Document) []string {
	var out []string
	for i := range d.Lines {
		if s := lineText(d, i); s != "" {
			out = append(out, s)
		}
	}
	return out
}

func TestDisplayInlineKeepsOnOneLine(t *testing.T) {
	d := docC(t, `<div style="display:inline">a</div><div style="display:inline">b</div>`, 40)
	got := contentLines(d)
	if len(got) != 1 || got[0] != "a b" {
		t.Errorf("content lines = %q, want [\"a b\"]", got)
	}
}

func TestDisplayBlockBreaksInlineTag(t *testing.T) {
	d := docC(t, `<p><span style="display:block">a</span><span style="display:block">b</span></p>`, 40)
	got := contentLines(d)
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Errorf("content lines = %q, want [\"a\" \"b\"]", got)
	}
}

func TestFlexChildrenOnOneRow(t *testing.T) {
	d := docC(t, `<div class="row"><div>alpha</div><div>beta</div><div>gamma</div></div>`, 40,
		".row { display: flex }")
	got := contentLines(d)
	if len(got) != 1 || got[0] != "alpha beta gamma" {
		t.Errorf("content lines = %q, want [\"alpha beta gamma\"]", got)
	}
}

func TestFlexRowWraps(t *testing.T) {
	d := docC(t, `<div class="row"><div>aaa</div><div>bbb</div><div>ccc</div></div>`, 8,
		".row { display: flex }")
	got := contentLines(d)
	want := []string{"aaa bbb", "ccc"}
	if len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("content lines = %q, want %q", got, want)
	}
}

func TestJustifyContentCentresRow(t *testing.T) {
	d := docC(t, `<div class="row"><a href="/a">aa</a><a href="/b">bb</a></div>`, 40,
		".row { display: flex; justify-content: center }")
	// row is "aa bb" (5 cols); shift = (40-5)/2 = 17
	if got := lineText(d, 0); got != strings.Repeat(" ", 17)+"aa bb" {
		t.Errorf("line0 = %q, want centred \"aa bb\"", got)
	}
	if _, ok := d.LinkAt(0, 0); ok {
		t.Error("LinkAt(0,0) matches before the centred row")
	}
	if u, ok := d.LinkAt(0, 17); !ok || u != "https://example.org/a" {
		t.Errorf("LinkAt(0,17) = %q,%v, want first link at shifted column", u, ok)
	}
	if u, ok := d.LinkAt(0, 20); !ok || u != "https://example.org/b" {
		t.Errorf("LinkAt(0,20) = %q,%v, want second link at shifted column", u, ok)
	}
}

func TestJustifyContentFlexEndRightAligns(t *testing.T) {
	d := docC(t, `<div class="row"><a href="/a">aa</a><a href="/b">bb</a></div>`, 40,
		".row { display: flex; justify-content: flex-end }")
	// row is "aa bb" (5 cols); shift = 40-5 = 35
	if got := lineText(d, 0); got != strings.Repeat(" ", 35)+"aa bb" {
		t.Errorf("line0 = %q, want right-aligned \"aa bb\"", got)
	}
	if u, ok := d.LinkAt(0, 35); !ok || u != "https://example.org/a" {
		t.Errorf("LinkAt(0,35) = %q,%v, want first link at shifted column", u, ok)
	}
	if u, ok := d.LinkAt(0, 38); !ok || u != "https://example.org/b" {
		t.Errorf("LinkAt(0,38) = %q,%v, want second link at shifted column", u, ok)
	}
}

func TestJustifyContentDefaultUnchanged(t *testing.T) {
	src := `<div class="row"><a href="/a">aa</a><a href="/b">bb</a></div>`
	plain := docC(t, src, 40, ".row { display: flex }")
	if got := lineText(plain, 0); got != "aa bb" {
		t.Errorf("line0 = %q, want left-aligned \"aa bb\"", got)
	}
	start := docC(t, src, 40, ".row { display: flex; justify-content: flex-start }")
	if got := lineText(start, 0); got != "aa bb" {
		t.Errorf("flex-start line0 = %q, want left-aligned \"aa bb\"", got)
	}
}

func TestJustifyContentDoesNotLeak(t *testing.T) {
	d := docC(t, `<div class="row"><span>aa</span></div><p>after text</p>`, 40,
		".row { display: flex; justify-content: center }")
	got := contentLines(d)
	if len(got) != 2 {
		t.Fatalf("content lines = %q, want 2", got)
	}
	if got[0] != strings.Repeat(" ", 19)+"aa" {
		t.Errorf("row line = %q, want centred \"aa\"", got[0])
	}
	if got[1] != "after text" {
		t.Errorf("following line = %q, want left-aligned \"after text\"", got[1])
	}
}

func TestCenterTagCentres(t *testing.T) {
	d := doc(t, `<center><a href="/x">hi</a></center><p>after</p>`, 40)
	// "hi" is 2 cols; shift = (40-2)/2 = 19
	if got := lineText(d, 0); got != strings.Repeat(" ", 19)+"hi" {
		t.Errorf("line0 = %q, want centred \"hi\"", got)
	}
	if u, ok := d.LinkAt(0, 19); !ok || u != "https://example.org/x" {
		t.Errorf("LinkAt(0,19) = %q,%v, want link at shifted columns", u, ok)
	}
	got := contentLines(d)
	if len(got) != 2 || got[1] != "after" {
		t.Errorf("content lines = %q, want centring not to leak past </center>", got)
	}
}

func TestCenterTagCentresTable(t *testing.T) {
	d := doc(t, `<center><table><tr><td><a href="/x">abcd</a></td></tr></table></center>`, 40)
	// table box is 4 cols; shift = (40-4)/2 = 18
	if got := lineText(d, 0); got != strings.Repeat(" ", 18)+"abcd" {
		t.Errorf("line0 = %q, want centred table row", got)
	}
	if u, ok := d.LinkAt(0, 18); !ok || u != "https://example.org/x" {
		t.Errorf("LinkAt(0,18) = %q,%v, want link at shifted columns", u, ok)
	}
}

func TestAutoMarginCentres(t *testing.T) {
	d := docC(t, `<div style="width:20px;margin:0 auto"><a href="/x">hi</a></div>`, 40)
	// "hi" is 2 cols; shift = (40-2)/2 = 19
	if got := lineText(d, 0); got != strings.Repeat(" ", 19)+"hi" {
		t.Errorf("line0 = %q, want centred \"hi\"", got)
	}
	if _, ok := d.LinkAt(0, 0); ok {
		t.Error("LinkAt(0,0) matches before the centred text")
	}
	for _, col := range []int{19, 20} {
		if u, ok := d.LinkAt(0, col); !ok || u != "https://example.org/x" {
			t.Errorf("LinkAt(0,%d) = %q,%v, want link at shifted columns", col, u, ok)
		}
	}
	// 3-value shorthand centres too; margin-left:auto alone right-aligns.
	d2 := docC(t, `<div style="margin:8px auto 0">hi</div>`, 40)
	if got := lineText(d2, 0); got != strings.Repeat(" ", 19)+"hi" {
		t.Errorf("3-value shorthand line0 = %q, want centred", got)
	}
	d3 := docC(t, `<div style="margin-left:auto">hi</div>`, 40)
	if got := lineText(d3, 0); got != strings.Repeat(" ", 38)+"hi" {
		t.Errorf("margin-left:auto line0 = %q, want right-aligned", got)
	}
}

func TestInlineBackgroundDoesNotFillLine(t *testing.T) {
	d := docC(t, `<p>go <span style="background:#000080">hot</span> end</p>`, 20)
	if got := lineText(d, 0); got != "go hot end" {
		t.Fatalf("line0 = %q", got)
	}
	line := d.Lines[0]
	if len(line) != 10 {
		t.Fatalf("line length = %d, want 10 (no full-width padding)", len(line))
	}
	navy := cell.RGB{R: 0, G: 0, B: 0x80}
	for i, c := range line {
		if i >= 3 && i <= 5 {
			if !c.HasBg || c.Bg != navy {
				t.Errorf("cell %d: HasBg=%v Bg=%v, want navy behind the span's glyphs", i, c.HasBg, c.Bg)
			}
			continue
		}
		if c.HasBg {
			t.Errorf("cell %d has HasBg outside the inline element", i)
		}
	}
}

func TestBlockBackgroundStillFillsLine(t *testing.T) {
	d := docC(t, `<div style="background:#ff6600">Hi</div>`, 20)
	line := d.Lines[0]
	if len(line) != 20 {
		t.Fatalf("line length = %d, want 20 (full-width fill)", len(line))
	}
	want := cell.RGB{R: 255, G: 102, B: 0}
	for i, c := range line {
		if !c.HasBg || c.Bg != want {
			t.Errorf("cell %d: HasBg=%v Bg=%v, want block background", i, c.HasBg, c.Bg)
		}
	}
}

// pixelStart returns the line index and starting column of the first
// image-pixel line, or (-1, -1) when none exists.
func pixelStart(d *Document) (line, col int) {
	for i, ln := range d.Lines {
		for j, c := range ln {
			if c.Rune == '▀' && c.HasFg && c.HasBg {
				return i, j
			}
		}
	}
	return -1, -1
}

func TestCentredImageShifts(t *testing.T) {
	// 272x32 px: 34 cols x 2 rows at 8x16 px per cell.
	fetch, _ := solidImage(272, 32)
	d := renderWith(t, `<center><img src="/logo.png"></center>`, 100, fetch)
	line, col := pixelStart(d)
	if line < 0 {
		t.Fatal("no pixel lines rendered")
	}
	want := (100 - 34) / 2
	if col < want-1 || col > want+1 {
		t.Errorf("centred image starts at column %d, want ~%d", col, want)
	}
}

func TestCentredImageLinkShifts(t *testing.T) {
	fetch, _ := solidImage(272, 32)
	d := renderWith(t, `<center><a href="/home"><img src="/logo.png"></a></center>`, 100, fetch)
	line, col := pixelStart(d)
	if line < 0 {
		t.Fatal("no pixel lines rendered")
	}
	url, ok := d.LinkAt(line, col)
	if !ok || !strings.HasSuffix(url, "/home") {
		t.Errorf("LinkAt(%d, %d) = %q, %v; want the image link", line, col, url, ok)
	}
	if url, ok := d.LinkAt(line, 0); ok {
		t.Errorf("LinkAt(%d, 0) = %q; want no link at the left edge", line, url)
	}
}

func TestUncentredImageUnchanged(t *testing.T) {
	fetch, _ := solidImage(272, 32)
	d := renderWith(t, `<div><img src="/logo.png"></div>`, 100, fetch)
	line, col := pixelStart(d)
	if line < 0 {
		t.Fatal("no pixel lines rendered")
	}
	if col != 0 {
		t.Errorf("unaligned image starts at column %d, want 0", col)
	}
}
