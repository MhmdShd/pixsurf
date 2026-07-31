package render

import (
	"image"
	"image/color"
	"testing"
)

func solid(w, h int, c color.RGBA) image.Image {
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, c)
		}
	}
	return img
}

func TestToCellsSolidColor(t *testing.T) {
	img := solid(100, 60, color.RGBA{R: 200, G: 10, B: 30, A: 255})
	grid := ToCells(img, 10, 5)
	if len(grid) != 5 || len(grid[0]) != 10 {
		t.Fatalf("grid = %dx%d, want 5x10", len(grid), len(grid[0]))
	}
	want := RGB{R: 200, G: 10, B: 30}
	for y := range grid {
		for x := range grid[y] {
			c := grid[y][x]
			if c.Top != want || c.Bottom != want {
				t.Fatalf("cell %d,%d = %+v, want top/bottom %+v", x, y, c, want)
			}
		}
	}
}

func TestToCellsTwoTone(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 10, 10))
	red := color.RGBA{R: 255, A: 255}
	blue := color.RGBA{B: 255, A: 255}
	for y := 0; y < 10; y++ {
		for x := 0; x < 10; x++ {
			if y < 5 {
				img.Set(x, y, red)
			} else {
				img.Set(x, y, blue)
			}
		}
	}
	grid := ToCells(img, 1, 1)
	c := grid[0][0]
	if (c.Top != RGB{R: 255}) {
		t.Errorf("Top = %+v, want pure red", c.Top)
	}
	if (c.Bottom != RGB{B: 255}) {
		t.Errorf("Bottom = %+v, want pure blue", c.Bottom)
	}
}

func TestToCellsTinyImage(t *testing.T) {
	img := solid(2, 2, color.RGBA{R: 9, G: 9, B: 9, A: 255})
	grid := ToCells(img, 20, 10) // grid larger than image
	if len(grid) != 10 || len(grid[0]) != 20 {
		t.Fatalf("grid = %dx%d, want 10x20", len(grid), len(grid[0]))
	}
}

func TestCellToPage(t *testing.T) {
	// scale 16: cell (0,0) center -> page (8, 16)
	px, py := CellToPage(0, 0, 16)
	if px != 8 || py != 16 {
		t.Errorf("CellToPage(0,0,16) = %v,%v, want 8,16", px, py)
	}
	// cell (4,2): x = 4.5*16 = 72, y = (2*2+1)*16 = 80
	px, py = CellToPage(4, 2, 16)
	if px != 72 || py != 80 {
		t.Errorf("CellToPage(4,2,16) = %v,%v, want 72,80", px, py)
	}
}
