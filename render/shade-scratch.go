package render

import (
	"image"
	"math"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/weakpixel/engine/gfx"
)

// This file lets the kinds that are made of several quads (text, nine-slice
// panels, shapes) draw through a shader. Such an item is first drawn
// unshaded, white and unrotated, in its own pixels, into the scratch image;
// then that region is drawn as one shaded quad with the item's transform,
// tint, blend and Params, exactly like a sprite. So the shader sees the
// whole item as imageSrc0 (an outline runs around the whole text, a dissolve
// across the whole panel) instead of one glyph or panel piece at a time.

// shadeMargin is the transparent border, in pixels, around an item in the
// scratch image. It gives outline and glow shaders room to paint outside
// the item; sprites get that room from padding in their frame.
const shadeMargin = 4

// maxScratchSide is the largest scratch region in pixels per axis. An item
// that needs more is drawn without its shader: every GPU handles a texture
// of this size.
const maxScratchSide = 4096

// maxScratchRegions bounds the cache of region images (see shadeScratch).
const maxScratchRegions = 64

// shadeScratch is the Renderer's scratch image for shaded multi-quad items.
//
// The image only grows (to the next power of two per axis), so a steady
// frame allocates nothing. regions caches the sub-images handed out, by
// size: Ebiten allocates a sub-image object on every SubImage call, and a
// shader must see exactly the item's region as its source image, so the
// same sizes are reused from frame to frame.
type shadeScratch struct {
	img     *ebiten.Image
	regions map[image.Point]*ebiten.Image
}

// scratchSide returns the side of a scratch image that fits need pixels and
// is at least have: the next power of two, so a slowly growing text does not
// reallocate the image on every change.
func scratchSide(have, need int) int {
	side := max(have, 64)
	for side < need {
		side *= 2
	}
	return side
}

// scratchRegion returns the size in pixels of the scratch region of an item
// whose local bounds have the given size: rounded up to whole pixels, plus
// the margin on every side. ok is false when the item has no area or is too
// large for the scratch image.
func scratchRegion(size gfx.Vec2) (region image.Point, ok bool) {
	if !(size.X > 0 && size.Y > 0) { // also false for NaN
		return image.Point{}, false
	}
	w := math.Ceil(float64(size.X)) + 2*shadeMargin
	h := math.Ceil(float64(size.Y)) + 2*shadeMargin
	if w > maxScratchSide || h > maxScratchSide {
		return image.Point{}, false
	}
	return image.Pt(int(w), int(h)), true
}

// region returns the cleared scratch region of the given size, a sub-image
// at the scratch image's top-left corner. It allocates only when the image
// has to grow or the size was not used before.
func (s *shadeScratch) region(size image.Point) *ebiten.Image {
	if s.img == nil || s.img.Bounds().Dx() < size.X || s.img.Bounds().Dy() < size.Y {
		w, h := 0, 0
		if s.img != nil {
			w, h = s.img.Bounds().Dx(), s.img.Bounds().Dy()
			s.img.Deallocate()
		}
		s.img = ebiten.NewImage(scratchSide(w, size.X), scratchSide(h, size.Y))
		clear(s.regions) // sub-images of the old image
	}
	reg := s.regions[size]
	if reg == nil {
		if s.regions == nil {
			s.regions = make(map[image.Point]*ebiten.Image)
		}
		if len(s.regions) >= maxScratchRegions {
			clear(s.regions)
		}
		reg = s.img.SubImage(image.Rectangle{Max: size}).(*ebiten.Image)
		s.regions[size] = reg
	}
	reg.Clear()
	return reg
}

// beginScratch starts drawing a shaded multi-quad item whose local bounds
// (relative to its pivot, before scale and rotation) start at lo and have
// the given size. It returns the image to draw the item into and the
// transform that places the pivot there, unscaled and unrotated, and sets
// the style to plain white. ok is false when the item cannot go through the
// scratch image (no area, or too large): draw it unshaded then.
//
// The caller draws the item and then calls endScratch with the same region
// and lo.
func (r *Renderer) beginScratch(lo, size gfx.Vec2) (region *ebiten.Image, at transform, ok bool) {
	px, ok := scratchRegion(size)
	if !ok {
		return nil, transform{}, false
	}
	r.flush() // the item's quads go to another target
	region = r.scratch.region(px)
	r.setStyle(0, BlendNormal)
	at = transform{
		pos:   gfx.Vec2{X: shadeMargin - lo.X, Y: shadeMargin - lo.Y},
		scale: gfx.Vec2{X: 1, Y: 1},
	}
	return region, at, true
}

// endScratch draws the scratch region, which now holds the item, onto dst
// through sh: one shaded quad placed by t like a sprite whose top-left
// corner is at lo minus the margin relative to its pivot, with the item's
// tint, blend, Params and second image.
func (r *Renderer) endScratch(dst, region *ebiten.Image, sh *ebiten.Shader, it *Item, t transform, lo gfx.Vec2) {
	r.flush() // finish the item in the scratch image before it is read
	geoM(&r.opts.GeoM, gfx.Vec2{X: lo.X - shadeMargin, Y: lo.Y - shadeMargin}, false, false, t)
	r.setStyle(it.Color, it.Blend)
	r.addShadedQuad(dst, region, r.aux(it.Aux), &r.opts.GeoM, &r.opts.ColorScale, r.blend, sh, &it.Params)
	// The next shaded item reuses the scratch image, so this quad cannot
	// wait in the batch.
	r.flush()
}
