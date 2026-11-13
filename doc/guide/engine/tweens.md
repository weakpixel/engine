# Tweens

Package `tween` animates component fields over time: "move this enemy to (120, 40) with a little overshoot", "pulse this coin forever", "fade this sprite out, then delete it". One call starts a tween, the engine runs it, and an `ID` controls it.

Tweens are not components. A `tween.Tweens` resource owns every running tween, in one typed pool per field, so starting or ending a tween never moves an entity between archetypes and any number of tweens can run on one entity. They are runtime behavior started by code; levels, prefabs and the editor's inspector do not see them.

- [Starting a tween](#starting-a-tween)
- [Options](#options)
- [Easing](#easing)
- [Sequences](#sequences)
- [Control](#control)
- [Overwrite and Additive](#overwrite-and-additive)
- [Your own fields](#your-own-fields)
- [Callbacks and their cost](#callbacks-and-their-cost)
- [Dead targets](#dead-targets)
- [Pause and the editor](#pause-and-the-editor)
- [Performance](#performance)
- [Panics](#panics)

## Starting a tween

`game.Engine` creates the `Tweens`, publishes it as a World resource and returns it from `Engine.Tweens()`. A system gets it from the World:

```go
func hitSystem(ctx *system.Context) {
	tw := ecs.GetResource[tween.Tweens](ctx.World)
	tween.To(tw, enemy, tween.Pos, gfx.Vec2{X: 120, Y: 40}, 0.4).Ease(tween.OutBack)
}
```

`To(tw, e, lens, to, seconds)` tweens the field `lens` selects on entity `e` from its current value to `to`, over `seconds`. `From(tw, e, lens, from, seconds)` goes the other way: from `from` to the current value. Both read the current value when the tween actually starts, after its delay and after the previous tween of its sequence, so game code may move the entity until then. A zero or negative `seconds` jumps to the end value when the tween starts.

`To` and `From` are functions, not methods, because Go methods cannot have type parameters. The value type is checked at compile time: `tween.To(tw, e, tween.Pos, 3, 1)` does not compile, since `Pos` animates a `gfx.Vec2`.

The built-in lenses:

| Lens | Field | Value |
| --- | --- | --- |
| `tween.Pos` | `gfx.Transform.Pos` | `gfx.Vec2` |
| `tween.Scale` | `gfx.Transform.Scale` | `gfx.Vec2`; a zero `Scale` counts as (1, 1), as when drawing |
| `tween.Rot` | `gfx.Transform.Rot` | `float32`, radians |
| `tween.Tint` | `gfx.Sprite.Tint` | `gfx.Color`; the zero color counts as opaque white |
| `tween.Params` | `gfx.Sprite.Params` | `[4]float32` |
| `tween.Param(i)` | `gfx.Sprite.Params[i]` | `float32`, one shader parameter |

Fade a sprite out by tweening `Tint` to a color with zero alpha, such as `gfx.RGBA(255, 255, 255, 0)`. Drive a shader parameter with `Param(i)`; tweens on different parameters of one sprite do not overwrite each other.

## Options

`To` and `From` return a `tween.Builder`. Chain options on it; each writes straight into the tween, so their order does not matter.

| Option | Effect |
| --- | --- |
| `Ease(e)` | The easing curve ([Easing](#easing)); default `Linear` |
| `EaseFunc(f)` | A custom curve `func(t float32) float32`, used instead of `Ease` |
| `Delay(s)` | Wait `s` seconds before starting; negative counts as 0 |
| `Speed(f)` | Scale the tween's clock, delay included: 2 is twice as fast, 0 freezes it |
| `Repeat(n)` | Run the cycle `n` more times; negative repeats forever |
| `Yoyo()` | Run every second cycle backwards, with the ease mirrored |
| `Relative()` | The `To` (or `From`) value is an offset from the start value |
| `Additive()` | Write the change per tick instead of the value ([Overwrite and Additive](#overwrite-and-additive)) |
| `DespawnOnDone()` | Delete the entity when the tween finishes |
| `OnDone(f)` | Call `f` when the tween finishes ([Callbacks](#callbacks-and-their-cost)) |
| `Then(next)` | Hold `next` until this tween finishes ([Sequences](#sequences)) |
| `ID()` | The tween's `tween.ID` |

`Relative` and `Additive` are read when the tween starts, so set them right after `To` or `From`. Both are meant for numeric values (`float32`, `gfx.Vec2`, `[4]float32`), not colors.

Some common shapes:

```go
// Hit pop: from 1.3 times the size back to normal.
tween.From(tw, e, tween.Scale, gfx.Vec2{X: 1.3, Y: 1.3}, 0.18).Ease(tween.OutQuad)

// Bob up and down by 3 pixels, forever.
tween.To(tw, coin, tween.Pos, gfx.Vec2{Y: -3}, 0.6).Relative().Repeat(-1).Yoyo().Ease(tween.InOutSine)

// Death fade, then delete.
tween.To(tw, e, tween.Tint, gfx.RGBA(255, 255, 255, 0), 0.5).DespawnOnDone()
```

With `Repeat(n)` and `Yoyo()`, the tween finishes on the start value when `n` is odd (there and back) and on the end value when it is even.

## Easing

`tween.Ease` is an enum. Every curve is exactly 0 at the start of a cycle and exactly 1 at its end. In curves start slowly, Out curves end slowly, InOut curves do both.

| Ease | Shape |
| --- | --- |
| `Linear` | Constant speed (the zero value) |
| `InQuad`, `OutQuad`, `InOutQuad` | Gentle acceleration (t²) |
| `InCubic`, `OutCubic`, `InOutCubic` | Stronger acceleration (t³) |
| `InSine`, `OutSine`, `InOutSine` | Softest; good for bobbing and pulsing |
| `InExpo`, `OutExpo`, `InOutExpo` | Very sharp |
| `InBack`, `OutBack`, `InOutBack` | Pulls back past the start, or overshoots the end, by about 10% |
| `InElastic`, `OutElastic` | Springs around the start or end |
| `InBounce`, `OutBounce` | Bounces like a dropped ball |
| `Step` | Stays at the start value, jumps to the end value at the end of the cycle |

`EaseFunc(f)` takes any curve: `f` gets the time fraction in (0, 1) and returns the progress. It should return 0 at 0 and 1 at 1; the last cycle writes the exact end value either way, so a curve that ends off target cannot leave the field off target.

## Sequences

Start tweens one after another with `Sequence`, which holds each tween after the first until the previous one finishes and returns the first ID:

```go
up := tween.To(tw, e, tween.Scale, gfx.Vec2{X: 1.3, Y: 1.3}, 0.1).ID()
back := tween.To(tw, e, tween.Scale, gfx.Vec2{X: 1, Y: 1}, 0.1).ID()
fade := tween.To(tw, e, tween.Tint, gfx.RGBA(255, 255, 255, 0), 0.4).DespawnOnDone().ID()
tween.Sequence(tw, up, back, fade)
```

`b.Then(next)` does the same for two tweens. For parallel tweens, just start several. A held tween reads its start value when it is released, in the tick after the previous one finished. Killing a tween in a sequence kills the ones after it; completing it releases the next.

## Control

Every call on a finished or killed `ID` is a no-op, so gameplay code never has to track whether a tween still runs. The zero `ID` is never valid.

| Method | Effect |
| --- | --- |
| `tw.Kill(id)` | Stop where it is, without the end value or `OnDone`; kills the rest of its sequence |
| `tw.Complete(id)` | Write the end value and finish now: `OnDone` runs, the next tween of the sequence is released, `DespawnOnDone` is queued. A tween that has not started reads its start value first. For a tween that repeats forever, the end value is the `To` value |
| `tw.Pause(id, on)` | Pause or resume; the tween keeps its place and its delay |
| `tw.Running(id)` | Whether the tween has neither finished nor been killed (delayed, paused and held tweens are running) |
| `tw.KillEntity(e)` | Kill every tween on `e` |
| `tween.KillLens(tw, e, lens)` | Kill every tween of `lens` on `e`, additive ones included |
| `tw.Clear()` | Drop every tween, without values or callbacks |
| `tw.Len()` | The number of running tweens |

## Overwrite and Additive

When a tween starts, it kills a running tween on the same entity and lens: a new "move to" never fights an old one. The check happens at the start, not when `To` is called, so a delayed tween does not cut the running one short.

`Additive()` opts out. An additive tween writes the change since its last tick instead of a value, so additive tweens on one field stack with each other and with game code that moves the field, such as a shake on top of a move or on top of physics:

```go
tween.To(tw, e, tween.Pos, gfx.Vec2{X: 64}, 1).Relative().Additive()                      // move 64 right
tween.To(tw, e, tween.Pos, gfx.Vec2{Y: 2}, 0.05).Relative().Additive().Repeat(9).Yoyo() // shake on top
```

Use `Relative()` with `Additive()` for a "move by". An additive `To` with an absolute target still aims at that target on every axis, so it pulls the other tweens' offsets back. A non-additive tween on the same field writes absolute values and wins while it runs.

## Your own fields

A lens is one line. Create it once, as a package-level variable:

```go
type Health struct {
	Current, Max int32
	Shown        float32 // what the HP bar draws
}

var hpBar = tween.NewLens(func(h *Health) *float32 { return &h.Shown }, tween.LerpFloat)

tween.To(tw, player, hpBar, 40, 0.3).Ease(tween.OutCubic) // ease the bar down to 40
```

`get` returns a pointer into the component it is given. `lerp(a, b, t)` returns the value `t` of the way from `a` to `b`: exactly `a` at 0 and `b` at 1, and it must extrapolate for `t` outside [0, 1], because the Back and Elastic eases overshoot and `Relative` and `Additive` compute through it. The package's `LerpFloat`, `LerpVec2`, `LerpFloat4` and `LerpColor` (per channel in 0..1 RGBA, clamped) do. Each lens is its own field for the overwrite rule, so create a lens once and reuse it.

## Callbacks and their cost

`OnDone(f)` runs when the tween finishes by itself or through `Complete`, never for killed tweens or dead targets. Callbacks run after every tween has advanced for the tick, so a callback may start and kill tweens, including on the same entity:

```go
tween.To(tw, door, tween.Pos, open, 0.5).OnDone(func() { tween.To(tw, door, tween.Pos, closed, 0.5).Delay(2) })
```

A function without captures costs nothing. A closure that captures variables allocates once, when it is created; the tick itself allocates nothing. The same holds for `EaseFunc`.

`DespawnOnDone` queues the delete on the World's `system.Commands`, which the schedule applies after the stage.

## Dead targets

A tween whose entity was deleted, or no longer has the component, is dropped in the next tick: no value is written, `OnDone` does not run, and the rest of its sequence is killed. A tween started on a dead entity is dropped the same way. Nothing panics, so a system can delete an entity without killing its tweens first.

## Pause and the editor

The engine runs `tween.System` first in the camera stage ([tick order](engine.md#tick-order)): a tween started by a game system runs in the same tick, and the follow camera sees the tweened position. `Transform.PrevPos` was snapshotted before, so tweened movement is interpolated like any other.

The camera stage is paused with the game, so tweens stop while a Tool pauses it (`game.TickPause`, the editor's edit mode). The editor's Stop clears every tween before it calls `Config.OnReset`, so nothing started during Play keeps animating the level. Loading a level clears nothing: tweens on persistent entities keep running, and tweens on deleted level entities end as dead targets.

Without a `game.Engine`, create `tween.New(w)` and run `tween.NewSystem(tw)` in your schedule, or call `tw.Update(dt)` yourself.

Tweens use no randomness and no wall clock: the same ticks give the same values.

## Performance

A running tween is a row in its lens's pool: per tick it costs one `ecs.GetCompOrNil`, one ease and one lerp. `Update` allocates nothing once the pools have grown, and `To` plus its options allocate nothing once the pools, the ID table and the overwrite map have room. Benchmarks are in [README.md](README.md#performance).

## Panics

| Panic | Cause |
| --- | --- |
| `tween: nil lens` | `To`, `From` or `KillLens` with a nil lens |
| `tween: nil lens function` | `NewLens` with a nil `get` or `lerp` |
| `tween: Param index out of range: <i>` | `Param(i)` with `i` outside 0..3 |
| `tween: negative speed: <f>` | `Speed` with a negative factor |
| `tween: nil world` | `New(nil)` |
| `tween: nil Tweens` | `NewSystem(nil)` |
