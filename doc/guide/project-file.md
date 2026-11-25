# Project settings: `.weakpixel.json`

`.weakpixel.json` sits at the root of a game project and tells the tools and the [editor](engine/editor.md) where the project's files live and how to run the game. Every key is optional, and so is the file: without it, the tools keep their defaults.

```json
{
  "god": {
    "out": "build/god/project.god.json"
  },
  "godDoc": {
    "out": "build/god-doc"
  },
  "schema": {
    "out": "build/schema"
  },
  "assets": {
    "root": "assets",
    "prefabs": "global/prefabs"
  },
  "editor": {
    "game": ".",
    "tags": ["editor"],
    "snap": 8,
    "prefab": { "background": "#14121a", "grid": 16 }
  }
}
```

| Key | Used by | Meaning | Without it |
| --- | --- | --- | --- |
| `god.specs` | `wpix gen`, `wpix doc` | Folder of the per-package GOD files. `go generate` writes each package's file there as `<group>.god.json`; `wpix doc` reads that folder when no inputs are given. The GOD files are build output: leave the key out and they go to `build/god` below the module root (ignored by git). Set it to a folder you commit if people without Go work in the project with the VSCode extension, which reads the files for the inspector, code lenses and the assets view. | `build/god` below the module root; `wpix doc` reads that folder when it exists, else `./...` |
| `god.out` | `wpix doc --format god` | The merged GOD of the whole project (all components and objects in one file). | `--format god` needs `-o` |
| `godDoc.out` | `wpix doc` | Folder for the rendered docs: `index.html`, `god.md`, or the `--split` Markdown files. | `god-doc.<format>` (`god-doc/` with `--split`) in the current directory |
| `schema.out` | `wpix schema` | Folder the JSON schemas of the game's JSON files are written to, see [tools](tools.md#schema). | `build/schema` |
| `assets.root` | `wpix assets`, VSCode extension | The asset [library](engine/asset-library.md) root. Asset keys are relative to it. | `wpix assets` needs the `<root>` argument; the extension uses the nearest folder above the file that holds a `global/` folder or a `library.json` |
| `assets.prefabs` | VSCode extension | Folder under `assets.root` where Create Prefab from Selection starts. Must stay inside the root. | `<package of the open level>/prefabs`, else `global/prefabs` |
| `editor.game` | VSCode extension | Directory of the package the editor builds to WebAssembly (the one with the [editor `main`](engine/editor.md#making-a-game-editable-in-vscode)) | The nearest directory with a `package main` at or above the opened file |
| `editor.tags` | VSCode extension | Build tags of that build, an array | `["editor"]` |
| `editor.snap` | VSCode extension | Grid in world units that dragged objects snap to; 0 is off. Alt ignores it while dragging. | 0 |
| `editor.screenshots` | VSCode extension | Folder the extension saves [screenshots](engine/engine.md#screenshots) of the level editor and the tool panels into, relative to this file; the editor build never writes files itself (see the [protocol](../design/editor-protocol.md#protocol)) | `screenshots` |
| `editor.prefab.background` | VSCode extension | Background of the [prefab stage](engine/editor.md#stages) as `#rrggbb` | `#24242a` |
| `editor.prefab.grid` | VSCode extension | Grid step of the prefab stage in world units; 0 draws no grid | 16 |
| `editor.rate.idle`, `editor.rate.active`, `editor.rate.hidden` | Editor hosts | Tick rates of the editor build's [pacing](engine/editor.md#pacing): while nothing happens in edit mode, while you work in the view, while the view is hidden (0 stops ticks) | 10, 60, 0 |

With `assets.root` set, `wpix assets` runs without its `<root>` argument (`wpix assets check`, `wpix assets keys --pkg main -o zz__gen.assets.go`); a root on the command line wins.

## Rules

- **Lookup.** A tool finds the file by walking up from its directory (the package directory for `wpix gen`, the working directory for `wpix doc`) and uses the first `.weakpixel.json` it meets. The walk stops at the Go module root (the first directory with a `go.mod`), so a nested module never uses the file of the module around it and keeps its own.
- **Paths** are relative to the directory holding the file.
- **Unknown keys are errors.** All of them are reported at once (`unknown key "god.spec"`), as is a section that is not an object (`god: want an object`). A typo is never silently ignored.
- **Flags win.** A command-line input, root or `-o` overrides the file.
- **Values are checked.** `editor.snap` and `editor.prefab.grid` must not be negative, `editor.prefab.background` is `#rrggbb`, `editor.rate.idle` and `editor.rate.hidden` are 0 or more and `editor.rate.active` 1 or more, and `assets.prefabs` stays inside the root. Every problem is reported at once.

Go programs read the file with package `github.com/weakpixel/engine/project` (`project.Load`), which applies these rules; an editor host in another module uses it too, and `github.com/weakpixel/engine/jsonedit` for minimal edits of level and prefab text.

The [GOD guide](ecs/game-object-definition.md) explains the files these keys point at; [Tools](tools.md) lists the commands.
