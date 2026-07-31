package browser

import "testing"

func TestNormalizeURL(t *testing.T) {
	cases := map[string]string{
		"github.com":         "https://github.com",
		"  github.com  ":     "https://github.com",
		"https://github.com": "https://github.com",
		"http://example.com": "http://example.com",
		"file:///tmp/x.html": "file:///tmp/x.html",
	}
	for in, want := range cases {
		if got := NormalizeURL(in); got != want {
			t.Errorf("NormalizeURL(%q) = %q, want %q", in, got, want)
		}
	}
}
