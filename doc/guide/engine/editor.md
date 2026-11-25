# Level editor

`editor` is the core of the Weakpixel level editor: a [`game.Tool`](engine.md#development-tools) that edits one GOD level of the running game. The game renders the level with its real systems and assets; the editor picks, selects and drags objects, moves its own camera, and runs Play/Stop. It has no widgets and no undo history: a *shell* drives it. The shell is the [VSCode extension](https://github.com/weakpixel/weakpixel-engine-vscode) (its own repository), which runs the game as WebAssembly in an editor tab and keeps the level file as a text document, so VSCode owns undo, save and diff.

A game that does not attach the editor links none of its code. For the VSCode editor the game has a second `main` behind the `editor` build tag that calls [`host.Run`](#making-a-game-editable-in-vscode). To drive a session by hand instead (your own shell, a test):

```go
s := editor.New(editor.Config{Path: "world-1/level-1.level.json"})
cfg.Tool = s
```

A game made with `wpix new` does this: its `editor.go` is the reference.

## Session

| API | Does |
| --- | --- |
| `editor.New(Config)` | A session in edit mode. `Config.Path` names the level (asset scope, `Level.Path`); `Config.Events` receives [events](#events); `Config.OnReset` runs after Stop; `Config.OnOpen` runs when `OpenAt` switches files; `Config.Stage` makes it a [prefab stage](#stages). |
| `Open(text)` | Loads the level document (`LoadLevelData`). Every object needs a unique `"id"`. Errors are joined; nothing spawns then. A document whose [prefabs](#prefab-instances) are still loading opens empty and is applied once they are ready (`Pending()`, then an `Applied` event). |
| `Apply(text)` | Applies a new version of the document [by id](#applying-a-document). All or nothing; waits for loading prefabs like `Open`. |
| `Name()` | The top-level `"name"` of the last document `Open` accepted (`""` if none); a rejected document leaves it. The extension reads the file itself, so the protocol is unchanged. |
| `Pending()` | Whether a document given to `Open` or `Apply` waits for its prefabs. |
| `Level()`, `IDs()`, `Entity(id)` | The level, its object ids in document order, an object's entity. |
| `Select(ids...)`, `Selected()` | Set or read the selection (ids). |
| `SetColliders(show)`, `Colliders()` | Turn the collider outlines of edit mode on (default) or off; read the state. The extension sends it with the `settings` message. |
| `Pick(worldPos)`, `Bounds(id)` | Front-most object at a world point; an object's world bounds. |
| `Frame()` | Centers the camera on the selection, or on the whole level. |
| `Reveal()` | Pans the camera to the selection only when it is not inside the view shrunk by 10% per side; zoom stays. Nothing in Play or without a selection. |
| `MapLayers()`, `SetMapLayerHidden(map, path, hidden)` | The tile and image layers of the Tiled maps spawned in the world, in draw order (a `LayersChanged` event reports changes); hide one in edit mode without touching the map file. |
| `DefaultLayer(pos)` | The render layer a new object at `pos` starts on: below the map's topmost object layer. `false` without such a map. The bridge applies it to drops and created objects. |
| `Play()`, `Stop()`, `Step()`, `Playing()`, `Pause(on)`, `Paused()` | See [modes](#modes). |
| `Inject(Injection)` | Hold keys, a mouse and a virtual gamepad (`InjectedPad`) for `Ticks` game ticks in Play, on top of the devices; see [modes](#modes). |

Use a session on the game goroutine only. Open it once the engine exists, for example from the `Init` of the system that loads the level, or a Start hook.

## Applying a document

`Apply` compares the new document with the last applied one, object by object:

- an id that is new, or whose `group:class` changed, gets a new entity;
- an id that is gone is deleted;
- a component is set only if its JSON changed (whitespace does not count), and removed if an optional component disappeared;
- when objects were created or deleted, components with entity references are decoded again, so references follow the current entities.

The whole document is checked first (JSON shape, ids, schemas, every changed component through `types.DecodeComponent`). On any problem nothing changes and the errors come back joined, each naming the object, for example `objects[0] (a): Transform: ...`. Asset keys resolve in the level's asset scope.

### Prefab instances

An object can be an [instance of a prefab](game-objects.md#prefabs): `{"id", "prefab": "<key>", "components": {overrides}}`. A prefab file's optional top-level `"name"` is accepted and ignored by the session (the extension reads it to list prefabs, file name as fallback); the protocol is unchanged. The session loads the prefab into the level's asset scope (it stays held until the level is unloaded) and compares the **merged** components, the overrides over the prefab's values, so the rules above apply unchanged. Its entity carries `types.PrefabInstance` like a spawned one (`types.PrefabOf`), and an object that becomes an instance, or switches prefab, gets a new entity.

- **Loading.** While a prefab the document uses is loading, `Open` and `Apply` return nil and the document waits (`Pending()`); it is applied in a later edit tick and the outcome is an `Applied{Err}` event. A newer document replaces one that waits. A prefab that fails is an error of the objects that use it: `objects[0] (a): prefab "global/prefabs/villager.prefab.json": ...`.
- **Saved prefabs.** When a prefab the level uses is reloaded (its file was saved, so the shell reported it with `asset.Server.Changed`), the session applies the current document again in the next edit tick, so every instance that does not override the changed field follows it; the outcome is an `Applied` event. A variant follows a saved base too (its file is read again).
- **Drags** of an instance report its override, not the whole component: `Edit.Override` is set and `Edit.JSON` holds only the fields that differ from the prefab (`{}` when none do, and the shell drops the entry).

## Modes

In **edit mode** (after `Open`) the engine runs paused (`TickPause`): assets load and reload, animations play and the frame draws, but game systems and the input stage do not run, except systems that opt in with [`game.EditRunner`](engine.md#development-tools) (setup work the edited scene needs, such as placing the hero). The editor reads the mouse itself:

| Input | Does |
| --- | --- |
| Left press on an object | Selects it (replacing the selection) and starts a drag. A part of a prefab instance is selected by itself; hold Alt to select its whole instance |
| Shift, Ctrl or Meta + left press on an object | Adds it to the selection, or removes it if selected, and drags the selection |
| Left press on a selected object of several | Keeps the selection and drags all of it; released without a drag, the selection narrows to that object |
| Left drag | Moves the dragged objects' `Transform.Pos` (the `hierarchy:Parent` `Local.Pos` of an attached object), the grabbed one rounded to the snap grid unless Alt is held; the others keep their offset to it. An object whose parent is dragged too is not moved by itself. An attached object with `PlaceAbsolute` moves with a dragged ancestor too (it is placed from its `Local` again on each step) and gets no edit, since its `Local` does not change |
| Left press on a gizmo of the selected object | Starts a move along one axis, a rotation or a scale |
| Left release | Ends the drag: one `Edit` event per changed object with its whole `Transform` (a prefab instance's `Transform` override), or, for an attached object, its whole `hierarchy:Parent` (an override for a part or an instance) |
| Left press on empty space, then drag | Draws a selection box; on release it selects the objects whose bounds it touches (added to the selection with Shift, Ctrl or Meta) |
| Left click on empty space | Clears the selection (a box under 3 window pixels; with a modifier it changes nothing) |
| Middle drag, or Space with left drag | Pans the editor camera (the right button does nothing). A left press with Space held pans instead of picking; a drag that began earlier is not interrupted |
| Wheel | Zooms around the cursor: 10% per notch, eased over a few ticks |

Picking measures sprites by the visible art of their loaded frame (the bounds of the pixels with alpha above 0, found once per frame and cached; pivot, trim, scale; rotation and flips ignored). While the pixels cannot be read (the sprite is loading), the whole frame counts, `DebugShape`s by their outline, anything else as an 8×8 handle at its position. Higher render layers win, then lower on screen, then later in the document. When the click hits no object, an object smaller on screen than 12 device-independent pixels is picked within a 12 px square around its center (the nearest center wins), so tiny or far zoomed-out objects stay clickable. The selection is outlined in window pixels, never smaller than that square.

**Move handle.** With exactly one object selected (and a `Transform`), two arrows start at its position: X red to the right, Y green down, 40 window pixels long however the camera is zoomed. A press within 6 pixels of an arrow (handles are tested before objects) drags along that axis; the other axis stays at its start value. **Snapping:** `SetSnap(grid)` (world units, 0 = off) rounds the dragged `Pos` to multiples of `grid`; holding Alt during the drag turns it off. The position is always the start position plus the cursor's movement since the press, rounded, so a drag that returns to its start changes nothing.

**Rotate and scale.** With one object selected, a ring of radius 56 window pixels carries a blue knob at the object's `Transform.Rot`, and a white scale box stands on the diagonal at (26, 26) from the object. Dragging the knob turns the object so it keeps following the cursor's angle around the object; dragging the box scales both axes by the ratio of the cursor's distance to the distance at the press. **Shift** rounds the rotation to 15 degrees and the scale to steps of 0.25. A zero `Scale` is read as (1, 1) and written out. The gizmos are tested knob, box, then arrows, all before objects; none shows for several objects. For an object attached with `hierarchy:Parent` the gizmos stand at its world position and edit its `Local` (move: `Pos`, rotate: `Rot`, scale: `Scale`) relative to its parent, so a part of a multi-object prefab can be placed in the viewport; a dotted line joins a selected attached object to its parent.

**Markers and collider outlines.** In edit mode an object with a `Transform` and no art (no sprite, panel, label, particle emitter or debug shape) is drawn as a 12 px box with a cross, so a sound emitter, a spawn point or a sensor can be seen and picked: cyan with a `SoundEmitter`, blue with a `Collider`, grey else. An object whose art is smaller on screen than 12 px gets a white 12 px box around it, the area that picks it. Sizes are device-independent pixels, so they look the same on a HiDPI display. Every `collision.Collider` is outlined in the color of its lowest layer, as the debug overlay does in play (placed with `collision.ShapeOf`, since the collision `Space` is empty while paused; custom shape kinds are not drawn). `SetColliders(false)` turns the outlines off.

**Camera.** The editor camera is reported as a `Camera{X, Y, Zoom}` event once it has stayed put for 15 ticks after a pan, zoom or frame. `SetCamera(pos, zoom)` restores it, for a shell that rebuilt the game; a level opened afterwards is not framed over it.

`Step()` runs the game for one tick, in edit mode or in a paused Play. **`Pause(true)`** holds Play still without resetting anything (the game keeps drawing, its systems and input stage do not run) and `Pause(false)` resumes it; both emit `Mode{Play, Paused}`, and edit mode ignores them. **`Inject`** adds input for agents and tests: the keys, mouse and pad of an `Injection` are merged into what the game polls for its next `Ticks` ticks (a paused Play polls nothing, so they wait for steps); a new injection replaces the rest of the last, edit mode ignores it and `Stop` drops it. The pad is announced to the router as connected the first time and stays connected. **`Play()`** restores the game's camera and runs the game with input. **`Stop()`** deletes the entities created during Play, restores the level's objects from the document (every component, and objects the game deleted), keeps the selection, clears every tween (`Engine.Tweens().Clear()`), calls `Config.OnReset` and returns to the editor camera. Entities the game owned before Play (for example a hero spawned by a system) keep their Play state unless `OnReset` resets them.

## Pacing

A session paces the game so an editor that sits still costs little CPU. It sets the tick rate (`ebiten.SetTPS`) by what is happening, and draws a frame only after a tick that changed the picture (it is a [`game.DrawGate`](engine.md#development-tools)):

| State | Tick rate (`editor.DefaultRates`) | Frames drawn |
| --- | --- | --- |
| Edit mode, a paused Play or a stage, nothing happening | `Idle`, 10 | One after each tick that changed the picture |
| The same while the user works in the view, and for `editor.ActiveFor` (500 ms) after | `Active`, 60 | One after each tick |
| An unpaused Play | The game's own rate (`ebiten.TPS()` at the first tick, so `game.Config.TPS` holds) | Every frame |
| Hidden (`SetVisible(false)`), in any mode | `Hidden`, 0: no ticks | None |

Work in the view is a mouse move, a button or key held, the wheel, a drag, or a camera that moved in an edit tick. `Session.Wake()` (safe from any goroutine) raises the rate at once, so the next tick comes within a frame, and that tick sets the rate of its state again: a wake is not work, so a host that polls the game does not keep it at the active rate. The bridge calls it for every message it receives, and in a browser `editor/host` calls it on every pointer, wheel and key event of the page, so the first click after an idle stretch is not delayed. A frame between two ticks would show the same picture, since `Draw` never reads the World and the editor camera does not blend between ticks; only the shader `Time` and animations step at the tick rate, so at the idle rate they play at 10 frames a second. A tick that changed the picture is one with work in it, one after `Session.Invalidate()` (the bridge calls it for every message but `status`, `inspect` and `screenshot`, since the session draws the selection and outlines over the frame), or one that [`Engine.FrameChanged`](engine.md#development-tools) reports: a level that stands still draws nothing until something moves.

`Session.SetRates(r)` sets the rates (`Rates.Check`: `Idle` and `Hidden` 0 or more, `Active` 1 or more; idle equal to active gives a fixed rate), `SetVisible` hides or shows the view (a session starts visible; showing it again counts as work), `TPS()` is the rate of the current state and `Draws()` counts the drawn frames. `Config.FixedRate` turns pacing off: the rate is never changed and every frame is drawn. A headless run (`editor/host.RunHeadless`) sets it, since it ticks on its own clock.

A level with anything that moves on its own (a sprite clip, an animated tile, particles, an item with a shader, a post pass) changes on every tick, so it draws at the idle rate forever. `Session.SetAnimate(false)` stops that: outside Play (edit mode and a stage) the session calls [`Engine.SetAnimationsFrozen(true)`](engine.md#development-tools), so clips and animated tiles keep their frame, particles stand still and emit nothing, and the shader `Time` stays, and an idle view draws no frames. Play always animates. A session starts with `Animate()` true; the bridge sets it from `settings.animate`.

## Bridge

`editor/bridge` puts the [JSON protocol](../../design/editor-protocol.md#protocol) in front of a session for a host such as the VSCode extension. `bridge.New(cfg, transport)` returns a `*Bridge` (it embeds the `*Session`; set it as `Config.Tool`); `Receive(msg)` queues a message from any goroutine. In a browser, `bridge.ConnectHost(cfg)` connects to the page's host script and returns false when there is none, so a game falls back to a plain session.

## Making a game editable in VSCode

The [VSCode extension](https://github.com/weakpixel/weakpixel-engine-vscode) builds the game's main package with `GOOS=js GOARCH=wasm` and `-tags editor`, and runs it in the editor tab. The game does not wire the editor in itself: `editor/host` is the entry of the editor build. The game describes itself with a `host.Game` and a `main` that exists only under the `editor` tag; its normal `main` (`//go:build !editor`) is not compiled in.

```go
//go:build editor

package main

func main() {
	err := host.Run(host.Game{
		FS:     diskAssets,              // the game's own assets (a folder on disk, in-memory extras); no embed
		Level:  "world-1/level-1.level.json", // opened when there is no VSCode
		Config: func(fsys fs.FS) (game.Config, error) { /* library.Open(fsys), resolution, ... */ },
		Setup:  func(e *game.Engine) { /* AddRunner ... */ },
	})
	if err != nil {
		log.Fatal(err)
	}
}
```

| `host.Game` field | Does |
| --- | --- |
| `FS` | The game's own assets: a folder on disk (`os.DirFS`) or in-memory extras. An editor build embeds nothing (embedding stays a feature of the game's normal build). In VSCode the workspace's files are read first (over HTTP, so a saved file reloads in place) and these fill in what the workspace lacks; without VSCode they are the assets. Directories are listed from the page's file index (`weakpixelHost.files`, see the design) plus these files, because HTTP cannot list them. |
| `Config` | The engine config over the FS the host built. The host sets `Tool`. |
| `Setup` | Adds the game's systems to the engine. Level stage only. |
| `Prefab` | Sets up rendering for the prefab stage (render layers, post effects). Optional. |
| `Backdrop`, `NoBackdrop` | What surrounds the edited level, see [stages](#stages). |
| `Level` | The level key opened when there is no VSCode (`go run -tags editor`), and the default first file. |

`host.Run` connects the bridge (a plain `editor.Session` without VSCode), builds the FS, creates the engine and opens the file. In a level the extension sends the document after the game's `ready`; without VSCode the host reads `Game.Level` from the FS. `host.Build` does the same without running the window, for tests. The `editor.go` that `wpix new` writes is the reference.

### Stages

The extension tells the page which stage to run (`globalThis.weakpixelHost.stage`) and which file to open first (`path`), and reloads the page when the stage changes, because Ebiten runs one game per process. Switching between two levels, or two prefabs, stays in the page.

| Stage | Files | Runs |
| --- | --- | --- |
| `level` | `*.level.json` | `Game.Setup` and the level's backdrop. The game starts paused; Play runs the game's systems. |
| `prefab` | `*.prefab.json` | The engine's own stages only: no `Setup`, no levels, no camera rig, no input to the game. The prefab sits on a dark background with a grid of 16 world units and red and green axes at the origin. Play and Step do nothing. `Game.Prefab` can set up rendering. Animations and particles preview. |

**Backdrop.** A level is edited among its map or world. The host picks the backdrop per file, never spawning the edited document (the session does): for `<name>.level.json` the sibling `<name>.tmj` (loaded alone), else the sibling `<name>.world` (its children's documents still spawn), else, with a library, the package's own level `<pkg>/<pkg>` (loaded with `LoadLevelBase` when `<pkg>/<pkg>.tmj` or `.world` exists, with the other documents). It reloads when the target changes, so a level never shows the previous level's world. A custom `Game.Backdrop` is called for the first file and when the editor opens a file of another package, and it should skip the edited document with `SpawnOptions.SkipDocuments`. A game whose systems load their own world sets `NoBackdrop` (for example a level system that loads and places its own world). A file at the root of the library, or in a package without a map of its own and no sibling map, has no backdrop.

## wpix-editor

`wpix-editor` opens one file in a window, without VSCode, and shows its tool's panel in two columns over the viewport. The file name picks the kind, and every kind has one tool: a kind that can be edited opens its editor, any other kind opens its preview. There are no modes and no tool names on the command line. The design is in [tool-shell.md](../../design/tool-shell.md).

```sh
go tool wpix-editor assets/player/idle.png
go tool wpix-editor assets/player/player.atlas.json
go tool wpix-editor assets/global/fonts/ui.ttf
go tool wpix-editor assets/player/player.anim.json
go tool wpix-editor --sheet assets/player/player.aseprite.json assets/player/new.anim.json
go tool wpix-editor assets/gfx/tiles.sheet.json
go tool wpix-editor --image assets/gfx/tiles.png assets/gfx/new.sheet.json
go tool wpix-editor assets/global/fx/sparks.fx.json
go tool wpix-editor assets/talk/kip.voice.json
go tool wpix-editor help
```

`--sheet <export>` is only for a new `.anim.json` and `--image <texture>` only for a new `.sheet.json`; on any other file they are a usage error. `go tool wpix-editor --help` lists the kinds. Usage errors (no file, two files, a file of no kind, a flag for the wrong kind) exit with 2, other failures with 1.

A project made by `wpix new` has the tool. For an existing project, add it once:

```sh
go get -tool github.com/weakpixel/engine/tools/wpix-editor
```

| File | Opens | Shows |
| --- | --- | --- |
| `*.sheet.json` | [Editor](#the-sheet-tool) | The [grid sheet](asset-kinds.md#grid-sheet) over its texture: edit the frame size, margin and spacing, see the frame count and the index of the frame under the mouse |
| `*.anim.json` | [Editor](#the-anim-tool) | The [animation definition](animation.md#animation-definition-animjson) on its sheet: play a clip, draw and edit the pivot, body and hit and hurt boxes, and edit the rest of the format in the panel |
| `*.fx.json` | [Editor](#the-fx-tool) | A [particle effect](particles.md#authoring) with presets, image and shader pickers and the fields that matter |
| `.png`, `.jpg`, `.jpeg` | [Preview](#previews) | The texture on a grid, with its size, the pixel under the mouse and the grid cell under the mouse |
| `*.atlas.json` | [Preview](#previews) | The atlas's frames over its texture, with their indices and names |
| `.ttf`, `.otf` | [Preview](#previews) | A font as the engine draws it, at the sizes the project uses |
| `.kage` | Preview | A [shader](shaders.md) drawn on a texture, with a control per `+shader:param` marker |
| `.tmj`, `.tsj`, `.world` | [Preview](#previews) | A Tiled [map](tilemaps.md), tileset or world |
| `*.aseprite.json`, `*.ase.json` | Preview | An Aseprite export's clips with the sheet's own timing, in the [anim tool's](#the-anim-tool) layout, read-only (the format belongs to Aseprite) |
| `*.voice.json`, `*.dialogue.json` | Preview, with sound | A [voice](speech.md#previewing-voices-and-dialogues) says a line you type in the panel, with the piano roll of its notes; a dialogue plays in a box line by line. Edit them in the Voice Editor |

The suffixes of the engine's own formats (`.sheet.json`, `.anim.json`, `.fx.json`, and the Aseprite exports) match in lower case only; the others match in any case. The content of a file is never looked at.

**New files.** A file of an edited kind that does not exist yet opens as an unsaved document, and Ctrl/Cmd+S creates it: a `.fx.json` starts from the default effect (its built-in sprite is copied next to it as `<name>.png` on the first save), a `.sheet.json` needs `--image <texture>` and a `.anim.json` needs `--sheet <export>`. Without the flag a missing sheet or definition is an error that names the flag, for example `new.sheet.json does not exist: give --image <texture> to start a new sheet`. A missing file of any other kind is an error.

**Sound.** Only a window that opens a voice or a dialogue starts the sound.

The asset root is `assets.root` of the [`.weakpixel.json`](../project-file.md) found from the file's folder, or the file's folder without one; a file outside the root is an error. The file's asset key is its path relative to the root. A PNG saved while the tool is open reloads.

| Input | Does |
| --- | --- |
| Middle mouse drag, or Space with left drag | Pan (the same in the level editor) |
| Mouse wheel | Zoom around the cursor (1/16 to 64): 10% per notch, eased over a few ticks |
| `F` | Frame the content (the panel's Fit) |
| `1` | Zoom 1 |
| `Tab` | Hide or show both panels |
| Ctrl or Cmd with `Z`, `Shift+Z` or `Y`, `S` | Undo, redo and save, for tools that edit a file |

The shortcuts of all editors, including the VSCode level editor keys (F5, F10, F, Delete), are listed on one page in the extension repository: [`doc/shortcuts.md`](https://github.com/weakpixel/weakpixel-engine-vscode/blob/main/doc/shortcuts.md). Play and step are F5/F10 in the level editor and `Enter`/`.` in the tools on purpose.

The panels lie over the viewport, which fills the window: the left one holds the View section and the lists you pick content from, the right one the properties of what is selected (and the status line). The left panel is left out in a window narrower than 800 pixels; its sections then join the right one. Fit frames the content in the free area between the panels; with the panels hidden (`Tab`) it uses the whole window. The View section holds the grid (on or off, cell size, offset), the background, the zoom, and the Fit and 1:1 buttons. Zoomed out so far that cells would be under 4 pixels, the grid shows every eighth line (every 64th, and so on), so a large map or world still has one. Mouse and keys go to a panel while the cursor is over it or a text field has focus. In VSCode the panel stays one column in the Weakpixel Tool view.

### Previews

The preview (the `view` tool) shows every kind that is not edited, with the engine's own loaders, so it needs no game code and works while the game does not compile.

- **Atlas.** A `*.atlas.json` (a `*.sheet.json` opens in [its editor](#the-sheet-tool)): the texture with every frame outlined, labelled with its index and name (the Labels dropdown on the left picks index, name, both or none; small frames are labelled only when hovered). The left Frames list picks a frame like a click on it does (`(none)` clears the selection). The panel describes the frame under the mouse: its rect in the texture, untrimmed size, trim offset and pivot. A click selects a frame and keeps it described; a click on nothing clears it. Where frames overlap, the smaller one is picked.
- **World.** The maps of a `.world` file with their bounds. The left Maps list selects a map like a click does (it does not open a level; a double click does).
- **Map.** The left panel holds the layer toggles, the Names switch and the levels that use the map; the Object section on the right describes the selected object, else the one under the mouse.
- **Font.** Sample text at the sizes the project uses: the view scans the `*.level.json` and `*.prefab.json` files of the asset root for `fonts/ui.ttf#16` style keys of the opened font (at most 300 files), or shows 12, 16, 24 and 32 px when there are none. The panel changes the sample text and adds one more size to compare, and lists each size's line height. The text is drawn with the face a `gfx.Label` uses, in black or white to suit the background.

### The fx tool

`go tool wpix-editor <name>.fx.json` with a file that does not exist yet opens the default smoke effect, already playing; Ctrl/Cmd+S writes it and the built-in sprite as `<name>.png` next to it, so the saved effect never names a built-in image. In VSCode, New Particle Effect opens the same effect untitled, and **Save as** writes `global/fx/<name>.fx.json` (and the sprite) and continues on the new file; Ctrl/Cmd+Z undoes edits before and after saving. With an existing `*.fx.json` file it edits that effect and writes the whole file in the canonical form ([effect file](particles.md#the-effect-file)). The effect is placed in a level with a `gfx:Particles` component; layer and seed belong to that component and are not edited here.

| Input | Does |
| --- | --- |
| Click in the viewport | Burst: the cycle starts again |
| Drag the cross, double click it | Move the emitter (to try `Local`); put it back at the origin. The position is never written |
| `Enter`, `.`, `R` | Play and pause, step one tick, restart (also the toolbar; icons from [Tabler Icons](https://github.com/tabler/tabler-icons), MIT, `editor/toolshell/icons/LICENSE`) |

The left panel has Emitters (the list of emitters, Add, Duplicate, Delete, Move up and down, Mute, Solo, Reset), Preview (play, step, speed 0.1× to 2×, scrub, overlays, checker and image backgrounds) and View. The right panel has Effect (preset and Apply preset, Keep image and shader, the live particle count, Restart), Emitter (the selected emitter's name and layer), Look (image, shader and its parameters, blend, layer, color, change over life, scale, fades), Emission, Shape, Motion (angles in degrees, the direction a -180..180 slider, min/max pairs that never cross, drag never below 0), Advanced (seed, Local), Save as and Problems. A field appears only when it acts: Loop needs a Duration, the radius a circle (radius, width and height are 0..256 pixel sliders). A typed number is kept inside the field's bounds. Choosing a preset only picks it; Apply preset replaces the whole effect in one undoable edit: every other emitter is removed. A single-emitter preset keeps the shader and a project image you picked on the selected emitter; a preset marked "(N emitters)" brings its own. The Emitters section's Reset removes all emitters and starts over with one default emitter. The shader picker also offers built-in shaders: Burn (`fx-builtin/burn.kage`) burns particles away along a glowing edge instead of fading them; the tint's alpha is the part not burned yet, so let it run from opaque to transparent over the life (the Burning scraps preset shows it). Save as copies a built-in shader next to the effect like a built-in image. The shader's parameters are labelled sliders, toggles and colors when its source has [`+shader:param` markers](shaders.md#parameter-markers) and four plain numbers otherwise.

### The anim tool

`go tool wpix-editor <file>.anim.json` plays the clips of the definition's sheet. An Aseprite export named `<name>.aseprite.json` or `<name>.ase.json` opens in the preview in the same layout, **read-only**: the clips play with the sheet's own timing, and the panel offers no editing; make a definition of it with `--sheet` (below) to add pivot and boxes. Boxes are drawn relative to the pivot, which is the origin of the view; coordinates are whole pixels. To start a definition from an Aseprite export, give a file that does not exist and the export: `wpix-editor --sheet player.aseprite.json player.anim.json` (the same starter as `wpix anim create`; Ctrl/Cmd+S creates the file).

| Key | Does |
| --- | --- |
| `S`, `P`, `H`, `U`, `B` | Select, pivot, hit box, hurt box, body tool (also the toolbar at the top center of the viewport and the panel's Tool list; icons from [Tabler Icons](https://github.com/tabler/tabler-icons), MIT, `editor/toolshell/icons/LICENSE`) |
| Click, drag (select) | Pick the topmost box of the frame, move it; the eight handles resize it |
| Click-drag (hit, hurt, body) | Draw a box. A hit or hurt box drawn on frame n covers frame n only |
| Click or drag (pivot) | Place the pivot of the clip (its own, if it has one) or of the file |
| `Delete`, `Backspace` | Delete the selected box |
| `Alt`+arrow keys (`Shift`: 8) | Nudge the selected box by 1 pixel |
| `Enter`, `,` `.` | Play and pause, previous and next frame |
| `[` `]`, Up and Down | Previous and next clip |
| Left and Right | Previous and next frame (as `,` `.`) |
| `X` | Flip H preview |

The left panel has Playback (the clip list, play, frame, flip, tool), the Boxes list and View; the right panel has File (default clip, pivot, body), Clip (loop, fps, speed, own pivot, the duration of the shown frame), Box (the selected box's kind, id, frame range and rect, Copy to this frame, Extend to this frame, Delete; shown only with a box selected) and Problems: the findings of `wpix anim validate`. A box needs a pivot, so the first box tool use without one asks for it. A document that does not parse (an edit in a text editor, say) stays on screen dimmed and is never overwritten.

### The sheet tool

`go tool wpix-editor <file>.sheet.json` draws the grid of a [grid sheet](asset-kinds.md#grid-sheet) over its texture: every frame is outlined and labelled with its index, the pixels no frame covers (right of and below the grid) are dimmed, and the frame under the mouse is highlighted. A click selects a frame and keeps it described; a click on nothing clears it. The left panel has Frames (the Labels dropdown, where every mode but None shows the index since grid frames have no names, and the frame list, which selects a frame like a click does; `(none)` clears the selection), Grid (image size, columns × rows, frame count, the unused pixels, and the index and rectangle of the selected frame, else the one under the mouse) and View; the right panel has Sheet (the image and number fields for frame width, frame height, margin and spacing) and Problems (a missing image, or a frame that does not fit the image). A grid of more than 4096 frames is not listed, and its frames are not labelled. Selecting and the label mode do not change the file. Each change of a Sheet field writes the whole file in the canonical form and is one undo step; a selected frame that the change removes is no longer selected. To start a sheet from a texture, give a file that does not exist and the image: `wpix-editor --image tiles.png tiles.sheet.json` starts with 16 pixel cells (smaller if the image is smaller); Ctrl/Cmd+S creates the file. The pivot of an existing sheet is kept but not edited here.

## Events

`Config.Events` gets, synchronously:

| Event | When |
| --- | --- |
| `Edit{ID, Component, JSON, Override}` | A drag ended. `JSON` is the whole component as the document holds it (asset keys, entity ids); the session already counts it as applied, so sending the edited document back changes nothing. For a prefab instance `Override` is set and `JSON` is the component's override (`{}`: none). |
| `Applied{Err}` | A document was applied (or rejected) later than `Open`/`Apply` returned: after the prefabs it waited for loaded, or after a prefab it uses changed. |
| `Selection{IDs}` | The selection changed by mouse or by `Apply` (deleted objects drop out). Not sent for `Select`. |
| `Mode{Play, Paused}` | Play or Stop, or Play paused or resumed. |
| `Camera{X, Y, Zoom}` | The editor camera settled after moving (see Modes). |
