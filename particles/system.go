package particles

import (
	"math"

	"github.com/weakpixel/asset"
	"github.com/weakpixel/ecs"
	"github.com/weakpixel/ecs/system"
	"github.com/weakpixel/engine/gfx"
)

// System simulates the particles of every entity that has a gfx.Transform
// and a gfx.Particles: it spawns, moves and removes particles, and writes the
// component's runtime fields (Pool, Time, Live, Done). Run it in the anim
// stage, after the AnimationSystem, so it sees the tick's final position and
// keeps playing while a Tool pauses the game.
//
// An entity whose effect is not loaded yet (or failed, or is the zero
// handle) has no particles. The component's override fields are applied on
// top of the effect every tick (see [Resolve]). When a hot reload or an
// override change alters an effect's emitter, the pools
// that play it start over if the cycle, the capacity or the spawn area
// changed (Duration, Loop, Burst, Max, Shape, ShapeSize), which live
// particles cannot follow; any other change applies to the live particles
// at once.
//
// The particles live in pools owned by the System, one per entity, with a
// lane for each emitter of its effect (see
// [Extract] for drawing them). A pool is released in the tick after its
// entity is deleted or loses the component, and its buffers are reused by
// the next emitter. Run does not allocate once the pools exist.
type System struct {
	view    ecs.View2[gfx.Transform, gfx.Particles]
	effects EffectStore
	// pools is indexed by Particles.Pool - 1. A free pool has the zero
	// owner, which is never a live entity.
	pools []*pool
	free  []int
}

// NewSystem returns the system for w. effects resolves the components'
// effect handles; take it from [EffectStoreOf].
func NewSystem(w *ecs.World, effects EffectStore) *System {
	return &System{view: ecs.NewView2[gfx.Transform, gfx.Particles](w), effects: effects}
}

// Run implements system.Runner. A [Clock] resource in the world, when
// present and not at its defaults, scales, pauses or seeks the simulation.
func (s *System) Run(ctx *system.Context) {
	dt := float32(ctx.Time.Delta.Seconds())
	if c := ecs.GetResourceOrNil[Clock](ctx.World); c != nil && !c.idle() {
		s.runClocked(c, dt)
		return
	}
	s.advance(dt, true)
}

// advance runs one tick of dt for every emitter. With step false it only
// keeps the pools alive (a paused simulation).
func (s *System) advance(dt float32, step bool) {
	for ent, comps := range s.view.Each() {
		t, c := comps.Unpack()
		eff, state := s.effects.Lookup(c.Effect)
		if state != asset.Ready || len(eff.Emitters) == 0 {
			continue // nothing to play; an unseen pool is released
		}
		n := len(eff.Emitters)
		p := s.poolOf(ent, c.Pool)
		switch {
		case p == nil:
			c.Pool, p = s.acquire(ent)
			s.restart(p, ent, c, eff)
		case len(p.lanes) != n:
			s.restart(p, ent, c, eff) // the file gained or lost emitters
		}
		p.seen = true
		if !step {
			continue
		}
		seed := seedOf(c, ent)
		live, done := 0, true
		for i, l := range p.lanes {
			if c.Time == 0 {
				l.time = 0 // the game rewound the effect to replay it
			}
			resolved := Resolve(&eff.Emitters[i], c)
			em := &resolved
			if key := cycleOf(em); l.cycle != key {
				l.cycle = key
				l.grow(capacityOf(em))
				l.reset(laneSeed(seed, i))
			}
			l.step(c.Stop, em, t, dt)
			live += len(l.ps)
			done = done && l.done
		}
		c.Time = p.lanes[0].time
		c.Live = uint16(min(live, math.MaxUint16))
		c.Done = done
	}
	s.sweep()
}

// runClocked runs one tick under a non-default clock.
func (s *System) runClocked(c *Clock, dt float32) {
	if c.seek {
		c.seek = false
		s.seekTo(c.Seek, dt)
		return
	}
	switch {
	case c.Paused && c.Step > 0:
		c.Step--
		s.advance(dt*c.scale(), true)
	case c.Paused:
		s.advance(0, false)
	default:
		s.advance(dt*c.scale(), true)
	}
}

// seekTo restarts every emitter and simulates sec seconds in steps of the
// tick's dt, so a seek equals the same number of normal ticks.
func (s *System) seekTo(sec, dt float32) {
	steps := 0
	if dt > 0 {
		steps = min(int(sec/dt+0.5), maxSeekSteps)
	}
	for ent, comps := range s.view.Each() {
		_, c := comps.Unpack()
		c.Time, c.Done = 0, false
		if p := s.poolOf(ent, c.Pool); p != nil {
			seed := seedOf(c, ent)
			for i, l := range p.lanes {
				l.reset(laneSeed(seed, i))
			}
		}
	}
	for range steps {
		s.advance(dt, true)
	}
	if steps == 0 {
		s.advance(0, true)
	}
}

// poolOf returns the live pool that entity ent owns under id, or nil. A
// copied id (a cloned entity), a stale one (a loaded level) and zero all
// return nil.
func (s *System) poolOf(ent ecs.Entity, id uint32) *pool {
	if id == 0 || int(id) > len(s.pools) {
		return nil
	}
	if p := s.pools[id-1]; p.owner == ent {
		return p
	}
	return nil
}

// restart starts the effect over: p gets one lane per emitter of eff, the
// cycles and the live particles go, and a lane grows if the emitter's Max
// needs it.
func (s *System) restart(p *pool, ent ecs.Entity, c *gfx.Particles, eff *Effect) {
	c.Time, c.Done = 0, false
	p.setLanes(len(eff.Emitters))
	seed := seedOf(c, ent)
	for i := range p.lanes {
		em := Resolve(&eff.Emitters[i], c)
		if p.lanes[i] == nil {
			p.lanes[i] = newLane(capacityOf(&em))
		}
		l := p.lanes[i]
		l.grow(capacityOf(&em))
		l.reset(laneSeed(seed, i))
		l.cycle = cycleOf(&em)
	}
}

// acquire gives ent a pool, recycling a released one when possible. The
// caller sets up its lanes with restart.
func (s *System) acquire(ent ecs.Entity) (uint32, *pool) {
	var idx int
	var p *pool
	if n := len(s.free); n > 0 {
		idx, s.free = s.free[n-1], s.free[:n-1]
		p = s.pools[idx]
	} else {
		idx = len(s.pools)
		p = &pool{}
		s.pools = append(s.pools, p)
	}
	p.owner = ent
	return uint32(idx + 1), p
}

// sweep releases the pools whose emitter was not seen this tick (mark and
// sweep, so no hook is needed for deleted entities or removed components)
// and clears the marks of the others.
func (s *System) sweep() {
	for i, p := range s.pools {
		switch {
		case p.owner == (ecs.Entity{}):
		case p.seen:
			p.seen = false
		default:
			p.owner = ecs.Entity{}
			for _, l := range p.lanes {
				l.ps = l.ps[:0]
			}
			s.free = append(s.free, i)
		}
	}
}

// Pools returns the number of pools (entities with particle state) in use, for tests and diagnostics.
func (s *System) Pools() int { return len(s.pools) - len(s.free) }
