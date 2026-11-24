# Shaders, layer groups and post-processing

How custom shaders, layer groups and post-processing passes fit into the renderer of [engine-renderer.md](engine-renderer.md).
The API is in the [shaders guide](../guide/engine/shaders.md).

## Shaders

Per-sprite shaders are item data, like the tint: `Item.Shader` (a `gfx.ShaderHandle`) and `Item.Params` (`[4]float32`), filled from `gfx.Sprite`.
How to use them: [shaders guide](../guide/engine/shaders.md).
Decisions:

- **Fields on `Sprite`, not a separate component.**
  Views have no exclusion filter, so a `Material` component would need a per-entity lookup in `SpriteExtract` or a second extract that queues shaded sprites twice.
  As fields, an effect toggles without an archetype move, and GOD files and Lua get them through the schema.
  `Params` is `omitzero`, so existing GOD files stay valid.
- **One `vec4` of parameters.**
  A fixed block keeps items pointer-free and the queue 0 allocs.
  Effects that need more data than four floats belong in a texture or, later, a layer shader.
- **Shaded quads in the quad batch, from the frame's sub-image.**
  A shaded sprite is a quad of its image's size placed by the same `GeoM` as `DrawImage`, drawn with `DrawTrianglesShader32`; the sprite's sub-image is `Images[0]`, so `imageSrc0At` is clipped to the frame and `imageSrc0Origin`/`imageSrc0Size` are the frame, exactly as with the `DrawRectShader` call it replaced (see [Batching](engine-renderer.md#batching)).
  A nil shader (zero, loading, failed) takes the plain path, which keeps the placeholder rule.
- **Uniforms without allocations.**
  Ebiten takes uniforms as `map[string]any`, and storing a fresh value in it boxes and allocates.
  The renderer builds the map once with a `[]float32` behind `Params` and overwrites its elements per draw.
  Ebiten ignores names a shader does not declare, but panics on a type mismatch, so the shader loader draws each new shader once into a 1x1 image with the same map and turns that panic into a load error.
- **Canvases take shaders too.**
  `drawCanvas` branches into `drawShaded` like `drawSprite`, so a chunked tile layer's shader runs on the baked chunk image each frame.
  The chunk is baked without the shader (`bake` paints plain tile items), otherwise `Time` would be frozen into the image.
  Particles needed no renderer work: `particles.Extract` already queues one `KindSprite` item per particle, so the emitter's `Shader` and `Params` ride on it, and consecutive particles of one frame share a shaded batch.
  A tile layer's shader is a map dependency (property `shader`), so a bad one fails the map instead of drawing unshaded.
- **Text, nine-slices and shapes through a scratch image.**
  These kinds are several quads (or Ebiten's text drawing), so a shader run per quad would see one glyph or panel piece and could not outline or dissolve the whole item.
  A shaded one is drawn white, at scale 1 and unrotated, into a renderer-owned scratch image with a 4 pixel transparent margin, and that region is drawn as one shaded quad with the item's transform, tint, blend and `Params`, so the shader contract is the one of sprites (`render/shade-scratch.go`).
  The scratch image grows to the next power of two and never shrinks, and the region sub-images are cached by size, so a steady frame allocates nothing of its own.
  The cost is two flushes per item: shaded items of these kinds never batch, which is fine for the few titles and panels that need it.
  Local pixels rather than screen pixels keep the scratch size independent of zoom and make the shader see the same texels at every scale, as for sprites.
- **A second image as a sprite handle.**
  `Item.Aux` (from `gfx.Sprite.Aux`) is a `gfx.SpriteHandle`, so the second image is any loaded image or atlas frame and needs no new asset kind or store.
  It goes into `Images[1]` of the shaded batch and is part of the batch key.
  Ebiten's pixel mode allows source images of different sizes (texel mode does not), and it passes each image's region as uniforms, so an atlas frame works as is; `imageSrc1At` takes `imageSrc0Origin` plus a position in that region, which the guide shows.
  A zero, loading or failed handle gives no image rather than the placeholder sprite, so a mask or noise shader degrades to "nothing" instead of sampling a checkerboard.
  One image, not three: a sprite with more inputs is rare enough to pack them into one texture.
- **A built-in `Time` uniform, set once per frame.**
  Every uniform map also holds a `Time` entry, and all of them point at the same one-element `[]float32`, so `Renderer.SetTime` writes one float and every shader of the frame (sprite, group, pass, final) sees it, with no allocation.
  `Time` is a `float`, so the loader check covers it too.
  The renderer does not read a clock: whoever drives it decides what time means.
  `game.Engine` passes simulation time (tick durations summed by `Update`) interpolated with the draw's alpha, so shader motion is as smooth as interpolated positions, stops when updates stop, and is reproducible in replays; a wall clock would keep running while the game is paused and differ between runs.
  It is a `float32` in seconds; precision loss after hours is left to the shader (`mod`) rather than wrapped by the engine, because no period suits every effect.

## Layer groups

A `GroupConfig` maps disjoint layer ranges to offscreen images (`GroupOptions`: `From`..`To`, `Shader`, `Params`, `Blend`, `Clear`, `SampleBelow`).
How to use them: [shaders guide](../guide/engine/shaders.md#layer-groups).
Decisions:

- **Ranges of layers, not a per-layer flag.**
  Layers already order the queue, so a group's items are contiguous after `Sort`.
  `Draw` walks the groups (sorted by `From`) alongside the items: when an item's layer enters a group the target switches to the group image, when it leaves the image is composited onto the screen.
  No per-item lookup, no second pass over the queue.
  Ranges may not overlap (panic), so there is no nesting.
- **Screen-sized images, reused.**
  One image per group at the logical screen size, reallocated only when that size changes.
  Clearing it each frame is one fill.
  A copy for `SampleBelow` is one more image shared by all groups.
- **Empty groups still composite when `Clear` is visible.**
  A light map with an ambient color must darken the scene even on a frame with no light sprites; a transparent group with no items is skipped, so an unused group costs nothing.
- **`NoComposite` instead of a second "layer to target" mechanism.**
  Groups already route a layer range into an offscreen image with no per-item cost; what was missing was reading that image from a pass and skipping its composite.
  A `NoComposite` group never composites, not even empty with a visible `Clear`, and is not counted in `Stats.Groups`.
  Group images are the logical screen size; reduced-resolution layers would need a scaled transform per item and are not built.
- **`BlendMultiply`** (`dst = src*dst + dst*(1-srcAlpha)`) is the light-map blend: a group cleared to the ambient color, with additive light sprites, multiplied onto the scene.
  It is also usable per item (shadows).
- **`BlendScreen`, `BlendSubtract`, `BlendLighten`, `BlendErase`** are fixed-function Ebiten blends on premultiplied colors (screen: factors One / OneMinusSourceColor; subtract: ReverseSubtract; lighten: Max; erase: `BlendDestinationOut`), so they batch like the others.
  **No Darken:** the Min operation ignores blend factors, so the all-zero transparent parts of a sprite or group image would black out the destination.
  **No overlay or soft light:** they need the destination inside a non-linear formula, which is what `SampleBelow` shaders are for.
  `Erase` belongs inside a group (fog of war, flashlights); a group's own `Blend` may be Erase too, which erases to the window background.
- **The group shader is a normal Kage shader** with the same `Params` uniform map as sprite shaders (built once, 0 allocs), drawn as one screen quad with `DrawTrianglesShader32` (`drawShaderQuad`, shared with post passes and the final pass).
  `imageSrc1` is the copy of the scene below, when asked for: sampling `dst` directly is not possible in Ebiten.
- **`Set` between frames.**
  Changing `Params`, `Shader`, `Blend` or `Clear` allocates nothing (the clear color is boxed once per change), so systems can animate groups every tick.
  `game.Engine` registers its `*GroupConfig` as a world resource for that.

## Post-processing

`PostChain{Passes []Pass; Targets []Target; Final *Pass; Fade gfx.Color}`, `Pass{Shader, Params, Off, Src, Extra, Out}`.
How to use it: [shaders guide](../guide/engine/shaders.md#post-processing).
Decisions:

- **Game code owns the chain.**
  The renderer keeps the pointer (`SetPost`) and reads it in every `Draw`; `game.Engine` registers it as a world resource so systems change `Params` or `Off` between frames.
  No copy, no per-frame setup.
- **Zero overhead when nothing runs.**
  `Draw` first checks whether any pass is on with a loaded shader.
  If none is, it draws straight onto `dst` and touches no offscreen image; output is identical to a renderer without a chain.
- **Copy, then ping-pong.**
  With passes, `Draw` copies `dst` into the frame image (so the background the caller filled is part of the frame), draws items and groups into it, then runs the passes alternating between the frame image and a second one; the last pass draws onto `dst`.
  The second image exists only for chains with two or more running passes.
  Both follow the screen size like group images.
- **Passes are a small graph.**
  A pass reads `Src` and up to three `Extra` images and writes `Out`: the chain, or a target declared on the chain.
  Targets are typed IDs (`i+1` for `Targets[i]`), not strings, so a typo cannot compile and a pass costs no string compare.
  The chain keeps its ping-pong; targets are separate images sized `ceil(screen/Scale)` and allocated on first use.
  Legacy passes leave all three fields zero and behave and cost as before.
- **Every pass is one `DrawTrianglesShader` quad.**
  `DrawRectShader` needs every source image to have the rectangle's size; `DrawTrianglesShader` in pixel units does not.
  The quad spans `Out` and its source coordinates span `Src`, so downsampling needs no scale uniform: the shader runs per small pixel and `src` is the block corner.
  One code path serves legacy and graph passes; the pixel tests of the old chains prove it renders the same.
  Kage takes the position of `imageSrcNAt` in `imageSrc0`'s texture, which is why the guide maps `src` and adds `imageSrc0Origin()`.
- **The frame survives only when asked for.**
  `TargetFrame` is the first image.
  A pass that writes the chain twice overwrites it in the plain ping-pong, so a chain that reads `TargetFrame` after a chain write gets a third image.
  Chains that do not pay nothing.
- **Previous frame by history images.**
  A running pass reading `TargetPrevious` makes the chain's last pass write one of two history images, which is then copied to the screen (two images and a copy, only then); the pair swaps every frame.
  The first frame and the first after a resize seed the previous image from the current frame, so feedback effects do not flash black.
  The images are freed when no running pass reads it.
- **Groups are input-only targets.**
  `TargetGroup(id)` is `TargetID(-3 - id)`, so `Src` and `Extra` need no new fields and `Pass` keeps its size; the encoding is private to `groupOf`.
  A pass sees the raw group image (items over `Clear`, before shader and blend).
  Each group records the frame it was last cleared in (`Renderer.begun`), and `postInput` begins a group that no item reached this frame, so a pass never reads stale pixels and groups nobody reads cost nothing.
  Writing a group is a panic: group images are written by the queue.
- **Validation per Draw.**
  Running passes are checked each frame (unknown target or group, reading and writing one target, writing `TargetFrame`/`TargetPrevious` or a group); the loop is allocation-free.
  Skipped passes are not checked and write nothing, so their `Out` keeps its last content.
- **Bloom is a helper over this graph, with built-in shaders.**
  `AddBloom` appends three targets and five passes; its Kage sources (`render/shaders/`, embedded) are compiled by the Renderer the first time a pass uses them, through an unexported `Pass` field, so they need no asset scope and no loading frames.
  A compile error panics (it is an engine bug; `TestBuiltinShadersCompile`).
  The glow is 8-bit: Ebiten v2.10 has only RGBA8 images, so HDR targets cannot be built on it.
  `BloomOptions.Src` is the substitute: the bright pass reads a layer group (`TargetGroup`) instead of the chain, so what glows is chosen by layer rather than by brightness above 1.0, and it costs nothing beyond the group, which is drawn offscreen anyway.
- **Passes replace, they do not blend.**
  Each pass uses `BlendCopy` over the whole frame, so no image needs a clear and a pass fully decides each pixel.
- **The final pass is separate** (`Final`, `Renderer.DrawFinal`), because pixel-art games want most effects at the logical resolution and only the CRT mask at window resolution.
  `DrawFinal` has the `ebiten.FinalScreenDrawer` signature and draws one quad of the frame's size placed by Ebiten's `geoM` (`drawShaderQuad`, snapped like `DrawRectShader`; `TestDrawFinalMatchesDrawRectShader`), so the shader runs per window pixel while sampling the frame in logical pixels; without a final pass it calls `ebiten.DefaultDrawFinalScreen`.
- **Own uniform maps.**
  Passes and the final pass each reuse a map built once (see [Shaders](#shaders)), so changing `Params` every frame allocates nothing.
- **Fade without a shader.**
  `Fade` is the renderer's white pixel stretched over the frame with a color scale, drawn after the passes (or straight after the items without them) and before `Final`.
  A built-in fade needs no shader asset to ship or wait for, and costs one draw only while its alpha is above zero.
  The zero value means no fade, unlike a sprite tint, so an untouched chain does nothing.
