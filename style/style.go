// Package style maps HTML tags and inline attributes to display styles.
package style

import (
	"strings"

	"golang.org/x/net/html"

	"github.com/MhmdShd/pixsurf/cell"
)

// Style is the resolved display style for a DOM subtree.
type Style struct {
	Fg, Bg                                        cell.RGB
	HasFg, HasBg                                  bool
	Bold, Italic, Underline, Strike, Reverse, Dim bool
}

var linkColor = cell.RGB{R: 95, G: 175, B: 255}

// ForTag returns parent adjusted by tag defaults (inheritance preserved).
func ForTag(tag string, parent Style) Style {
	s := parent
	switch tag {
	case "h1", "h2":
		s.Bold, s.Underline = true, true
	case "h3", "h4", "h5", "h6":
		s.Bold = true
	case "a":
		s.Underline, s.HasFg, s.Fg = true, true, linkColor
	case "b", "strong", "th":
		s.Bold = true
	case "i", "em":
		s.Italic = true
	case "u":
		s.Underline = true
	case "s", "del", "strike":
		s.Strike = true
	case "code", "kbd":
		s.Reverse = true
	case "pre":
		s.Reverse = false
	case "blockquote":
		s.Dim = true
	}
	return s
}

var hidden = map[string]bool{
	"script": true, "style": true, "head": true,
	"noscript": true, "template": true, "title": true,
}

// Hidden reports whether a tag's subtree renders nothing.
func Hidden(tag string) bool { return hidden[tag] }

var named = map[string]cell.RGB{
	"black": {R: 0, G: 0, B: 0}, "silver": {R: 192, G: 192, B: 192}, "gray": {R: 128, G: 128, B: 128},
	"white": {R: 255, G: 255, B: 255}, "maroon": {R: 128, G: 0, B: 0}, "red": {R: 255, G: 0, B: 0},
	"purple": {R: 128, G: 0, B: 128}, "fuchsia": {R: 255, G: 0, B: 255}, "green": {R: 0, G: 128, B: 0},
	"lime": {R: 0, G: 255, B: 0}, "olive": {R: 128, G: 128, B: 0}, "yellow": {R: 255, G: 255, B: 0},
	"navy": {R: 0, G: 0, B: 128}, "blue": {R: 0, G: 0, B: 255}, "teal": {R: 0, G: 128, B: 128},
	"aqua": {R: 0, G: 255, B: 255},
}

// ParseColor parses #rgb, #rrggbb, or a basic named color.
func ParseColor(v string) (cell.RGB, bool) {
	v = strings.ToLower(strings.TrimSpace(v))
	if c, ok := named[v]; ok {
		return c, true
	}
	if strings.HasPrefix(v, "#") {
		hex := v[1:]
		if len(hex) == 3 {
			hex = string([]byte{hex[0], hex[0], hex[1], hex[1], hex[2], hex[2]})
		}
		if len(hex) == 6 {
			var out [3]uint8
			for i := 0; i < 3; i++ {
				n, ok := hexByte(hex[i*2], hex[i*2+1])
				if !ok {
					return cell.RGB{}, false
				}
				out[i] = n
			}
			return cell.RGB{R: out[0], G: out[1], B: out[2]}, true
		}
	}
	return cell.RGB{}, false
}

func hexByte(a, b byte) (uint8, bool) {
	hi, ok1 := hexNibble(a)
	lo, ok2 := hexNibble(b)
	return hi<<4 | lo, ok1 && ok2
}

func hexNibble(c byte) (uint8, bool) {
	switch {
	case c >= '0' && c <= '9':
		return c - '0', true
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10, true
	}
	return 0, false
}

// ApplyInline overlays style="" declarations and legacy color/bgcolor attrs.
func ApplyInline(s Style, n *html.Node) Style {
	attr := func(key string) string {
		for _, a := range n.Attr {
			if strings.EqualFold(a.Key, key) {
				return a.Val
			}
		}
		return ""
	}
	if c, ok := ParseColor(attr("color")); ok {
		s.HasFg, s.Fg = true, c
	}
	if c, ok := ParseColor(attr("bgcolor")); ok {
		s.HasBg, s.Bg = true, c
	}
	for _, decl := range strings.Split(attr("style"), ";") {
		k, v, ok := strings.Cut(decl, ":")
		if !ok {
			continue
		}
		switch strings.TrimSpace(strings.ToLower(k)) {
		case "color":
			if c, ok := ParseColor(v); ok {
				s.HasFg, s.Fg = true, c
			}
		case "background-color", "background":
			if c, ok := ParseColor(v); ok {
				s.HasBg, s.Bg = true, c
			}
		}
	}
	return s
}
