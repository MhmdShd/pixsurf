package markup

import (
	"strings"
	"testing"
)

func TestMarkdownByContentType(t *testing.T) {
	out := ToHTML("# Title\n\nsome *text*", "text/markdown", "https://example.com/doc")
	if !strings.Contains(out, "<h1") {
		t.Errorf("no <h1 in output: %q", out)
	}
	if !strings.Contains(out, "<em") {
		t.Errorf("no <em in output: %q", out)
	}
}

func TestMarkdownByExtension(t *testing.T) {
	out := ToHTML("# Title\n\nsome *text*", "", "https://example.com/repo/README.md")
	if !strings.Contains(out, "<h1") {
		t.Errorf("no <h1 in output: %q", out)
	}
	if !strings.Contains(out, "<em") {
		t.Errorf("no <em in output: %q", out)
	}
}

func TestGFMTable(t *testing.T) {
	body := "| a | b |\n|---|---|\n| 1 | 2 |\n"
	out := ToHTML(body, "text/markdown", "")
	if !strings.Contains(out, "<table") {
		t.Errorf("no <table in output: %q", out)
	}
}

func TestPlainTextWrapped(t *testing.T) {
	out := ToHTML("a < b\n", "text/plain", "https://example.com/notes.txt")
	if !strings.Contains(out, "<pre") {
		t.Errorf("no <pre in output: %q", out)
	}
	if !strings.Contains(out, "&lt;") {
		t.Errorf("< not escaped: %q", out)
	}
}

func TestHTMLPassthrough(t *testing.T) {
	body := "<html><body><p>hi & <em>there</em></p></body></html>"
	if out := ToHTML(body, "text/html", "https://example.com/"); out != body {
		t.Errorf("html not passed through byte-identical:\n got %q\nwant %q", out, body)
	}
}
