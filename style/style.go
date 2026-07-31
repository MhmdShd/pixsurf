// Package style maps HTML tags and inline attributes to display styles.
package style

import (
	"strings"

	"golang.org/x/net/html"

	"github.com/MhmdShd/pixsurf/cell"
)

// Align is a horizontal text alignment (CSS text-align).
type Align uint8

const (
	AlignNone Align = iota
	AlignLeft
	AlignCenter
	AlignRight
)

// Display is the CSS display class relevant to line layout: whether an
// element breaks the line for itself, and whether it lays its immediate
// children out inline (flex/grid containers).
type Display uint8

const (
	DisplayUnset       Display = iota // no declaration: tag default applies
	DisplayInline                     // inline: never block-breaks
	DisplayInlineBlock                // inline-block: an inline box; descendant blocks stay inside it
	DisplayBlock                      // block, list-item, table, flow-root: always breaks
	DisplayFlex                       // flex, grid: block container, children flow inline
	DisplayInlineFlex                 // inline-flex, inline-grid: inline container, children inline
)

// ParseDisplay maps a CSS display value to its layout class. Values
// with no line-layout meaning here (none, contents, table-cell, ...)
// return false so the tag default stays in effect.
func ParseDisplay(v string) (Display, bool) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "inline":
		return DisplayInline, true
	case "inline-block":
		return DisplayInlineBlock, true
	case "block", "list-item", "table", "flow-root":
		return DisplayBlock, true
	case "flex", "grid":
		return DisplayFlex, true
	case "inline-flex", "inline-grid":
		return DisplayInlineFlex, true
	}
	return DisplayUnset, false
}

// Transform is a text case transform (CSS text-transform).
type Transform uint8

const (
	TransformNone Transform = iota
	TransformUpper
	TransformLower
	TransformCapitalize
)

// Style is the resolved display style for a DOM subtree.
type Style struct {
	Fg, Bg                                        cell.RGB
	HasFg, HasBg                                  bool
	Bold, Italic, Underline, Strike, Reverse, Dim bool

	// Backdrop is the colour actually painted behind this element's
	// glyphs: its own declared background when it has one, else the
	// nearest ancestor's Backdrop. Unlike Bg/HasBg — the cascade value
	// of the non-inherited background-color property — the backdrop
	// models paint: an element with no background is transparent and
	// shows the ancestor sheet through it, so the backdrop propagates
	// through every element regardless of CSS inheritance rules.
	// Maintained by SyncBackdrop after each style computation.
	Backdrop    cell.RGB
	HasBackdrop bool

	// OwnBg reports that this element declared a background of its own
	// (as opposed to a backdrop showing through from an ancestor). Only
	// a block-level element's own background fills whole lines; an
	// inline element's paints just behind its glyphs. Never inherited.
	OwnBg bool

	// Display is the element's own display class; never inherited.
	Display Display

	// Auto horizontal margins (from margin-left/right or the margin
	// shorthand): both auto centres a block, left auto alone pushes it
	// right. Never inherited.
	MarginLeftAuto, MarginRightAuto bool

	// Justify is the element's own justify-content main-axis alignment,
	// meaningful only when the element is a flex/grid container: center
	// centres the container's laid-out row(s), right right-aligns them.
	// AlignNone/left values leave layout untouched. Never inherited.
	Justify Align

	// CSS-driven effects; never set by ForTag. Align shifts flushed
	// lines, Transform recases emitted text, Pre selects the verbatim
	// no-wrap path (CSS white-space: pre / pre-wrap).
	Align     Align
	Transform Transform
	Pre       bool
}

// SyncBackdrop updates the painted backdrop after declarations have
// been applied: an element with its own background paints on it; one
// without keeps the ancestor backdrop it inherited by copy (including
// the case where a higher-priority background:transparent cleared
// HasBg again).
func (s *Style) SyncBackdrop() {
	if s.HasBg {
		s.HasBackdrop, s.Backdrop = true, s.Bg
	}
}

var linkColor = cell.RGB{R: 95, G: 175, B: 255}

// ForTag returns parent adjusted by tag defaults (inheritance preserved).
func ForTag(tag string, parent Style) Style {
	s := parent
	// Non-inherited, per-element properties never carry over.
	s.OwnBg = false
	s.Display = DisplayUnset
	s.MarginLeftAuto, s.MarginRightAuto = false, false
	s.Justify = AlignNone
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
	case "center":
		// Legacy <center>: UA default text-align:center, inherited by
		// descendants exactly as the CSS property would be. Sites still
		// serving simple pages (google.com's no-JS homepage, say) centre
		// their logo/search/buttons with it.
		s.Align = AlignCenter
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
		s.HasBg, s.Bg, s.OwnBg = true, c, true
	}
	for _, decl := range strings.Split(attr("style"), ";") {
		k, v, ok := strings.Cut(decl, ":")
		if !ok {
			continue
		}
		switch prop := strings.TrimSpace(strings.ToLower(k)); prop {
		case "color":
			if c, ok := ParseColor(v); ok {
				s.HasFg, s.Fg = true, c
			}
		case "background-color", "background":
			if c, ok := ParseColor(v); ok {
				s.HasBg, s.Bg, s.OwnBg = true, c, true
			}
		case "display":
			if d, ok := ParseDisplay(v); ok {
				s.Display = d
			}
		case "margin", "margin-left", "margin-right":
			ApplyMargin(&s, prop, v)
		case "justify-content":
			if j, ok := ParseJustify(v); ok {
				s.Justify = j
			}
		}
	}
	s.SyncBackdrop()
	return s
}

// ParseJustify maps a justify-content value to a line alignment.
// center centres; flex-end/end/right right-align. flex-start, start,
// left, the space-* distributions and anything unknown report false:
// on a character grid they all leave the row where it already is.
func ParseJustify(v string) (Align, bool) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "center":
		return AlignCenter, true
	case "flex-end", "end", "right":
		return AlignRight, true
	}
	return AlignNone, false
}

// ApplyMargin applies a margin-left, margin-right or margin shorthand
// declaration to s, recording only whether each horizontal margin is
// auto (lengths are meaningless on a character grid). The shorthand
// takes 1-4 values: all; vertical horizontal; top horizontal bottom;
// top right bottom left.
func ApplyMargin(s *Style, prop, val string) {
	auto := func(v string) bool { return strings.EqualFold(strings.TrimSpace(v), "auto") }
	switch prop {
	case "margin-left":
		s.MarginLeftAuto = auto(val)
	case "margin-right":
		s.MarginRightAuto = auto(val)
	case "margin":
		parts := strings.Fields(val)
		var left, right string
		switch len(parts) {
		case 1:
			left, right = parts[0], parts[0]
		case 2, 3:
			left, right = parts[1], parts[1]
		case 4:
			right, left = parts[1], parts[3]
		default:
			return
		}
		s.MarginLeftAuto, s.MarginRightAuto = auto(left), auto(right)
	}
}
