# Math

Package `xmath` (`github.com/weakpixel/engine/xmath`) is the float32 math a 2D game needs: vectors, rectangles, transform matrices, geometry tests, seeded random numbers, noise, interpolation, angles, smoothing and easing. It imports only the standard library, so you can use it without the engine. Every identifier has a Go doc comment with a short game example; this page explains the ideas.

`gfx.Vec2` and `gfx.Rect` are aliases of `xmath.Vec2` and `xmath.Rect`: components keep saying `gfx.Vec2`, game code may say either, and the JSON shape in levels, prefabs and GOD files is unchanged (the types have no `MarshalJSON`).

- [Rules](#rules)
- [Scalars](#scalars)
- [Smoothing and why dt matters](#smoothing-and-why-dt-matters)
- [Angles](#angles)
- [Vec2](#vec2)
- [Rect](#rect)
- [Mat3](#mat3)
- [Geometry](#geometry)
- [Random](#random)
- [Noise and hashing](#noise-and-hashing)
- [Easing](#easing)

## Rules

- **float32 everywhere**, because components, vertices and shader uniforms are float32. A few scalar helpers are generic over signed numbers (`Abs`, `Sign`, `Clamp`, `Approach`, `Wrap`); integer-only helpers are named (`FloorDiv`, `ModInt`).
- **Pure and allocation-free.** Value receivers, no globals, no panics. Degenerate input returns a documented value: `Vec2{}.Norm()` is zero, `InvLerp(a, a, v)` is 0, `ClampLen` with a negative limit is zero.
- **Radians, clockwise on screen.** Y points down, so a positive angle turns right toward down, like `Transform.Rot`. Convert at the edges with `Rad` and `Deg`.
- **Short names.** `V(x, y)`, `Len`, `Norm`, `Dist`, `Approach`. There is no `Vec3` and no `Vec2i`; tile cells stay `cx, cy int` pairs.

## Scalars

```go
speed := xmath.Clamp(speed+accel*dt, 0, maxSpeed)
vel.X = xmath.Approach(vel.X, 0, friction*dt) // never overshoots zero
pos.X = xmath.Wrap(pos.X, 0, screenW)         // Asteroids-style edges
cell := xmath.FloorDiv(px, 16)                // -1 for px in [-16, 0), unlike px / 16
pitch := xmath.Remap(speed, 0, 300, 0.8, 1.4)
```

`Lerp(a, b, t)` returns exactly `a` at 0 and exactly `b` at 1 and extrapolates outside. The `float32` wrappers (`Sqrt`, `Sin`, `Atan2`, ...) save you the `float32(math.Sqrt(float64(x)))` casts.

## Smoothing and why dt matters

`pos = Lerp(pos, target, 0.1)` closes 10 % of the gap per frame, so it is twice as fast at 120 fps as at 60 fps. Use the `dt`-based functions instead; they give the same result at any frame rate, and two steps of `dt` equal one step of `2*dt`.

```go
cam = cam.Damp(player, 0.1, dt)         // gap halves every 0.1 s
zoom = xmath.Damp(zoom, targetZoom, 0.15, dt)
vel = vel.Mul(xmath.DampFactor(0.9, dt)) // keeps 10 % of the speed per second
trauma = xmath.Decay(trauma, 2, dt)
```

| Function | Use |
| --- | --- |
| `Damp`, `Vec2.Damp` | Follow a target; the argument is a half-life in seconds. |
| `ExpFactor(dt, tau)` | The same with a time constant `tau`, as the share of the gap to close. |
| `DampFactor(d, dt)` | Drag: keep `1-d` of a value per second. |
| `SmoothDamp` | Critically damped follow with a speed limit; you keep the velocity. |
| `SpringStep`, `Vec2.Spring` | Exact damped spring (`freq` in Hz, `zeta` below 1 overshoots), stable for any `dt`. |

## Angles

```go
aim := target.Sub(pos).Angle()
rot = xmath.ApproachAngle(rot, aim, turnSpeed*dt) // turns the short way
if xmath.Abs(xmath.AngleDiff(rot, aim)) < 0.1 { fire() }
```

`WrapAngle` returns an angle in (-Pi, Pi]. `AngleDiff`, `LerpAngle`, `ApproachAngle` and `DampAngle` take the shortest arc, so they behave across the ±Pi seam.

## Vec2

```go
pos = pos.Add(vel.Mul(dt))
dir := input.Norm()                       // zero stays zero
vel = vel.ClampLen(maxSpeed)
muzzle := t.Pos.Add(gunOffset.Rotate(t.Rot))
vel = vel.Reject(wallNormal)              // slide along a wall
ball.Vel = ball.Vel.Reflect(hit.Normal)
```

`Len`/`LenSq`, `Dist`/`DistSq` (compare squared values to avoid the square root), `Dot`, `Cross`, `Perp`, `Project`, `Lerp`, `Approach`, per-axis `Min`/`Max`/`Clamp`/`Floor`/`Round`/`Snap`, and `Zero`, `One`, `Right`, `Down`, `Left`, `Up` (`Down` is +Y). `FromAngle` and `FromPolar` build vectors from angles.

## Rect

A `Rect` runs from `Min` (inclusive) to `Max` (exclusive), so `Contains` and `Overlaps` agree with tile math: rectangles that only share an edge do not overlap.

```go
box := xmath.RectCentered(pos, xmath.V(12, 16))
if box.Overlaps(coin) { collect() }
push := wall.Penetration(box)             // smallest move that separates them
pos = arena.ClampPoint(pos)
camPos = bounds.ClampCenter(camPos, view.HalfSize()) // centered if the view is bigger
```

Also `RectXYWH`, `RectPoints`, `Intersect`, `Union`, `Extend`, `Translate`, `Grow`, `ContainsRect`, `ClosestPoint`, `Lerp` and `Corners`.

## Mat3

A `Mat3` is a 2D affine transform (rotation, scale and translation in one value). The zero value maps everything to the origin, so start from `Identity()`. `a.Mul(b)` applies `b` first and then `a`.

```go
world := xmath.TRS(t.Pos, t.Rot, t.Scale)     // scale, then rotate, then move
muzzle := world.Apply(gunOffset)              // a point: the translation counts
dir := world.ApplyDir(xmath.Right)            // a direction: it does not
hand := parent.Mul(local).Apply(socket)       // a child attached to a parent
if inv, ok := world.Inverse(); ok {
	local := inv.Apply(mouse)                 // the mouse in the entity's space
}
bounds := world.ApplyRect(sprite)             // axis-aligned box of a rotated sprite
```

`TRS` uses the order the renderer draws sprites in. `Inverse` reports false for a singular matrix (a scale of 0). `Translation`, `Rotation` and `Scaling` build the single steps.

## Geometry

Tests for the shapes games collide with. Rays are written `from + t·d` with `t` from 0 to 1, so `d` is the whole step, not a direction. They return a `Hit` with the fraction `T`, the contact `Point` and the unit surface `Normal`. A ray that starts inside a shape reports `T` 0 and a zero normal; one that only grazes an edge does not hit.

```go
if h, ok := xmath.RayRect(muzzle, aim.Mul(reach), crate); ok { spark(h.Point, h.Normal) }
if h, ok := xmath.SweepRect(bullet, step, wall); ok { bullet = bullet.Translate(step.Mul(h.T)) }
if xmath.CirclesOverlap(blast, xmath.Circle{C: enemy, R: 6}) { hurt() }
if xmath.PointInPolygon(player, zone) { enter() }
```

Also `RayCircle`, `RaySegment`, `SegmentsIntersect`, `ClosestOnSegment`, `SegmentDist`, `CircleRectOverlap`, `CirclePenetration`, `PolygonBounds`, `PolygonArea` (positive when clockwise on screen) and `Barycentric`. `collision.Space` has its own broad phase and raycast; these functions are the building blocks for code that tests a few shapes directly.

## Random

`Rand` is a seeded generator (PCG). There is no package-wide generator: give each system its own `Rand`, and the same seed replays the same run, which is what replays and tests need. It is not safe for concurrent use. `Seed` restarts the sequence in place.

```go
rng := xmath.NewRand(seed)
if rng.Chance(0.25) { crit() }
dmg := rng.IntRange(3, 6)                     // both ends included
spark := pos.Add(rng.InCircle(8))             // uniform in a disc
aim = rng.Spread(aim, xmath.Rad(5))           // aim ± 5°
loot := xmath.Pick(rng, drops)
kind := rng.Weighted([]float32{70, 25, 5})    // index by weight, -1 if all zero
xmath.Shuffle(rng, deck)
```

Also `Float`, `Range`, `Sign`, `Angle`, `UnitVec`, `OnCircle`, `InRect` and `Gauss`.

## Noise and hashing

`Noise1(seed, x)` and `Noise2(seed, x, y)` are smooth gradient noise in -1..1. They are zero at integer coordinates, continuous in between and the same on every run; each seed is an unrelated field. Use `x = time * frequency` for shake or flicker, and scale the coordinates to choose the feature size of 2D noise.

```go
flicker := 1 + 0.2*xmath.Noise1(torchID, time*8)
height := xmath.Noise2(worldSeed, x*0.05, y*0.05)
```

`Hash32` mixes an integer and `HashFloat` maps it to 0..1: the same cell always gets the same value without a `Rand`, e.g. `xmath.HashFloat(uint32(cx*73856093 ^ cy))` for a grass tint.

## Easing

`EaseFunc` curves map 0..1 to 0..1 (`Back` and `Elastic` overshoot in between) and are exactly 0 at 0 and 1 at 1: `EaseInQuad`, `EaseOutCubic`, `EaseInOutSine`, `EaseOutBack`, `EaseOutBounce`, `EaseOutElastic` and the rest of the Penner set. `Reverse` turns an In curve into an Out curve and `Mirror` into an InOut curve.

```go
y := xmath.Lerp(startY, endY, xmath.EaseOutBack(t))
```

`tween.Ease` is the serialized enum for [tweens](tweens.md#easing); it indexes these same functions.
