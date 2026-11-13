package toolshell

import (
	"image"
	"math"

	"github.com/weakpixel/engine/editor/camzoom"
	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/input"
	"github.com/weakpixel/engine/render"
)

const (
	// minZoom and maxZoom bound the camera zoom (1/16 to 64).
	minZoom, maxZoom = camzoom.Min, camzoom.Max
	// frameMargin is the share of the viewport a framed rectangle fills.
	frameMargin = 0.9
)

// zoomOf returns the camera's zoom, treating the zero value as 1.
func zoomOf(cam *render.Camera) float32 { return camzoom.Of(cam) }

// ZoomAt zooms cam at once by steps wheel notches around the screen point
// cur (the world point under the cursor stays under it). The zoom stays
// within 1/16 and 64. The wheel itself eases instead, see [camzoom].
func ZoomAt(cam *render.Camera, cur gfx.Vec2, screen image.Point, steps float32) {
	before := cam.ScreenToWorld(cur, screen)
	z := zoomOf(cam) * float32(math.Pow(camzoom.Step, float64(steps)))
	cam.Zoom = min(max(z, minZoom), maxZoom)
	cam.Pos = cam.Pos.Add(before.Sub(cam.ScreenToWorld(cur, screen)))
}

// PanTo moves cam so that the world point under the screen point prev is
// under cur.
func PanTo(cam *render.Camera, prev, cur gfx.Vec2, screen image.Point) {
	cam.Pos = cam.Pos.Add(cam.ScreenToWorld(prev, screen).Sub(cam.ScreenToWorld(cur, screen)))
}

// FrameRect centers r in the area free of the screen (logical pixels) and
// zooms so that r fills most of it; an empty r only centers. An empty free
// is the camera's whole viewport. The panels of the native window are
// overlays, so the camera keeps drawing the whole viewport and free is the
// part between them. The zoom stays within 1/16 and 64.
func FrameRect(cam *render.Camera, r gfx.Rect, screen image.Point, free image.Rectangle) {
	if free.Empty() {
		free = cam.ViewportRect(screen)
	}
	if free.Dx() <= 0 || free.Dy() <= 0 {
		return // no screen yet (a webview before its first layout): fit again later
	}
	if w, h := r.Max.X-r.Min.X, r.Max.Y-r.Min.Y; w > 0 && h > 0 {
		z := min(float32(free.Dx())/w, float32(free.Dy())/h) * frameMargin
		cam.Zoom = min(max(z, minZoom), maxZoom)
	}
	// Center r on the screen point in the middle of free: put it at the
	// camera center, then move by what is under that point.
	center := r.Min.Add(r.Max).Mul(0.5)
	cam.Pos = center
	mid := gfx.Vec2{X: float32(free.Min.X+free.Max.X) / 2, Y: float32(free.Min.Y+free.Max.Y) / 2}
	cam.Pos = cam.Pos.Add(center.Sub(cam.ScreenToWorld(mid, screen)))
	cam.PrevPos = cam.Pos
}

// cameraInput turns mouse input into camera movement: it remembers the
// cursor and buttons of the last tick.
type cameraInput struct {
	prev    gfx.Vec2
	buttons uint8
	zoom    camzoom.Zoom
}

// update applies one tick of mouse input. Middle button, or the left button
// with panMod (Space held), pans; the wheel zooms around the cursor, eased.
// A drag only continues if it started in the viewport, which the caller
// decides by not calling update while a panel has the mouse.
func (c *cameraInput) update(cam *render.Camera, screen image.Point, m *input.MouseFrame, panMod bool) {
	cur := gfx.Vec2{X: m.X, Y: m.Y}
	was := func(b input.MouseButton) bool { return c.buttons&(1<<b) != 0 }
	dragging := (m.Down(input.MouseMiddle) && was(input.MouseMiddle)) ||
		(panMod && m.Down(input.MouseLeft) && was(input.MouseLeft))
	if dragging {
		PanTo(cam, c.prev, cur, screen)
	}
	c.zoom.Wheel(cam, m.WheelY)
	c.zoom.Update(cam, cur, screen)
	c.prev, c.buttons = cur, m.Buttons
}

// remember records the cursor and buttons without moving the camera, for
// ticks when the panel has the mouse.
func (c *cameraInput) remember(m *input.MouseFrame) {
	c.prev, c.buttons = gfx.Vec2{X: m.X, Y: m.Y}, 0
}
