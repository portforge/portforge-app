package main

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"image"
	"image/draw"
	"image/jpeg"
	"image/png"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"

	_ "image/gif" // registered so GIF sources decode; output is PNG
)

// Artwork in the catalog is stored at print resolution — covers are 600x900 and
// key art is 3840x1240 — but the UI shows covers in a ~190px grid cell and key
// art in a 380px-tall band. Handing the WebView the full-size file means it
// decodes and holds a 2MB (cover) to 18MB (key art) RGBA bitmap and resamples it
// down on every paint, which on WebKitGTK is enough to make hover states lag by
// about a second once a handful of covers are on screen.
//
// So artwork requests carry a ?w= hint and are served from a disk cache of
// downscaled copies. Generating one costs a decode in Go, off the render thread,
// once per source file.

// Requested widths snap up to one of these so a hostile or buggy caller cannot
// fill the cache with a thumbnail per pixel width. Snapping up rather than down
// means a thumbnail is never drawn larger than it was generated. The upper
// buckets track real window widths, since key art is requested full-bleed.
var thumbWidths = []int{192, 288, 384, 576, 768, 1152, 1536, 1920, 2560, 3072}

// A source file is decoded once even if several requests for the same thumbnail
// arrive together, which matters for key art: three concurrent misses on the
// same 3840x1240 PNG would otherwise hold three 18MB bitmaps at once.
var thumbLocks sync.Map // cache key -> *sync.Mutex

// artworkHandler serves files under the catalog directory returned by root.
// Without a ?w= parameter it serves the original bytes; with one it serves a
// cached downscale. root is a func because the catalog path is only known after
// startup runs.
func artworkHandler(root func() string, cacheDir string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		base := root()
		if base == "" {
			http.NotFound(w, r)
			return
		}

		// http.FileServer does this containment check itself, but we resolve the
		// path by hand to generate thumbnails, so we have to do it here too.
		rel := strings.TrimPrefix(r.URL.Path, "/")
		src := filepath.Join(base, filepath.FromSlash(rel))
		if !strings.HasPrefix(src, filepath.Clean(base)+string(os.PathSeparator)) {
			http.NotFound(w, r)
			return
		}

		want, err := strconv.Atoi(r.URL.Query().Get("w"))
		if err != nil || want <= 0 {
			http.FileServer(http.Dir(base)).ServeHTTP(w, r)
			return
		}

		path, err := thumbnail(src, snapWidth(want), cacheDir)
		if err != nil {
			// A source that cannot be decoded is still a valid file to serve —
			// an SVG or a format Go has no decoder for is not an error case.
			http.FileServer(http.Dir(base)).ServeHTTP(w, r)
			return
		}

		f, err := os.Open(path)
		if err != nil {
			http.NotFound(w, r)
			return
		}
		defer f.Close()
		st, err := f.Stat()
		if err != nil {
			http.NotFound(w, r)
			return
		}

		// The cache key covers the source's modification time and size, so a
		// given URL+width can never refer to different bytes than it did before.
		w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
		http.ServeContent(w, r, path, st.ModTime(), f)
	})
}

func snapWidth(want int) int {
	i := sort.SearchInts(thumbWidths, want)
	if i == len(thumbWidths) {
		return thumbWidths[len(thumbWidths)-1]
	}
	return thumbWidths[i]
}

// thumbnail returns the path to a cached copy of src no wider than maxW,
// generating it if absent. When src is already that narrow it returns src
// unchanged rather than re-encoding it.
func thumbnail(src string, maxW int, cacheDir string) (string, error) {
	info, err := os.Stat(src)
	if err != nil {
		return "", err
	}

	f, err := os.Open(src)
	if err != nil {
		return "", err
	}
	// Only the header is needed to decide whether a downscale is worth doing.
	cfg, format, err := image.DecodeConfig(f)
	f.Close()
	if err != nil {
		return "", err
	}
	if cfg.Width <= maxW {
		return src, nil
	}

	// JPEG sources stay JPEG; everything else becomes PNG so alpha survives.
	ext := ".png"
	if format == "jpeg" {
		ext = ".jpg"
	}

	sum := sha256.Sum256(fmt.Appendf(nil, "%s\x00%d\x00%d\x00%d", src, info.ModTime().UnixNano(), info.Size(), maxW))
	dst := filepath.Join(cacheDir, hex.EncodeToString(sum[:12])+ext)

	if _, err := os.Stat(dst); err == nil {
		return dst, nil
	}

	lock, _ := thumbLocks.LoadOrStore(dst, &sync.Mutex{})
	mu := lock.(*sync.Mutex)
	mu.Lock()
	defer mu.Unlock()

	// Another request may have generated it while we waited for the lock.
	if _, err := os.Stat(dst); err == nil {
		return dst, nil
	}
	if err := writeThumb(src, dst, maxW, ext); err != nil {
		return "", err
	}
	return dst, nil
}

func writeThumb(src, dst string, maxW int, ext string) error {
	f, err := os.Open(src)
	if err != nil {
		return err
	}
	img, _, err := image.Decode(f)
	f.Close()
	if err != nil {
		return err
	}

	b := img.Bounds()
	h := b.Dy() * maxW / b.Dx()
	if h < 1 {
		h = 1
	}
	out := boxResize(toRGBA(img), maxW, h)

	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	// Written to a temp file and renamed so a reader can never observe a
	// half-encoded thumbnail, and so a crash mid-encode leaves no bad cache entry.
	tmp, err := os.CreateTemp(filepath.Dir(dst), ".thumb-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())

	if ext == ".jpg" {
		err = jpeg.Encode(tmp, out, &jpeg.Options{Quality: 88})
	} else {
		enc := png.Encoder{CompressionLevel: png.BestSpeed}
		err = enc.Encode(tmp, out)
	}
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	return os.Rename(tmp.Name(), dst)
}

// toRGBA normalises any image to an *image.RGBA anchored at the origin, so the
// resampler can index Pix directly without carrying a bounds offset through
// every read.
func toRGBA(img image.Image) *image.RGBA {
	b := img.Bounds()
	if src, ok := img.(*image.RGBA); ok && b.Min == (image.Point{}) {
		return src
	}
	dst := image.NewRGBA(image.Rect(0, 0, b.Dx(), b.Dy()))
	draw.Draw(dst, dst.Bounds(), img, b.Min, draw.Src)
	return dst
}

// boxResize averages each destination pixel over the source pixels it covers.
// For the reductions here — 600px to 192px, 3840px to 1536px — a box filter is
// the correct choice: it samples every source pixel exactly once, so nothing
// aliases, and unlike a windowed filter it needs no weight tables.
//
// image.RGBA is alpha-premultiplied, so averaging the channels directly is
// correct and needs no un-premultiply round trip.
func boxResize(src *image.RGBA, dstW, dstH int) *image.RGBA {
	sw, sh := src.Bounds().Dx(), src.Bounds().Dy()
	dst := image.NewRGBA(image.Rect(0, 0, dstW, dstH))

	// Column spans are the same for every row, so they are computed once.
	spans := make([][2]int, dstW)
	for x := range spans {
		x0, x1 := x*sw/dstW, (x+1)*sw/dstW
		if x1 <= x0 {
			x1 = x0 + 1
		}
		spans[x] = [2]int{x0, x1}
	}

	for y := 0; y < dstH; y++ {
		y0, y1 := y*sh/dstH, (y+1)*sh/dstH
		if y1 <= y0 {
			y1 = y0 + 1
		}
		drow := dst.Pix[y*dst.Stride : y*dst.Stride+dstW*4]

		for x, span := range spans {
			var r, g, b, a uint32
			for sy := y0; sy < y1; sy++ {
				row := src.Pix[sy*src.Stride+span[0]*4 : sy*src.Stride+span[1]*4]
				for i := 0; i < len(row); i += 4 {
					r += uint32(row[i])
					g += uint32(row[i+1])
					b += uint32(row[i+2])
					a += uint32(row[i+3])
				}
			}
			n := uint32((span[1] - span[0]) * (y1 - y0))
			o := x * 4
			drow[o] = uint8(r / n)
			drow[o+1] = uint8(g / n)
			drow[o+2] = uint8(b / n)
			drow[o+3] = uint8(a / n)
		}
	}
	return dst
}

// thumbCacheDir is where downscaled artwork is kept. It is a cache in the strict
// sense: deleting it costs nothing but the work to regenerate.
func thumbCacheDir() string {
	if dir, err := os.UserCacheDir(); err == nil {
		return filepath.Join(dir, "PortForge", "thumbs")
	}
	if dir, err := configDir(); err == nil {
		return filepath.Join(dir, "thumbs")
	}
	return filepath.Join(os.TempDir(), "portforge-thumbs")
}
