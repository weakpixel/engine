# Engine Runtime guide

The Engine Runtime is the 2D game layer of the Weakpixel engine, built on the [ECS](https://github.com/weakpixel/ecs) (module `github.com/weakpixel/ecs`) and [Ebiten](https://ebitengine.org). Import path: `github.com/weakpixel/engine`, part of the root Go module. Only the Engine Runtime packages that say so import Ebiten; the ECS never does ([dependency rules](../../design/architecture.md)).

**New game?** `go run github.com/weakpixel/engine/tools/wpix@latest new mygame` creates a project that already runs, tests, builds for the browser and opens in the level editor ([wpix new](../tools.md#new)); its code is the shortest tour of the pieces below.

**Start with [`game.Engine`](engine.md)**: one struct that wires the World, assets, renderer, camera and schedule and runs with `ebiten.RunGame`. The other pages explain the pieces it is made of.

| Page | Read it to learn |
| --- | --- |
| [engine.md](engine.md) | `game.Engine`: config, tick order, levels, escape hatches |
| [getting-started.md](getting-started.md) | The smallest game wired by hand from the low-level pieces: schedule, resources, `Update`/`Draw` |
| [components.md](components.md) | `gfx`: `Transform`, `Sprite`, `Animator`, `Label`, ... and the zero-value rules |
| [asset-kinds.md](asset-kinds.md) | What each asset kind is (sprite, atlas, clip, font, ...), what it is for, how to load it, file formats |
| [asset-library.md](asset-library.md) | The standard asset folder layout: packages, `global/` and `boot/`, `LoadLevelBase`, `LoadPackage`, packs, `wpix assets` |
| [assets.md](assets.md) | The engine's asset server, game-facing handles, hot reload and HTTP (the asset system itself: [`github.com/weakpixel/asset`](https://github.com/weakpixel/asset/blob/main/doc/guide.md)) |
| [rendering.md](rendering.md) | `extract` + `render`: queue, camera, layers, interpolation, culling, writing an extract system |
| [camera.md](camera.md) | `camera`: the follow camera, its modes (`default`, `smooth`, `deadzone`, `spring`, `lookahead`) and how to combine them, bounds, snapping |
| [shaders.md](shaders.md) | Kage shaders on sprites: how to write one, tested examples, driving `Params`, costs |
| [tilemaps.md](tilemaps.md) | `tiled` + `tilemap`: Tiled maps, tilesets and worlds, what is supported, the map data API, the collision grid, spawning, parallax, flips |
| [input.md](input.md) | `input`: actions, bindings files, mapping contexts, players and devices, rebinding and profiles, touch, replay |
| [audio.md](audio.md) | `sound`: effects, music crossfades, buses, positional sound, `SoundEmitter` |
| [speech.md](speech.md) | `speech`: speaking entities and dialogue boxes with the voices of the voice library, events, pausing |
| [math.md](math.md) | `xmath`: vectors, rectangles, transform matrices, geometry, random numbers, noise, interpolation, angles, smoothing, springs, easing |
| [particles.md](particles.md) | `particles`: `gfx.Particles`, the `*.fx.json` effect file and its keys, how the pools work, hot reload, authoring |
| [tweens.md](tweens.md) | `tween`: animating fields over time (move, scale, fade, shader params), easing, sequences, overwrite, your own fields |
| [collision.md](collision.md) | `collision`: colliders, layers and masks (who reacts), `Enter`/`Exit` events, contacts, point/rect/ray queries, tile grids, replacing the broad or narrow phase |
| [physics.md](physics.md) | `physics`: `Body`, `Velocity`, `Touching`, `Material`, the solver, tiles, one-way and moving platforms, `Follow` with tweens, `MoveAndSlide`, `NewMoveSystem`, hooks |
| [hierarchy.md](hierarchy.md) | `hierarchy`: attaching an entity to a parent, local placement, layers and Y-sorting of children, deleting subtrees |
| [rooms.md](rooms.md) | `room`: exits and spawn points placed in the editor, moving between levels, fade, iris and custom transitions |
| [animation.md](animation.md) | `anim`: Aseprite clips, `*.anim.json` definitions, hit/hurt boxes, `wpix anim` |
| [game-objects.md](game-objects.md) | Authoring levels and prefabs as GOD: `resolver`, asset fields, runtime fields |
| [editor.md](editor.md) | `editor`: the level editor core (a `game.Tool`): applying a level document by id, picking and dragging, Play/Stop, the protocol bridge, making a game editable in VSCode |
| [web.md](web.md) | Running a game in the browser: building to wasm, the web shell, assets over HTTP, browser differences |
| [kinds.md](kinds.md) | Every "kind" in one place: asset kinds, item kinds, shapes, loop modes, spaces, blends |

## Packages

| Package | Job |
| --- | --- |
| `game` | `Engine`: implements `ebiten.Game` and wires everything below. Start here. |
| `xmath` | Float32 math (root package, standard library only): `Vec2`, `Rect`, `Mat3`, ray and shape tests, seeded `Rand`, noise, interpolation, angles, frame-rate independent smoothing and springs, easing curves. `gfx.Vec2` and `gfx.Rect` are aliases of its types. Guide: [math.md](math.md). |
| `gfx` | Components (`Transform`, `Sprite`, `ScreenSprite`, `Animator`, `Label`, `DebugShape`, `SoundEmitter`, `Particles`, `PlayerSlot`), value types (`Vec2` and `Rect` as aliases of `xmath`, packed `Color`, `Box`), and the handles components hold (`SpriteHandle`, `FontHandle`, `ClipHandle`, `ShaderHandle`, `SoundHandle`, `MusicHandle`). `Transform`, `Sprite`, `Animator`, `Label`, `SoundEmitter` and `Particles` are Game Object components. |
| `github.com/weakpixel/asset` | Handle-based asset system (its own module): `Handle[T]`, `Store[T]`, `Server`, `Scope`, background loading, dependencies, placeholders; `httpfs` for browser builds. |
| `asset/library` | The opt-in asset library: folder convention, groups, catalog, zip packs mounted per scope, overlays. Standard library and the asset module only. |
| `render` | Asset types and loaders (`Texture`, `Sprite`, `Atlas` (grid sheet, Tiled `*.tsj`, TexturePacker), `FontSource`, `Font`, `Shader`), the game-facing sprite, font, shader and atlas API, `Queue`, `Camera`, `LayerConfig`, `Renderer`, `Layout`. |
| `extract` | Systems that turn components into queue items during `Update`, plus `AssetSystem` and `SortQueue`. |
| `camera` | Follow camera: the `Target` component, `Rig` (follow modes, bounds, snapping) and the `System` that moves `render.Camera`. |
| `anim` | `Clip`/`ClipSet` assets from Aseprite exports and `*.anim.json` definitions, `AnimationSystem`, `ShapesOf`. |
| `anim/animfile` | Parsing and checks of the animation file formats, shared by `anim` and `wpix anim`. |
| `tiled` | The Tiled JSON file model (`.tmj`, `.tsj`, `.world`) and its checks; standard library only. |
| `tilemap` | `Tileset`/`Map`/`World` assets, the `TileLayer` component, `Spawn`/`SpawnWorld`, `TilemapExtract`, the map data API, `CollisionGrid`. |
| `particles` | `System` (simulates `gfx.Particles` pools) and `Extract` (one sprite item per live particle). |
| `particles/fxfile` | Parsing, checks and writing of the `*.fx.json` effect file, shared by `particles`, the `fx` tool and `wpix mcp`. No Ebiten. |
| `tween` | `Tweens` (the resource that owns running tweens), `To`/`From`, `Lens` and the built-in lenses, `Ease`, `Sequence`, `System`. Standard library, `ecs`, `ecs/system`, `gfx` and `xmath` only. |
| `collision` | Opt-in collision detection: the `collision:Collider` component, layer helpers, `Space` (contacts, queries, tile grids), `System`, `Enter`/`Exit` events, the `Grid` broad phase. Standard library, `ecs`, `ecs/system`, `gfx` and `xmath` only; `game` never adds it. |
| `physics` | Opt-in physics on top of `collision`: the `Body`, `Velocity`, `Touching`, `Material` and `Follow` components, the `System` (integration, impulse solver, one-way and moving platforms), `MoveAndSlide`, `MoveSystem`, the `Impact` event. Standard library, `ecs`, `ecs/system`, `gfx`, `xmath`, `collision` and `tween` only; `game` never adds it. |
| `hierarchy` | `hierarchy:Parent` and its `Local`, the `System` that writes children's world `Transform` and layers and deletes orphans, `Attach`, `Detach`, `Delete`, `Children`, `WorldOf`, `CheckCycles`. Imports `ecs`, `ecs/system`, `gfx`; `extract` and `game` import it. |
| `room` | Opt-in rooms: `Install`, `Rooms` (`GoTo`, `Start`, `SetCurrent`), the `room:Exit`, `room:Spawn` and `room:Traveler` components, `Transition` (`Fade`, `Cut`, `Func`, `Shader`). Imports `game`; `game` never imports it. |
| `input` | Actions, bindings files (`ParseBindings`, loader), `Router`, `Player`, `State`, profiles and rebinding, names and glyph keys, `Recorder`/`ReplaySource`. Standard library and `asset` only. `input/ebiteninput` is the Ebiten source; `input/inputsys` has `InputSystem` and `TouchControlsExtract`. |
| `sound` | `Sound`/`Music` assets (WAV, Ogg, MP3), `Queue`, `Mixer` and buses, `Listener`, `AudioSystem`, the Ebiten and null backends. |
| `speech` | Opt-in speech with the `github.com/weakpixel/voice` library: `Install`, `Speech` (`Say`, `Play`, `Next`, ...), the `speech:Speaker` and `speech:DialogueBox` components, voice, language and dialogue assets, `Event`. Imports `game`; `game` never imports it. No Ebiten calls. Guide: [speech.md](speech.md). |
| `resolver` | The `types.AssetResolver` that turns asset keys in GOD files into handles and back. |
| `assetkind` | The registry of asset kinds the resolver looks up; each asset package registers its own. |

Which package imports which, and why: [design/architecture.md](../../design/architecture.md#engine-runtime-packages).

## Ground rules

- Components hold **handles and packed values**, never Ebiten images or pointers.
- A zero, missing, loading, failed or stale handle draws a **placeholder** (a magenta checker by default). Never nothing, never a panic.
- `Draw` never touches the `World`: systems fill a `render.Queue` during `Update`, and `Draw` reads only the queue, the camera and the asset stores ([the frame contract](getting-started.md#the-frame-contract)).
- Zero-valued components draw normally: `Color` 0 is white, `Transform.Scale` zero is (1,1), `Camera.Zoom` zero is 1 ([zero values](components.md#zero-values-draw-normally)).
- Programmer errors panic (`asset: `, `render: `, `game: `, `input: `, `collision: `, `physics: `, `ecs: `). Bad files put an asset in `Failed` and go to `Options.OnError`.

## Performance

Measured on an Intel i7-9750H (`go test ./... -run '^$' -bench . -benchmem` at the repository root):

| Work | 10k entities | Allocs |
| --- | --- | --- |
| Full render stage: snapshot, reset, extract, sort | about 0.92 ms | 0 |
| Sorting shuffled items | about 1.45 ms | 0 |
| `AnimationSystem` | about 0.30 ms (23 µs for 1k) | 0 |
| `TilemapExtract`, 256x256 map, 4 layers, 3,840 visible tiles | about 80 µs | 0 |
| The same map [chunked](tilemaps.md#chunk-pre-rendering): 32 chunk items | about 1 µs (whole frame with sort and draw: about 107 µs instead of 955 µs) | 0 |

`particles.System` and `particles.Extract` ([particles.md](particles.md)), 10,000 live particles (Xeon 2.8 GHz, `go test -bench Particle ./particles/`): the simulation takes about 72 µs for one emitter and about 79 µs for 100 emitters of 100, the extract about 375 µs (100 emitters: 390 µs); 0 allocs.

`tween.System` ([tweens.md](tweens.md)), 10,000 running tweens (Xeon 2.1 GHz, `go test -bench Tween ./tween/`): about 0.34 ms for 10k on `Pos` with `InOutSine`, about 0.24 ms for 10k spread over `Pos`, `Scale`, `Tint` and `Param(0)` on 2,500 entities; 0 allocs. Each pool looks its component up through a view, so the type is resolved once per pool, not per tween.

`sound.AudioSystem.Run` allocates nothing in steady state either; starting a voice allocates its Ebiten player.

Sprites (shaded or not) and shapes are drawn with reused options and allocate nothing; text may allocate (Ebiten's text layout). Unshaded sprites, tiles and shapes are batched into one `DrawTriangles` call per run of quads with the same texture and blend mode, which makes `Draw` of 10k sprites about 5x cheaper on the CPU than one `DrawImage` per sprite ([rendering guide](rendering.md#batching)). `ShapesOf`, `Box.World` and the asset stores' `Get` are 0 allocs.
