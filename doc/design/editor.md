# Level editor

How the level editor is built and why.
The API is in the guide: [engine/editor.md](../guide/engine/editor.md).

## Shape

```
VSCode extension (weakpixel-engine-vscode)    webview (Chromium)
  level file = TextDocument  ── postMessage ──>  host script ── weakpixelHost.toGame ──> bridge.Bridge
  undo, save, diff, outline  <── postMessage ──  (media/)     <── weakpixelHost.fromGame ── (game.Tool)
                                                                                        └ editor.Session
                                                                                          └ game.Engine (wasm)
```

- **The game is the viewport.**
  The user's own game, built for js/wasm with `-tags editor`, runs in the editor tab through [`editor/host`](#the-host-and-the-stages).
  Only that binary knows the game's components, systems and Lua bindings, so the extension builds it rather than shipping a generic player.
  An incremental wasm build takes about two seconds.
- **One core, any shell.**
  `editor.Session` is a `game.Tool` with no UI: selection, picking, dragging, the editor camera, applying documents and Play/Stop.
  `bridge.Bridge` puts a JSON protocol in front of it.
  An in-engine shell (Go widgets) or a native process speaking the same protocol over stdin/stdout could drive the same core.
- **The document owns undo.**
  The level file is a VSCode `TextDocument`.
  The viewport never changes the level on its own authority: a drag ends with an `edit` message, the extension turns it into a minimal `WorkspaceEdit`, and the changed text comes back to the game as a `document`, which `Session.Apply` applies by id.
  Typing in the JSON, undo, redo and git checkout all take that same path.
  The session counts the JSON of its own edits as already applied, so the echo changes nothing.
- **The MCP server is a second document backend.**
  `wpix mcp` ([tools.md](../guide/tools.md#mcp)) edits the same files without VSCode: its edit tools compute minimal splices with `jsonedit`, check the result offline against the merged GOD, and write the file.
  The file stays the single source of truth, so a game or an editor that has it open takes the change as any other edit of the text.
- **Ids are identity.**
  Every object in an editor document has an `"id"`.
  Entities change across Play/Stop and class changes; ids do not.
  The extension can rename an id and then rewrites the fields of the level that refer to it, as a best effort (see its [README](https://github.com/weakpixel/weakpixel-engine-vscode#readme)); the engine only sees the changed document.

## The host and the stages

**The engine owns the entry.**
Running the game's own `main` in the editor was brittle: it ran whatever `main` did (flags, window calls, `log.Fatal` on a missing file), every game copied the same bridge, `httpfs` and FS layering code, and the game hard-coded which level the editor owned.
`editor/host` is the entry of an editor build instead.
The game describes itself (`host.Game`: config, systems, prefab rendering) and the host connects the bridge, builds the FS, creates the engine and opens the file.
A second `main` behind the `editor` build tag keeps the game's own `main` out of the wasm; both share the game's config and setup functions, so no package split is needed.
Rejected: generating a main with `go build -overlay` (hides what runs), and a generic player shipped by the engine (it would not know the game's components, systems or Lua bindings).

**Two stages, one wasm.**
A level is edited among the game: its systems run in edit mode if they opt in ([`game.EditRunner`](../guide/engine/engine.md#development-tools)) and its map is its backdrop.
A prefab used to open the same way, so it sat at (0, 0) in the middle of a world, with the hero, the camera rig's bounds and a Play that ran the whole game around it.
The prefab stage starts none of the game's systems and no levels, resets the camera rig, paints a neutral background and draws a grid and the origin over the picture (`editor.Stage`, drawn in `DrawFinal` in window pixels).
Play and Step are no-ops.
Rendering the prefab like the game does (layers, post effects) is the game's `Game.Prefab` hook.

**The page names the stage.**
The extension sets `weakpixelHost.stage` (`level` or `prefab`, by file suffix) and `weakpixelHost.path` (the first file's asset key) beside `assetBase`.
Ebiten runs one game per process, so a change of stage reloads the page; switching between two files of the same stage uses `open`.
These page fields are not protocol messages.

**Listing.**
HTTP cannot list directories, so the page also sets `weakpixelHost.files`: the asset keys of every file under the asset root, read when the page is made.
The host lists directories from that index (plus `Game.FS`'s files) and reads the files themselves over HTTP, so the editor build needs no embedded copy of the assets and the library scans the real folder.
A file created while the editor runs is read at once but joins the library's listing on the next page load.

**Backdrop.**
`Session.OpenAt` calls `Config.OnOpen` before it opens the document; the host uses it to unload the previous backdrop and load the next one's when the target changes: per file, the sibling `<name>.tmj` (`NoDocuments`), else the sibling `<name>.world` or the package's own level `<pkg>/<pkg>` (`LoadLevelBase`, `SkipDocuments` with the edited file), since the session spawns the edited document itself.
A custom `Game.Backdrop` is still called per package.

**The native transport.**
A native host reaches the game the same way, over loopback TCP instead of the page's script (`bridge.DialHost`).
The host listens on `127.0.0.1:0` and starts the editor build with `WEAKPIXEL_BRIDGE` (the address), `WEAKPIXEL_BRIDGE_TOKEN`, `WEAKPIXEL_STAGE`, `WEAKPIXEL_PATH` and optionally `WEAKPIXEL_HEADLESS=1` ([variables](../guide/engine/engine.md#host-variables)); `host.PageFromHost` and `bridge.ConnectHost` read them where the browser build reads `globalThis.weakpixelHost`.
The game sends the token as the first line, then each message is one JSON object per line (a JSON text holds no raw newline).
Stdin and stdout were rejected because games print to stdout, and extra file descriptors because `exec.Cmd.ExtraFiles` does not exist on Windows.
When the host closes the connection the bridge quits the engine at its next tick.
A headless run (`host.RunHeadless`) calls `Engine.Update` on a ticker without `ebiten.RunGame`, with no sound and a manual input source; since nothing is drawn, a `screenshot` is never answered, which the host must know.
This is a transport only: the messages are the same.
The first user is `wpix mcp` ([tools.md](../guide/tools.md#the-game)), whose host side is `tools/internal/mcpserver`.
It cannot import `bridge` (Ebiten), so it repeats the message shapes and a test checks the constants.

**A round trip is a `status`.**
The bridge handles queued messages at the start of a tick and answers them in order, so the `status` answer to a `status` sent after a request says that the game handled the request and that every answer to it arrived.
The MCP server sends one after each request (and twice, to know that a `step` ran).

## Attached objects in the viewport

A part's `Transform`, and that of any object with `hierarchy:Parent`, is derived by the hierarchy stage, so the handles edit its `Local` instead.
A gesture sets the live `Transform` as for any object (so the art follows the cursor), then `syncLocal` converts the field it changes (`Pos` for a move, `Rot` for a rotate, `Scale` for a scale) with `hierarchy.LocalOf(parent's Transform, world)`; the other `Local` fields and `Entity` are untouched.
The finished gesture is one `Edit` with component `hierarchy:Parent` and the whole component (the entity as an id), instead of `gfx:Transform`; for a prefab instance or one of its parts it is the override against the prefab (`Part` set for a part), so the file gets `children.<part>["hierarchy:Parent"]` with only the changed fields.
A click selects the part under the cursor; Alt+click selects its instance root.
When a selected object has a selected ancestor, a move skips it (it follows the ancestor).
A selected attached object gets a dotted line to its parent.
A multi-object `*.prefab.json` opens in the prefab stage like any prefab: its document has several objects, the hierarchy stage runs, and the same handles edit the `Local` in the prefab document.
Esc does not cancel a drag.

## Applying a document

`Session.Apply` decides per object whether its entity is kept (same id, class, prefab and part), created (new id, class or prefab) or deleted (id gone), and per component whether to set it (compact JSON changed), remove it (optional or extra component gone) or leave it alone.
Leaving unchanged components alone keeps runtime state (animation frames, values a paused game system wrote) and avoids archetype moves.
Entities for new objects are created before any component is decoded, so references to them resolve; if any component fails, those entities are deleted and the World is as before.
When objects were created or deleted, components with entity references are decoded again even if their text is unchanged, because the entity behind an id may be new.

**Prefab instances are expanded first.**
`types.Prefab.ExpandInstance` turns an instance entry into plain objects: its overrides (`components`, and `children` for the other objects of a [multi-object prefab](../guide/ecs/game-object-definition.md#multi-object-prefabs)) merged over the prefab's objects, the parts with ids `<id>/<part>`.
Everything after compares merged components, so a saved prefab is just another document version: the session keeps a stamp (`Version`, state) of every prefab it used and re-applies the current document when one changes.
Prefabs load asynchronously and `Apply` must stay synchronous for a shell that answers every document, so a document with a loading prefab is kept as pending and finished in an edit tick; the bridge answers its `document` then.
Drags of instances are reduced to overrides in Go (`types.Prefab.PartOverrides`, against the prefab's object expanded for that instance), where the schema is known, so the extension never diffs; it only merges, to show values.
Without `WatchInterval` the asset server keeps no Decode results, so a dependent of a changed asset (a variant of a saved base) is read from its file again instead of re-finished.

## Modes

Edit mode is `game.TickPause` every tick: the engine's own stages run (assets, snapshot, animation, extract, audio), game systems and the input stage do not.
The editor polls the device source itself because the paused input stage does not; pad connects it consumes are handed to the game's next poll.
Play is `TickRun`, and `TickPause` while a `pause` holds it (no reset; `step` returns `TickStep` once, as in edit mode).
Stop deletes the entities that did not exist when Play started (everything the game spawned), re-applies the document with every component forced and clears every tween, so no tween started during Play keeps animating the restored objects.
Game state outside the level is the game's business (`Config.OnReset`).

**Pacing.**
Ebitengine in a browser runs one frame per `requestAnimationFrame` and calls `Draw` every frame whatever the TPS, and `Engine.Draw` renders the whole queue each time, so a lower TPS alone saves little.
The session therefore picks the tick rate (idle, active, the game's own in Play, hidden) and is a `game.DrawGate` that draws only the first frame after a tick in edit mode, and only when that tick changed the picture (`Engine.FrameChanged` compares the extracted queue with the last drawn one, which is cheaper than drawing it); `editor/host.Run` sets `SetScreenClearedEveryFrame(false)` so a skipped frame keeps its image, and Ebitengine stops presenting after a few unchanged frames.
`FPSModeVsyncOffMinimum` with `ScheduleFrame` is not used: it is deprecated and forces TPS to follow the frame rate.
The rate changes from other goroutines go through `ebiten.SetTPS`, which is safe there; the pacer compares with the actual rate each tick instead of its own last value.
Pacing is on in every editor build; the rates and the states are in the [guide](../guide/engine/editor.md#pacing).
The cost that remains follows the drawn frames, not the ticks: Ebitengine presents for a few frames after each modified one, and under software rendering each present is most of the work.
Measured in the studio under Xvfb (llvmpipe), the test game's animated level in edit mode went from about 250% of one core to about 130%; the same level at 1 idle tick a second used under 30%.

**Switching files.**
One game shows one file at a time.
`Session.OpenAt(path, text)` replaces the level with another file's document: it stops Play as Stop does, clears the selection, takes `path` as the level's path and asset scope, and frames the new level unless the shell restores a camera with `SetCamera` afterwards (a camera restored before `Open` survives it; one restored before `OpenAt` does not).
A document that does not parse changes nothing.
The bridge's `open` message calls it, so a shell switches files without reloading the game.

## Not built yet

- A start zoom for the prefab stage, and a prefab shown in the context of a level.
- Re-scanning the asset library on `assetsChanged`, so that a file created in VSCode joins the library's listing without a page reload.
- Re-parenting an object in the viewport.
- Snapping in the inspector, label and anim-box picking, per-axis scale handles, rotating or scaling several objects at once.
- Editing several selected objects at once in the inspector.
- Running the extension's own tests inside VSCode (`@vscode/test-electron`): only the VSCode-free parts and the webview page are tested automatically.
