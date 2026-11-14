# Architecture

The Weakpixel Engine is one Go module, `github.com/weakpixel/engine`. The ECS core is a separate module, `github.com/weakpixel/ecs` (repository [weakpixel/ecs](https://github.com/weakpixel/ecs), design in its [design.md](https://github.com/weakpixel/ecs/blob/main/doc/design.md)), which the engine requires. On these pages `ecs` and `ecs/system` mean its packages `github.com/weakpixel/ecs` and `github.com/weakpixel/ecs/system`. Inside the module the layers are packages: the Game Object runtime `god` and the tools, and the Engine Runtime (`game`, `gfx`, `render` and the other packages at the repository root) on Ebiten. Each layer depends only on the layers below it, so the ECS packages need the standard library and nothing else, and a program that imports only `ecs` and `god` compiles no Ebiten code.

```
                 Engine Runtime (game, gfx, render, ..., Ebiten)
                       │
         ┌─────────────┼──────────────────────────────────┐
         ▼             ▼                                  ▼
   god      ecs/system ─────────────────────► ecs ◄──── tools/internal/gen (generates code that uses ecs, god)
   (GOD runtime)  (Schedule, Commands)               (core)    tools/internal/goddoc (reads godspecs/*.god.json, uses god)
         └──────────────────────────────────────────►
```

## Modules

| Module | Path | Depends on | Why separate |
| --- | --- | --- | --- |
| ECS | `github.com/weakpixel/ecs` (own repository) | nothing | The core has no dependencies and rarely changes; it versions on its own |
| Asset system | `github.com/weakpixel/asset` (own repository) | nothing | Handles, stores, loading and hot reload, usable without the engine |
| Voice | `github.com/weakpixel/voice` (own repository) | nothing | Dialogue voices, usable without the engine |
| engine | `github.com/weakpixel/engine` (this repository) | `github.com/weakpixel/ecs`, `github.com/weakpixel/asset`, `github.com/weakpixel/voice` (`speech` only), Ebiten (Engine Runtime only), `golang.org/x/image`, `golang.org/x/tools` (`tools/internal/gen` only) | `god`, the tools and the Engine Runtime version together |
| extmod | `tools/internal/gen/testdata/extmod` | engine (`replace`) | A test fixture: proves generated Game Object code that uses engine components compiles from another module |

Other modules build on the engine and are not part of this repository: the Lua bindings ([`github.com/weakpixel/contrib/lua`](https://github.com/weakpixel/contrib/tree/main/lua)) and the demo games (examples, kept with their art in a private repository). Nothing here imports them, and the guides do not name the demos: they use neutral sample names (`TestDocsDoNotNameExamples` in `internal/deps`) and tests use generated test data (`testdata/assets`, a small asset library drawn in code, and `testdata/gametypes`).

The module boundary does not keep Ebiten out of the headless packages, so the package rules below are enforced by `internal/deps` (`go test ./internal/deps/`): `ecs`, `ecs/system`, `god`, `xmath`, `tools/internal/mcp` and `internal/marker` have only standard-library dependencies (`xmath` not even `ecs`, `tools/internal/mcp` and `marker` no weakpixel package), `internal/shaderparam` adds `internal/marker` to that, and `asset/library`, `input`, `gfx`, `anim/animfile`, `sheetfile`, `particles/fxfile`, `tiled` and the tools (`tools/weakpixel`, `tools/weakpixel/cmd` and every package under `tools/internal`) have no Ebiten dependency.

## Root packages

| Package | Imports | Job |
| --- | --- | --- |
| `ecs` (module `github.com/weakpixel/ecs`) | stdlib | `World`, entities, components, views, queries, resources. Design: [ECS design](https://github.com/weakpixel/ecs/blob/main/doc/design.md) |
| `xmath` | stdlib | Float32 game math: `Vec2`, `Rect`, `Mat3`, geometry (rays, segments, circles, polygons), seeded `Rand`, noise, interpolation, angles, smoothing, easing. Imports nothing from this module, `ecs` does not import it, and `gfx` aliases its vector and rectangle types (`internal/deps` checks the stdlib-only rule). Guide: [math.md](../guide/engine/math.md) |
| `ecs/system` (module `github.com/weakpixel/ecs`) | `ecs` | `Schedule`, `Context`, `Commands`, the `Events` queues. Not part of `World`: `ecs` has no system concept |
| `god` | `ecs` (the module), stdlib | Registered Game Object schemas, `TypeSchema`, JSON (de)serialization, `AssetResolver` |
| `internal/project` | stdlib | Loads `.weakpixel.json` for `weakpixel gen` and `weakpixel doc` |
| `internal/marker` | stdlib | The `+name` marker parser behind `gen`: a registry of marker structs, `Parse` for one marker and `Collect` for a declaration's comment group. Knows nothing about weakpixel and is kept standalone-ready (`internal/deps` keeps it on the standard library); the grammar is in its package doc. Its sub-package `internal/marker/enum` lists the constants of a Go enum type (`go/types`, stdlib only); `gen` applies its own enum rules on top |
| `internal/shaderparam` | stdlib, `internal/marker` | Reads the `+shader:param` markers of a Kage shader (`Parse` for `Params`, `ParseAll` for any `float`/`vec2`..`vec4` uniform, `Param`) for the particle editor and the shader preview. The renderer does not use it; `internal/deps` keeps it off Ebiten, `render` and `ecs` |
| `tools/weakpixel` | Cobra, the tool packages below, `tools/internal/cli` | The single `weakpixel` command, a Cobra command tree (`cmd`, one file per subcommand: `gen`, `doc`, `anim`, `assets`, `tileset`, `schema`, `framelog`, `serve`, `new`, `mcp`, `help`, `version`). The tools are internal packages that take an options struct and return an error; the command line is their API. `internal/deps` checks that they import neither Cobra nor `flag` |
| `tools/internal/gen` | `god`, `internal/marker`, `golang.org/x/tools` | `weakpixel gen`: the `+game:*` generator. Design: [game-objects.md](game-objects.md) |
| `tools/internal/goddoc` | `god` | `weakpixel doc`: HTML and Markdown docs and the merged project GOD from the GOD files |
| `tools/internal/anim` | `anim/animfile` | `weakpixel anim`: validates and creates `*.anim.json` definitions. Its tests load a created file with `anim` and `render` |
| `tools/internal/assets` | `asset/library`, `anim/animfile` | `weakpixel assets`: checks, lists, catalogs and packs an asset library; writes the optional key constants |
| `tools/internal/mcp` | stdlib | The MCP protocol layer of `weakpixel mcp`: JSON-RPC 2.0 over stdio, the 2026-07-28 and 2025-11-25 lifecycles, the tool registry and the JSON Schema subset its tools use. Knows nothing about weakpixel; `internal/deps` keeps it on the standard library (no MCP SDK) |
| `internal/jsonedit` | stdlib | Minimal text edits of a JSON document (find, replace, set, insert, delete by path) that keep every other byte; the Go counterpart of the extension's `jsonc-parser` edits. Shared by the MCP server; `internal/deps` keeps it on the standard library |
| `tools/internal/mcpserver` | `tools/internal/mcp`, `tools/internal/goddoc`, `asset/library`, `anim/animfile`, `sheetfile`, `particles/fxfile`, `god`, `internal/jsonedit`, `internal/project` | `weakpixel mcp`: the tools, the project (settings, asset root, merged GOD with the engine's `godspecs/`), the path guard, the offline document checks and the document writer behind the edit tools, and the game tools: the editor build as a child process, its protocol client, logs, asset watcher (stdlib only, no Ebiten) |
| `tools/internal/framelog` | stdlib | `weakpixel framelog`: summarizes a frame log (`WEAKPIXEL_FRAMELOG`) and lists the frames that stutter |
| `tools/internal/newgame` | stdlib | `weakpixel new`: writes a game project from its embedded template and runs `git` and `go` in it. Its test builds a new project against the checkout (`-replace`) |
| `tools/weakpixel-editor` | Cobra, Ebiten, `httpfs` (asset module), `editor/animedit`, `editor/fxedit`, `editor/sheetedit`, `editor/hostlink`, `editor/toolshell`, `editor/viewer`, `editor/voiceview`, `internal/project`, `tools/internal/cli`; natively `editor/toolshell/debugpanel` | The `weakpixel-editor` command, a Cobra command tree with one subcommand per tool (`cmd`). `internal/editorapp` (tool table, file resolution) is shared with the js/wasm entry point; `internal/native` is the window and the only debugui user besides `debugpanel`. The js/wasm build links neither Cobra nor debugui (checked by `internal/deps`) |
| `tools/internal/cli` | Cobra | What the command trees share: the exit codes (0 ok, 1 failure, 2 usage), `UsageError`, `Execute`, `Version` and the `version` command |

Rules:

- `god` imports `ecs`; `ecs` never imports `god` or anything under `tools/` (it lives in another module and knows nothing about this one).
- An ECS change merges in `weakpixel/ecs` first; an engine PR then bumps the version in the root and `tools/internal/gen/testdata/extmod` `go.mod` files. For work on both at once, use an untracked `go.work` (`go work init . ../ecs`; `go.work` is in `.gitignore`), never a committed `replace`.
- The asset system (packages `asset` and `httpfs`) is the module `github.com/weakpixel/asset` (repository weakpixel/asset), standard library only, and knows nothing about this repository. An asset change merges there first; an engine PR then bumps the version in `go.mod` (and wherever `go mod tidy` lists it). Work on both with `go work init . ../asset`.
- Generated Game Object code uses exported `ecs` and `god` API only, so it compiles from other modules (tested by `tools/internal/gen/testdata/extmod`).

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

  editor/hostlink (stdlib) ◄── editor/bridge, editor/host, editor/toolshell, tools/weakpixel-editor

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
| `anim/animfile` | stdlib only | The animation file formats, shared by the `anim` loader and `weakpixel anim`, so the tool checks exactly what the loader accepts without Ebiten or ECS |
| `sheetfile` | stdlib only | The grid sheet format (`*.sheet.json`: `Decode`, `Encode`, the frame layout), shared by the atlas loader in `render` and the `sheet` tool |
| `particles/fxfile` | `gfx`, `xmath` | The particle effect format (`*.fx.json`: `Emitter`, `Spec`, `Decode`, `Encode`), shared by the `particles` loader, the `fx` tool and `weakpixel mcp`, so all three accept the same files without Ebiten. It keeps the `gfx` enum, color and vector types so `gfx.Particles` overrides and the emitter share one type (`internal/deps` limits it to these imports) |
| `tiled` | stdlib only | The Tiled JSON formats (`.tmj`, `.tsj`, `.world`), shared by the `render` atlas format, the `tilemap` loaders and future tools |
| `gfx` | `asset`, `ecs`, `god`, `xmath` | Plain-data components and the handles they hold. No Ebiten, so tools never compile a renderer. `ecs` and `god` come in only through the generated Game Object code (`zz__gen.go`) that registers `Visual`, `AnimatedVisual`, `Text`, `SoundSource` and `ParticleSource` |
| `render` | Ebiten, `asset`, `assetkind`, `gfx`, `sheetfile`, `tiled` | Asset types and loaders, `Queue`, `Camera`, `Renderer`. Never imports `ecs`: it draws a list of plain items. `tiled` gives the atlas loader its `*.tsj` format. |
| `extract` | `ecs`, `ecs/system`, `asset`, `collision`, `gfx`, `hierarchy`, `render` | The bridge: systems that read components in `Update` and fill the queue |
| `anim` | `ecs`, `ecs/system`, `asset`, `assetkind`, `gfx`, `render`, `anim/animfile` | Clip assets and the `AnimationSystem`. No Ebiten calls of its own |
| `tilemap` | `ecs`, `ecs/system`, `asset`, `collision`, `gfx`, `render`, `extract`, `tiled` | Tiled `Tileset`, `Map` and `World` assets, the `TileLayer` component, `Spawn` and the `TilemapExtract` (which reads `extract.Screen`). No Ebiten calls of its own |
| `particles` | `ecs`, `ecs/system`, `gfx`, `render`, `extract`, `asset`, `assetkind`, `particles/fxfile` | The effect asset (`*.fx.json` loader over `particles/fxfile`) and the `System` that simulates `gfx.Particles` pools and the `Extract` that queues one sprite item per live particle. Particles are pool entries, not entities. No Ebiten calls of its own (`internal/deps` checks it) |
| `sound` | Ebiten `audio` (and its decoders), `ecs`, `ecs/system`, `asset`, `assetkind`, `gfx` | `Sound` and `Music` assets, `Queue`, `Mixer`, `Listener`, the `AudioSystem` and its `Backend`s. Independent of `render`: audio needs no GPU, and a headless server can run it on the `NullBackend` |
| `camera` | `ecs`, `ecs/system`, `gfx`, `render`, `extract` | The follow camera (`Target`, `Rig`, `System`) and its trauma shake (`Rig.Shake`, `Rig.AddTrauma`). Its own package because `render` never imports `ecs` and `game` stays thin; the follow math is plain functions on `gfx.Vec2`, tested without a window. Never imports `game` |
| `tween` | `ecs`, `ecs/system`, `gfx`, `xmath` | `Tweens`, the resource that owns every running tween in typed pools, its typed `Lens`es and the `System` that advances them. Tweens are pool entries, not components. Headless: no `render`, no Ebiten (`internal/deps` checks it). Design: [engine-renderer.md](engine-renderer.md#tweens) |
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
| `editor/toolshell` | Ebiten, `ecs`, `editor/hostlink`, `extract`, `game`, `gfx`, `input`, `render` | The shell of `weakpixel-editor`: viewport camera and grid, the tool's `Panel`, document backends, the tool protocol. Never imports debugui, and never links the level editor (`editor`, `editor/bridge`, `editor/host`): what it shares with the bridge is in `editor/hostlink`. Design: [editor.md](editor.md#tools) |
| `editor/toolshell/debugpanel` | `editor/toolshell`, Ebiten, debugui | Draws a `Panel` in the native window. The only package that imports debugui, besides `tools/weakpixel-editor/internal/native` that wires it |
| `editor/viewer` | `ecs`, `editor/toolshell`, `game`, `gfx`, `render`, `internal/shaderparam` | The `view` tool: a texture on a grid |
| `editor/voiceview` | Ebiten, `ecs`, `ecs/system`, `asset`, `editor/toolshell`, `gfx`, `speech` | The `voice` tool: a voice or dialogue file played with sound, the line's piano roll. Uses only `speech`'s API, never the voice library. Never imports debugui |
| `editor/sheetedit` | `ecs`, `editor/toolshell`, `game`, `gfx`, `render`, `sheetfile` | The `sheet` tool: a `*.sheet.json` editor. Model, edits and panel are Ebiten-free files, covered without a display. Never imports debugui |
| `editor/animedit` | `ecs`, `anim`, `anim/animfile`, `asset`, `editor/toolshell`, `game`, `gfx`, `input`, `render` | The `anim` tool: an `*.anim.json` editor. Model, edits, geometry, playback and panel are Ebiten-free files, covered without a display. Never imports debugui |

Rules: Ebiten is imported only by `render`, `sound` (its `audio` packages), `input/ebiteninput`, `game` and `editor` (including `editor/...`); outside the Engine Runtime, only `tools/weakpixel-editor` and the tests of `tools/internal/anim` (through `render`) use it. `input` imports neither Ebiten nor `ecs`. Components hold asset handles and packed values, not Ebiten types or pointers. debugui is imported only by `editor/toolshell/debugpanel` and `tools/weakpixel-editor/internal/native`, never by the js/wasm build. The renderer design behind this split is in [engine-renderer.md](engine-renderer.md).

## Cross-cutting conventions

**Command-line tools are Cobra command trees.** Cobra (`github.com/spf13/cobra`) may be imported only under `tools/`, never by `ecs`, the Engine Runtime or the js/wasm editor build (`internal/deps` checks it). A binary is `tools/<binary>/cmd/root.go` (`Execute`, `newRootCmd`) plus one file per subcommand, and `main.go` only calls `cmd.Execute`. Each command is built by its own constructor returning a fresh `*cobra.Command`: no package-level commands, no `init()` registration, no state shared between runs. Flag values are variables or fields of an options struct bound with `cmd.Flags().XxxVar`; argument counts and required flags use Cobra's own validation (`Args`, `MarkFlagRequired`, ...), and `RunE` stays short, handing a long job to an options struct of a package under `tools/internal` (`Run(opts, stdout) error`, or `Run(ctx, opts, stdout)` for the dev server, which stops with the command's context; a usage problem only the tool can see is `cli.Usagef`). Flags are GNU style (`--long`, `-o` for an output file); a single-dash long flag is a usage error. Output goes to `cmd.OutOrStdout()` and `cmd.ErrOrStderr()` so tests capture it. `tools/internal/cli` maps errors to the exit codes 0 ok, 1 failure, 2 usage (a wrong command line, flag or argument count).

**Panics for programmer errors, errors for bad data.** Every package panics with its own prefix: `ecs: `, `system: `, `god: `, `asset: `, `library: `, `render: `, `game: `, `input: `, `tween: `, `collision: `, `physics: `. The panic texts listed in the guide's panic tables are API, and tests assert them with `strings.Contains`. Bad input data (a malformed GOD document, a broken image, a typo in an `*.anim.json`) is an error, never a panic, and validation collects all problems before reporting them (`errors.Join`, generator diagnostics with `file:line:col`).

**One goroutine.** A `World` is single-goroutine, like Ebiten's `Update` and `Draw`. The exceptions are deliberate and narrow: the component registry is process-global and safe for concurrent `RegisterComp` (package `init`s, parallel tests), and asset `Decode` runs on worker goroutines that never see a store or the World.

**0 allocs on hot paths.** `View.Each`, `GetComp[T]`, `SetComp[T]`, `NewEntityN`, `View.NewEntity`, `View.Get`, `View.Set`, `Schedule.Run`, the extract systems, the sort, `Renderer.Draw` for sprites and shapes, `AnimationSystem` and asset `Store.Get` are 0 allocs/op in steady state. This is enforced with `testing.AllocsPerRun` tests, not by reading the code. `make bench` writes `god` benchmark results to `bench/` (the ECS core has its own benchmarks in its repository); `make engine-bench` writes Engine Runtime benchmark results to `bench/engine-<name>.txt` (the committed baseline is `bench/engine-baseline.txt`). The ECS [performance guide](https://github.com/weakpixel/ecs/blob/main/doc/performance-guide.md#measuring) explains how to compare them.

**Thin generated code.** Generated files (`zz__gen.go`, the GOD files `<god.specs>/*.god.json`, `build/god` by default) are never edited by hand. Logic lives in hand-written, tested unexported helpers that the generated code calls; the generators only stamp out the arity- or object-specific parts.

**Model the domain.** Each concept gets its own type and, where it fits, its own package (registry, entity table, storage, resource store). `World` composes them and delegates; it is not a god object.

## Tests

- Each package tests with the standard `testing` package only, table-driven, with a `mustPanic(t, want, f)` helper that matches a panic substring.
- Doc examples that can run live in `example_test.go` files.
- `internal/deps` runs `go list -deps` and fails, listing every offender, when an ECS package gains a non-ECS dependency or a headless engine package imports Ebiten.
- Engine tests that read pixels back need a running Ebiten game, so they sit behind the `ebitentest` build tag and need a display: `go test -tags ebitentest ./render` (add `-run '^$' -bench Draw` for the draw benchmarks). Everything else (`make all`) runs headless.
