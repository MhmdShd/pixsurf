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
	"mime"
	"net/http"
	"net/http/cookiejar"
	"strings"
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
	maxAssetBytes = 512 << 10
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

// Response is a fetched page.
type Response struct {
	Body        string // decoded to UTF-8, capped at 5MB
	URL         string // final URL after redirects (base for relative resolution)
	ContentType string // media type only, lowercased, parameters stripped; "" if absent
	Truncated   bool   // body hit the 5MB cap
}

// Page fetches a page, decodes it to UTF-8, and caps it at 5MB.
func (c *Client) Page(rawURL string) (*Response, error) {
	resp, err := c.get(context.Background(), c.hc, rawURL)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	header := resp.Header.Get("Content-Type")
	utf8Reader, err := charset.NewReader(resp.Body, header)
	if err != nil {
		return nil, err
	}
	data, err := io.ReadAll(io.LimitReader(utf8Reader, maxPageBytes+1))
	if err != nil {
		return nil, err
	}
	out := &Response{URL: resp.Request.URL.String()}
	if len(data) > maxPageBytes {
		data = data[:maxPageBytes]
		out.Truncated = true
	}
	out.Body = string(data)
	if mt, _, err := mime.ParseMediaType(header); err == nil {
		out.ContentType = strings.ToLower(mt)
	}
	return out, nil
}

// Asset fetches a text asset such as a stylesheet, capped at 512KB with
// a 5s timeout. Same session (cookies) as pages. Bodies over the cap are
// truncated, not rejected: a partial stylesheet still yields usable rules.
func (c *Client) Asset(rawURL string) (string, error) {
	resp, err := c.get(context.Background(), c.imgHC, rawURL) // imgHC: the 5s-timeout client
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxAssetBytes))
	if err != nil {
		return "", err
	}
	return string(data), nil
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
