package layout

import (
	"strings"

	"github.com/MhmdShd/pixsurf/cell"
	"github.com/MhmdShd/pixsurf/dom"
	"github.com/MhmdShd/pixsurf/style"
)

// renderTable lays out a <table>: one output row-group per <tr>, columns
// sharing the width equally with 1-space gutters. Each cell's content is
// laid out by a nested mini-layout at the column width, so nested tables
// flatten into sequential blocks naturally. colspan is ignored.
func (w *walker) renderTable(n *dom.Node, st style.Style) {
	rows := tableRows(n)
	if len(rows) == 0 { // no tr/td structure: fall back to block flow
		w.blockStart()
		w.recordAnchor(n)
		w.walkChildren(n, st)
		w.blockEnd()
		return
	}
	w.blockStart()
	w.recordAnchor(n)
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == dom.ElementNode && strings.EqualFold(c.Data, "caption") {
			w.walkChildren(c, style.ForTag("caption", st))
			w.flushLine()
		}
	}
	for _, row := range rows {
		if len(row) == 0 {
			continue
		}
		ncols := len(row)
		colWidth := (w.width - ncols + 1) / ncols
		if colWidth < 1 {
			colWidth = 1
		}
		subs := make([]*Document, ncols)
		height := 0
		for j, cn := range row {
			subs[j] = w.miniLayout(cn, colWidth, st)
			if len(subs[j].Lines) > height {
				height = len(subs[j].Lines)
			}
		}
		if height > 0 {
			w.emitRow(subs, colWidth, height)
		}
	}
	w.blockEnd()
}

// tableRows collects this table's <tr> elements (via thead/tbody/tfoot),
// each as its ordered <td>/<th> cells. Nested tables are not descended
// into; they belong to their cell's mini-layout.
func tableRows(table *dom.Node) [][]*dom.Node {
	var rows [][]*dom.Node
	var walk func(*dom.Node)
	walk = func(m *dom.Node) {
		for c := m.FirstChild; c != nil; c = c.NextSibling {
			if c.Type != dom.ElementNode {
				continue
			}
			switch strings.ToLower(c.Data) {
			case "tr":
				rows = append(rows, rowCells(c))
			case "table", "td", "th", "caption":
				// nested table, stray cell, or caption: not this table's rows
			default:
				walk(c)
			}
		}
	}
	walk(table)
	return rows
}

// rowCells returns the <td>/<th> children of a <tr>.
func rowCells(tr *dom.Node) []*dom.Node {
	var cells []*dom.Node
	for c := tr.FirstChild; c != nil; c = c.NextSibling {
		if c.Type != dom.ElementNode {
			continue
		}
		switch strings.ToLower(c.Data) {
		case "td", "th":
			cells = append(cells, c)
		}
	}
	return cells
}

// miniLayout renders one node's subtree into a fresh narrow Document.
func (w *walker) miniLayout(n *dom.Node, width int, st style.Style) *Document {
	sub := &Document{Anchors: map[string]int{}}
	mw := &walker{doc: sub, src: w.src, width: width, images: w.images, values: w.values, linkOpen: -1, formIdx: -1}
	mw.linkURL = w.linkURL // an <a> wrapping the table keeps its links inside
	mw.skipChrome = w.skipChrome
	mw.hfDepth = w.hfDepth
	mw.renderNode(n, st)
	mw.flushLine()
	return sub
}

// truncCells cuts a cell line to max columns, blanking a wide rune whose
// continuation would be severed. Index == display column, so a plain slice
// is column-accurate.
func truncCells(line []cell.Cell, max int) []cell.Cell {
	if len(line) <= max {
		return line
	}
	out := line[:max]
	if max > 0 && line[max].Continuation {
		out = append(append([]cell.Cell{}, out[:max-1]...), cell.Cell{Rune: ' '})
	}
	return out
}

// emitRow merges the cells' mini-layouts side by side into output lines,
// padding short cells and offsetting link ranges and anchors into page
// coordinates.
func (w *walker) emitRow(subs []*Document, colWidth, height int) {
	w.flushLine()
	if w.pendingBlank && len(w.doc.Lines) > 0 {
		w.doc.Lines = append(w.doc.Lines, nil)
	}
	w.pendingBlank = false
	rowBase := len(w.doc.Lines)

	xoffs := make([]int, len(subs))
	x := 0
	for j := range subs {
		if j > 0 {
			x++ // gutter
		}
		xoffs[j] = x
		x += colWidth
	}
	total := x

	// Merge the cells' lines, dropping merged lines left entirely empty
	// (cell-internal blank separators and clipped-away content); newIdx
	// maps a sub line index to its kept output offset within the row.
	newIdx := make([]int, height)
	for y := 0; y < height; y++ {
		line := make([]cell.Cell, 0, total)
		for j, sub := range subs {
			if j > 0 {
				line = append(line, cell.Cell{Rune: ' '})
			}
			if y < len(sub.Lines) {
				line = append(line, truncCells(sub.Lines[y], colWidth)...)
			}
			for len(line) < xoffs[j]+colWidth {
				line = append(line, cell.Cell{})
			}
		}
		line = truncCells(line, w.width)
		if emptyCells(line) {
			newIdx[y] = -1
			continue
		}
		newIdx[y] = len(w.doc.Lines) - rowBase
		w.doc.Lines = append(w.doc.Lines, line)
	}

	for j, sub := range subs {
		for _, l := range sub.Links {
			if l.Line >= height || newIdx[l.Line] < 0 {
				continue // line was dropped: no visible cells to hit
			}
			start, end := l.Start+xoffs[j], l.End+xoffs[j]
			if start >= w.width { // column clipped entirely
				continue
			}
			if end > w.width {
				end = w.width
			}
			if s := xoffs[j] + colWidth; end > s { // clamp to own column
				end = s
			}
			if end <= start {
				continue
			}
			w.doc.Links = append(w.doc.Links, Link{
				Line:  rowBase + newIdx[l.Line],
				Start: start,
				End:   end,
				URL:   l.URL,
			})
		}
		for id, ln := range sub.Anchors {
			if _, ok := w.doc.Anchors[id]; !ok {
				if ln >= height {
					ln = height - 1
				}
				for ln > 0 && newIdx[ln] < 0 {
					ln-- // anchor on a dropped line: nearest kept above
				}
				kept := newIdx[ln]
				if kept < 0 {
					kept = 0
				}
				at := rowBase + kept
				if at >= len(w.doc.Lines) { // every merged line dropped
					at = len(w.doc.Lines) - 1
				}
				if at < 0 {
					at = 0
				}
				w.doc.Anchors[id] = at
			}
		}
	}
}

// emptyCells reports whether a merged row line shows nothing (only
// spaces, padding, and continuations).
func emptyCells(line []cell.Cell) bool {
	for _, c := range line {
		if c.Continuation {
			continue
		}
		if c.Rune != 0 && c.Rune != ' ' {
			return false
		}
	}
	return true
}
