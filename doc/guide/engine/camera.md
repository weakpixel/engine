# Follow camera

Package `camera` moves the [`render.Camera`](rendering.md#camera) after one entity. Give the entity a `camera.Target`, choose a follow mode, and set world bounds if you want them. The camera follows the entity, and interpolation between ticks works as usual. `game.Engine` runs the system for you, so you don't write a camera system yourself.

## Quick start

```go
e := game.New(game.Config{
	CameraRig: camera.Rig{Mode: camera.FollowSmooth},
})
hero := ecs.NewEntity2(e.World(), gfx.Transform{}, gfx.Sprite{ /* ... */ })
ecs.SetComp(e.World(), hero, camera.Target{})

// Once the level has spawned:
e.CameraRig().Bounds = level.Bounds() // keep the view inside the maps
e.CameraRig().Snap()                  // the hero was just placed: jump, don't sweep
```

`Engine.CameraRig()` returns the `*camera.Rig`, which is also the World resource `*camera.Rig`. Change any of its fields at any time, from a system or from a tool. With no `Target` entity the system does nothing, so a game that moves the camera itself is not affected.

## Target

`camera.Target{Offset, Priority}` is a runtime-only component. It is not part of any Game Object schema, so add it with `ecs.SetComp`. The entity also needs a `gfx.Transform`.

- The camera aims at `Transform.Pos + Offset`, for example a point above a character's feet.
- If several entities have a `Target`, the highest `Priority` wins. Among equal priorities, the entity with the lowest `ecs.Entity` ID wins, so the choice doesn't change when entities move between archetypes.
- When the camera switches from one target to another, it doesn't jump. The smooth motions ease over to the new target; the hard motion moves there in one tick.

## Modes

A tick runs a small pipeline:

1. The **goal** is the target's position plus `Offset`.
2. The optional **look-ahead** and **dead-zone** stages reshape the goal.
3. A **motion** moves the Rig's follow position toward the reshaped goal.
4. `Camera.Pos` becomes the follow position, clamped to `Bounds`.

Each `FollowMode` is a preset of this pipeline. `DeadZone.On` and `LookAhead.On` add their stage to any preset.

| `Rig.Mode` | Look-ahead | Dead zone | Motion | Use it for |
| --- | --- | --- | --- | --- |
| `default` (or `""`) | – | – | hard | Locked on the target |
| `smooth` | – | – | smooth | A soft follow; the usual choice |
| `deadzone` | – | on | hard | Platformers: small moves don't scroll the view |
| `spring` | – | – | spring | A springy, physical feel; overshoots when under-damped |
| `lookahead` | on | – | smooth | Fast movers: show more of where the target is heading |

An unknown mode panics with `camera: unknown follow mode "x"`. `game.New` checks `Config.CameraRig.Mode` and panics with the same message, and `System.Run` panics if the mode is changed to an unknown value later. `FollowMode.Check()` runs the same test.

### Parameters and defaults

A zero field uses the default in this table.

| Field | Default | Meaning |
| --- | --- | --- |
| `Smooth.Time` | 150 ms | Time constant of the smooth motion. After `Time`, the camera has covered 63% of a jump. Each tick covers `1 - exp(-dt/Time)` of the remaining distance, so the motion is the same at any tick rate. |
| `DeadZone.HalfSize` | (32, 24) | Half the size of the dead-zone window, in world units. It doesn't scale with `Zoom`. |
| `Spring.Frequency` | 2 Hz | Natural frequency of the spring. Higher is stiffer. |
| `Spring.Damping` | 1 | Damping ratio. 1 is critical: the fastest approach without overshoot. Below 1 overshoots; above 1 is sluggish. The spring is solved in closed form, so it is stable at any tick rate. |
| `LookAhead.Distance` | 48 | How far ahead of the target the goal moves, in world units. |
| `LookAhead.Time` | 300 ms | Easing time of the look offset, both toward the direction of motion and back to zero. |
| `LookAhead.MinSpeed` | 1 unit/s | Below this target speed the look offset returns to zero. |
| `TeleportDistance` | 0 (off) | Snap when the goal is further than this from the follow position. |
| `Bounds` | zero `Rect` (unbounded) | The world rectangle the view must stay inside. |

How the stages work:

- **Dead zone**: the stage keeps a window center. The center stays still while the goal is inside `±HalfSize` around it. When the goal leaves the window, the center moves just enough to put the goal back on the window's edge. With the hard motion the camera sits on the window center; with smooth or spring it eases toward it.
- **Look-ahead**: the target's velocity is `(Transform.Pos - Transform.PrevPos) / dt`. The look offset eases toward `Distance` in that direction.
- When a stage is turned off, its state goes back to neutral: no look offset, and the window centered on the goal.
- A tick with `dt <= 0` moves nothing.

### Combining stages

```go
rig := camera.Rig{
	Mode:      camera.FollowSmooth,
	DeadZone:  camera.DeadZone{On: true, HalfSize: gfx.Vec2{X: 24, Y: 16}},
	LookAhead: camera.LookAhead{On: true, Distance: 32},
}
```

This camera eases after the target, ignores small moves inside the dead zone, and looks ahead in the direction of motion. The tested version is `ExampleRig` in [camera/example_test.go](../../../camera/example_test.go).

## Bounds

After the motion, the camera is clamped so that the view stays inside `Rig.Bounds`. The view's half size is the viewport size (`Camera.ViewportRect`, so a custom `Viewport` counts) divided by `2*Zoom`. If `Bounds` is smaller than the view on an axis, the camera is centered on `Bounds` on that axis. `Rotation` is ignored when clamping.

For bounds that match the level's maps:

- `game.Level.Bounds()` is the union of a map level's maps, in world coordinates. It is the zero `Rect` until `Level.Spawned()`, and for a GOD level.
- `tilemap.Map.Bounds()` is one map's rectangle in map pixels ([tilemaps.md](tilemaps.md#query-api)).

The clamp only changes the output, `Camera.Pos`. The Rig keeps its own unclamped follow position, and the next tick continues from that position. So the spring doesn't build up speed against an edge, and the camera leaves the edge as soon as the target moves away from it.

## Snapping

This section is about jumping to the goal. Rounding the drawn view to whole pixels for pixel art is `render.Camera.PixelSnap` ([rendering.md](rendering.md#pixel-perfect-pixel-art)); it works with every follow mode.

A snap moves the camera straight to the goal. It sets `Camera.PrevPos = Pos`, so the rendered frame doesn't sweep across the level, and it resets the stages: the window moves to the goal, and the spring velocity and look offset become zero. A snap happens:

- on the next tick after `Rig.Snap()`. Call it after a teleport or a level load.
- on the first tick with a target after a tick without one, for example the first tick of the game, or a level reload that spawns the target again.
- when the goal is further than `TeleportDistance` from the follow position.

## Shake

The Rig has a built-in, trauma-based screen shake. Call `AddTrauma` from any system when something hits:

```go
e.CameraRig().AddTrauma(0.4) // a hit; an explosion might add 0.8
```

- **Trauma** is a number in [0, 1]. `AddTrauma(t)` adds `t` and clamps the sum, so several hits add up; a negative `t` reduces it. `Trauma()` returns the current value, and `StopShake()` sets it to 0.
- Trauma falls by `Shake.Decay` per second. The shake strength is `trauma^Exponent`, so small hits barely move the view and big hits shake hard.
- The offset is `MaxOffset` times the strength times smooth noise, a separate curve per axis. The noise is deterministic: the same hits give the same shake on every run.

Configure it in `Rig.Shake`. A zero field uses the default in this table.

| Field | Default | Meaning |
| --- | --- | --- |
| `MaxOffset` | (6, 6) | The offset at full shake, per axis, in world units. |
| `MaxAngle` | 0 (no rotation) | The rotation at full shake, in radians. |
| `Frequency` | 20 Hz | How fast the noise changes. Higher shakes faster. |
| `Decay` | 1 per second | Trauma lost per second. At 1, full trauma settles in one second. |
| `Exponent` | 2 | The shake strength is `trauma^Exponent`. |
| `InBounds` | false | Clamp the shaken view to `Bounds`. |
| `Off` | false | Turn the shake off, for example as an accessibility setting. `AddTrauma` still counts and trauma still decays, but the camera doesn't move. |

How the shake fits the rest of the camera:

- **Bounds.** By default the shake is added after the bounds clamp, so it stays visible at a level edge and the view can leave `Bounds` by up to `MaxOffset`. With `InBounds` the shaken view is clamped: it never shows outside the level, but the shake flattens against an edge.
- **The follow position is never shaken.** The Rig's stages and motions continue from the unshaken position, so a shake doesn't disturb the spring, the dead zone or the look-ahead. A snap keeps a running shake.
- **Rotation.** With a non-zero `MaxAngle` the shake changes `Camera.Rotation` by the difference to the last tick's angle, so a rotation the game set stays. `Camera.PixelSnap` is ignored while the view is rotated, so pixel art looks best without rotation.
- **Without a target** the system moves `Camera.Pos` only by the change of the shake offset. A camera the game moves itself shakes too, and ends where it started.
- **Time.** The shake runs in the camera stage, so it keeps running under `Engine.Hold` (a hit stop) and freezes while a `game.Tool` pauses the game. The editor's Stop calls `StopShake`.

## Other camera-stage systems

In `game.Engine`, `camera.System` is the first runner of `StageCamera`, so your camera systems run after it and see the followed position:

- **Add an offset** to `Camera.Pos`. A screen shake is built in ([Shake](#shake)), but your own offset system works as well. Offsets don't build up over ticks while there is a target, because the next tick starts again from the Rig's follow position.
- **Override `Camera.Pos`**: the override lasts for that tick only. To take the camera over for good, for example in a cutscene, remove the `Target` component, and add it back afterwards. The camera snaps when the target returns.
- **Change `Zoom`**: the clamp uses the zoom as it is when the follow system runs. A zoom change in a later system is clamped from the next tick on.

When a `game.Tool` pauses the game, the camera stage is skipped like every other game stage.

## Without game.Engine

The system only assigns `Camera.Pos`, plus `PrevPos` when it snaps. Something earlier in the tick must copy `Camera.PrevPos = Camera.Pos`, as the engine's snapshot stage does. The world needs the resources `*render.Camera` and `*extract.Screen`. `System.Init` creates a zero `*camera.Rig` resource if there isn't one. `Init` caches all three resources, so set them before the first `Run`.

```go
s.AddStage("snapshot", extract.NewSnapshotTransforms(w).Run,
	func(*system.Context) { cam.PrevPos = cam.Pos })
// ... input and simulation stages ...
s.AddStage("camera")
s.AddRunner(camera.NewSystem(w))
```

The full version is `ExampleSystem` in [camera/example_test.go](../../../camera/example_test.go).

## Tuning tips

- Start with `smooth` and tune `Smooth.Time`: 0.1 s is snappy, 0.3 s is floaty. At 60 TPS, the old per-tick `Lerp(target, 0.1)` is about `Time` 0.16 s.
- In platformers, use `deadzone`. Add `smooth` motion with `DeadZone.On` so the camera doesn't jerk when the target reaches the window's edge.
- In a fast top-down game, use `lookahead`. Raise `LookAhead.Time` if the view swings too much when the target turns.
- For a `spring` that bounces, set `Damping` to about 0.5. For a heavy feel, lower `Frequency`.
- Set `TeleportDistance` to about one screen width if the target can teleport and the game doesn't call `Snap`.

## Costs

`System.Run` makes no allocations, also while shaking; `TestSystemRunDoesNotAllocate` checks this for every mode. Each tick it visits every `Target` entity once to pick one, so keep the number of targets small.
