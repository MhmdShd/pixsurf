package css

import (
	"strings"

	"github.com/andybalholm/cascadia"
	"github.com/tdewolff/parse/v2"
	cssparse "github.com/tdewolff/parse/v2/css"
)

// decl is one property declaration in source order.
type decl struct {
	prop      string
	val       string
	important bool
}

// rule is one compiled selector with its declarations.
type rule struct {
	sel   cascadia.Sel
	key   string // rightmost simple selector: tag, ".class", "#id" or "*"
	spec  cascadia.Specificity
	order int
	decls []decl
}

// parseSheet appends text's rules to rules. order numbers rules across
// sheets so document order breaks specificity ties. Malformed rules are
// skipped, never fatal: selectors that fail to compile (pseudo-elements,
// vendor junk) drop that selector only, unknown at-rule blocks are
// skipped wholesale, and @media blocks are kept only for screen-capable
// queries.
func parseSheet(text string, rules []rule, order int) ([]rule, int) {
	p := cssparse.NewParser(parse.NewInputString(text), false)
	var sels []string
	var cur []int // indices into rules for the open ruleset
	var at []bool // include/skip per open at-rule block
	skipping := func() bool {
		for _, ok := range at {
			if !ok {
				return true
			}
		}
		return false
	}
	for {
		gt, _, data := p.Next()
		switch gt {
		case cssparse.ErrorGrammar:
			return rules, order
		case cssparse.QualifiedRuleGrammar:
			// One comma-separated selector of the upcoming ruleset.
			sels = append(sels, joinTokens(p.Values()))
		case cssparse.BeginRulesetGrammar:
			sels = append(sels, joinTokens(p.Values()))
			cur = cur[:0]
			if !skipping() {
				for _, st := range sels {
					st = strings.TrimSpace(st)
					sel, err := cascadia.Parse(st)
					if err != nil {
						continue
					}
					rules = append(rules, rule{
						sel: sel, key: indexKey(st),
						spec: sel.Specificity(), order: order,
					})
					cur = append(cur, len(rules)-1)
					order++
				}
			}
			sels = sels[:0]
		case cssparse.EndRulesetGrammar:
			cur = nil
		case cssparse.DeclarationGrammar:
			if len(cur) == 0 {
				continue // declaration outside a ruleset (@font-face etc.)
			}
			val, imp := declValue(p.Values())
			if val == "" {
				continue
			}
			d := decl{prop: strings.ToLower(string(data)), val: val, important: imp}
			for _, i := range cur {
				rules[i].decls = append(rules[i].decls, d)
			}
		case cssparse.BeginAtRuleGrammar:
			ok := false
			if strings.ToLower(string(data)) == "@media" {
				ok = mediaOK(joinTokens(p.Values()))
			}
			at = append(at, ok)
		case cssparse.EndAtRuleGrammar:
			if len(at) > 0 {
				at = at[:len(at)-1]
			}
		}
	}
}

// parseInline parses a style="" attribute's declarations.
func parseInline(s string) []decl {
	if s == "" {
		return nil
	}
	p := cssparse.NewParser(parse.NewInputString(s), true)
	var out []decl
	for {
		gt, _, data := p.Next()
		switch gt {
		case cssparse.ErrorGrammar:
			return out
		case cssparse.DeclarationGrammar:
			if val, imp := declValue(p.Values()); val != "" {
				out = append(out, decl{
					prop: strings.ToLower(string(data)), val: val, important: imp,
				})
			}
		}
	}
}

func joinTokens(ts []cssparse.Token) string {
	var b strings.Builder
	for _, t := range ts {
		b.Write(t.Data)
	}
	return b.String()
}

// declValue joins value tokens and splits off a trailing !important.
func declValue(ts []cssparse.Token) (string, bool) {
	v := strings.TrimSpace(joinTokens(ts))
	low := strings.ToLower(v)
	if i := strings.LastIndexByte(low, '!'); i >= 0 && strings.TrimSpace(low[i+1:]) == "important" {
		return strings.TrimSpace(v[:i]), true
	}
	return v, false
}

// mediaOK reports whether a media query applies to us: it mentions
// screen or all, or is a bare query that never mentions print.
func mediaOK(q string) bool {
	q = strings.ToLower(q)
	if strings.Contains(q, "screen") || strings.Contains(q, "all") {
		return true
	}
	return !strings.Contains(q, "print")
}

// indexKey extracts the rightmost simple selector for bucketing,
// preferring #id over .class over tag; anything else lands in "*".
func indexKey(sel string) string {
	// Blank bracket/paren contents so combinator and pseudo scanning
	// cannot trip on them; byte positions stay aligned.
	b := []byte(sel)
	depth := 0
	for i := range b {
		switch b[i] {
		case '[', '(':
			depth++
			b[i] = 0
		case ']', ')':
			depth--
			b[i] = 0
		default:
			if depth > 0 {
				b[i] = 0
			}
		}
	}
	s := string(b)
	start := 0
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case ' ', '\t', '\n', '\r', '>', '+', '~':
			start = i + 1
		}
	}
	comp := s[start:]
	if i := strings.IndexByte(comp, ':'); i >= 0 {
		comp = comp[:i] // strip pseudo-classes
	}
	if i := strings.IndexByte(comp, '#'); i >= 0 {
		if id := ident(comp[i+1:]); id != "" {
			return "#" + id
		}
	}
	if i := strings.IndexByte(comp, '.'); i >= 0 {
		if cl := ident(comp[i+1:]); cl != "" {
			return "." + cl
		}
	}
	if t := ident(comp); t != "" && t != "*" {
		return strings.ToLower(t)
	}
	return "*"
}

// ident returns the leading identifier run of s.
func ident(s string) string {
	for i := 0; i < len(s); i++ {
		c := s[i]
		if c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' ||
			c >= '0' && c <= '9' || c == '-' || c == '_' || c >= 0x80 {
			continue
		}
		return s[:i]
	}
	return s
}
