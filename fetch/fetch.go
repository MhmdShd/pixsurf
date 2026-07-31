// Package fetch retrieves pages and images over HTTP with safety caps.
package fetch

import (
	"bytes"
	"context"
	"fmt"
	"image"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"io"
	"net/http"
	"net/http/cookiejar"
	"time"

	_ "golang.org/x/image/webp"
	"golang.org/x/net/html/charset"
)

const (
	userAgent     = "pixsurf/0.2 (+https://github.com/MhmdShd/pixsurf)"
	pageTimeout   = 15 * time.Second
	imageTimeout  = 5 * time.Second
	maxPageBytes  = 5 << 20
	maxImageBytes = 2 << 20
)

// Client is an HTTP session with an in-memory cookie jar.
type Client struct {
	hc    *http.Client
	imgHC *http.Client
}

// New creates a Client. Cookies live for the process lifetime only.
func New() *Client {
	jar, _ := cookiejar.New(nil)
	return &Client{
		hc:    &http.Client{Jar: jar, Timeout: pageTimeout},
		imgHC: &http.Client{Jar: jar, Timeout: imageTimeout},
	}
}

// get performs the request. No host allow-list by design: this is
// user-driven browser semantics, so localhost/internal URLs are fetched
// like any other host (accepted risk).
func (c *Client) get(ctx context.Context, hc *http.Client, rawURL string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", rawURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", userAgent)
	resp, err := hc.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		resp.Body.Close()
		return nil, fmt.Errorf("HTTP %s", resp.Status)
	}
	return resp, nil
}

// Page fetches a page, decodes it to UTF-8, and caps it at 5MB.
// finalURL is the URL after redirects (base for relative resolution).
func (c *Client) Page(rawURL string) (body, finalURL string, truncated bool, err error) {
	resp, err := c.get(context.Background(), c.hc, rawURL)
	if err != nil {
		return "", "", false, err
	}
	defer resp.Body.Close()

	utf8Reader, err := charset.NewReader(resp.Body, resp.Header.Get("Content-Type"))
	if err != nil {
		return "", "", false, err
	}
	data, err := io.ReadAll(io.LimitReader(utf8Reader, maxPageBytes+1))
	if err != nil {
		return "", "", false, err
	}
	if len(data) > maxPageBytes {
		data = data[:maxPageBytes]
		truncated = true
	}
	return string(data), resp.Request.URL.String(), truncated, nil
}

// Image fetches and decodes an image (png/jpeg/gif/webp), capped at 2MB.
// Canceling ctx aborts an in-flight request, so callers can abandon images
// for a page the user has navigated away from.
func (c *Client) Image(ctx context.Context, rawURL string) (image.Image, error) {
	resp, err := c.get(ctx, c.imgHC, rawURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxImageBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxImageBytes {
		return nil, fmt.Errorf("image exceeds %d bytes", maxImageBytes)
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	return img, err
}
