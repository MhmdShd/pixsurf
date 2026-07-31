package style

import (
	"strings"
	"testing"

	"golang.org/x/net/html"

	"github.com/MhmdShd/pixsurf/cell"
)

func node(t *testing.T, src, tag string) *html.Node {
	t.Helper()
	root, err := html.Parse(strings.NewReader(src))
	if err != nil {
		t.Fatal(err)
	}
	var found *html.Node
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == tag {
			found = n
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(root)
	if found == nil {
		t.Fatalf("tag %q not found", tag)
	}
	return found
}

func TestTagDefaults(t *testing.T) {
	base := Style{}
	if s := ForTag("h1", base); !s.Bold || !s.Underline {
		t.Errorf("h1 = %+v, want bold+underline", s)
	}
	if s := ForTag("a", base); !s.Underline || !s.HasFg {
		t.Errorf("a = %+v, want underline + fg color", s)
	}
	if s := ForTag("code", base); !s.Reverse {
		t.Errorf("code = %+v, want reverse", s)
	}
	if s := ForTag("blockquote", base); !s.Dim {
		t.Errorf("blockquote = %+v, want dim", s)
	}
	if s := ForTag("em", base); !s.Italic {
		t.Errorf("em = %+v, want italic", s)
	}
	// inheritance: bold parent stays bold in unknown child
	bold := Style{Bold: true}
	if s := ForTag("span", bold); !s.Bold {
		t.Errorf("span under bold = %+v, want bold inherited", s)
	}
}

func TestParseColor(t *testing.T) {
	cases := map[string]cell.RGB{
		"#fff":    {255, 255, 255},
		"#002b36": {0, 43, 54},
		"red":     {255, 0, 0},
		"navy":    {0, 0, 128},
	}
	for in, want := range cases {
		got, ok := ParseColor(in)
		if !ok || got != want {
			t.Errorf("ParseColor(%q) = %v,%v want %v", in, got, ok, want)
		}
	}
	if _, ok := ParseColor("notacolor"); ok {
		t.Error("ParseColor accepted garbage")
	}
}

func TestApplyInline(t *testing.T) {
	n := node(t, `<p style="color: red; background-color: #002b36">x</p>`, "p")
	s := ApplyInline(Style{}, n)
	if !s.HasFg || s.Fg != (cell.RGB{255, 0, 0}) {
		t.Errorf("fg = %+v", s)
	}
	if !s.HasBg || s.Bg != (cell.RGB{0, 43, 54}) {
		t.Errorf("bg = %+v", s)
	}
	legacy := node(t, `<font color="navy">x</font>`, "font")
	s2 := ApplyInline(Style{}, legacy)
	if !s2.HasFg || s2.Fg != (cell.RGB{0, 0, 128}) {
		t.Errorf("legacy color = %+v", s2)
	}
}

func TestHidden(t *testing.T) {
	for _, tag := range []string{"script", "style", "head", "noscript", "template"} {
		if !Hidden(tag) {
			t.Errorf("Hidden(%q) = false", tag)
		}
	}
	if Hidden("p") {
		t.Error("Hidden(p) = true")
	}
}
