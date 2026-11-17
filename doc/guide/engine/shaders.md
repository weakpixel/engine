# Shaders

A sprite can be drawn through a [Kage](https://ebitengine.org/en/documents/shader.html) shader: a small program that runs once per pixel of the sprite and decides its color. Hit flashes, outlines, palette swaps, dissolves and "frozen" looks are a few lines each.

Shaders apply in four places: **per sprite**; **per layer group**, a range of layers drawn into an offscreen image that is composited through a shader (water, fog, desaturating the world) or with a blend mode (light maps); **post-processing**, a chain of passes over the whole finished frame (vignette, screen flash, color grading); and a **final pass** at window resolution (CRT scanlines).

Contents: [Quick start](#quick-start) · [How a sprite shader works](#how-a-sprite-shader-works) · [Examples](#examples) · [Driving Params](#driving-params) · [Time](#time) · [Tile layers](#tile-layers) · [Particles](#particles) · [Text, panels and shapes](#text-panels-and-shapes) · [A second image](#a-second-image) · [Layer groups](#layer-groups) · [Post-processing](#post-processing) · [Loading and errors](#loading-and-errors) · [Performance](#performance) · [Limits](#limits) · [API reference](#api-reference)

## Quick start

Three steps: write a `.kage` file, load it, and put the handle on a sprite.

`assets/fx/flash.kage`:

```kage
//kage:unit pixels

package main

// Params is Sprite.Params. x: how white the sprite is, 0..1.
var Params vec4

func Fragment(dst vec4, src vec2, color vec4) vec4 {
	c := imageSrc0At(src)
	return mix(c, vec4(c.a), Params.x) * color
}
```

Go:

```go
// Hit makes an entity flash white and fade back.
type Hit struct {
	Left float32 // seconds of flash left
}

const flashTime = 0.15

// newFlashSystem turns Hit.Left into the shader's Params.x.
func newFlashSystem(w *ecs.World) system.System {
	hits := ecs.NewView2[Hit, gfx.Sprite](w)
	return func(ctx *system.Context) {
		dt := float32(ctx.Time.Delta.Seconds())
		for _, c := range hits.Each() {
			hit, sp := c.Unpack()
			hit.Left = max(hit.Left-dt, 0)
			sp.Params[0] = hit.Left / flashTime
		}
	}
}

e := game.New(game.Config{FS: assets})
e.AddSystem(game.StageSimulate, newFlashSystem(e.World()))
e.Start(func(e *game.Engine) {
	sc := e.Assets().Global()
	flash := render.LoadShader(sc, "fx/flash.kage")
	hero := render.LoadSprite(sc, "chars/player.png")
	p := gfx.Vec2{X: 100, Y: 80}
	ecs.NewEntity3(e.World(),
		gfx.Transform{Pos: p, PrevPos: p},
		gfx.Sprite{ID: hero, Shader: flash},
		Hit{})
})
```

When the hero takes damage: `ecs.GetComp[Hit](w, hero).Left = flashTime`.

The shader handle stays on the sprite the whole time. With `Params.x` at 0 the shader draws the sprite unchanged, so "turning the effect on" is just writing a number. That never moves the entity between archetypes.

In a level or prefab file the same sprite is:

```json
"gfx:Sprite": { "ID": "chars/player.png", "Shader": "fx/flash.kage", "Params": [0, 0, 0, 0] }
```

`Shader` is an asset key (kind `shader`) and `Params` is optional: absent means four zeros.

## How a sprite shader works

The renderer draws the sprite's rectangle with your shader instead of copying the image. Position, rotation, scale, flips, pivot, interpolation and the blend mode are exactly as for an unshaded sprite. `Fragment` runs for every pixel of that rectangle and returns its color.

| Input | What it is |
| --- | --- |
| `src` (2nd argument) | Where this pixel is in the sprite's image, in **texture pixels**. Pass it to `imageSrc0At`. |
| `imageSrc0At(pos)` | The sprite image's color at `pos`, or transparent outside the sprite's frame (so neighbors of an edge pixel read as empty). |
| `imageSrc0Origin()`, `imageSrc0Size()` | Top-left corner and size of the sprite's frame inside its texture. `src - imageSrc0Origin()` is the position inside the frame, `(0,0)` at its top-left. |
| `color` (3rd argument) | `Sprite.Tint` (`Item.Color`) as a premultiplied color. **Multiply your result by it**, or tinting and fading stop working for this sprite. |
| `var Params vec4` | `Sprite.Params` (`Item.Params`). `Params.x` is `Params[0]`, and so on. Declare it only if you use it. |
| `var Time float` | The frame's time in seconds, the same for every shader in the frame (see [Time](#time)). Declare it only if you use it. |
| `dst` (1st argument) | The pixel's position on screen. Rarely needed. |

Rules:

- **Start every file with `//kage:unit pixels`.** Without it `src` is in texels and the examples here sample the wrong place.
- **Colors are premultiplied.** `imageSrc0At` returns `rgb` already multiplied by `a`, and so must your result: a half-transparent white is `vec4(0.5, 0.5, 0.5, 0.5)`, not `vec4(1, 1, 1, 0.5)`. Divide by `a` before comparing colors (see [palette swap](#palette-swap)).
- **`Params` must be `vec4` and `Time` must be `float`.** A shader that declares either with another type fails to load (see [errors](#loading-and-errors)).
- **No other uniforms are set.** You can declare others, but they stay zero. Put constants in the code.
- **Atlas frames work as is.** `imageSrc0` is the sprite's own frame, and `imageSrc0At` returns transparent outside it, so a shader cannot bleed into the neighboring frame.

## Examples

Each example is a complete `.kage` file. A test (`TestShaderGuideExamplesLoad` in `render`) loads every `kage` block on this page, so they all compile and pass the uniform check.

### Hit flash

The quick-start shader: blend towards white by `Params.x`. `vec4(c.a)` is premultiplied white with the pixel's own alpha, so transparent pixels stay transparent.

```kage
//kage:unit pixels

package main

// Params.x: 0 = normal, 1 = white.
// +shader:param:slot=x,label=Flash,min=0,max=1
var Params vec4

func Fragment(dst vec4, src vec2, color vec4) vec4 {
	c := imageSrc0At(src)
	return mix(c, vec4(c.a), Params.x) * color
}
```

### Solid color (silhouette, team color)

Fill the sprite's shape with one color. Good for "behind a wall" silhouettes, a red damage flash or a team color.

```kage
//kage:unit pixels

package main

// Params.rgb: fill color. Params.a: amount, 0 = normal, 1 = solid fill.
var Params vec4

func Fragment(dst vec4, src vec2, color vec4) vec4 {
	c := imageSrc0At(src)
	fill := vec4(Params.rgb, 1) * c.a
	return mix(c, fill, Params.a) * color
}
```

`Params: [4]float32{1, 0, 0, 0.6}` tints a sprite 60% towards solid red.

### Outline

Draw a one-pixel outline around the opaque part of the sprite: an empty pixel that has an opaque neighbor becomes the outline color.

```kage
//kage:unit pixels

package main

// Params.rgb: outline color. Params.a: 1 = show the outline, 0 = hide it.
var Params vec4

func Fragment(dst vec4, src vec2, color vec4) vec4 {
	c := imageSrc0At(src)
	if c.a > 0 || Params.a == 0 {
		return c * color
	}
	n := imageSrc0At(src+vec2(1, 0)).a + imageSrc0At(src-vec2(1, 0)).a +
		imageSrc0At(src+vec2(0, 1)).a + imageSrc0At(src-vec2(0, 1)).a
	if n == 0 {
		return vec4(0)
	}
	return vec4(Params.rgb, 1) * Params.a * color
}
```

The shader can only paint inside the sprite's rectangle. The outline therefore needs at least one transparent pixel of padding around the art in the frame. Atlases trimmed by TexturePacker or Aseprite cut that padding away: export with a 1-pixel border ("extrude" or "border padding") for outlined sprites.

### Palette swap

Replace one color of the art by another, for example to give enemies different armor colors from one sprite sheet. The art uses a key color that never appears elsewhere.

```kage
//kage:unit pixels

package main

// Params.rgb: the color that replaces the key color.
var Params vec4

func Fragment(dst vec4, src vec2, color vec4) vec4 {
	key := vec3(1, 0, 1) // magenta in the art
	c := imageSrc0At(src)
	if c.a > 0 && distance(c.rgb/c.a, key) < 0.1 {
		c = vec4(Params.rgb*c.a, c.a)
	}
	return c * color
}
```

`c.rgb/c.a` undoes the premultiplication before comparing, and `Params.rgb*c.a` redoes it. For several swaps, use several key colors and spread the targets over more sprites, or wait for texture palettes (a lookup texture), which need more inputs than `Params` offers.

### Dissolve

Make the sprite crumble away pixel by pixel as `Params.x` goes from 0 to 1, for deaths and teleports.

```kage
//kage:unit pixels

package main

// Params.x: 0 = whole sprite, 1 = gone.
var Params vec4

// hash returns a stable pseudo-random value in [0,1) for a pixel position.
func hash(p vec2) float {
	return fract(sin(dot(p, vec2(12.9898, 78.233))) * 43758.5453)
}

func Fragment(dst vec4, src vec2, color vec4) vec4 {
	local := floor(src - imageSrc0Origin())
	if hash(local) < Params.x {
		return vec4(0)
	}
	return imageSrc0At(src) * color
}
```

The noise uses the position inside the frame (`src - imageSrc0Origin()`), not the texture position, so the pattern is the same for every frame of an animation and does not jump between frames.

### Desaturate (frozen, stunned, disabled)

Fade the sprite to gray. Combine it with a light blue `Tint` for a frozen look.

```kage
//kage:unit pixels

package main

// Params.x: 0 = full color, 1 = gray.
var Params vec4

func Fragment(dst vec4, src vec2, color vec4) vec4 {
	c := imageSrc0At(src)
	g := dot(c.rgb, vec3(0.299, 0.587, 0.114))
	return mix(c, vec4(g, g, g, c.a), Params.x) * color
}
```

Because the colors are premultiplied, `g` already carries the pixel's alpha. No extra multiplication is needed.


## Driving Params

`Params` is plain data on the `Sprite` component, so anything that can change a component can drive an effect:

- **A Go system**, as in the [quick start](#quick-start): keep the effect's state (time left, target color) in your own component and write `Params` from it every tick.
- **Lua**: `Params` is part of the `Sprite` schema. Arrays reach Lua as a copied table, so assign the whole table (`Params = {1, 0, 0, 0}`); changing one element of the copy changes nothing.
- **Level and prefab files** set initial values (`"Params": [1, 0, 0, 1]` for a red outline that is on from the start).

Patterns:

- **Keep the shader, animate `Params`.** One shader per effect family, with `Params` set to "off" values (usually zeros) when the effect is idle. Swapping `Sprite.Shader` also works and does not move the entity between archetypes either, but a fixed shader keeps sprites that share it batched (see [Performance](#performance)).
- **One effect per sprite.** A sprite has one shader. For a flash on an outlined sprite, write one shader that does both (`Params.x` flash, `Params.yzw` outline color) instead of stacking.
- **Let the shader read `Time` for looping motion.** Pulsing, wobbling and scrolling need no system at all: declare `var Time float` (see [Time](#time)). Keep `Params` for what game logic decides (on or off, strength, color, a per-sprite phase).

### Parameter markers

The doc comment of a uniform tells editors what each part of it means. For `var Params vec4` that lets the [particle editor](editor.md), the shader preview (`wpix-editor view fx/flash.kage`) and later the inspector show a labelled slider instead of four bare numbers. The game ignores the comments.

```text
// +shader:param:slot=x,label=Amount,min=0,max=1,default=0.5
// +shader:param:slot=yzw,label=Tint,default=#ff8000
```

| Key | Meaning |
| --- | --- |
| `slot` | `x`, `y`, `z`, `w` (or `r`, `g`, `b`, `a`), or a group of distinct letters such as `rgb`. |
| `label` | The control's name; quote it when it holds a space or a comma (`label="On, off"`). Defaults to the slot. |
| `min`, `max` | Bounds of a number, set together. |
| `default` | The value a new sprite starts with: a number, or `#rrggbb` / `#rrggbbaa` for a color. |
| `widget` | `color` (default for a group of 3 or 4 slots), `toggle` (a checkbox, one slot) or `angle` (degrees shown, radians stored, one slot). |

Each slot of a uniform belongs to one marker. Put the markers in the comment directly above a `var X T` declaration (or above `X` inside a `var ( ... )` group) whose type is `float`, `vec2`, `vec3` or `vec4`. A slot letter must exist in the type:

| Type | Slots |
| --- | --- |
| `float` | `x` |
| `vec2` | `x`, `y` |
| `vec3` | `x`, `y`, `z` |
| `vec4` | `x`, `y`, `z`, `w` |

A color group needs a `vec3` or `vec4`. `Time` takes no marker (the engine sets it). A declaration with several names (`var A, B vec4`), an array, a matrix or an inferred type cannot carry markers. A `+shader:param` anywhere else, such as in a function, above a constant, above `package` or in a loose comment, is an error.

The game feeds only `Params` (and `Time`), so the editors show the markers of `Params` only; the markers of other uniforms are read (`ParseAll`) and checked but not yet driven by any editor.

The editors read the markers with `internal/shaderparam` (not part of the game API): `Parse` returns the `Params` markers, `ParseAll` those of every uniform. It reads the source as Go syntax, as Kage does. A source that does not parse returns that parse error and no parameters; a source without any `+shader:param` text is not parsed at all. Every other mistake is reported with its line number (`line 7: slot x is already used by the marker on line 3`), and the valid markers are still returned.

## Time

Every shader the renderer runs gets `var Time float`: sprite shaders, [layer group](#layer-groups) shaders, [post passes](#post-processing) and the final pass. A pulsing glow that needs nothing but the shader:

```kage
//kage:unit pixels

package main

// Params.x: glow strength, 0 = off. Params.y: phase in seconds, so sprites
// that share the shader do not pulse in lockstep.
var Params vec4

// Time is the frame's time in seconds.
var Time float

func Fragment(dst vec4, src vec2, color vec4) vec4 {
	c := imageSrc0At(src)
	pulse := 0.5 + 0.5*sin((Time+Params.y)*6)
	return mix(c, vec4(c.a), Params.x*pulse) * color
}
```

What `Time` is:

- **Simulation seconds.** With `game.Engine` it is the sum of the tick durations of every `Update` so far, interpolated between the last two ticks with the same alpha that interpolates positions. So it moves smoothly at any frame rate, and it stops whenever the game stops updating; it does not keep running in the background like a wall clock.
- **One value per frame.** Every shader of a frame sees the same `Time`, so it does not split Ebiten's batches (see [Performance](#performance)).
- **Without `game.Engine`** it is 0 until you call `renderer.SetTime(d)` (a `time.Duration`) before `Draw`. Use whatever clock your game has.
- **A `float32`.** It has about seven significant digits, so after a few hours of play it advances in steps of a millisecond or more and slow animations start to stutter. For long sessions wrap it in the shader: `mod(Time, 60)` with a period your effect repeats in.

Effects that start at an event (a flash on a hit, a dissolve on death) still belong in `Params`: a system writes the progress, because only the game knows when the event happened. Reading `Time` there would need the start time in `Params` anyway.

## Tile layers

A tile or image layer can draw through a shader, set in Tiled with two layer properties or from code:

| Tiled property | Meaning |
| --- | --- |
| `shader` | The `.kage` key, relative to the map file like an image layer's image. Type `file` or `string`. |
| `shaderParams` | `string`, up to four comma-separated numbers (`"0.25, 2"`); missing ones are 0. |

`tilemap.Spawn` copies them into `TileLayer.Shader` and `TileLayer.Params`, which you can write at run time like `gfx.Sprite.Shader`. Group layers do not pass the properties on to their children.

- **Inputs.** The same as for sprites: `imageSrc0` is the tile image, the color argument is the layer tint, `Params` and `Time` are the uniforms. In a `Chunked` layer `imageSrc0` is the baked chunk image instead, and the shader still runs every frame, so `Time` animates it. Effects that read only the texel, `Time`, `Params` and the position `dst` look the same either way; effects that sample neighbouring texels (distortion, outline) see tile or chunk borders. For those use a [layer group](#layer-groups).
- **A bad shader fails the map.** The shader is a dependency of the map like a tileset image: a missing file or a compile error fails the map load with the shader's error, and a `shaderParams` that is not up to four numbers fails it with the layer name. A zero `TileLayer.Shader` (or one set from code that is still loading or failed) draws normally.
- **Cost.** An unchunked layer costs one shader draw per visible tile; Ebiten merges them into one GPU batch, but the CPU cost is one call per tile. A `Chunked` layer costs one draw per visible chunk and per animated tile, so prefer `Chunked` for shaded layers ([chunk pre-rendering](tilemaps.md#chunk-pre-rendering)). Layers without a shader are unchanged.

## Particles

A particle effect has `shader` and `params` keys, like a sprite has `Shader` and `Params` ([particles.md](particles.md#the-effect-file)). Every particle of the effect is drawn through the shader: `imageSrc0` is the particle sprite and the color argument is the particle's tint with its fades, so per-particle variation (a flicker that fades out) comes from the color, and `Params` are the same for all particles. In the `*.fx.json` file, `shader` is an asset key and `params` an optional array of 4 numbers.

A shaded particle is one draw call, so a shaded emitter costs CPU in proportion to its live particles instead of batching into a few quads. Keep shaded emitters small (`Max`) and put them on a layer of their own. A shader that is zero, loading or failed draws the particles normally.

## Text, panels and shapes

`gfx.Label` and `gfx.NineSlice` (and so `ScreenNineSlice`) have `Shader` and `Params` like a sprite. An outlined title, a glowing button frame or a dissolving dialog box use the same shaders as sprites:

```go
outline := render.LoadShader(sc, "fx/outline.kage")
ecs.NewEntity2(w, gfx.Transform{Pos: gfx.Vec2{X: 8, Y: 8}}, gfx.Label{
	Text: "GAME OVER", Font: font, Layer: 10,
	Shader: outline, Params: [4]float32{0, 0, 0, 1}, // black outline
})
```

In a level or prefab both fields are written like on `gfx:Sprite`: `"Shader": "fx/outline.kage"`, `"Params": [0, 0, 0, 1]`.

What the shader sees:

- **The whole item as one image.** The text (all its lines) or the panel at its `Size` is drawn first, white, at scale 1 and unrotated, into an offscreen image; the shader then runs over that image, which is placed, scaled, rotated and blended like a sprite. So `imageSrc0` is the whole text or panel, not one glyph or one panel piece: an outline runs around the outside of the text, a dissolve runs across the whole panel.
- **A margin of 4 pixels.** The image has a transparent border of 4 pixels on every side, so the [outline](#outline) and glow shaders have room to paint outside the item. Unlike sprites, nothing needs padding in the art.
- **The color argument** is `Label.Color` or `NineSlice.Tint` (premultiplied), applied once to the whole item, as for a sprite's tint.

Custom extract systems get the same for `render.Item` of kind `KindText`, `KindNineSlice`, `KindRect`, `KindLine` and `KindCircle`: set `Item.Shader` and `Item.Params`. (`gfx.DebugShape` has no shader fields: debug drawing stays plain.)

**Cost.** Each shaded text, panel or shape is drawn on its own: the pending batch is flushed, the item is drawn into the offscreen image, and one shaded quad draws it. It never batches with its neighbours, so it costs a few draw calls where an unshaded one might cost none extra. That is fine for titles, damage numbers and dialog panels; do not shade hundreds of labels. An item larger than 4096 pixels (with the margin) in either direction, or one without area, draws without its shader. A shader that is zero, loading or failed draws the item normally.

## A second image

`gfx.Sprite.Aux` hands the shader a second image as `imageSrc1`: a noise texture for a dissolve that looks hand-drawn, a mask, a palette strip, a normal map. It is a sprite handle, so any image or atlas frame works, of any size. In a level or prefab it is an asset key like `ID`: `"Aux": "fx/noise.png"`.

```go
noise := render.LoadSprite(sc, "fx/noise.png")
ecs.NewEntity2(w, gfx.Transform{}, gfx.Sprite{
	ID: hero, Shader: dissolve, Aux: noise,
	Params: [4]float32{0.3}, // 30% gone
})
```

`imageSrc1At` does not take a position in the second image. Like every `imageSrcN` after the first, it takes **`imageSrc0Origin()` plus the position inside the second image**, and returns transparent outside it. So map the position in the sprite's frame to the second image first. To stretch the second image over the frame, whatever the two sizes:

```kage
//kage:unit pixels

package main

// Params.x: 0 = whole sprite, 1 = gone. imageSrc1 is a grayscale noise
// image; its red channel decides when each pixel goes.
var Params vec4

func Fragment(dst vec4, src vec2, color vec4) vec4 {
	uv := (src - imageSrc0Origin()) / imageSrc0Size() // 0..1 inside the frame
	noise := imageSrc1At(imageSrc0Origin() + uv*imageSrc1Size())
	if noise.r < Params.x {
		return vec4(0)
	}
	return imageSrc0At(src) * color
}
```

To tile it at its own pixel size instead, use `mod(src - imageSrc0Origin(), imageSrc1Size())` as the position inside it.

- **Ignored without a shader.** `Aux` is only read through `Shader`.
- **No image is transparent.** The zero handle, and a sprite that is still loading or failed, give `imageSrc1At` transparent pixels, never the placeholder sprite.
- **Text, panels and shapes** take it through `render.Item.Aux` (custom extract systems); `gfx.Label` and `gfx.NineSlice` have no `Aux` field.
- **Batching.** `Aux` is part of the batch key: shaded sprites with the same shader, frame, `Params`, blend and `Aux` image still share a draw call.

## Layer groups

A layer group draws a range of render layers into an offscreen image instead of the screen. When the last item of the group is drawn, the image is composited onto what is below it: through a shader, with a blend mode, or both. Use it when an effect belongs to a layer, not to a sprite: water that ripples the scene behind it, a light map that darkens everything except around torches, the world turning gray while the game is paused.

```go
groups := e.Groups() // *render.GroupConfig; render.NewRenderer users call r.SetGroups(groups)
water := groups.Add(render.GroupOptions{
	From: 2, To: 2, // layers 2..2, inclusive
	Shader: render.LoadShader(e.Assets().NewScope("fx"), "fx/water.kage"),
	SampleBelow: true, // the shader gets the scene below as imageSrc1
})
```

| `GroupOptions` field | Meaning |
| --- | --- |
| `From`, `To` | The layers of the group, inclusive. Groups may not share a layer. |
| `Shader`, `Params` | Composites the group image. Zero, loading or failed: the image is composited unchanged. |
| `Blend` | How the result lands on what is below: any `render.Blend` ([kinds](kinds.md#blend-modes-renderblend)); `BlendMultiply` for light maps. |
| `Clear` | The color the image starts from every frame. Zero is transparent. A visible `Clear` composites even when no item is in the group. |
| `SampleBelow` | Copy the screen below the group and pass it to the shader as `imageSrc1`. One screen-sized copy per frame. |
| `NoComposite` | Draw the group's layers into its image and never composite it, not even an empty group with a visible `Clear`. `Shader`, `Params`, `Blend` and `SampleBelow` are ignored and the group is not counted in `Stats().Groups`. Only [post passes that read it](#layers-as-pass-inputs) see the image. |

Layers outside every group draw straight to the screen, as before. Layers above a group draw on top of its result.

### Group shaders

A group shader is a Kage shader like a sprite shader, but it runs once per **screen** pixel:

- `imageSrc0At(src)` is the group image at that pixel (logical screen size, `src` in pixels, same as the screen position).
- `imageSrc1At(src)` is the scene below the group, with `SampleBelow`. Without it, it is transparent.
- `Params` is the group's `Params`. There is no tint; `color` is white.
- `Time` is the frame's [time](#time) in seconds, as for sprite shaders.

Water: ripple the scene behind the water layer and tint it with the water sprites (Params.x is the ripple height in pixels):

```kage
//kage:unit pixels

package main

var Params vec4

var Time float

func Fragment(dst vec4, src vec2, color vec4) vec4 {
	water := imageSrc0At(src)
	if water.a == 0 {
		return vec4(0) // no water here: keep the scene as it is
	}
	wave := sin(src.y/6 + Time*3) * Params.x
	behind := imageSrc1At(src + vec2(wave, 0))
	return behind*(1-water.a) + water // water over the rippled scene
}
```

Because the shader replaces the group's pixels with the scene plus water, it only works with `SampleBelow`. Pixels it returns transparent leave the screen as it was.

Gray world while paused: a group over all world layers (UI layers stay above it), with the desaturate shader from the [examples](#desaturate-frozen-stunned-disabled) adapted to read the group image:

```kage
//kage:unit pixels

package main

var Params vec4

func Fragment(dst vec4, src vec2, color vec4) vec4 {
	c := imageSrc0At(src)
	g := dot(c.rgb, vec3(0.299, 0.587, 0.114))
	return mix(c, vec4(g, g, g, c.a), Params.x)
}
```


### Light maps

A light map needs no shader. Clear a group to the ambient light color, draw lights into it additively, and multiply the result onto the scene:

```go
const lightLayer = 10 // above the world, below the UI
groups.Add(render.GroupOptions{
	From: lightLayer, To: lightLayer,
	Clear: color.RGBA{R: 40, G: 40, B: 70, A: 255}, // night: dark blue
	Blend: render.BlendMultiply,
})
// Each torch: a soft white (or warm) radial sprite on lightLayer, BlendAdditive.
```

Where no light is, the scene is multiplied by the ambient color; under a light it gets brighter, up to unchanged at full white. Because `Clear` is visible, the darkness applies even on frames without any light sprite.

### Fog of war and flashlights

`BlendErase` cuts holes by the source's alpha. Clear a group to the fog color, draw soft white sprites into it with `BlendErase`, and composite the group normally:

```go
const fogLayer = 10
groups.Add(render.GroupOptions{
	From: fogLayer, To: fogLayer,
	Clear: color.RGBA{A: 230}, // dark fog, almost opaque
})
// Each viewer: a radial gradient sprite on fogLayer with Blend: gfx.BlendErase.
```

Where an eraser sprite is opaque the fog is gone; its soft edge fades the fog back in. Erase only makes sense inside a group, on the screen it erases to the window background.

### Animating a group

`Set` replaces a group's options between frames. Changing `Params`, `Shader`, `Blend` or `Clear` allocates nothing, so a system can do it every tick. `game.Engine` registers the config as a world resource:

```go
func WaterSystem(ctx *system.Context) {
	groups := ecs.GetResource[render.GroupConfig](ctx.World)
	o := groups.Options(water)
	o.Params[0] = 2 // ripple height; the ripples move with the built-in Time
	if ecs.GetResource[Weather](ctx.World).Storm {
		o.Params[0] = 6
	}
	groups.Set(water, o)
}
```

### Costs

Each group in use is one more screen-sized image, one clear and one composite draw per frame; `SampleBelow` adds a copy. `Renderer.Stats().Groups` counts the groups composited in the last frame. Drawing with groups allocates nothing (`TestDrawGroupsNoAllocs`).

## Post-processing

A post chain runs full-screen shaders over the finished frame: everything `Draw` drew, layer groups included, plus the background the engine filled before. Each pass reads the output of the one before it; the last one writes the screen.

```go
fx := e.Assets().NewScope("fx")
post := e.Post() // *render.PostChain; render.NewRenderer users call r.SetPost(post)
post.Passes = []render.Pass{
	{Shader: render.LoadShader(fx, "fx/flash.kage")},                               // 0: screen flash
	{Shader: render.LoadShader(fx, "fx/vignette.kage"), Params: [4]float32{0.7}},   // 1: vignette
}
post.Final = &render.Pass{Shader: render.LoadShader(fx, "fx/crt.kage"), Params: [4]float32{0.35}}
```

| `Pass` field | Meaning |
| --- | --- |
| `Shader` | Runs once per pixel of the frame. Zero, loading or failed: the pass is skipped. |
| `Params` | The shader's `Params` vec4. |
| `Off` | Skips the pass without removing it (a settings toggle). |

`PostChain.Fade` (a `gfx.Color`) covers the frame with a color after the passes and before `Final`, without a shader: its alpha says how much, and, unlike a sprite tint, a zero alpha byte (the zero value) means no fade. `post.Fade = gfx.RGBA(0, 0, 0, 128)` darkens the frame by half; a fade costs one draw while it is on. Room transitions fade through it ([rooms.md](rooms.md#custom-transitions)).

A post shader works like a [group shader](#group-shaders): `imageSrc0At(src)` is the frame so far, at logical resolution, `src` in pixels. Its result replaces the pixel, alpha included, so return an opaque color unless you want the window to show through.

With no pass that runs (no passes, all `Off`, shaders still loading) the renderer draws straight to the screen as if there were no chain, and allocates nothing.

### Targets and multiple inputs

A pass can read up to four images and write either the chain or an offscreen **target**, and targets can be smaller than the screen. That is what effects like bloom and multi-scale blur need: shrink the frame, blur the small copy, add it back. Passes that set none of the fields below work as described above.

```go
post := e.Post()
half := post.AddTarget(render.Target{Scale: 2}) // half the screen size
half2 := post.AddTarget(render.Target{Scale: 2})
// down, blur and add are shader handles loaded with render.LoadShader.
post.Passes = []render.Pass{
	{Shader: down, Out: half},                       // frame -> half-size copy
	{Shader: blur, Src: half, Out: half2},           // blurred into another target
	{Shader: add, Extra: [3]render.TargetID{half2}}, // chain + half2 -> chain
}
```

| `Target` field | Meaning |
| --- | --- |
| `Scale` | Divides the screen size: 1 (or 0) is the screen size, 2 half, 4 a quarter, rounded up. The image follows screen resizes, is allocated the first frame a running pass uses it, and starts transparent. |

`post.AddTarget(t)` appends a target and returns its `render.TargetID`. Declare targets before the first frame that uses them and keep their order: passes name them by ID.

| `TargetID` | Meaning |
| --- | --- |
| `render.TargetChain` (0) | The chain so far. As `Src`: the output of the pass before (the frame for the first one). As `Out`: the next chain image, or the screen for the last pass that writes it. As an `Extra` entry, zero means unused. |
| `render.TargetFrame` | The frame `Draw` produced, before any pass. Input only. |
| `render.TargetPrevious` | What the chain showed on the previous frame (the current frame on the first frame and after a resize). Input only. |
| `render.TargetGroup(id)` | The image of layer group `id`: its items drawn over its `Clear`, before its shader and blend, at the screen size. Input only. See [Layers as pass inputs](#layers-as-pass-inputs). |
| any `AddTarget` result | That target. |

| `Pass` field | Meaning |
| --- | --- |
| `Src` | The image read as `imageSrc0`. Zero: the chain. |
| `Extra` | The images read as `imageSrc1` to `imageSrc3`. Zero: unused. |
| `Out` | The image written. Zero: the chain. A pass that writes a target leaves the chain as it was. |

If no running pass writes the chain, the frame reaches the screen unchanged. A skipped pass (`Off`, shader not loaded) writes nothing: its `Out` keeps what it held last frame, or is transparent if it was never written.

**Coordinates.** A pass draws one quad over its `Out` image, and `src` spans all of `Src`, in `Src`'s pixels. A pass that writes a quarter-size target from the full frame runs once per quarter pixel, and `src` is the corner shared by the 2x2 or 4x4 block of frame pixels it covers; sample the pixel centers around it to average them. Kage takes the position of `imageSrc1At` and the following in `imageSrc0`'s texture, so map `src` to the other image's pixels and add `imageSrc0Origin()`:

```kage
//kage:unit pixels

package main

// Adds imageSrc1 (Extra[0]), whatever its size, to imageSrc0 (Src).
func Fragment(dst vec4, src vec2, color vec4) vec4 {
	rel := (src - imageSrc0Origin()) / imageSrc0Size() * imageSrc1Size() // in imageSrc1's pixels
	return imageSrc0At(src) + imageSrc1At(imageSrc0Origin()+rel)
}
```

`imageSrcNAt` samples the nearest pixel, so a small image read at full size shows blocks. Sample four neighbours and `mix` them for a bilinear upsample; the bloom's `add` shader does.

A pass may not read the image it writes, and only the chain and targets can be written. These panic when `Draw` finds them in a running pass (skipped passes are not checked):

| Problem | Panic |
| --- | --- |
| `Src`, an `Extra` entry or `Out` is not a special value, a declared target or an existing group | `render: unknown post target <id> in pass <i>` |
| `Out` is also read by the pass | `render: post pass <i> reads and writes target <id>` |
| `Out` is `TargetFrame` or `TargetPrevious` | `render: post pass <i> writes TargetFrame` (or `TargetPrevious`) |
| `Out` is a `TargetGroup` | `render: post pass <i> writes group <g>` |

### Bloom

`PostChain.AddBloom` appends a ready-made bloom to the chain: the bright parts of the chain so far are shrunk to a half and a quarter of the screen size, blurred there and added back, so they glow. It needs no shader asset, so it works from the first frame.

```go
bloom := e.Post().AddBloom(render.BloomOptions{Threshold: 0.6, Strength: 0.8})
// Later, from a settings system:
bloom.Set(e.Post(), render.BloomOptions{Strength: 1.2})
bloom.SetOff(e.Post(), true) // off
```

| `BloomOptions` field | Meaning (a zero field takes the default) |
| --- | --- |
| `Threshold` | Brightness (the largest of red, green and blue, 0..1) above which a pixel glows, and only the part above it. Default 0.7. |
| `Strength` | Scales the glow added to the frame. Default 1. |
| `Radius` | Distance between the taps of the blur, in pixels of the quarter-size image. Default 1. |
| `Src` | What glows. Zero (`TargetChain`): the chain so far, the whole frame. `TargetGroup(id)`: only the layers of one [layer group](#layer-groups) (see below). |

It declares three targets (half, quarter and a quarter-size scratch image) and five passes. Passes added later see the bloom; the ones before it are what glows. Do not remove or reorder passes before the bloom's, because `Bloom` finds its passes by index. The frame should be opaque; a glow over a transparent pixel is added with matching alpha.

**Glow only for some layers.** Targets are 8 bits per channel, so a bloom over the whole frame cannot tell a white wall from a white-hot laser: both are 1.0. Put what should glow on layers of their own, make a group of them, and pass it as `Src`:

```go
glow := e.Groups().Add(render.GroupOptions{From: 20, To: 20}) // neon signs, lasers, magic
e.Post().AddBloom(render.BloomOptions{Src: render.TargetGroup(glow), Threshold: 0.3})
```

The group still draws normally; the bloom only reads it, and adds the glow to the whole chain. Bright sprites on other layers stay crisp, and a lower `Threshold` lets the glowing layers glow at any brightness. Add `NoComposite` to the group for a glow without the sprites themselves (a halo map). Costs are a group's ([costs](#costs)) plus the bloom's.

### Layers as pass inputs

`render.TargetGroup(id)` hands a pass the image of a [layer group](#layer-groups) as it was drawn: the group's items over its `Clear`, before its shader and blend. Put the layers an effect needs in a group with `NoComposite` and they reach the screen only through the passes that read them: a mask for an outline or glow, a displacement map for heat haze, a light map to blur before it is applied.

```go
glow := groups.Add(render.GroupOptions{From: 8, To: 8, NoComposite: true}) // glowing sprites live on layer 8
post.Passes = append(post.Passes, render.Pass{
	Shader: blurAdd, // reads the chain as imageSrc0 and the mask as imageSrc1
	Extra:  [3]render.TargetID{render.TargetGroup(glow)},
})
```

The image is always this frame's: a group no item fell into reads as its `Clear` color, not as last frame's pixels. A group is drawn offscreen once per frame however many passes read it. A pass can read a normal (composited) group too, and gets the same raw image. Group images are the screen size; a pass that wants less downsamples into a target, as the bloom does. A group image costs what a group costs (one screen-sized image and one clear); a group that only a pass reads adds no composite draw.


### Previous frame

A running pass that reads `TargetPrevious` sees the chain's output of the last frame, for trails and feedback effects. It costs two more screen-sized images and one more copy per frame, and the images are freed when no running pass reads it any more.

### Examples

Vignette: darken towards the corners (Params.x: strength, 0..1):

```kage
//kage:unit pixels

package main

var Params vec4

func Fragment(dst vec4, src vec2, color vec4) vec4 {
	c := imageSrc0At(src)
	p := (src-imageSrc0Origin())/imageSrc0Size() - 0.5 // -0.5..0.5
	d := length(p) * 1.4142                            // 0 in the center, 1 in a corner
	dark := 1 - Params.x*smoothstep(0.4, 1, d)
	return vec4(c.rgb*dark, c.a)
}
```

Screen flash and fade: mix the frame towards a color (Params.rgb: the color, Params.a: how much, 0..1). White for a hit, red for damage, black for a fade-out:

```kage
//kage:unit pixels

package main

var Params vec4

func Fragment(dst vec4, src vec2, color vec4) vec4 {
	c := imageSrc0At(src)
	return vec4(mix(c.rgb, Params.rgb, Params.a), c.a)
}
```

Drive it from a system. `game.Engine` registers the chain as a world resource:

```go
func FlashSystem(ctx *system.Context) {
	post := ecs.GetResource[render.PostChain](ctx.World)
	flash := &post.Passes[0].Params
	flash[3] = max(flash[3]-float32(ctx.Time.Delta.Seconds())*4, 0) // fade out
}

// On a hit: post.Passes[0].Params = [4]float32{1, 1, 1, 0.8}
```

### Final pass (window resolution)

Pixel-art games draw at a small logical size that Ebiten scales up to the window. Every pass above runs at that logical size, which is what most effects want. A CRT mask or scanlines need the window's real pixels instead: that is `PostChain.Final`. It runs once per **window** pixel while it draws the scaled frame, so `dst` is in window pixels and `imageSrc0At(src)` samples the frame in logical pixels:

```kage
//kage:unit pixels

package main

var Params vec4

// Params.x: how dark the scanlines are, 0..1.
func Fragment(dst vec4, src vec2, color vec4) vec4 {
	c := imageSrc0At(src)
	if mod(floor(dst.y), 2) == 1 {
		return vec4(c.rgb*(1-Params.x), c.a)
	}
	return c
}
```

`game.Engine` runs the final pass through Ebiten's `FinalScreenDrawer`. Without `game`, implement `DrawFinalScreen(screen, offscreen, geoM)` on your `ebiten.Game` and call `renderer.DrawFinal(screen, offscreen, geoM)`. With no final pass (nil, `Off` or not loaded) `DrawFinal` draws the frame with Ebiten's default scaling.

### Costs

A running chain costs one screen-sized copy of the frame plus one draw per pass (over the pass's `Out` image, so a pass into a quarter-size target is cheap), and one or two screen-sized images (two only with two or more running passes). A pass that reads `TargetFrame` after the chain was written twice or more keeps the frame in a third image. Each target in use is one image of its `Scale`. `TargetPrevious` adds two screen-sized images and one copy; a chain with no pass that writes the chain adds one copy. `AddBloom` is five draws and a half, a quarter and a quarter-size image. The final pass is one draw at window size. `Renderer.Stats().PostPasses` counts the passes that ran. Steady-state frames allocate nothing (`TestDrawPostNoAllocs`, `TestDrawPostTargetsNoAllocs`, `TestDrawBloomNoAllocs`).

## Loading and errors

`render.LoadShader(sc, key)` loads a `.kage` file in the background, like every asset. Until it is ready the sprite draws **normally** (no shader), so nothing flickers or disappears while it loads.

| Problem | What happens |
| --- | --- |
| The file is missing | The asset fails; the sprite draws normally. |
| The Kage source does not compile | The asset fails with the compiler message; the sprite draws normally. |
| `Params` is declared with a type other than `vec4`, or `Time` with a type other than `float` | The asset fails with `shader uniforms: ... (want: var Params vec4, var Time float)`; the sprite draws normally. Without this check Ebiten would panic in the middle of drawing. |
| The shader handle is zero | No shader: the sprite draws normally. |

The error reaches `Config.OnAssetError` (by default it is logged) and `render.ShaderStoreOf(srv).Err(h)`. Use `render.LoadShaderSync` in tests and tools to load and get the error in one call.

## Hot reload

With [asset hot reload](assets.md#hot-reload) on (`Config.WatchInterval`, or a host calling `Assets().Changed`), saving a `.kage` file recompiles it and swaps the shader in place: the handle stays valid, and sprites, tile layers, particles, layer groups and post passes draw with the new shader on the next frame. A save that does not compile, or that declares `Params` or `Time` with the wrong type, keeps the previous shader drawing and reports `asset: reload failed, keeping previous version: <key>: <error>` once through `Config.OnAssetError`. In the [level editor](../../design/editor.md) the same error is shown in VSCode.

`Params` that come from data follow too: a changed Tiled `shader` or `shaderParams` reaches the spawned [`TileLayer`](tilemaps.md) when its map reloads, unless game code wrote that field since `Spawn`. `Params` that game code sets (post passes, groups, systems) are code and do not reload.

## Performance

- **Drawing allocates nothing.** Shaded sprites, like unshaded ones, add no allocations per frame (`TestDrawShadedNoAllocs`).
- **Batching.** Consecutive shaded sprites with the same shader, the same **frame**, the same blend mode, **the same `Params`** and the same `Aux` image are one draw call ([rendering guide](rendering.md#batching)), so the particles of one effect cost about as much as unshaded ones. Each sprite with another frame or other `Params` is its own draw call and its own GPU batch: the shader sees its frame as `imageSrc0`, and Ebiten merges only draws with equal uniforms. The queue sorts sprites in a layer by shader, then by texture, so sprites that share both stay together. That is fine for the few sprites that are flashing or outlined at any moment. Avoid giving thousands of sprites individually different `Params` every frame, and make a shaded tile layer `Chunked` (every tile of a layer is another frame; a chunked layer runs the shader once per chunk).
- **Y-sorted layers** sort by Y first, so shaded and unshaded sprites interleave there and batch less.
- **Shaded text, panels and shapes** are a draw call of their own each, plus the draw into the offscreen image ([text, panels and shapes](#text-panels-and-shapes)). They allocate nothing of the engine's own (`TestDrawShadedKindsNoAllocs`).
- `Renderer.Stats().ShaderDraws` counts the draw calls that went through a shader in the last frame, `ShadedQuads` the items drawn through one.

## Limits

- Tiled object layers are map data and are not drawn, so they have no shader. `gfx.DebugShape` has no shader fields (`render.Item` shapes do).
- A sprite's shader paints only inside the sprite's rectangle: glows and outlines need transparent padding in the frame. Text and panels get a 4 pixel margin instead.
- The per-sprite inputs are the `Params` vec4, the tint and one second image (`Aux`); for particles, the tint is the only per-particle input and there is no second image. `Time` is the same for all sprites.
- Layer groups cannot nest or overlap.
- Post targets are 8 bits per channel (Ebiten has no floating-point images), so colors cannot go above 1.0 and a bloom cannot tell bright from very bright; choose what glows with a [layer group](#bloom) instead. The final pass reads one image.

## API reference

| API | Package | What it does |
| --- | --- | --- |
| `gfx.Sprite.Shader` | `gfx` | `gfx.ShaderHandle`: the shader, zero for none. GOD: asset key, kind `shader`. |
| `gfx.Sprite.Params` | `gfx` | `[4]float32`: the shader's `Params` vec4. GOD: optional array of 4 numbers. |
| `gfx.Sprite.Aux` | `gfx` | `gfx.SpriteHandle`: the shader's second image, `imageSrc1` ([a second image](#a-second-image)). GOD: asset key, kind `sprite`, optional. |
| `gfx.Label.Shader`, `.Params`, `gfx.NineSlice.Shader`, `.Params` | `gfx` | The same two fields on text and panels ([text, panels and shapes](#text-panels-and-shapes)). |
| `render.LoadShader(sc, key)` | `render` | Handle now, load in the background. `LoadShaderSync` blocks; `HoldShader` adds an owner. |
| `render.ShaderStore` | `render` | `Get` (nil `Shader` field = no shader), `Ready`, `Err`, `Key`, `Lookup`, `Generic`. From `ShaderStoreOf(srv)` or `StoresOf(srv).Shaders`. |
| `tilemap.TileLayer.Shader`, `.Params` | `tilemap` | The shader and `Params` of a tile or image layer; `Spawn` sets them from the Tiled properties `shader` and `shaderParams`. |
| `shader`, `params` in `*.fx.json` | `particles` | The same two fields on a particle effect: asset key and optional array of 4 numbers. |
| `render.Item.Shader`, `Item.Params`, `Item.Aux` | `render` | The same fields on queue items of every drawn kind (sprite, canvas, text, nine-slice, rect, line, circle), for your own extract systems. |
| `render.ShaderUniformParams` | `render` | `"Params"`: the uniform's name. |
| `render.ShaderUniformTime` | `render` | `"Time"`: the name of the time uniform, a `float` in seconds. |
| `Renderer.SetTime(d)` | `render` | Sets `Time` for the following `Draw` and `DrawFinal`. `game.Engine` calls it every frame. |
| `Renderer.Stats().ShaderDraws`, `.ShadedQuads` | `render` | Shader draw calls and shaded items in the last `Draw`. |
| `render.GroupConfig` | `render` | Layer groups: `Add(GroupOptions) GroupID`, `Set`, `Options`, `Len`. `Renderer.SetGroups`; `game.Engine.Groups()`. |
| `render.GroupOptions` | `render` | `From`, `To`, `Shader`, `Params`, `Blend`, `Clear`, `SampleBelow`. |
| `render.Blend` | `render` | Blend modes of items and groups, `BlendNormal` to `BlendErase`: [kinds](kinds.md#blend-modes-renderblend). `BlendMultiply` multiplies onto what is below (light maps), `BlendErase` cuts holes in a group. |
| `Renderer.Stats().Groups` | `render` | Groups composited in the last `Draw`. |
| `render.PostChain` | `render` | `Passes []Pass` over the frame, `Final *Pass` at window resolution. `Renderer.SetPost`; `game.Engine.Post()` (also a world resource). |
| `render.Pass` | `render` | `Shader`, `Params`, `Off`, and the inputs and output: `Src`, `Extra`, `Out`. |
| `render.Target`, `PostChain.AddTarget` | `render` | An offscreen image for passes (`Scale` divides the screen size) and its declaration; returns a `TargetID`. |
| `render.TargetID` | `render` | `TargetChain`, `TargetFrame`, `TargetPrevious`, `TargetGroup(id)` or a declared target. |
| `PostChain.AddBloom(BloomOptions) Bloom` | `render` | Appends a bloom (three targets, five passes); `BloomOptions` has `Threshold`, `Strength`, `Radius` and `Src` (what glows). `Bloom.Set` retunes, `Bloom.SetOff` toggles it. |
| `Renderer.DrawFinal(screen, offscreen, geoM)` | `render` | The final pass, for `ebiten.FinalScreenDrawer`. `game.Engine` calls it. |
| `Renderer.Stats().PostPasses` | `render` | Post passes that ran, the final pass included. |

How it is built and why: [design/engine-renderer.md](../../design/engine-renderer.md#shaders).
