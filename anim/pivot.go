package anim

import (
	"github.com/weakpixel/engine/anim/animfile"
	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/render"
)

// PivotOrigin converts a pivot p in pixels of the trimmed sprite image into
// the renderer's Sprite.Origin: a fraction of the untrimmed size, measured
// from the untrimmed top-left, which is where the trimmed image starts minus
// Offset. The pivot is measured from each frame's own trimmed rectangle, so
// it stays on the same art pixel when every frame has the same trim offset.
// sp must have a positive Size. The animation loader and the anim editor use
// this one formula.
func PivotOrigin(sp *render.Sprite, p animfile.Point) gfx.Vec2 {
	o := gfx.Vec2{
		X: (sp.Offset.X + float32(p.X)) / sp.Size.X,
		Y: (sp.Offset.Y + float32(p.Y)) / sp.Size.Y,
	}
	if o == (gfx.Vec2{}) {
		// The zero Origin means "the sprite's default pivot" to the renderer;
		// a tiny offset keeps a top-left pivot.
		o = gfx.Vec2{X: 1e-4, Y: 1e-4}
	}
	return o
}
