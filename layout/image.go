package layout

import (
	"image"
	"strings"

	"github.com/MhmdShd/pixsurf/cell"
	"github.com/MhmdShd/pixsurf/dom"
	"github.com/MhmdShd/pixsurf/render"
	"github.com/MhmdShd/pixsurf/style"
)

// maxImageRows caps how tall a rendered image may be, in terminal rows.
const maxImageRows = 15

// emitImage renders an <img>: half-block pixel lines when a fetcher is set
// and succeeds, otherwise a dim [alt]/[image] placeholder.
func (w *walker) emitImage(n *dom.Node, st style.Style) {
	if src := dom.Attr(n, "src"); w.images != nil && src != "" {
		if img, err := w.images(w.src.Resolve(src)); err == nil && img != nil {
			if w.emitPixels(img) {
				return
			}
		}
	}
	alt := strings.TrimSpace(dom.Attr(n, "alt"))
	text := "[image]"
	if alt != "" {
		text = "[" + alt + "]"
	}
	st.Dim = true
	w.emitText(text, st)
}

// emitPixels converts img to half-block cells scaled to fit the content
// width (1 cell per source pixel column at most), proportional height
// capped at maxImageRows, and emits them as standalone lines.
func (w *walker) emitPixels(img image.Image) bool {
	b := img.Bounds()
	if b.Dx() < 1 || b.Dy() < 1 {
		return false
	}
	cols := b.Dx()
	if cols > w.width-w.indentCols() {
		cols = w.width - w.indentCols()
	}
	if cols < 1 {
		cols = 1
	}
	rows := (b.Dy()*cols/b.Dx() + 1) / 2 // 2 pixels per cell row
	if rows < 1 {
		rows = 1
	}
	if rows > maxImageRows {
		rows = maxImageRows
	}
	grid := render.ToCells(img, cols, rows)

	w.flushLine()
	for _, gr := range grid {
		for _, pc := range gr {
			w.putCell(cell.Cell{
				Rune:  '▀',
				Fg:    cell.RGB(pc.Top),
				Bg:    cell.RGB(pc.Bottom),
				HasFg: true,
				HasBg: true,
			})
		}
		w.flushLine()
	}
	return true
}

// putCell appends a prebuilt cell to the current line, keeping the
// link-range and line-start bookkeeping putRune provides.
func (w *walker) putCell(c cell.Cell) {
	if !w.started {
		w.startLine()
	}
	if w.linkURL != "" && w.linkOpen < 0 {
		w.linkOpen = w.col
	}
	w.line = append(w.line, c)
	w.col++
}
