// Command pixsurf is a terminal web browser: pages render as colored pixels.
package main

import (
	"fmt"
	"image"
	"os"
	"time"

	"github.com/MhmdShd/pixsurf/browser"
	"github.com/MhmdShd/pixsurf/cell"
	"github.com/MhmdShd/pixsurf/render"
	"github.com/MhmdShd/pixsurf/ui"
)

// toCellGrid converts a render.Cell grid to a cell.Cell grid.
// TEMP shim, removed in v2 Task 7.
func toCellGrid(cells [][]render.Cell) [][]cell.Cell {
	out := make([][]cell.Cell, len(cells))
	for y, row := range cells {
		out[y] = make([]cell.Cell, len(row))
		for x, c := range row {
			out[y][x] = cell.Cell{
				Rune:  '▀',
				Fg:    cell.RGB{R: c.Top.R, G: c.Top.G, B: c.Top.B},
				Bg:    cell.RGB{R: c.Bottom.R, G: c.Bottom.G, B: c.Bottom.B},
				HasFg: true,
				HasBg: true,
			}
		}
	}
	return out
}

const pageWidth = 1280.0
const scrollStep = 60.0 // page pixels per arrow key
const debounce = 50 * time.Millisecond

func main() {
	startURL := "https://example.com"
	if len(os.Args) > 1 {
		startURL = os.Args[1]
	}

	b, err := browser.New()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer b.Close()

	u, err := ui.New()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer u.Close()

	app := &app{b: b, u: u}
	app.resize()
	app.do(func() error { return b.Open(startURL) })
	app.run()
}

type app struct {
	b       *browser.Browser
	u       *ui.UI
	cols    int
	rows    int
	scale   float64
	lastErr string

	lastCells [][]render.Cell

	pendingScroll float64
	scrollTimer   *time.Timer
}

// resize matches the Chrome viewport to the current terminal grid. It is a
// no-op if the terminal reports a degenerate size (e.g. mid-resize).
func (a *app) resize() {
	cols, rows := a.u.GridSize()
	if cols < 1 || rows < 1 {
		return
	}
	a.cols, a.rows = cols, rows
	a.scale = pageWidth / float64(a.cols)
	pageH := int(float64(a.rows*2) * a.scale)
	if err := a.b.SetViewport(int(pageWidth), pageH); err != nil {
		a.lastErr = err.Error()
	}
}

// do runs a browser action, records any error, and re-renders.
func (a *app) do(fn func() error) {
	a.lastErr = ""
	if err := fn(); err != nil {
		a.lastErr = err.Error()
	}
	a.refresh()
}

// refresh captures a screenshot and redraws the terminal. On screenshot
// failure it still redraws (using the last good frame, if any) so the error
// reaches the status bar instead of leaving a stale screen.
func (a *app) refresh() {
	if a.cols < 1 || a.rows < 1 {
		return
	}
	img, err := a.b.Screenshot()
	if err != nil {
		if a.lastErr == "" {
			a.lastErr = err.Error()
		}
		cells := a.lastCells
		if cells == nil {
			cells = render.ToCells(image.NewRGBA(image.Rect(0, 0, 1, 1)), a.cols, a.rows)
		}
		a.u.Draw(toCellGrid(cells), "error: "+a.lastErr)
		return
	}
	cells := render.ToCells(img, a.cols, a.rows)
	a.lastCells = cells
	status := a.normalStatus()
	if a.lastErr != "" {
		status = "error: " + a.lastErr
	}
	a.u.Draw(toCellGrid(cells), status)
}

// normalStatus builds the default (non-error) status bar text.
func (a *app) normalStatus() string {
	return a.b.CurrentURL() + "   [g:url b:back f:fwd r:reload q:quit]"
}

// queueScroll coalesces rapid scroll keys into one Chrome round-trip.
func (a *app) queueScroll(dy float64) {
	a.pendingScroll += dy
	if a.scrollTimer == nil {
		a.scrollTimer = time.NewTimer(debounce)
		return
	}
	if !a.scrollTimer.Stop() {
		select {
		case <-a.scrollTimer.C:
		default:
		}
	}
	a.scrollTimer.Reset(debounce)
}

func (a *app) run() {
	for {
		var timerC <-chan time.Time
		if a.scrollTimer != nil {
			timerC = a.scrollTimer.C
		}
		select {
		case ev := <-a.u.Events():
			if quit := a.handle(ev); quit {
				if a.scrollTimer != nil {
					a.scrollTimer.Stop()
				}
				return
			}
		case <-timerC:
			dy := a.pendingScroll
			a.pendingScroll = 0
			a.scrollTimer = nil
			a.do(func() error { return a.b.ScrollBy(dy) })
		}
	}
}

func (a *app) handle(ev ui.Event) (quit bool) {
	switch e := ev.(type) {
	case ui.ActionEvent:
		switch e.Kind {
		case ui.Quit:
			return true
		case ui.ScrollUp:
			a.queueScroll(-scrollStep)
		case ui.ScrollDown:
			a.queueScroll(scrollStep)
		case ui.PageUp:
			a.queueScroll(-float64(a.rows*2) * a.scale * 0.9)
		case ui.PageDown:
			a.queueScroll(float64(a.rows*2) * a.scale * 0.9)
		case ui.Back:
			a.do(a.b.Back)
		case ui.Forward:
			a.do(a.b.Forward)
		case ui.Reload:
			a.do(a.b.Reload)
		}
	case ui.ClickEvent:
		x, y := render.CellToPage(e.X, e.Y, a.scale)
		a.do(func() error { return a.b.ClickAt(x, y) })
	case ui.ResizeEvent:
		a.lastErr = ""
		cols, rows := a.u.GridSize()
		if a.lastCells != nil && cols == a.cols && rows == a.rows {
			// Grid size is unchanged (e.g. a URL-bar keystroke firing a
			// redraw request) — skip the SetViewport+Screenshot round-trip
			// and just repaint the last frame with the current status.
			a.u.Draw(toCellGrid(a.lastCells), a.normalStatus())
			return false
		}
		a.resize()
		a.refresh()
	case ui.URLEvent:
		a.do(func() error { return a.b.Open(e.URL) })
	}
	return false
}
