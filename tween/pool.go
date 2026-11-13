package tween

import (
	"github.com/weakpixel/ecs"
)

// common is the part of a tween that does not depend on the value type, so
// the untyped Builder and the control functions can reach it through the
// runner interface.
type common struct {
	id     ID
	entity ecs.Entity
	next   ID // started when this one finishes (Then, Sequence)

	time     float32 // seconds into the delay, then into the current cycle
	duration float32
	delay    float32
	speed    float32
	repeat   int32 // extra cycles; -1 repeats forever
	cycle    int32 // index of the current cycle

	ease   Ease
	easeFn func(float32) float32
	onDone func()

	isFrom   bool // started by From: arg is the start value
	yoyo     bool
	relative bool
	additive bool
	despawn  bool
	paused   bool
	held     bool // waits for the previous tween of a sequence
	started  bool // the delay is over and the start value was read
	dead     bool // finished or killed; the row is removed by the next run
}

// eased returns the eased fraction of the current cycle.
func (c *common) eased(t float32) float32 {
	if c.yoyo && c.cycle%2 == 1 {
		t = 1 - t
	}
	if c.easeFn != nil && t > 0 && t < 1 {
		return c.easeFn(t)
	}
	return c.ease.At(t)
}

// endsAtFrom reports whether the last cycle runs backwards, so a finished
// tween rests on its start value (a yoyo with an odd Repeat count).
func (c *common) endsAtFrom() bool {
	return c.yoyo && c.repeat > 0 && c.repeat%2 == 1
}

// track is one tween of a pool.
type track[V any] struct {
	common
	arg      V // the value passed to To or From
	from, to V // set when the tween starts
	prev     V // last value written by an additive tween
}

// runner is what Tweens needs of a pool without knowing its types.
type runner interface {
	// run advances every tween by dt seconds. Finished tweens are queued on
	// tw.done; their callbacks run after every pool has run.
	run(tw *Tweens, dt float32)
	at(row int32) *common
	// complete writes the end value of the tween at row and finishes it.
	complete(tw *Tweens, row int32)
	// each calls f for every tween that is not dead.
	each(f func(*common))
	clear()
	len() int
}

// pool holds the tweens of one lens. Rows are removed by swap-remove, which
// keeps the slice dense and the tick a plain loop; the slot table is updated
// for the row that moves.
type pool[C, V any] struct {
	lens *Lens[C, V]
	rows []track[V]
}

func (p *pool[C, V]) at(row int32) *common { return &p.rows[row].common }

func (p *pool[C, V]) len() int { return len(p.rows) }

func (p *pool[C, V]) clear() {
	clear(p.rows) // drop OnDone closures and ease functions
	p.rows = p.rows[:0]
}

func (p *pool[C, V]) each(f func(*common)) {
	for i := range p.rows {
		if !p.rows[i].dead {
			f(&p.rows[i].common)
		}
	}
}

func (p *pool[C, V]) add(tw *Tweens, e ecs.Entity, arg V, seconds float32, from bool) ID {
	row := int32(len(p.rows))
	id := tw.slots.alloc(p.lens.id, row)
	p.rows = append(p.rows, track[V]{
		common: common{id: id, entity: e, duration: seconds, speed: 1, isFrom: from},
		arg:    arg,
	})
	return id
}

// remove swap-removes row i. The row is dead, so its slot is already free;
// the row moved into its place gets its slot updated unless it is dead too
// (its slot may then belong to another tween already).
func (p *pool[C, V]) remove(tw *Tweens, i int) {
	last := len(p.rows) - 1
	if i != last {
		p.rows[i] = p.rows[last]
		if r := &p.rows[i]; !r.dead {
			tw.slots.s[r.id.index].row = int32(i)
		}
	}
	p.rows[last] = track[V]{} // drop references held by the removed row
	p.rows = p.rows[:last]
}

func (p *pool[C, V]) run(tw *Tweens, dt float32) {
	w := tw.world
	// Index loop: remove swaps the last row into i, which is then looked
	// at again.
	for i := 0; i < len(p.rows); {
		r := &p.rows[i]
		switch {
		case r.dead:
			p.remove(tw, i)
			continue
		case !w.IsAlive(r.entity):
			tw.drop(r.id)
			p.remove(tw, i)
			continue
		case r.paused || r.held:
			i++
			continue
		}
		r.time += dt * r.speed
		if !r.started && r.time < r.delay {
			i++
			continue
		}
		c := ecs.GetCompOrNil[C](w, r.entity)
		if c == nil {
			tw.drop(r.id)
			p.remove(tw, i)
			continue
		}
		field := p.lens.get(c)
		if !r.started {
			r.time -= r.delay
			p.start(tw, r, field)
			if r.dead {
				// Claiming the field killed this tween's own sequence
				// (a cycle of Then); it must not write.
				p.remove(tw, i)
				continue
			}
		}
		if p.advance(r) {
			p.write(r, field, p.end(r))
			tw.finish(&r.common)
			p.remove(tw, i)
			continue
		}
		p.write(r, field, p.lens.lerp(r.from, r.to, r.eased(r.time/r.duration)))
		i++
	}
}

// start reads the start value and claims the field (see Tweens.claim).
func (p *pool[C, V]) start(tw *Tweens, r *track[V], field *V) {
	v := *field
	if p.lens.norm != nil {
		v = p.lens.norm(v)
	}
	arg := r.arg
	if r.relative {
		var zero V
		arg = p.lens.add(v, arg, zero)
	}
	if r.isFrom {
		r.from, r.to = arg, v
	} else {
		r.from, r.to = v, arg
	}
	r.prev = v
	r.started = true
	if !r.additive {
		tw.claim(&r.common, p.lens.id)
	}
}

// advance moves r into the cycle r.time falls in and reports whether the
// last cycle is over. A zero duration is over at once.
func (p *pool[C, V]) advance(r *track[V]) bool {
	if r.duration <= 0 {
		return true
	}
	if r.time < r.duration {
		return false
	}
	// Whole cycles passed this tick (usually one). Subtracting them keeps
	// r.time small, so a tween that repeats forever does not lose float
	// precision over time.
	n := int32(r.time / r.duration)
	if r.repeat >= 0 && r.cycle+n > r.repeat {
		return true
	}
	r.cycle += n
	r.time -= float32(n) * r.duration
	return false
}

func (p *pool[C, V]) end(r *track[V]) V {
	if r.endsAtFrom() {
		return r.from
	}
	return r.to
}

// write stores v in the field: as is, or for an additive tween as the change
// since its last write, so additive tweens stack with each other and with
// game code that moves the field.
func (p *pool[C, V]) write(r *track[V], field *V, v V) {
	if r.additive {
		*field = p.lens.add(*field, v, r.prev)
		r.prev = v
		return
	}
	*field = v
}

func (p *pool[C, V]) complete(tw *Tweens, row int32) {
	r := &p.rows[row]
	var c *C
	if tw.world.IsAlive(r.entity) {
		c = ecs.GetCompOrNil[C](tw.world, r.entity)
	}
	if c == nil {
		tw.drop(r.id)
		return
	}
	field := p.lens.get(c)
	if !r.started {
		p.start(tw, r, field)
		if r.dead {
			return
		}
	}
	p.write(r, field, p.end(r))
	tw.finish(&r.common)
}
