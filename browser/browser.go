// Package browser drives a headless Chrome/Chromium instance.
package browser

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/png"
	"os"
	"strings"
	"time"

	"github.com/chromedp/chromedp"
)

const navTimeout = 15 * time.Second

// settle gives pages a moment to paint after the load event.
const settle = 500 * time.Millisecond

// Browser wraps a headless Chrome session.
type Browser struct {
	ctx     context.Context
	cancels []context.CancelFunc
}

// New launches headless Chrome. Running as root adds --no-sandbox, which
// Chrome requires in that case.
func New() (*Browser, error) {
	opts := append([]chromedp.ExecAllocatorOption{}, chromedp.DefaultExecAllocatorOptions[:]...)
	if os.Geteuid() == 0 {
		opts = append(opts, chromedp.NoSandbox)
	}
	allocCtx, cancelAlloc := chromedp.NewExecAllocator(context.Background(), opts...)
	ctx, cancelCtx := chromedp.NewContext(allocCtx)
	if err := chromedp.Run(ctx); err != nil {
		cancelCtx()
		cancelAlloc()
		return nil, fmt.Errorf("could not start Chrome — is Chrome or Chromium installed? (%w)", err)
	}
	return &Browser{ctx: ctx, cancels: []context.CancelFunc{cancelCtx, cancelAlloc}}, nil
}

// NormalizeURL prefixes https:// when no scheme is present.
func NormalizeURL(s string) string {
	s = strings.TrimSpace(s)
	if s == "" || strings.Contains(s, "://") {
		return s
	}
	return "https://" + s
}

// SetViewport sets the page viewport in page pixels.
func (b *Browser) SetViewport(w, h int) error {
	return chromedp.Run(b.ctx, chromedp.EmulateViewport(int64(w), int64(h)))
}

// Open navigates to rawURL and waits for the page to be ready.
func (b *Browser) Open(rawURL string) error {
	ctx, cancel := context.WithTimeout(b.ctx, navTimeout)
	defer cancel()
	return chromedp.Run(ctx,
		chromedp.Navigate(NormalizeURL(rawURL)),
		chromedp.Sleep(settle),
	)
}

// Screenshot captures the current viewport.
func (b *Browser) Screenshot() (image.Image, error) {
	var buf []byte
	if err := chromedp.Run(b.ctx, chromedp.CaptureScreenshot(&buf)); err != nil {
		return nil, err
	}
	return png.Decode(bytes.NewReader(buf))
}

// ScrollBy scrolls the page vertically by dy page pixels.
func (b *Browser) ScrollBy(dy float64) error {
	js := fmt.Sprintf("window.scrollBy(0, %f)", dy)
	return chromedp.Run(b.ctx, chromedp.Evaluate(js, nil))
}

// ClickAt clicks at page coordinates and waits briefly for any navigation.
func (b *Browser) ClickAt(x, y float64) error {
	ctx, cancel := context.WithTimeout(b.ctx, navTimeout)
	defer cancel()
	return chromedp.Run(ctx,
		chromedp.MouseClickXY(x, y),
		chromedp.Sleep(settle),
	)
}

// Back navigates one entry back in history.
func (b *Browser) Back() error {
	ctx, cancel := context.WithTimeout(b.ctx, navTimeout)
	defer cancel()
	return chromedp.Run(ctx, chromedp.NavigateBack(), chromedp.Sleep(settle))
}

// Forward navigates one entry forward in history.
func (b *Browser) Forward() error {
	ctx, cancel := context.WithTimeout(b.ctx, navTimeout)
	defer cancel()
	return chromedp.Run(ctx, chromedp.NavigateForward(), chromedp.Sleep(settle))
}

// Reload reloads the current page.
func (b *Browser) Reload() error {
	ctx, cancel := context.WithTimeout(b.ctx, navTimeout)
	defer cancel()
	return chromedp.Run(ctx, chromedp.Reload(), chromedp.Sleep(settle))
}

// CurrentURL returns the page's current location.
func (b *Browser) CurrentURL() string {
	var u string
	if err := chromedp.Run(b.ctx, chromedp.Location(&u)); err != nil {
		return ""
	}
	return u
}

// Close shuts the browser down.
func (b *Browser) Close() {
	for i := len(b.cancels) - 1; i >= 0; i-- {
		b.cancels[i]()
	}
}
