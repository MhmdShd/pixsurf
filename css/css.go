// Package css parses author stylesheets and computes cascaded styles
// for DOM nodes. It implements layout's StyleResolver interface:
// Hidden prunes display:none / visibility:hidden subtrees and Resolve
// returns a node's computed style.Style given its parent's.
package css

import (
	"sort"
	"strconv"
	"strings"

	"github.com/MhmdShd/pixsurf/cell"
	"github.com/MhmdShd/pixsurf/dom"
	"github.com/MhmdShd/pixsurf/style"
)

// Engine holds parsed rules indexed by rightmost simple selector so
// each node only tests candidate rules, plus per-node memoisation.
type Engine struct {
	rules  []rule
	index  map[string][]int
	decls  map[*dom.Node][]decl // cascade-ordered declarations
	styles map[*dom.Node]style.Style
	vis    map[*dom.Node]visInfo
}

type visInfo struct {
	displayNone bool
	visibility  string // "", "visible", "hidden", "collapse"
}

// New parses sheets — stylesheet texts in document order — into an
// Engine. Malformed rules are dropped, never fatal.
func New(sheets []string) *Engine {
	e := &Engine{
		index:  map[string][]int{},
		decls:  map[*dom.Node][]decl{},
		styles: map[*dom.Node]style.Style{},
		vis:    map[*dom.Node]visInfo{},
	}
	order := 0
	for _, s := range sheets {
		e.rules, order = parseSheet(s, e.rules, order)
	}
	for i, r := range e.rules {
		e.index[r.key] = append(e.index[r.key], i)
	}
	return e
}

// Resolve returns n's computed style given its parent's computed
// style. Cascade order, lowest to highest: UA defaults (style.ForTag),
// legacy color/bgcolor attributes, author rules by (specificity,
// source order), the inline style attribute, then !important
// declarations in the same relative order.
func (e *Engine) Resolve(n *dom.Node, parent style.Style) style.Style {
	if s, ok := e.styles[n]; ok {
		return s
	}
	s := style.ForTag(strings.ToLower(n.Data), parent)
	// background-color does not inherit; drop the parent's copy.
	s.HasBg, s.Bg = false, cell.RGB{}
	if c, ok := parseColor(dom.Attr(n, "color")); ok {
		s.HasFg, s.Fg = true, c
	}
	if c, ok := parseColor(dom.Attr(n, "bgcolor")); ok {
		s.HasBg, s.Bg = true, c
	}
	for _, d := range e.cascade(n) {
		applyDecl(&s, d.prop, d.val)
	}
	e.styles[n] = s
	return s
}

// Hidden reports whether n renders nothing: display:none on n or any
// ancestor, or an effective visibility of hidden/collapse. Subtree
// semantics: descendants of a display:none element are Hidden;
// visibility follows CSS inheritance, so visibility:visible on a
// descendant of a visibility:hidden ancestor un-hides it.
func (e *Engine) Hidden(n *dom.Node) bool {
	if n != nil && n.Type != dom.ElementNode {
		n = n.Parent
	}
	vis := ""
	for p := n; p != nil; p = p.Parent {
		if p.Type != dom.ElementNode {
			continue
		}
		vi := e.visInfo(p)
		if vi.displayNone {
			return true
		}
		if vis == "" && vi.visibility != "" {
			vis = vi.visibility
		}
	}
	return vis == "hidden" || vis == "collapse"
}

// visInfo returns n's own cascaded display:none / visibility values.
func (e *Engine) visInfo(n *dom.Node) visInfo {
	if v, ok := e.vis[n]; ok {
		return v
	}
	var v visInfo
	for _, d := range e.cascade(n) {
		switch d.prop {
		case "display":
			v.displayNone = strings.EqualFold(strings.TrimSpace(d.val), "none")
		case "visibility":
			v.visibility = strings.ToLower(strings.TrimSpace(d.val))
		}
	}
	e.vis[n] = v
	return v
}

// cascade returns n's declarations in application order: matching
// rules' normal declarations sorted by (specificity, source order),
// inline normal declarations, then the same again for !important.
func (e *Engine) cascade(n *dom.Node) []decl {
	if d, ok := e.decls[n]; ok {
		return d
	}
	var idxs []int
	idxs = append(idxs, e.index[strings.ToLower(n.Data)]...)
	if id := dom.Attr(n, "id"); id != "" {
		idxs = append(idxs, e.index["#"+id]...)
	}
	for _, c := range strings.Fields(dom.Attr(n, "class")) {
		idxs = append(idxs, e.index["."+c]...)
	}
	idxs = append(idxs, e.index["*"]...)
	var matched []int
	for _, i := range idxs {
		if e.rules[i].sel.Match(n) {
			matched = append(matched, i)
		}
	}
	sort.Slice(matched, func(a, b int) bool {
		ra, rb := &e.rules[matched[a]], &e.rules[matched[b]]
		if ra.spec != rb.spec {
			return ra.spec.Less(rb.spec)
		}
		return ra.order < rb.order
	})
	inline := parseInline(dom.Attr(n, "style"))
	var out []decl
	for _, imp := range []bool{false, true} {
		for _, i := range matched {
			for _, d := range e.rules[i].decls {
				if d.important == imp {
					out = append(out, d)
				}
			}
		}
		for _, d := range inline {
			if d.important == imp {
				out = append(out, d)
			}
		}
	}
	e.decls[n] = out
	return out
}

// applyDecl overlays one declaration onto s. Unknown properties and
// unparseable values are ignored.
func applyDecl(s *style.Style, prop, val string) {
	v := strings.ToLower(strings.TrimSpace(val))
	switch prop {
	case "color":
		if c, ok := parseColor(val); ok {
			s.HasFg, s.Fg = true, c
		}
	case "background-color":
		if v == "transparent" || v == "none" {
			s.HasBg = false
			return
		}
		if c, ok := parseColor(val); ok {
			s.HasBg, s.Bg = true, c
		}
	case "background":
		if v == "transparent" || v == "none" {
			s.HasBg = false
			return
		}
		if c, ok := backgroundColor(val); ok {
			s.HasBg, s.Bg = true, c
		}
	case "font-weight":
		switch v {
		case "bold", "bolder":
			s.Bold = true
		case "normal", "lighter":
			s.Bold = false
		default:
			if n, err := strconv.Atoi(v); err == nil {
				s.Bold = n >= 600
			}
		}
	case "font-style":
		switch v {
		case "italic", "oblique":
			s.Italic = true
		case "normal":
			s.Italic = false
		}
	case "text-decoration", "text-decoration-line":
		for _, w := range strings.Fields(v) {
			switch w {
			case "none":
				s.Underline, s.Strike = false, false
			case "underline":
				s.Underline = true
			case "line-through":
				s.Strike = true
			}
		}
	case "text-align":
		switch v {
		case "left", "start":
			s.Align = style.AlignLeft
		case "center":
			s.Align = style.AlignCenter
		case "right", "end":
			s.Align = style.AlignRight
		}
	case "text-transform":
		switch v {
		case "uppercase":
			s.Transform = style.TransformUpper
		case "lowercase":
			s.Transform = style.TransformLower
		case "capitalize":
			s.Transform = style.TransformCapitalize
		case "none":
			s.Transform = style.TransformNone
		}
	case "white-space":
		switch v {
		case "pre", "pre-wrap":
			s.Pre = true
		case "normal", "nowrap", "pre-line":
			s.Pre = false
		}
	}
}

// backgroundColor finds the colour component of a background shorthand
// by trying each top-level space- or comma-separated part.
func backgroundColor(val string) (cell.RGB, bool) {
	depth, start := 0, 0
	for i := 0; i <= len(val); i++ {
		if i == len(val) || (depth == 0 && (val[i] == ' ' || val[i] == ',')) {
			if part := strings.TrimSpace(val[start:i]); part != "" {
				if c, ok := parseColor(part); ok {
					return c, true
				}
			}
			start = i + 1
			continue
		}
		switch val[i] {
		case '(':
			depth++
		case ')':
			depth--
		}
	}
	return cell.RGB{}, false
}
