package particles

import (
	"github.com/weakpixel/ecs"
	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/particles/fxfile"
	"github.com/weakpixel/engine/xmath"
)

// defaultMax is the pool capacity of an emitter whose Max is zero.
const defaultMax = 256

// particle is one live particle. Pos and PrevPos are world positions, or
// offsets from the emitter when the emitter is Local.
type particle struct {
	Pos, PrevPos gfx.Vec2
	Vel          gfx.Vec2
	Age, Life    float32  // seconds
	Rot, Spin    float32  // radians, radians per second
	Size         gfx.Vec2 // the emitter's Transform.Scale at spawn
}

// pool is the particle state of one entity: one lane per emitter of its
// effect.
type pool struct {
	// owner is the entity the pool belongs to. A copied gfx.Particles.Pool
	// id (a cloned entity) fails the owner check and gets a pool of its own.
	owner ecs.Entity
	// seen is set by the System each tick the owner has a ready effect; pools
	// left unseen are released.
	seen bool
	// lanes[:n] are the lanes in use. The slots beyond n keep their buffers,
	// so an effect that gains an emitter on a hot reload reuses them.
	lanes []*lane
}

// lane holds the live particles of one emitter and its cycle.
type lane struct {
	// cycle is the part of the resolved emitter a live lane cannot follow.
	// The System compares it every tick, so a hot reload or an override
	// change of a cycle key restarts the pool.
	cycle cycleKey
	// time is how long the current cycle has run, in seconds.
	time float32
	// done is set when the emitter has stopped and its particles are gone.
	done bool
	// bursted is set once the Burst of the current cycle has spawned.
	bursted bool
	// carry is the fractional particle left over from the Rate accumulator.
	carry float32

	// ps holds the live particles. It is allocated once at the lane's
	// capacity and only shrinks and grows in place, so a tick appends
	// nothing to the heap; dead particles leave by swap-remove (order is
	// irrelevant, the renderer sorts).
	ps []particle

	// rng is reseeded in place when a released pool is recycled.
	rng *xmath.Rand
}

func newLane(capacity int) *lane {
	return &lane{ps: make([]particle, 0, capacity), rng: xmath.NewRand(0)}
}

// reset starts the lane's cycle over with the given seed.
func (l *lane) reset(seed uint64) {
	l.time = 0
	l.done = false
	l.bursted = false
	l.carry = 0
	l.ps = l.ps[:0]
	l.rng.Seed(seed)
}

// grow makes room for capacity particles, keeping none.
func (l *lane) grow(capacity int) {
	if cap(l.ps) < capacity {
		l.ps = make([]particle, 0, capacity)
	}
}

// laneSeed derives the seed of emitter i from the pool's seed, so equal
// emitters do not spawn identical particles and a fixed Seed still replays
// the same.
func laneSeed(seed uint64, i int) uint64 {
	return seed + uint64(i)*0x9e3779b97f4a7c15
}

// seedOf returns the random seed of a particle component: Seed, or a value
// derived from the entity.
func seedOf(c *gfx.Particles, owner ecs.Entity) uint64 {
	if c.Seed != 0 {
		return uint64(c.Seed)
	}
	return uint64(owner.ID)<<32 | uint64(owner.Generation) | 1<<63
}

// capacityOf returns the pool capacity of an emitter: its Max, or the default.
func capacityOf(e *fxfile.Emitter) int {
	if e.Max == 0 {
		return defaultMax
	}
	return int(e.Max)
}

// cycleKey is the part of an emitter that a hot reload cannot apply to live
// particles: the cycle, the pool capacity and the spawn area. The System
// restarts a pool when it changes; any other change shows at once.
type cycleKey struct {
	Duration  float32
	Loop      bool
	Burst     uint16
	Max       uint16
	Shape     gfx.EmitShape
	ShapeSize gfx.Vec2
}

func cycleOf(e *fxfile.Emitter) cycleKey {
	return cycleKey{e.Duration, e.Loop, e.Burst, e.Max, e.Shape, e.ShapeSize}
}

// setLanes makes p hold n lanes. A slot that was in use before keeps its
// buffer; a new one is nil until the caller fills it.
func (p *pool) setLanes(n int) {
	for len(p.lanes) > n {
		p.lanes = p.lanes[:len(p.lanes)-1]
	}
	for len(p.lanes) < n {
		i := len(p.lanes)
		if i < cap(p.lanes) && p.lanes[:i+1][i] != nil {
			p.lanes = p.lanes[:i+1]
			continue
		}
		p.lanes = append(p.lanes[:i], nil)
	}
}
