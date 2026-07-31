// Command pixsurf is a terminal web browser: pages render as colored pixels.
package main

import (
	"fmt"
	"os"
	"time"

	"github.com/MhmdShd/pixsurf/browser"
	"github.com/MhmdShd/pixsurf/render"
	"github.com/MhmdShd/pixsurf/ui"
)

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

	pendingScroll float64
	scrollTimer   *time.Timer
}

// resize matches the Chrome viewport to the current terminal grid.
func (a *app) resize() {
	a.cols, a.rows = a.u.GridSize()
	if a.cols < 1 || a.rows < 1 {
		return
	}
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

// refresh captures a screenshot and redraws the terminal.
func (a *app) refresh() {
	img, err := a.b.Screenshot()
	if err != nil {
		a.lastErr = err.Error()
		return
	}
	cells := render.ToCells(img, a.cols, a.rows)
	status := a.b.CurrentURL() + "   [g:url b:back f:fwd r:reload q:quit]"
	if a.lastErr != "" {
		status = "error: " + a.lastErr
	}
	a.u.Draw(cells, status)
}

// queueScroll coalesces rapid scroll keys into one Chrome round-trip.
func (a *app) queueScroll(dy float64) {
	a.pendingScroll += dy
	if a.scrollTimer == nil {
		a.scrollTimer = time.NewTimer(debounce)
	} else {
		a.scrollTimer.Reset(debounce)
	}
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
		a.resize()
		a.refresh()
	case ui.URLEvent:
		a.do(func() error { return a.b.Open(e.URL) })
	}
	return false
}
