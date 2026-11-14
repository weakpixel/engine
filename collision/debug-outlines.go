package collision

import "github.com/weakpixel/engine/gfx"

// colliderColors are the outline colors of collision layers 0..7; layer n
// uses colliderColors[n%8].
var colliderColors = [8]gfx.Color{
	gfx.RGBA(255, 255, 255, 255), // 0, default: white
	gfx.RGBA(80, 220, 100, 255),  // green
	gfx.RGBA(255, 80, 80, 255),   // red
	gfx.RGBA(255, 210, 60, 255),  // yellow
	gfx.RGBA(80, 160, 255, 255),  // blue
	gfx.RGBA(255, 120, 220, 255), // pink
	gfx.RGBA(80, 230, 230, 255),  // cyan
	gfx.RGBA(255, 150, 50, 255),  // orange
}

// ColliderColor returns the debug outline color of a collider on layers l:
// the color of its lowest layer, from a palette of eight that repeats.
func ColliderColor(l Layers) gfx.Color {
	for i := range 32 {
		if l&(1<<i) != 0 {
			return colliderColors[i%len(colliderColors)]
		}
	}
	return colliderColors[0]
}

// debugOutlines yields the colliders placed in the last run as outlines
// colored by [ColliderColor]. Custom shapes draw as their bounds. It is the
// [gfx.DebugOutlines] source of the Space.
func (s *Space) debugOutlines(yield func(gfx.DebugOutline) bool) {
	for sh := range s.Shapes() {
		o := gfx.DebugOutline{Bounds: sh.Bounds, Color: ColliderColor(sh.Layer)}
		if sh.Kind == ShapeCircle {
			o.Circle, o.Center, o.Radius = true, sh.Center, sh.Radius
		}
		if !yield(o) {
			return
		}
	}
}
