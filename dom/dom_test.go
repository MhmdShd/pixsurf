package dom

import (
	"strings"
	"testing"
)

func TestParseAndResolve(t *testing.T) {
	d, err := Parse("<p><a href=/wiki/Go>go</a>", "https://en.example.org/wiki/Home")
	if err != nil {
		t.Fatal(err)
	}
	if got := d.Resolve("/wiki/Go"); got != "https://en.example.org/wiki/Go" {
		t.Errorf("Resolve = %q", got)
	}
	if got := d.Resolve("Sub"); got != "https://en.example.org/wiki/Sub" {
		t.Errorf("relative Resolve = %q", got)
	}
	if got := d.Resolve("https://other.org/x"); got != "https://other.org/x" {
		t.Errorf("absolute Resolve = %q", got)
	}
}

func TestBaseHref(t *testing.T) {
	d, err := Parse(`<head><base href="https://cdn.example.org/app/"></head><body><a href=img>x</a></body>`, "https://example.org/page")
	if err != nil {
		t.Fatal(err)
	}
	if got := d.Resolve("img"); got != "https://cdn.example.org/app/img" {
		t.Errorf("Resolve with base = %q", got)
	}
}

func TestAttr(t *testing.T) {
	d, err := Parse(`<img SRC="a.png" alt="pic">`, "https://example.org/")
	if err != nil {
		t.Fatal(err)
	}
	var found bool
	Walk(d.Root, func(n *Node) {
		if n.Type == ElementNode && n.Data == "img" {
			found = true
			if Attr(n, "src") != "a.png" {
				t.Errorf("Attr src = %q", Attr(n, "src"))
			}
		}
	})
	if !found {
		t.Error("img node not found")
	}
	_ = strings.TrimSpace // keep strings import
}
