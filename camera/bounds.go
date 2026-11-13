package camera

import (
	"image"

	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/render"
)

// viewHalfSize returns half the size of the world area cam shows, in world
// units: half the viewport (Camera.ViewportRect, so a custom Viewport
// counts) divided by the zoom. Rotation is ignored.
func viewHalfSize(cam *render.Camera, screen image.Point) gfx.Vec2 {
	zoom := cam.Zoom
	if zoom <= 0 {
		zoom = 1
	}
	vp := cam.ViewportRect(screen)
	return gfx.Vec2{X: float32(vp.Dx()) / (2 * zoom), Y: float32(vp.Dy()) / (2 * zoom)}
}

// clampToBounds returns p, the view center, moved so that a view of half
// size half stays inside b. On an axis where b is smaller than the view the
// result is b's center on that axis. The zero b means unbounded.
func clampToBounds(p, half gfx.Vec2, b gfx.Rect) gfx.Vec2 {
	if b == (gfx.Rect{}) {
		return p
	}
	return b.ClampCenter(p, half)
}
