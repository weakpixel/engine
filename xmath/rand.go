package xmath

import "math/rand/v2"

// Rand is a seeded random source for gameplay. There is no package-wide
// generator: a system owns a Rand, so the same seed replays the same game and
// tests are deterministic. A Rand is not safe for concurrent use.
type Rand struct {
	pcg *rand.PCG
	r   *rand.Rand
}

// NewRand returns a generator that always produces the same sequence for the
// same seed, e.g. the same dungeon for the same seed.
func NewRand(seed uint64) *Rand {
	pcg := rand.NewPCG(0, 0)
	g := &Rand{pcg: pcg, r: rand.New(pcg)}
	g.Seed(seed)
	return g
}

// Seed restarts the sequence as if r was made by NewRand(seed). It does not
// allocate, so a pool can reseed per spawn.
func (r *Rand) Seed(seed uint64) { r.pcg.Seed(seed, seed^0x9e3779b97f4a7c15) }

// Float returns a uniform number in [0, 1), e.g.
// if r.Float() < 0.1 { dropLoot() }.
func (r *Rand) Float() float32 { return r.r.Float32() }

// Range returns a uniform number in [lo, hi), e.g. a spawn delay of
// r.Range(0.5, 1.5) seconds.
func (r *Rand) Range(lo, hi float32) float32 { return lo + (hi-lo)*r.r.Float32() }

// IntRange returns a uniform integer in [lo, hi] (both included), e.g.
// damage := r.IntRange(3, 6). If hi < lo it returns lo.
func (r *Rand) IntRange(lo, hi int) int {
	if hi <= lo {
		return lo
	}
	return lo + r.r.IntN(hi-lo+1)
}

// Chance reports true with probability p, e.g. a 25 % crit: r.Chance(0.25).
// p <= 0 is never, p >= 1 always.
func (r *Rand) Chance(p float32) bool { return r.r.Float32() < p }

// Sign returns -1 or 1, e.g. the random initial direction of a bat.
func (r *Rand) Sign() float32 {
	if r.r.Uint32()&1 == 0 {
		return -1
	}
	return 1
}

// Angle returns a uniform angle in [0, Tau), e.g. for debris flying in any
// direction.
func (r *Rand) Angle() float32 { return r.r.Float32() * Tau }

// UnitVec returns a random unit vector, e.g. a scattered shotgun pellet.
func (r *Rand) UnitVec() Vec2 { return FromAngle(r.Angle()) }

// InCircle returns a uniform point inside the disc of the given radius around
// the origin, e.g. an explosion's sparks. (The distance is radius·√u, so the
// points do not bunch up in the middle.)
func (r *Rand) InCircle(radius float32) Vec2 {
	d := radius * Sqrt(r.r.Float32())
	return FromAngle(r.Angle()).Mul(d)
}

// OnCircle returns a random point on the ring of the given radius around the
// origin, e.g. to spawn enemies around the player.
func (r *Rand) OnCircle(radius float32) Vec2 { return r.UnitVec().Mul(radius) }

// InRect returns a uniform point in rc, e.g. rain drops across the screen.
func (r *Rand) InRect(rc Rect) Vec2 {
	return Vec2{r.Range(rc.Min.X, rc.Max.X), r.Range(rc.Min.Y, rc.Max.Y)}
}

// Spread returns center ± spread, uniformly, e.g. Spread(aim, Rad(5)) for
// weapon spread.
func (r *Rand) Spread(center, spread float32) float32 {
	return center + spread*(2*r.r.Float32()-1)
}

// Gauss returns a normally distributed number, e.g. natural-looking damage
// rolls. A negative stddev behaves like its absolute value.
func (r *Rand) Gauss(mean, stddev float32) float32 {
	return mean + stddev*float32(r.r.NormFloat64())
}

// Weighted returns an index chosen with probability proportional to its
// weight, e.g. a loot table of 70, 25 and 5. Negative weights count as 0.
// It returns -1 when no weight is positive.
func (r *Rand) Weighted(weights []float32) int {
	var sum float32
	for _, w := range weights {
		sum += max(w, 0)
	}
	if sum <= 0 {
		return -1
	}
	x := r.r.Float32() * sum
	last := -1
	for i, w := range weights {
		if w <= 0 {
			continue
		}
		last = i
		if x < w {
			return i
		}
		x -= w
	}
	return last // rounding left x at the very end
}

// Pick returns a random element of s, e.g. a footstep sound from a set. An
// empty s gives the zero value.
func Pick[T any](r *Rand, s []T) T {
	if len(s) == 0 {
		var zero T
		return zero
	}
	return s[r.r.IntN(len(s))]
}

// Shuffle reorders s in place, e.g. a deck of cards or a bag of tetrominoes.
func Shuffle[T any](r *Rand, s []T) {
	r.r.Shuffle(len(s), func(i, j int) { s[i], s[j] = s[j], s[i] })
}
