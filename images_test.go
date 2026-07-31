package main

import (
	"fmt"
	"image"
	"testing"

	"github.com/MhmdShd/pixsurf/dom"
	"github.com/MhmdShd/pixsurf/layout"
)

// pixelLines counts document lines containing half-block image cells.
func pixelLines(doc *layout.Document) int {
	n := 0
	for _, line := range doc.Lines {
		for _, c := range line {
			if c.Rune == '▀' {
				n++
				break
			}
		}
	}
	return n
}

func TestImageCacheTwoPassRender(t *testing.T) {
	d, err := dom.Parse(
		`<html><body><p>hi</p><img src="/pic.png" alt="pic"></body></html>`,
		"https://x.example/page")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	c := newImageCache()

	doc := layout.Render(d, 40, c.fetcher(), nil, nil)
	if got := pixelLines(doc); got != 0 {
		t.Errorf("first pass: %d pixel lines, want 0 (placeholder)", got)
	}

	wanted := c.takeWanted()
	if len(wanted) != 1 || wanted[0] != "https://x.example/pic.png" {
		t.Fatalf("wanted = %v, want [https://x.example/pic.png]", wanted)
	}
	if !c.store(wanted[0], image.NewRGBA(image.Rect(0, 0, 16, 16))) {
		t.Fatal("store rejected under cap")
	}

	doc = layout.Render(d, 40, c.fetcher(), nil, nil)
	if got := pixelLines(doc); got == 0 {
		t.Error("second pass: 0 pixel lines, want >0 (cache hit)")
	}
	if left := c.takeWanted(); len(left) != 0 {
		t.Errorf("cache hit re-recorded wanted: %v", left)
	}
}

func TestImageCacheCap(t *testing.T) {
	c := newImageCache()
	f := c.fetcher()
	for i := 0; i < imageCacheCap+10; i++ {
		if _, err := f(fmt.Sprintf("https://x.example/%d.png", i)); err == nil {
			t.Fatal("miss returned nil error")
		}
	}
	if got := len(c.takeWanted()); got != imageCacheCap {
		t.Errorf("wanted after cap overflow = %d, want %d", got, imageCacheCap)
	}
	if _, err := f("https://x.example/extra.png"); err == nil {
		t.Fatal("miss returned nil error")
	}
	if got := len(c.takeWanted()); got != 0 {
		t.Errorf("over-cap miss recorded %d wanted, want 0", got)
	}
}

func TestKeepArrival(t *testing.T) {
	if !keepArrival(3, 3) {
		t.Error("current-generation arrival dropped")
	}
	if keepArrival(2, 3) {
		t.Error("stale arrival kept")
	}
}
