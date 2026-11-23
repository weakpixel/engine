# game.Engine

`game` is the high-level entry point. An `Engine` implements `ebiten.Game` and does the wiring that [getting-started.md](getting-started.md) shows by hand: World, asset server with the `render`, `anim`, `tilemap`, `sound` and `input` loaders, GOD resolver, renderer, camera, extract schedule, audio, input, `Layout`, alpha interpolation and shutdown.

```go
e := game.New(game.Config{Width: 320, Height: 180, FS: os.DirFS("assets")})
e.Layers().Set(1, render.LayerOptions{YSort: true})
e.AddSystem(game.StageSimulate, move)
e.Start(func(e *game.Engine) { e.LoadLevel("one/one.level.json") })

// The Engine is an ebiten.Game; the window is yours to configure.
ebiten.SetWindowSize(640, 360)
ebiten.SetWindowTitle("demo")
ebiten.SetWindowResizingMode(ebiten.WindowResizingModeEnabled)
err := ebiten.RunGame(e)
e.Close() // stops sounds and asset workers; idempotent
if err != nil {
	log.Fatal(err)
}
```

There is no `Engine.Run`: the engine does not pick the window size, title or resizing mode, so you keep control of the main function (extra Ebiten options, `ebiten.RunGameWithOptions`, your own wrapper `Game`). `Config.Width`/`Height` are the logical resolution only; the window may be any size and `Layout` follows it. Call `Close` after `RunGame` returns, also when it returns an error. `RunGame` returns nil when the game ended through `Engine.Quit` or closing the window.

## Config

Plain struct, every zero value is a default: `Width`/`Height` (640x360), `IntegerScale`, `TPS` (Ebiten's), `FS`, `Library` (none; see below), `Workers` (GOMAXPROCS-1), `CommitBudget` (2ms), `KeepAlive` (0: released assets unload at once; [keep-alive](assets.md#keep-alive)), `WatchInterval` (off; [asset hot reload](assets.md#hot-reload) for development builds), `OnAssetError` (log), `TransparentPlaceholders`, `Background` (black), `Camera` (Zoom 1), `CameraRig` (the follow camera; zero is a hard follow of the `camera.Target` entity, see [camera.md](camera.md)), `World` (new), `Debug`, `StatsOverlay`, `SampleRate` (48000, or the rate of an existing Ebiten audio context), `NoAudio` (play through `sound.NullBackend`, never open the device), `FrameLog` (off), `Screenshots` (captures go to `./screenshots`; see [Screenshots](#screenshots)), `Startup` (none; see [Startup screen](#startup-screen)), `Input` (see below), `Tool` (none; see [Development tools](#development-tools)).


`Config.Library` uses an [asset library](asset-library.md) instead of `FS`: the engine reads `Library.FS()`, loads `global/` at boot (`e.Boot()`), `e.LoadLevelBase(base, opts)` loads a level by its base key (`"forest/cave"`: its GOD document and map) into one `Level` and mounts the packages it needs, and `e.LoadPackage(sc, name)` preloads a package.

`Config.Input` (`game.InputConfig`) sets up the [input router](input.md): `Actions` (frozen by `New`; nil is an empty table), `Bindings` (asset key of the default bindings file, loaded on the Global scope and applied when ready), `Players` (1), `Assign` (`input.AssignSingle`), `JoinAction`, `Source` (nil reads Ebiten; set an `input.ManualSource` for tests), `OnError` (log), `HideTouchControls`.

## Tick order

The engine builds the schedule on the first `Update` and fixes the order, so extract can never run before your systems or after the sort:

```
assets, snapshot | input, simulate, camera, post | hierarchy, anim, render-prepare, extract, sort | audio
```

- The `hierarchy` stage writes the world `Transform` and the draw layers of entities with a `hierarchy.Parent` and deletes the children of dead parents; the `assets` stage runs it once more right after a document spawned objects. Like `anim` it is not gated by a Tool pause or `Hold`. See [hierarchy.md](hierarchy.md).
- The `assets` stage also spawns the layers of maps from `LoadMap` that became Ready this tick, before the snapshot. The extract stage queues tiles (`tilemap.TilemapExtract`) before sprites, and the particles (`particles.Extract`) right after them. The `anim` stage runs the `AnimationSystem` and then the `particles.System`; like animations, particles keep playing while a Tool pauses the game.
- The `input` stage starts with the engine's `inputsys.InputSystem`: it applies a newly loaded bindings file and runs `Router.Update`, so your input systems read this tick's actions. The extract stage draws the touch regions (`inputsys.TouchControlsExtract`) after labels. See [input.md](input.md).
- `AddSystem(stage, fns...)` and `AddRunner(stage, runners...)` fill the four slots (`StageInput`, `StageSimulate`, `StageCamera`, `StagePost`). Runners get `Init`; plain functions do not. Lua systems ([`github.com/weakpixel/contrib/lua`](https://github.com/weakpixel/contrib/tree/main/lua)) are runners. They are the systems of the root [scene](#scenes).
- Adding after the first `Update` panics (`game: AddSystem after the first Update`).
- The engine snapshots `Transform.PrevPos` and `Camera.PrevPos` before your systems run. Camera systems just assign `Camera.Pos`.
- The `camera` stage starts with the engine's `tween.System`, which advances the tweens of `Engine.Tweens()` (the `*tween.Tweens` resource): tweens your systems start run in the same tick, and stop while a Tool pauses the game. See [tweens.md](tweens.md).
- Next in the `camera` stage, the engine's `camera.System` moves the camera after the `camera.Target` entity as `CameraRig()` says, and does nothing when there is no target. It sees the tweened position. Your camera systems run after it and can add a shake or override the position for a tick. See [camera.md](camera.md).
- `Hold()` holds `StageSimulate` until the returned release is called (holds nest; releasing twice is a no-op), and `Held()` reports it. The input, camera and post stages and the engine stages keep running, so tweens and the follow camera move during, say, a screen transition ([rooms.md](rooms.md#what-happens-in-a-transition)).
- Systems pass events to each other through typed queues (`system.EventsOf`). The schedule ages them once per engine tick; systems in a held or paused stage miss events older than one tick. See [events](https://github.com/weakpixel/ecs/blob/main/doc/api-systems.md#events).
- `Start(fn)` hooks run once at the first `Update`, after the schedule is built and before tick 0.
- While the [startup screen](#startup-screen) is up, the `input`, `simulate`, `camera` and `post` stages run only the engine's input system; the engine's other stages run as usual.
- `Quit()` ends the game: `Update` returns `ebiten.Termination`.
- `SetDebug(true)` draws `gfx.DebugShape`.
- `SetStatsOverlay(true)` (or `Config.StatsOverlay`) draws the renderer stats, FPS and TPS in the top-left corner after each frame ([rendering.md](rendering.md#stats-and-the-overlay)).
- The `audio` stage moves the `sound.Listener` to the camera and runs the `sound.AudioSystem`, so every sound requested during the tick starts in it. See [audio.md](audio.md).
- `AddAudioRunner(runners...)` adds runners to the `audio` stage, before the `AudioSystem`. They run every tick, whatever the scene stack, a Tool's pause or `Hold` do, so an audio source can tell that the game froze and silence what it holds (package `speech` releases its notes when its own `StagePost` system did not run). Use it for that, never for game logic. It panics like `AddRunner`.
- `AddExtract(runners...)` adds runners to the extract stage, after the engine's own extract systems (touch controls included) and before the debug shapes and the sort. They push their own `render.Item`s onto the `render.Queue` resource (shapes or text through a shader, custom sprites), and the engine sorts them with the rest. Like the other extract systems they run every tick, whatever the scene stack, a Tool's pause or `Hold` do, so their items also show in the editor. Use it for drawing only, never for game logic. It panics like `AddRunner`. See [rendering.md](rendering.md#writing-your-own).

## Startup screen

With `Config.Startup` set, the game shows a startup screen from its first frame until the initial assets have loaded, and for at least 5 seconds even when loading is faster. nil, the default, shows none. A project made with `wpix new` turns it on in its `main` (not in its tests or editor build).

```go
// The default: the Weakpixel engine image on a dark background.
cfg.Startup = &game.StartupConfig{}

// Replace the default, or add screens before or after it.
cfg.Startup = &game.StartupConfig{
	MinDuration: 3 * time.Second,
	Screens: []game.StartupScreen{
		{Path: "boot/splash/studio.png", Background: color.White},
		game.WeakpixelScreen(),
		{Draw: loadingBar, Duration: time.Second},
	},
}

func loadingBar(dst *ebiten.Image, s game.StartupState) {
	w := dst.Bounds().Dx()
	if s.Total > 0 {
		w = w * s.Ready / s.Total
	}
	vector.FillRect(dst, 0, float32(dst.Bounds().Dy()-8), float32(w), 8, color.White, false)
}
```

| `StartupConfig` field | Meaning |
| --- | --- |
| `NoSkip` | Input does not skip a screen. |
| `Screens` | The screens, shown in order. Empty means `[]StartupScreen{WeakpixelScreen()}`. |
| `MinDuration` | Minimum time of every screen without its own `Duration`. 0 means 5 s. |
| `Ready` | `func(*Engine) bool`: an extra condition for the end, checked after the initial load is done. |

| `StartupScreen` field | Meaning |
| --- | --- |
| `Image` | The picture, scaled to fit the window (aspect kept, centred, linear filtering). |
| `Path` | A PNG or JPEG that `New` reads from `Config.FS` (or the library, `boot/` and `global/` included) and shows like `Image`. A file that fails is reported through `OnAssetError` and the screen is left out. |
| `Draw` | `func(dst *ebiten.Image, s StartupState)`: draws the screen itself into a window-sized image, cleared to `Background`. `StartupState` has `Screen`, `Screens`, `Elapsed`, `Duration`, `Ready`, `Total` (initial-load progress) and `Alpha`. |
| `Duration` | Minimum time on screen, fades included. 0 means `MinDuration`. |
| `Background` | Fills the window around the image. nil means black. |
| `Fade` | Length of the fade in and of the fade out, from and to black. 0 means 250 ms, negative means none. |

Exactly one of `Image`, `Path` and `Draw` must be set; `New` panics with `game: startup screen <i>: set one of Image, Path or Draw` otherwise.

**Timing.** Each screen fades in, holds and fades out, and stays up for at least its `Duration`. The minimum is per screen, so two 5 s screens take at least 10 s. Earlier screens advance on time alone; the last one stays up until the initial load is done (and `Ready`, if set, returns true), then fades out. Time is simulation time (the sum of tick lengths), so the screen does not run down in a hidden browser tab.

**The initial load** is done when the boot load of `global/` (`e.Boot()`), the global scope and every level loaded so far are done and those levels have spawned. A failed asset or level counts as done (its error went to `OnAssetError`), so the screen never waits for something broken. Start the loads in a `Start` hook or a system's `Init`: game systems do not run while the screen is up, so a load started in a system's first `Run` comes after the screen. Use `Ready` for anything else.

**Skipping.** Unless `NoSkip` is set, a key, mouse button, touch or pad button ends the current screen early. A key held since launch must be released first. Skipping never skips loading: on the last screen, a press before the initial load is done ends the screen as soon as loading finishes.

**While it is up**, `e.StartupActive()` is true. The game's systems in `StageInput` to `StagePost` do not run; the engine's own stages do, so assets commit, maps spawn and music plays. Nothing of the game is drawn: the screen is drawn at window resolution in `DrawFinalScreen`, without the post chain. The tick in which the last screen ends runs the whole schedule, so gameplay starts then. Afterwards the screen's images are freed and a tick costs one nil check.

**Not shown** when `Config.Tool` is set (the editor). Tests that tick an Engine should leave `Startup` nil, or their systems wait for the screen.

**Cost.** The default image is a 720x720 JPEG of about 50 KB, embedded in every binary that references `WeakpixelScreen`, which `New` does and decoded on first use. In a browser, the [web shell](web.md#the-web-shell) starts the game at the "Click to start" click, so the screen runs in view.

## Frame log

To find stutter, set `Config.FrameLog` to a path, or run any engine game with `WEAKPIXEL_FRAMELOG=frames.csv` (the config field wins). The engine then writes one CSV row per `Draw` and flushes it on `Close` (and every 120 frames, so a crash loses little). It is a debug tool: it allocates and does file I/O. If the file cannot be created, the engine logs the error and runs without it.

| Column | Meaning |
| --- | --- |
| `frame`, `t_ms` | Draw counter and milliseconds since the first Draw |
| `draw_gap_ms` | Time since the previous Draw (a spike is a late or dropped frame) |
| `ticks` | `Update`s that ran since the previous Draw (normally 1 at 60 Hz; 0 or 2+ is a hitch) |
| `since_update_ms`, `alpha` | Time since the last `Update` ended, and the interpolation alpha the draw used (clamped to 0..1) |
| `update_ms`, `draw_ms` | CPU time of the last `Update` and of this `Draw` (the render calls only; Ebiten submits the GPU work later) |
| `cam_x`, `cam_y`, `cam_prev_x`, `cam_prev_y` | The camera's `Pos` and `PrevPos` |
| `view_x`, `view_y` | The interpolated camera position the frame was drawn at |
| `gc` | Number of completed GC cycles (a step between rows is a GC during that frame) |
| `fps`, `tps` | `ebiten.ActualFPS` and `ebiten.ActualTPS` |
| `origin_x`, `origin_y` | Where the frame drew the world origin, in logical pixels, through the view as drawn (`Camera.PixelSnap` included) |
| `scroll_x`, `scroll_y` | Change of `origin_x`/`origin_y` since the previous row: how far the frame scrolled the world (0 in the first row) |

Reading it: while the camera follows a steady walk, `scroll_x`/`scroll_y` should be the same from row to row (with `PixelSnap` and a whole zoom, the same whole number of pixels). A step that is 0 or twice the others is what the eye sees as a stutter or a jump; look for a `ticks` of 0 or 2+, a `draw_gap_ms` spike or a `gc` change in the same or the previous row. [`framelog`](../tools.md#framelog) lists these rows for you: `go run ./weakpixel framelog frames.csv`.

## Screenshots

`Engine.Screenshot` saves what the game shows as a PNG. Call it on the game goroutine, for example from a system that reacts to one of the game's own [input actions](input.md):

```go
e.Screenshot(game.ShotRequest{}) // the next frame, into ./screenshots
e.Screenshot(game.ShotRequest{Scale: 3, Done: func(r game.ShotResult) {
	if r.Err != nil {
		log.Print(r.Err)
	}
}})
```

A request is served by the next `Draw`, inside the real frame: the engine never renders an extra one, so the PNG is the frame the player saw. Requests queue; several in one frame share one readback per target and scale. While the [startup screen](#startup-screen) is up, they wait until it is gone.

| `ShotRequest` field | Meaning |
| --- | --- |
| `Target` | `game.ShotGame` (default): the logical frame at `Config.Width` x `Config.Height`, with layer groups, post passes and `PostChain.Fade`. `game.ShotWindow`: the frame as the window shows it, at window resolution, with the final pass (or Ebiten's default scaling) and the letterbox bars of `IntegerScale`. Pixel-art captures want the first; a final-pass effect such as CRT only shows in the second. |
| `Scale` | Enlarges a `ShotGame` capture by a whole multiple (1..8, 0 means 1) with nearest-neighbour filtering. Ignored for `ShotWindow`. |
| `KeepAlpha` | Keeps the frame's alpha. By default the PNG is opaque: the frame over black, as the window shows it. |
| `Path` | The file to write natively (folder created, file overwritten). Empty means a new name in `Config.Screenshots.Dir`. |
| `ID` | An opaque tag echoed in `ShotResult.ID`; the editor protocols use it to match answers to requests. |
| `Done` | Receives the `ShotResult` on the game goroutine at the start of a later `Update` (or in `Close`). Without `Done`, the engine logs the file written or the error. |

A capture never contains the stats overlay or anything a [development tool](#development-tools) draws in `DrawFinal` (an editor's selection, markers and outlines, a tool's grid and panels). Debug shapes and touch controls are part of the frame and are captured.

`ShotResult` has `Path` (the file written, `""` when none was), `PNG` (the encoded image), `ID`, `Width`, `Height`, `Tick` (the number of `Update`s before the frame) and `Err`. `Screenshot` never panics: an unknown target, a scale outside 1..8 or an image larger than 4096 pixels (or Ebiten's lower device limit, `ebiten.MaxImageSize`) on a side are reported in `Err`.

Where the PNG goes (`Config.Screenshots`):

| Build | Default |
| --- | --- |
| Native | A file. `Dir` (default `screenshots`, relative to the working directory, created on first use) and the name `screenshot-YYYYMMDD-HHMMSS-<tick>.png` in local time; an existing name gets `-2`, `-3` before `.png`. |
| Browser | A download with the same name. `Dir` and `Path` are ignored and `ShotResult.Path` stays empty. |

`Screenshots.Sink func(game.ShotResult) error` replaces both: it receives every successful capture (with `PNG`, without `Path`) on the game goroutine before `Done`, and a returned error becomes `Err`. `Engine.SetScreenshotSink` sets it later. Editors use it to send captures to their host: under the [level editor and the tools](../../design/editor.md#protocol) the game never writes a file.

### Hotkey and environment variables

`Config.Screenshots.Key` (an `input.Key`, zero for none) is a development hotkey: every press, the edge from up to down of the raw key state, takes one `ShotGame` capture (`KeyTarget` picks `ShotWindow` instead). It needs no input action and works in any game. It also works while a [development tool](#development-tools) pauses the game; keys the tool's own UI used never reach it. `game.DevScreenshotKey()` is F12, or F9 in a browser, which keeps F12 for its developer tools. The [starter project](../tools.md) turns it on, and so do the `wpix-editor` tools.

Without code changes:

| Variable | Effect |
| --- | --- |
| `WEAKPIXEL_SCREENSHOT_KEY` | A key name such as `F12`: turns the hotkey on. A `Key` set in code wins. |
| `WEAKPIXEL_SCREENSHOT_AT` | Comma-separated tick numbers, counted from the first tick after the startup screen. Each takes one `ShotGame` capture. |
| `WEAKPIXEL_SCREENSHOT_DIR` | Overrides `Config.Screenshots.Dir`. |
| `WEAKPIXEL_SCREENSHOT_QUIT` | `1`: quit after the last `WEAKPIXEL_SCREENSHOT_AT` capture was written. |

`WEAKPIXEL_SCREENSHOT_AT=1,60 WEAKPIXEL_SCREENSHOT_QUIT=1 go run ./mygame` writes two PNGs and exits, which is how to take golden images in CI. A native window still opens, because Ebiten needs a display: use `xvfb-run` on Linux CI. A bad value is logged and ignored. They work for every program built on `game.Engine`, the `wpix-editor` tools included.

### Host variables

A native program that drives an editor build (the [MCP server](../tools.md#the-game) does) sets these instead of a browser page; `editor/host` and `editor/bridge` read them. They share no name with the `WEAKPIXEL_SCREENSHOT_*` variables above, and unset they change nothing.

| Variable | Effect |
| --- | --- |
| `WEAKPIXEL_BRIDGE` | `host:port` of the host's TCP listener. The editor bridge connects to it, sends `WEAKPIXEL_BRIDGE_TOKEN` and a newline, then speaks the [editor protocol](../../design/editor.md#protocol) as JSON lines. When the host closes the connection the game quits. A host that cannot be reached is logged and the editor runs plain. |
| `WEAKPIXEL_BRIDGE_TOKEN` | The secret the game sends first; the host refuses a connection that does not present it. |
| `WEAKPIXEL_STAGE` | `level` or `prefab`: the editor stage. |
| `WEAKPIXEL_PATH` | The asset key of the first file. |
| `WEAKPIXEL_HEADLESS` | `1`: no window, no sound; the engine ticks at its rate. Needs `WEAKPIXEL_BRIDGE` (the game ends with the connection). Nothing is drawn, so screenshots are not served. |

Costs: a frame without a request does no extra work and allocates nothing. A capture frame copies the frame once on the GPU and waits for the GPU to finish it (`ReadPixels`), one stall on the game goroutine. The PNG is encoded and written on another goroutine; natively that keeps the frames that follow smooth, in a browser (one thread) the encode takes time from them. `ShotWindow` runs the final pass a second time.

## Levels

`LoadLevel(path)` reads a GOD file from `Config.FS`, spawns its objects at once (a document with [prefab instances](game-objects.md#prefabs) once its prefabs are Ready, see `Level.Spawned()`) and returns a `Level` with `Progress()`, `Done()`, `Err()`, `Objects()`. `Name()` returns the GOD file's optional top-level `"name"` (a [label for editors](../ecs/game-object-definition.md#document), `""` if none or for a map level). Assets load in the background (placeholders until ready) into a scope owned by the level. `UnloadLevel(l)` deletes the surviving entities and releases the scope. To switch without a gap: load the new level, wait for `Done()`, unload the old one.

`LoadLevelBase(base, opts)` loads a level of the [asset library](asset-library.md#engine) by its base key (`"forest/cave"`): the optional `<base>.level.json` and `<base>.tmj`/`.world` into one `Level`; the document is the pair of the map (see `LoadMap`) and spawns once. With a library it first mounts the level's package and the packages it uses. `UnloadLevel` also unmounts its packs.

To move between levels through exits placed in the editor, with a transition, install the opt-in director of package `room`: [rooms.md](rooms.md).

`LoadLevelData(path, data)` does the same for a document in memory (an editor's unsaved text, a downloaded level): `path` only names the level, assets still load from `Config.FS`. Errors read `game: LoadLevel "<path>": ...` for both. `Levels()` returns the loaded levels (GOD and map levels) in load order, as a copy; `UnloadLevel` removes a level from it. For tools that change a level's objects in place: `Level.Scope()` is the asset scope its keys resolve into, and `Level.SetObjects(objs)` replaces the list `UnloadLevel` deletes.

### Tiled maps

`LoadMap(path, opts)` loads a Tiled map (`*.tmj`) or world (`*.world`) into a scope owned by the returned `Level` and returns at once. The engine spawns the layers (one entity per tile and image layer, see [tilemaps.md](tilemaps.md#spawning-and-draw-order)) in the `assets` stage of the tick in which the map, or every map of the world, becomes Ready; game systems of that tick already see them. Until then `Level.Spawned()` is false and `Level.Maps()` is empty. `Level.Maps()` lists each map's handle, world position and layer entities. `Level.Bounds()` is the union of the maps' rectangles in world coordinates (zero until spawned), for example for the camera bounds (`e.CameraRig().Bounds = level.Bounds()`). A map that fails to load spawns nothing and sets `Level.Err()`. `UnloadLevel` deletes the layer entities (and cancels a level that has not spawned yet). Another suffix is an error. `LoadLevel` stays GOD-only.

**Paired documents.** A map `x.tmj` is loaded together with its GOD document `x.level.json` in the same folder; a world `w.world` with `w.level.json` plus each child map's `<map>.level.json`. Everything of one load lives in the `Level`'s one asset scope, `Level.Objects()` lists the objects of all documents in spawn order (the world's own document first, then per map in world-file order: its layers, then its document), `Level.Name()` is the first document's name, and one `UnloadLevel` removes it all. Every `*.level.json` is optional, and a missing one is not an error. A document that does not spawn (unreadable, invalid JSON or object, failed prefab) is joined into `Level.Err()` and sent to `OnAssetError`; the map and the other documents still spawn. A document with prefab instances spawns once its prefabs are Ready, independently of the others. `SpawnOptions.NoDocuments` loads the map alone, and `SpawnOptions.SkipDocuments` lists asset keys of documents not to spawn (for a tool that spawns that file itself); `tilemap.Spawn` ignores both. A group level looks files up in its group's file list; other levels use `fs.Stat` on `Config.FS`.

```go
level, _ := e.LoadMap("world-1.world", tilemap.SpawnOptions{BaseLayer: 0})
// later, in a system:
if level.Spawned() && !placed {
	for _, lm := range level.Maps() {
		m := e.Stores().Maps().Get(lm.Map)
		if l, o, ok := m.FindObject("player"); ok {
			hero.Pos = tilemap.ObjectPos(l, o, lm.Pos)
		}
	}
}
```

## Scenes

A game with screens (title, gameplay, pause, inventory) keeps them on a stack of `game.Scene`s. The systems you add with `Engine.AddSystem` and `AddRunner` form the root scene (`Engine.Root()`), always at the bottom: it cannot be popped or replaced, and a game that never pushes a scene only has it.

```go
pause := game.NewScene("pause", game.SceneOptions{DrawBelow: true, Layer: 1000})
pause.AddSystem(game.StageInput, func(ctx *system.Context) { /* Esc: e.Pop() */ })
pause.OnEnter(func(e *game.Engine, s *game.Scene) { s.Own(spawnPanel(e, s.Scope())) })

e.Push(pause) // from a system: the root freezes next tick
```

- `Push(s)`, `Pop()` and `Replace(s)` only queue the change. `Update` applies the queue in call order at its start, before any system runs, so systems never see a half-switched stack. Called before the first `Update` or from a `Start` hook, they apply before the first tick. `Top()` is the topmost scene, `Scenes()` a copy of the stack, root first.
- `AddSystem` and `AddRunner` of a scene work like the Engine's and panic with `game: AddSystem after the scene was pushed` once the scene was pushed. A runner's `Init` runs the first time the scene is pushed, once per `Scene` value (on the very first tick, before the root's own `Init`).
- Hooks: `OnEnter` (each push, after `Init`, the scene already on the stack), `OnExit` (pop or replace, before cleanup), `OnCover` (a scene was pushed on top) and `OnUncover` (the scene above was popped). `Replace` is a pop and a push without `OnUncover` and `OnCover` in between.
- Popping deletes what the scene owns: the levels loaded with `s.LoadLevel`, `LoadMap` and `LoadLevelBase` (like the Engine's, but owned), the entities passed to `s.Own(...)` that are still alive, and `s.Scope()` (an asset scope named `scene/<name>`, created on first use). A scene pushed again starts empty; its `Init` does not run again. Entities your systems spawn are yours to `Own`; nothing is tracked automatically.
- Panics: `game: nil scene`, `game: scene already on the stack` (applying a push of a scene that is on it), `game: Pop with only the root scene` (also for `Replace`), `game: Scope before the scene was pushed`.

**Options.** `SceneOptions{UpdateBelow, InputBelow, DrawBelow bool; Layer int16}` say what the scenes below keep doing. The zero value is the pause/menu default: they are frozen, read no input and are not drawn.

| Option | When false (default) |
| --- | --- |
| `UpdateBelow` | The scene below, and so every scene under it, does not run. A pause scene freezes the root too. |
| `InputBelow` | Scenes below skip their `StageInput` systems even if `UpdateBelow` keeps them simulating, so the top scene owns the presses. Read intent in `StageInput` (the convention); systems that read the router elsewhere are not filtered. |
| `DrawBelow` | Items on layers below the scene's `Layer` are not drawn (`render.Queue.SetMinLayer`, counted as culled). Give stacked scenes increasing layer bands: the game 0 to 999, the pause scene 1000 and up. |

Per tick, a stage runs the root's systems and then those of the active scenes, bottom to top, so an overlay acts after the game below it. The engine's own systems are not per scene: the input system, tweens, follow camera, animations, particles, extraction and audio run once for the whole World, so a frozen game below a pause menu keeps its animations and particles playing and the simulation time advances. Per-scene time and tweens are not covered. A Tool's pause, the startup screen and `Hold()` apply to scene systems exactly as to the root's; while a Tool pauses, scene runners that are `EditRunner`s keep running. The `EventReader`s of a frozen scene miss events older than two ticks, as any reader that does not run.

A tick costs 0 allocs with and without scenes; only a stack change allocates. Rooms can live in a scene: `room.Config.Scene` ([rooms.md](rooms.md#config)).

## Development tools

`Config.Tool` attaches a `game.Tool`, an interface for development tools such as a level editor. The engine only calls it; a game without a tool links no tool code and pays one nil check per `Update` and `Draw`.

| Method | Called |
| --- | --- |
| `Init(e)` | Once, at the end of `New`. |
| `Input(src) input.Source` | Once, in `New` before `Init`, with the device source. The router reads the returned source, so a tool can hide the clicks its own UI used. |
| `Tick(e) TickMode` | At the start of every `Update` (after the first one's `Start` hooks). |
| `DrawFinal(e, screen)` | At the end of `DrawFinalScreen`, after the final pass, in window pixels. |

`Tick` returns `game.TickRun` (normal tick), `TickPause` or `TickStep` (one full tick, then the tool returns `TickPause` again). A paused tick skips the `input` stage (including the engine's input system) and every system in `simulate`, `camera` and `post`, and still runs the engine's own stages: assets load and hot-reload, transforms are snapshotted, animations play, the frame is extracted and audio runs. So the paused scene keeps drawing, with live sprites, and the shader `Time` keeps advancing. `Engine.Paused()` reports the mode of the current tick. With a tool set, each of those four stages runs its systems through one gate (one branch per stage per tick); `Init` of wrapped runners still runs once, in order.

A runner that implements `game.EditRunner` (`RunsWhilePaused() bool`) and returns true keeps running in those stages while the tool pauses, in its place among the other runners. The gate splits each stage's runners once, when the schedule is built. Use it for setup and presentation work an edited scene needs (placing the hero once the world has spawned), never for simulation; it has no effect without a tool.

A tool may also implement `game.DrawGate` (`DrawFrame(e) bool`) to skip frames: `Draw` calls it once per frame and, when it returns false, leaves the screen as it is. Ebitengine then keeps showing the last frame and, after a few skipped frames, stops presenting and calling `DrawFinalScreen`. The engine draws anyway while the startup screen is up or a capture waits for a frame, and does not ask then. Skipped frames only keep their image when the screen is not cleared between frames, so whoever runs the game calls `ebiten.SetScreenClearedEveryFrame(false)` (`editor/host.Run` does); a drawn frame fills the whole screen first. The interface is checked once, in `New`. The editor uses it to draw only after a tick that changed the picture ([pacing](editor.md#pacing)).

`Engine.FrameChanged()` tells a gate whether a `Draw` now would show something new: the render queue of the last `Update` differs from the last drawn one item by item, or the camera, background, `Post().Fade` or window size changed, or an asset finished loading or reloaded since (an image can change behind the same handle). A frame that depends on time is always new: an item with a shader, a post-processing pass or a final pass (their `Time` uniform moves every tick). It is true before the first drawn frame. With a gate set, `Draw` keeps a copy of the queue for it (no allocation once its capacity has grown); it costs about one copy of the queue per call.

`Engine.WindowToLogical(p)` maps a window pixel (`ebiten.CursorPosition` as a tool sees it, and the space `DrawFinal` draws in) to logical pixels, the input of `Camera().ScreenToWorld`. It inverts the transform of the last `DrawFinalScreen` (the identity before the first frame). `Engine.LogicalToWindow(p)` is its inverse, for drawing over a world position in `DrawFinal`. The level editor core built on this hook is [`editor`](editor.md).

## Escape hatches

`World()`, `Assets()`, `Library()` (`Config.Library`), `Boot()` (the load of `global/`), `Camera()`, `CameraRig()` (the follow camera's `*camera.Rig`; also a world resource; [camera.md](camera.md)), `Layers()`, `Groups()` (layer groups, [shaders guide](shaders.md#layer-groups); also a world resource), `Canvases()` (pre-rendered images for chunked tile layers, [rendering.md](rendering.md#canvases); also a world resource), `Post()` (post-processing passes and the final window-resolution pass, [shaders guide](shaders.md#post-processing); also a world resource; the engine implements `ebiten.FinalScreenDrawer` for it), `Renderer()` (created lazily: it needs the game goroutine), `Sound()` (the `sound.Queue`), `Mixer()`, `Listener()`, `Audio()` (the `AudioSystem`, for `Stats`), `SampleRate()` (the audio sample rate: the audio context's, or `Config.SampleRate` under `NoAudio`; a stream source must produce PCM at it), `Input()` (the `input.Router`; also the `*input.Router` and `*input.Events` world resources), `TouchControls()`. Everything the engine created is a normal low-level object; you can mix in your own extract systems with `AddExtract(...)`.

## Not covered

Multiple cameras, variable timestep, per-scene time, tweens and animation.
