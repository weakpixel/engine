# Collision

Package `collision` (`github.com/weakpixel/engine/collision`, GOD group `collision`) finds overlapping colliders every tick, sends `Enter` and `Exit` events and answers point, rect and ray queries, also against tile grids. It detects only: nothing is moved. It is headless (`ecs`, `ecs/system`, `gfx` and the standard library) and opt-in: `game` never adds it.

```go
var (
	Player = collision.Layer(1) // name the layers once
	Pickup = collision.Layer(3)
	Wall   = collision.Layer(4)
)

sys := collision.NewSystem(e.World(), collision.Config{})
e.AddRunner(game.StageSimulate, mover, sys, pickups) // after the moves, before the readers

hero := e.World().NewEntity(gfx.Transform{Pos: start},
	collision.Collider{Size: gfx.Vec2{X: 8, Y: 13}, Offset: gfx.Vec2{X: -4, Y: -13},
		Layer: Player, Mask: collision.LayersOf(Wall, Pickup)})
coin := e.World().NewEntity(gfx.Transform{Pos: p}, collision.Collider{Size: gfx.Vec2{X: 8, Y: 8}, Layer: Pickup})

// In the pickup system (reader created once, in Init):
for ev := range enters.Read() { // *system.EventReader[collision.Enter]
	if ecs.HasComp[Coin](w, ev.Other) { ... }
}
```

## Collider

`collision:Collider` is a Game Object component, so levels and prefabs carry it and the inspector edits it. An entity collides when it has a `gfx.Transform` and a `Collider`.

| Field | Meaning |
| --- | --- |
| `Shape` | `ShapeRect` (0), `ShapeCircle` (1), `ShapeAnimBody` (2), or a custom kind from `ShapeCustom` (128) on. A plain number in GOD, since the set is open |
| `Offset` | added to `Transform.Pos`: the top-left corner of a rect, the center of a circle |
| `Size` | width and height of a rect, the radius (in `X`) of a circle |
| `Layer` | bitmask of the layers the collider is on. Zero means `DefaultLayer` (layer 0) |
| `Mask` | bitmask of the layers it reacts to. Zero reacts to nothing (a coin, a wall) |
| `Static` | never moves on its own; see [Static colliders](#static-colliders) |
| `Sensor` | for physics: report contacts, never push. Detection treats every collider the same |
| `OneWay` | for physics: block from one side only, `OneWayNone` (0), `OneWayUp`, `OneWayDown`, `OneWayLeft`, `OneWayRight`. The name is the blocking face: `OneWayUp` is a platform you jump through from below |

Shapes are axis-aligned. `Transform.Scale` scales `Offset` and `Size` (zero is (1, 1); a negative scale mirrors, as `gfx.Box.World` does; a circle uses `|Scale.X|`). `Transform.Rot` is ignored.

### The collider from the animation body

`ShapeAnimBody` takes its box from the `body` of the entity's [`*.anim.json`](animation.md#animation-definition-animjson): `animbody.System` (package `anim/animbody`; add it to the simulate stage before `collision.System`) writes the union of the `body` rects into `Offset` and `Size` (the hero's `{"x": -4, "y": -13, "w": 8, "h": 13}` gives Offset (-4, -13), Size (8, 13)) from the current clip, `Clip.Body`, when they differ. It never uses the per-frame boxes: a roll's `hurt` shapes do not change the collider, and neither does `Sprite.FlipX`, so author a symmetric body or use an explicit rect. The collider is skipped while `Size` is zero (the clip is not loaded yet; the first `animbody.System` run after it loads fills it). Explicit rects stay the default because the solid box (the feet, top-down) and the hurt box (the whole sprite) usually differ.

## Layers and masks: who reacts

**`Mask` is what I react to.** A collider *detects* another when its `Mask` has a layer the other is on: `a.Detects(&b)` is `a.Mask.Any(b.LayerOrDefault())`. A pair is tested when either side detects the other, and **each detecting side gets its own event, with itself as `Self`**:

| Case | Colliders | Events |
| --- | --- | --- |
| Coin | player: `Layer: Player, Mask: LayersOf(Wall, Pickup)`; coin: `Layer: Pickup` | one, `Self` = player, `Other` = coin |
| Bullet | bullet: `Layer: Bullet, Mask: Enemy`; enemy: `Layer: Enemy, Mask: Bullet` | two: `{Self: bullet, Other: enemy}` and `{Self: enemy, Other: bullet}`, so a bullet system and a damage system each see the pair from their side |
| Trigger zone | zone: `Layer: Zone, Mask: Player` | one, `Self` = zone; the player's mask need not know zones |

Physics follows the same rule: a body is pushed out of another only when its own mask has the other's layer (a ghost drops `Wall` from its mask).

Layers are a `uint32` bitmask (32 layers) with helpers, so game code never spells out bits:

| API | Does |
| --- | --- |
| `Layer(n)` | the single layer `n`, 0..31; `DefaultLayer` is `Layer(0)`, also `NoLayers`, `AllLayers` |
| `LayersOf(ls...)` | the union |
| `l.Has(o)`, `l.Any(o)` | every layer of `o` is in `l`; at least one is |
| `l.With(o)`, `l.Without(o)`, `l.IsEmpty()` | add, remove, empty |
| `l.String()` | `"1\|3\|4"`, or names: `NameLayers(map[Layers]string{Player: "player"})` gives `"player\|3"` (startup only, changes no behavior) |
| `c.Detects(&o)`, `c.DetectedBy(&o)`, `c.LayerOrDefault()` | the rule above, and the zero-Layer default |

For rules a mask cannot express (a bullet ignores its shooter), set `Config.Filter func(self, other ecs.Entity) bool`. It is called per detecting side after the mask test; false drops that side. It is not called for tile grids.

## Events and contacts

| Type | Sent when |
| --- | --- |
| `Enter{Contact}` | `Self` starts detecting `Other` (one per side and pair; one per collider and tile grid, however many cells touch). The contact is the pair's first this tick |
| `Stay{Contact}` | only with `Config.Stay`: every run in which `Self` still detects `Other`, after the run of the `Enter` (one per side and pair, one per collider and tile grid) |
| `Exit{Self, Other, Tiles}` | `Self` stops detecting `Other`: they no longer overlap, the mask or filter no longer accepts the pair, the tile grid was removed, or either entity was deleted or lost its `Collider` (then the entity is stale: check `World.IsAlive`) |

Events go through `system.EventsOf[collision.Enter]`, `[collision.Stay]` and `[collision.Exit]` with the usual lifetime: readable until the end of the next tick, once per reader ([events](https://github.com/weakpixel/ecs/blob/main/doc/api-systems.md#events)). Shapes that only touch along an edge do not overlap.

By default there is no `Stay` event: "still touching" is `SpaceOf(w).Contacts(e)`, the contacts in which `e` is `Self` in the last run, sorted by `Other` and cell. A hazard that hurts every tick reads that, and pairs that stay in contact (a hero on a floor, a crowd) cost no events. Set `Config.Stay` to get a `Stay` event per pair per tick instead, when a system would rather read one event stream.

| `Contact` field | |
| --- | --- |
| `Self`, `Other` | the detecting and the detected entity; `Other` is zero for a tile |
| `Normal` | unit vector from `Self` out of `Other`: the direction to push `Self` (up when `Self` stands in a floor) |
| `Depth` | penetration along `Normal`, >= 0 |
| `Tiles`, `Cell` | the grid and the solid cell, for a tile contact |

`Contacts` and `AllContacts` return slices owned by the `Space`, valid until the next run; do not change them.

## The Space and the System

`collision.SpaceOf(w)` returns the World's `Space` resource (created on first use), shared by game systems, physics and the debug overlay. `NewSystem(w, cfg)` configures it and returns the `System` (a `system.Runner`) that updates it: place it where the tick's moves are done, usually at the end of `StageSimulate`. Readers that run after it see its events in the same tick.

| `Config` field | Default | |
| --- | --- | --- |
| `CellSize` | 64 | cell size of the default grids, in world units. About twice a typical collider is good. `Raycast` also walks the broad phase in steps of this length |
| `Broadphase`, `StaticBroadphase` | a `Grid` | replace the broad phase of the dynamic or the static colliders |
| `Filter` | nil (accept) | see [above](#layers-and-masks-who-reacts) |
| `Stay` | false | also send a `Stay` event per pair per tick that stays in contact ([events](#events-and-contacts)) |

Queries and `Contacts` read the state of the last run: a collider spawned or moved this tick counts from the next run on.

`System.Run` is `Detect` (place the colliders, find the contacts) followed by `Publish` (send the events). Package physics calls `Detect` once per substep and `Publish` once per tick, so events stay once per tick and report the last `Detect`'s contacts.

### Static colliders

`Static` colliders live in their own broad phase, rebuilt only when one of them is added, removed or changed. The System still compares every static collider's Transform and Collider with the last run's each tick (a compare, no Shape or grid work), so a "static" door moved from code or the editor never leaves a stale grid. Static pairs (static vs static) are never tested, and statics are not tested against tile grids.

## Tile grids

A `TileGrid` is a grid of solid cells: `*tilemap.CollisionGrid` is one ([collision grid](tilemaps.md#collision-grid)).

```go
walls := tilemap.NewCollisionGrid(m, wallLayer)
space.AddTiles(walls, collision.TileOptions{Layer: Wall})
space.AddTiles(platforms, collision.TileOptions{Layer: Wall, OneWay: collision.OneWayUp})
space.RemoveTiles(walls) // its pairs end with an Exit on the next run
```

- A grid is on one layer (`TileOptions.Layer`, zero is `DefaultLayer`). Walls and water in different layers are two grids built from two Tiled layers. Grids never detect.
- Colliders whose mask has the grid's layer get one contact per overlapped solid cell and one `Enter`/`Exit` per grid, so walking along a floor does not flood the queue.
- A solid cell is a full box, also a `CollisionPartial` one. Slopes and per-shape tile tests are not built.
- Cell (0, 0) starts at world (0, 0): a map spawned at an offset is not supported yet.
- The grid is read every run, so cells changed with `Set` count from the next run on.

## Queries

```go
for e := range space.QueryPoint(p, collision.AllLayers) { ... }
for e := range space.QueryRect(r, Enemy) { ... }
if hit, ok := space.Raycast(eye, target, Wall); ok { ... } // line of sight
```

| Method | Returns |
| --- | --- |
| `QueryPoint(p, mask)` | colliders on a layer of `mask` that contain `p` (as `gfx.Rect.Contains` for boxes, the open disc for circles) |
| `QueryRect(r, mask)` | colliders on a layer of `mask` that overlap `r` (touching is not enough) |
| `Raycast(from, to, mask)` | the nearest `RayHit` of the segment with a collider or a solid tile cell on a layer of `mask` |
| `QueryShapes(r, mask)` | the placed `*Shape`s on a layer of `mask` whose bounds overlap `r` (valid until the next run): what a custom mover tests itself against |
| `TileGrids()` | every tile grid with its `TileOptions`, in the order added |

Queries do not search tile grids; use the grid's own lookups. Custom shapes count by their bounds. `RayHit` has `Entity` (zero for a tile), `Tiles` and `Cell`, `Point`, `Normal` (of the face entered) and `Fraction` (0..1 along the segment). A ray that only grazes a shape (along an edge, through a corner) misses it; a ray that starts inside a shape hits it at `Fraction` 0 with a zero `Normal`. Queries may nest and do not allocate when ranged over directly.

## Replacing the broad and narrow phase

The broad phase is the `Broadphase` interface (`Reset`, `Insert(Proxy, gfx.Rect, layer, mask Layers)`, `Pairs(out []Pair) []Pair`, `Query(gfx.Rect, out []Proxy) []Proxy`); its methods append to caller-owned slices, so the `Space` calls them without allocating. `Pairs` may leave out pairs in which neither side's mask holds a layer of the other, so colliders that ignore each other (a crowd of bullets) cost no narrow-phase work. `NewGrid(cellSize)` is the default: a uniform grid kept as a slice sorted by cell, rebuilt each tick without allocating; when the occupied cells fill a small rectangle it counting-sorts the entries and looks rows up in an offsets table instead of a binary search. A sweep-and-prune or a tree goes into `Config`. A `Grid` holds at most `MaxGridProxies` (2^24) proxies and clamps cells to ±2^19 per axis.

Custom shapes:

```go
const Diamond = collision.ShapeCustom
space.RegisterShape(Diamond, func(t *gfx.Transform, c *collision.Collider) gfx.Rect { ... })
space.RegisterPair(Diamond, collision.ShapeRect, func(self, other *collision.Shape) (gfx.Vec2, float32, bool) { ... })
```

`RegisterPair(a, b, fn)` also derives `(b, a)` by flipping the normal. `ShapeRect` stands for `ShapeAnimBody` and for tile cells. A `NarrowFunc` returns the normal from `self` out of `other`, the depth and whether they overlap. `Space.Shapes()` yields the placed shapes of the last run (`Shape` has the entity, kind, bounds, center, radius, layers and a copy of the collider). `ShapeOf(t, c)` places one collider like the Space does, without a run (the level editor draws colliders with it); it reports false for custom kinds.

## Debug overlay

While the engine's debug overlay is on, `extract.DebugExtract` outlines the colliders of the last run, colored by the collider's lowest layer (`collision.ColliderColor`; eight colors that repeat). Custom shapes draw as their bounds. `extract` does not import `collision`: `collision.SpaceOf` sets a `gfx.DebugOutlines` resource and `DebugExtract` draws what it yields.

## Panics

| Message | Cause |
| --- | --- |
| `collision: layer <n> out of range 0..31` | `Layer(n)` |
| `collision: NameLayers: <l> is not a single layer` | a `NameLayers` key with zero or several layers |
| `collision: Space already has a system` | a second `NewSystem` for one World |
| `collision: unknown shape kind <k>: register it with Space.RegisterShape` | a collider of a custom kind without `RegisterShape` |
| `collision: no narrow phase for shapes <a> and <b>` | two colliders whose kinds have no `RegisterPair` meet |
| `collision: shape kind <k> is built in` | `RegisterShape` with a kind below `ShapeCustom` |
| `collision: shapes <a> and <b> are built in` | `RegisterPair` with two built-in kinds |
| `collision: nil bounds`, `collision: nil narrow phase` | nil function |
| `collision: tile grid already added`, `collision: nil tile grid` | `AddTiles` |
| `collision: Grid proxy <id> out of range` | more than `MaxGridProxies` colliders in one `Grid` |

Kinds print as `rect`, `circle`, `anim-body`, `custom(<n>)`.

## Performance

`System.Run`, `Contacts`, `QueryRect`, `QueryPoint` and `Raycast` do not allocate once warm (`AllocsPerRun` tests). Measured on a Xeon 2.8 GHz (`go test -bench . ./collision/`), with moving colliders plus 10,000 static ones and a tile floor:

| Moving colliders | Spread over 4000x4000 | Clustered in 800x800 |
| --- | --- | --- |
| 1,000 | about 1.0 ms | about 7 ms |
| 10,000 | about 8.4 ms | about 98 ms (dense: most colliders overlap several others) |

The same benchmarks on a 4-core cloud box, before and after the grid, static-compare and contact-sort changes: 1,000 uniform 1.18 ms to 0.81 ms, 1,000 clustered 8.2 ms to 7.6 ms, 10,000 uniform 9.7 ms to 7.5 ms, 10,000 clustered 129 ms to 106 ms, and 5,000 bullets + 200 enemies + 10,000 statics 3.4 ms to 2.4 ms. The Xeon table above predates them.

A 5,600-unit diagonal `Raycast` through the 1,000 + 10,000 scene takes about 11 µs.

## Not built yet

Rotated shapes, polygons and capsules (possible as custom kinds), slopes and partial tile shapes, tile grids at a map offset, per-frame hit and hurt boxes as colliders, continuous detection (fast objects can tunnel through thin colliders between ticks), collision events in Lua, named layers in the editor, editor gizmos for collider bounds. Moving and resolving bodies is the job of [physics](physics.md).
