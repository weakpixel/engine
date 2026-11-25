# Engine Runtime packages

What each Engine Runtime package does and depends on.
Modules, root packages and the cross-cutting rules are in [architecture.md](architecture.md).

## Engine Runtime packages

```
asset (stdlib) ◄── asset/library (stdlib, asset)    anim/animfile (stdlib)    tiled (stdlib)
   ▲                    ▲                     ▲
  gfx (ecs, god via generated code, asset, xmath)    prefab (ecs, god, asset, gfx)
   ▲         ▲
   │      particles/fxfile (gfx, xmath)
   │
  render (Ebiten, asset, gfx, tiled)     never imports ecs       sound (Ebiten audio, ecs, ecs/system, asset, gfx)
   ▲                                                                ▲
  extract, anim, tilemap, particles (ecs, ecs/system, render, ...)   │
   ▲                                                                │
  game (everything, asset/library) ◄──────────────────────────────────┘
   ▲
  editor ◄── editor/bridge ◄── editor/host

  editor/hostlink (stdlib) ◄── editor/bridge, editor/host, editor/toolshell, tools/wpix-editor

  assetkind (stdlib, asset) ◄── render, anim, sound, prefab, particles, speech
         ▲
  resolver (ecs, god, asset, assetkind) ◄── game

  input (stdlib only) ◄── input/ebiteninput (Ebiten)
         ▲
  input/inputsys (ecs, ecs/system, gfx, render) ◄── game

  camera (ecs, ecs/system, gfx, render, extract) ◄── game

  hierarchy (ecs, ecs/system, god via generated code, gfx) ◄── extract, game

  tween (ecs, ecs/system, gfx) ◄── game

  collision (ecs, ecs/system, gfx) ◄── anim, extract, tilemap   (game never imports it)
         ▲
  physics (ecs, ecs/system, gfx, collision, tween)              (game never imports it)

  game ◄── room (ecs, god, extract, gfx, input, render, tilemap, tween)
  game ◄── speech (ecs, ecs/system, god, asset, assetkind, gfx, sound, github.com/weakpixel/voice)
```

| Package | Imports | Why it sits there |
| --- | --- | --- |
| `asset` (module `github.com/weakpixel/asset`) | stdlib only | Handles, stores, scopes, loaders, hot reload. Testable with `fstest.MapFS` and fake loaders, without a GPU. Asset types that need Ebiten plug in through `Loader`. Design: [asset design](https://github.com/weakpixel/asset/blob/main/doc/design.md), engine side: [asset-system.md](asset-system.md) |
| `asset/library` | stdlib, the asset module | The asset library: folder convention, catalog, zip packs in a mount table. Knows kinds by file name, not their types, so it stays free of Ebiten; `game` maps kinds to loaders |
| `anim/animfile` | stdlib only | The animation file formats, shared by the `anim` loader and `wpix anim`, so the tool checks exactly what the loader accepts without Ebiten or ECS |
| `sheetfile` | stdlib only | The grid sheet format (`*.sheet.json`: `Decode`, `Encode`, the frame layout), shared by the atlas loader in `render` and the `sheet` tool |
| `particles/fxfile` | `gfx`, `xmath` | The particle effect format (`*.fx.json`: `Emitter`, `Spec`, `Decode`, `Encode`), shared by the `particles` loader, the `fx` tool and `wpix mcp`, so all three accept the same files without Ebiten. It keeps the `gfx` enum, color and vector types so `gfx.Particles` overrides and the emitter share one type (`internal/deps` limits it to these imports) |
| `tiled` | stdlib only | The Tiled JSON formats (`.tmj`, `.tsj`, `.world`), shared by the `render` atlas format, the `tilemap` loaders and future tools |
| `gfx` | `asset`, `ecs`, `god`, `xmath` | Plain-data components and the handles they hold. No Ebiten, so tools never compile a renderer. `ecs` and `god` come in only through the generated Game Object code (`zz__gen.go`) that registers `Visual`, `AnimatedVisual`, `Text`, `SoundSource` and `ParticleSource` |
| `render` | Ebiten, `asset`, `assetkind`, `gfx`, `sheetfile`, `tiled` | Asset types and loaders, `Queue`, `Camera`, `Renderer`. Never imports `ecs`: it draws a list of plain items. `tiled` gives the atlas loader its `*.tsj` format. |
| `extract` | `ecs`, `ecs/system`, `asset`, `collision`, `gfx`, `hierarchy`, `render` | The bridge: systems that read components in `Update` and fill the queue |
| `anim` | `ecs`, `ecs/system`, `asset`, `assetkind`, `gfx`, `render`, `anim/animfile` | Clip assets and the `AnimationSystem`. No Ebiten calls of its own |
| `tilemap` | `ecs`, `ecs/system`, `asset`, `collision`, `gfx`, `render`, `extract`, `tiled` | Tiled `Tileset`, `Map` and `World` assets, the `TileLayer` component, `Spawn` and the `TilemapExtract` (which reads `extract.Screen`). No Ebiten calls of its own |
| `particles` | `ecs`, `ecs/system`, `gfx`, `render`, `extract`, `asset`, `assetkind`, `particles/fxfile` | The effect asset (`*.fx.json` loader over `particles/fxfile`) and the `System` that simulates `gfx.Particles` pools and the `Extract` that queues one sprite item per live particle. Particles are pool entries, not entities. No Ebiten calls of its own (`internal/deps` checks it) |
| `sound` | Ebiten `audio` (and its decoders), `ecs`, `ecs/system`, `asset`, `assetkind`, `gfx` | `Sound` and `Music` assets, `Queue`, `Mixer`, `Listener`, the `AudioSystem` and its `Backend`s. Independent of `render`: audio needs no GPU, and a headless server can run it on the `NullBackend` |
| `camera` | `ecs`, `ecs/system`, `gfx`, `render`, `extract` | The follow camera (`Target`, `Rig`, `System`) and its trauma shake (`Rig.Shake`, `Rig.AddTrauma`). Its own package because `render` never imports `ecs` and `game` stays thin; the follow math is plain functions on `gfx.Vec2`, tested without a window. Never imports `game` |
| `tween` | `ecs`, `ecs/system`, `gfx`, `xmath` | `Tweens`, the resource that owns every running tween in typed pools, its typed `Lens`es and the `System` that advances them. Tweens are pool entries, not components. Headless: no `render`, no Ebiten (`internal/deps` checks it). Design: [engine-renderer.md](simulation.md#tweens) |
| `collision` | `ecs`, `ecs/system`, `gfx`, `xmath` | Collision detection: the `collision:Collider` component, the `Space` resource (broad phase, narrow phase, contacts, queries, tile grids) and the `System` that sends `Enter`/`Exit` events. Headless. `tilemap` (the `TileGrid` assertion) may import it; `game`, `anim` and `extract` do not, so it stays opt-in (`internal/deps` checks them). `extract` draws collider outlines through the `gfx.DebugOutlines` resource that `collision.SpaceOf` sets, and the anim-body sync is `anim/animbody`. Guide: [collision.md](../guide/engine/collision.md) |
| `physics` | `ecs`, `ecs/system`, `gfx`, `xmath`, `collision`, `tween` | Bodies, velocity, materials, the impulse solver, `MoveAndSlide`. Drives the World's `collision.System` (detect per substep, publish per tick). Imports `tween` only for the `FollowTarget` lens. Headless and opt-in like `collision` (`internal/deps` checks it). Guide: [physics.md](../guide/engine/physics.md) |
| `assetkind` | stdlib, `asset` | The registry of asset kinds (`sprite`, `clip`, `voice`, ...): each owner (`render`, `anim`, `sound`, `prefab`, `particles`, `speech`) registers how to load a key of its kind and how to find a handle's key again. A leaf so that `render`, which never imports `ecs`, can register its kinds (`internal/deps` checks it) |
| `resolver` | `ecs`, `god`, `asset`, `assetkind` | `types.AssetResolver` on an `asset.Server`. Knows no kind itself; it looks them up in `assetkind` (`internal/deps` checks it) |
| `input` | stdlib only | Actions, bindings files, `Router`, players, profiles, rebinding, recording. Headless: tested and replayed without Ebiten. Design: [input.md](input.md) |
| `input/ebiteninput` | Ebiten, `input` | The Ebiten `input.Source`: the only Ebiten import of the input system |
| `input/inputsys` | `ecs`, `ecs/system`, `asset`, `gfx`, `render`, `input` | `InputSystem` and `TouchControlsExtract`: the router as a World resource, touch regions as draw items; and the bindings asset (loader, handle, store) |
| `httpfs` (module `github.com/weakpixel/asset`, package `httpfs`) | stdlib only | A read-only `fs.FS` over HTTP and the change-event reader for browser builds. See [web.md](../guide/engine/web.md) |
| `prefab` | `ecs`, `god`, `asset`, `assetkind`, `gfx` | Prefab assets (`*.prefab.json`), `Store.Spawn` and `Source` (the engine's `types.PrefabSource`). No Ebiten, no render |
| `game` | all of the above, Ebiten | `Engine`, the default wiring |
| `hierarchy` | `ecs`, `ecs/system`, `god` (generated code), `gfx` | The `hierarchy:Parent` component and the `System` that composes children's world `Transform` and layers from the parent chain. Its own package because `gfx` imports `ecs` only through generated code; `extract` reads `Parent.RootY` and `game` installs the system. No Ebiten, no `render` (`internal/deps` checks it). Guide: [hierarchy.md](../guide/engine/hierarchy.md) |
| `room` | `ecs`, `ecs/system`, `god`, `extract`, `game`, `gfx`, `input`, `render`, `tilemap`, `tween` | Rooms and transitions on top of `game`: the director `Rooms` (a `StagePost` runner), the `room:*` components and the `Transition`s. Opt-in, so `game` never imports it (`internal/deps` checks it). No Ebiten calls of its own. Guide: [rooms.md](../guide/engine/rooms.md) |
| `speech` | `ecs`, `ecs/system`, `god` (generated code), `asset`, `assetkind`, `game`, `gfx`, `sound`, the core module `github.com/weakpixel/voice` | Speaking entities and dialogue boxes: the voice, language and dialogue assets, the `speech:*` components and the `Speech` controller (a `StagePost` runner plus an audio-stage check). The only package that imports the voice library, which has no dependencies; the synthesizer plays through a `sound` stream, so it makes no Ebiten calls. Opt-in, so `game` never imports it; it registers its asset kinds in `assetkind`. Guide: [speech.md](../guide/engine/speech.md) |
| `editor` | Ebiten, `ecs`, `god`, `asset`, `extract`, `game`, `gfx`, `input`, `prefab`, `render`, `resolver` | The level editor core: `Session`, a `game.Tool`. Design: [editor.md](editor.md) |
| `editor/host` | `httpfs`, `camera`, `editor`, `editor/bridge`, `editor/hostlink`, `game`, `tilemap` | The entry of a game's editor build: bridge, workspace assets, the level and prefab stages |
| `editor/hostlink` | stdlib only (`syscall/js` in the browser) | What a game or tool under an editor host shares with it, whichever protocol it speaks: the protocol version (`hostlink.Version`, which is `bridge.Version`), the `Transport` interface, the connection to the host page and the settings the host passes (stage, tool, first file, asset root, file index; natively the `WEAKPIXEL_STAGE`, `WEAKPIXEL_PATH` and `WEAKPIXEL_HEADLESS` variables). A leaf, so a tool links neither the bridge nor the level editor (`internal/deps` checks it) |
| `editor/bridge` | `ecs`, `god`, `anim`, `editor`, `editor/hostlink`, `game`, `gfx`, `prefab` | The JSON protocol between a `Session` and a host (the VSCode extension through the host page, or a native host over TCP) |
| `editor/camzoom` | `gfx`, `render` | The wheel zoom of the editors' cameras: a fixed factor per wheel unit, eased toward the target around the cursor. Shared by `editor` and `editor/toolshell` |
| `editor/toolshell` | Ebiten, `ecs`, `editor/hostlink`, `extract`, `game`, `gfx`, `input`, `render` | The shell of `wpix-editor`: viewport camera and grid, the tool's `Panel`, document backends, the tool protocol. Never imports debugui, and never links the level editor (`editor`, `editor/bridge`, `editor/host`): what it shares with the bridge is in `editor/hostlink`. Design: [editor.md](tool-shell.md#tools) |
| `editor/toolshell/debugpanel` | `editor/toolshell`, Ebiten, debugui | Draws a `Panel` in the native window. The only package that imports debugui, besides `tools/wpix-editor/internal/native` that wires it |
| `editor/filekind` | standard library | The file kinds `wpix-editor` opens: suffixes, edited or previewed, sound. Read by the preview router and the command |
| `editor/viewkit` | Ebiten, `ecs`, `editor/toolshell`, `gfx`, `render` | What the tools that show a texture share: the texture sprite kept on screen, frames with hover, selection, labels and a list, overlay text |
| `editor/viewer` | `ecs`, `editor/animedit`, `editor/filekind`, `editor/toolshell`, `editor/viewkit`, `editor/voiceview`, `game`, `gfx`, `render`, `internal/shaderparam` | The `view` tool: the preview router of every kind that is not edited (textures, atlases, fonts, shaders, Tiled files, Aseprite exports through `animedit`, voices and dialogues through `voiceview`) |
| `editor/voiceview` | Ebiten, `ecs`, `ecs/system`, `asset`, `editor/toolshell`, `gfx`, `speech` | The voice and dialogue previewer of `viewer`: a voice or dialogue file played with sound, the line's piano roll. Uses only `speech`'s API, never the voice library. Never imports debugui |
| `editor/sheetedit` | `ecs`, `editor/toolshell`, `editor/viewkit`, `game`, `gfx`, `render`, `sheetfile` | The `sheet` tool: a `*.sheet.json` editor. Model, edits and panel are Ebiten-free files, covered without a display. Never imports debugui |
| `editor/animedit` | `ecs`, `anim`, `anim/animfile`, `asset`, `editor/toolshell`, `game`, `gfx`, `input`, `render` | The `anim` tool: an `*.anim.json` editor. Model, edits, geometry, playback and panel are Ebiten-free files, covered without a display. Never imports debugui |

Rules: Ebiten is imported only by `render`, `sound` (its `audio` packages), `input/ebiteninput`, `game` and `editor` (including `editor/...`); outside the Engine Runtime, only `tools/wpix-editor` and the tests of `tools/internal/anim` (through `render`) use it.
`input` imports neither Ebiten nor `ecs`.
Components hold asset handles and packed values, not Ebiten types or pointers. debugui is imported only by `editor/toolshell/debugpanel` and `tools/wpix-editor/internal/native`, never by the js/wasm build.
The renderer design behind this split is in [engine-renderer.md](engine-renderer.md).
