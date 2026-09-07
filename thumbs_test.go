package main

import (
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestSnapWidth(t *testing.T) {
	cases := map[int]int{
		1:    192,
		192:  192,
		193:  288,
		600:  768,
		1920: 1920,
		9999: 3072, // anything past the top bucket clamps rather than allocating
	}
	for want, expect := range cases {
		if got := snapWidth(want); got != expect {
			t.Errorf("snapWidth(%d) = %d, want %d", want, got, expect)
		}
	}
}

// A flat image must survive resampling unchanged: any weighting bug shows up as
// a shifted or darkened result.
func TestBoxResizePreservesFlatColour(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 600, 900))
	fill := color.RGBA{200, 100, 50, 255}
	for i := 0; i < len(src.Pix); i += 4 {
		src.Pix[i], src.Pix[i+1], src.Pix[i+2], src.Pix[i+3] = fill.R, fill.G, fill.B, fill.A
	}

	out := boxResize(src, 192, 288)
	if out.Bounds().Dx() != 192 || out.Bounds().Dy() != 288 {
		t.Fatalf("got %v, want 192x288", out.Bounds())
	}
	for i := 0; i < len(out.Pix); i += 4 {
		if out.Pix[i] != fill.R || out.Pix[i+1] != fill.G || out.Pix[i+2] != fill.B || out.Pix[i+3] != fill.A {
			t.Fatalf("pixel %d = %v, want %v", i/4, out.Pix[i:i+4], fill)
		}
	}
}

// Halving a checkerboard averages each 2x2 block to mid grey. This is the case
// a nearest-neighbour shortcut would fail, so it pins the filter as an average.
func TestBoxResizeAverages(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for y := 0; y < 4; y++ {
		for x := 0; x < 4; x++ {
			v := uint8(0)
			if (x+y)%2 == 0 {
				v = 255
			}
			src.SetRGBA(x, y, color.RGBA{v, v, v, 255})
		}
	}

	out := boxResize(src, 2, 2)
	for i := 0; i < len(out.Pix); i += 4 {
		if out.Pix[i] != 127 {
			t.Errorf("pixel %d = %d, want 127 (the 0/255 average)", i/4, out.Pix[i])
		}
	}
}

// A source narrower than the requested width is served as-is: re-encoding it
// would cost work and lose quality for no benefit.
func TestThumbnailSkipsUpscale(t *testing.T) {
	dir := t.TempDir()
	src := filepath.Join(dir, "small.png")
	writePNG(t, src, 100, 150)

	got, err := thumbnail(src, 192, filepath.Join(dir, "cache"))
	if err != nil {
		t.Fatal(err)
	}
	if got != src {
		t.Errorf("got %q, want the source path %q", got, src)
	}
}

func TestThumbnailCachesAndReuses(t *testing.T) {
	dir := t.TempDir()
	cache := filepath.Join(dir, "cache")
	src := filepath.Join(dir, "cover.png")
	writePNG(t, src, 600, 900)

	first, err := thumbnail(src, 192, cache)
	if err != nil {
		t.Fatal(err)
	}
	if first == src {
		t.Fatal("expected a generated thumbnail, got the source")
	}

	f, err := os.Open(first)
	if err != nil {
		t.Fatal(err)
	}
	cfg, _, err := image.DecodeConfig(f)
	f.Close()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Width != 192 || cfg.Height != 288 {
		t.Errorf("got %dx%d, want 192x288 (aspect ratio preserved)", cfg.Width, cfg.Height)
	}

	// A second call must reuse the file rather than re-encode it.
	st, err := os.Stat(first)
	if err != nil {
		t.Fatal(err)
	}
	second, err := thumbnail(src, 192, cache)
	if err != nil {
		t.Fatal(err)
	}
	if second != first {
		t.Fatalf("cache key is unstable: %q then %q", first, second)
	}
	st2, err := os.Stat(second)
	if err != nil {
		t.Fatal(err)
	}
	if !st2.ModTime().Equal(st.ModTime()) {
		t.Error("thumbnail was regenerated on the second call")
	}
}

// Editing the source must produce a different cache entry, or the UI would keep
// showing stale art after a catalog refresh.
func TestThumbnailKeyTracksSource(t *testing.T) {
	dir := t.TempDir()
	cache := filepath.Join(dir, "cache")
	src := filepath.Join(dir, "cover.png")

	writePNG(t, src, 600, 900)
	before, err := thumbnail(src, 192, cache)
	if err != nil {
		t.Fatal(err)
	}

	writePNG(t, src, 600, 1200) // different size and mtime
	after, err := thumbnail(src, 192, cache)
	if err != nil {
		t.Fatal(err)
	}
	if before == after {
		t.Error("cache key did not change when the source did")
	}
}

func TestArtworkHandler(t *testing.T) {
	root := t.TempDir()
	cache := filepath.Join(t.TempDir(), "cache")
	if err := os.MkdirAll(filepath.Join(root, "art"), 0755); err != nil {
		t.Fatal(err)
	}
	writePNG(t, filepath.Join(root, "art", "cover.png"), 600, 900)

	h := artworkHandler(func() string { return root }, cache)

	t.Run("serves the original without a width", func(t *testing.T) {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/art/cover.png", nil))
		if w.Code != http.StatusOK {
			t.Fatalf("status %d", w.Code)
		}
		cfg, _, err := image.DecodeConfig(w.Body)
		if err != nil {
			t.Fatal(err)
		}
		if cfg.Width != 600 {
			t.Errorf("width %d, want the untouched 600", cfg.Width)
		}
	})

	t.Run("downscales with a width", func(t *testing.T) {
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/art/cover.png?w=192", nil))
		if w.Code != http.StatusOK {
			t.Fatalf("status %d", w.Code)
		}
		if cc := w.Header().Get("Cache-Control"); cc == "" {
			t.Error("no Cache-Control on a content-addressed thumbnail")
		}
		cfg, _, err := image.DecodeConfig(w.Body)
		if err != nil {
			t.Fatal(err)
		}
		if cfg.Width != 192 {
			t.Errorf("width %d, want 192", cfg.Width)
		}
	})

	t.Run("refuses to escape the catalog", func(t *testing.T) {
		// The Go http server normalises ".." out of the request target, so the
		// traversal is smuggled in already-decoded form.
		req := httptest.NewRequest(http.MethodGet, "/x", nil)
		req.URL.Path = "/../../etc/passwd"
		w := httptest.NewRecorder()
		h.ServeHTTP(w, req)
		if w.Code != http.StatusNotFound {
			t.Errorf("status %d, want 404", w.Code)
		}
	})

	t.Run("falls back when the catalog is unknown", func(t *testing.T) {
		w := httptest.NewRecorder()
		artworkHandler(func() string { return "" }, cache).
			ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/art/cover.png", nil))
		if w.Code != http.StatusNotFound {
			t.Errorf("status %d, want 404", w.Code)
		}
	})

	t.Run("serves undecodable files untouched", func(t *testing.T) {
		if err := os.WriteFile(filepath.Join(root, "art", "notes.txt"), []byte("hello"), 0644); err != nil {
			t.Fatal(err)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/art/notes.txt?w=192", nil))
		if w.Code != http.StatusOK || w.Body.String() != "hello" {
			t.Errorf("status %d body %q", w.Code, w.Body.String())
		}
	})
}

func writePNG(t *testing.T, path string, w, h int) {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.SetRGBA(x, y, color.RGBA{uint8(x), uint8(y), 128, 255})
		}
	}
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := png.Encode(f, img); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
}
