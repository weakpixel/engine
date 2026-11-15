# Animation (`anim`)

Aseprite clips plus a system that plays them. The renderer never sees animation: the system writes `Sprite.ID` (and `Sprite.Origin` for clips with a pivot). An optional [animation definition](#animation-definition-animjson) (`player.anim.json`) adds loop and timing overrides, a pivot and hit/hurt boxes on top of the Aseprite export.

## Setup

```go
render.RegisterLoaders(srv, render.LoaderOptions{})
anim.RegisterLoaders(srv) // after render: ClipSet depends on render.Atlas (panics otherwise)

s.AddRunner(extract.NewAssetSystem(w))      // first, so clips are Ready
// ...
s.AddRunner(anim.NewAnimationSystem(w))     // simulate stage, before the extract stage
```

## Export from Aseprite

*File > Export Sprite Sheet*, JSON data (hash or array), with **Meta: Tags** enabled. You get a PNG and a JSON next to it; name the JSON `<name>.aseprite.json` or `<name>.ase.json` so the asset library and the editor know it as an Aseprite export (any other `*.json` still loads by key). Each tag becomes a clip, and per-frame durations are kept.

| Aseprite tag direction | `Clip.Loop` |
| --- | --- |
| forward | `Loop` |
| reverse | `Loop`, frames already reversed |
| pingpong | `PingPong` |

`repeat` is ignored. A file without tags has one looping clip named `all`.

## Using clips

```go
// One clip, by tag:
run := anim.LoadClip(sc, "chars/player.aseprite.json#run") // gfx.ClipHandle

ecs.NewEntity3(w,
	gfx.Transform{Pos: p, PrevPos: p},
	gfx.Sprite{Layer: 2},
	gfx.Animator{Clip: run, Speed: 1},
)

// All clips of a file:
set, _ := anim.LoadClipSetSync(sc, "chars/player.aseprite.json") // anim.ClipSetHandle
cs := anim.ClipSetStoreOf(srv).Get(set)
cs.Clips["idle"]; cs.Names // tag names in file order
```

Loading a `#tag` loads the set, atlas and texture too; a held clip keeps them alive.

Switch clips:

```go
a := ecs.GetComp[gfx.Animator](w, e)
a.Clip, a.Time, a.Frame, a.Done = idle, 0, 0, false
```

## AnimationSystem

For each entity with `gfx.Animator` + `gfx.Sprite`: `Time += dt * Speed`, find the frame, write its sprite to `Sprite.ID`, set `Animator.Frame`. If the frame has a pivot (`Frame.Origin` non-zero, from an `*.anim.json`), write `Sprite.Origin` too.

- Zero clip handle or a clip that isn't loaded: the `Sprite` is left alone (placeholder or your sprite shows).
- `Done` (non-looping clip finished): the entity is skipped.
- A clip with a pivot **owns `Sprite.Origin`**: whatever you set is overwritten every tick. Clips without a pivot never touch it.
- Needs the `*asset.Server` resource. 0 allocs.

## Types

| Type | |
| --- | --- |
| `Clip` | `Frames []Frame`, `Loop LoopMode`, `Total time.Duration`; `FrameAt(t) (frame, done)`, `ShapesAt(frame) []gfx.Box` |
| `Frame` | `Sprite`, `Duration`, `Events uint32` (free bitmask for your gameplay; the loader leaves it 0), `Origin gfx.Vec2` (pivot, zero = none), `Boxes []gfx.Box` (hit/hurt boxes, nil = none) |
| `ClipSet` | `Atlas render.AtlasHandle`, `Clips map[string]gfx.ClipHandle`, `Names []string`, `Default string` (`defaultClip` of an `*.anim.json`, else empty) |

`LoopMode`: see [kinds.md](kinds.md#loop-modes-animloopmode). Reading `Frame` after the system runs lets gameplay react to a frame, e.g. a footstep.

## Animation definition (`*.anim.json`)

A sidecar next to the Aseprite export that becomes the root asset of an animated character. It is loaded as a `ClipSet` like the export itself; the loader picks the format by the `.anim.json` suffix (plain `.json` stays the Aseprite path).

```go
set, _ := anim.LoadClipSetSync(sc, "chars/player.anim.json")
slash := anim.LoadClip(sc, "chars/player.anim.json#Slash")
```

The definition declares its `sheet` (the Aseprite export, itself a `ClipSet` with an atlas) as a dependency, copies the sheet's clips, applies the overrides and publishes them as its own children. Both key spaces stay valid: `player.json#run` is the plain clip, `player.anim.json#run` the overridden one. `ClipSet.Atlas` is the sheet's atlas, `Names` is the sheet's tag order.

There is **no collision detection** here: the boxes are data. Gameplay asks for the boxes of the current frame and decides what overlaps and what a hit means.

### Format (version 1)

A definition, abridged:

```json
{
  "version": 1,
  "sheet": "player.aseprite.json",
  "pivot": { "x": 19, "y": 45 },
  "defaultClip": "Idle",
  "body": [ { "x": -4, "y": -13, "w": 8, "h": 13 } ],
  "clips": {
    "Run":   { "loop": true, "fps": 8 },
    "Slash": { "loop": false, "fps": 8,
      "shapes": [ { "kind": "hit", "id": "slash", "frames": { "from": 2 },
                    "rect": { "x": -17, "y": -2, "w": 35, "h": 17 } } ] }
  }
}
```

| Field | Type | Meaning |
| --- | --- | --- |
| `version` | int | must be `1` |
| `sheet` | string | the Aseprite export, relative to the definition. Required. Not another `*.anim.json` |
| `pivot` | `{x, y}` | default pivot of every clip (see coordinate spaces). Optional: without one, `Sprite.Origin` is left alone, so the sprite's default origin applies: the frame's `pivot` from the sheet JSON if it has one, else the **center of the untrimmed frame** (not top-left) |
| `defaultClip` | string | the clip to start with, stored as `ClipSet.Default`. Informational; must name a tag of the sheet |
| `body` | `[rect]` | default hurt boxes of every frame of every clip (ID `"body"`). Their union is `Clip.Body`, which a `collision.ShapeAnimBody` collider uses as its box ([collision.md](collision.md#the-collider-from-the-animation-body)) |
| `clips` | `{name: clip}` | per-clip overrides, keyed by Aseprite tag name. Unlisted clips keep the sheet's loop mode and timing but still get `pivot` and `body` |

Clip:

| Field | Type | Meaning |
| --- | --- | --- |
| `loop` | bool | `false`: `Once`. `true`: keep `Loop`/`PingPong` from the Aseprite direction, else `Loop`. Absent: the sheet's mode |
| `fps` | int > 0 | every frame lasts `1000/fps` ms, rounded to a whole ms, at least 1 ms |
| `durations` | `[int]` ms | one positive value per frame; wins over `fps` |
| `speed` | float > 0 | divides every duration (after `durations`/`fps`). The only float in the format |
| `pivot` | `{x, y}` | overrides the top-level pivot for this clip |
| `shapes` | `[shape]` | hit and hurt boxes |

Shape:

| Field | Type | Meaning |
| --- | --- | --- |
| `kind` | `"hit"` or `"hurt"` | `gfx.BoxHit` (deals hits) or `gfx.BoxHurt` (receives them) |
| `id` | string | free name, may be empty. Boxes of one attack usually share it ("one hit per swing" is gameplay's job) |
| `frames` | `{from, to?}` | inclusive range in play order; `to` omitted means the single frame `from`; `frames` omitted means every frame |
| `rect` | `{x, y, w, h}` | relative to the pivot; `w`, `h` > 0 |

Timing: base = `durations` if present, else `fps`, else the Aseprite durations; then `speed` divides. `Clip.Total` is recomputed. `Animator.Speed` still multiplies on top per entity. Use `speed` to keep the artist's rhythm faster or slower, `fps` to flatten it, `durations` to retime single frames.

### Coordinate spaces

All geometry is **integer pixels**, origin top-left, y down.

- **`pivot`** is in pixels of the **trimmed sprite image** (the rectangle Aseprite packed into the sheet). The loader converts it per frame to the renderer's fraction, `Frame.Origin = (Sprite.Offset + pivot) / Sprite.Size`, The pivot is measured from **each frame's own trimmed rectangle**: it stays on the same art pixel when every frame of the clip has the same trim offset (Aseprite's "Trim Sprite", as in the hero sheet), but with "Trim Cels" (each frame trimmed separately) the art shifts from frame to frame. `wpix anim validate` warns about such clips. An all-zero result becomes `(1e-4, 1e-4)`, because the zero `Origin` means "default pivot" to the renderer.
- **`rect`** (body and shapes) is relative to the pivot: the pivot is `(0, 0)`, so a box above the feet has a negative `y`.
- **Frame indices** (`frames`, `durations`) are in the clip's **play order**: for a reversed Aseprite tag, frame 0 is the tag's last frame.

### Boxes per frame

Each frame gets, in this order: the `body` boxes, then every shape whose range covers it, in file order. If any **`hurt` shape** covers a frame, the clip's hurt shapes **replace** the body boxes on that frame (a duck or a roll changes where the character can be hit). `hit` shapes never replace anything.

Each clip keeps one backing array; `Frame.Boxes` is a sub-slice of it with its capacity capped at its length. A clip from a plain Aseprite export has no boxes.

### Querying boxes

```go
clips := anim.ClipStoreOf(srv) // cache it
for _, c := range view.Each() {        // View3[gfx.Transform, gfx.Animator, gfx.Sprite]
	t, an, sp := c.Unpack()
	for _, b := range anim.ShapesOf(clips, an) {
		r := b.World(t, sp.FlipX, sp.FlipY) // gfx.Rect in world units
		if b.Kind == gfx.BoxHit { /* test r against other entities' hurt boxes */ }
	}
}
```

- `anim.ShapesOf(clips, an)` returns the boxes of `an.Frame`, or nil for a zero handle, a clip that is not `Ready`, a frame out of range, or a clip without boxes. `Clip.ShapesAt(frame)` is the same for a clip in hand. Both return a slice into the clip (don't modify it) and do not allocate.
- `Box.World(t, flipX, flipY)` places a box like the renderer places the sprite: a flip mirrors around the pivot (`x' = -(x + w)`, same for y), then `Transform.Scale` scales (zero means `(1, 1)`, a negative scale mirrors too) and `Transform.Pos` moves it. **Rotation is ignored**: the result is axis-aligned.
- `Animator.Frame` is written by the `AnimationSystem`, which the `game.Engine` runs after all game stages. Systems in those stages therefore see the frame of the previous tick.

### Animation end

Polled, not an event: a `Once` clip sets `Animator.Done` when it has reached its last frame (and the system stops advancing it); `Animator.Frame` tells where a clip is. Restart a clip by zeroing `Time`, `Frame` and `Done`.

### Errors

Unknown fields are errors (typos don't pass silently), as is a float where an int is expected. Syntax errors, a bad `version` and a missing `sheet` fail in Decode. Everything else (unknown or duplicate clip names, an unknown `defaultClip`, frames out of range, `w`/`h` <= 0, a bad `kind`, `fps`/`speed`/`durations` that are not positive, a `durations` length that doesn't match the frames) is collected into **one** error, one line per problem, each naming its clip and shape:

```
anim json: clip "jump": no such tag in sheet "player.json"
clip "idle": "fps" is -1, want > 0
clip "idle": shape 0: frames 9..9 are outside the clip's 2 frames
```

Not supported: per-frame pivots, rotated boxes, `Animator.Loops`/wrap events, blending.

With [asset hot reload](assets.md#hot-reload) on, editing the Aseprite export, its PNG or an `*.anim.json` updates the clips in place: `Animator.Clip` handles stay valid and playing animators continue at their current time.

### Package `animfile`

Parsing and checking live in `anim/animfile` (standard library only: no Ebiten, no ECS). The loader is an adapter over it, and `wpix anim` uses it too, so both accept exactly the same files.

| | |
| --- | --- |
| `DecodeSheet(data) (*Sheet, error)` | the Aseprite export: frames (duration, trim offset) and tags; `Sheet.PlayOrder()` gives `Resolve` its input |
| `Decode(data) (*File, error)` | syntax, unknown fields, ints, `version`, `sheet` |
| `Resolve(file, tags) (*Resolved, error)` | everything that needs the sheet; per clip the final loop mode, durations, pivot and boxes, plus `Omitted` (tags without a `clips` entry) |
| `Encode(file) ([]byte, error)` | 2-space indented JSON, clips in slice order |
| `Diagnose(file, sheet) (*Resolved, []Finding)` | `Resolve` plus the warnings and notes of `wpix anim validate` (uneven trims under a pivot, a sheet over 8192, tags the file does not list), as findings with a level; the resolved definition is nil when there are errors |
| `Starter(sheet, ref) *File` | the definition `wpix anim create` writes |

### Tooling: `wpix anim`

The `wpix-editor anim` tool edits these files visually ([editor guide](editor.md#the-anim-tool)). `anim.PivotOrigin(sprite, pivot)` is the pivot-to-`Sprite.Origin` conversion the loader and the editor share.

```bash
go run ./tools/wpix anim validate chars/player.anim.json [more.anim.json ...]
go run ./tools/wpix anim create --file chars/player.json [-o chars/player.anim.json | -o -] [--force]
```

**`validate`** checks each definition against its sheet JSON (`sheet`, relative to the definition) with the loader's rules, without reading the PNG. One line per finding, then a summary:

```
chars/player.anim.json: error: clip "jump": no such tag in sheet "player.json"
chars/player.anim.json: warn: clip "walk": frames are trimmed differently (offsets 0,3 and 2,1); the pivot is measured from each frame's trimmed image, so the art shifts between frames
chars/player.anim.json: info: tag "Death" is not listed in clips: it keeps the sheet's loop mode and timing
chars/player.anim.json: 1 error
```

| Level | When |
| --- | --- |
| `error` | anything the loader rejects: unreadable files, syntax, unknown fields, unknown or duplicate clips, bad ranges, rects, kinds, timing; an invalid sheet (`sheet "x.json": ...`) |
| `warn` | a clip with a pivot whose frames are trimmed differently; a file name that does not end in `.anim.json` (the loader would read it as an Aseprite export); a sheet wider or taller than 8192, which many laptops, phones and browsers cannot load (pack it into rows: Aseprite's "By Rows" or a fixed width) |
| `info` | a tag of the sheet the file does not list (not an error: the clip keeps the sheet's loop mode and timing and still gets `pivot` and `body`) |

A valid file ends with `<file>: ok (N tags, M listed, K omitted)`. Not checked: anything that needs the atlas or PNG (frame count against the image, sprite sizes).

**`create --file sheet.json`** writes a starter definition: `version` 1, `sheet` relative to the output file, `defaultClip` = the first tag, and every tag in sheet order as `{"loop": true}` (keeps the sheet's behavior, including ping-pong, and shows where `false` goes). No `pivot` (so the sheet's default origin applies, see the `pivot` row above), no `body`, no shapes. A sheet without tags gives the clip `all`.

| Flag | |
| --- | --- |
| `--file` | the Aseprite sheet JSON (required) |
| `-o` | output path; default: the sheet path with `.json` replaced by `.anim.json`; `-` writes to stdout (then `sheet` is the sheet's base name: put the file next to it) |
| `--force` | replace an existing output file (otherwise it is an error) |

Exit codes (both commands): `0` ok (warnings and info allowed), `1` errors, `2` usage error. 
