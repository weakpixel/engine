package camera

import (
	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/xmath"
)

// lookStep eases the look-ahead offset look over dt: toward vel's direction
// scaled to distance while the speed exceeds minSpeed, otherwise toward
// zero. tau is the easing time constant. A dt of zero or less leaves look
// unchanged.
func lookStep(look, vel gfx.Vec2, dt, distance, tau, minSpeed float32) gfx.Vec2 {
	if dt <= 0 {
		return look
	}
	var want gfx.Vec2
	if speed := vel.Len(); speed > minSpeed {
		want = vel.Mul(distance / speed)
	}
	return look.Lerp(want, xmath.ExpFactor(dt, tau))
}

// velocity is the target's velocity from its position at the start and end
// of the tick. dt must be positive.
func velocity(prev, pos gfx.Vec2, dt float32) gfx.Vec2 {
	return pos.Sub(prev).Mul(1 / dt)
}
