# Collision and physics

Design of the `collision` and `physics` packages.
The API is in the [collision](../guide/engine/collision.md) and [physics](../guide/engine/physics.md) guides.

## Collision

Package `collision` detects overlaps ([guide](../guide/engine/collision.md)); package `physics` moves bodies with the same `Collider` and `Space` ([guide](../guide/engine/physics.md)).
Decisions:

- **Detection and physics are separate packages over one data model.**
  A trigger, a coin or a hit area needs only a `Collider`; a body adds a physics component.
  The shape and the filter live once.
- **"Mask is what I react to", one event per detecting side.**
  Game code never swaps A and B: the actor is always `Self`.
  Physics uses the same `Detects` rule for who is pushed.
- **No hash map in the broad phase.**
  The grid is a slice of packed `uint64` entries (row, column, proxy) sorted each tick: rebuilding reuses memory, sorting plain integers is several times faster than sorting structs with a comparison function, and a row's cells are consecutive keys, so a query does one binary search per row.
  A pair that shares several cells is reported only by the cell holding the top-left corner of the two bounds' intersection.
- **Statics are compared, not flagged.**
  The System compares each tick's static shapes with the last run's and rebuilds their grid only on a difference, so moving a "static" collider never needs an explicit call.
- **No Stay event by default.**
  Ongoing contact is state, read with `Space.Contacts`; a per-tick event for every held pair would fill the queue for floors and crowds.
  `Config.Stay` opts in, sent from the same merge as Enter and Exit.
- **Enter/Exit is a merge of two sorted key lists** (this run's detecting pairs against the last run's), no map.
  Contacts are sorted by `Self`, which also makes `Contacts(e)` a binary search.
- **Rays walk the broad phase in cell-long pieces** and stop at the first piece that ends behind the best hit, instead of querying the ray's bounding box (a long diagonal ray would test every collider in the square it spans).
- **The anim body is copied by `anim/animbody`, not read by `collision` and not written by `anim`,** so `collision` stays free of `anim` and `render` and `anim` stays free of `collision`; the fixed `body` is used, never per-frame boxes, so a solid box does not change shape mid-animation.

Not built: rotated shapes and polygons, slopes and partial tiles, tile grids at a map offset, per-frame hit and hurt boxes as proxies (several per entity, not entities), continuous detection, Lua bindings, named layers in the editor.

## Physics

Package `physics` moves bodies on top of `collision` ([guide](../guide/engine/physics.md)).
Decisions:

- **Physics drives detection.**
  `collision.System` is split into `Detect` and `Publish`; physics detects once per substep and publishes once per tick, so events stay once per tick and detection code exists once.
- **Upright bodies, dynamic and kinematic only**.
  Dropping rotation removes the hardest part of a 2D solver; pixel-art bodies stay upright.
  Static is a `Collider` without a `Body`.
- **Detect, solve velocities, then move** (the Box2D order).
  Contacts found at the start of a substep are resolved on the velocities before positions move, so gravity never pushes a resting body into its floor; the cost is that contacts and flags lag positions by one substep.
- **Sequential impulses with warm starting, then position correction.**
  Accumulated impulses are kept per contact between substeps (a map keyed by pair and tile cell), so a stack settles with the default four passes.
  Position correction takes the largest push per direction and axis instead of the sum, so a box on two tile cells is not lifted twice.
- **Mirrored contacts become one constraint.**
  When both sides detect each other, collision reports two contacts; solving both would push the pair twice.
- **Internal tile edges are dropped** by checking the neighbor cell in the normal's direction, the classic fix for snagging on tile seams.
- **One-way contacts persist.**
  A one-way contact is accepted when the body is no deeper than it moves in a substep, and kept while it lasts, since position correction leaves a body that landed deep inside for a few substeps.
- **Riders get platform-relative velocity.**
  A dynamic body on a kinematic body moves by the platform's velocity and keeps its own velocity relative to it, switching frames when its ground changes.
  Game code that writes `Velocity` every tick then walks on a moving platform as on solid ground, and leaving it keeps the platform's momentum.
  A stack on a kinematic body shares its frame: `carrierOf` follows the grounds down the stack (at most 8 bodies, which also ends a cycle) to the kinematic body, so the bodies in it keep their relative velocities when the platform changes speed.
  Riders on dynamic bodies no platform carries use friction instead: zeroing a dynamic carrier's velocity in the contact would break momentum exchange in stacks.
  `MoveAndSlide` movers are carried from their `Touching.Ground`, as the last move wrote it, by the step of the body they stand on.
- **No per-body resolver**: a contact has two sides, and mixed resolvers in one pile do not converge.
  `Material`, `MoveAndSlide` and `PreSolve` cover per-object behavior; `Config.Resolver` replaces the solver for the whole World.
- **`Follow` instead of tweening positions**: tweens run after physics, so a tweened position is a teleport.
  `Follow` turns the tween's target into a velocity at the start of the tick.
- **`MoveAndSlide` is axis-separated and reads the Space only.**
  It is what character controllers in pixel-art games use (X then Y, stop flush), it works with detection alone, and it never touches the Space's proxies.
  It reads the candidates from the Space with a 16 unit margin and tests the current bounds of non-static colliders, because bodies move after detection.
  An overlap of up to one unit blocks (the mover stays where it is) so a body pushed into it or a platform rising into it does not let it fall through; deeper overlaps pass so it can always get out.
- **`MoveAndSlide` pushes by displacement, not impulse.**
  With `MoveOptions.Push`, it moves the pushed bodies' positions in the same call, through a sweep with each body's own collider, and leaves their velocity alone.
  An impulse would land one physics step later, by when the mover overlaps the body by more than the one unit it tolerates and walks through it.
  Leaving the velocity alone gives the usual platformer crate that stops when the player stops; friction and gravity still act on it in the solver.
  Mass enters as a speed share (`Push / (Push + M)`) instead of a threshold, so heavy crates feel heavy without a cutoff.

Not built: rotation, joints, sleeping, continuous collision, slopes, carrying riders on dynamic bodies no platform carries, bodies pushing `MoveAndSlide` movers, per-level gravity, a level-editor path mover, Lua bindings.
The pile benchmark spends about a fifth of its time in map lookups (warm-start and impact maps); a slice-based contact cache is the next step if it matters.
