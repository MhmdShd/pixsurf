// Command pixsurf is a terminal web browser with its own pure-Go engine.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

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

	a := &app{u: u, client: fetch.New(), noImages: *noImages, arrivals: make(chan int, 64)}
	a.cols, a.rows = u.GridSize()
	a.navigate(startURL, true)
	a.run()
}

// promptPurpose identifies what an active status-bar prompt is for.
type promptPurpose int

const (
	purposeNone promptPurpose = iota
	purposeURL
	purposeField
)

type app struct {
	u        *ui.UI
	client   *fetch.Client
	noImages bool

	cols, rows int
	dom        *dom.Doc // parsed DOM of the current page (re-layout cache)
	doc        *layout.Document
	offset     int
	url        string
	lastErr    string

	history []string
	histPos int // index of current page in history

	purpose  promptPurpose
	fieldIdx int               // Fields index the active field prompt is for
	values   layout.FormValues // user-entered field values; cleared on load

	pendingFrag string // fragment to jump to after the next successful load

	// Async image pipeline (all fields touched only by the main loop;
	// workers communicate exclusively via cache/arrivals).
	cache     *imageCache
	imgCancel context.CancelFunc
	imgCtx    context.Context
	jobs      chan string
	arrivals  chan int // generation tags from workers
	gen       int      // page generation; bumped on every successful load
	dirty     bool     // an image arrived; re-layout pending
	debounce  *time.Timer
}

func (a *app) fetcher() layout.ImageFetcher {
	if a.noImages || a.cache == nil {
		return nil
	}
	return a.cache.fetcher()
}

// resetImages tears down the previous page's image pipeline and starts a
// fresh one: new cache, new context, new 4-worker pool, next generation.
func (a *app) resetImages() {
	if a.imgCancel != nil {
		a.imgCancel()
	}
	a.gen++
	a.dirty = false
	if a.noImages {
		return
	}
	a.cache = newImageCache()
	a.imgCtx, a.imgCancel = context.WithCancel(context.Background())
	a.jobs = make(chan string, imageJobsCap)
	for i := 0; i < imageWorkers; i++ {
		go imageWorker(a.imgCtx, a.client, a.cache, a.jobs, a.arrivals, a.gen)
	}
}

// spawnFetches hands the render pass's wanted URLs to the worker pool.
func (a *app) spawnFetches() {
	if a.noImages || a.cache == nil {
		return
	}
	for _, u := range a.cache.takeWanted() {
		select {
		case a.jobs <- u:
		default: // pool queue full (can't happen under the cap); placeholder stays
		}
	}
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
	a.dom = d
	a.values = nil // new page: previous pages' field values are stale
	a.resetImages()
	a.doc = layout.Render(d, a.cols, a.fetcher(), a.values)
	a.spawnFetches()
	a.url = finalURL
	if truncated {
		a.lastErr = "page truncated at 5MB"
	}
	return true
}

// navigate fetches and lays out rawURL, scrolls to the top, and draws.
// push records the page in history (truncating any forward entries).
func (a *app) navigate(rawURL string, push bool) {
	frag := a.pendingFrag
	a.pendingFrag = ""
	if !a.load(rawURL) {
		a.draw()
		return
	}
	a.offset = 0
	if frag != "" {
		if ln, ok := a.doc.Anchors[frag]; ok {
			a.offset = ln
			a.clampOffset()
		} else {
			a.lastErr = "anchor not found: #" + frag
		}
	}
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

// relayout re-flows the cached DOM at the current width — no network —
// keeping the scroll position proportionally.
func (a *app) relayout() {
	if a.dom == nil {
		a.draw()
		return
	}
	ratio := 0.0
	if a.doc != nil && len(a.doc.Lines) > 0 {
		ratio = float64(a.offset) / float64(len(a.doc.Lines))
	}
	a.doc = layout.Render(a.dom, a.cols, a.fetcher(), a.values)
	a.spawnFetches()
	a.offset = int(ratio * float64(len(a.doc.Lines)))
	a.clampOffset()
	a.draw()
}

// imageRelayout re-flows the cached DOM after image arrivals, keeping the
// exact scroll offset (clamped). Small jumps when images land above the
// viewport are accepted.
func (a *app) imageRelayout() {
	a.dirty = false
	if a.dom == nil {
		return
	}
	a.doc = layout.Render(a.dom, a.cols, a.fetcher(), a.values)
	a.spawnFetches()
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
	line := a.offset + y
	if i, ok := a.doc.FieldAt(line, x); ok {
		fl := a.doc.Fields[i]
		label := fl.Name
		if label == "" {
			label = "text"
		}
		a.purpose = purposeField
		a.fieldIdx = i
		a.u.Prompt(label+": ", fl.Value)
		return
	}
	if i, ok := a.doc.SubmitAt(line, x); ok {
		a.submit(i)
		return
	}
	href, ok := a.doc.LinkAt(line, x)
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
		// Cross-page anchor: navigate to the base URL, then jump to the
		// fragment's line once the page has loaded (see navigate).
		if i := strings.Index(href, "#"); i >= 0 {
			a.pendingFrag = href[i+1:]
			href = href[:i]
		}
		a.navigate(href, true)
	}
}

// submit builds the GET URL for form formIdx from the current field values
// and navigates to it. Errors (POST, bad action) land in the status bar.
func (a *app) submit(formIdx int) {
	form := a.doc.Forms[formIdx]
	var fields []layout.Field
	for _, fl := range a.doc.Fields {
		if fl.FormIdx == formIdx {
			fields = append(fields, fl)
		}
	}
	target, err := submitURL(form, fields, a.values)
	if err != nil {
		a.lastErr = err.Error()
		a.draw()
		return
	}
	a.navigate(target, true)
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

// run is the main loop: a select over ui events, image arrivals, and the
// redraw debounce timer. All drawing happens here.
func (a *app) run() {
	const debounceDelay = 200 * time.Millisecond
	for {
		var timerC <-chan time.Time
		if a.debounce != nil {
			timerC = a.debounce.C
		}
		select {
		case ev, ok := <-a.u.Events():
			if !ok || a.handleEvent(ev) {
				if a.imgCancel != nil {
					a.imgCancel()
				}
				return
			}
			// A prompt may have just closed with a re-layout still pending.
			if a.dirty && a.purpose == purposeNone && a.debounce == nil {
				a.imageRelayout()
			}
		case g := <-a.arrivals:
			if !keepArrival(g, a.gen) {
				continue // stale page generation
			}
			a.dirty = true
			if a.debounce == nil {
				a.debounce = time.NewTimer(debounceDelay)
			} else {
				if !a.debounce.Stop() {
					select {
					case <-a.debounce.C:
					default:
					}
				}
				a.debounce.Reset(debounceDelay)
			}
		case <-timerC:
			a.debounce = nil
			if a.dirty && a.purpose == purposeNone {
				a.imageRelayout()
			}
			// Prompt active: hold dirty; applied when the prompt closes.
		}
	}
}

// handleEvent processes one ui event; it reports whether to quit.
func (a *app) handleEvent(ev ui.Event) bool {
	switch e := ev.(type) {
	case ui.ActionEvent:
		switch e.Kind {
		case ui.Quit:
			return true
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
				break
			}
			a.navigate(a.url, false)
		case ui.URLBar:
			a.purpose = purposeURL
			a.u.Prompt("URL: ", "")
		}
	case ui.ClickEvent:
		a.click(e.X, e.Y)
	case ui.ResizeEvent:
		cols, rows := a.u.GridSize()
		if cols == a.cols && rows == a.rows {
			a.draw() // redraw request (prompt typing)
			break
		}
		if cols < 1 || rows < 1 {
			break
		}
		a.cols, a.rows = cols, rows
		a.relayout()
	case ui.InputEvent:
		purpose := a.purpose
		a.purpose = purposeNone
		switch purpose {
		case purposeURL:
			if e.Text == "" {
				a.draw()
				break
			}
			a.navigate(e.Text, true)
		case purposeField:
			if a.doc == nil || a.fieldIdx >= len(a.doc.Fields) {
				a.draw()
				break
			}
			fl := a.doc.Fields[a.fieldIdx]
			if fl.Name == "" {
				a.lastErr = "field has no name; not submitted"
				a.draw()
				break
			}
			if a.values == nil {
				a.values = layout.FormValues{}
			}
			form := a.doc.Forms[fl.FormIdx]
			a.values[layout.ValuesKey(form.Action, fl.Name)] = e.Text
			a.relayout() // show the typed value in its box
			a.submit(fl.FormIdx)
		default:
			a.draw()
		}
	case ui.InputCancelEvent:
		a.purpose = purposeNone
		a.draw()
	}
	return false
}
