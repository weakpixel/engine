# Kinds reference

Everything in the engine that comes in "kinds", in one place.

## Asset kinds

The kinds an asset field can name in `+game:asset:kind=...`, interpreted by `resolver`. The package that owns an asset type registers its kinds in `assetkind` and exports the name: `render.AssetKindSprite`, `render.AssetKindFont`, `render.AssetKindShader`, `anim.AssetKindClip`, `sound.AssetKindSound`, `sound.AssetKindMusic`, `particles.AssetKindEffect`, `prefab.AssetKind`, `speech.AssetKindVoice`, `speech.AssetKindDialogue`. A kind exists in a program that links its owner: an `Engine` links all of them except `speech`, which a game adds with `speech.Install`. Without the owner, a field of that kind fails with `unknown asset kind "effect"`. How to add a kind: [game-objects.md](game-objects.md#custom-kinds).See [game-objects.md](game-objects.md#asset-kinds).

| Kind | Loads | Go asset type | Component field | Handle | Key example |
| --- | --- | --- | --- | --- | --- |
| `sprite` | whole image or atlas frame | `render.Sprite` | `Sprite.ID`, `Sprite.Aux` | `gfx.SpriteHandle` | `"ui/cursor.png"`, `"chars/player.aseprite.json#idle_0"` |
| `clip` | Aseprite tag (from the export or an `*.anim.json`) | `anim.Clip` | `Animator.Clip` | `gfx.ClipHandle` | `"chars/player.aseprite.json#run"` |
| `font` | font at a size | `render.Font` | `Label.Font` | `gfx.FontHandle` | `"fonts/ui.ttf#16"` |
| `shader` | compiled Kage shader | `render.Shader` | `Sprite.Shader`, `Label.Shader`, `NineSlice.Shader` | `gfx.ShaderHandle` | `"fx/flash.kage"` |
| `sound` | effect, decoded to PCM | `sound.Sound` | `SoundEmitter.Sound` | `gfx.SoundHandle` | `"sfx/hit.wav"` |
| `music` | track, streamed | `sound.Music` | none in `gfx` (use it in your own components) | `gfx.MusicHandle` | `"music/level1.ogg"` |
| `effect` | particle effect (`*.fx.json`) | `particles.Effect` | `Particles.Effect` | `gfx.EffectHandle` | `"global/fx/sparks.fx.json"` |
| `prefab` | prefab file (`*.prefab.json`) | `prefab.Prefab` | none in `gfx` (use it in your own components, for example a spawner) | `gfx.PrefabHandle` (same type as `prefab.Handle`) | `"global/prefabs/goblin.prefab.json"` |
| `voice` (package `speech`) | voice file or starter voice | `speech.Voice` | `speech:Speaker.Voice` | `gfx.VoiceHandle` | `"talk/bard.voice.json"`, `"chirpy"` |
| `dialogue` (package `speech`) | dialogue file (`*.dialogue.json`) | `speech.Dialogue` | `speech:DialogueBox.Dialogue` | `gfx.DialogueHandle` | `"talk/village.dialogue.json"` |

A `prefab` field resolves without blocking, so check `prefab.StoreOf(srv).Ready(h)` before `Spawn`. The level scope holds the prefab. A prefab whose own fields name itself, directly or through other prefabs, is never released, because cycles are not detected.

Every asset type, its key and its file formats: [asset-kinds.md](asset-kinds.md).

## Asset states (`asset.State`)

`Missing`, `Queued`, `Decoding`, `Waiting`, `Ready`, `Failed`. Only `Ready` returns the real value; everything else gives the placeholder.

## Event kinds (`asset.EventKind`)

| Kind | Meaning |
| --- | --- |
| `EventLoaded` | committed, now `Ready` |
| `EventFailed` | failed, see `Event.Err` |
| `EventModified` | hot reload replaced the value; same handle, `Version` + 1 |
| `EventUnloaded` | unloaded, handles are now stale |

## Item kinds (`render.Kind`)

| Kind | Draws | Uses |
| --- | --- | --- |
| `KindSprite` | a sprite | `Sprite`, `Scale`, `Origin`, `FlipX/Y`, `Color` (tint), `Shader`, `Params`, `Aux` ([shaders](shaders.md)) |
| `KindRect` | a 1-px rect outline (top-left at `Pos`) | `Size`, `Color` |
| `KindLine` | a line from `Pos` | `Size` (end offset), `Color` |
| `KindCircle` | a circle outline: an antialiased ring 1 pixel wide, centered on the radius, smooth at any size; a draw call of its own | `Size.X` (radius, times `Scale.X`), `Color` |
| `KindCanvas` | a pre-rendered image ([canvases](rendering.md#canvases)) | `Canvas`, `Scale`, `Origin` (zero: top-left), `FlipX/Y`, `Color` (tint) |
| `KindText` | text | `Text`, `Font`, `Color` |

Fields a kind doesn't use are ignored. Note: `Item.Scale` is used as is (extract resolves "zero means 1"); a hand-built item with zero `Scale` has size zero. `Queue.Sprite` sets it to `(1, 1)`.

## Shape kinds (`gfx.ShapeKind`)

| Kind | `Size` means |
| --- | --- |
| `ShapeRect` | width, height |
| `ShapeCircle` | radius in `X` |
| `ShapeLine` | end offset |

## Collider shapes (`collision.ShapeKind`)

| Kind | `Size` means |
| --- | --- |
| `ShapeRect` (0) | width, height; `Offset` is the top-left corner |
| `ShapeCircle` (1) | radius in `X`; `Offset` is the center |
| `ShapeAnimBody` (2) | a rect written from the anim `body` ([collision.md](collision.md#the-collider-from-the-animation-body)) |
| `ShapeCustom` (128) and up | the game's own, registered with `Space.RegisterShape` |

One-way faces (`collision.OneWay`): `OneWayNone`, `OneWayUp`, `OneWayDown`, `OneWayLeft`, `OneWayRight` (the blocking face).

## Body kinds (`physics.BodyKind`)

`Dynamic` (0): moved by gravity, velocity and contacts. `Kinematic`: moved only by its velocity, never pushed. See [physics](physics.md#body).

## Emit shapes (`gfx.EmitShape`)

| Shape | `ShapeSize` means |
| --- | --- |
| `EmitPoint` | unused: particles spawn at the emitter |
| `EmitCircle` | radius in `X` |
| `EmitRect` | width, height |

See [particles.md](particles.md).

## Spaces (`render.Space`)

| Space | Coordinates | Camera |
| --- | --- | --- |
| `WorldSpace` | world units | applies |
| `ScreenSpace` | logical pixels (HUD, UI, labels) | ignored; not culled |

## Blend modes (`render.Blend`)

The same values exist as `gfx.Blend` (`Sprite.Blend`, the `blend` key of an effect file; `render.Blend(b)` converts). In documents only these values validate; in Go, an unknown `render.Blend` draws as `BlendNormal`.

| Value | Mode | Result | Use |
| --- | --- | --- | --- |
| 0 | `BlendNormal` (default) | source over destination | everything |
| 1 | `BlendAdditive` | dst + src | glows, fire, sparks |
| 2 | `BlendMultiply` | dst * src | light maps, shadows ([layer groups](shaders.md#light-maps)) |
| 3 | `BlendScreen` | src + dst * (1 - src) | soft glows that never exceed white, flashes |
| 4 | `BlendSubtract` | dst - src (alpha unchanged) | darkening tints, negative light |
| 5 | `BlendLighten` | max(src, dst) | overlapping glows that do not add up |
| 6 | `BlendErase` | dst * (1 - src alpha) | holes in a layer group: [fog of war](shaders.md#fog-of-war-and-flashlights) |

Colors are premultiplied, so a transparent source pixel leaves the destination unchanged in every mode but Erase (which erases by alpha). There is no Darken: Ebiten's min operation ignores alpha and would blacken what is below the transparent parts of a sprite. Overlay, soft light and similar modes need the destination inside a formula; use a [`SampleBelow` group shader](shaders.md#group-shaders).

## Camera follow modes (`camera.FollowMode`)

`default` (also `""`), `smooth`, `deadzone`, `spring`, `lookahead`. They are presets of the follow pipeline; `DeadZone.On` and `LookAhead.On` combine stages with any of them. What each does and its parameters: [camera.md](camera.md#modes).

## Loop modes (`anim.LoopMode`)

| Mode | At the last frame |
| --- | --- |
| `Loop` | restart |
| `Once` | stop on the last frame; `Animator.Done` is set. Comes from `"loop": false` in an `*.anim.json` or from clips you build yourself; the Aseprite export alone never produces it |
| `PingPong` | play backward, then forward again, without repeating the end frames |

## Box kinds (`gfx.BoxKind`)

| Kind | `*.anim.json` `kind` | Meaning |
| --- | --- | --- |
| `BoxHurt` | `"hurt"` | where the entity can be hit (also the `body` boxes) |
| `BoxHit` | `"hit"` | where the entity deals a hit |

See [animation.md](animation.md#animation-definition-animjson).

## Tilemap asset keys

| Asset type | Key | Handle |
| --- | --- | --- |
| `tilemap.Tileset` | `"tiles/temple.tsj"` | `tilemap.TilesetHandle` |
| `tilemap.Map` | `"level-1.tmj"` | `tilemap.MapHandle` |
| `tilemap.World` | `"world-1.world"` | `tilemap.WorldHandle` |

They are not GOD asset kinds: maps are loaded with `game.Engine.LoadMap`, not referenced from Game Objects. See [tilemaps.md](tilemaps.md).

## Map layer kinds (`tilemap.LayerKind`)

| Kind | Tiled type | Spawns a `TileLayer` | Data |
| --- | --- | --- | --- |
| `Tiles` | `tilelayer` | yes | `Origin`, `W`, `H`, `GIDs` |
| `Objects` | `objectgroup` | no | `Objects`, `DrawOrder` |
| `Image` | `imagelayer` | yes | `Image`, `ImageSize`, `RepeatX/Y` |
| `Group` | `group` | no | its children follow it in `Map.Layers` |

## Object shapes (`tiled.Shape`)

`ShapeRect`, `ShapePoint`, `ShapeEllipse`, `ShapePolygon`, `ShapePolyline` (`Points` relative to `X`, `Y`), `ShapeText` (`Text`), `ShapeTile` (`GID`, `Flip`; `X`, `Y` is the bottom-left corner).

## Tile flip flags (`tiled.Flip`)

| Flag | GID bit | Meaning |
| --- | --- | --- |
| `FlipH` | `0x80000000` | mirror horizontally |
| `FlipV` | `0x40000000` | mirror vertically |
| `FlipD` | `0x20000000` | transpose (applied first); with H/V it makes 90° rotations |
| `FlipHex` | `0x10000000` | hexagonal 120° rotation; ignored |

`tiled.SplitGID(raw)` returns the GID and the flags, `tiled.JoinGID` the reverse.

## Render orders (`tilemap.RenderOrder`)

`RightDown` (default), `RightUp`, `LeftDown`, `LeftUp`: the order `TilemapExtract` queues a layer's cells (rows top-down or bottom-up, cells left-to-right or right-to-left).

File formats of atlases, clip sets and maps: [asset-kinds.md](asset-kinds.md#file-formats).

## Audio buses (`sound.Bus`)

| Bus | For |
| --- | --- |
| `BusSFX` (zero) | gameplay effects: `Queue.Play`, `SoundEmitter` |
| `BusMusic` | music; `Queue.PlayMusic` always uses it |
| `BusUI` | interface sounds |
| `BusVoice` | dialogue; package `speech` plays on it |
| 4 to 7 | yours; `NumBuses` is 8, higher values count as `BusSFX` |

## Speech events (`speech.EventKind`)

| Kind | From | When |
| --- | --- | --- |
| `Started`, `Completed`, `Skipped`, `Stopped` | `Speech.Say` | a line began, played to its end, was skipped, was stopped or replaced |
| `LineStarted`, `LineDone`, `DialogueFinished` | a `speech:DialogueBox` | a line began to speak, a line is done, the dialogue gave way after its last line |

See [speech.md](speech.md#events).

## Audio formats (`sound.Format`)

| Format | Extension |
| --- | --- |
| `FormatWAV` | `.wav` (16-bit PCM) |
| `FormatOgg` | `.ogg` (Vorbis) |
| `FormatMP3` | `.mp3` |
