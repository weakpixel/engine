package editor

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/render"
)

// alphaKey identifies a measured frame. The pixel rectangle is part of the
// key so a hot-reloaded sprite that moved in its sheet is measured again.
type alphaKey struct {
	id   gfx.SpriteHandle
	rect image.Rectangle
}

// alphaCache measures the visible art of sprite frames once. Picking runs
// on every mouse move, and reading pixels back from the GPU is slow.
type alphaCache struct {
	m map[alphaKey]image.Rectangle
	// read returns the bounds of the non-transparent pixels of img, relative
	// to img. ok is false when the pixels cannot be read (yet) or there are
	// none. It is a field so tests can run without a display.
	read func(img *ebiten.Image) (r image.Rectangle, ok bool)
}

func newAlphaCache() *alphaCache {
	return &alphaCache{m: map[alphaKey]image.Rectangle{}, read: readOpaque}
}

// of returns the rectangle of sp's visible art relative to the frame's
// pixel rectangle. It is the whole frame when the sprite is not loaded, its
// pixels cannot be read or it is fully transparent, so such an object stays
// pickable by its frame.
func (c *alphaCache) of(sprites render.SpriteStore, id gfx.SpriteHandle, sp *render.Sprite) image.Rectangle {
	full := image.Rectangle{Max: sp.Rect.Size()}
	if c == nil || sp.Image == nil || !sprites.Ready(id) {
		return full
	}
	k := alphaKey{id, sp.Rect}
	if r, ok := c.m[k]; ok {
		return r
	}
	r, ok := c.read(sp.Image)
	if !ok {
		return full // not cached: try again once pixels can be read
	}
	c.m[k] = r
	return r
}

// readOpaque reads img back from the GPU and returns the bounds of its
// pixels with alpha > 0. Ebiten allows reading only while the game runs
// (inside Update or Draw) and panics before; that counts as "cannot read".
func readOpaque(img *ebiten.Image) (r image.Rectangle, ok bool) {
	defer func() {
		if recover() != nil {
			r, ok = image.Rectangle{}, false
		}
	}()
	b := img.Bounds()
	w, h := b.Dx(), b.Dy()
	pix := make([]byte, 4*w*h)
	img.ReadPixels(pix)
	r = image.Rectangle{Min: image.Pt(w, h)}
	for y := range h {
		row := pix[4*w*y : 4*w*(y+1)]
		for x := range w {
			if row[4*x+3] == 0 {
				continue
			}
			r.Min.X, r.Min.Y = min(r.Min.X, x), min(r.Min.Y, y)
			r.Max.X, r.Max.Y = max(r.Max.X, x+1), max(r.Max.Y, y+1)
		}
	}
	return r, !r.Empty()
}
