# Weakpixel Engine

A 2D game engine for Go: the Engine Runtime on Ebiten (module `github.com/weakpixel/engine`, repository weakpixel/engine, Go 1.27, MIT) on top of the archetype-based Entity Component System of module `github.com/weakpixel/ecs` (repository weakpixel/ecs), plus a code generator that turns `// +game:*` annotated structs into typed "Game Object" schemas and a JSON definition (GOD).

## The docs are the spec

All documentation lives in `doc/` ([doc/README.md](doc/README.md) is the map). `doc/guide/` is end-user documentation and describes the intended public API and behavior: implement against it. `doc/design/` explains how things are built and why, and lists what is not built yet. Every topic has one home; link to it instead of repeating it.

- If the code and a doc disagree, don't silently pick one. Fix the code, or if the doc is wrong or underspecified, update the doc in the same change and say so.
- Behavior the docs leave open (for example the exact `: <detail>` suffix of a panic, or whether `Each` checks the revision after `yield` returns false): decide, implement, test, and **add it to the doc**.
- Panic messages in the doc's panic table are API. Tests assert them with `strings.Contains`.

## Layout

Packages, modules and the dependency rules are in [doc/design/architecture.md](doc/design/architecture.md); per-area status is in [doc/design/README.md](doc/design/README.md). Read the page for the area you touch, not the whole doc set. Where to look first:

| Area | Code | Skill or doc |
| --- | --- | --- |
| ECS core, systems, arity-N API | not here: module `github.com/weakpixel/ecs` (repository weakpixel/ecs, its own CLAUDE.md and skills) | [ECS guide](https://github.com/weakpixel/ecs/blob/main/doc/README.md) |
| Game Objects, GOD, prefabs | `tools/weakpixel`, `tools/internal/{gen,goddoc}`, `god`, `testdata/gametypes`, `tools/internal/gen/testdata/extmod` (uses engine components) | `game-object-gen` |
| Asset system | not here: module `github.com/weakpixel/asset` (repository weakpixel/asset: packages `asset` and `httpfs`, its own CLAUDE.md and skills) | [asset guide](https://github.com/weakpixel/asset/blob/main/doc/guide.md), [engine side](doc/guide/engine/assets.md) |
| Engine Runtime | packages at the repository root: `game`, `gfx`, `render`, `extract`, `asset/library`, `assetkind`, `anim`, `tiled`, `tilemap`, `sound`, `input`, `camera`, `prefab`, `resolver`, `particles`, `tween`, `room`, `speech` | [doc/guide/engine/](doc/guide/engine/README.md) |
| Web builds | `web`, `httpfs` (asset module) | [web.md](doc/guide/engine/web.md) |
| Level editor, tool shell | `editor`, `editor/bridge`, `editor/hostlink`, `editor/host`, `editor/toolshell`, `editor/viewer`, `editor/voiceview`, `editor/animedit`, `editor/fxedit`, `tools/weakpixel-editor` | [doc/design/editor.md](doc/design/editor.md) |
| Benchmarks | `bench/` (`god` results, `engine-baseline.txt` for the engine packages) | [architecture.md](doc/design/architecture.md#cross-cutting-conventions) |

The ECS core is its own repository: an ECS change merges in weakpixel/ecs first, then an engine PR bumps `github.com/weakpixel/ecs` in the root and `extmod` `go.mod` files. Work on both with an untracked `go.work` (`go work init . ../ecs`), never a committed `replace`. The asset system (weakpixel/asset) and the voice library (weakpixel/voice) are their own repositories in the same way: a change merges there first, then an engine PR bumps the version in `go.mod` (`go mod tidy` updates the indirect lines); add `../asset` or `../voice` to the `go.work`. Those repositories never name the engine. Don't edit the module cache.

The demo games and the Lua bindings build on this module from other repositories; nothing here imports them, and the docs don't name the demos.

The VSCode extension is its own repository, weakpixel/weakpixel-engine-vscode. The protocol version must match there: `bridge.Version` here, `PROTOCOL_VERSION` in its `src/controller.ts`.

`god`, [tools/internal/gen/generator.go.tpl](tools/internal/gen/generator.go.tpl) and generated Game Object code consume the `ecs` API. Don't change `ecs` in ways that break them without updating them in the same change.

Implementation order and the definition of done for each phase are in the `ecs-roadmap` skill. Load it when starting or resuming implementation work.

## Workflow: /plan → /implement

Non-trivial work goes through two steps, each triggered by the user:

1. **`/plan <request>`** writes `plan/<plan-slug>.plan.md` (or the plan folder the project instructions or project memory name) (auto approve writing the plan file!) and nothing else (no code changes). The plan records the goal, spec references, decisions, scope, ordered steps with verify commands, acceptance criteria and open questions. 
2. **`/implement [plan-slug]`** carries out the selected plan step by step. It ticks steps in the plan file as their checks pass, logs deviations and follow-ups in the plan, and sets the status (`draft` → `ready` → `in-progress` → `done` or `blocked`).

- `plan/` is the record of the work. Keep plan files up to date while implementing so an interrupted session can resume from the file alone.
- During `/implement`, the plan's scope wins. Changes to scope, the public API or a recorded decision need the user's OK and an update to the plan first.
- Roadmap phases (`ecs-roadmap`) are the usual unit of planning. Large phases become several linked plans.
- Small, obvious requests (a typo, a one-line fix, a question) don't need a plan.

## Commands

```bash
make all                      # generate, fmt, build, test, vet, then the display tests (test-ebiten)
make wasm-build               # explicit js/wasm cross-build, plus the editor test game with -tags editor
make wasm-test                # non-Ebiten js/wasm tests under Node
go test ./god/...       # Game Object runtime
go test ./tools/internal/gen/...       # generator
go vet ./...
make bench name=<label>       # god benchmarks -> bench/<label>.txt
make benchcmp new=<label>     # benchstat vs bench/baseline.txt
```

While iterating, run only the tests of the packages you changed, and send long command output to a file and read the failures from it. Run `make all` once before calling a task done, and add explicit WASM targets only when the touched code or docs affect browser or js/wasm behavior. A change isn't done if `make all` leaves generated-file changes uncommitted.

## Hard rules

- **Never commit build outputs** (executables, `*.test` from `go test -c`, `.wasm`, anything over 1 MB that is generated). `go build` without `-o` inside a main package directory (for example `tools/weakpixel-editor`) writes the binary right there: build with `-o build/<name>` (`build/` is ignored) or use `go vet` / `go build ./... ` (which writes nothing). Before every commit run `git status --short` and `git diff --cached --stat`, and unstage anything you did not mean to add; stage files by name, not with `git add -A` or `git add .`. `.gitignore` covers the usual suspects but is not a substitute for looking.
- **Never hand-edit a generated file** (a package's Game-Object `zz__gen.go`, or a GOD file: `godspecs/*.god.json`, `zz__gen.god.json`). Change the generator or template and regenerate.
- **No demo assets in tests or docs.** Tests use generated test data: `testdata/assets` (an asset library drawn in code, size-guarded) and `testdata/gametypes`, never snapshots of game art. The docs under `doc/` and the README don't name or link the demo games (`TestDocsDoNotNameExamples` in `internal/deps`); snippets use neutral sample names.
- **One module.** `./...` covers everything. `godspecs/` holds the GOD files of the engine's own packages (committed; the MCP server and games read them from the module); anything else generates into `build/god` by default.
- **Dependency direction:** `god` imports `ecs` (`github.com/weakpixel/ecs`) and the standard library only (the module also holds Ebiten, so this is a package rule, enforced by `go test ./internal/deps/`). The ECS module knows nothing about this repository. `golang.org/x/tools` is allowed only in `tools/internal/gen` (and so in `tools/weakpixel`, which links it).
- **Ebiten stays in its packages.** Ebiten may be imported only in `render`, `sound`, `input/ebiteninput`, `game` and `editor`, and outside the Engine Runtime only by `tools/weakpixel-editor` and tests of `tools/internal/anim`; debugui only in `editor/toolshell/debugpanel` and `tools/weakpixel-editor/internal/native`; `anim/animfile` and `tiled` are stdlib only (`asset/library` imports only the stdlib and the asset module `github.com/weakpixel/asset`, which is stdlib only itself), `input` imports only the stdlib (panics start with `input: `; its bindings asset is in `input/inputsys`), `gfx` imports neither Ebiten nor `render` (it imports `ecs` and `god` only through its generated Game Object code), and `render` never imports `ecs`. Pixel-reading tests need a display and sit behind the `ebitentest` build tag.
- **Cobra only under `tools/`.** The command-line tools are Cobra command trees; the conventions are in [architecture.md](doc/design/architecture.md#cross-cutting-conventions) and the shared exit codes and helpers in `tools/internal/cli`. `ecs`, the Engine Runtime and `internal` never import Cobra, nor does the js/wasm build of `weakpixel-editor` (`internal/deps` checks it).
- **Generated Game Object code may only use exported `ecs` API.** It must compile from other modules (see `extmod`).
- **Panics, not errors**, for programmer errors. `god` panics with `god: ` and returns errors for bad input data. The ECS rules (`ecs: ` panics, single-goroutine `World`, 0-alloc hot paths) live in the ECS repository.
- **Hot paths are 0 allocs/op** here too: generated Game Object views, the extract systems, `Renderer.Draw`, asset `Store.Get` ([architecture.md](doc/design/architecture.md#cross-cutting-conventions)). Enforce this with `testing.AllocsPerRun` tests, not by eye.

## Code quality

- **Readability first.** Prefer clear names, small functions and straightforward control flow. If an optimization makes code harder to follow, it must earn its place with a benchmark (`make bench`, `make benchcmp`).
- **Model the domain; don't build a god object.** Give each concept its own type, and its own file or package when that fits (for example a component registry, entity slots/generations, archetypes and columns, the resource store). `World` composes these parts and delegates to them. Don't pile every field and method onto `World` or another master struct. New unexported packages go under an `internal/` folder of the area they serve.
- **Document the public API.** Every exported identifier gets a Go doc comment that states what it does, its contract (preconditions, ownership, pointer validity, cost) and which panics it can raise.
- **Explain performance tricks inline.** When code relies on something non-obvious for speed or 0 allocs (unsafe pointer arithmetic, swap-remove, cached indices, avoiding a closure or interface conversion to prevent escapes, preallocation, bounds-check hints), add a short comment saying what the trick is and why it's needed, so a reader doesn't "simplify" it away.

## Conventions (match the existing code)

- File names are kebab-case: `type-schema.go`, `schema-registry.go`, `schema-registry_test.go`.
- Doc comment on every exported identifier and on non-obvious unexported ones. Comments explain *why* or describe a contract. Comment density is moderate, as in `god`.
- Collect all validation errors and report them together (`errors.Join`, generator diags with `file:line:col`). Don't stop at the first error.
- Tests use the standard `testing` package only (no testify). Prefer table-driven tests with `t.Run`, and use a `mustPanic(t, want, f)` helper that matches a substring (see [god/schema-registry_test.go](god/schema-registry_test.go)).
- Use modern Go: `iter.Seq2`, range-over-func, `for i := range n`, `reflect.TypeFor[T]()`, `slices`/`maps`.
- Keep generated code thin. Put the logic in hand-written, tested unexported helpers that the generated code calls.

## Skills

| Skill | Use when |
| --- | --- |
| `plan` | User runs `/plan`: write `plan/<slug>.plan.md` |
| `implement` | User runs `/implement`: carry out a plan file |
| `ecs-roadmap` | Starting or resuming implementation: phase order, current status check, definition of done |
| `game-object-gen` | Working on `weakpixel gen` (`tools/internal/gen`), its template or tests, `testdata/gametypes`, `extmod`, GOD JSON, `god` |
| `doc-writer` | Any documentation has to be written or updated (runs on Haiku) |
