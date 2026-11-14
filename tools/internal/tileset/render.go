package tileset

import (
	"image"
	"image/draw"
)

// Render copies every sub-texture of a from src into its cell on a
// transparent NRGBA image (rotated sub-textures turned upright) of l.Size. draw.Src converts any source format
// (paletted included) to non-premultiplied NRGBA.
func Render(src image.Image, a *Atlas, l Layout, align Align) *image.NRGBA {
	dst := image.NewNRGBA(image.Rectangle{Max: l.Size()})
	for i, s := range a.SubTextures {
		at := l.Origin(i).Add(l.Offset(s, align))
		if s.Rotated {
			drawUpright(dst, at, src, s.Rect)
			continue
		}
		draw.Draw(dst, image.Rectangle{Min: at, Max: at.Add(s.Rect.Size())}, src, s.Rect.Min, draw.Src)
	}
	return dst
}

// drawUpright copies r of src to dst at "at", turned 90 degrees
// counter-clockwise: the atlas stores a rotated sub-texture clockwise. The
// result is r.Dy() wide and r.Dx() high.
func drawUpright(dst *image.NRGBA, at image.Point, src image.Image, r image.Rectangle) {
	for y := range r.Dx() {
		for x := range r.Dy() {
			dst.Set(at.X+x, at.Y+y, src.At(r.Max.X-1-y, r.Min.Y+x))
		}
	}
}
