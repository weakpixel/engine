# Tools

The engine's command-line tools are subcommands of one command, `wpix`. It runs with `go run`, so nothing needs installing. The commands below run from the repository root; from another module, use the full import path (`go run github.com/weakpixel/engine/tools/wpix gen`).

| Command | Does |
| --- | --- |
| [`wpix gen`](#gen) | Generates Game Object code and the package's GOD file from `+game:*` markers |
| [`wpix doc`](#doc) | Merges the GOD files of a project into one GOD, or documents them as HTML or Markdown |
| [`wpix anim`](#anim) | Validates and creates `*.anim.json` animation definitions |
| [`wpix assets`](#assets) | Checks, lists, catalogs and packs an asset library |
| [`wpix tileset`](#tileset) | Packs a texture atlas of differently sized sprites into a uniform-tile sheet and a Tiled tileset |
| [`wpix schema`](#schema) | Writes JSON Schemas for the game's JSON files, so editors complete and check them |
| [`wpix framelog`](#framelog) | Summarizes a frame log and lists the frames that stutter |
| [`wpix serve`](#serve) | Serves a browser build for development and pushes asset changes to the open page |
| [`wpix new`](#new) | Creates a game project that runs, tests, builds for the browser and opens in the level editor |
| [`wpix mcp`](#mcp) | Serves the project to an AI agent over the Model Context Protocol: components, object types, assets, levels and prefabs |
| `wpix help [command]`, `wpix version` | Lists the commands or shows one command's usage; prints the module version |
| [VSCode extension](#vscode-extension) | Edits `*.level.json` levels and `*.prefab.json` prefabs in a live view of the game ([own repository](https://github.com/weakpixel/weakpixel-engine-vscode)) |

Every subcommand exits 0 on success, 1 on errors in the input and 2 on a usage error. The commands are built on [Cobra](https://github.com/spf13/cobra): flags are GNU style (`--long`, and `-o` where a command has an output file), may stand before or after the arguments, and a single-dash long flag such as `-format` is a usage error. `wpix help <command>` (or `<command> --help`) lists a command's flags. Shell completion scripts are not provided.

## Using it in a game

`go run github.com/weakpixel/engine/tools/wpix <command>` always works and uses the engine version in your `go.mod`. To type less, register it once as a Go tool and call it as `go tool wpix`:

```sh
go get -tool github.com/weakpixel/engine/tools/wpix
go tool wpix doc
```

The `//go:generate` lines below also work as `//go:generate go tool wpix gen` once the tool is registered.

## gen

```go
//go:generate go run github.com/weakpixel/engine/tools/wpix gen
```

Put the directive in a package that declares `+game:*` markers, run `go mod tidy` once (the generator needs `golang.org/x/tools`) and `go generate`. It writes `zz__gen.go` next to the sources and the package's GOD file into the project's specs folder as `<group>.god.json` (`god.specs` of [`.weakpixel.json`](project-file.md), found by walking up from the package directory), or, without that key, into `build/god` below the module root (build output: it stays out of git, and a fresh clone needs `go generate` once before the extension shows GOD data). `zz__gen.god.json` files from earlier versions next to the sources are removed. It removes stale files it wrote earlier, and refuses to overwrite another package's file in the specs folder (two packages with the same group). Problems are reported all at once as `file:line:col: message`. Markers and the generated API: [Game Object API](ecs/api-game-object.md). The JSON file: [GOD](ecs/game-object-definition.md#2-god-json-schema).

## doc

`wpix doc` merges the GOD files of a project into one GOD or one document.

```sh
go run github.com/weakpixel/engine/tools/wpix doc                       # docs from .weakpixel.json: god.specs -> godDoc.out/index.html
go run github.com/weakpixel/engine/tools/wpix doc --format god          # merged project GOD -> god.out
go run github.com/weakpixel/engine/tools/wpix doc ./...                 # god-doc.html (without a .weakpixel.json)
go run github.com/weakpixel/engine/tools/wpix doc -o docs.md ./...      # Markdown
go run github.com/weakpixel/engine/tools/wpix doc --split -o docs ./... # Markdown, one file per group
```

- Defaults come from the project's [`.weakpixel.json`](project-file.md) (found by walking up from the working directory); flags and inputs override them.
- Inputs are GOD files (`*.god.json`), directories (searched recursively) or patterns ending in `...` (default: `god.specs`, else `build/god` when it exists, else `./...`). A walk skips merged GOD files that `--format god` wrote (`"generator": "wpix doc"`); named explicitly, they are read. The filesystem is walked directly, so no module loading happens and nested modules are included. Below the root of a search, `vendor`, `testdata`, `node_modules` and directories starting with `.` or `_` are skipped; a root named explicitly is always searched. An input that matches no file is an error.
- All inputs are merged: components by `group:name`, objects by `group:class`. Identical definitions (docs ignored) collapse into one; a different definition under the same key is an error naming both files (`b.god.json: component g:C differs from the one in a.god.json`), and so is an object using a component no input defines. All problems are reported at once.
- The document has an index of all objects (grouped by `group`) and an index of all component types by key, each with its Go type and the objects that use it (possibly none). Components reached through an embedded object are labelled `embedded (Common)`. A field of a [`+game:enum`](ecs/game-object-definition.md#enums) type has the type `enum (A, B, ...)` and lists each value with its name and doc. A field with a [`+game:default`](ecs/game-object-definition.md#defaults) shows `Default: <value>` in its description (an enum by constant name). Output is deterministic.
- `--format html|md|god` (default: from the `-o` extension, `.md`/`.markdown` is Markdown, `.god.json` is god, else HTML), `-o file` or `--output file` (`-` is stdout; default `index.html`/`god.md` in `godDoc.out`, else `god-doc.html`/`god-doc.md`; for `god`, `god.out`, which it then requires), `--title text`. `--split` (Markdown only) makes `-o` a directory (default `godDoc.out`, else `god-doc/`) holding `index.md`, `components.md` and one `group-<name>.md` per group.
- `god` writes the merged GOD: the [GOD layout](ecs/game-object-definition.md#2-god-json-schema) without `package`, with `"generator": "wpix doc"`.
- HTML is one self-contained file (inline CSS and JS, no network); the filter box, the Objects/Components tabs and the light/dark toggle are optional enhancements. The page follows the browser's color scheme until the user toggles it; the choice is kept in `localStorage`. Markdown is GitHub-flavoured with tables.
- Only `version` 0 files are read; regenerate others with `wpix gen`.

## anim

```bash
go run ./tools/wpix anim validate chars/player.anim.json [more.anim.json ...]
go run ./tools/wpix anim create --file chars/player.json [-o chars/player.anim.json | -o -] [--force]   # --file is also -f
```

`validate` checks a definition against its Aseprite sheet with the loader's rules; `create` writes a starter definition. Output, levels and flags: [Animation: wpix anim](engine/animation.md#tooling-weakpixel-anim).

## assets

```bash
go run ./tools/wpix assets check   [--json] [root]
go run ./tools/wpix assets ls      [root]
go run ./tools/wpix assets catalog [root] [-o file|-]
go run ./tools/wpix assets pack    [root] [-o out]          # default build/assets
go run ./tools/wpix assets keys    [root] [--pkg name] [-o file|-]
```

`check` reports missing references, case mismatches, levels using each other's files, bad names and oversized textures (`check --json` prints the findings as JSON for editors); `catalog` and `pack` write the catalog and the packed build (`pack -o` must be empty or missing, default `build/assets`); `keys` writes optional Go constants for the keys ([generated keys](engine/asset-library.md#generated-keys-optional)). `<root>` may be left out when [`assets.root`](project-file.md) is set. Exit codes 0 ok, 1 errors, 2 usage. Details: [Asset library: wpix assets](engine/asset-library.md#weakpixel-assets).

## tileset

```bash
go run ./tools/wpix tileset from-atlas [-o out.png] [--tile WxH] [--columns n] [--margin n] [--spacing n] [--align center|topleft|bottomleft] [--image atlas.png] [--name s] <atlas.xml>...
```

`from-atlas` reads one or more packed Starling/ShoeBox `TextureAtlas` (`<SubTexture>` XML plus its PNG) and writes a PNG in which every sub-texture sits in a cell of one size, plus a Tiled JSON tileset (`.tsj`) next to it. Load the `.tsj` as a [tileset](engine/tilemaps.md) or open it in Tiled.

- The cell is the largest frame of the atlas unless `--tile` sets it. A sub-texture larger than the cell is an error, never cropped. Smaller ones are placed by `--align` (default `center`; frames that already fill the cell are unaffected), the rest of the cell is transparent. Trim offsets (`frameX`/`frameY`) are honoured.
- Tile ids follow the order of the XML. Each tile has a string property `name` with its sub-texture name, to find a tile again after packing.
- `--columns` (default 16, at most the tile count), `--margin` and `--spacing` (default 0) set the grid. `-o` or `--out` (default `<atlas>.tiles.png`) must end in `.png`; the tileset is written as `<out>.tsj`, named by `--name` (default the output name without `.png`). The PNG is non-premultiplied RGBA.
- The image is `--image`, else the XML's `imagePath` next to it, else the XML's name with `.png` (exports often name a stale `imagePath`).
- Rotated sub-textures (`rotated="true"`, or a frame that is the rect with width and height swapped, which ShoeBox writes without the flag) are turned back upright, counter-clockwise (the atlas stores them clockwise). Errors (a sub-texture larger than the cell, a rect outside the image, a frame that does not hold the pixels) are reported all at once with the sub-texture name. Only PNG is read. Several atlases (`from-atlas *.xml`) are converted one by one, each to its own `<atlas>.tiles.png` and `.tsj`; `-o`, `--image` and `--name` then are errors, and a failing atlas does not stop the others (exit 1). Quoted patterns are expanded by the tool. Exit codes 0 ok, 1 errors, 2 usage.

## schema

```bash
go run ./tools/wpix schema [-o dir] [god inputs...]   # -o is also --out
```

Writes `<kind>.schema.json` (JSON Schema draft-07) for `anim`, `sheet` (grid sheets), `atlas` (TexturePacker), `bindings`, `library`, `weakpixel` (`.weakpixel.json`), and `level` and `prefab`. The first six have a fixed shape; `level` and `prefab` come from the project's GOD files: every registered component is a property of `components` with its field types, `x-enum` names, asset kinds and doc comments, an object of a known class requires its non-optional components, and an object with `prefab` takes overrides, where nothing is required. A tool such as the VSCode extension points `jsonValidation` at the files. The GOD inputs are files, directories or `./...` patterns as for `wpix doc`, default `god.specs` of [`.weakpixel.json`](project-file.md), else `./...`; without GOD files `level` and `prefab` are skipped. Output folder: `-o`, else `schema.out`, else `build/schema`. The files are generated: don't edit them.

## framelog

```bash
WEAKPIXEL_FRAMELOG=/tmp/frames.csv go run .   # play, then quit
go run ./tools/wpix framelog [--skip 60] /tmp/frames.csv
```

Reads a [frame log](engine/engine.md#frame-log) and prints the frame count, histograms of the draw gaps (whole milliseconds) and of the ticks per frame, then one line per hitch with its reasons:

| Reason | Meaning |
| --- | --- |
| `N ticks` | the frame ran 0 or 2+ `Update`s instead of 1 |
| `late`, `early` | the draw gap is above 1.5x or below 0.75x the median |
| `gc` | a GC cycle finished during the frame |
| `scroll S, around E` | while the world scrolls, the frame's scroll step differs from the median of the 5 frames on each side by more than max(1 px, E/4) |

`--skip` drops the first rows (default 60, about one second), where window creation and asset loading make every frame late. A cut-off last line (the game was killed while writing) is ignored with a note. Logs from before the scroll columns are read without the scroll check. Exit codes: 0 ok, 1 unreadable or unparsable log, 2 usage error.

## serve

```bash
go tool wpix serve [--addr :8080] [--assets <dir>] <dir>
```

Serves `<dir>` (the page and `game.wasm`) at `/` and the `--assets` directory at `/assets/`, both with `Cache-Control: no-cache`. It polls the asset tree every 250 ms (modtime and size) and sends the keys of new, changed and removed files, slash-separated and relative to the asset directory, as server-sent events on `/_weakpixel/changes`: one `data: {"paths": [...]}` per batch, the [change-event format](https://github.com/weakpixel/asset/blob/main/doc/guide.md#change-events) `httpfs.WatchChanges` reads. The game side is `httpfs.WatchChanges` and `game.Config.AssetChanges` ([hot reload](engine/assets.md#hot-reload)). A browser cannot list directories, so `/assets/assets.catalog.json` is answered from a scan of the asset directory on every request, unless the directory has its own catalog; a file added while the page is open shows after a reload of the page. `make web-dev` of a new game runs it. Exit codes: 0 ok, 1 the server failed, 2 usage error.

## new

```sh
go run github.com/weakpixel/engine/tools/wpix@latest new [flags] <dir>
```

Creates a game module in `<dir>` and makes it ready to open: it writes the files, then runs `git init`, `go mod tidy`, `go generate ./...` and `go build ./...` in it. The new module requires the engine at the version the tool runs as (a pseudo-version of `main` under `@latest`) and registers the tool, so every other command runs there as `go tool wpix <command>` at that version.

| Flag | Default | Does |
| --- | --- | --- |
| `--module path` | the base name of `<dir>` | The module path |
| `--name group` | the base name of `<dir>` in lower case, other characters as `_` | The game's Game Object group (`+game:group:name`), `[a-z][a-z0-9_]*`, not an engine group such as `gfx` |
| `--replace path` | none | Adds `replace github.com/weakpixel/engine => <path>` (absolute) for a local engine checkout. Required when the tool runs from a checkout (`go run ./tools/wpix new`), where its version is unknown |
| `--force` | off | Writes into a directory that is not empty. Existing files are never overwritten: if any file the tool would write exists, it names every one and writes nothing, with or without `--force` |
| `--no-git` | off | Skips `git init` (it is also skipped, with a note, when `git` is not installed) |
| `--no-run` | off | Only writes the files: no `git init`, `go mod tidy`, `go generate` or `go build` |

All problems with the flags are reported at once. Exit codes: 0 ok, 1 an existing file or a failed step (its output is printed), 2 a usage error.

What it creates:

| Path | What |
| --- | --- |
| `go.mod` | The module, requiring the engine, with `tool github.com/weakpixel/engine/tools/wpix` |
| `.weakpixel.json` | [Project settings](project-file.md): `god.specs` `build/god` (the default), `god.out` `build/god/project.god.json`, `godDoc.out` `build/god-doc`, `assets.root` `assets`, `editor.snap` 8 |
| `main.go` | Opens the window and runs the game (not in the editor build). `-assets <dir>` reads loose files with hot reload, `-pack <dir>` writes the [packed build](engine/asset-library.md#packs) |
| `assets.go`, `assets_web.go` | The asset library: embedded, or packs over HTTP in the browser build with the `webassets` tag |
| `editor.go` | The [editor build](engine/editor.md#making-a-game-editable-in-vscode) (`-tags editor`) on `editor/host` |
| `main_test.go`, `editor_test.go` | The game and the editor build ticked without a window: the player walks, the editor opens the level |
| `game/` | Package `game`: `game.go` (settings, input actions, `AddSystems`, `LoadFirstLevel`), `components.go` (a `PlayerControl` component and a `Player` Game Object), `player-system.go`; `zz__gen.go` is generated |
| `<god.specs>/<group>.god.json` | Generated by `go generate` |
| `assets/` | The [library layout](engine/asset-library.md#the-layout): `global/input/player.bindings.json`, `global/prefabs/player.prefab.json`, `global/sprites/player.png` (a placeholder the tool draws), and the first level `start/start.level.json`, which places the player prefab |
| `web/index.html` | The [web shell](engine/web.md) |
| `Makefile` | `run`, `dev` (loose assets, hot reload), `generate`, `test`, `check`, `doc`, `web`, `web-packed`, `web-dev` (browser build with hot reload), `editor`, `serve`, `clean` |
| `README.md`, `CLAUDE.md` | What each file is and the commands; short notes for coding agents that point to this guide |
| `.vscode/extensions.json`, `.gitignore` | Recommends the [VSCode extension](#vscode-extension) and the Go extension; ignores `build/` and `*.wasm` |

The first level is `start/start.level.json` in the package `start`. `game.LoadFirstLevel` loads it with `LoadLevel`, which mounts the package first (at once from embedded files, after its pack is mounted in the `webassets` build); the editor build does not call it, because the editor opens the level itself.

## mcp

```sh
go tool wpix mcp [--project dir] [--game dir] [--tags editor,...]
```

An [MCP](https://modelcontextprotocol.io) server on stdin and stdout for an AI agent such as Claude Code. It serves one client until stdin closes. stdout carries only protocol messages; diagnostics go to stderr. It speaks the protocol revision 2026-07-28 (stateless: no handshake, `server/discover`, the version in each request's `_meta`) and, for older clients, 2025-11-25 and 2025-06-18 through `initialize`. It offers tools only: no resources, prompts or subscriptions. It is written on the standard library; no MCP SDK is linked.

Register it in a game made by [`wpix new`](#new):

```sh
claude mcp add weakpixel -- go tool wpix mcp
```

or with a `.mcp.json` at the project root:

```json
{
  "mcpServers": {
    "weakpixel": {"command": "go", "args": ["tool", "wpix", "mcp"]}
  }
}
```

`wpix new` does not write this file.

| Flag | Default | Does |
| --- | --- | --- |
| `--project dir` | the working directory | Where the lookup of [`.weakpixel.json`](project-file.md) starts. The project root is the folder of that file, else the Go module root |
| `--game dir` | `editor.game` | Package of the editor build that `game_start` builds and runs |
| `--tags list` | `editor.tags`, else `editor` | Build tags of that build |

The server reads:

- **The GOD** from `god.specs` (else every `*.god.json` below the project root) and the `godspecs/` folder of the engine module the project uses (`go list -m github.com/weakpixel/engine`), so engine components a level adds are known even when the game's own GOD files do not list them. It reloads the files when one changes.
- **The asset library** at `assets.root`, else the first folder at or up to three levels below the project root that holds a `global/` folder or a `library.json`.

Tools. The project and document tools only read; the edit tools further down change files.

| Tool | Returns |
| --- | --- |
| `project_info` | Project root, asset root, GOD inputs and any load error, editor package, tags and snap, whether a game runs, warnings about missing settings |
| `list_components`, `describe_component` | Component types by key (`group:Name`, optional `group` filter); one type's JSON schema with its `x-editor` hints, the initial value an editor writes, its doc and the object types that use it |
| `list_object_types`, `describe_object_type` | Object types by key (`group:Class`); one type's components with field, optional, readonly and embedded-via |
| `list_assets` | Asset keys with their kind, filtered by `kind` and `prefix`, at most `limit` (default 200, up to 5000), with the total |
| `check_assets` | The findings of [`wpix assets check`](#assets) |
| `list_documents` | Levels and prefabs with name and object count |
| `read_document` | A level or prefab: its text, the SHA-256 of the text, its objects (index, id, type, prefab) and the problems of the offline checks below |
| `get_object` | One object by `id` or `index`; with `merged`, a prefab instance merged over its prefab, and the fields it overrides (`gfx:Transform.Pos.X`) |

The offline checks are what the game would reject: the document parses (only `name` and `objects`, known keys in each object), ids are unique, a prefab holds one object, prefab instances expand (missing prefabs, variant cycles, a `group`/`class` that differs from the prefab's), every object type and component key is in the GOD, required components are present, every value matches its schema, every entity reference (`format: entity`) names an object of the document, and every asset key (`format: asset`) names a file in the asset root of the kind the field takes. Each problem names the object.

### Editing

The edit tools change the file on disk, the way the VSCode extension edits its `TextDocument`: each call reads the current text, changes only the bytes of the edited value (indentation, key order and line ends elsewhere stay, so the git diff is the one or two lines you expect), and writes the file in one step (a temporary file in the same folder, then a rename). They never mutate a running game's state directly: with [a game running](#the-game) on the document, the new text is also sent to it as a `document` message (below); VSCode with the file open picks the change up as it picks up any edit of the file. There is no undo in the server: git and the editor's own undo cover that.

An edit is refused, and the file left alone, when its result has a problem the document did not have before. A document that already had problems can therefore still be fixed one problem at a time; the answer lists the problems that remain. Every edit tool answers with `path`, `changed`, the new `sha`, a `diff` (the changed lines) and the remaining `problems`. A tool that takes `expect` (the `sha` of `read_document`) is refused when the file changed since that read, so read-modify-write is safe.

An object is selected by `id`, or by `index` for one without an id. A field is a path such as `["Pos","X"]`; an integer step indexes an array.

| Tool | Does |
| --- | --- |
| `set_component`, `set_field` | Replace a component the object has, or one field of it. On a prefab instance the value is an override; a field inside an array writes the whole array, since overrides replace arrays |
| `add_component` | Add a component the object lacks, with the given value or its initial value. Refuses one it has |
| `remove_component` | Remove a component. Required components are refused. On an instance a component of the prefab is removed by writing `null`; one only the instance added is deleted |
| `set_override`, `revert_override` | Write an override on a prefab instance; remove one (and the objects above it that held only it) so the prefab's value shows again |
| `place_object` | Append an object: an object type (`class`) with every required component at its initial value and `gfx:Transform` at `x`, `y`, or an instance of a `prefab` that overrides only the position. The id is `<class or prefab name>-<n>` unless given. `layer`, when given, is written into the drawn components; the editor's default layer from a Tiled backdrop is not computed here |
| `remove_object` | Delete an object. Refused, with the referrers named, while another object refers to its id |
| `set_asset_field` | Set an asset field; the key must exist and be a file of the kind the field takes |
| `create_document` | Create an empty `*.level.json`, or a `*.prefab.json` with one object of `class`. Never overwrites |
| `write_asset_document` | Replace the whole text of a `*.level.json`, `*.prefab.json`, `*.sheet.json`, `*.anim.json` or `*.fx.json` after the engine's own decoder accepts it (an animation definition also needs its sheet to resolve; every `sprite` and `shader` key of a particle effect must name an image or shader in the asset root). Takes `expect` |

### The game

The game tools run the project's editor build as a child process and drive it over the [editor protocol](../design/editor.md#protocol), the one the VSCode extension speaks. The build needs `editor.game` (or `--game`): `game_start` runs `go build -tags <tags> -o build/wpix-mcp/<name>` in that package, never next to the source, and starts the binary there. **`build/` must be in `.gitignore`** (the generated starter's is; `game_start` warns when it is not). Only one game runs at a time; it is stopped when the server ends. The tools run the project's own code only: nothing an agent writes is compiled or executed except through the files it edits.

By default the game opens a window, so the developer sees what the agent does. `headless: true` (or `WEAKPIXEL_HEADLESS=1` in the game's environment) runs the same build without a window or sound, ticking at the game's rate, for CI and machines without a display. A headless game draws nothing: `step`, `play` and the edit tools work, `screenshot` is refused.

Both sides share the document: an edit tool writes the file and sends the text to the game, which answers whether it applied it. If the game refuses (an unknown class, a value that does not decode), the file is put back as it was and the error, which names the object and component, is the tool's error. The other way, a drag in the game window ends up in the file with the same minimal edits, and a change of the open file by anything else (the developer's editor) is sent to the game within half a second. Other asset files (textures, shaders, sounds, sheets, effects, prefabs) are polled every 500 ms and reported to the game as changed, which reloads them. Code changes need `game_rebuild`: it builds first, and only when that works replaces the running game, with the same document open and the same camera; a compile error is returned (at most 200 lines) and the old game keeps running.

| Tool | Does |
| --- | --- |
| `game_start`, `game_stop`, `game_rebuild` | Build and start the game on a level or prefab (`path`, `headless`); end it (asked to quit, killed after 3 s); replace it by a rebuilt one |
| `game_status` | Running or not, pid, open document, play mode and pause, the game's tick count and scene stack (names, bottom first), the last error the game reported for a document, and how the previous game ended |
| `read_logs` | The game's stdout and stderr, errors it reported over the protocol (stream `bridge`) and the server's own notes (stream `server`). Each line has a sequence number; pass the returned `next` as `since` to read only newer lines. The last 5000 lines are kept, also after the game ended |
| `list_live_objects`, `select_objects` | The objects the game has from the open document and the selection; select by id, `reveal` pans to them (edit mode) |
| `play`, `stop`, `step` | Run the level (levels only), return to the document's state, run `count` (1 to 600) ticks of the paused game. `step` needs edit mode or a paused play: a running play runs by itself |
| `pause` | Pause (`paused` true, the default) or resume play: the game keeps drawing, its systems stop, `step` advances it, and nothing is reset (`stop` does that). Play mode only |
| `inspect_entities` | Live component values, encoded as the document holds them: every object of the open level, or the `ids` given, only the `components` keys given, with `all` also the entities systems spawned (no id), at most `limit` (default 100; `truncated` says it cut). Works in edit and play mode |
| `send_input` | Hold `keys` (engine key names: `Space`, `A`, `ArrowLeft`), a `mouse` (`x`, `y` in logical pixels, `buttons`, `wheel_x`, `wheel_y`) and a virtual gamepad `pad` (`buttons` as bindings name them: `south`, `start`, `up`; `axes`, `triggers`) for `ticks` ticks (default 1, up to 600) on top of the real devices, then release them. Returns once those ticks ran; in a paused play the input waits for the ticks `step` runs. Play mode only |
| `frame_view`, `get_camera`, `set_camera` | The editor camera |
| `open_document` | Switch to another level of the same stage (a level cannot become a prefab: stop and start again) |
| `reload_assets` | Force a reload of the named asset keys |
| `list_map_layers`, `set_map_layer_hidden` | The Tiled map's tile and image layers behind the level; hide one in the view |
| `editor_settings` | Drag snap grid and collider outlines |
| `screenshot` | A PNG of the running game (`target` `game` or `window`, `scale`, `keep_alpha`) as image content, with its size and tick; `last` returns the capture the developer took with the hotkey instead. Needs a window; nothing is written to disk |

Environment of the game process (set by the server; see [the engine guide](engine/engine.md#host-variables)): `WEAKPIXEL_BRIDGE`, `WEAKPIXEL_BRIDGE_TOKEN`, `WEAKPIXEL_STAGE`, `WEAKPIXEL_PATH`, `WEAKPIXEL_HEADLESS`. The server listens on `127.0.0.1` only, and the game must present the token. The game runs with the editor package as its working directory, as `go run` from there would.

Safety: no tool runs code an agent supplies: no shell, no Lua, no `go run` of other packages (`game_start` builds the configured editor package only). The edit tools write only the documents and asset files named above, inside the asset root; they delete no file. Every path argument is an asset key (relative to the asset root) or an absolute path inside it; `..`, paths outside the root and symbolic links that lead out of it are refused. A tool's failure (bad arguments, an unknown key, a missing file, a refused edit) comes back as a tool error whose text says what to change. Exit codes: 0 when stdin closes, 1 an unreadable `.weakpixel.json`, 2 usage error.

## VSCode extension

The level editor's VSCode extension lives in its own repository, [weakpixel/weakpixel-engine-vscode](https://github.com/weakpixel/weakpixel-engine-vscode): it opens `*.level.json` levels and `*.prefab.json` prefabs in a live view of your game (built to WebAssembly with `-tags editor`) next to an inspector, and writes every change into the file as a normal VSCode edit. Install, settings, commands and development are in its README. What a game needs for it, and the editor core and protocol it talks to, are here: [Making a game editable in VSCode](engine/editor.md#making-a-game-editable-in-vscode) and [doc/design/editor.md](../design/editor.md).

## make targets

Run from the repository root.

| Target | Does |
| --- | --- |
| `make all` | `go generate`, `go fmt`, `go build`, `go test`, `go vet` and the display tests (`make test-ebiten`); no WASM |
| `make test-ebiten` | The pixel-reading tests behind the `ebitentest` build tag; they need a display (`xvfb-run` on a headless machine) |
| `make wasm-build` | Explicit js/wasm cross-build of the module, and of the editor test game with `-tags editor` |
| `make wasm-test` | Runs the tests that need no browser under js/wasm (`go_js_wasm_exec`, needs Node; skipped without it). Not part of `all` |
| `make engine-bench name=<label>` | Engine package benchmarks into `bench/engine-<label>.txt` |
| `make god-doc` | The merged GOD `build/god/project.god.json` and `build/god-doc/index.html` and `god.md` for the Game Objects in the `godspecs/` folder |
| `make bench name=<label>` | Benchmarks of `god` and its subpackages (`./god/...`) into `bench/<label>.txt` |
| `make benchcmp new=<label>` | `benchstat bench/baseline.txt bench/<label>.txt`; fails with a hint while there is no baseline |
| `make race`, `make vet` | Race tests and vet |
| `make modtidy` | `go mod tidy` |
| `make build-tools` | Builds `wpix` into `build/` |
