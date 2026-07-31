package main

import (
	"context"
	"errors"
	"image"
	"sync"

	"github.com/MhmdShd/pixsurf/fetch"
	"github.com/MhmdShd/pixsurf/layout"
)

const (
	imageCacheCap = 50 // max images tracked per page; beyond → placeholders
	imageWorkers  = 4  // concurrent fetches per page
	imageJobsCap  = 64 // ≥ imageCacheCap so enqueues never drop
)

// errImagePending is returned by the cache fetcher on a miss so layout
// draws the [alt] placeholder; the URL is queued for async fetching.
var errImagePending = errors.New("image not fetched yet")

// imageCache is the per-page image store shared between the render pass
// (main goroutine) and the fetch workers. All maps are mutex-guarded.
type imageCache struct {
	mu        sync.Mutex
	images    map[string]image.Image
	wanted    map[string]bool // render misses awaiting dispatch
	requested map[string]bool // dispatched; never re-recorded as wanted
}

func newImageCache() *imageCache {
	return &imageCache{
		images:    map[string]image.Image{},
		wanted:    map[string]bool{},
		requested: map[string]bool{},
	}
}

// fetcher returns the layout.ImageFetcher backing a render pass: cache hit
// → image; miss → the URL is recorded as wanted (subject to the per-page
// cap) and errImagePending makes layout draw the placeholder. Never blocks
// on the network, so first paint is instant.
func (c *imageCache) fetcher() layout.ImageFetcher {
	return func(u string) (image.Image, error) {
		c.mu.Lock()
		defer c.mu.Unlock()
		if img, ok := c.images[u]; ok {
			return img, nil
		}
		if !c.requested[u] && len(c.images)+len(c.requested)+len(c.wanted) < imageCacheCap {
			c.wanted[u] = true
		}
		return nil, errImagePending
	}
}

// takeWanted drains the wanted set, marking every URL as requested so a
// later render pass cannot queue it again.
func (c *imageCache) takeWanted() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	urls := make([]string, 0, len(c.wanted))
	for u := range c.wanted {
		urls = append(urls, u)
		c.requested[u] = true
	}
	c.wanted = map[string]bool{}
	return urls
}

// store saves a fetched image, reporting whether it was kept (cap guard).
func (c *imageCache) store(u string, img image.Image) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.images[u]; !ok && len(c.images) >= imageCacheCap {
		return false
	}
	c.images[u] = img
	return true
}

// keepArrival reports whether an arrival tagged arrivalGen belongs to the
// current page generation (stale arrivals are dropped).
func keepArrival(arrivalGen, currentGen int) bool {
	return arrivalGen == currentGen
}

// imageWorker fetches queued URLs until the page context is canceled or
// the jobs channel closes. Successes go into the cache and are announced
// on arrivals tagged with the page generation. It never draws; failures
// are silent (the placeholder stays).
func imageWorker(ctx context.Context, client *fetch.Client, cache *imageCache,
	jobs <-chan string, arrivals chan<- int, gen int) {
	for {
		select {
		case <-ctx.Done():
			return
		case u, ok := <-jobs:
			if !ok {
				return
			}
			img, err := client.Image(u)
			if err != nil || img == nil {
				continue
			}
			if !cache.store(u, img) {
				continue
			}
			select {
			case arrivals <- gen:
			case <-ctx.Done():
				return
			}
		}
	}
}
