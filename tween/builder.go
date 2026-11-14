package tween

import "fmt"

// Builder sets the options of a tween just started with [To] or [From]. It
// is a small value (the Tweens and the ID), so chaining allocates nothing.
// Each option writes straight into the tween, so their order does not
// matter. On a tween that has finished or was killed every option is a
// no-op.
//
// Relative and Additive are read when the tween starts, so set them right
// after To or From.
type Builder struct {
	tw *Tweens
	id ID
}

// ID returns the tween's handle, for [Tweens.Kill], [Sequence] and the other
// control functions.
func (b Builder) ID() ID { return b.id }

func (b Builder) state() *common {
	if b.tw == nil {
		return nil
	}
	return b.tw.lookup(b.id)
}

// Ease sets the easing curve; the default is Linear.
func (b Builder) Ease(e Ease) Builder {
	if c := b.state(); c != nil {
		c.ease = e
	}
	return b
}

// EaseFunc sets a custom easing curve, used instead of Ease. f maps the
// time fraction in (0, 1) to the progress; it should return 0 at 0 and 1 at
// 1, but the last cycle writes the exact end value either way. Storing f
// costs nothing per tick; a closure that captures variables allocates once,
// when it is created.
func (b Builder) EaseFunc(f func(t float32) float32) Builder {
	if c := b.state(); c != nil {
		c.easeFn = f
	}
	return b
}

// Delay waits seconds (scaled by Speed) before the tween starts and reads
// its start value. A negative delay counts as 0.
func (b Builder) Delay(seconds float32) Builder {
	if c := b.state(); c != nil {
		c.delay = max(seconds, 0)
	}
	return b
}

// Speed scales the tween's clock, delay included: 2 runs twice as fast, 0
// freezes it.
//
// Panics with "tween: negative speed: <f>" if f < 0.
func (b Builder) Speed(f float32) Builder {
	if f < 0 {
		panic(fmt.Sprintf("tween: negative speed: %v", f))
	}
	if c := b.state(); c != nil {
		c.speed = f
	}
	return b
}

// Repeat runs the cycle n more times; a negative n repeats forever. A tween
// that repeats forever never finishes by itself.
func (b Builder) Repeat(n int) Builder {
	if c := b.state(); c != nil {
		c.repeat = int32(max(n, -1))
	}
	return b
}

// Yoyo runs every second cycle backwards, from the end value to the start
// value, with the ease mirrored. Use it with Repeat: Repeat(1).Yoyo() goes
// there and back once.
func (b Builder) Yoyo() Builder {
	if c := b.state(); c != nil {
		c.yoyo = true
	}
	return b
}

// Relative makes the To (or From) value an offset from the start value:
// To(tw, e, Pos, gfx.Vec2{X: 16}, 0.2).Relative() moves 16 to the right.
// Meant for numeric values (float32, gfx.Vec2, [4]float32), not colors.
func (b Builder) Relative() Builder {
	if c := b.state(); c != nil {
		c.relative = true
	}
	return b
}

// Additive makes the tween write its change each tick instead of its value,
// and exempts it from the overwrite rule. Additive tweens on one field stack
// with each other and with game code that moves the field (a shake on top of
// a move or of physics). A non-additive tween on the same field still writes
// absolute values and wins while it runs. Meant for numeric values.
func (b Builder) Additive() Builder {
	if c := b.state(); c != nil {
		c.additive = true
	}
	return b
}

// DespawnOnDone deletes the entity (through system.Commands) when the tween
// finishes by itself or through Complete. A killed tween despawns nothing.
func (b Builder) DespawnOnDone() Builder {
	if c := b.state(); c != nil {
		c.despawn = true
	}
	return b
}

// OnDone sets a callback that runs when the tween finishes by itself or
// through Complete, after every pool has run for the tick, so it may start
// and kill tweens. It does not run for killed tweens or dead targets. A
// closure that captures variables allocates once, when it is created.
func (b Builder) OnDone(f func()) Builder {
	if c := b.state(); c != nil {
		c.onDone = f
	}
	return b
}

// Then holds next until this tween finishes, then lets it start; killing
// this tween kills next. [Sequence] chains several. A stale next, or next
// equal to this tween, is ignored.
func (b Builder) Then(next ID) Builder {
	c := b.state()
	if c == nil || next == b.id {
		return b
	}
	n := b.tw.lookup(next)
	if n == nil {
		return b
	}
	c.next = next
	n.held = true
	return b
}
