# Asset kinds: what they are and what they are for

This page explains each kind of asset the engine loads, when you use it, how to load it and which [file formats](#file-formats) it reads. For the mechanics shared by all kinds (handles, scopes, states, placeholders) see the [asset system guide](https://github.com/weakpixel/asset/blob/main/doc/guide.md); for the load and store API per kind see [assets.md](assets.md#game-facing-handles). The `*.anim.json` animation definition has its own reference in [animation.md](animation.md#animation-definition-animjson).

## The short version

| Kind | It is | You use it for | Key | Load with | Handle |
| --- | --- | --- | --- | --- | --- |
| [Sprite](#sprite) | One drawable picture | Anything on screen: characters, tiles, items, UI icons | `img.png` or `atlas.json#frame` | `render.LoadSprite` | `gfx.SpriteHandle` |
| [Atlas](#atlas) | One image cut into many named sprites | Tilesets, sprite sheets, UI skins | `atlas.json` or `grid.sheet.json` | `render.LoadAtlas` | `render.AtlasHandle` |
| [Clip](#clip) | One animation: frames with durations | Playing "run", "idle", "attack" on an entity | `player.aseprite.json#tag` or `player.anim.json#tag` | `anim.LoadClip` | `gfx.ClipHandle` |
| [Clip set](#clip-set) | All animations of one character file | Switching between a character's animations | `player.aseprite.json` or `player.anim.json` | `anim.LoadClipSet` | `anim.ClipSetHandle` |
| [Tileset, Map, World](#tileset-map-world) | Tiled tilesets, maps and worlds | Levels authored in Tiled | `tiles.tsj`, `level.tmj`, `world.world` | `tilemap.LoadMap`, `LoadWorld`, `LoadTileset` | `tilemap.MapHandle`, ... |
| [Sound](#sound-and-music) | A short effect, decoded to memory | Hits, steps, clicks, UI | `sfx/hit.wav` | `sound.LoadSound` | `gfx.SoundHandle` |
| [Music](#sound-and-music) | A track, streamed while playing | Background music | `music/level1.ogg` | `sound.LoadMusic` | `gfx.MusicHandle` |
| [Font](#font) | A typeface at one pixel size | Labels, HUD, debug text | `font.ttf#size` | `render.LoadFont` | `gfx.FontHandle` |
| [Texture](#texture) | A whole image on the GPU | Rarely directly; sprites and atlases sit on top of it | `img.png` | `asset.Load[render.Texture]` | `render.TextureID` |
| [Font source](#font-source) | A parsed font file, no size | Never directly; fonts share it | `font.ttf` | `asset.Load[render.FontSource]` | `render.FontSourceID` |
| [Shader](#shader) | A compiled Kage shader | Sprite effects (`Sprite.Shader`), custom draw code | `fx.kage` | `render.LoadShader` | `gfx.ShaderHandle` |
| [Effect](#effect) | A particle effect with its sprite and shader | Sparks, smoke, glows (`Particles.Effect`) | `fx/sparks.fx.json` | `particles.LoadEffect` | `gfx.EffectHandle` |
| [Voice](#voices-and-dialogues) | How a character speaks, with its language and samples | `speech:Speaker.Voice` | `talk/bard.voice.json` or `chirpy` | `speech.LoadVoice` | `gfx.VoiceHandle` |
| [Dialogue](#voices-and-dialogues) | A conversation with its speakers' voices | `speech:DialogueBox.Dialogue` | `talk/village.dialogue.json` | `speech.LoadDialogue` | `gfx.DialogueHandle` |

Rule of thumb: **components hold sprites, clips and fonts**. You load atlases and clip sets to *find* sprites and clips by name. You never need textures or font sources unless you write a loader or your own renderer.

All examples assume the loaders are registered and a scope exists. `render.RegisterLoaders` panics if the types are already registered and must run on the game goroutine; `anim.RegisterLoaders` and `tilemap.RegisterLoaders` must come after it, because clip sets and tilesets depend on `render.Atlas`:

```go
render.RegisterLoaders(srv, render.LoaderOptions{})
anim.RegisterLoaders(srv)    // after render
tilemap.RegisterLoaders(srv) // after render
sound.RegisterLoaders(srv, ctx.SampleRate()) // any order; the rate of the audio.Context
sc := srv.Global()        // or a level scope: srv.NewScope("level1")
```

## Sprite

**What it is.** A rectangle of a texture that can be drawn: either a whole image file (`"ui/cursor.png"`) or one named frame of an atlas (`"chars/player.sheet.json#idle_0"`). It knows its size, its default pivot (`Origin`) and how much transparent border the packer trimmed.

**What it is used for.** Everything you see. A `gfx.Sprite` or `gfx.ScreenSprite` component holds a `gfx.SpriteHandle`; the renderer draws it at the entity's `Transform`.

```go
cursor := render.LoadSprite(sc, "ui/cursor.png")
coin := render.LoadSprite(sc, "items/items.sheet.json#3") // frame "3" of a grid sheet

ecs.NewEntity2(w, gfx.Transform{Pos: p, PrevPos: p}, gfx.Sprite{ID: coin, Layer: 1})
```

Until the file is loaded (or if it fails) the sprite draws a magenta placeholder. Use `render.SpriteStoreOf(srv).Lookup(h)` when you need to know whether it is the real image.

A `render.Sprite` value holds `Image` (the cached sub-image), `Texture`, `Rect`, `Origin` (default pivot, `0..1`), `Size` (untrimmed) and `Offset` (trim). Whole-image sprites have a center pivot. Never call `SubImage` per frame; the sprite caches it.

## Atlas

**What it is.** One image (a *sprite sheet*) plus a description of where each frame sits in it. The engine reads three formats: a **grid sheet** (`*.sheet.json`: fixed-size cells, named `"0"`, `"1"`, ... left to right, top to bottom), a single-image **Tiled tileset** (`*.tsj`: frames named by local tile ID) and **TexturePacker JSON** (named, possibly trimmed frames). Loading an atlas creates one sprite per frame.

**What it is used for.**
- **Tilesets.** A level's tiles in one image. Pick tiles by index with `Order[i]`.
- **Sprite sheets.** All frames of a character or of the items in one image, picked by name with `Sprites["name"]`.
- **UI skins.** Buttons and icons packed by TexturePacker.

It is also faster to draw: sprites from one atlas share a texture, so the renderer batches them.

```go
tiles, err := render.LoadAtlasSync(sc, "level1/tiles.sheet.json")
if err != nil {
	log.Fatal(err)
}
atlas := render.AtlasStoreOf(srv).Get(tiles)
grass := atlas.Order[0]          // gfx.SpriteHandle, by position
door := atlas.Sprites["door_open"] // by name (TexturePacker)
```

You don't need the atlas handle to draw one frame: `render.LoadSprite(sc, "level1/tiles.sheet.json#0")` loads the atlas behind the scenes. Load the atlas itself when you want to **enumerate or index** its frames. Frames are children of the atlas: a held frame keeps the atlas and its texture alive. An unknown frame name fails the asset. The two formats are described under [File formats](#file-formats).

## Clip

**What it is.** One animation: a list of frames, each a sprite plus a duration, and a loop mode (`Loop`, `Once`, `PingPong`). Clips come from Aseprite tags (`"chars/player.aseprite.json#run"`) or from an animation definition (`"chars/player.anim.json#run"`) that adds a pivot and hit/hurt boxes.

**What it is used for.** Animating an entity. Put the clip in a `gfx.Animator` next to a `gfx.Sprite`; the `AnimationSystem` advances time and writes the current frame's sprite into `Sprite.ID`.

```go
run := anim.LoadClip(sc, "chars/player.aseprite.json#run")
ecs.NewEntity3(w, gfx.Transform{Pos: p, PrevPos: p}, gfx.Sprite{Layer: 2},
	gfx.Animator{Clip: run, Speed: 1})
```

For gameplay, `anim.ShapesOf(anim.ClipStoreOf(srv), animator)` returns the hit and hurt boxes of the frame on screen, and `Animator.Frame` tells you which frame it is (for footsteps, attack timing).

## Clip set

**What it is.** All clips of one character file: one per Aseprite tag, by name, plus the atlas they draw from. Loaded from the Aseprite export (`"chars/player.aseprite.json"`) or an animation definition (`"chars/player.anim.json"`), which can also name a default clip.

**What it is used for.** A character that switches animations. Load the set once at spawn, keep its clip handles, and switch `Animator.Clip` when the state changes.

```go
setH, _ := anim.LoadClipSetSync(sc, "chars/player.anim.json")
set := anim.ClipSetStoreOf(srv).Get(setH)
idle, run := set.Clips["idle"], set.Clips["run"] // gfx.ClipHandle

// later, on state change:
a := ecs.GetComp[gfx.Animator](w, e)
a.Clip, a.Time, a.Frame, a.Done = run, 0, 0, false
```

`set.Names` lists the tags in file order (handy for debug menus), `set.Default` is the definition's `defaultClip`. To start an `*.anim.json` for an Aseprite export, run `go run ./tools/wpix anim create --file chars/player.json`; `wpix anim validate` checks one against its sheet (see [animation.md](animation.md#tooling-weakpixel anim)). If you only need one clip, `anim.LoadClip` with a `#tag` key is enough: it loads the set behind the scenes.

## Tileset, Map, World

**What they are.** The [Tiled](https://www.mapeditor.org/) JSON formats: a tileset (`*.tsj`), a map (`*.tmj`: layers of tiles, objects and images) and a world (`*.world`: maps placed side by side). A loaded `tilemap.Map` is read-only data: layers flattened in draw order, a GID table, objects and custom properties.

**What they are used for.** Levels drawn in Tiled. `game.Engine.LoadMap("world-1.world", tilemap.SpawnOptions{})` loads a map or world and spawns one entity per drawable layer when it is ready; `tilemap.TilemapExtract` draws them. Objects and properties are yours to interpret.

```go
h, err := tilemap.LoadMapSync(sc, "world-1/level-1.tmj")
m := tilemap.MapStoreOf(srv).Get(h)
_, player, _ := m.FindObject("player")
```

Everything about them, including what is supported: [tilemaps.md](tilemaps.md).

## Sound and music

**What they are.** Audio files. A `sound.Sound` is decoded completely on a worker to 16-bit stereo PCM at the engine's sample rate; a `sound.Music` keeps the compressed file and decodes while it plays.

**What they are used for.** Effects go into the `sound.Queue` or a `gfx.SoundEmitter`; music goes to `Queue.PlayMusic`, which crossfades. The placeholder is silence.

```go
hit := sound.LoadSound(sc, "sfx/hit.wav")
e.Sound().Play(sound.PlaySound{Sound: hit})
```

Everything about playing them: [audio.md](audio.md).

## Font

**What it is.** A typeface at one pixel size: `"fonts/ui.ttf#16"`. It holds the face that draws and measures text and its line height. Every size of the same file shares one parsed font source.

**What it is used for.** Text: a `gfx.Label` component holds a `gfx.FontHandle`. Load one handle per size you use.

```go
ui := render.LoadFont(sc, "fonts/ui.ttf#16")
ecs.NewEntity2(w, gfx.Transform{Pos: gfx.Vec2{X: 8, Y: 8}},
	gfx.Label{Text: "Score: 0", Font: ui, Layer: 10})
```

Every size of the same file parses it once: `"fonts/ui.ttf#12"` and `"fonts/ui.ttf#24"` share one [font source](#font-source). `Font.Face` draws and measures text, `Font.LineHeight` is the baseline distance. While loading, or if the file fails, text draws with Go Regular at 12 px.

## Texture

**What it is.** A whole image uploaded to the GPU, with its size and memory estimate. Every sprite and atlas depends on one.

**What it is used for.** Mostly nothing in game code: sprites and atlases load their texture for you and share it. Reach for it when you draw with Ebiten yourself, need a full image for a custom effect, or want GPU memory stats.

```go
th := asset.Load[render.Texture](sc, "bg/sky.png")
img := asset.StoreOf[render.Texture](srv).Get(th).Image // valid until the next Update
```

**Size limit.** A texture larger than the GPU's maximum texture size fails to load with `render: texture <w>x<h> is larger than this GPU supports` (the asset becomes `Failed`, shows the placeholder, and its dependents fail). The limit is the device's own: 16384 on most desktop GPUs, 8192 on many laptops, phones and browsers. Keep sheets at most 8192 on either side to run everywhere; `wpix anim validate` warns about larger Aseprite sheets.

## Font source

**What it is.** A parsed TTF or OTF file with no size.

**What it is used for.** Nothing in game code: fonts (`#size`) depend on it so one file is parsed once. Load it only if you build faces yourself.

## Shader

**What it is.** A compiled [Kage](https://ebitengine.org/en/documents/shader.html) shader from a `.kage` file. A compile error fails the asset with the compiler message (see `ShaderStore.Err` or `Options.OnError`). So does a shader that declares a uniform the renderer sets with another type (`var Params float` instead of `vec4`, or `Time` as anything but `float`), so that mismatch never panics while drawing.

**What it is used for.** Per-sprite effects (hit flashes, outlines, palette swaps): put the handle in `Sprite.Shader` and drive it with `Sprite.Params` ([shaders](shaders.md)). Layer and full-screen shaders are not built yet. In draw code you write with Ebiten, the zero handle and the placeholder have a nil `Shader` field: treat nil as "draw normally".

```go
flash := render.LoadShader(sc, "fx/flash.kage")
sh := render.ShaderStoreOf(srv).Get(flash).Shader // *ebiten.Shader or nil
```

## Effect

**What it is.** A particle effect from a `*.fx.json` file: one emitter's settings plus its sprite and shader, which load as dependencies. Format and keys: [particles.md](particles.md#the-effect-file).

**What it is used for.** Put the handle in `Particles.Effect` ([particles](particles.md)). A level or prefab names the file by its key; editing the file while the game runs updates playing effects ([hot reload](particles.md#hot-reload)). Register the loader with `particles.RegisterLoaders(srv)` after `render.RegisterLoaders`; the engine does it for you.

```go
fx := particles.LoadEffect(sc, "fx/sparks.fx.json")
```

## Voices and dialogues

**What they are.** A `speech.Voice` is a `*.voice.json` file of the [voice library](https://github.com/weakpixel/voice/blob/main/doc/guide.md) with its language (`*.lang.json`, a `speech.Language`) and the WAV samples its instruments name, which load as dependencies; a key that is a starter name (`"chirpy"`) loads the library's starter voice without a file. A `speech.Dialogue` is a `*.dialogue.json` file with the voices of its speakers.

**What they are used for.** Put the handles in `speech:Speaker.Voice` and `speech:DialogueBox.Dialogue` ([speech](speech.md)). `speech.Install` registers the loaders (or `speech.RegisterLoaders(srv)` without the rest of speech).

```go
v := speech.LoadVoice(sc, "talk/bard.voice.json")
d := speech.LoadDialogue(sc, "talk/village.dialogue.json")
```

## File formats

The loader is chosen by the asset type you load and, where one type reads several formats, by the file name.

| Asset | File | Format |
| --- | --- | --- |
| Texture | `*.png`, `*.jpg` | PNG or JPEG |
| Atlas | `*.sheet.json` | [Grid sheet](#grid-sheet) |
| Atlas | `*.tsj` | [Tiled tileset](#tiled-tileset-as-an-atlas) (single image only) |
| Atlas | any other `*.json` | [TexturePacker JSON](#texturepacker-json) (hash or array form) |
| Tileset (`tilemap.Tileset`) | `*.tsj` | [Tiled JSON tileset](tilemaps.md#what-is-supported) |
| Map (`tilemap.Map`) | `*.tmj` | [Tiled JSON map](tilemaps.md#what-is-supported) |
| World (`tilemap.World`) | `*.world` | Tiled world with a `maps` list |
| Clip set | `*.anim.json` | [Animation definition](animation.md#animation-definition-animjson): overrides, pivot, boxes over an Aseprite export |
| Effect | `*.fx.json` | [Particle effect](particles.md#the-effect-file) |
| Voice, Language, Dialogue | `*.voice.json`, `*.lang.json`, `*.dialogue.json` | [Voice library formats](https://github.com/weakpixel/voice/blob/main/doc/guide.md) |
| Clip set | any other `*.json` | [Aseprite sprite sheet export](animation.md#export-from-aseprite) (hash or array, "Meta: Tags") |
| Font source | `*.ttf`, `*.otf` | TrueType or OpenType |
| Shader | `*.kage` | Kage source |
| Sound, Music | `*.wav`, `*.ogg`, `*.mp3` | PCM WAV, Ogg Vorbis, MP3; resampled to the engine's rate |

### Grid sheet

`*.sheet.json`: fixed-size cells over one image. Frames are named `"0"`, `"1"`, ... left to right, top to bottom. `wpix-editor sheet` [edits the file](editor.md#the-sheet-tool).

```json
{ "image": "player.png", "frameW": 16, "frameH": 16, "margin": 0, "spacing": 0,
  "pivot": { "x": 0.5, "y": 1 } }
```

### Tiled tileset as an atlas

A single-image `*.tsj` loaded as an atlas is cut by `tilewidth`/`tileheight`, `margin`, `spacing` and `columns` into `tilecount` frames named by local tile ID (`"tiles/temple.tsj#92"`), row by row; `Order[i]` is local tile `i`. `"image"` is relative to the tileset file. An image collection tileset has no single image and fails as an atlas; load it as a `tilemap.Tileset`.

### TexturePacker JSON

Any other `*.json` loaded as an atlas, hash or array form. Trim and pivot data are honored. `"image"` is relative to the JSON file's directory.

### Placeholders in release builds

`render.LoaderOptions{TransparentPlaceholders: true}` makes missing, loading and failed assets draw **nothing** instead of the magenta checker. Use it in release builds; keep the checker in development so gaps are visible. Fonts still fall back to Go Regular at 12 px.

## Which kinds can be authored in levels (GOD)

Component fields marked `+game:asset` are written as keys in prefab and level JSON, and `resolver` turns them into handles. Eight kinds exist: `sprite` (`Sprite.ID`), `clip` (`Animator.Clip`), `font` (`Label.Font`), `shader` (`Sprite.Shader`), `sound` (`SoundEmitter.Sound`), `effect` (`Particles.Effect`), `music` and `prefab` (the last two have no `gfx` field; use them in your own components). Package `speech` adds `voice` and `dialogue` ([speech](speech.md)). See [game-objects.md](game-objects.md) and [kinds.md](kinds.md#asset-kinds).

## Your own kinds

Dialog trees, level data and sound banks are asset types too. Register a loader with `asset.Register[T, D]` and use the generic API (`asset.Load[T]`, `asset.StoreOf[T]`). If components should hold its handles, give it a concrete handle and a small store facade like `render.SpriteStore` (see [assets.md](assets.md#game-facing-handles)).
