# Scenes, rooms and the startup screen

Design of the scene stack (`game`), room transitions (`room`) and the startup screen.
The API is in the [engine guide](../guide/engine/engine.md#scenes) and [rooms.md](../guide/engine/rooms.md).

## Startup screen

The startup screen ([guide](../guide/engine/engine.md#startup-screen)) lives in `game` because it draws with Ebiten.
Its timing is a small Ebiten-free state machine (`startupSeq`) tested on its own: a screen starts leaving once its `Duration` minus the fade has passed (or on a skip), and the last one only once the initial load is done, so a fast load ends exactly at `Duration`.

Holding the game reuses the `gate` that a Tool's `TickPause` uses: the gate is installed when a Tool is set or the startup screen is on, and while held it runs only the engine's input system, so device state stays current for the skip press.
The press is read from the router's raw frame as the change from nothing held to something held.
The screen is drawn in `DrawFinalScreen` at window resolution, so a large image stays sharp in a small logical resolution, and `Draw` skips the half-loaded game.
Time is simulation time, which keeps tests deterministic.
Everything is dropped when the last screen ends, so the steady-state tick stays 0 allocs; the readiness check walks scope closures and may allocate, only while the screen is up.

The default image is a JPEG re-encode of the original artwork (720x720, quality 85, about 50 KB; `images/weakpixel-screen.png` is a 640x640 palette PNG of about 165 KB for the README); `game/startup/README.md` says how to redo it.

## Rooms

Package `room` moves the game between levels through exits ([guide](../guide/engine/rooms.md)).
It sits on top of `game` and is opt-in.
Decisions:

- **Exits and spawns are GOD components**, not Tiled objects, so the editor validates and places them with prefabs and overrides; spawn names also match Tiled objects, and game code can still fire `GoTo` from Tiled areas.
- **The entering edge comes from `PrevPos`.**
  The engine snapshots `PrevPos` before the game's systems, and the director runs in `StagePost`, so `Contains(Pos) && !Contains(PrevPos)` is "walked in this tick" with no per-traveler state.
  Placing sets both, so a spawn inside an exit does not fire it.
- **Load once covered.**
  The target group loads when `Out` ends, not during it: its map spawns at its world position and would show through a half-covered old room.
  The swap (detach travelers with `Level.SetObjects`, unload, `OnEnter`) happens as soon as the group is ready; placing waits for every level of the new room, so `OnEnter` can add levels (`SetCurrent`) that hold the spawn.
- **Hold only simulate.**
  `Engine.Hold` skips `StageSimulate` alone, so tweens, the follow camera and input keep running and transitions can animate with them.
  It wraps the slot around the gate of the Tool and the startup screen, so either can hold the stage; a held tick costs one branch.
- **Shader transitions borrow a post pass** that the director appends at `Begin` and removes at `End`, and find again by shader handle if game code changed the chain.
- **The timing is a separate state machine** (`seq`, one event per tick), tested without an engine.

Not built: slide transitions with both rooms visible, preloading during `Out`, saved room state, an editor gizmo beyond the debug shape, Lua bindings.

## Scenes

`game.Scene` stacks the top-level screens of a game ([guide](../guide/engine/engine.md#scenes)).
Decisions:

- **One World, scene-owned systems and entities.**
  Resources (camera, tweens, input router, assets) stay shared, and moving the hero between scenes needs nothing.
  Isolation comes from ownership and from which systems run, not from separate storage.
- **The root scene is the Engine's own systems.**
  `Engine.AddSystem` fills the root, which is always the bottom of the stack, so a game without scenes changes nothing.
  It follows the same rules as any scene below the top: a pause scene with `UpdateBelow: false` freezes it.
- **Applied between ticks.**
  `Push`, `Pop` and `Replace` queue; `Update` applies the queue at its start, after the Tool tick and before the schedule runs.
  A scene's runners are initialized outside the schedule (which can only append, and initializes what it was built with), with a context built for that call, so the first push of a scene before the very first tick runs `Init` before the root's.
- **A dispatcher per stage.**
  The schedule is built once, so each of the four game stage slots holds one dispatcher runner (after the engine's input system, tweens and follow camera) that ranges over a list rebuilt when the stack changes: the root's runners, then the active scenes', and for the input stage only those allowed by `InputBelow`.
  Active and editing lists (for a Tool's pause, split once per rebuild by `EditRunner`) are precomputed, so a tick allocates nothing.
  The dispatcher sits inside the existing gates (Tool pause, startup, `Hold`), which therefore apply unchanged.
- **Engine stages are not per scene.**
  Assets, snapshot, tweens, camera follow, animation, particles, extraction and audio run once, as under `Engine.Hold`.
  Per-scene time, tweens or animation freezing would need those stages to know the scene of an entity; deferred.
- **Drawing below by a layer floor.**
  Everything draws into one queue and an entity does not know its scene, so `DrawBelow: false` uses the scene's `Layer`: `Queue.Sort` drops the items below the floor of the topmost such scene (counted as culled), one compare per item and no change to the extract systems.
  The cost is the layer-band convention in the guide.
- **Rooms** take an optional scene (`room.Config.Scene`): the director is added to it and rooms load through it, so replacing the gameplay scene unloads the room.
  Editor and Tool need nothing: the editor opens levels in the Engine (root-owned) and scenes keep their stack while a Tool pauses.

Not built: one World per scene and scene-local resources, scene transitions (use `PostChain.Fade` or rooms), per-scene time, saving a popped scene's state, scenes in GOD files and Lua bindings.
