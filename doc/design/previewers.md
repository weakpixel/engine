# Previewers

How the `view` tool of `wpix-editor` previews every file kind that is not edited (`editor/viewer`, `editor/voiceview`).
The shell it runs on is in [tool-shell.md](tool-shell.md); the editing tools are in [editor-tools.md](editor-tools.md).

- **`view` is the preview of every kind that is not edited**, with a previewer per kind (`viewer`: texture, atlas, font, Tiled map, tileset, world and shader; an Aseprite export goes to `animedit`, a voice or a dialogue to `voiceview`), chosen from `filekind` when a file opens.
  An edited kind is refused: it opens in its editor.
  An editor never routes; it runs on its own shell with its document.
  The atlas previewer lists frames from `render.Atlas.Order` (file order), the font previewer reads the face from the engine's font store.
  Overlay text in window pixels is rendered once to small images (Go Regular, so no font file is needed), since `text.Draw` cannot target a `FinalScreen`.
  An Aseprite export plays in the anim tool read-only (`animedit.NewPreview`, which loads the clip set in the viewer's scope) by treating the export as an empty definition of itself: edits fail with `ErrReadOnly`, so there is one code path for playback.
  The host's `open` message switches the viewer to another file in place: the previewer's entities are deleted, the viewer's asset scope is released (the old file's assets unload unless the new file holds them too) and the new file is framed, while the grid and background stay.
  The shader previewer (`.kage`) builds on the texture previewer: the sprite gets the shader and a `Params` value per `+shader:param` marker (a number or slider, toggle, angle in degrees or color widget; four plain numbers without markers), "Enabled" compares with the plain texture, and the texture is the built-in sample (served from the overlay) or any project image.
  A compile error or a marker problem is shown in the panel and sent as an `error` naming the file (and the marker line).
  `Time` is the engine's simulation time, which keeps running under the shell's pause; the values are not saved.
  A later previewer (M7 bindings) is one more entry in the viewer.
- **The Tiled previewers load with the engine's loaders and draw objects as overlays.**
  A map (`*.tmj`) and a world (`*.world`) are loaded with `LoadMapSync` and `LoadWorldSync` in the viewer's scope and spawned with `tilemap.Spawn` and `SpawnWorld`, so tile and image layers are the same `TileLayer` entities the game draws (tints, opacity, shaders and parallax as in the game).
  Objects are not entities in the engine, so they are window-space outlines (`viewer/shapes.go`: rect, ellipse as 24 segments, polygon, polyline, a cross for a point, the box of a tile object or text; hit test by box, point-in-polygon, or 5 device-independent pixels for lines and points, where a point's cross has 4 px arms), named with the same label cache and size rules as atlas frames.
  Overlay labels are 13 px Go Regular on a dark plate, in device-independent pixels: the final screen is in device pixels, so the label cache renders at 13 times the display scale factor (re-rendering when it changes), and the 24 px minimum width a frame or object needs for an unforced label scales the same way.
  Each layer's checkbox is Tiled's own `visible` (read from the file again, since the loaded map has it composed with the groups); a group's checkbox hides its children, and the toggles are kept by layer path across a hot reload.
  A tileset (`*.tsj`) is one sprite entity per tile on the tileset's grid (`columns`, else a square-ish grid for a collection), so single-image tilesets and collections look alike; animated tiles take `Tile.FrameAt` of a clock the previewer advances by 1/60 s per tick (the "Animate" checkbox pauses them at frame 0).
  A world shows each map's rectangle with its file and levels.
  A click selects and a double click opens the first level of a map, so hovering and selecting are safe.
  A hot reload (the asset version of the map, tileset or world changed) deletes and spawns the entities again and keeps the camera.
  Tiled files are never written (Tiled is the editor).
- **Voices and dialogues preview with sound** (`editor/voiceview`, routed by `viewer`).
  A page or window starts with sound when its first file's kind has sound (`filekind.Info.Sound` turns off `game.Config.NoAudio`, natively and in the js/wasm build), because `NoAudio` is fixed when the engine starts and package speech must be installed before the schedule is built.
  The viewer's `Init` then creates the voice previewer, whose `Init` calls `speech.Install`, and keeps it across files; a viewer that started without sound refuses a voice or a dialogue, so the host reloads the page for it. since the shell pauses the game, `Sync` calls `Speech.Step` with one tick.
  A `*.voice.json` speaks the panel's line (text field, speed, Say, Skip, Stop) on an entity with a `speech:Speaker` and a `gfx.Label` but no `Transform`, so the renderer does not draw the label; the tool draws the shown text itself in `Draw`, wrapped once over the whole line so lines do not move while it types (Go Regular at 16 device-independent pixels), and under it a piano roll of the line from `Speech.LineNotes` (time left to right, pitch bottom to top over at least an octave, a color per instrument) with a playhead from `Speech.LinePosition`.
  A `*.dialogue.json` plays in a `speech:DialogueBox` entity with a name label (Next, Restart, Stop; Next after the end plays again), its status line names the line (from `LineStarted`) and the speaker.
  A reloaded voice says the line again; a reloaded dialogue restarts.
  In a webview the sound starts after the first click inside the canvas (a click on the panel does not unlock the page's audio, the voice library's finding); the panel says so.
  Nothing is written: voices and dialogues are edited in the library's Voice Editor.
  It is a `Closer`, so the viewer can switch files.
- **Which levels use a map.**
  Only in a library package: the `<map base>.level.json` beside the map (the engine spawns it with the map, see `LoadMap`), and the package's `<pkg>/<pkg>.level.json` when its backdrop (`<pkg>/<pkg>.tmj`, else `.world`) is or lists the map.
  The previewer looks the files up with `Shell.Files()` (the page build) or `fs.Stat` (native); it lists no directory.
- **Overlay primitives.**
  `Shell.FillRect` draws axis-aligned rectangles; `Shell.StrokeLine` and `Shell.StrokePolyline` draw lines of any angle and width in window pixels (the shared 1×1 white pixel, scaled and rotated, with square ends and no anti-aliasing).
  `Shell.WindowRect` and `Shell.WindowPoint` map world positions to window pixels.
