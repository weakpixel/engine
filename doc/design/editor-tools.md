# Editor tools

Design of the `fx`, `anim` and `sheet` tools of `wpix-editor`.
The shell they run on is in [tool-shell.md](tool-shell.md).

## The fx tool

`wpix-editor <file>.fx.json` (`editor/fxedit`) makes a particle effect: a `*.fx.json` file holding one to eight `fxfile.Emitter` values ([format](../guide/engine/particles.md#the-effect-file)).
Everything but `tool.go` is plain Go without Ebiten.

- **The model is the whole file.**
  The file is small and fully owned by the tool, so there is no text splice: `fxfile.Decode` reads it and `fxfile.Encode` writes the canonical form, which the tool also uses as the document text.
  An edit replaces the document with the encoded emitter, so undo, dirty state and the diff in VSCode work on the file as any other.
  The tool's `Emitter` is `fxfile.Emitter` itself: authored units (degrees are the file's, radians the simulation's) and sprite and shader as asset keys.
- **Several emitters.**
  `State` holds the emitter list (`Ems`), the selected index (`Sel`) and a copy of the selected emitter (`Em`), which every panel field edits.
  The Emitters section (a list on the left) selects, adds, duplicates, deletes and reorders (one document edit each); the Emitter section on the right edits the name and the draw layer (one document edit each).
  Mute and solo are preview-only: `Muted` never reaches the document, and the preview text leaves out muted emitters (all muted plays one that spawns nothing).
  The Live readout and the Max cap are sums over the emitters; Save as exports every built-in sprite (`ExportAll`: `name.png`, then `name-2.png`, ...).
- **The preview is the real asset.**
  The tool writes the encoded text into `toolshell.OverlayFS` as `fx-builtin/preview.fx.json`; the first publish loads it with `particles.LoadEffect`, later ones call `Changed` so the engine's hot reload applies them.
  A drag does not restart the effect; only a cycle key does (see the renderer notes).
- **Presets and built-in sprites are code.**
  The presets are Go values tested to be valid and to play, each emitter on its own: single emitters (smoke, sparks, fire, rain, snow, ...) and whole effects of up to eight named emitters (campfire, fireworks, thunderstorm, confetti, magic portal, dragon breath, and supernova, which uses all eight).
  Burning scraps shows the built-in Burn shader, which reads the tint's alpha as the part not burned yet (a particle shader's only per-particle input is its tint).
  Applying a preset replaces the whole effect in one edit and clears the mutes and the selection; a single-emitter preset applies Keep to the selected emitter, a multi-emitter one ignores it.
  Reset (Emitters section) is one edit back to the default preset's emitter.
  The sprites (soft circle, dot, spark, ring, star, square, streak, flake, heart, plus) are drawn with `image/png`; the built-in shaders are embedded Kage files (`fxedit/shaders/`, so far `burn.kage`).
  While editing they are served from `toolshell.OverlayFS` under `fx-builtin/`, which the engine loads like any asset.
  Save as copies the ones in use next to the effect (`name.png`, `name.kage`, then `-2`, ...) and rewrites the keys, so a saved effect never names `fx-builtin/`; a saved file that does gets a Problems finding.
- **One panel edit is one document edit.**
  The preview controls use `particles.Clock`, a world resource the engine registers at its defaults: scale, pause, step and a seek that restarts and replays up to 600 ticks.
  The preview entity is one entity without a `Seed`, so its pool seed is stable and a scrub is deterministic.
- **Shader parameters** come from `shaderparam.Parse` (`internal/shaderparam`, [shaders.md](../guide/engine/shaders.md#parameter-markers)); the tool reads the shader's source when it changes or its file does, and without markers shows four plain numbers.
- **Untitled effects.**
  In VSCode, New Particle Effect starts the tool on a `MemoryDocument` holding the smoke preset (natively a missing file starts from the same effect, see the starters above).
  Save as writes the effect and sprite natively through `Shell.Create` and continues on a `FileDocument` with the undo history; in VSCode the wasm build cannot write, so `Create` sends the `create` message and the extension writes and reopens.
  The target is asked about before it is overwritten (the button becomes Overwrite?).
- **A document that does not parse** is handled as in the anim tool; a missing image or shader is a Problems finding and a placeholder, not a failure.
- **Not built:** curves and gradients over life, sub-emitters, trails and flipbooks (not in the engine), A/B snapshots, atlas frames in the image picker (only files), a live level as background.

## The anim tool

`wpix-editor <file>.anim.json` (`editor/animedit`) edits an [animation definition](../guide/engine/animation.md#animation-definition-animjson) on its Aseprite sheet.

- **The model is the file.**
  The tool holds the decoded `animfile.File`.
  Every change is a function on it, then `animfile.Encode`, then one `Shell.Edit`; an undo, a change on disk or the host's `document` comes back through `Load`.
  The file it writes is one the loader and `wpix anim validate` accept, and `animfile.Diagnose` is the single source of the findings both print (the editor shows them in a Problems section).
- **The sheet is read for tags and pixels; the definition is not loaded through the asset server.**
  The sheet's JSON goes through `animfile.DecodeSheet` (tags, trim offsets) and the sheet is loaded as a clip set for the frame sprites, so the PNG and the sheet hot-reload.
  The loader would reject an invalid definition, and a half-edited file must still draw.
- **World space is pivot space.**
  The shown clip's pivot is world (0, 0) and `body` and shape rects are used as world rectangles unchanged, so a drag in whole world pixels is the new `rect`.
  Flip H mirrors the drawing around x = 0 and never changes the file.
  `anim.PivotOrigin` turns a pivot into `Sprite.Origin`; the loader uses the same function.
- **One edit per gesture.**
  A drag previews and writes on release, so one drag is one undo step.
  A box drawn on frame n becomes `frames {from: n}`; "Copy to this frame" keys a box per frame.
- **An invalid document is shown, not overwritten.**
  If the text does not parse the tool keeps the last good version dimmed, says why in the status line and ignores edits until a valid text arrives.
- **Playback runs on the tool's clock.**
  The shell ticks paused with no animation system, and the editor needs stepping and scrubbing, so the tool keeps its own time and asks `anim.Clip.FrameAt` (built from the resolved durations and loop mode) for the frame.
- **New definitions.**
  `--sheet <export.json>` with a missing file starts from `animfile.Starter` as an unsaved document; Ctrl/Cmd+S creates the file.

## The sheet tool

`wpix-editor [--image <texture>] <file>.sheet.json` (`editor/sheetedit`) edits a [grid sheet](../guide/engine/asset-kinds.md#grid-sheet).
Everything but `tool.go` is plain Go without Ebiten.

- **One reader and writer.**
  `sheetfile` (stdlib only) holds the format: `Decode` and `Validate` (the loader's checks, all problems at once), `Encode` (two-space indent, zero margin and spacing left out) and the layout (`File.Layout`, `Grid.Rect`, `Grid.At`).
  The atlas loader in `render` expands its frames with the same `Layout`, so the tool shows the frames the game gets.
- **The model is the file.**
  A panel change is a function on `sheetfile.File`, then `Encode`, then one `Shell.Edit`, so undo, dirty state and the diff in VSCode work on the file as for any other document.
  Values the format refuses (a frame size below 1, a negative margin or spacing, a fraction) are rejected by `Set` and nothing is written.
  The pivot is kept by the encoder and not edited.
  A document that does not parse is handled as in the anim tool: the last good sheet stays, edits fail with `ErrStale`.
- **The texture is a sprite at the origin.**
  It is loaded with `render.LoadSpriteSync` from the sheet's folder and the `image` value, with its top-left corner at world (0, 0), so a world point is an image pixel; `viewkit.Texture` keeps it from being culled.
  A missing or changed image is a Problems finding and a hot reload, not a failure.
  The grid is a window-space overlay drawn by `viewkit.Frames`, as the atlas previewer draws its frames: index labels, hover, click selection and the Frames list, with the leftovers dimmed and, below 4 window pixels per cell, only the border of the grid.
  The list follows the grid after every edit and keeps a selection whose index still exists.
  Above `sheetedit.MaxListed` (4096) frames the list stays empty and only the cells on screen are outlined, so a frame width of 1 typed on a big image does not send a list of thousands of entries to the panel.
- **New sheets.**
  `--image` with a file that does not exist starts an unsaved document (`sheetedit.NewSheet`, `sheetfile.Starter`).
  In VSCode the extension writes the starter text itself.
- **Not built:** pivot editing, frame names, trimming empty cells.
