// Package render converts page screenshots into terminal half-block cells.
package render

import "image"

// RGB is an 8-bit color.
type RGB struct{ R, G, B uint8 }

// Cell is one terminal character cell: a '▀' drawn with Top as the
// foreground color and Bottom as the background color.
type Cell struct{ Top, Bottom RGB }

// ToCells downscales img to cols x (rows*2) pixels using area averaging and
// packs vertical pixel pairs into cells. The result is a rows x cols grid.
func ToCells(img image.Image, cols, rows int) [][]Cell {
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	grid := make([][]Cell, rows)
	for cy := 0; cy < rows; cy++ {
		grid[cy] = make([]Cell, cols)
		for cx := 0; cx < cols; cx++ {
			grid[cy][cx] = Cell{
				Top:    areaAvg(img, b, cx, cy*2, cols, rows*2, w, h),
				Bottom: areaAvg(img, b, cx, cy*2+1, cols, rows*2, w, h),
			}
		}
	}
	return grid
}

// areaAvg averages the source pixels that target pixel (tx, ty) covers when
// the source (w x h) is scaled to tw x th.
func areaAvg(img image.Image, b image.Rectangle, tx, ty, tw, th, w, h int) RGB {
	x0, x1 := tx*w/tw, (tx+1)*w/tw
	y0, y1 := ty*h/th, (ty+1)*h/th
	if x1 <= x0 {
		x1 = x0 + 1
	}
	if y1 <= y0 {
		y1 = y0 + 1
	}
	var r, g, bl, n uint64
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			pr, pg, pb, _ := img.At(b.Min.X+x, b.Min.Y+y).RGBA()
			r += uint64(pr >> 8)
			g += uint64(pg >> 8)
			bl += uint64(pb >> 8)
			n++
		}
	}
	return RGB{uint8(r / n), uint8(g / n), uint8(bl / n)}
}

// CellToPage maps a terminal cell to page pixel coordinates at the cell's
// center. scale is page pixels per terminal cell width (pageWidth/gridCols).
func CellToPage(cellX, cellY int, scale float64) (float64, float64) {
	px := (float64(cellX) + 0.5) * scale
	py := (float64(cellY)*2 + 1) * scale
	return px, py
}
