package physics

import (
	"github.com/weakpixel/engine/collision"
	"github.com/weakpixel/engine/gfx"
)

// oneWaySlop is how deep a body may be inside a one-way collider beyond
// what it moved into it this substep and still land on it: resting bodies
// stay a little inside.
const oneWaySlop = 0.5

// face returns the direction the blocking face of a one-way collider looks
// to: the normal a blocked body is pushed along.
func face(ow collision.OneWay) gfx.Vec2 {
	switch ow {
	case collision.OneWayUp:
		return gfx.Vec2{Y: -1}
	case collision.OneWayDown:
		return gfx.Vec2{Y: 1}
	case collision.OneWayLeft:
		return gfx.Vec2{X: -1}
	case collision.OneWayRight:
		return gfx.Vec2{X: 1}
	}
	return gfx.Vec2{}
}

// oneWayBlocks reports whether a one-way collider (body index o, or -1)
// blocks body p (or -1) in a contact with normal n out of the collider: the
// normal points out of the blocking face and p is not deeper inside than
// it moves into the collider in a substep, so it came from the blocking
// side. A contact that blocked in the last substep (held is true) keeps
// blocking while the normal still points out of the face: a body that
// landed deep is pushed out over a few substeps. A body with DropThrough is
// never blocked.
func (s *System) oneWayBlocks(p, o int32, ow collision.OneWay, n gfx.Vec2, depth float32, held bool) bool {
	var moved gfx.Vec2
	h := s.solve.Dt
	if p >= 0 {
		b := &s.bodies[p]
		b.oneWay = true
		if b.b.DropThrough {
			return false
		}
		moved = b.step(s, h)
	}
	if o >= 0 {
		moved = moved.Sub(s.bodies[o].step(s, h))
	}
	f := face(ow)
	if n.Dot(f) < 0.7 {
		return false
	}
	if held {
		return true
	}
	return depth <= max(-moved.Dot(f), 0)+oneWaySlop
}
