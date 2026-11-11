package notebook

import (
	"bytes"
	"container/list"
	"encoding/base64"
	"image"
	"image/png"
	"strings"

	"github.com/egoist/mygo/ui"
	"golang.org/x/image/draw"
)

// Only derived display bitmaps live here. Original bytes remain owned by
// documents/undo, so eviction cannot change copy, export or recovery.
// A Book owns one cache; drawing and eviction run only on the UI thread.
type imageCache struct {
	entries map[[32]byte]*list.Element
	lru     list.List
	bytes   int
}

type cachedImage struct {
	key    [32]byte
	bitmap *ui.Bitmap
	bytes  int
}

// Hold a small working set of Retina previews without cycling the decoder on
// every tab switch. This bounds retained preview pixels, not total process/GPU
// memory; original documents and transient decoding allocations are separate.
const imageCacheBudget = 32 * 1024 * 1024

func (c *imageCache) bitmap(key [32]byte, url string) *ui.Bitmap {
	if e := c.entries[key]; e != nil {
		c.lru.MoveToFront(e)
		return e.Value.(cachedImage).bitmap
	}
	// Attachments have already passed a complete decode. Their immutable
	// originals are kept even when this display preview has been evicted.
	raw, _ := base64.StdEncoding.DecodeString(strings.TrimPrefix(url, "data:image/png;base64,"))
	pixels, _ := png.Decode(bytes.NewReader(raw))
	bounds := pixels.Bounds()
	w, h := bounds.Dx(), bounds.Dy()
	// Twice the normal 560×320 DIP attachment size, preserving aspect ratio.
	// Large originals are still copied/exported at their original resolution.
	scale := min(float64(1), 1120/float64(w), 640/float64(h))
	if scale < 1 {
		w, h = max(1, int(float64(w)*scale)), max(1, int(float64(h)*scale))
		preview := image.NewRGBA(image.Rect(0, 0, w, h))
		draw.ApproxBiLinear.Scale(preview, preview.Bounds(), pixels, bounds, draw.Src, nil)
		pixels = preview
	}
	bitmap := ui.NewBitmap(pixels)
	// Include an estimate for a full mip pyramid, not just base RGBA pixels.
	cost := w * h * 4 * 4 / 3
	for c.bytes+cost > imageCacheBudget && c.lru.Len() > 0 {
		e := c.lru.Back()
		old := e.Value.(cachedImage)
		delete(c.entries, old.key)
		c.bytes -= old.bytes
		c.lru.Remove(e)
	}
	if c.entries == nil {
		c.entries = make(map[[32]byte]*list.Element)
	}
	c.entries[key] = c.lru.PushFront(cachedImage{key, bitmap, cost})
	c.bytes += cost
	return bitmap
}
