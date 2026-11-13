package physics

import (
	"math"

	"github.com/weakpixel/ecs"
	"github.com/weakpixel/engine/collision"
	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/xmath"
)

// pushJoin is how far apart two dynamic bodies may be and still push as
// one row: bodies resting side by side are rarely exactly flush after the
// solver, and a pixel mover could never close a gap of less than a unit.
const pushJoin = 0.5

// pushed is what a push did: how far the mover moved along X (signed) and
// the total mass of the row.
type pushed struct {
	moved, mass float32
}

// pushable reports whether MoveAndSlide may push e: a dynamic body free
// to move along X, with a collider it can place. A tile hit has no entity.
func pushable(w *ecs.World, e ecs.Entity) bool {
	if e == (ecs.Entity{}) {
		return false
	}
	b := ecs.GetCompOrNil[Body](w, e)
	if b == nil || b.Kind != Dynamic || b.LockX {
		return false
	}
	t := ecs.GetCompOrNil[gfx.Transform](w, e)
	c := ecs.GetCompOrNil[collision.Collider](w, e)
	if t == nil || c == nil || c.Shape >= collision.ShapeCustom {
		return false
	}
	_, ok := boundsOf(t, c)
	return ok
}

// massOf returns the mass of a dynamic body: Body.Mass, zero meaning 1.
func massOf(b *Body) float32 {
	if b.Mass <= 0 {
		return 1
	}
	return b.Mass
}

// push moves the row of dynamic bodies starting at first (the X blocker of
// a mover moving by dx) and returns how far the mover moves with it, or
// false when the row is blocked and the mover stops at first as at a wall.
// It sets the X remainder of a pixel move only when it returns true.
func push(w *ecs.World, sp *collision.Space, first blocker, dx float32, opts MoveOptions) (pushed, bool) {
	s := xmath.Sign(dx)
	r := xmath.Abs(dx) - first.gap // the part of the move that pushes
	// A fixed array keeps the row on the stack: MoveAndSlide does not
	// allocate.
	var row [maxStack]ecs.Entity
	row[0] = first.entity
	mass, n := massOf(ecs.GetComp[Body](w, first.entity)), 1
	free := r // how far the row can move before something blocks it
	for {
		b := sweepBody(w, sp, row[n-1], row[:n], r*s)
		if !b.hit {
			break
		}
		if b.gap > pushJoin || !pushable(w, b.entity) {
			free = b.gap
			break
		}
		if n == maxStack {
			return pushed{}, false
		}
		row[n] = b.entity
		mass += massOf(ecs.GetComp[Body](w, b.entity))
		n++
	}
	// The front body was swept above; the others can be blocked on their
	// own (a taller crate under a low ceiling).
	for _, e := range row[:n-1] {
		if b := sweepBody(w, sp, e, row[:n], r*s); b.hit {
			free = min(free, b.gap)
		}
	}
	p := r * opts.Push / (opts.Push + mass)
	limited := free < p
	p = min(p, free)
	total := first.gap + p
	if opts.Pixel {
		total = float32(math.Trunc(float64(total)))
	}
	move := max(total-first.gap, 0)
	if limited && move == 0 {
		return pushed{}, false
	}
	if opts.Pixel {
		if limited {
			opts.Remainder.X = 0
		} else {
			opts.Remainder.X += (first.gap + p - total) * s
		}
	}
	for _, e := range row[:n] {
		ecs.GetComp[gfx.Transform](w, e).Pos.X += move * s
	}
	return pushed{moved: total * s, mass: mass}, true
}

// sweepBody sweeps the pushed body e along X by d with its own collider,
// mask and DropThrough, ignoring the bodies in row.
func sweepBody(w *ecs.World, sp *collision.Space, e ecs.Entity, row []ecs.Entity, d float32) blocker {
	t := ecs.GetComp[gfx.Transform](w, e)
	c := ecs.GetComp[collision.Collider](w, e)
	box, _ := boundsOf(t, c)
	return sweep(w, sp, e, row, c.Mask, box, 0, d, ecs.GetComp[Body](w, e).DropThrough)
}
