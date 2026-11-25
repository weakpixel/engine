# Weakpixel Engine

A 2D game engine for Go: the Engine Runtime on Ebiten (module `github.com/weakpixel/engine`, Go 1.27, MIT) on top of the Weakpixel ECS (`github.com/weakpixel/ecs`), plus a generator that turns `// +game:*` annotated structs into typed Game Object schemas and a JSON definition (GOD).

## The docs are the spec

`doc/` holds all documentation ([doc/README.md](doc/README.md) is the map). `doc/guide/` is the public API and behavior: implement against it. `doc/design/` is how and why, plus what is not built yet; [doc/design/README.md](doc/design/README.md) lists every page with its status, so read that table and then only the page you need. Every topic has one home; link to it.

- Code and a doc disagree: fix the code, or fix the doc in the same change and say so. Never silently pick one.
- Behavior the docs leave open: decide, implement, test, and add it to the doc.
- Panic messages in a doc's panic table are API; tests assert them with `strings.Contains`.

## Layout

| Area | Code | Read |
| --- | --- | --- |
| Packages, modules, dependency rules | everything | [architecture.md](doc/design/architecture.md), [packages.md](doc/design/packages.md) |
| Game Objects, GOD, prefabs | `god`, `tools/wpix`, `tools/internal/{gen,goddoc}`, `testdata/gametypes`, `tools/internal/gen/testdata/extmod` | skill `game-object-gen` |
| Engine Runtime | root packages: `game`, `gfx`, `render`, `extract`, `asset/library`, `assetkind`, `anim`, `tiled`, `tilemap`, `sound`, `input`, `camera`, `prefab`, `resolver`, `particles`, `tween`, `room`, `speech`, ... | [doc/guide/engine/](doc/guide/engine/README.md) |
| Web builds | `web` | [web.md](doc/guide/engine/web.md) |
| Level editor, tools | `editor/...`, `tools/wpix-editor` | [editor.md](doc/design/editor.md), [editor-protocol.md](doc/design/editor-protocol.md), [tool-shell.md](doc/design/tool-shell.md) |
| Benchmarks | `bench/` | [architecture.md](doc/design/architecture.md#cross-cutting-conventions) |

Other repositories:

- **weakpixel/ecs, weakpixel/asset, weakpixel/voice** are their own modules and never name the engine. A change merges there first; then an engine PR bumps the version (`go.mod`, and `extmod` for ECS; `go mod tidy`). Work across them with an untracked `go.work` (`go work init . ../ecs`), never a committed `replace`, and never edit the module cache.
- **weakpixel-engine-vscode** hosts the editor: `bridge.Version` here equals `PROTOCOL_VERSION` in its `src/controller.ts`, and a protocol change merges here first.
- **The demos and weakpixel/contrib** (Lua in `lua/`) build on this module; nothing here imports them, and the docs don't name the demos.
- `god`, [generator.go.tpl](tools/internal/gen/generator.go.tpl) and generated Game Object code use the ECS API: an ECS change that breaks them updates them in the same change.

## Workflow

Non-trivial work goes through `/plan <request>` (one plan file in the plan folder the project names, else `plan/`; no code) and `/implement [plan-slug]` (step by step, ticking steps as their checks pass, logging deviations, status `draft` → `ready` → `in-progress` → `done` or `blocked`). The plan file is the record: keep it current. During `/implement` the plan's scope wins; changes to scope, public API or a recorded decision need the user's OK first. Small, obvious requests need no plan.

## Commands

```bash
make all                 # generate, fmt, build, test, vet, then the display tests (test-ebiten); headless: xvfb-run -a make all
make wasm-build          # js/wasm cross-build, plus the editor test game with -tags editor
make wasm-test           # non-Ebiten js/wasm tests under Node
go test ./god/... ./tools/internal/gen/... ./internal/deps/
make bench name=<label>  # god benchmarks -> bench/<label>.txt; make benchcmp new=<label>
```

While iterating, run only the tests of the packages you changed, with long output in a file. Run `make all` once before calling a task done (WASM targets only when browser behavior is touched); a change isn't done while `make all` leaves generated files uncommitted.

## Hard rules

- **No build outputs in git** (binaries, `*.test`, `.wasm`, generated files over 1 MB). Build with `-o build/<name>` or `go build ./...`; never `go build` bare inside a main package. Before every commit run `git status --short` and `git diff --cached --stat`, and stage files by name.
- **Never hand-edit generated files** (`zz__gen.go`, `godspecs/*.god.json`, `zz__gen.god.json`): change the generator or template and regenerate.
- **No demo assets in tests or docs.** Tests use `testdata/assets` (drawn in code, size-guarded) and `testdata/gametypes`; docs use neutral sample names (`TestDocsDoNotNameExamples`).
- **One module.** `./...` covers everything. `godspecs/` holds the engine's own GOD files; other GOD output goes to `build/god`.
- **Import rules, enforced by `go test ./internal/deps/`:**
  - `god` imports only the ECS and the stdlib; `golang.org/x/tools` only in `tools/internal/gen`.
  - Ebiten only in `render`, `sound`, `input/ebiteninput`, `game`, `editor`, `tools/wpix-editor` and tests of `tools/internal/anim`; debugui only in `editor/toolshell/debugpanel` and `tools/wpix-editor/internal/native`.
  - stdlib only: `input` (panics start `input: `), `anim/animfile`, `tiled`; `asset/library` adds only the asset module.
  - `gfx` imports neither Ebiten nor `render`; `render` never imports `ecs`.
  - Cobra only under `tools/` and never in the js/wasm build of `wpix-editor`; shared CLI helpers in `tools/internal/cli`.
  - Pixel-reading tests need a display and use the `ebitentest` build tag.
- **Generated Game Object code uses exported ECS API only**: it must compile from other modules (`extmod`).
- **Panics for programmer errors, errors for bad input.** `god` panics start with `god: `.
- **Hot paths are 0 allocs/op** (generated views, extract systems, `Renderer.Draw`, asset `Store.Get`), enforced with `testing.AllocsPerRun` tests.

## Code quality and conventions

- Readability first: clear names, small functions, plain control flow. An optimization that hurts readability needs a benchmark (`make bench`, `make benchcmp`).
- Model the domain: each concept gets its own type, and its own file or package where it fits; no god object. New unexported packages go under the area's `internal/`.
- Every exported identifier has a doc comment with its contract (preconditions, ownership, pointer validity, cost) and its panics. Comments explain why. Explain performance tricks inline so nobody "simplifies" them away.
- Collect all validation errors (`errors.Join`, generator diags with `file:line:col`).
- File names are kebab-case (`schema-registry_test.go`). Tests use the standard `testing` package, table-driven with `t.Run`, and `mustPanic(t, want, f)` (see [god/schema-registry_test.go](god/schema-registry_test.go)).
- Modern Go: `iter.Seq2`, range-over-func, `for i := range n`, `reflect.TypeFor[T]()`, `slices`, `maps`. Generated code stays thin and calls tested hand-written helpers.

## Skills

| Skill | Use when |
| --- | --- |
| `plan`, `implement` | The user runs `/plan` or `/implement` |
| `engine-status` | Checking what is built, partial or not started, or what to do next |
| `game-object-gen` | `wpix gen`, its template or tests, `testdata/gametypes`, `extmod`, GOD JSON, `god` |
| `doc-writer` | Any documentation change (runs on Haiku) |
