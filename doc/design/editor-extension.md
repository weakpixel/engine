# Editor in VSCode

How the VSCode extension (weakpixel-engine-vscode) hosts the level editor and the tools.
The editor's design is in [editor.md](editor.md), the messages in [editor-protocol.md](editor-protocol.md).

## The extension

The extension is the separate repository [weakpixel/weakpixel-engine-vscode](https://github.com/weakpixel/weakpixel-engine-vscode) (TypeScript, no bundler).
The protocol above is versioned together: `bridge.Version` here and `PROTOCOL_VERSION` in its `src/controller.ts`; a mismatch makes the extension refuse the game.
`docEdits.ts` and `controller.ts` hold the logic and import nothing from `vscode`, so `node --test` covers them; `levelEditor.ts` (the `CustomTextEditorProvider`), `build.ts` (the wasm build as a VSCode task) and `outline.ts` (the TreeView) are thin VSCode glue.
Document edits are computed with `jsonc-parser` `modify` at a JSON path, so a drag or an inspector change is a one-line diff that keeps the file's formatting.

The webview page (`media/`) relays protocol messages between the extension (`{to: "game"}` / `{from: "game"}` over `postMessage`) and the game.
It exposes `globalThis.weakpixelHost` for transport and page configuration.
The page has its own messages: `{from: "page"}` `started`, `drop` (an Explorer URI and device pixels), `setField`, `revertField`, `openPrefab` and `dropField` from the inspector; `{to: "page"}` `inspect`.

**The inspector reads the document, not the game.**
The extension sends the selected object's JSON from the document with its schema (from `ready`), the level's ids and the field errors parsed from the last `applied` (`objects[i] (<id>): <Component>: $.<path>: <message>`).
A committed field becomes one `setField` edit; the changed document comes back to the game like any other.
So the inspector shows exactly what will be saved, needs no wasm exports, and works while the game is still loading.
Widgets come from the `TypeSchema`: numbers (integer checks, minimum, maximum), checkboxes, text, asset keys (with drop), entity ids (a select), `x-enum` integers (a dropdown of the constant names), vector-like objects as one row, short fixed arrays as one row, and JSON text for maps and long arrays.
`x-editor` hints (`+game:editor`) refine them: a slider from `min`/`max`/`step`, a color picker, degrees for radians (`unit=deg`), a label, an order, hidden fields and fields shown only while a sibling has a value (`showIf`).
They need no protocol change: the schemas in `ready` carry them.
Keys typed in it stop at the inspector so neither the game nor VSCode keybindings (Delete) see them.

**Level names.**
A level may carry a top-level `name` like a prefab (`Level.Name()`, `Session.Name()`).
The extension is meant to list levels by it, with the path as fallback, via a `prefabLabel`-style helper; that change is planned in the extension repository and needs no protocol change.

**Attached objects in the viewport.**
A part's `Transform`, and that of any object with `hierarchy:Parent`, is derived by the hierarchy stage, so the handles edit its `Local` instead.
A gesture sets the live `Transform` as for any object (so the art follows the cursor), then `syncLocal` converts the field it changes (`Pos` for a move, `Rot` for a rotate, `Scale` for a scale) with `hierarchy.LocalOf(parent's Transform, world)`; the other `Local` fields and `Entity` are untouched.
The finished gesture is one `Edit` with component `hierarchy:Parent` and the whole component (the entity as an id), instead of `gfx:Transform`; for a prefab instance or one of its parts it is the override against the prefab (`Part` set for a part), so the file gets `children.<part>["hierarchy:Parent"]` with only the changed fields.
A click selects the part under the cursor; Alt+click selects its instance root.
When a selected object has a selected ancestor, a move skips it (it follows the ancestor).
A selected attached object gets a dotted line to its parent.
`bridge.Version` stays 0.
A multi-object `*.prefab.json` opens in the prefab stage like any prefab: its document has several objects, the hierarchy stage runs, and the same handles edit the `Local` in the prefab document.
Esc does not cancel a drag.
Re-parenting in the editor is not built.

**Prefabs in the extension.**
A prefab is listed by its top-level `name` (file name as fallback); the game's document parser accepts the key and ignores it, so `bridge.Version` did not change.
`src/prefabs.ts` repeats the engine's merge (its cases are the shared fixture `god/testdata/merge-patch.json`, copied to the extension's `test/fixtures/`) and resolves variant chains by reading prefab files from the asset root.
The inspector gets an instance merged, with its own overrides: overridden fields are marked and have a revert button, a committed field is written as an override at its path (a field inside an array writes the whole array, which overrides replace), and removing a component the prefab has writes `null`.
The custom editor also opens `*.prefab.json`: a prefab is a GOD document, so the game opens it like a level.
The commands Create Prefab from Selection, Unpack Prefab and Open Prefab are document edits plus a file write; the game is not involved.

**The editor test game.**
`tools/wpix-editor/testdata/game` is a small game for the extension's browser test: built with `-tags editor` it is the editor build, and run without the tag (`go run ./tools/wpix-editor/testdata/game -o <dir>`) it writes its asset library to a folder for the page to serve.
Its assets are the generated test library of `testdata/assets` plus `world-1/level-1.level.json` (two plain objects and two instances of `global/prefabs/villager.prefab.json`), and it uses only the engine's own components, so it needs no `go generate`.
`go test ./tools/wpix-editor/` opens its level natively and cross-builds it to WebAssembly.

**Assets come from the workspace.**
The extension serves the asset root to the webview (`asWebviewUri`), the game reads it with `httpfs` (asset module), and a saved file is sent as `assetsChanged`.
Ebiten sizes its canvas from `body.clientWidth`, so the inspector column narrows `body` instead of padding it.
