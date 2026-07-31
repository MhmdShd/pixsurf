package layout

import (
	"strings"

	"github.com/MhmdShd/pixsurf/dom"
	"github.com/MhmdShd/pixsurf/style"
)

// Content-extraction thresholds.
const (
	minMainText   = 200 // a candidate root needs this much text to win
	minMainShare  = 0.3 // ...and this share of the body's text
	maxChromeCull = 0.9 // never skip chrome holding more than this share
)

// contentRoot picks the node the body walk should start from: a <main>
// element (or the page's single <article>) when it carries enough of the
// page's text, otherwise the <body> (or the whole tree as a last resort).
func contentRoot(d *dom.Doc) *dom.Node {
	body := findElement(d.Root, "body")
	if body == nil {
		body = d.Root
	}
	cand := findElement(body, "main")
	if cand == nil {
		arts := findElements(body, "article")
		if len(arts) == 1 {
			cand = arts[0]
		}
	}
	if cand == nil {
		return body
	}
	ct, bt := visibleTextLen(cand), visibleTextLen(body)
	if ct >= minMainText && bt > 0 && float64(ct) >= minMainShare*float64(bt) {
		return cand
	}
	return body
}

// isChrome reports whether an element is navigation/branding furniture
// whose subtree should be skipped inside the content root.
func isChrome(n *dom.Node) bool {
	if n.Type != dom.ElementNode {
		return false
	}
	switch strings.ToLower(n.Data) {
	case "nav", "header", "footer", "aside":
		return true
	}
	switch strings.ToLower(strings.TrimSpace(dom.Attr(n, "role"))) {
	case "navigation", "banner", "contentinfo", "search":
		return true
	}
	return false
}

// chromeSkipSafe reports whether skipping chrome subtrees under root
// would leave enough content behind: skipping is unsafe when chrome
// holds more than maxChromeCull of the root's visible text (the page is
// nav-structured, e.g. a link aggregator).
func chromeSkipSafe(root *dom.Node) bool {
	total := visibleTextLen(root)
	if total == 0 {
		return false
	}
	chrome := chromeTextLen(root)
	return float64(chrome) <= maxChromeCull*float64(total)
}

// chromeTextLen sums the visible text inside root's outermost chrome
// subtrees (nested chrome is not double-counted).
func chromeTextLen(root *dom.Node) int {
	var sum int
	var walk func(*dom.Node)
	walk = func(n *dom.Node) {
		if n != root && isChrome(n) {
			sum += visibleTextLen(n)
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(root)
	return sum
}

// visibleTextLen measures the renderable text under n: hidden subtrees
// (script/style/etc.) contribute nothing and each text node is counted
// with surrounding whitespace trimmed.
func visibleTextLen(n *dom.Node) int {
	if n.Type == dom.TextNode {
		return len(strings.TrimSpace(n.Data))
	}
	if n.Type == dom.ElementNode && style.Hidden(strings.ToLower(n.Data)) {
		return 0
	}
	var sum int
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		sum += visibleTextLen(c)
	}
	return sum
}

// findElement returns the first element with the given tag under root.
func findElement(root *dom.Node, tag string) *dom.Node {
	els := findN(root, tag, 1)
	if len(els) == 0 {
		return nil
	}
	return els[0]
}

// findElements returns up to two elements with the given tag; callers
// only need to distinguish zero, one, and many.
func findElements(root *dom.Node, tag string) []*dom.Node {
	return findN(root, tag, 2)
}

func findN(root *dom.Node, tag string, max int) []*dom.Node {
	var out []*dom.Node
	var walk func(*dom.Node)
	walk = func(n *dom.Node) {
		if len(out) >= max {
			return
		}
		if n.Type == dom.ElementNode && strings.EqualFold(n.Data, tag) {
			out = append(out, n)
			return // nested same-tag elements don't add candidates
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walk(c)
		}
	}
	walk(root)
	return out
}
