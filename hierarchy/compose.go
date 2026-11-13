package hierarchy

import "github.com/weakpixel/engine/gfx"

// scaleOrOne applies the Transform rule: the zero Scale is (1, 1).
func scaleOrOne(s gfx.Vec2) gfx.Vec2 {
	if s == (gfx.Vec2{}) {
		return gfx.Vec2{X: 1, Y: 1}
	}
	return s
}

// mirrored reports whether a scale flips the handedness (one negative axis).
func mirrored(s gfx.Vec2) bool { return s.X*s.Y < 0 }

// Compose returns the world transform of a child placed at l in parent's
// space. Only Pos, Rot and Scale are set; PrevPos is the caller's business.
// Non-uniform scale with rotation has no shear: the offset is scaled and then
// rotated, and the child's own scale and rotation stay independent.
func Compose(parent gfx.Transform, l Local) gfx.Transform {
	ps := scaleOrOne(parent.Scale)
	out := gfx.Transform{
		Pos:   parent.Pos.Add(l.Pos.MulV(ps).Rotate(parent.Rot)),
		Scale: ps.MulV(scaleOrOne(l.Scale)),
	}
	if mirrored(ps) {
		out.Rot = parent.Rot - l.Rot
	} else {
		out.Rot = parent.Rot + l.Rot
	}
	return out
}

// LocalOf is the inverse of Compose: the Local (Pos, Rot, Scale) that places
// a child at world under parent. A parent scale axis of zero cannot be
// inverted; the local axis is then zero.
func LocalOf(parent, world gfx.Transform) Local {
	ps := scaleOrOne(parent.Scale)
	d := world.Pos.Sub(parent.Pos).Rotate(-parent.Rot)
	l := Local{
		Pos:   gfx.Vec2{X: safeDiv(d.X, ps.X), Y: safeDiv(d.Y, ps.Y)},
		Scale: gfx.Vec2{X: safeDiv(scaleOrOne(world.Scale).X, ps.X), Y: safeDiv(scaleOrOne(world.Scale).Y, ps.Y)},
	}
	if mirrored(ps) {
		l.Rot = parent.Rot - world.Rot
	} else {
		l.Rot = world.Rot - parent.Rot
	}
	return l
}

func safeDiv(a, b float32) float32 {
	if b == 0 {
		return 0
	}
	return a / b
}
