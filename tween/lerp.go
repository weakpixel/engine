package tween

import (
	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/xmath"
)

// The lerp functions return the value t of the way from a to b: a at 0, b
// at 1, exactly. They extrapolate for t outside [0, 1], which overshooting
// eases and [Builder.Relative] and [Builder.Additive] rely on.
//
// They are thin wrappers of [xmath.Lerp] and [xmath.Vec2.Lerp] that exist as
// function values for [NewLens].

// LerpFloat interpolates a float32.
func LerpFloat(a, b, t float32) float32 { return xmath.Lerp(a, b, t) }

// LerpVec2 interpolates each coordinate of a gfx.Vec2.
func LerpVec2(a, b gfx.Vec2, t float32) gfx.Vec2 { return a.Lerp(b, t) }

// LerpFloat4 interpolates each element of a [4]float32, such as
// gfx.Sprite.Params.
func LerpFloat4(a, b [4]float32, t float32) [4]float32 {
	return [4]float32{
		LerpFloat(a[0], b[0], t), LerpFloat(a[1], b[1], t),
		LerpFloat(a[2], b[2], t), LerpFloat(a[3], b[3], t),
	}
}

// LerpColor interpolates each channel of a gfx.Color in 0..1 RGBA,
// non-premultiplied, and clamps the result to the valid range. The zero
// Color counts as opaque white, as everywhere in gfx. It returns a and b
// themselves at t = 0 and t = 1.
func LerpColor(a, b gfx.Color, t float32) gfx.Color {
	switch t {
	case 0:
		return a
	case 1:
		return b
	}
	ar, ag, ab, aa := a.Floats()
	br, bg, bb, ba := b.Floats()
	c := gfx.RGBA(channel(ar, br, t), channel(ag, bg, t), channel(ab, bb, t), channel(aa, ba, t))
	if c == 0 {
		// 0 would mean opaque white; any other bits with zero alpha are
		// the same transparent color.
		c = gfx.RGBA(0, 0, 1, 0)
	}
	return c
}

// channel interpolates one 0..1 channel and converts it to a rounded byte.
func channel(a, b, t float32) uint8 {
	v := LerpFloat(a, b, t)*255 + 0.5
	switch {
	case v <= 0:
		return 0
	case v >= 255:
		return 255
	}
	return uint8(v)
}
