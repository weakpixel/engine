// Package camzoom is the wheel zoom of the editors' cameras: each wheel unit
// scales the zoom by a fixed factor (so a trackpad's small deltas zoom
// slowly and a mouse notch a little), and the camera eases toward the
// target zoom around the cursor instead of jumping.
package camzoom

import (
	"image"
	"math"

	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/render"
	"github.com/weakpixel/engine/xmath"
)

const (
	// Min and Max bound the camera zoom (1/16 to 64).
	Min, Max = 1.0 / 16, 64
	// Step is the zoom factor per wheel unit; a mouse notch is one unit.
	Step = 1.1
	// maxWheel limits the wheel movement of one tick, so a browser that
	// reports a notch in pixels cannot zoom by more than a few steps.
	maxWheel = 3
	// ease is the share of the remaining (logarithmic) distance to the
	// target that one tick covers.
	ease = 0.25
	// settle is the ratio to the target below which the zoom snaps to it.
	settle = 1.002
)

// Zoom eases a camera toward a wheel-driven target zoom. The zero value is
// ready; it follows a camera that other code (framing, restoring) changes.
type Zoom struct {
	target float32 // zoom the camera eases toward
	last   float32 // zoom this Zoom last set, to notice other writers
}

// Of returns the camera's zoom, treating the zero value as 1.
func Of(cam *render.Camera) float32 {
	if cam.Zoom == 0 {
		return 1
	}
	return cam.Zoom
}

// Clamp limits z to [Min, Max].
func Clamp(z float32) float32 { return xmath.Clamp(z, Min, Max) }

// Wheel moves the target zoom by wheel units (positive zooms in, as
// [input.MouseFrame] reports it, negative out); call it once per tick with
// that tick's wheel movement, then [Zoom.Update].
func (z *Zoom) Wheel(cam *render.Camera, wheel float32) {
	z.sync(cam)
	wheel = min(max(wheel, -maxWheel), maxWheel)
	z.target = Clamp(z.target * float32(math.Pow(Step, float64(wheel))))
}

// Update moves the camera one tick toward the target, keeping the world
// point under the screen point cur where it is. It does nothing when the
// camera is at the target.
func (z *Zoom) Update(cam *render.Camera, cur gfx.Vec2, screen image.Point) {
	z.sync(cam)
	if z.target == z.last {
		return
	}
	next := z.target
	if r := float64(z.target / z.last); r > settle || r < 1/settle {
		next = z.last * float32(math.Pow(r, ease))
	}
	before := cam.ScreenToWorld(cur, screen)
	cam.Zoom = next
	cam.Pos = cam.Pos.Add(before.Sub(cam.ScreenToWorld(cur, screen)))
	z.last = next
}

// sync restarts from the camera when something else changed its zoom.
func (z *Zoom) sync(cam *render.Camera) {
	if c := Of(cam); c != z.last {
		z.target, z.last = c, c
	}
}
