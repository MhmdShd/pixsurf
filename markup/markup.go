// Package markup converts non-HTML document bodies (Markdown, plain text)
// into HTML so the existing HTML pipeline can render them.
package markup

import (
	"bytes"
	"html"
	"net/url"
	"strings"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
)

var md = goldmark.New(
	goldmark.WithExtensions(
		extension.Table,
		extension.Strikethrough,
		extension.Linkify,
		extension.TaskList,
	),
)

// ToHTML turns a fetched body into HTML based on its media type and URL.
// Markdown (text/markdown, text/x-markdown, or a .md/.markdown URL path)
// is converted with GFM extensions; text/plain is escaped and wrapped in
// <pre>; anything else passes through unchanged.
func ToHTML(body, contentType, rawURL string) string {
	if isMarkdown(contentType, rawURL) {
		var buf bytes.Buffer
		if err := md.Convert([]byte(body), &buf); err != nil {
			// Conversion failure: fall back to the escaped-plain-text path
			// so the user still sees the document.
			return wrapPre(body)
		}
		return "<html><body>" + buf.String() + "</body></html>"
	}
	if contentType == "text/plain" {
		return wrapPre(body)
	}
	return body
}

func wrapPre(body string) string {
	return "<html><body><pre>" + html.EscapeString(body) + "</pre></body></html>"
}

func isMarkdown(contentType, rawURL string) bool {
	if contentType == "text/markdown" || contentType == "text/x-markdown" {
		return true
	}
	path := rawURL
	if u, err := url.Parse(rawURL); err == nil {
		path = u.Path
	}
	path = strings.ToLower(path)
	return strings.HasSuffix(path, ".md") || strings.HasSuffix(path, ".markdown")
}
