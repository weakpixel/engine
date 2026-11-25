# Tool shell

How the `wpix-editor` tools are built on one shell.
The tools themselves are in [editor-tools.md](editor-tools.md) and [previewers.md](previewers.md); the level editor is in [editor.md](editor.md).

## Tools

`wpix-editor` (`tools/wpix-editor`) is a second binary that opens one file in the tool of its kind on a shared shell, `editor/toolshell`: `wpix-editor <file>`, with no tool names and no modes.
It is built from the user's module, like the level editor's game, and runs natively in a window or, built for js/wasm, in the extension's webview.

- **One kind table.**
  `editor/filekind` (standard library only) lists every file kind once: its suffixes (the engine's own formats `.sheet.json`, `.anim.json`, `.fx.json` and the Aseprite exports in lower case only, the rest in any case), whether it is edited and whether it has sound.
  The file name is the only rule; the content is never looked at, as for the engine's loaders.
  `editorapp.Kinds` adds the tool of each kind, its help line and its starter.
  An edited kind (sheet, anim, fx) has its own editor; every other kind opens in the preview router of `viewer`, so a `*.sheet.json` has one tool, not an editor and an atlas preview.
  The suffix list lives in its own package because `viewer` routes by it and `editorapp` imports `viewer`.
  `editorapp.Table` prints the table in the form the extension's file lists are compared with.
- **The tool contract is tested, not only documented.**
  The shell finds a tool's optional interfaces (`Syncer`, `Initializer`, `Closer`, `Checker`, `DocumentTool`) with type assertions, so a method with a wrong signature would silently drop the behavior; every tool asserts the interfaces it implements at compile time (`var _ toolshell.Syncer = (*Tool)(nil)`), and the viewer does the same for its previewers.
  `TestToolContract` (`tools/wpix-editor/internal/editorapp`) opens a sample file of every kind of `editorapp.Kinds` in its tool the way the page build does (host transport, host document for an edited kind) and sends every editable panel field back with its own value, a button with `null`; any `error` fails, so a field without its case in `Set`, or one `Set` always refuses, fails in tests instead of on the first click.
  `TestToolCapabilities` checks that an edited kind's tool is a `DocumentTool` and that only the `view` tool can switch files (a `Closer` without a document), which the extension relies on when it sends `open`.
  A new kind needs a sample in `contract_test.go`, or the test fails.
- **Missing files start from a starter.**
  A file of an edited kind that does not exist opens as an unsaved document from its kind's starter (`--image` for a sheet, `--sheet` for a definition, the default effect for an effect); without the flag a missing sheet or definition is an error naming the flag.
  A starter can name files the first save writes with the document (`FileDocument.AddPending`, served by the overlay until then): a new effect copies its built-in sprite next to it, as Save as does, so a saved effect never names a built-in file.
- **The shell is a `game.Tool` on an engine with no game systems**, like the prefab stage.
  Assets load through the engine's asset server and real loaders, the content is drawn by the engine's renderer (the texture and atlas previewers spawn one sprite entity; the font previewer draws its samples itself in `Draw`, because a `gfx.Label` lives in screen space and could not pan or zoom), the shell's camera is `Engine.Camera`, and the grid and a tool's overlays are drawn in `DrawFinal` in window pixels.
  The tick mode is always `TickPause`, so only the engine's own stages run.
  The engine is configured with `IntegerScale` and a logical size larger than any window, which makes the logical screen the window itself: viewport, mouse and overlays share one pixel space.
- **A tool that installs engine packages does it in `Init`.**
  `toolshell.Initializer` (optional) is called from the shell's `game.Tool.Init`, before the schedule is built, because `AddRunner` panics after the first update.
  Its systems still do not run under `TickPause`; the tool steps them itself (the voice previewer calls `Speech.Step` in `Sync`).
- **Tools describe their panel; they do not draw it.**
  A `Panel` is data (sections of fields: number, checkbox, dropdown, text, button, label) with ids, and `Tool.Set(id, value)` takes a change from either renderer.
  Natively `debugpanel` draws it with debugui.
  In VSCode the shell sends the same JSON and the extension draws HTML inputs, so the native build looks like a debug tool and the VSCode one like VSCode.
  The shell compares the marshalled panel every tick and sends it when it changed; a change of labels only (a mouse readout) at most every 100 ms, the answer to a `set` at once (even unchanged, so an input shows the value the tool accepted).
  Per-field diffs were rejected: panels are small and the extension rebuilds a section by field id and keeps focus.
  The widgets are number, checkbox, dropdown, text, button, label, range (`[lo, hi]` that never cross), color (`"#rrggbbaa"`; natively a swatch whose Edit button opens a picker with a saturation and brightness square, hue and alpha bars and a hex field, since debugui has none, and in VSCode the native color input with an alpha slider) and asset (a key from `options`, `kind` image or shader, `thumbs` for entries the page cannot fetch); a tool adds a widget in both renderers in the same change.
- **Two panels over a full-window viewport.**
  Each `Section` has a `Side` (`left` or `right`, empty is right; `Panel.Check` rejects others): a tool tags its sections and never lays out widgets.
  The left side holds the view settings (`Shell.ViewSection` is left) and the lists content is picked from, the right side the properties of the selection.
  The panels are overlays: the camera draws the whole window, and `Renderer.Insets()` (the left and right widths in window pixels, 0 for a panel not shown) only define `Shell.FreeRect()`, the area between them.
  Fit and auto-fit center the content in that area and zoom from its size (auto-fit refits when it changes); zoom at the cursor, hit tests, gizmos and the grid keep window coordinates, and content may sit under a panel.
  `debugpanel` builds both debugui windows in one update (so its captured flag covers both) with fixed widths, since debugui windows restart on resize: left `min(w/5, 220)`, right `min(w/4, 280)` UI pixels times the UI scale.
  The left panel is hidden when the window is under 800 UI pixels (its sections join the right one) or the tool has no left section; the status line is at the bottom of the right panel, or of the left one when there is no right section.
  The `wpix-editor` tools turn on the screenshot hotkey ([`game.DevScreenshotKey`](../guide/engine/engine.md#hotkey-and-environment-variables): F12, F9 in a browser) next to the Screenshot button of the view section.
  Tab (a shell shortcut, which tools must not bind) hides both: the renderer is neither updated nor drawn and the insets count as 0.
  `Toolbar` is one row of icon buttons centered at the top of the free area, so it follows the insets and Tab.
  A dropdown with `Field.List` rows (`Field.AsList`) is a list: `debugpanel` draws a label, then a scrollable panel of that many rows (fewer when there are fewer options) with one button per option, the selected one marked `> `, and a click sets the option index like the dropdown does.
  Lists put a `(none)` option first when the selection can be cleared.
  `Check` rejects `List` on any other widget.
  The JSON gains only `side` and `list`, which the extension ignores (one column, a list stays a dropdown).
- **Three document backends.**
  A `DocumentTool` edits a text file through `Shell.Edit(text)` with the bytes of its format's encoder; the shell never diffs.
  `FileDocument` (native) keeps its own undo stack, saves with Ctrl/Cmd+S, marks the title dirty, reloads a file changed on disk when it has no unsaved edits and reports the conflict in the status line when it has.
  `HostDocument` (VSCode) sends `edit` and gets `document` back, ignoring the echo of its last edit, as `Session` does; VSCode owns undo and save.
  `MemoryDocument` is an untitled document with the undo stack of `FileDocument` (shared code), no file and no host; it is dirty from the start and `Shell.Create` turns it into a `FileDocument`.
- **One document model.**
  `toolshell.Model[F]` is the model of a tool whose state is one decoded file: `File`, an `Encode` func and `Emit` (the tool passes `Shell.Edit`).
  `Model.Do(edit)` runs a change on `File`, encodes it and emits the text, so one change is one document edit.
  Its `toolshell.DocStatus` (`Loaded`, `Stale`, `ReadOnly`) refuses edits with `ErrReadOnly`, `ErrStale` or `ErrNoDocument` through `Editable`.
  Decoding stays with the tool, because it also loads what the file names.
  The sheet and anim tools embed `Model`; the fx tool embeds only `DocStatus`, since its model is the emitter list and a copy of the selected emitter.
- **Shared texture drawing.**
  `editor/viewkit` holds what the tools that show a texture share: `Texture` (one sprite entity with its top-left corner at world (0, 0), pinned at the center of its visible part, since the engine culls a sprite by its position alone), `Frames` (frame outlines, label modes, hover, click selection and the left Frames list) and `Labels` (overlay text).
  The texture, atlas and shader previewers and the sheet tool use `Texture`; the atlas previewer and the sheet tool use `Frames` (ids `atlas.labels`, `atlas.frame` and `sheet.labels`, `sheet.frame`); every previewer that writes overlay text uses `Labels`.
- **debugui stays out of the wasm build.** debugui is imported only by `toolshell/debugpanel` and `tools/wpix-editor/internal/native`, and Cobra, which the command line uses, only by `tools/wpix-editor/cmd`.
  A game module that builds `wpix-editor` for js/wasm (the extension does) then needs no new `go.sum` entries (neither for debugui nor for Cobra), because Go only asks for modules of packages it loads.
  The native build needs debugui in the game module's `go.sum`: the module runs it as a Go tool (`tool` directive, `go get -tool`), which adds debugui to `go.mod` and `go.sum`.
  `internal/deps` enforces both.

**Tool protocol (js/wasm only).**
The same page transport as the level editor (`weakpixelHost.fromGame` and `toGame`), JSON objects with a `type`.
The page connection, the host's settings and the protocol version are in the leaf package `editor/hostlink` (`hostlink.ConnectPage`, `hostlink.Version`), which the bridge uses too, so a tool links neither the bridge nor the level editor.
The page fields are `path` (the file's asset key), `untitled` (bool), `assetBase` and `files`; there is no tool field: the wasm build picks the tool from the file name in `path` through the kind table (`hostlink.Path`).
With `untitled` true the file has no name on disk yet and opens as an untitled document from its kind's untitled text; only a particle effect can be untitled (VSCode's "New Particle Effect" sends `path` `untitled.fx.json`), and its `ready` has `path` `""`.
These are page fields, not messages, so `hostlink.Version` stays 0.
A level editor page and a tool page never share a game, so message types may repeat.
`hostlink.Version` is sent in `ready` (`bridge.Version` is the same constant).

| Direction | Type | Fields | Meaning |
| --- | --- | --- | --- |
| tool → host | `ready` | `version`, `tool`, `path`, `document` (bool), `panel` | First tick, and after every `open`. `tool` is the kind's tool: `sheet`, `anim`, `fx`, or `view` for every previewed kind (the host allows `open` for `view`). `path` is the file's asset key, `""` for an untitled document. `document` says the tool wants the file's text. |
| tool → host | `panel` | `panel` | The panel changed. |
| tool → host | `edit` | `text` | Replace the document with this text (document tools). |
| tool → host | `create` | `path`, `text`, `files` (`[{path, data}]`, data base64) | Write a new document and the files it names, asking the user before overwriting, then open it in the tool and close the untitled one (an untitled tool's `ready` has `path` `""` and `document` false). Paths are asset keys. |
| tool → host | `open` | `path` | Ask the host to open the file at asset key `path` (a level found by a Tiled previewer, a map to preview). A request: the host decides how the file opens (a `.level.json` in the level editor, a previewable file in the Preview panel, anything else as text) and an unknown key is the host's error. Natively there is no host: `Shell.OpenFile` puts "Open <key> in your editor" in the status line. The host's `open` below has the same name and the opposite direction. |
| tool → host | `screenshot` | `id`, `png` (base64), `width`, `height`, `tick`, `error?` | The answer to a host `screenshot`, or (`id` `""`) a capture the tool took itself, from the Screenshot button or the hotkey: the same message as the level editor's. The tool's content only, at window size; the grid, the overlays and the panels are never in it. Under a host the shell installs the screenshot sink, so no file is written. |
| tool → host | `error` | `message` | A message failed, or the file or an asset failed to load. |
| host → tool | `document` | `text` | The document's current text (after `ready`, and after every change). |
| host → tool | `set` | `id`, `value` | A widget changed; answered by `panel` (or `error`). A button's value is `null`. |
| host → tool | `assetsChanged` | `paths` | As for the level editor. |
| host → tool | `screenshot` | `id`, `target?`, `scale?`, `keepAlpha?` | As for the level editor; answered by `screenshot`. |
| host → tool | `open` | `path` | Show the file at asset key `path` in the same game, without a page reload; answered by `ready` for it (after an `error` if it fails to load). Only `view` can switch files (a `toolshell.Closer`); other tools answer with an `error` and keep their file. |

The view section every tool shows (grid, cell and offset, background, zoom, Fit, 1:1 and Screenshot) comes from `Shell.ViewSection`, and the shell handles its `set`s. A checker background is a world layer, since the grid and overlays are drawn after the world: the `fx` tool adds a generated checker sprite on the lowest layer (the `anim` tool does not: the Dark, Black, Gray and Light backgrounds are enough for pixel art).
A tool that reads a file as raw bytes (not through a loader) uses `Shell.FS`, the asset filesystem the command passes in `Config.FS`.
