# Architecture

The Weakpixel engine is one Go module, `github.com/weakpixel/engine`.
The ECS core is a separate module, `github.com/weakpixel/ecs` (repository [weakpixel/ecs](https://github.com/weakpixel/ecs), design in its [design.md](https://github.com/weakpixel/ecs/blob/main/doc/design.md)), which the engine requires.
On these pages `ecs` and `ecs/system` mean its packages `github.com/weakpixel/ecs` and `github.com/weakpixel/ecs/system`.
Inside the module the layers are packages: the Game Object runtime `god` and the tools, and the Engine Runtime (`game`, `gfx`, `render` and the other packages at the repository root) on Ebiten.
Each layer depends only on the layers below it, so the ECS packages need the standard library and nothing else, and a program that imports only `ecs` and `god` compiles no Ebiten code.

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

Other modules build on the engine and are not part of this repository: the Lua bindings ([`github.com/weakpixel/contrib/lua`](https://github.com/weakpixel/contrib/tree/main/lua)) and the demo games (examples, kept with their art in a private repository).
Nothing here imports them, and the guides do not name the demos: they use neutral sample names (`TestDocsDoNotNameExamples` in `internal/deps`) and tests use generated test data (`testdata/assets`, a small asset library drawn in code, and `testdata/gametypes`).

The module boundary does not keep Ebiten out of the headless packages, so the package rules below are enforced by `internal/deps` (`go test ./internal/deps/`):
`ecs`, `ecs/system`, `god`, `xmath`, `tools/internal/mcp` and `internal/marker` have only standard-library dependencies (`xmath` not even `ecs`, `tools/internal/mcp` and `marker` no weakpixel package), `internal/shaderparam` adds `internal/marker` to that, and `asset/library`, `input`, `gfx`, `anim/animfile`, `sheetfile`, `particles/fxfile`, `tiled` and the tools (`tools/wpix`, `tools/wpix/cmd` and every package under `tools/internal`) have no Ebiten dependency.

## Root packages

| Package | Imports | Job |
| --- | --- | --- |
| `ecs` (module `github.com/weakpixel/ecs`) | stdlib | `World`, entities, components, views, queries, resources. Design: [ECS design](https://github.com/weakpixel/ecs/blob/main/doc/design.md) |
| `xmath` | stdlib | Float32 game math: `Vec2`, `Rect`, `Mat3`, geometry (rays, segments, circles, polygons), seeded `Rand`, noise, interpolation, angles, smoothing, easing. Imports nothing from this module, `ecs` does not import it, and `gfx` aliases its vector and rectangle types (`internal/deps` checks the stdlib-only rule). Guide: [math.md](../guide/engine/math.md) |
| `ecs/system` (module `github.com/weakpixel/ecs`) | `ecs` | `Schedule`, `Context`, `Commands`, the `Events` queues. Not part of `World`: `ecs` has no system concept |
| `god` | `ecs` (the module), stdlib | Registered Game Object schemas, `TypeSchema`, JSON (de)serialization, `AssetResolver` |
| `project` | stdlib | Loads `.weakpixel.json` for the tools (`wpix gen`, `wpix doc`, `wpix mcp`, `wpix-editor`) and for editor hosts in other modules; `internal/deps` keeps it on the standard library |
| `internal/marker` | stdlib | The `+name` marker parser behind `gen`: a registry of marker structs, `Parse` for one marker and `Collect` for a declaration's comment group. Knows nothing about weakpixel and is kept standalone-ready (`internal/deps` keeps it on the standard library); the grammar is in its package doc. Its sub-package `internal/marker/enum` lists the constants of a Go enum type (`go/types`, stdlib only); `gen` applies its own enum rules on top |
| `internal/shaderparam` | stdlib, `internal/marker` | Reads the `+shader:param` markers of a Kage shader (`Parse` for `Params`, `ParseAll` for any `float`/`vec2`..`vec4` uniform, `Param`) for the particle editor and the shader preview. The renderer does not use it; `internal/deps` keeps it off Ebiten, `render` and `ecs` |
| `tools/wpix` | Cobra, the tool packages below, `tools/internal/cli` | The single `wpix` command, a Cobra command tree (`cmd`, one file per subcommand: `gen`, `doc`, `anim`, `assets`, `tileset`, `schema`, `framelog`, `serve`, `new`, `mcp`, `help`, `version`). The tools are internal packages that take an options struct and return an error; the command line is their API. `internal/deps` checks that they import neither Cobra nor `flag` |
| `tools/internal/gen` | `god`, `internal/marker`, `golang.org/x/tools` | `wpix gen`: the `+game:*` generator. Design: [game-objects.md](game-objects.md) |
| `tools/internal/goddoc` | `god` | `wpix doc`: HTML and Markdown docs and the merged project GOD from the GOD files |
| `tools/internal/anim` | `anim/animfile` | `wpix anim`: validates and creates `*.anim.json` definitions. Its tests load a created file with `anim` and `render` |
| `tools/internal/assets` | `asset/library`, `anim/animfile` | `wpix assets`: checks, lists, catalogs and packs an asset library; writes the optional key constants |
| `tools/internal/mcp` | stdlib | The MCP protocol layer of `wpix mcp`: JSON-RPC 2.0 over stdio, the 2026-07-28 and 2025-11-25 lifecycles, the tool registry and the JSON Schema subset its tools use. Knows nothing about weakpixel; `internal/deps` keeps it on the standard library (no MCP SDK) |
| `jsonedit` | stdlib | Minimal text edits of a JSON document (find, replace, set, insert, delete by path) that keep every other byte; the Go counterpart of the extension's `jsonc-parser` edits. Public for editor hosts in other modules; used by the MCP server; `internal/deps` keeps it on the standard library |
| `tools/internal/mcpserver` | `tools/internal/mcp`, `tools/internal/goddoc`, `asset/library`, `anim/animfile`, `sheetfile`, `particles/fxfile`, `god`, `jsonedit`, `project` | `wpix mcp`: the tools, the project (settings, asset root, merged GOD with the engine's `godspecs/`), the path guard, the offline document checks and the document writer behind the edit tools, and the game tools: the editor build as a child process, its protocol client, logs, asset watcher (stdlib only, no Ebiten) |
| `tools/internal/framelog` | stdlib | `wpix framelog`: summarizes a frame log (`WEAKPIXEL_FRAMELOG`) and lists the frames that stutter |
| `tools/internal/newgame` | stdlib | `wpix new`: writes a game project from its embedded template and runs `git` and `go` in it. Its test builds a new project against the checkout (`-replace`) |
| `tools/wpix-editor` | Cobra, Ebiten, `httpfs` (asset module), `editor/animedit`, `editor/filekind`, `editor/fxedit`, `editor/sheetedit`, `editor/hostlink`, `editor/toolshell`, `editor/viewer`, `editor/voiceview`, `project`, `tools/internal/cli`; natively `editor/toolshell/debugpanel` | The `wpix-editor <file>` command (`cmd`, Cobra). `internal/editorapp` (the kind table with each kind's tool and starter, file resolution) is shared with the js/wasm entry point; `internal/native` is the window and the only debugui user besides `debugpanel`. The js/wasm build links neither Cobra nor debugui (checked by `internal/deps`) |
| `tools/internal/cli` | Cobra | What the command trees share: the exit codes (0 ok, 1 failure, 2 usage), `UsageError`, `Execute`, `Version` and the `version` command |

Rules:

- `god` imports `ecs`; `ecs` never imports `god` or anything under `tools/` (it lives in another module and knows nothing about this one).
- An ECS change merges in `weakpixel/ecs` first; an engine PR then bumps the version in the root and `tools/internal/gen/testdata/extmod` `go.mod` files.
  For work on both at once, use an untracked `go.work` (`go work init . ../ecs`; `go.work` is in `.gitignore`), never a committed `replace`.
- The asset system (packages `asset` and `httpfs`) is the module `github.com/weakpixel/asset` (repository weakpixel/asset), standard library only, and knows nothing about this repository.
  An asset change merges there first; an engine PR then bumps the version in `go.mod` (and wherever `go mod tidy` lists it).
  Work on both with `go work init . ../asset`.
- Generated Game Object code uses exported `ecs` and `god` API only, so it compiles from other modules (tested by `tools/internal/gen/testdata/extmod`).

## Cross-cutting conventions

**Command-line tools are Cobra command trees.**
Cobra (`github.com/spf13/cobra`) may be imported only under `tools/`, never by `ecs`, the Engine Runtime or the js/wasm editor build (`internal/deps` checks it).
A binary is `tools/<binary>/cmd/root.go` (`Execute`, `newRootCmd`) plus one file per subcommand, and `main.go` only calls `cmd.Execute`.
Each command is built by its own constructor returning a fresh `*cobra.Command`: no package-level commands, no `init()` registration, no state shared between runs.
Flag values are variables or fields of an options struct bound with `cmd.Flags().XxxVar`; argument counts and required flags use Cobra's own validation (`Args`, `MarkFlagRequired`, ...), and `RunE` stays short, handing a long job to an options struct of a package under `tools/internal` (`Run(opts, stdout) error`, or `Run(ctx, opts, stdout)` for the dev server, which stops with the command's context; a usage problem only the tool can see is `cli.Usagef`).
Flags are GNU style (`--long`, `-o` for an output file); a single-dash long flag is a usage error.
Output goes to `cmd.OutOrStdout()` and `cmd.ErrOrStderr()` so tests capture it.
`tools/internal/cli` maps errors to the exit codes 0 ok, 1 failure, 2 usage (a wrong command line, flag or argument count).

**Panics for programmer errors, errors for bad data.**
Every package panics with its own prefix: `ecs: `, `system: `, `god: `, `asset: `, `library: `, `render: `, `game: `, `input: `, `tween: `, `collision: `, `physics: `.
The panic texts listed in the guide's panic tables are API, and tests assert them with `strings.Contains`.
Bad input data (a malformed GOD document, a broken image, a typo in an `*.anim.json`) is an error, never a panic, and validation collects all problems before reporting them (`errors.Join`, generator diagnostics with `file:line:col`).

**One goroutine.**
A `World` is single-goroutine, like Ebiten's `Update` and `Draw`.
The exceptions are deliberate and narrow: the component registry is process-global and safe for concurrent `RegisterComp` (package `init`s, parallel tests), and asset `Decode` runs on worker goroutines that never see a store or the World.

**0 allocs on hot paths.**
`View.Each`, `GetComp[T]`, `SetComp[T]`, `NewEntityN`, `View.NewEntity`, `View.Get`, `View.Set`, `Schedule.Run`, the extract systems, the sort, `Renderer.Draw` for sprites and shapes, `AnimationSystem` and asset `Store.Get` are 0 allocs/op in steady state.
This is enforced with `testing.AllocsPerRun` tests, not by reading the code.
`make bench` writes `god` benchmark results to `bench/` (the ECS core has its own benchmarks in its repository); `make engine-bench` writes Engine Runtime benchmark results to `bench/engine-<name>.txt` (the committed baseline is `bench/engine-baseline.txt`).
The ECS [performance guide](https://github.com/weakpixel/ecs/blob/main/doc/performance-guide.md#measuring) explains how to compare them.

**Thin generated code.**
Generated files (`zz__gen.go`, the GOD files `<god.specs>/*.god.json`, `build/god` by default) are never edited by hand.
Logic lives in hand-written, tested unexported helpers that the generated code calls; the generators only stamp out the arity- or object-specific parts.

**Model the domain.**
Each concept gets its own type and, where it fits, its own package (registry, entity table, storage, resource store).
`World` composes them and delegates; it is not a god object.

## Tests

- Each package tests with the standard `testing` package only, table-driven, with a `mustPanic(t, want, f)` helper that matches a panic substring.
- Doc examples that can run live in `example_test.go` files.
- `internal/deps` runs `go list -deps` and fails, listing every offender, when an ECS package gains a non-ECS dependency or a headless engine package imports Ebiten.
- Engine tests that read pixels back need a running Ebiten game, so they sit behind the `ebitentest` build tag and need a display: `go test -tags ebitentest ./render` (add `-run '^$' -bench Draw` for the draw benchmarks).
  Everything else (`make all`) runs headless.
