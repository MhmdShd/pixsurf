package fetch

import (
	"bytes"
	"fmt"
	"image"
	"image/png"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestUserAgentAndRedirectAndFinalURL(t *testing.T) {
	var gotUA string
	mux := http.NewServeMux()
	mux.HandleFunc("/start", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/end", http.StatusFound)
	})
	mux.HandleFunc("/end", func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		fmt.Fprint(w, "<html><body>hi</body></html>")
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	c := New()
	body, finalURL, truncated, err := c.Page(srv.URL + "/start")
	if err != nil {
		t.Fatal(err)
	}
	if truncated {
		t.Error("unexpected truncation")
	}
	if !strings.Contains(body, "hi") {
		t.Errorf("body = %q", body)
	}
	if finalURL != srv.URL+"/end" {
		t.Errorf("finalURL = %q, want %q", finalURL, srv.URL+"/end")
	}
	if !strings.HasPrefix(gotUA, "pixsurf/") {
		t.Errorf("User-Agent = %q", gotUA)
	}
}

func TestPageCap(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(make([]byte, 6<<20)) // 6MB of zero bytes
	}))
	defer srv.Close()
	c := New()
	body, _, truncated, err := c.Page(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if !truncated {
		t.Error("want truncated=true for 6MB body")
	}
	if len(body) > maxPageBytes {
		t.Errorf("body len %d exceeds cap %d", len(body), maxPageBytes)
	}
}

func TestCharsetDecoding(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=iso-8859-1")
		w.Write([]byte{'c', 'a', 'f', 0xE9}) // "café" in latin-1
	}))
	defer srv.Close()
	c := New()
	body, _, _, err := c.Page(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(body, "café") {
		t.Errorf("body = %q, want café decoded", body)
	}
}

func TestCookiesPersist(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/set", func(w http.ResponseWriter, r *http.Request) {
		http.SetCookie(w, &http.Cookie{Name: "sid", Value: "42"})
		fmt.Fprint(w, "ok")
	})
	mux.HandleFunc("/check", func(w http.ResponseWriter, r *http.Request) {
		if c, err := r.Cookie("sid"); err == nil && c.Value == "42" {
			fmt.Fprint(w, "have-cookie")
		} else {
			fmt.Fprint(w, "no-cookie")
		}
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	c := New()
	if _, _, _, err := c.Page(srv.URL + "/set"); err != nil {
		t.Fatal(err)
	}
	body, _, _, err := c.Page(srv.URL + "/check")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(body, "have-cookie") {
		t.Errorf("cookie not persisted, body = %q", body)
	}
}

func TestImageCap(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write(make([]byte, 3<<20)) // 3MB, over the 2MB image cap
	}))
	defer srv.Close()
	c := New()
	if _, err := c.Image(srv.URL); err == nil {
		t.Error("want error for 3MB image, got nil")
	}
}

func TestImageDecodeSuccess(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	pngBytes := buf.Bytes()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		w.Write(pngBytes)
	}))
	defer srv.Close()

	c := New()
	got, err := c.Image(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil {
		t.Fatal("want non-nil image")
	}
	b := got.Bounds()
	if b.Dx() != 4 || b.Dy() != 4 {
		t.Errorf("bounds = %v, want 4x4", b)
	}
}

func TestPageCapBoundary(t *testing.T) {
	t.Run("exactly at cap", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write(make([]byte, maxPageBytes))
		}))
		defer srv.Close()
		c := New()
		body, _, truncated, err := c.Page(srv.URL)
		if err != nil {
			t.Fatal(err)
		}
		if truncated {
			t.Error("want truncated=false at exactly maxPageBytes")
		}
		if len(body) != maxPageBytes {
			t.Errorf("body len = %d, want %d", len(body), maxPageBytes)
		}
	})

	t.Run("one byte over cap", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Write(make([]byte, maxPageBytes+1))
		}))
		defer srv.Close()
		c := New()
		body, _, truncated, err := c.Page(srv.URL)
		if err != nil {
			t.Fatal(err)
		}
		if !truncated {
			t.Error("want truncated=true for maxPageBytes+1")
		}
		if len(body) != maxPageBytes {
			t.Errorf("body len = %d, want %d", len(body), maxPageBytes)
		}
	})
}

func TestHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer srv.Close()
	c := New()
	_, _, _, err := c.Page(srv.URL)
	if err == nil {
		t.Fatal("want error for 404 response, got nil")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Errorf("err = %q, want it to contain \"404\"", err.Error())
	}
}
