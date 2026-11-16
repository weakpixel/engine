# Weakpixel engine documentation

The documentation has two parts. The **guide** is for people who build games with the Weakpixel engine: what the API does and how to use it. The **design** part is for people who work on the engine itself: how it is built, why, and what is not built yet. Every topic has one home; other pages link to it instead of repeating it.

## Guide

### ECS (module `github.com/weakpixel/ecs`)

The core ECS (`World`, entities, components, views, queries, resources, and the `system` package with `Schedule`, `Commands` and events) is its own module in the [weakpixel/ecs](https://github.com/weakpixel/ecs) repository. Its guide is there: start with [the ECS guide](https://github.com/weakpixel/ecs/blob/main/doc/README.md), then the [generic API](https://github.com/weakpixel/ecs/blob/main/doc/api-generic.md), the [low-level API](https://github.com/weakpixel/ecs/blob/main/doc/api-low-level.md), [systems](https://github.com/weakpixel/ecs/blob/main/doc/api-systems.md) and [usage and performance](https://github.com/weakpixel/ecs/blob/main/doc/performance-guide.md).

### Game Objects (`god`, `wpix gen`)

| Page | Covers |
| --- | --- |
| [Game Objects](guide/ecs/api-game-object.md) | `+game:*` markers and the generated `<Object>View` |
| [Game Object Definition (GOD)](guide/ecs/game-object-definition.md) | Component keys (`group:name`), the GOD files, loading and saving entities as JSON (`god`) |

### Lua (module `github.com/weakpixel/contrib/lua`)

Lua scripting is an add-on module in the [weakpixel/contrib](https://github.com/weakpixel/contrib/tree/main/lua) repository, not part of this one. It runs Lua systems on the Game Object schemas (`god`) through [`Engine.AddRunner`](guide/engine/engine.md). Its guide is in the [lua folder](https://github.com/weakpixel/contrib/blob/main/lua/README.md).

### Engine Runtime (Ebiten)

| Page | Covers |
| --- | --- |
| [Overview](guide/engine/README.md) | Packages, ground rules, where to start |
| [game.Engine](guide/engine/engine.md) | The high-level entry point that wires everything |
| [Getting started by hand](guide/engine/getting-started.md) | The low-level wiring `game.Engine` does for you |
| [Components](guide/engine/components.md) | `gfx`: `Transform`, `Sprite`, `Animator`, `Label`, zero values |
| [Asset kinds](guide/engine/asset-kinds.md) | Sprites, atlases, clips, fonts, ...: what they are for, how to load them, file formats |
| [Assets in the engine](guide/engine/assets.md) | The engine's asset server, game-facing handles, hot reload and HTTP; the asset system itself is [`github.com/weakpixel/asset`](https://github.com/weakpixel/asset/blob/main/doc/guide.md) |
| [Asset library](guide/engine/asset-library.md) | `asset/library`: the standard folder layout, group loading, packs, `wpix assets` |
| [Rendering](guide/engine/rendering.md) | `extract` + `render`: queue, camera, layers, interpolation, culling |
| [Shaders](guide/engine/shaders.md) | Kage shaders on sprites: writing one, examples (flash, outline, palette swap, dissolve), driving `Params` |
| [Tilemaps](guide/engine/tilemaps.md) | `tiled` + `tilemap`: Tiled maps, tilesets and worlds, spawning, the map data API |
| [Animation](guide/engine/animation.md) | `anim`: Aseprite clips, `*.anim.json`, hit/hurt boxes, `wpix anim` |
| [Input](guide/engine/input.md) | `input`: actions, bindings files, mapping contexts, players, rebinding and profiles, touch, replay |
| [Audio](guide/engine/audio.md) | `sound`: effects, music crossfades, buses, positional sound, `SoundEmitter` |
| [Speech](guide/engine/speech.md) | `speech`: speaking entities and dialogue boxes with the voice library, events, pausing |
| [Collision](guide/engine/collision.md) | `collision`: colliders, layers and masks, `Enter`/`Exit` events, queries, tile grids |
| [Physics](guide/engine/physics.md) | `physics`: bodies, velocity, materials, one-way and moving platforms, `MoveAndSlide`, hooks |
| [Rooms](guide/engine/rooms.md) | `room`: exits, spawn points, moving between levels, transitions |
| [Levels as Game Objects](guide/engine/game-objects.md) | Authoring levels and prefabs as GOD, `resolver` |
| [Kinds reference](guide/engine/kinds.md) | Every enum-like "kind" in one place |

### Tools

[Tools](guide/tools.md): `wpix gen`, `wpix doc`, `wpix anim`, `wpix assets`, `wpix tileset`, `wpix schema` and the `make` targets. [Project settings](guide/project-file.md): `.weakpixel.json`.

## Design

[Design overview](design/README.md): architecture and dependency rules, the Game Object generator, the asset system, the renderer and input, each with its status.

## Conventions for these docs

- The guide describes the public API and behavior. Where the code and a guide page disagree, one of them is a bug: fix the code or the page in the same change.
- Panic messages in the guide's panic tables are API; tests assert them.
- Design pages explain *why* and record what is planned but not built. They are not a second copy of the API: link to the guide for signatures.
- Doc comments in the code cover single identifiers; these pages cover how the pieces fit together.
