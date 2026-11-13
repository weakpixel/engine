package tween

import (
	"github.com/weakpixel/ecs"
	"github.com/weakpixel/ecs/system"
)

// Tweens owns every running tween of a World, in one pool per [Lens]. The
// game.Engine creates one and publishes it as a resource:
//
//	tw := ecs.GetResource[tween.Tweens](ctx.World)
//
// Start tweens with [To] and [From]; [System] advances them. Like the World,
// a Tweens is single-goroutine.
type Tweens struct {
	world *ecs.World
	cmds  *system.Commands
	// pools is indexed by Lens.id; order lists the same pools in creation
	// order, which is the order they run in.
	pools []runner
	order []runner
	slots slots
	// active maps an entity and lens to its non-additive tween, for the
	// overwrite rule. It changes only when tweens start or end.
	active map[activeKey]ID
	// done holds the tweens that finished during the current Update (or
	// Complete). Their OnDone callbacks run from it after the pools, so a
	// callback can start and kill tweens. The slice is reused.
	done     []finished
	flushing bool
}

type activeKey struct {
	entity ecs.Entity
	lens   uint32
}

// finished is what runs after a tween ends: the despawn, the next tween of
// its sequence and its callback.
type finished struct {
	entity  ecs.Entity
	next    ID
	onDone  func()
	despawn bool
}

// New returns an empty Tweens for w. DespawnOnDone queues on w's
// system.Commands, the buffer the schedule applies after every stage.
func New(w *ecs.World) *Tweens {
	if w == nil {
		panic("tween: nil world")
	}
	return &Tweens{world: w, cmds: system.CommandsOf(w), active: map[activeKey]ID{}}
}

// To starts a tween of the field lens selects on e, from its current value
// to to, over seconds (one cycle). The start value is read when the tween
// actually starts, after its delay and after the previous tween of its
// sequence. Chain options on the returned Builder:
//
//	tween.To(tw, e, tween.Pos, target, 0.4).Ease(tween.OutBack)
//
// Overwrite: when the tween starts, it kills a running tween on the same
// entity and lens (unless either is [Builder.Additive]), so a new "move to"
// never fights an old one.
//
// To is a function, not a method, because Go methods cannot have type
// parameters. A zero or negative seconds jumps to the end value when the
// tween starts. A dead entity, or one without C, never panics: the tween is
// dropped in the next tick. Once the lens's pool has grown, To allocates
// nothing.
//
// Panics with "tween: nil lens" if lens is nil.
func To[C, V any](tw *Tweens, e ecs.Entity, lens *Lens[C, V], to V, seconds float32) Builder {
	return Builder{tw: tw, id: poolFor(tw, lens).add(tw, e, to, seconds, false)}
}

// From is [To] in reverse: it tweens from from to the field's current value,
// read when the tween starts.
func From[C, V any](tw *Tweens, e ecs.Entity, lens *Lens[C, V], from V, seconds float32) Builder {
	return Builder{tw: tw, id: poolFor(tw, lens).add(tw, e, from, seconds, true)}
}

// poolFor returns lens's pool, creating it on first use.
func poolFor[C, V any](tw *Tweens, lens *Lens[C, V]) *pool[C, V] {
	if lens == nil {
		panic("tween: nil lens")
	}
	if int(lens.id) < len(tw.pools) {
		if r := tw.pools[lens.id]; r != nil {
			// Asserting to the concrete pointer type does not allocate.
			return r.(*pool[C, V])
		}
	} else {
		tw.pools = append(tw.pools, make([]runner, int(lens.id)+1-len(tw.pools))...)
	}
	p := &pool[C, V]{lens: lens}
	tw.pools[lens.id] = p
	tw.order = append(tw.order, p)
	return p
}

// Sequence runs the tweens one after another: each one after the first is
// held until the previous one finishes (see [Builder.Then]). It returns the
// first ID. To run tweens in parallel, just start them.
func Sequence(tw *Tweens, ids ...ID) ID {
	if len(ids) == 0 {
		return ID{}
	}
	for i := 1; i < len(ids); i++ {
		Builder{tw: tw, id: ids[i-1]}.Then(ids[i])
	}
	return ids[0]
}

// Update advances every tween by dt seconds, then runs the OnDone callbacks
// of the tweens that finished. [System] calls it once per tick; call it
// yourself only when driving a Tweens without the System. 0 allocations
// once the pools have grown (callbacks aside).
func (tw *Tweens) Update(dt float32) {
	for _, p := range tw.order {
		p.run(tw, dt)
	}
	tw.flush()
}

// Kill stops the tween where it is, without writing its end value or
// running OnDone. The tweens after it in a sequence are killed too. A stale
// id is a no-op.
func (tw *Tweens) Kill(id ID) {
	for c := tw.lookup(id); c != nil; c = tw.lookup(id) {
		id = c.next
		tw.retire(c)
	}
}

// Complete writes the tween's end value and finishes it now: its OnDone
// runs, the next tween of its sequence is released and DespawnOnDone is
// queued. A tween that has not started yet reads its start value first. The
// end value is the one the tween would rest on when finishing by itself;
// for one that repeats forever it is the To value. A stale id is a no-op.
func (tw *Tweens) Complete(id ID) {
	sl := tw.slots.get(id)
	if sl == nil {
		return
	}
	tw.pools[sl.lens].complete(tw, sl.row)
	tw.flush()
}

// Pause pauses (on) or resumes the tween. A paused tween keeps its place and
// its delay. A stale id is a no-op.
func (tw *Tweens) Pause(id ID, on bool) {
	if c := tw.lookup(id); c != nil {
		c.paused = on
	}
}

// Running reports whether id is a tween that has not finished or been
// killed. A tween in its delay, paused or waiting in a sequence is running.
func (tw *Tweens) Running(id ID) bool { return tw.lookup(id) != nil }

// KillEntity kills every tween on e (and the rest of their sequences).
func (tw *Tweens) KillEntity(e ecs.Entity) {
	for _, p := range tw.order {
		p.each(func(c *common) {
			if c.entity == e {
				tw.Kill(c.id)
			}
		})
	}
}

// KillLens kills every tween of lens on e (and the rest of their
// sequences), additive ones included.
func KillLens[C, V any](tw *Tweens, e ecs.Entity, lens *Lens[C, V]) {
	poolFor(tw, lens).each(func(c *common) {
		if c.entity == e {
			tw.Kill(c.id)
		}
	})
}

// Clear drops every tween without writing values or running callbacks. The
// editor calls it when Play stops. Every ID becomes stale.
func (tw *Tweens) Clear() {
	for _, p := range tw.order {
		p.clear()
	}
	tw.slots.clear()
	clear(tw.active)
	clear(tw.done)
	tw.done = tw.done[:0]
}

// Len returns the number of running tweens, for tests and diagnostics.
func (tw *Tweens) Len() int { return tw.slots.live() }

// lookup returns the state of a running tween, or nil for a stale id.
func (tw *Tweens) lookup(id ID) *common {
	sl := tw.slots.get(id)
	if sl == nil {
		return nil
	}
	return tw.pools[sl.lens].at(sl.row)
}

// claim registers c as the tween of its entity and lens and kills the one it
// replaces (the overwrite rule).
func (tw *Tweens) claim(c *common, lens uint32) {
	key := activeKey{c.entity, lens}
	if old, ok := tw.active[key]; ok && old != c.id {
		tw.Kill(old)
	}
	tw.active[key] = c.id
}

// retire marks c dead and frees its ID; the pool removes the row in its next
// run.
func (tw *Tweens) retire(c *common) {
	key := activeKey{c.entity, tw.slots.s[c.id.index].lens}
	if id, ok := tw.active[key]; ok && id == c.id {
		delete(tw.active, key)
	}
	tw.slots.release(c.id)
	c.dead = true
}

// drop ends a tween whose target is gone: like Kill, no value and no
// callback.
func (tw *Tweens) drop(id ID) { tw.Kill(id) }

// finish ends c normally and queues what runs after it.
func (tw *Tweens) finish(c *common) {
	tw.done = append(tw.done, finished{entity: c.entity, next: c.next, onDone: c.onDone, despawn: c.despawn})
	tw.retire(c)
}

// flush runs what the finished tweens queued. Callbacks may finish more
// tweens (Complete); those are appended and handled by the same loop.
func (tw *Tweens) flush() {
	if tw.flushing {
		return
	}
	tw.flushing = true
	defer func() { tw.flushing = false }()
	// Index loop, not range: callbacks may append to tw.done.
	for i := 0; i < len(tw.done); i++ {
		f := tw.done[i]
		tw.done[i] = finished{} // do not keep the callback alive
		if f.despawn {
			tw.cmds.Despawn(f.entity)
		}
		if c := tw.lookup(f.next); c != nil {
			c.held = false
		}
		if f.onDone != nil {
			f.onDone()
		}
	}
	tw.done = tw.done[:0]
}
