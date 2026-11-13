package camera

import (
	"github.com/weakpixel/engine/gfx"
)

// deadZoneStep returns the dead-zone window center after the goal moved:
// unchanged while goal is within ±half of it, otherwise moved on each axis
// just enough to put goal on the window's edge.
func deadZoneStep(window, goal, half gfx.Vec2) gfx.Vec2 {
	return gfx.Vec2{
		X: deadZoneAxis(window.X, goal.X, half.X),
		Y: deadZoneAxis(window.Y, goal.Y, half.Y),
	}
}

func deadZoneAxis(c, g, half float32) float32 {
	switch {
	case g > c+half:
		return g - half
	case g < c-half:
		return g + half
	}
	return c
}
