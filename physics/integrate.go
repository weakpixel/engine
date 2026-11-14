package physics

import (
	"github.com/weakpixel/ecs"
	"github.com/weakpixel/ecs/system"
	"github.com/weakpixel/engine/collision"
	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/xmath"
)

// body is the solver's view of one body for a tick. The pointers point into
// the World's columns: Run makes no structural change, so they stay valid
// for the tick and the solver loop needs no ECS lookups.
type body struct {
	e        ecs.Entity
	t        *gfx.Transform
	vel      *gfx.Vec2
	b        *Body
	col      *collision.Collider // nil without a collider
	touching *Touching           // nil without one
	mat      Material
	inv      gfx.Vec2 // per-axis inverse mass: zero for kinematic bodies and locked axes
	carrier  int32    // index of the kinematic body it stands on (directly or through a stack), or -1
	fl       flags
	// The position correction of a substep: the largest push in each
	// direction per axis. Pushes in one direction do not add up, so a box
	// resting on two tile cells is not lifted twice.
	corrMin, corrMax gfx.Vec2
	oneWay           bool // touched a one-way collider this substep
}

// flags are the contact flags of one substep.
type flags struct {
	onGround, onCeiling bool
	onWall              int8
	ground              ecs.Entity
	groundNormal        gfx.Vec2
}

// addMissingVelocities gives bodies without a Velocity a zero one through
// deferred commands, so a prefab with only a Body still works from the next
// tick on.
func (s *System) addMissingVelocities(cmds *system.Commands) {
	s.missing = s.missing[:0]
	for e := range s.noVel.Each() {
		s.missing = append(s.missing, e)
	}
	if len(s.missing) == 0 {
		return
	}
	if cmds == nil {
		cmds = system.CommandsOf(s.w)
	}
	for _, e := range s.missing {
		cmds.Do(func(w *ecs.World) {
			if w.IsAlive(e) && !ecs.HasComp[Velocity](w, e) {
				ecs.SetComp(w, e, Velocity{})
			}
		})
	}
}

// applyFollow sets the Velocity of every Follow body so that it reaches its
// target at the end of the tick.
func (s *System) applyFollow(dt float32) {
	for e, c := range s.follows.Each() {
		f := c.Comp2
		if !f.started {
			f.Target, f.started = c.Comp1.Pos, true
		}
		if v := ecs.GetCompOrNil[Velocity](s.w, e); v != nil {
			v.Vel = f.Target.Sub(c.Comp1.Pos).Mul(1 / dt)
		}
	}
}

// collect fills the body list and the tile grids for this tick.
func (s *System) collect() {
	s.bodies = s.bodies[:0]
	clear(s.index)
	for e, c := range s.view.Each() {
		b := body{e: e, t: c.Comp1, b: c.Comp2, vel: &c.Comp3.Vel, carrier: -1}
		b.col = ecs.GetCompOrNil[collision.Collider](s.w, e)
		b.touching = ecs.GetCompOrNil[Touching](s.w, e)
		b.mat = s.materialOf(e)
		if b.b.Kind == Dynamic {
			m := massOf(b.b)
			if !b.b.LockX {
				b.inv.X = 1 / m
			}
			if !b.b.LockY {
				b.inv.Y = 1 / m
			}
		}
		b.fl.ground = s.rides[e].ground
		s.index[e] = int32(len(s.bodies))
		s.bodies = append(s.bodies, b)
	}
	// Velocities of riders are relative to last tick's carrier; the first
	// substep switches them if the ground changed.
	for i := range s.bodies {
		b := &s.bodies[i]
		if j, ok := s.index[s.rides[b.e].carrier]; ok && s.bodies[j].b.Kind == Kinematic {
			b.carrier = j
		}
	}
	s.riders = s.riders[:0]
	for _, c := range s.movers.Each() {
		ts := c.Comp3
		if !ts.OnGround {
			continue
		}
		if j, ok := s.index[ts.Ground]; ok && (s.bodies[j].b.Kind == Kinematic || s.bodies[j].carrier >= 0) {
			s.riders = append(s.riders, rider{t: c.Comp1, carrier: j})
		}
	}
	s.grids = s.grids[:0]
	for g, opts := range s.space.TileGrids() {
		m, ok := s.tileMats[g]
		if !ok {
			m = s.defMat
		}
		s.grids = append(s.grids, gridInfo{grid: g, opts: opts, mat: m})
	}
}

// materialOf returns e's Material or the default one.
func (s *System) materialOf(e ecs.Entity) Material {
	if m := ecs.GetCompOrNil[Material](s.w, e); m != nil {
		return *m
	}
	return s.defMat
}

// maxStack bounds how many dynamic bodies carrierOf follows down a stack,
// so a cycle of grounds (two bodies resting on each other) ends.
const maxStack = 8

// carrierOf returns the index of the kinematic body b stands on, directly
// or through a stack of dynamic bodies, or -1. Only dynamic bodies are
// carried. A whole stack shares the kinematic body's frame, so the bodies
// in it do not slide against each other when the carrier speeds up.
func (s *System) carrierOf(b *body) int32 {
	if b.b.Kind != Dynamic {
		return -1
	}
	for range maxStack {
		j, ok := s.index[b.fl.ground]
		if !ok {
			return -1
		}
		if s.bodies[j].b.Kind == Kinematic {
			return j
		}
		b = &s.bodies[j]
	}
	return -1
}

// updateCarriers switches riders whose ground changed in the last substep
// between world and carrier-relative velocity: a rider that leaves a
// platform keeps the platform's velocity, one that lands on it has it
// removed (the carry adds it back as motion).
func (s *System) updateCarriers() {
	for i := range s.bodies {
		b := &s.bodies[i]
		nc := s.carrierOf(b)
		if nc == b.carrier {
			continue
		}
		v := *b.vel
		if b.carrier >= 0 {
			v = v.Add(*s.bodies[b.carrier].vel)
		}
		if nc >= 0 {
			v = v.Sub(*s.bodies[nc].vel)
		}
		*b.vel = lock(b.b, v)
		b.carrier = nc
	}
}

// integrateVelocities applies gravity, damping and the speed limit to the
// dynamic bodies (semi-implicit Euler: velocities first, positions after
// the contacts are resolved).
func (s *System) integrateVelocities(h float32) {
	for i := range s.bodies {
		b := &s.bodies[i]
		v := *b.vel
		if b.b.Kind == Dynamic {
			v = v.Add(s.gravity.Mul(b.b.GravityScale * h))
			v.X *= xmath.DampFactor(b.b.Damping.X, h)
			v.Y *= xmath.DampFactor(b.b.Damping.Y, h)
			v.X = xmath.ClampAbs(v.X, b.b.MaxSpeed.X)
			v.Y = xmath.ClampAbs(v.Y, b.b.MaxSpeed.Y)
		}
		*b.vel = lock(b.b, v)
	}
}

// integratePositions moves every body by its velocity, and riders also by
// their carrier's. MoveAndSlide movers that stand on a kinematic body, or
// on a dynamic body carried by one, move with it.
func (s *System) integratePositions(h float32) {
	for i := range s.bodies {
		b := &s.bodies[i]
		b.t.Pos = b.t.Pos.Add(lock(b.b, b.step(s, h)))
	}
	for _, r := range s.riders {
		cb := &s.bodies[r.carrier]
		r.t.Pos = r.t.Pos.Add(lock(cb.b, cb.step(s, h)))
	}
}

// step returns how far b moves in a substep of h: its velocity plus its
// carrier's.
func (b *body) step(s *System, h float32) gfx.Vec2 {
	d := b.vel.Mul(h)
	if b.carrier >= 0 {
		d = d.Add(s.bodies[b.carrier].vel.Mul(h))
	}
	return d
}

// finishTick writes Touching and keeps each body's ground for the next
// tick.
func (s *System) finishTick() {
	clear(s.rides)
	for i := range s.bodies {
		b := &s.bodies[i]
		if r := (ride{ground: b.fl.ground}); r.ground != (ecs.Entity{}) || b.carrier >= 0 {
			if b.carrier >= 0 {
				r.carrier = s.bodies[b.carrier].e
			}
			s.rides[b.e] = r
		}
		if t := b.touching; t != nil {
			*t = Touching{
				OnGround: b.fl.onGround, OnCeiling: b.fl.onCeiling, OnWall: b.fl.onWall,
				Ground: b.fl.ground, GroundNormal: b.fl.groundNormal,
			}
		}
	}
	s.impPrev, s.impSeen = s.impSeen, s.impPrev
	clear(s.impSeen)
}

// lock zeroes the locked axes of v.
func lock(b *Body, v gfx.Vec2) gfx.Vec2 {
	if b.LockX {
		v.X = 0
	}
	if b.LockY {
		v.Y = 0
	}
	return v
}
