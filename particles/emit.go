package particles

import (
	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/particles/fxfile"
	"github.com/weakpixel/engine/xmath"
)

// minLife keeps a particle's lifetime above zero, so Age/Life is defined.
const minLife = 0.001

// step advances one emitter by dt seconds: it moves the live particles,
// removes the dead ones and spawns new ones. stop is the component's Stop, e
// the resolved emitter and t the entity's Transform. It updates l.time and
// l.done.
//
// Existing particles move first, so a particle spawned this tick shows at
// its spawn position and starts moving on the next tick.
func (p *lane) step(stop bool, e *fxfile.Emitter, t *gfx.Transform, dt float32) {
	p.simulate(e, dt)
	f := frameOf(t)

	// Cycle: a looping emitter restarts when Duration has elapsed; time == 0
	// (a fresh emitter, or one the game rewound to replay) starts a new burst.
	if e.Loop && e.Duration > 0 && p.time >= e.Duration {
		p.time = 0
	}
	if p.time == 0 {
		p.bursted = false
	}
	active := !stop && (e.Duration <= 0 || p.time < e.Duration)
	if active {
		if !p.bursted {
			p.bursted = true
			for range e.Burst {
				p.spawn(e, &f)
			}
		}
		// Rate is accumulated, not rounded: 0.5 particles per tick spawns one
		// every other tick.
		if e.Rate > 0 {
			p.carry += e.Rate * dt
			n := int(p.carry)
			p.carry -= float32(n)
			for range n {
				p.spawn(e, &f)
			}
		}
		p.time += dt
	}
	p.done = !active && len(p.ps) == 0
}

// simulate ages and moves every particle with explicit Euler integration and
// swap-removes the ones whose life is over.
func (p *lane) simulate(e *fxfile.Emitter, dt float32) {
	drag := max(0, 1-e.Drag*dt)
	for i := 0; i < len(p.ps); {
		q := &p.ps[i]
		q.Age += dt
		if q.Age >= q.Life {
			last := len(p.ps) - 1
			*q = p.ps[last]
			p.ps = p.ps[:last]
			continue // the swapped-in particle has not been moved yet
		}
		q.Vel.X = (q.Vel.X + e.Gravity.X*dt) * drag
		q.Vel.Y = (q.Vel.Y + e.Gravity.Y*dt) * drag
		q.PrevPos = q.Pos
		q.Pos.X += q.Vel.X * dt
		q.Pos.Y += q.Vel.Y * dt
		q.Rot += q.Spin * dt
		i++
	}
}

// frame is the emitter's Transform as spawn needs it. The sine and cosine
// of Rot are computed once per tick, not once per particle.
type frame struct {
	origin   gfx.Vec2
	rot      float32
	sin, cos float32
	scale    gfx.Vec2
}

// frameOf returns the spawn frame of t, resolving the "zero Scale means
// (1, 1)" rule of gfx.Transform.
func frameOf(t *gfx.Transform) frame {
	sin, cos := xmath.Sincos(t.Rot)
	f := frame{origin: t.Pos, rot: t.Rot, sin: sin, cos: cos, scale: t.Scale}
	if f.scale == (gfx.Vec2{}) {
		f.scale = gfx.Vec2{X: 1, Y: 1}
	}
	return f
}

// place maps a point of the emit shape to the emitter's frame: scaled by
// Scale, then rotated by Rot, as the renderer does for a sprite. It does not
// add the origin.
func (f *frame) place(v gfx.Vec2) gfx.Vec2 {
	x, y := v.X*f.scale.X, v.Y*f.scale.Y
	return gfx.Vec2{X: x*f.cos - y*f.sin, Y: x*f.sin + y*f.cos}
}

// spawn adds one particle, unless the pool is full. The emitter's rotation
// turns the emission direction, the spawn shape and the particle's start
// angle; its scale stretches the spawn shape and the particle size. Both
// apply at spawn only: a live particle keeps its direction and size when
// the emitter turns or scales later.
func (p *lane) spawn(e *fxfile.Emitter, f *frame) {
	if len(p.ps) >= capacityOf(e) {
		return // dropped, not queued
	}
	r := p.rng
	pos := f.place(p.spawnOffset(e))
	if !e.Local {
		pos = pos.Add(f.origin)
	}
	life := lifeOf(e, r.Float())
	speed := between(r.Float(), e.SpeedMin, e.SpeedMax)
	dir := f.rot + e.Angle + (r.Float()-0.5)*e.Spread
	q := particle{
		Pos: pos, PrevPos: pos,
		Vel:  xmath.FromAngle(dir).Mul(speed),
		Life: life,
		Rot:  f.rot,
		Spin: between(r.Float(), e.SpinMin, e.SpinMax),
		Size: f.scale,
	}
	if e.RandomRotation {
		q.Rot += r.Angle()
	}
	p.ps = append(p.ps, q)
}

// spawnOffset returns a random point of the emit shape around the emitter.
func (p *lane) spawnOffset(e *fxfile.Emitter) gfx.Vec2 {
	switch e.Shape {
	case gfx.EmitCircle:
		// InCircle is uniform over the disc, not clustered at the center. It
		// draws the radius first and the angle second.
		return p.rng.InCircle(e.ShapeSize.X)
	case gfx.EmitRect:
		return gfx.Vec2{
			X: (p.rng.Float() - 0.5) * e.ShapeSize.X,
			Y: (p.rng.Float() - 0.5) * e.ShapeSize.Y,
		}
	}
	return gfx.Vec2{}
}

// lifeOf maps t in [0,1) to a lifetime: LifeMin to LifeMax, where a zero
// LifeMax or one below LifeMin means LifeMin, and no lifetimes at all means
// 1 s.
func lifeOf(e *fxfile.Emitter, t float32) float32 {
	if e.LifeMin <= 0 && e.LifeMax <= 0 {
		return 1
	}
	return max(minLife, between(t, e.LifeMin, e.LifeMax))
}

// between maps t in [0,1) to [lo, hi]; hi below lo means lo.
func between(t, lo, hi float32) float32 {
	if hi <= lo {
		return lo
	}
	return lo + (hi-lo)*t
}

// scaleAt returns the particle size at t = Age/Life. Zero Scale means 1 and
// zero EndScale means Scale.
func scaleAt(e *fxfile.Emitter, t float32) float32 {
	start := e.Scale
	if start == 0 {
		start = 1
	}
	end := e.EndScale
	if end == 0 {
		end = start
	}
	return start + (end-start)*t
}

// colorAt returns the particle tint at t = Age/Life: Color blended to
// EndColor, with the alpha ramped by FadeIn and FadeOut. ok is false when
// the particle is fully transparent and need not be drawn.
func colorAt(e *fxfile.Emitter, t float32) (c gfx.Color, ok bool) {
	r0, g0, b0, a0 := e.Color.Floats()
	r1, g1, b1, a1 := r0, g0, b0, a0
	if e.EndColor != 0 {
		r1, g1, b1, a1 = e.EndColor.Floats()
	}
	a := a0 + (a1-a0)*t
	if e.FadeIn > 0 && t < e.FadeIn {
		a *= t / e.FadeIn
	}
	if e.FadeOut > 0 && t > 1-e.FadeOut {
		a *= (1 - t) / e.FadeOut
	}
	if a < 1.0/512 {
		return 0, false
	}
	c = gfx.RGBA(unit8(r0+(r1-r0)*t), unit8(g0+(g1-g0)*t), unit8(b0+(b1-b0)*t), unit8(a))
	return c, true
}

// unit8 converts a [0,1] channel to a byte, clamped.
func unit8(f float32) uint8 {
	return uint8(min(max(f, 0), 1)*255 + 0.5)
}
