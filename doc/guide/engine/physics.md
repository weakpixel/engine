# Physics

Package `physics` (`github.com/weakpixel/engine/physics`, GOD group `physics`) moves entities: gravity, velocity, damping, mass, bounce and friction for dynamic bodies, code-driven kinematic bodies (moving platforms, elevators), surface materials (ice, trampolines, conveyors), one-way platforms, riders carried by the platform they stand on, and ground/wall/ceiling flags for game code. `MoveAndSlide` moves characters whose movement the game owns. It runs on [collision](collision.md): the same `Collider`, layers, masks, events and queries. It is headless (`ecs`, `ecs/system`, `gfx`, `collision`, `tween` and the standard library) and opt-in: `game` never adds it.

```go
var (
	Body = collision.Layer(1)
	Wall = collision.Layer(2)
)

phys := physics.NewSystem(e.World(), physics.Config{Gravity: gfx.Vec2{Y: 980}})
e.AddRunner(game.StageSimulate, playerInput, phys, landingDust) // after Velocity writers, before readers
phys.Space().AddTiles(walls, collision.TileOptions{Layer: Wall})

crate := e.World().NewEntity(gfx.Transform{Pos: p},
	collision.Collider{Size: gfx.Vec2{X: 16, Y: 16}, Layer: Body, Mask: collision.LayersOf(Body, Wall)},
	physics.Body{GravityScale: 1}, physics.Velocity{}, physics.Touching{})

// A jump, in a system before physics:
if t := ecs.GetComp[physics.Touching](w, hero); t.OnGround {
	ecs.GetComp[physics.Velocity](w, hero).Vel.Y = -300
}
```

Use `physics.NewSystem` instead of `collision.NewSystem`, not with it: it creates the World's collision system and drives it. Detection-only colliders (coins, trigger zones) work unchanged next to bodies.

## Components

A body is an entity with a `gfx.Transform`, a `physics.Body` and a `physics.Velocity`; add a `collision.Collider` for it to collide. A static object is a `Collider` without a `Body` (set `Static` for speed). Bodies stay upright: `Transform.Rot` is ignored.

### Body

| Field | Zero value | Meaning |
| --- | --- | --- |
| `Kind` | `Dynamic` | `Dynamic`: moved by gravity, velocity and contacts. `Kinematic`: moved only by its `Velocity`; pushes dynamic bodies, carries riders, is never pushed, passes through statics and other kinematic bodies |
| `Mass` | 1 | mass of a dynamic body; zero or negative is 1. Kinematic bodies have infinite mass |
| `Damping` | none | fraction of the velocity lost per second, per axis: 0.5 halves it every second, 1 or more stops at once |
| `GravityScale` | 0 (no gravity) | multiplies `Config.Gravity`: 1 full, 0.5 floaty, negative falls up. Tweening it to 0 ends at no gravity |
| `MaxSpeed` | unlimited | speed limit per axis, in world units per second; zero means none |
| `LockX`, `LockY` | false | the body does not move along that axis (an elevator locks X) |
| `DropThrough` | false | ignore one-way colliders and grids; physics clears it once the body touches none |

A zero `Body` is a dynamic body of mass 1 **without gravity**. Falling bodies set `GravityScale: 1`.

### Velocity

`Velocity{Vel gfx.Vec2}` is the speed in world units per second. Physics moves bodies by it, game code sets it (a jump) or reads it (walk or idle animation). It is its own component so that non-bodies use it too: `NewMoveSystem` and `MoveAndSlide` movers. A `Body` without a `Velocity` gets a zero one through deferred commands on its first tick and moves from the next; add both.

While a dynamic body stands on a kinematic body, its `Velocity` is relative to that body: the platform's motion is added on top. When it leaves the platform (a jump, walking off), the platform's velocity is added to it, so a jump from a rising elevator goes higher.

### Touching

`Touching` is runtime state, never serialized. Physics writes it every tick for bodies that have it (it never adds one); `MoveAndSlide` writes it on every call.

| Field | Meaning |
| --- | --- |
| `OnGround` | stands on something (a contact normal pointing up, within 45 degrees) |
| `OnCeiling` | something blocks it from above |
| `OnWall` | -1 a wall on the left, +1 on the right, else 0 |
| `Ground` | the entity it stands on; zero for a tile grid |
| `GroundNormal` | unit normal of the ground, pointing up out of it |

Only the detecting side gets flags: a body whose mask does not have the floor's layer is not on the ground.

### Material

`Material` sets the surface behavior of any collider: a dynamic crate, a static wall, a kinematic platform. Without one, the collider uses `Config.DefaultMaterial` (`{Bounce: 0, Friction: 0.4}`). A present `Material` is taken literally, so `Friction: 0` is ice.

| Field | Meaning |
| --- | --- |
| `Bounce` | restitution: 0 does not bounce, 1 keeps the speed, more than 1 adds to it (a trampoline) |
| `Friction` | Coulomb friction coefficient; 0 is ice |
| `SurfaceVel` | velocity the surface drags touching bodies toward, along the contact (a conveyor belt) |

Per contact the two sides combine: bounce is the larger one, friction is `sqrt(a * b)`, surface velocity is the other side's `SurfaceVel` minus this side's. A tile grid gets a material with `System.SetTileMaterial(grid, m)` (one grid per material: an ice layer, a conveyor layer).

```go
w.NewEntity(gfx.Transform{Pos: p}, collision.Collider{Size: size, Layer: Wall, Static: true},
	physics.Material{Bounce: 1.2}) // trampoline
```

Bounces below `Solve.RestThreshold()` (twice what gravity adds in a substep) are dropped, so resting bodies do not jitter.

## Layers and masks

Physics uses the [collision rule](collision.md#layers-and-masks-who-reacts): a body is pushed out of another collider only when its own `Mask` has a layer the other is on. When both bodies detect each other, the push is split by inverse mass; when only one does, it is pushed as if the other were immovable and the other is not touched. A dynamic body against a kinematic body or a collider without a body takes the whole correction. `Sensor` colliders report contacts and events but never push.

## The System

`physics.NewSystem(w, cfg)` returns the `System` (a `system.Runner`). Add it to `StageSimulate` after the systems that set `Velocity` (input, AI) and before the ones that read `Touching`, contacts or events.

| `Config` field | Default | |
| --- | --- | --- |
| `Gravity` | zero | world gravity in world units per second squared: `(0, 980)` for a platformer, zero for top-down. `System.SetGravity` changes it at run time |
| `Substeps` | 1 | steps per tick. More keep fast bodies from passing through thin colliders, at the cost of one collision detection each |
| `Iterations` | 4 | resolver passes per substep |
| `DefaultMaterial` | `{Bounce: 0, Friction: 0.4}` | material of colliders without a `Material` |
| `Collision` | zero | the `collision.Config` of the collision system physics creates |
| `Resolver`, `PreSolve`, `PostSolve` | nil | [hooks](#customizing) |

`System.Space()` and `System.Collision()` return the collision `Space` and `System`.

### What a tick does

The tick length is `ctx.Time.Delta`. First, every `Follow` body gets its velocity. Then, per substep:

1. Riders whose ground changed switch between world and platform-relative velocity.
2. Collision detection (`collision.System.Detect`, no events).
3. Dynamic bodies get gravity, damping, the speed limit and the axis locks on their velocity.
4. Solid contacts are built (mirrored pairs merged, sensors, one-way passes and contacts nobody is pushed by dropped, materials mixed, `PreSolve` called) and resolved for `Iterations` passes: sequential impulses along the normal with restitution, then Coulomb friction along the tangent, warm started with the last substep's impulses.
5. `PostSolve` is called for each contact.
6. Positions move by the velocities (semi-implicit Euler); riders also by their platform's velocity.
7. Overlaps are pushed apart by 80% of the depth beyond 0.01 units, split by inverse mass, so resting stacks neither jitter nor sink.
8. Ground, wall and ceiling flags and `Impact` events.

After the last substep, `collision.System.Publish` sends the tick's `Enter`/`Exit` (and `Stay`) events once, and `Touching` is written. Contacts, events and flags are those detected at the start of the last substep, before its move: they lag the positions by one substep.

### Tiles

A body against a tile grid treats each solid cell as a box. A contact whose normal points into a neighboring solid cell is dropped (internal edges), so a box sliding along a tile floor or landing on the seam between two cells does not snag.

### One-way platforms

A collider or tile grid with `OneWay` set (`collision.OneWayUp` is a platform you jump through from below) blocks a body only when the contact normal points out of its blocking face and the body is not deeper inside it than it moves in one substep, so it came from the blocking side. A contact that blocked in the last substep keeps blocking while it lasts. A body with `Body.DropThrough` ignores all one-way contacts; physics clears the flag once the body touches no one-way collider.

### Moving platforms

A dynamic body standing on a kinematic body is carried: it moves by the platform's velocity in every substep, horizontally and vertically, so it neither slides off nor sinks into a rising elevator. A stack of dynamic bodies on a kinematic body (up to 8 high) is carried as a whole, so the bodies in it do not bounce against each other when the platform changes speed. A `MoveAndSlide` mover whose `Touching` says it stands on a kinematic body, or on a dynamic body carried by one, is moved with that body too. Dynamic bodies standing on dynamic bodies that no platform carries are not carried; friction drags them.

### Events

The tick's `collision.Enter`, `Exit` and `Stay` events are sent once per tick, whatever the substep count. Physics also sends `physics.Impact` when a solid contact starts (not for sensors): once per detecting side and pair, and once per (body, tile grid), like `Enter`.

```go
type Impact struct {
	Self, Other ecs.Entity        // Other is zero for a tile grid
	Tiles       collision.TileGrid
	Normal      gfx.Vec2          // from Self out of Other
	Speed       float32           // relative speed along the normal, >= 0
}
```

Scale landing dust or hit sounds with `Speed`.

## Follow: tweened platforms

Tweens run in `StageCamera`, after physics. A platform tweened with `tween.Pos` teleports after physics has run: physics sees no velocity, so riders are not carried, crates are pushed out without momentum and friction cannot drag. Tween its `Follow` target instead:

```go
platform := w.NewEntity(gfx.Transform{Pos: start}, collision.Collider{Size: size, Layer: Wall},
	physics.Body{Kind: physics.Kinematic}, physics.Velocity{}, physics.Follow{})
tween.To(e.Tweens(), platform, physics.FollowTarget, gfx.Vec2{X: 300, Y: 80}, 2).
	Ease(tween.InOutSine).Yoyo().Repeat(-1)
```

At the start of each tick physics sets the body's `Velocity` to `(Target - Pos) / dt`, so it reaches the target at the end of the tick as an ordinary kinematic body. Platform and riders run one tick behind the tween clock, both by the same amount. A fresh `Follow` takes the body's position as its target on the first tick, so it never jumps to the origin. `Follow.Target` is runtime state. Removing `Follow` leaves the body with its last `Velocity`: zero it.

## MoveAndSlide

For a player or enemy whose movement the game owns (a tight platformer, a top-down hero), move it with `MoveAndSlide` instead of making it a dynamic body:

```go
v := ecs.GetComp[physics.Velocity](w, hero)
v.Vel.Y += 980 * dt
res := physics.MoveAndSlide(w, hero, v.Vel.Mul(dt), physics.MoveOptions{})
if res.HitY || (res.OnGround && v.Vel.Y > 0) {
	v.Vel.Y = 0 // landed, standing or bumped the ceiling; Touching is written too
}
```

`MoveAndSlide(w, e, delta, opts) MoveResult` moves `e`'s `Transform.Pos` by `delta` against the solids its `Collider.Mask` detects: colliders that are not sensors, tile grids, and one-way colliders and grids from their blocking side. It moves along X first, then Y, each axis stopping flush at the first blocker, so a diagonal move into a wall slides along it and tile seams never snag. It tests colliders where they are now, not where the last detection saw them. A solid the mover overlaps by up to one unit blocks it where it is (a box that moved into it, a platform that rose into its feet); a solid it overlaps by more does not block it, so a mover can always get out.

| `MoveOptions` field | Meaning |
| --- | --- |
| `Pixel`, `Remainder` | move in whole units and carry the fraction in `*Remainder` (kept by the game, for example in its own component), so positions stay on the pixel grid. A hit zeroes that axis's remainder |
| `DropThrough` | ignore one-way solids for this move |
| `Push` | the mover's pushing mass, in the unit of `Body.Mass`; zero (the default) does not push. See [Pushing bodies](#pushing-bodies) |
| `CornerCorrect` | largest sideways shift, in world units, that lets an upward move clear a ceiling corner it only clips; zero is off |
| `LedgeCorrect` | largest up or down shift, in world units, that lets a sideways move clear a ledge or overhang edge it only clips; zero is off |

| `MoveResult` field | Meaning |
| --- | --- |
| `Moved` | how far it moved |
| `HitX`, `HitY` | the move stopped on that axis |
| `HitXEntity`, `HitYEntity` | the collider hit; zero for a tile |
| `HitXTiles`, `HitYTiles`, `HitXCell`, `HitYCell` | the grid and cell of a tile hit |
| `OnGround` | a hit going down, or the mover ends on or less than one unit above a solid without moving up (also when the move is zero, as a pixel move that rounds to nothing) |
| `OnCeiling`, `OnWall` | a hit going up, left (-1) or right (+1) |
| `Pushed`, `PushedMass` | the first body a `Push` move pushed and the total mass of the bodies it pushed; zero when it pushed nothing |
| `CornerShift`, `LedgeShift` | the X shift of a corner correction and the Y shift of a ledge correction, signed; zero when none. `Moved` includes them |

**Corner correction** applies only to a hit going up (`delta.Y < 0`); landing on a ledge edge stays a landing. With `delta.X < 0` it tries only shifts to the left, with `delta.X > 0` only to the right, and with `delta.X == 0` it alternates left then right (1 left, 1 right, 2 left, ...). The smallest shift that works wins. **Ledge correction** applies to any X hit and picks its direction the same way from `delta.Y`: up only when moving up, down only when moving down, up then down when `delta.Y == 0`. Both shift in whole units, 1 up to the limit, in pixel and non-pixel mode alike. A shift works when the mover can move by it (a solid beside it, such as a wall next to the head, blocks the nudge) and the original move then continues by at least one unit; a later hit on that axis is reported as a normal hit. A cleared corner leaves `HitY` and `OnCeiling` false, a cleared ledge leaves `HitX` and `OnWall` false. A correction does not zero any remainder. Celeste-like values for an 8-wide mover are 4.

Ledge correction on a move that does not go down acts as a step-up: a walking mover climbs edges no higher than `LedgeCorrect`. A walking move with gravity goes down, so it tries only down shifts and does not step up. Pass `LedgeCorrect` only on the moves that want it (a dash, a jump) unless a step-up is wanted. Physics bodies are never corrected.

```go
opts := physics.MoveOptions{CornerCorrect: 4}
if dashing {
	opts.LedgeCorrect = 4
}
res := physics.MoveAndSlide(w, hero, v.Vel.Mul(dt), opts)
```

`OnGround` is true every tick a mover stands on the ground, so a jump check never misses a frame. Zero the fall speed when it is set, or gravity builds it up while standing. If `e` has a `Touching`, it is written from the result.

The recommended mover has a `Velocity`, a `Collider` and no `Body`: physics never pushes it, it stops at dynamic bodies it walks into (or pushes them, with `Push`), and its contacts and events work as for any collider. (A kinematic body would also be moved by physics through its `Velocity`.) It needs only the `Space`, so it works with only `collision.System` running (a top-down game with walls and no physics).

### Pushing bodies

With `MoveOptions.Push` set, an X move into a dynamic body pushes it:

```go
res := physics.MoveAndSlide(w, hero, v.Vel.Mul(dt), physics.MoveOptions{Push: 2})
pushing := res.Pushed != (ecs.Entity{}) // play the push animation
```

- The mover moves freely up to the body, then the rest of the move is shared: mover and bodies move by `rest * Push / (Push + M)`, where `M` is the total mass of the pushed bodies (`Body.Mass`, zero counts as 1). Push 1 against a crate of mass 1 moves at half speed, Push 2 at two thirds; a very heavy crate barely moves. A game that wants a "too heavy" threshold checks `Body.Mass` itself and passes `Push` 0.
- Only X moves push. Landing on a body or bumping it from below is a normal Y hit.
- Only dynamic bodies without `LockX` are pushed. Kinematic bodies, colliders without a `Body` and tiles block as without `Push`.
- A row of up to 8 dynamic bodies that touch (less than half a unit apart) is pushed together; a ninth body in the row blocks it.
- `MoveAndSlide` moves the pushed bodies' `Transform.Pos` itself and leaves their `Velocity` alone, so a crate stops when the mover stops. Each body is swept with its own collider and mask, so the row stops flush at a wall and the mover stops flush at the row. `HitX` stays false while the row moved; when it cannot move, the move stops as at a wall (`HitX`, `HitXEntity` is the first body, `Pushed` zero). A push comes before ledge correction, so `LedgeCorrect` still applies to a blocked row.
- Bodies standing on a pushed body are not moved with it; friction in the next physics step drags them.
- In pixel mode the pushed move is rounded toward zero to whole units and the fraction is carried in `Remainder.X`, so the average speed matches the share and the positions stay on the grid.

Limits: it finds candidate colliders through the Space, so a collider that moved more than 16 units since the last detection can be missed. Blockers are tested by their bounds (a circle as its box), and a circle mover moves as its bounds. Custom shape kinds panic. A mover needs `Touching` to be carried by a [moving platform](#moving-platforms). It does not allocate.

## Simple movers

`physics.NewMoveSystem(w)` returns a runner that moves every entity with a `Transform` and a `Velocity` but no `Body` by `Velocity x dt`: no gravity, no collision. Use it for bullets, drifting clouds and props, also without the physics system (with a `collision.System` that reports bullet hits).

## Customizing

There is no per-body resolver: a contact has two sides, so a per-body choice needs a "whose resolver wins" rule, and mixing resolvers in one pile keeps the solver from converging (stacks jitter or sink). Per-object behavior comes from `Material` (surfaces), `MoveAndSlide` (characters) and `PreSolve` (anything else, keyed on the game's own components).

- `Config.PreSolve func(c *physics.Contact) bool` is called for every solid contact of every substep before resolution. Return false to ignore the contact for that substep (a player passing through enemies while invincible). It may change `c.Bounce`, `c.Friction` and `c.SurfaceVel` for this contact (a trampoline that bounces only when hit from above: `if c.Normal.Y < -0.7 { c.Bounce = 1 }`).
- `Config.PostSolve func(c *physics.Contact)` is called after resolution with the applied `c.Impulse` (damage from crush force).
- `Config.Resolver` replaces the built-in impulse resolver for the whole World. `Resolve(s *physics.Solve, contacts []physics.Contact)` is called `Iterations` times per substep and changes velocities; physics then corrects positions on its own. `Solve` gives `Dt`, `Pass`, `Vel(c)` (velocity pointers of both sides), `InvMass(c)` (per-axis inverse masses, zero for a side that must not change) and `RestThreshold()`.

Hooks must not add or remove entities or components; queue those as commands.

`physics.Contact` embeds the `collision.Contact` (`Self`, `Other`, `Normal` from `Self` out of `Other`, `Depth`, `Tiles`, `Cell`) and adds `Bounce`, `Friction`, `SurfaceVel` (relative to `Other`), `ApproachSpeed` (positive when the sides move into each other), `Impulse` and `FrictionImpulse`. When both sides detect each other, their two mirrored collision contacts are merged into one, with the first in the Space's order as `Self`. Before the first pass `Impulse` and `FrictionImpulse` hold the last substep's impulses of the same contact (warm start).

Bodies are plain components: teleporting is writing `Transform.Pos`.

## Panics

| Message | Cause |
| --- | --- |
| `collision: Space already has a system` | `physics.NewSystem` for a World that already has a collision or physics system |
| `physics: negative Substeps`, `physics: negative Iterations` | `Config` |
| `physics: MoveAndSlide needs a rect, anim-body or circle collider` | `MoveAndSlide` on a custom shape kind |

`MoveAndSlide` on an entity without a `Transform` or `Collider` panics as `ecs.GetComp` does.

## Performance

`System.Run`, `MoveSystem` and `MoveAndSlide` do not allocate once warm (`AllocsPerRun` tests). Measured on a Xeon 2.1 GHz (`go test -bench . ./physics/`), one tick with one substep:

| Scene | Time per tick |
| --- | --- |
| 1,000 boxes and balls in a pile on a tile floor | about 3.4 ms |
| 100 bodies among 1,000 static one-way colliders | about 0.18 ms |
| `MoveSystem` over 10,000 Velocity entities, a quarter of them bodies (skipped per archetype) | about 0.08 ms (4-core Xeon 2.8 GHz) |

Each substep runs a full collision detection, so substeps multiply the detection cost.

## Not built yet

Rotation, angular velocity and rotated shapes; joints, springs and ropes; sleeping bodies; continuous collision (use `Substeps` and `MaxSpeed`); slopes and partial tile shapes; carrying riders on dynamic bodies that no platform carries; bodies pushing `MoveAndSlide` movers (crushing, being shoved), pushing bodies on the Y axis; per-level gravity; a level-editor path mover that writes `Follow.Target`; a Lua binding; editor gizmos for velocity.
