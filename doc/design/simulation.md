# Particles, tweens and hierarchy

Design of the runtime packages `particles`, `tween` and `hierarchy`.
The renderer they feed is in [engine-renderer.md](engine-renderer.md).

## Particles

`gfx.Particles` (a handle to a `*.fx.json` effect) is drawn through the batched quad path, so the renderer needed nothing new ([guide](../guide/engine/particles.md)).
Decisions:

- **Particles are not entities.**
  Thousands of short-lived entities would churn archetypes and need deferred deletes.
  A pool is a reused slice with swap-remove, owned by `particles.System` (one pool per emitter entity); the component stores only a runtime pool id.
- **Plain `KindSprite` items, no new kind.**
  `particles.Extract` pushes one item per live particle and the batcher merges them.
  If profiles show the 120-byte item copy matters (see [Items and sorting](engine-renderer.md#items-and-sorting)), a span item that carries many quads is the follow-up.
- **Deterministic randomness.**
  Each pool owns a `math/rand/v2` PCG seeded from `Seed` or the entity, so frame-log and replay comparisons stay meaningful.
- **Pool lifecycle without hooks.**
  The system marks the pools it sees in a tick and releases the unseen ones (deleted entity, removed component).
  A pool records its owner entity, so a copied `Pool` id (a cloned entity) gets its own pool instead of sharing one.
  Released pools go to a free list and keep their buffers.
- **Schedule slot.**
  The system runs in the `anim` stage after the `AnimationSystem`, which keeps running under `TickPause`, so effects play live in the editor; the extract runs in `render-prepare` right after the `SpriteExtract`.
- **The effect is an asset.**
  A `*.fx.json` file (kind `effect`) holds one to eight emitters, so effects are shared, hot-reloaded and edited as files; the component keeps only per-instance state (layer, seed, stop).
  **Several emitters, one entity:** the system keeps one pool per entity with a lane (buffer, cycle time, burst, rate carry, random stream) per emitter, so stopping, moving and overriding an effect is one operation.
  Instance overrides apply to every emitter; per-emitter overrides are not built.
  `Live` is the sum and `Done` the conjunction over the lanes, `Time` is the first lane's. Emitter `i` seeds with a value derived from the seed and `i`, so equal emitters do not spawn identical particles.
  A hot reload restarts a lane when its cycle key changed and the whole pool when the emitter count changed.
  Extract queues each lane at the component's `Layer` plus the emitter's `layer`.
  A cap of eight bounds the pool memory (each lane has its own `max`).
  Editing a file restarts a pool only when a key that shapes the cycle changed (duration, loop, burst, max, shape, shapeSize); other keys apply in place so live particles survive a drag in the tool.
  The file is not a prefab, so it has no variants.
  `bridge.Version` is unchanged: the protocol only learned the `.fx.json` suffix as an asset kind.
- **The file format is its own package.**
  `particles/fxfile` holds `Emitter`, `Spec`, `Decode` and `Encode` and imports only `gfx` and `xmath`, so the `fx` tool and `wpix mcp` check effects exactly as the loader does without linking Ebiten (as `anim/animfile` does for animations).
  It sits under `particles` because the format belongs to that runtime package.
  It keeps the `gfx` blend, shape, color and vector types instead of its own, so the `gfx.Particles` overrides and the emitter share one type; a stdlib-only copy would make `gfx` import the format package.
  Defaults stay where they are applied: the simulation reads a zero value as its default, `fxfile` only documents them.
- **Rate is accumulated, not rounded**; spawns beyond `Max` are dropped, not queued.
- **Per-instance overrides are flat optional fields.**
  `gfx.Particles` has one pointer field (`omitzero`, `nullable` in the GOD) per `Emitter` value, nil meaning "from the effect", so a level lists only what it overrides.
  A nested `Overrides` struct would add a JSON level for nothing, and a bitmask beside value fields would list every value in every level.
  `Sprite` and `Shader` are handles (the asset marker needs a non-pointer), so the zero handle means "from the effect".
  `particles.System` resolves the emitter every tick instead of caching (about 30 nil checks and a copy, 0 allocs): a base or override change of a cycle key then restarts the pool through the existing `cycleKey` comparison, and every other value applies at once.
  Pointees are immutable because clones share them.
  The fx tool edits the file, not instances.
- **The Transform applies at spawn.**
  `Rot` and `Scale` place each new particle (direction, shape, start rotation, size); live particles don't follow later turns or scaling, and `Local` follows position only.
  Following rotation would need emitter-frame offsets and blended rotation in the interpolation, which no effect has needed yet.

Not built: flipbook particles, curves and gradients, collisions, sub-emitters, trails, prewarm, editor gizmos for the spawn shape.

## Tweens

Package `tween` animates component fields ([guide](../guide/engine/tweens.md)).
It follows the particles' "state owned by a system, outside the World" pattern.
Decisions:

- **Tweens are not components.**
  A `Tweens` resource owns every running tween, so there is no archetype churn per tween, no slot limit per entity and nothing in the GOD, levels or the inspector.
  Tweens are runtime behavior started by code; authored ambient motion (a `tween:Loop` component on a level object) waits for a real need.
- **Typed lenses, one pool per lens.**
  A `Lens[C, V]` holds a getter and a lerp; each lens gets a `pool[C, V]` of value rows with swap-remove, behind a small interface so `Tweens` runs all pools in one loop.
  Per row and tick: a component lookup through a `View1[C]` the pool holds (`GetOrNil`; the generic `ecs.GetCompOrNil[C]` would look the type up on every call), the ease, the lerp, a write through the getter.
  No strings, reflection or `unsafe`, and the value type is checked at compile time.
  Lens ids are process-wide, so a `Tweens` finds a pool by slice index; `Param(i)` returns prebuilt lenses.
- **Generational IDs.**
  An `ID` indexes a slot table that points to (pool, row); swap-remove updates the slot of the row it moves.
  Freeing a slot bumps its generation, so every call on a finished or killed tween is a no-op.
  Kill and finish only mark a row dead and free its ID; the pool removes dead rows in its next run, so nothing moves while a pool iterates.
- **Overwrite at start.**
  A map from (entity, lens) to ID, touched only when tweens start and end, kills the older tween on the same field.
  Additive tweens skip it.
- **Relative and Additive through the lerp.**
  A lens has no add function.
  For vector-like values `a + b - c` is an affine combination, so two lerps compute it (`lerp(c, lerp(a, b, 0.5), 2)`); relative targets and additive steps both use it.
  It is why lerps must extrapolate, and why both options are meant for numeric values, not colors.
- **Callbacks after the pools.**
  `OnDone`, sequence release and `DespawnOnDone` are queued in a reused slice and run after every pool has run, so a callback can start and kill tweens safely.
- **Schedule slot.**
  `tween.System` runs first in the camera stage: tweens started in `simulate` run in the same tick, the follow camera sees the result, `PrevPos` was snapshotted before, and the gated stage stops tweens under `TickPause`.
  The editor's Stop clears them.

Not built: authoring components and edit-mode preview, saving running tweens, Lua bindings, lenses over resources (the camera), keyframe tracks and path following.

## Hierarchy

`hierarchy.Parent` attaches an entity to a parent ([guide](../guide/engine/hierarchy.md)).
Decisions:

- **`gfx.Transform` stays world space.**
  Extract, collision, physics and the camera read it unchanged; the child's `Transform` is derived data that a system overwrites.
  The cost is that a tween or a physics write on a child's `Transform` does not stick, and that colliders on children are a tick late.
- **Walk up, never order.**
  For each child the system collects the chain to the root in a fixed array and composes the `Local`s onto the root's `Transform`.
  It reads no intermediate `Transform`, so there is no depth tag, no sort and no child list to keep in sync with deletions: O(children x depth) with a depth cap of 32, 0 allocs.
  A cycle shows up as a chain that does not end and panics; documents are checked on load (`CheckCycles`) so data cannot cause it.
- **Layers walk too.**
  A relative ancestor's own layer is derived, so reading it would depend on visiting order; the walk adds its `LayerOffset` and goes on to the nearest ancestor whose layer is authored (absolute or root).
- **Y-sort by the root.**
  A child's `SortY` is the root's Y plus or minus 1/1024, stored by the system in `Parent` (`RootY`) and read by the sprite and nine-slice extracts through a second view, while the plain view excludes `Parent` so its loop is unchanged.
  A group of parts therefore sorts as one against other actors.
- **A stage of its own, ungated.**
  It sits between `post` and `anim`, so the placement sees the tick's movement, and runs in edit mode so the editor moves children with their parent.
  After a document spawns, the assets stage runs it once early (only when something spawned), so children are placed before the snapshot.
- **Absolute placement is a mode, not a missing component.**
  `Local.PlaceMode` (`PlaceAbsolute`) keeps `Local` inside `Parent`, so the mode toggles in place without an archetype move or a file migration, and it is an enum like `LayerMode` so an upright mode can be added later.
  A child without `Transform` cannot be the absolute case (everything that draws or collides reads `Transform`), and splitting `Local` into its own component was declined for the migration and archetype moves it costs.
- **Absolute children are placed once, and `Local` is never written.**
  The first placement composes `Local` as for a relative child, which is what makes prefab parts and level instances start in the right place without the prefab code knowing about the hierarchy (`god` does not import it).
  After that the `Transform` is authored, so the walk reads it as a root and the visiting order stays irrelevant.
  Mirroring `Local` from the `Transform` each tick was declined: `Local` would have two meanings, and `SetPlace(..., keepWorld)` gives the no-jump switch back to relative on demand.
- **Orphans are deleted by the system**, from the chain walk, after the loop, so a whole subtree goes in one pass without child lists.

Not built: running the propagation inside physics substeps, a physics warning for bodies on children, inheriting `FlipX`, ordered children, named paths, relationship pairs, editor support, Lua bindings.
