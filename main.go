// Command pixsurf is a terminal web browser with its own pure-Go engine.
package main

import (
	"flag"
	"fmt"
	"image"
	"os"
	"strings"

	"github.com/MhmdShd/pixsurf/cell"
	"github.com/MhmdShd/pixsurf/dom"
	"github.com/MhmdShd/pixsurf/fetch"
	"github.com/MhmdShd/pixsurf/layout"
	"github.com/MhmdShd/pixsurf/ui"
)

func main() {
	noImages := flag.Bool("no-images", false, "skip fetching and rendering images")
	flag.Parse()

	startURL := "https://example.com"
	if flag.NArg() > 0 {
		startURL = flag.Arg(0)
	}

	u, err := ui.New()
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer u.Close()

	a := &app{u: u, client: fetch.New(), noImages: *noImages}
	a.cols, a.rows = u.GridSize()
	a.navigate(startURL, true)
	a.run()
}

type app struct {
	u        *ui.UI
	client   *fetch.Client
	noImages bool

	cols, rows int
	doc        *layout.Document
	offset     int
	url        string
	lastErr    string

	history []string
	histPos int // index of current page in history
}

func (a *app) fetcher() layout.ImageFetcher {
	if a.noImages {
		return nil
	}
	return func(u string) (image.Image, error) { return a.client.Image(u) }
}

// load fetches and lays out rawURL, replacing the current document. It does
// not draw or touch the scroll offset; it reports whether the load succeeded.
func (a *app) load(rawURL string) bool {
	a.lastErr = ""
	target := rawURL
	if !strings.Contains(target, "://") {
		target = "https://" + strings.TrimSpace(target)
	}

	body, finalURL, truncated, err := a.client.Page(target)
	if err != nil {
		a.lastErr = err.Error()
		return false
	}
	d, err := dom.Parse(body, finalURL)
	if err != nil {
		a.lastErr = err.Error()
		return false
	}
	a.doc = layout.Render(d, a.cols, a.fetcher())
	a.url = finalURL
	if truncated {
		a.lastErr = "page truncated at 5MB"
	}
	return true
}

// navigate fetches and lays out rawURL, scrolls to the top, and draws.
// push records the page in history (truncating any forward entries).
func (a *app) navigate(rawURL string, push bool) {
	if !a.load(rawURL) {
		a.draw()
		return
	}
	a.offset = 0
	if push {
		if len(a.history) == 0 {
			a.history = []string{a.url}
		} else {
			a.history = append(a.history[:a.histPos+1], a.url)
		}
		a.histPos = len(a.history) - 1
	}
	a.draw()
}

// relayout re-flows the current document at the current width, keeping the
// scroll position proportionally. Reflow requires the parsed DOM; simplest
// correct path is a re-fetch of the current URL — cheap pages, local net
// cost accepted for v2.
func (a *app) relayout() {
	if a.doc == nil {
		return
	}
	ratio := 0.0
	if len(a.doc.Lines) > 0 {
		ratio = float64(a.offset) / float64(len(a.doc.Lines))
	}
	if a.load(a.url) {
		a.offset = int(ratio * float64(len(a.doc.Lines)))
	}
	a.clampOffset()
	a.draw()
}

func (a *app) clampOffset() {
	max := 0
	if a.doc != nil {
		max = len(a.doc.Lines) - a.rows
	}
	if max < 0 {
		max = 0
	}
	if a.offset > max {
		a.offset = max
	}
	if a.offset < 0 {
		a.offset = 0
	}
}

// view returns the visible slice of the document, padded to rows x cols.
func (a *app) view() [][]cell.Cell {
	out := make([][]cell.Cell, a.rows)
	for i := 0; i < a.rows; i++ {
		out[i] = make([]cell.Cell, a.cols)
		if a.doc != nil && a.offset+i < len(a.doc.Lines) {
			copy(out[i], a.doc.Lines[a.offset+i])
		}
	}
	return out
}

func (a *app) draw() {
	status := a.url + "   [g:url b:back f:fwd r:reload q:quit]"
	if a.lastErr != "" {
		status = "error: " + a.lastErr
	}
	a.u.Draw(a.view(), status)
}

func (a *app) scroll(dy int) {
	a.offset += dy
	a.clampOffset()
	a.draw()
}

func (a *app) click(x, y int) {
	if a.doc == nil {
		return
	}
	href, ok := a.doc.LinkAt(a.offset+y, x)
	if !ok {
		return
	}
	switch {
	case strings.HasPrefix(href, "mailto:"), strings.HasPrefix(href, "javascript:"):
		a.lastErr = "unsupported link: " + href
		a.draw()
	case strings.Contains(href, "#") && strings.Split(href, "#")[0] == strings.Split(a.url, "#")[0]:
		frag := strings.SplitN(href, "#", 2)[1]
		if frag == "" {
			a.offset = 0 // bare "#": scroll to top
		} else if ln, ok := a.doc.Anchors[frag]; ok {
			a.offset = ln
			a.clampOffset()
		} else {
			a.lastErr = "anchor not found: #" + frag
		}
		a.draw()
	default:
		a.navigate(href, true)
	}
}

func (a *app) back() {
	if a.histPos > 0 {
		a.histPos--
		a.navigate(a.history[a.histPos], false)
	}
}

func (a *app) forward() {
	if a.histPos < len(a.history)-1 {
		a.histPos++
		a.navigate(a.history[a.histPos], false)
	}
}

func (a *app) run() {
	for ev := range a.u.Events() {
		switch e := ev.(type) {
		case ui.ActionEvent:
			switch e.Kind {
			case ui.Quit:
				return
			case ui.ScrollUp:
				a.scroll(-1)
			case ui.ScrollDown:
				a.scroll(1)
			case ui.PageUp:
				a.scroll(-(a.rows - 1))
			case ui.PageDown:
				a.scroll(a.rows - 1)
			case ui.Back:
				a.back()
			case ui.Forward:
				a.forward()
			case ui.Reload:
				if a.url == "" {
					a.lastErr = "nothing to reload"
					a.draw()
					continue
				}
				a.navigate(a.url, false)
			}
		case ui.ClickEvent:
			a.click(e.X, e.Y)
		case ui.ResizeEvent:
			cols, rows := a.u.GridSize()
			if cols == a.cols && rows == a.rows {
				a.draw() // redraw request (URL bar typing)
				continue
			}
			if cols < 1 || rows < 1 {
				continue
			}
			a.cols, a.rows = cols, rows
			a.relayout()
		case ui.URLEvent:
			a.navigate(e.URL, true)
		}
	}
}
