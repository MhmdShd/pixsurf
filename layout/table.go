package layout

import (
	"net/url"
	"strings"

	"github.com/MhmdShd/pixsurf/cell"
	"github.com/MhmdShd/pixsurf/dom"
	"github.com/MhmdShd/pixsurf/style"
)

// renderTable lays out a <table>: one output row-group per <tr>, column
// widths proportional to their content with 1-space gutters. Each cell's
// content is laid out by a nested mini-layout at the column width, so
// nested tables flatten into sequential blocks naturally. colspan is
// ignored.
func (w *walker) renderTable(n *dom.Node, st, parentSt style.Style) {
	rows := tableRows(n)
	if len(rows) == 0 { // no tr/td structure: fall back to block flow
		w.blockStart(parentSt)
		w.recordAnchor(n)
		w.walkChildren(n, st)
		w.blockEnd(parentSt)
		return
	}
	w.blockStart(parentSt)
	w.recordAnchor(n)
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == dom.ElementNode && strings.EqualFold(c.Data, "caption") {
			w.walkChildren(c, style.ForTag("caption", st))
			w.flushLine()
		}
	}
	// Column widths are content-proportional, computed per row shape
	// (rows sharing a cell count share columns), so a full-width
	// single-cell row does not inflate a narrow label column.
	widthsByN := map[int][]int{}
	for _, row := range rows {
		n := len(row)
		if n == 0 {
			continue
		}
		nat := widthsByN[n]
		if nat == nil {
			nat = make([]int, n)
			widthsByN[n] = nat
		}
		for j, cn := range row {
			if v := w.naturalCellWidth(cn, st); v > nat[j] {
				nat[j] = v
			}
		}
	}
	for n, nat := range widthsByN {
		widthsByN[n] = fitColumns(nat, w.width)
	}
	for _, row := range rows {
		if len(row) == 0 {
			continue
		}
		widths := widthsByN[len(row)]
		subs := make([]*Document, len(row))
		height := 0
		for j, cn := range row {
			subs[j] = w.miniLayout(cn, widths[j], st)
			if len(subs[j].Lines) > height {
				height = len(subs[j].Lines)
			}
		}
		if height > 0 {
			w.emitRow(subs, widths, height, st)
		}
	}
	w.blockEnd(parentSt)
}

// defaultImageCols is the assumed display width, in cells, of an <img>
// without a usable width attribute (emitPixels renders cellPxW px per cell).
const defaultImageCols = 3

// naturalCellWidth is the display width a cell's content wants: its
// longest laid-out line at full width (so wrapping never occurs), with
// images counted at their declared pixel width.
func (w *walker) naturalCellWidth(cn *dom.Node, st style.Style) int {
	saved := w.images
	w.images = nil // measure with placeholders; must not fetch
	w.measuring = true
	sub := w.miniLayout(cn, w.width, st)
	w.measuring = false
	w.images = saved
	widest := 0
	for _, ln := range sub.Lines {
		if len(ln) > widest {
			widest = len(ln)
		}
	}
	if w.images != nil { // images render as pixels: account for their width
		dom.Walk(cn, func(m *dom.Node) {
			if m.Type != dom.ElementNode || !strings.EqualFold(m.Data, "img") {
				return
			}
			aw, hasW := attrInt(m, "width")
			ah, hasH := attrInt(m, "height")
			if (hasW && aw <= spacerMaxPx) || (hasH && ah <= spacerMaxPx) {
				return // spacer: renders nothing
			}
			cols := defaultImageCols
			if hasW && aw > 0 {
				cols = (aw + cellPxW/2) / cellPxW
				if cols < 1 {
					cols = 1
				}
			}
			if cols > w.width {
				cols = w.width
			}
			if cols > widest {
				widest = cols
			}
		})
	}
	return widest
}

// fitColumns turns natural column widths into assigned widths within
// avail total columns (1-space gutters between columns). When the
// naturals fit, each column gets its natural width and the remainder
// stays unused; otherwise columns scale down proportionally with a
// floor so narrow label columns keep their width.
func fitColumns(natural []int, avail int) []int {
	n := len(natural)
	out := make([]int, n)
	sum := 0
	for j, v := range natural {
		if v < 1 {
			v = 1
		}
		out[j] = v
		sum += v
	}
	if sum+n-1 <= avail {
		return out
	}
	target := avail - (n - 1)
	if target < n {
		target = n // at least one column each; overflow is clipped later
	}
	floor := 3
	if floor*n > target {
		floor = 1
	}
	total := 0
	for j := range out {
		v := out[j] * target / sum
		if v < floor {
			v = floor
		}
		out[j] = v
		total += v
	}
	for total > target { // floors may overshoot: shave the widest columns
		k := -1
		for j := range out {
			if out[j] > floor && (k < 0 || out[j] > out[k]) {
				k = j
			}
		}
		if k < 0 {
			break
		}
		out[k]--
		total--
	}
	return out
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
// A <form> open around the table stays open inside the cell: the cell
// works against a local copy of it (fresh Hidden map so measuring never
// double-adds), which emitRow merges back into the page document.
func (w *walker) miniLayout(n *dom.Node, width int, st style.Style) *Document {
	sub := &Document{Anchors: map[string]int{}}
	mw := &walker{doc: sub, src: w.src, width: width, images: w.images, values: w.values, styles: w.styles, linkOpen: -1, formIdx: -1}
	if w.formIdx >= 0 {
		pf := w.doc.Forms[w.formIdx]
		sub.Forms = append(sub.Forms, Form{
			Action: pf.Action, Method: pf.Method, Hidden: url.Values{},
			SubmitLine: -1, SubmitStart: -1, SubmitEnd: -1,
		})
		mw.formIdx = 0
	}
	mw.hasStyleBg, mw.styleBg = st.HasBackdrop, st.Backdrop
	mw.linkURL = w.linkURL // an <a> wrapping the table keeps its links inside
	mw.skipChrome = w.skipChrome
	mw.hfDepth = w.hfDepth
	mw.measuring = w.measuring
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
// coordinates. When the table's style carries a background, gutter and
// padding cells take it so the row renders as a solid band.
func (w *walker) emitRow(subs []*Document, widths []int, height int, st style.Style) {
	w.flushLine()
	if w.pendingBlank && len(w.doc.Lines) > 0 {
		w.appendBlank()
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
		x += widths[j]
	}
	total := x

	// Merge the cells' lines, dropping merged lines left entirely empty
	// (cell-internal blank separators and clipped-away content); newIdx
	// maps a sub line index to its kept output offset within the row.
	gutter := cell.Cell{Rune: ' '}
	pad := cell.Cell{}
	if st.HasBackdrop {
		gutter.HasBg, gutter.Bg = true, st.Backdrop
		pad = gutter
	}
	newIdx := make([]int, height)
	for y := 0; y < height; y++ {
		line := make([]cell.Cell, 0, total)
		for j, sub := range subs {
			if j > 0 {
				line = append(line, gutter)
			}
			if y < len(sub.Lines) {
				line = append(line, truncCells(sub.Lines[y], widths[j])...)
			}
			for len(line) < xoffs[j]+widths[j] {
				line = append(line, pad)
			}
		}
		line = truncCells(line, w.width)
		if emptyCells(line) {
			newIdx[y] = -1
			continue
		}
		for st.HasBackdrop && len(line) < w.width {
			line = append(line, gutter)
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
			if s := xoffs[j] + widths[j]; end > s { // clamp to own column
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
		// Form controls inside this cell belong to the page: forms,
		// fields, submit regions, and hidden inputs collected by the
		// cell's mini-layout move into page coordinates here.
		formMap := make([]int, len(sub.Forms))
		for si := range sub.Forms {
			sf := sub.Forms[si]
			if si == 0 && w.formIdx >= 0 {
				// the copy of the open form seeded by miniLayout
				formMap[si] = w.formIdx
				pf := &w.doc.Forms[w.formIdx]
				for k, vs := range sf.Hidden {
					for _, v := range vs {
						pf.Hidden.Add(k, v)
					}
				}
				if ln, s, e := offsetRegion(sf.SubmitLine, sf.SubmitStart, sf.SubmitEnd,
					rowBase, newIdx, xoffs[j], widths[j], w.width, height); pf.SubmitLine < 0 && ln >= 0 {
					pf.SubmitLine, pf.SubmitStart, pf.SubmitEnd = ln, s, e
				}
				continue
			}
			sf.SubmitLine, sf.SubmitStart, sf.SubmitEnd = offsetRegion(
				sf.SubmitLine, sf.SubmitStart, sf.SubmitEnd,
				rowBase, newIdx, xoffs[j], widths[j], w.width, height)
			w.doc.Forms = append(w.doc.Forms, sf)
			formMap[si] = len(w.doc.Forms) - 1
		}
		for _, f := range sub.Fields {
			ln, s, e := offsetRegion(f.Line, f.Start, f.End,
				rowBase, newIdx, xoffs[j], widths[j], w.width, height)
			if ln < 0 {
				continue // line dropped or box clipped entirely
			}
			f.Line, f.Start, f.End = ln, s, e
			f.FormIdx = formMap[f.FormIdx]
			w.doc.Fields = append(w.doc.Fields, f)
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

// offsetRegion maps a (line, start, end) region from cell-local to page
// coordinates, clipping to the page width and the cell's own column like
// link ranges. It returns -1s when the region is unset, its line was
// dropped, or the box was clipped away entirely.
func offsetRegion(line, start, end, rowBase int, newIdx []int, xoff, colW, width, height int) (int, int, int) {
	if line < 0 || line >= height || newIdx[line] < 0 {
		return -1, -1, -1
	}
	start, end = start+xoff, end+xoff
	if start >= width {
		return -1, -1, -1
	}
	if end > width {
		end = width
	}
	if s := xoff + colW; end > s {
		end = s
	}
	if end <= start {
		return -1, -1, -1
	}
	return rowBase + newIdx[line], start, end
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
