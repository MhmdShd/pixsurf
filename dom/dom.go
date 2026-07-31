// Package dom parses HTML and resolves URLs against the page base.
package dom

import (
	"net/url"
	"strings"

	"golang.org/x/net/html"
)

// Node and ElementNode alias x/net/html types so callers avoid the import.
type Node = html.Node

const ElementNode = html.ElementNode
const TextNode = html.TextNode

// Doc is a parsed page.
type Doc struct {
	Root *Node
	Base *url.URL
}

// Parse parses src (already UTF-8) with pageURL as the default base.
// A <base href> tag overrides the base.
func Parse(src, pageURL string) (*Doc, error) {
	root, err := html.Parse(strings.NewReader(src))
	if err != nil {
		return nil, err
	}
	base, err := url.Parse(pageURL)
	if err != nil {
		return nil, err
	}
	Walk(root, func(n *Node) {
		if n.Type == ElementNode && n.Data == "base" {
			if href := Attr(n, "href"); href != "" {
				if u, err := base.Parse(href); err == nil {
					base = u
				}
			}
		}
	})
	return &Doc{Root: root, Base: base}, nil
}

// Resolve turns a possibly-relative href into an absolute URL string.
func (d *Doc) Resolve(href string) string {
	u, err := d.Base.Parse(strings.TrimSpace(href))
	if err != nil {
		return href
	}
	return u.String()
}

// Attr returns the value of the named attribute (case-insensitive), or "".
func Attr(n *Node, key string) string {
	for _, a := range n.Attr {
		if strings.EqualFold(a.Key, key) {
			return a.Val
		}
	}
	return ""
}

// Walk calls fn for every node in depth-first order.
func Walk(n *Node, fn func(*Node)) {
	fn(n)
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		Walk(c, fn)
	}
}
