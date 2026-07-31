package layout

import (
	"image"
	"strconv"
	"strings"

	"github.com/MhmdShd/pixsurf/cell"
	"github.com/MhmdShd/pixsurf/dom"
	"github.com/MhmdShd/pixsurf/render"
	"github.com/MhmdShd/pixsurf/style"
)

// maxImageRows caps how tall a rendered image may be, in terminal rows.
const maxImageRows = 20

// A terminal cell models 8 CSS px of width and 16 px of height (half-block
// rendering gives 2 vertical pixels per cell, i.e. 8 px per half-row).
const (
	cellPxW = 8
	cellPxH = 16
)

// spacerMaxPx: an image whose declared or natural width/height is at or
// below this is a layout spacer or tracker, not content; it is skipped
// entirely (no fetch when declared, no output, no placeholder).
const spacerMaxPx = 4

// attrInt parses an integer element attribute; ok is false when the
// attribute is absent or not a number.
func attrInt(n *dom.Node, name string) (int, bool) {
	v, err := strconv.Atoi(strings.TrimSpace(dom.Attr(n, name)))
	if err != nil {
		return 0, false
	}
	return v, true
}

// emitImage renders an <img>: half-block pixel lines when a fetcher is set
// and succeeds, otherwise a dim [alt]/[image] placeholder. Spacer/tracker
// images (declared or natural size <= spacerMaxPx) produce no output.
func (w *walker) emitImage(n *dom.Node, st style.Style) {
	aw, hasW := attrInt(n, "width")
	ah, hasH := attrInt(n, "height")
	if (hasW && aw <= spacerMaxPx) || (hasH && ah <= spacerMaxPx) {
		return // spacer: skip without fetching
	}
	if src := dom.Attr(n, "src"); w.images != nil && src != "" {
		if img, err := w.images(w.src.Resolve(src)); err == nil && img != nil {
			b := img.Bounds()
			if b.Dx() <= spacerMaxPx || b.Dy() <= spacerMaxPx {
				return // decoded to a tiny spacer/tracker: skip
			}
			// Display size in CSS px: attributes win over natural size;
			// a single attribute scales the other axis by aspect ratio.
			dw, dh := b.Dx(), b.Dy()
			switch {
			case hasW && hasH:
				dw, dh = aw, ah
			case hasW:
				dw, dh = aw, aw*b.Dy()/b.Dx()
			case hasH:
				dw, dh = ah*b.Dx()/b.Dy(), ah
			}
			// Pixel lines carry no style on their cells: record the style
			// background in effect so padding and blanks fill with it,
			// never with the image's own edge colours.
			w.hasStyleBg, w.styleBg = st.HasBg, st.Bg
			if w.emitPixels(img, dw, dh) {
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

// emitPixels converts img to half-block cells sized like a browser would
// display it: displayW/displayH are the CSS-px display size, one cell is
// cellPxW x cellPxH px. Cols clamp to the content width and rows to
// maxImageRows, preserving aspect ratio when clamping.
func (w *walker) emitPixels(img image.Image, displayW, displayH int) bool {
	b := img.Bounds()
	if b.Dx() < 1 || b.Dy() < 1 || displayW < 1 || displayH < 1 {
		return false
	}
	cols := (displayW + cellPxW/2) / cellPxW
	rows := (displayH + cellPxH/2) / cellPxH
	if cols < 1 {
		cols = 1
	}
	if rows < 1 {
		rows = 1
	}
	// Clamp to the content width and the row cap in a single pass: apply
	// the most restrictive scale factor to both dimensions so the drawn
	// grid keeps the image's aspect ratio regardless of which limit binds.
	maxCols := w.width - w.indentCols()
	if maxCols < 1 {
		maxCols = 1
	}
	scale := 1.0
	if s := float64(maxCols) / float64(cols); s < scale {
		scale = s
	}
	if s := float64(maxImageRows) / float64(rows); s < scale {
		scale = s
	}
	if scale < 1 {
		cols = int(float64(cols)*scale + 0.5)
		rows = int(float64(rows)*scale + 0.5)
		if cols < 1 {
			cols = 1
		}
		if rows < 1 {
			rows = 1
		}
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

// putCell appends a prebuilt image pixel cell to the current line, keeping
// the link-range and line-start bookkeeping putRune provides. The line is
// marked as pixel-bearing so background fill never treats the pixels'
// incidental colours as a style background.
func (w *walker) putCell(c cell.Cell) {
	if !w.started {
		w.startLine()
	}
	w.linePixels = true
	if w.linkURL != "" && w.linkOpen < 0 {
		w.linkOpen = w.col
	}
	w.line = append(w.line, c)
	w.col++
}
