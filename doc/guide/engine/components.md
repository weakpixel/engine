# Components (`gfx`)

Plain-data components. No Ebiten import, so they serialize and tools never compile a renderer. (`gfx` imports `ecs` only through its generated Game Object code.)

| Component | Space | Drawn by | Game Object |
| --- | --- | --- | --- |
| `Transform` | | all | yes |
| `Sprite` | world | `SpriteExtract` | yes |
| `Animator` | | `anim.AnimationSystem` | yes |
| `ScreenSprite` | screen | `ScreenSpriteExtract` | no |
| `Label` | screen | `LabelExtract` | yes (object `Text`) |
| `DebugShape` | world | `DebugExtract` | no |
| `SoundEmitter` | world (pans by `Transform`) | `sound.AudioSystem` (plays it) | yes (object `SoundSource`) |
| `Particles` | world | `particles.System` (simulates), `particles.Extract` (draws) | yes (object `ParticleSource`) |
| `PlayerSlot` | | game systems (read input) | standalone component |

```go
e := ecs.NewEntity2(w,
	gfx.Transform{Pos: gfx.Vec2{X: 100, Y: 50}},
	gfx.Sprite{ID: hero, Layer: 2, Origin: gfx.Vec2{X: 0.5, Y: 1}},
)
```

## Zero values draw normally

| Field | Zero means |
| --- | --- |
| `Transform.Scale` | `(1, 1)` |
| `Color` / `Sprite.Tint` / `Label.Color` | opaque white (no tint) |
| `Sprite.Origin` | the sprite's own default pivot (usually center). Written per frame by the `AnimationSystem` if the clip has a pivot |
| `Sprite.ID`, `Animator.Clip`, `Label.Font`, `SoundEmitter.Sound` | none: placeholder / untouched / silent |
| `Sprite.Shader` | no shader: the sprite draws normally |
| `SoundEmitter.Volume` | 1 (negative is silent) |
| `SoundEmitter.Bus` | `sound.BusSFX` |
| `Particles` fields | zero values: see [particles.md](particles.md#the-component); the effect itself has defaults per key, see [the effect file](particles.md#the-effect-file) |
| `Animator.Speed` | 1 (negative also means 1) |
| `Camera.Zoom` | 1 |

Top-left pivot: use a tiny non-zero origin, `gfx.Vec2{X: 0.0001, Y: 0.0001}`.

## Transform

```go
gfx.Transform{Pos: p, PrevPos: p, Rot: 0, Scale: gfx.Vec2{X: 2, Y: 2}}
```

`PrevPos` is filled by `extract.SnapshotTransforms` each tick. An entity spawned mid-tick has `PrevPos` zero until the next tick, so **spawn with `PrevPos = Pos`** to avoid a one-frame slide from the origin. `Rot` is radians, clockwise on screen.

## Sprite

| Field | Meaning |
| --- | --- |
| `ID SpriteHandle` | the image (asset kind `sprite`) |
| `Layer int16` | draw order, higher on top |
| `Origin Vec2` | pivot as a fraction of the sprite size. A clip with a pivot (`*.anim.json`) owns it: the `AnimationSystem` overwrites it every tick |
| `FlipX`, `FlipY` | mirror |
| `Tint Color` | multiplies the image color |
| `Blend Blend` | how the sprite lands on what is below (`gfx.Blend`, the same values as `render.Blend`, see [kinds](kinds.md#blend-modes-renderblend)). Zero is normal. Give blended sprites a layer of their own so they batch |
| `Shader ShaderHandle` | draws the sprite through a Kage shader (asset kind `shader`); see [shaders](shaders.md) |
| `Params [4]float32` | the shader's `Params` vec4 uniform. Optional in GOD files (absent means zeros). Animate it rather than swapping shaders |
| `Hidden bool` | skip drawing. A field, not a tag, so toggling it does not move the entity between archetypes |

```go
sp := gfx.Sprite{ID: render.LoadSprite(sc, "ui/cursor.png")}
sp.Tint = gfx.RGBA(255, 120, 120, 255)
```

`ScreenSprite{Sprite: sp}` is the same, but `Transform.Pos` is in logical screen pixels, the camera is ignored and there is no culling.

## NineSlice

A panel image that resizes without distorting its border. The image is cut by four insets into nine parts: the corners keep their pixel size, the edges stretch along one axis and the center stretches along both (or repeats, with `Tile`). One 24x24 panel then serves a dialog, a button and a health-bar frame.

| Field | Meaning |
| --- | --- |
| `ID SpriteHandle` | the image (asset kind `sprite`) |
| `Insets Insets` | `Left`, `Top`, `Right`, `Bottom` border widths in pixels of the image. All zero stretches the whole image |
| `Size Vec2` | the drawn size in pixels, before `Transform.Scale`. Zero (per axis) means the image's own size |
| `Layer int16` | draw order, as for `Sprite` |
| `Origin Vec2` | pivot as a fraction of `Size`. Zero is the top-left corner (not the sprite default) |
| `Tint Color`, `Blend Blend` | as for `Sprite` |
| `Tile bool` | repeat the edges and the center at the image's pixel size instead of stretching them; the last repeat in a row or column is cropped |
| `Hidden bool` | skip drawing |

`Transform.Scale` multiplies everything, corners included (pixel-art upscaling), and `Transform.Rot` rotates the panel around its origin. Insets larger than the image are clamped to it. When `Size` is smaller than the borders together, the borders shrink in proportion and the center is empty. Insets are measured on the frame as stored, so pack nine-slice art untrimmed.

A panel is one batch: its nine pieces come from one texture. Resize it by writing `Size`, which does not move the entity between archetypes.

```go
panel := render.LoadSprite(sc, "ui/panel.png")
box := ecs.NewEntity2(w, gfx.Transform{Pos: gfx.Vec2{X: 16, Y: 260}}, gfx.ScreenNineSlice{NineSlice: gfx.NineSlice{
	ID: panel, Insets: gfx.Insets{Left: 6, Top: 6, Right: 6, Bottom: 6}, Size: gfx.Vec2{X: 120, Y: 32}, Layer: 100,
}})
// Later, e.g. a typewriter effect: just change the size.
ecs.GetComp[gfx.ScreenNineSlice](w, box).NineSlice.Size.X = 200
```

`ScreenNineSlice{NineSlice: ns}` is the same in screen space (HUD, UI): `Transform.Pos` is in logical screen pixels, the camera is ignored and there is no culling. In a world, a `NineSlice` is culled with its size taken into account, so a panel whose pivot is off screen but whose body is visible is kept; a panel with a zero `Size` has nothing to add, so grow `NineSliceExtract.Margin` for those. The `gfx:Panel` object (`Transform`, `NineSlice`) places one in a level; the inspector shows `Insets` and `Size` as number fields and `ID` as a sprite asset field.

## Label

```go
font := render.LoadFont(sc, "fonts/ui.ttf#16")
ecs.NewEntity2(w, gfx.Transform{Pos: gfx.Vec2{X: 8, Y: 8}}, gfx.Label{Text: "Score: 0", Font: font, Layer: 10})
```

Drawn in screen space, not culled. Text may allocate (Ebiten's layout).

## DebugShape

Collider and path visualization in world space. See [shape kinds](kinds.md#shape-kinds-gfxshapekind).

```go
gfx.DebugShape{Kind: gfx.ShapeCircle, Size: gfx.Vec2{X: 10}, Color: gfx.RGBA(0, 255, 0, 255)}
```

## Animator

```go
an := gfx.Animator{Clip: anim.LoadClip(sc, "chars/player.aseprite.json#run"), Speed: 1}
```

`Time`, `Frame`, `Done` are written by the system. Restart a clip: set `Clip`, zero `Time`, `Frame`, `Done`. See [animation.md](animation.md).

## SoundEmitter

```go
em := gfx.SoundEmitter{Sound: sound.LoadSound(sc, "sfx/waterfall.ogg"), Loop: true, Volume: 0.6}
```

One-shot (`Loop` off) plays once and sets `Done`, and keeps playing if the entity dies; a looping emitter plays while the entity has it. `Voice` and `Done` are written by the system. See [audio.md](audio.md#soundemitter).

## Particles

```go
fx := particles.LoadEffect(sc, "global/fx/sparks.fx.json")
p := gfx.Particles{Effect: fx, Layer: 5}
```

Plays a particle effect (a `*.fx.json` asset) at the entity: spawns, moves and fades particles; the particles are pool entries, not entities. See [particles.md](particles.md).

## PlayerSlot

`gfx.PlayerSlot{Player: 1}` says which input player controls the entity: the index for `Router.Player`. A game's controller system reads that player's action state for every entity with the component, so one system drives every player's character. It is a standalone `+game:component`: mark a `*gfx.PlayerSlot` field of your own objects as a component to author it. See [input.md](input.md#the-controller-pattern).

## Collision components

Package `collision` (GOD group `collision`) has `collision:Collider`: a rect, circle or anim-body shape with the layers it is on and the layers it reacts to. See [collision.md](collision.md#collider).

## Room components

Package `room` (GOD group `room`) has its own components: `room:Exit`, `room:Spawn` and the tag `room:Traveler`. See [rooms.md](rooms.md#exits-spawns-and-travelers).

## Speech components

Package `speech` (GOD group `speech`) has `speech:Speaker` (an entity that speaks with `Say`) and `speech:DialogueBox` (an entity that plays a dialogue file), with the objects `speech:Talker` and `speech:Box`. See [speech.md](speech.md).

## Value types

```go
v := gfx.Vec2{X: 1, Y: 2}
v.Add(o); v.Sub(o); v.Mul(2); v.Lerp(o, 0.5)

r := gfx.Rect{Min: gfx.Vec2{}, Max: gfx.Vec2{X: 10, Y: 10}}
r.Contains(v)      // Min inclusive, Max exclusive
r.Grow(4)          // negative shrinks

c := gfx.RGBA(255, 0, 0, 255)   // packed 0xRRGGBBAA, non-premultiplied
gfx.White; gfx.Black
red, g, b, a := c.Floats()      // [0,1]; the zero Color returns opaque white
```

A fully transparent color is any value with alpha 0 but other bits set, e.g. `gfx.RGBA(255, 255, 255, 0)`.

## Boxes

Hit and hurt boxes of animation frames (from an `*.anim.json`, see [animation.md](animation.md#animation-definition-animjson)). Plain data, not components.

```go
b := gfx.Box{Kind: gfx.BoxHit, ID: "slash", Rect: gfx.IRect{X: -17, Y: -2, W: 35, H: 17}} // relative to the pivot, pixels
r := b.World(&transform, sprite.FlipX, sprite.FlipY) // gfx.Rect in world units; rotation ignored
```

`IRect{X, Y, W, H int32}` is an integer rect (top-left, y down). `BoxKind` is `BoxHurt` or `BoxHit` ([kinds](kinds.md#box-kinds-gfxboxkind)). `World` mirrors around the pivot for flips (`x' = -(x + w)`), honors `Transform.Scale` (zero means 1) and does not allocate.

## Handles

`gfx.SpriteHandle`, `gfx.FontHandle`, `gfx.ClipHandle`, `gfx.ShaderHandle`, `gfx.SoundHandle` and `gfx.MusicHandle` are the handle types components hold: 8-byte, comparable and pointer-free. The zero value means "none". Load them with `render.LoadSprite`, `render.LoadFont`, `anim.LoadClip`, `render.LoadShader`, `sound.LoadSound` and `sound.LoadMusic`; see [Game-facing handles](assets.md#game-facing-handles).
