# Engine and renderer design

Why the engine is split into plain-data components, extract systems and an ecs-free renderer, and what the first version leaves out. The API is in the [engine guide](../guide/engine/README.md); the package layering in [architecture.md](architecture.md#engine-runtime-packages).

## Three parts

| Part | Knows about | Runs in | Job |
| --- | --- | --- | --- |
| Components (`gfx`) | plain data, asset handles | n/a | What an entity looks like: pointer-free, handles instead of images |
| Extract systems (`extract`, `anim`) | `ecs` + `render` | `Update`, last stages of the schedule | Walk views, cull, write `render.Item`s into a `render.Queue` resource |
| Renderer (`render`) | Ebiten, not `ecs` | `Draw` | Draw the sorted queue |

Why split:

- **Ebiten's loop forces a handoff anyway.** `Update` runs zero or more times per `Draw`, and the World must not be touched in `Draw`. A queue built at the end of `Update` is that handoff. Ebiten calls both on one goroutine, so no double buffering is needed.
- **The renderer stays testable and swappable.** It takes a list of plain structs: sorting and culling are unit-tested without a World, and a queue can be replayed in a debug tool.
- **Ebiten stays out of `ecs`**, and out of `gfx` and `asset`, so tools and servers never compile it.
- **One place owns draw order and batching.** Systems say "draw sprite X on layer 3 at (x, y)"; only the renderer decides order and batching.

What is *not* split: deciding what is visible (culling, `Sprite.Hidden`) happens in extract, where the data is.

## The frame contract

```
Update (fixed TPS): assets → snapshot PrevPos → game stages → animation → reset queue → extract → sort
Draw (display rate): Renderer.Draw(screen, queue, camera, alpha)
```

- `AssetSystem` runs first so loaded assets are committed before anything reads them.
- `SnapshotTransforms` copies `Pos` to `PrevPos` before anything moves; `Draw` blends `PrevPos.Lerp(Pos, alpha)` so a 60 TPS simulation looks smooth on a 144 Hz display without running more ticks.
- **Pixel snap after blending, in `Draw`.** `Camera.PixelSnap` rounds the blended view once per frame so that viewport center − `Pos`·`Zoom` is an integer. It has to happen after blending: rounding `Pos` in the tick and blending two rounded positions gives a sub-pixel view again. The camera methods (`WorldToScreen`, `ScreenToWorld`, `WorldBounds`) round the same way, so picking matches the drawn pixels; `Draw` clears `PixelSnap` on its rounded copy so per-item transforms don't round again. It is opt-in (the zero value keeps the exact view for non-pixel-art and rotated cameras) and ignored with a rotation.
- **Culling covers the whole tick.** `Draw` shows any view between `PrevPos` and `Pos`, so extract culls against `Camera.DrawBounds`, the union of both views. Culling against `Pos` alone dropped a sliver at the trailing edge while the camera moved. No margin is needed for `PixelSnap`: rounding is monotonic, so the rounded blended view lies between the two rounded end views.
- `game.Engine` fixes this order and builds the schedule on the first `Update`, so user systems can only go into the four slots between snapshot and animation ([engine.md](../guide/engine/engine.md#tick-order)).
- Animation is not a renderer feature: `AnimationSystem` writes `Sprite.ID` (and `Origin` for pivoted clips); the renderer only ever sees the current sprite.

## Items and sorting

- `render.Item` is a fat value (136 bytes): kind, space, layer, sort Y, sprite handle, `Pos`/`PrevPos`, rotation, scale, origin, flips, size, color, blend, shader handle and params, text, font, nine-slice insets. The shader fields took it from 96 bytes, which made `BenchmarkTilemapExtract/large` (a pure item-copy loop) about 17% slower. Fine for tens of thousands of items and simple to fill. If profiles say otherwise, sort a packed `uint64` key slice instead of moving items.
- The queue is reused every tick (`Reset` keeps capacity), so steady state is 0 allocs.
- Sort order: layer, then `SortY` in Y-sorted layers, then the sprite's shader and texture (so draws from one atlas batch together, see [Batching](#batching)), then insertion order. The key is one `uint64` per item (shader slot high, texture slot low), computed once per sort. The texture key needs the sprite store, hence the optional `*render.Stores` resource.
- `Item.Scale` is used as is: the "zero means 1" rule of `Transform.Scale` is resolved in extract, not in the renderer.

## Zero values

Every zero-valued component draws something sensible: packed `gfx.Color` 0 is opaque white (so the zero tint is "no tint"), `Transform.Scale` zero is (1, 1), `Camera.Zoom` zero is 1, the zero handle is the placeholder, and the zero `Sprite.Origin` is the sprite's default pivot. That is why top-left needs a tiny non-zero origin. A packed `uint32` color (instead of `ebiten.ColorScale`) keeps `gfx` free of Ebiten.

## Placeholders

Components hold handles, and the renderer resolves them at draw time through read-only stores. A handle that is zero, missing, loading, failed or stale draws the placeholder: a magenta checker in development, nothing with `TransparentPlaceholders` in release. Because the queue holds handles, not images, a freshly loaded texture shows up on the next `Draw` with no extra work.

## Shaders

Per-sprite shaders are item data, like the tint: `Item.Shader` (a `gfx.ShaderHandle`) and `Item.Params` (`[4]float32`), filled from `gfx.Sprite`. How to use them: [shaders guide](../guide/engine/shaders.md). Decisions:

- **Fields on `Sprite`, not a separate component.** Views have no exclusion filter, so a `Material` component would need a per-entity lookup in `SpriteExtract` or a second extract that queues shaded sprites twice. As fields, an effect toggles without an archetype move, and GOD files and Lua get them through the schema. `Params` is `omitzero`, so existing GOD files stay valid.
- **One `vec4` of parameters.** A fixed block keeps items pointer-free and the queue 0 allocs. Effects that need more data than four floats belong in a texture or, later, a layer shader.
- **Shaded quads in the quad batch, from the frame's sub-image.** A shaded sprite is a quad of its image's size placed by the same `GeoM` as `DrawImage`, drawn with `DrawTrianglesShader32`; the sprite's sub-image is `Images[0]`, so `imageSrc0At` is clipped to the frame and `imageSrc0Origin`/`imageSrc0Size` are the frame, exactly as with the `DrawRectShader` call it replaced (see [Batching](#batching)). A nil shader (zero, loading, failed) takes the plain path, which keeps the placeholder rule.
- **Uniforms without allocations.** Ebiten takes uniforms as `map[string]any`, and storing a fresh value in it boxes and allocates. The renderer builds the map once with a `[]float32` behind `Params` and overwrites its elements per draw. Ebiten ignores names a shader does not declare, but panics on a type mismatch, so the shader loader draws each new shader once into a 1x1 image with the same map and turns that panic into a load error.
- **Canvases take shaders too.** `drawCanvas` branches into `drawShaded` like `drawSprite`, so a chunked tile layer's shader runs on the baked chunk image each frame. The chunk is baked without the shader (`bake` paints plain tile items), otherwise `Time` would be frozen into the image. Particles needed no renderer work: `particles.Extract` already queues one `KindSprite` item per particle, so the emitter's `Shader` and `Params` ride on it, and consecutive particles of one frame share a shaded batch. A tile layer's shader is a map dependency (property `shader`), so a bad one fails the map instead of drawing unshaded.
- **A built-in `Time` uniform, set once per frame.** Every uniform map also holds a `Time` entry, and all of them point at the same one-element `[]float32`, so `Renderer.SetTime` writes one float and every shader of the frame (sprite, group, pass, final) sees it, with no allocation. `Time` is a `float`, so the loader check covers it too. The renderer does not read a clock: whoever drives it decides what time means. `game.Engine` passes simulation time (tick durations summed by `Update`) interpolated with the draw's alpha, so shader motion is as smooth as interpolated positions, stops when updates stop, and is reproducible in replays; a wall clock would keep running while the game is paused and differ between runs. It is a `float32` in seconds; precision loss after hours is left to the shader (`mod`) rather than wrapped by the engine, because no period suits every effect.

## Layer groups

A `GroupConfig` maps disjoint layer ranges to offscreen images (`GroupOptions`: `From`..`To`, `Shader`, `Params`, `Blend`, `Clear`, `SampleBelow`). How to use them: [shaders guide](../guide/engine/shaders.md#layer-groups). Decisions:

- **Ranges of layers, not a per-layer flag.** Layers already order the queue, so a group's items are contiguous after `Sort`. `Draw` walks the groups (sorted by `From`) alongside the items: when an item's layer enters a group the target switches to the group image, when it leaves the image is composited onto the screen. No per-item lookup, no second pass over the queue. Ranges may not overlap (panic), so there is no nesting.
- **Screen-sized images, reused.** One image per group at the logical screen size, reallocated only when that size changes. Clearing it each frame is one fill. A copy for `SampleBelow` is one more image shared by all groups.
- **Empty groups still composite when `Clear` is visible.** A light map with an ambient color must darken the scene even on a frame with no light sprites; a transparent group with no items is skipped, so an unused group costs nothing.
- **`NoComposite` instead of a second "layer to target" mechanism.** Groups already route a layer range into an offscreen image with no per-item cost; what was missing was reading that image from a pass and skipping its composite. A `NoComposite` group never composites, not even empty with a visible `Clear`, and is not counted in `Stats.Groups`. Group images are the logical screen size; reduced-resolution layers would need a scaled transform per item and are not built.
- **`BlendMultiply`** (`dst = src*dst + dst*(1-srcAlpha)`) is the light-map blend: a group cleared to the ambient color, with additive light sprites, multiplied onto the scene. It is also usable per item (shadows).
- **`BlendScreen`, `BlendSubtract`, `BlendLighten`, `BlendErase`** are fixed-function Ebiten blends on premultiplied colors (screen: factors One / OneMinusSourceColor; subtract: ReverseSubtract; lighten: Max; erase: `BlendDestinationOut`), so they batch like the others. **No Darken:** the Min operation ignores blend factors, so the all-zero transparent parts of a sprite or group image would black out the destination. **No overlay or soft light:** they need the destination inside a non-linear formula, which is what `SampleBelow` shaders are for. `Erase` belongs inside a group (fog of war, flashlights); a group's own `Blend` may be Erase too, which erases to the window background.
- **The group shader is a normal Kage shader** with the same `Params` uniform map as sprite shaders (built once, 0 allocs), drawn as one screen quad with `DrawTrianglesShader32` (`drawShaderQuad`, shared with post passes and the final pass). `imageSrc1` is the copy of the scene below, when asked for: sampling `dst` directly is not possible in Ebiten.
- **`Set` between frames.** Changing `Params`, `Shader`, `Blend` or `Clear` allocates nothing (the clear color is boxed once per change), so systems can animate groups every tick. `game.Engine` registers its `*GroupConfig` as a world resource for that.

## Batching

Sprites (tiles and particles included), canvases and the segments of shapes are textured quads. `Draw` collects consecutive unshaded quads that share the target image, the source texture and the blend mode, and draws them with one `DrawTriangles32` call; consecutive shaded quads that share the target, the source image, the blend mode, the shader and `Params` go into one `DrawTrianglesShader32` call (`render/batch.go`). Decisions:

- **Batch in the renderer, not in a separate item kind.** Every system already queues one item per sprite or tile; batching them where they are drawn gives tilemaps, particles and ordinary sprites the speedup without a new API, and keeps draw order exactly as sorted, because triangles of one call draw in index order.
- **Why, when Ebiten merges `DrawImage` calls anyway.** Ebiten turns consecutive `DrawImage` calls into one GPU command, but each call still pays its own CPU setup. With 10k quads, that setup is most of `Draw`: `BenchmarkDraw10k`, `BenchmarkDrawTiles` and `BenchmarkDrawParticles` (`-tags ebitentest`, batched against unbatched) measure about 5x less CPU time per `Draw`.
- **Atlas frames are drawn from their texture.** A frame's `Image` is a sub-image, and different sub-images are different source images; the batch uses the whole texture (`Sprite.sheet`) with the frame's rectangle as source coordinates, so all frames of one atlas, and so all tiles of one tileset, share a batch. Both paths use Ebiten's unchecked address mode, so they sample the same pixels.
- **Same pixels as `DrawImage`.** Corners are computed with the float32 matrix and the destination snapping that `DrawImage` uses (`snapPixel`, a copy of Ebiten's `adjustDestinationPixel`); `DrawTriangles` does not snap on its own, and without it half-pixel positions covered different pixels. `TestDrawBatchedMatchesUnbatched` draws a scene both ways and compares every pixel.
- **Shaded batches use the frame, not the sheet.** A shader must see its frame as `imageSrc0`, so a shaded batch draws from the frame's sub-image and different frames of one atlas are different batches. Ebiten could not merge them on the GPU anyway: it passes every source region to the shader as uniforms and merges only draws with equal uniforms, so shaded quads of different frames, or with different `Params`, are one GPU draw each (and each such draw allocates inside Ebiten). What the shaded batch saves is CPU: for 10k shaded sprites of one frame and one `Params` (particles of one effect), `BenchmarkDrawShaded` measures about 6x less CPU per `Draw`; where every sprite starts a new batch it costs about 5% more than one `DrawRectShader` per sprite (building the vertices on our side). `TestDrawShadedMatchesDrawRectShader` compares every pixel with `DrawRectShader`.
- **One pending batch.** Unshaded and shaded quads share the batch and its buffers; a quad with another key flushes it, so draw order stays exactly as sorted.
- **What breaks a batch.** Another target (a layer group), another texture (for shaded quads: another frame), another blend mode, shader or `Params`, a switch between shaded and unshaded, and every item that is not a quad: text flushes the pending batch before it draws. The shader/texture sort key keeps those breaks rare inside a layer; Y-sorted layers that mix textures batch less.
- **Not built: sheet batching for shaders.** Drawing shaded frames from the whole sheet, with the frame rectangle or `Params` in the vertices' custom values, would merge them on the GPU too, but it changes what `imageSrc0` means to every shader. Chunked tile layers already run a layer shader once per chunk.
- **Reused buffers.** The vertex slice and the fixed index pattern grow to the largest batch and are reused, so batching allocates nothing in steady state. Tint goes into the vertex colors (premultiplied, like `DrawImage`'s `ColorScale`).

## Stats

How to read them: [rendering guide](../guide/engine/rendering.md#stats-and-the-overlay). Decisions:

- **The culled count travels in the `Queue`.** Culling happens in extract, which the renderer never sees, so extract systems add their totals with `Queue.AddCulled` (once per `Run`, not per item) and `Draw` copies `Queue.Culled` into `Stats`. `Reset` clears it with the items, so it is always the count of the tick being drawn.
- **Placeholders are one pointer compare.** `asset.Store.Get` returns the placeholder slot's pointer for every handle that is not a ready asset, so `Draw` fetches that pointer once (the zero handle) and compares. No `Lookup`, no state read per item.
- **The overlay lives in `render`, not `game`.** It needs only `Stats` and Ebiten, so games without `game.Engine` can use it. It uses Ebiten's debug font so it works before any font loads, and it may allocate (one string per frame) because it is off in release builds; `Renderer.Draw` stays 0 allocs.

## Post-processing

`PostChain{Passes []Pass; Targets []Target; Final *Pass; Fade gfx.Color}`, `Pass{Shader, Params, Off, Src, Extra, Out}`. How to use it: [shaders guide](../guide/engine/shaders.md#post-processing). Decisions:

- **Game code owns the chain.** The renderer keeps the pointer (`SetPost`) and reads it in every `Draw`; `game.Engine` registers it as a world resource so systems change `Params` or `Off` between frames. No copy, no per-frame setup.
- **Zero overhead when nothing runs.** `Draw` first checks whether any pass is on with a loaded shader. If none is, it draws straight onto `dst` and touches no offscreen image; output is identical to a renderer without a chain.
- **Copy, then ping-pong.** With passes, `Draw` copies `dst` into the frame image (so the background the caller filled is part of the frame), draws items and groups into it, then runs the passes alternating between the frame image and a second one; the last pass draws onto `dst`. The second image exists only for chains with two or more running passes. Both follow the screen size like group images.
- **Passes are a small graph.** A pass reads `Src` and up to three `Extra` images and writes `Out`: the chain, or a target declared on the chain. Targets are typed IDs (`i+1` for `Targets[i]`), not strings, so a typo cannot compile and a pass costs no string compare. The chain keeps its ping-pong; targets are separate images sized `ceil(screen/Scale)` and allocated on first use. Legacy passes leave all three fields zero and behave and cost as before.
- **Every pass is one `DrawTrianglesShader` quad.** `DrawRectShader` needs every source image to have the rectangle's size; `DrawTrianglesShader` in pixel units does not. The quad spans `Out` and its source coordinates span `Src`, so downsampling needs no scale uniform: the shader runs per small pixel and `src` is the block corner. One code path serves legacy and graph passes; the pixel tests of the old chains prove it renders the same. Kage takes the position of `imageSrcNAt` in `imageSrc0`'s texture, which is why the guide maps `src` and adds `imageSrc0Origin()`.
- **The frame survives only when asked for.** `TargetFrame` is the first image. A pass that writes the chain twice overwrites it in the plain ping-pong, so a chain that reads `TargetFrame` after a chain write gets a third image. Chains that do not pay nothing.
- **Previous frame by history images.** A running pass reading `TargetPrevious` makes the chain's last pass write one of two history images, which is then copied to the screen (two images and a copy, only then); the pair swaps every frame. The first frame and the first after a resize seed the previous image from the current frame, so feedback effects do not flash black. The images are freed when no running pass reads it.
- **Groups are input-only targets.** `TargetGroup(id)` is `TargetID(-3 - id)`, so `Src` and `Extra` need no new fields and `Pass` keeps its size; the encoding is private to `groupOf`. A pass sees the raw group image (items over `Clear`, before shader and blend). Each group records the frame it was last cleared in (`Renderer.begun`), and `postInput` begins a group that no item reached this frame, so a pass never reads stale pixels and groups nobody reads cost nothing. Writing a group is a panic: group images are written by the queue.
- **Validation per Draw.** Running passes are checked each frame (unknown target or group, reading and writing one target, writing `TargetFrame`/`TargetPrevious` or a group); the loop is allocation-free. Skipped passes are not checked and write nothing, so their `Out` keeps its last content.
- **Bloom is a helper over this graph, with built-in shaders.** `AddBloom` appends three targets and five passes; its Kage sources (`render/shaders/`, embedded) are compiled by the Renderer the first time a pass uses them, through an unexported `Pass` field, so they need no asset scope and no loading frames. A compile error panics (it is an engine bug; `TestBuiltinShadersCompile`). The glow is 8-bit; HDR targets are not built.
- **Passes replace, they do not blend.** Each pass uses `BlendCopy` over the whole frame, so no image needs a clear and a pass fully decides each pixel.
- **The final pass is separate** (`Final`, `Renderer.DrawFinal`), because pixel-art games want most effects at the logical resolution and only the CRT mask at window resolution. `DrawFinal` has the `ebiten.FinalScreenDrawer` signature and draws one quad of the frame's size placed by Ebiten's `geoM` (`drawShaderQuad`, snapped like `DrawRectShader`; `TestDrawFinalMatchesDrawRectShader`), so the shader runs per window pixel while sampling the frame in logical pixels; without a final pass it calls `ebiten.DefaultDrawFinalScreen`.
- **Own uniform maps.** Passes and the final pass each reuse a map built once (see [Shaders](#shaders)), so changing `Params` every frame allocates nothing.
- **Fade without a shader.** `Fade` is the renderer's white pixel stretched over the frame with a color scale, drawn after the passes (or straight after the items without them) and before `Final`. A built-in fade needs no shader asset to ship or wait for, and costs one draw only while its alpha is above zero. The zero value means no fade, unlike a sprite tint, so an untouched chain does nothing.

## Canvases

`render.Canvases` exists so that packages outside `render` can pre-render without importing Ebiten: `tilemap` builds a `Queue` of tiles and asks `Canvases.Paint` to draw it into an offscreen image, then queues `KindCanvas` items that name the image by `CanvasID`. Decisions:

- **Not an asset.** Canvases are made and freed by systems at run time, not loaded from keys, so they have their own generational ID (8 bytes, pointer-free like a handle) instead of a slot in the asset server.
- **Painted with a `Renderer`.** `Canvases` owns a private renderer, created by the first `Paint`, so a canvas looks exactly like the items drawn one by one (same placement, flips, snapping and batching). `tilemap`'s pixel test relies on that.
- **Painted in `Update`.** Ebiten allows drawing into offscreen images outside `Draw`, and the extract that knows what to paint runs in `Update`; deferring the paint to `Draw` would need a second queue.
- **Sort key 0.** Canvas items sort with the placeholder texture, before the sprites of their layer, and keep their insertion order among themselves (the chunk render order).

The tilemap side (chunk cache, bake budget, eviction, hot reload) is in `tilemap/chunks.go`; its behavior is in the [tilemaps guide](../guide/engine/tilemaps.md#chunk-pre-rendering).

## Costs

Sprites (shaded or not) and shapes reuse draw options (`GeoM.Reset()` per item), cached sub-images and the batch buffers; shapes are scaled copies of a 1x1 image, so they allocate nothing. Text may allocate (Ebiten's text layout). Measured numbers: [engine guide](../guide/engine/README.md#performance). `TestTickNoAllocs` asserts the whole render stage is 0 allocs; a new extract system gets a check there.

## Particles

`gfx.Particles` (a handle to a `*.fx.json` effect) is drawn through the batched quad path, so the renderer needed nothing new ([guide](../guide/engine/particles.md)). Decisions:

- **Particles are not entities.** Thousands of short-lived entities would churn archetypes and need deferred deletes. A pool is a reused slice with swap-remove, owned by `particles.System` (one pool per emitter entity); the component stores only a runtime pool id.
- **Plain `KindSprite` items, no new kind.** `particles.Extract` pushes one item per live particle and the batcher merges them. If profiles show the 120-byte item copy matters (see [Items and sorting](#items-and-sorting)), a span item that carries many quads is the follow-up.
- **Deterministic randomness.** Each pool owns a `math/rand/v2` PCG seeded from `Seed` or the entity, so frame-log and replay comparisons stay meaningful.
- **Pool lifecycle without hooks.** The system marks the pools it sees in a tick and releases the unseen ones (deleted entity, removed component). A pool records its owner entity, so a copied `Pool` id (a cloned entity) gets its own pool instead of sharing one. Released pools go to a free list and keep their buffers.
- **Schedule slot.** The system runs in the `anim` stage after the `AnimationSystem`, which keeps running under `TickPause`, so effects play live in the editor; the extract runs in `render-prepare` right after the `SpriteExtract`.
- **The effect is an asset.** A `*.fx.json` file (kind `effect`) holds one to eight emitters, so effects are shared, hot-reloaded and edited as files; the component keeps only per-instance state (layer, seed, stop). **Several emitters, one entity:** the system keeps one pool per entity with a lane (buffer, cycle time, burst, rate carry, random stream) per emitter, so stopping, moving and overriding an effect is one operation. Instance overrides apply to every emitter; per-emitter overrides are not built. `Live` is the sum and `Done` the conjunction over the lanes, `Time` is the first lane's. Emitter `i` seeds with a value derived from the seed and `i`, so equal emitters do not spawn identical particles. A hot reload restarts a lane when its cycle key changed and the whole pool when the emitter count changed. Extract queues each lane at the component's `Layer` plus the emitter's `layer`. A cap of eight bounds the pool memory (each lane has its own `max`). Editing a file restarts a pool only when a key that shapes the cycle changed (duration, loop, burst, max, shape, shapeSize); other keys apply in place so live particles survive a drag in the tool. The file is not a prefab, so it has no variants. `bridge.Version` is unchanged: the protocol only learned the `.fx.json` suffix as an asset kind.
- **The file format is its own package.** `particles/fxfile` holds `Emitter`, `Spec`, `Decode` and `Encode` and imports only `gfx` and `xmath`, so the `fx` tool and `wpix mcp` check effects exactly as the loader does without linking Ebiten (as `anim/animfile` does for animations). It sits under `particles` because the format belongs to that runtime package. It keeps the `gfx` blend, shape, color and vector types instead of its own, so the `gfx.Particles` overrides and the emitter share one type; a stdlib-only copy would make `gfx` import the format package. Defaults stay where they are applied: the simulation reads a zero value as its default, `fxfile` only documents them.
- **Rate is accumulated, not rounded**; spawns beyond `Max` are dropped, not queued.
- **Per-instance overrides are flat optional fields.** `gfx.Particles` has one pointer field (`omitzero`, `nullable` in the GOD) per `Emitter` value, nil meaning "from the effect", so a level lists only what it overrides. A nested `Overrides` struct would add a JSON level for nothing, and a bitmask beside value fields would list every value in every level. `Sprite` and `Shader` are handles (the asset marker needs a non-pointer), so the zero handle means "from the effect". `particles.System` resolves the emitter every tick instead of caching (about 30 nil checks and a copy, 0 allocs): a base or override change of a cycle key then restarts the pool through the existing `cycleKey` comparison, and every other value applies at once. Pointees are immutable because clones share them. The fx tool edits the file, not instances.
- **The Transform applies at spawn.** `Rot` and `Scale` place each new particle (direction, shape, start rotation, size); live particles don't follow later turns or scaling, and `Local` follows position only. Following rotation would need emitter-frame offsets and blended rotation in the interpolation, which no effect has needed yet.

Not built: flipbook particles, curves and gradients, collisions, sub-emitters, trails, prewarm, editor gizmos for the spawn shape.

## Tweens

Package `tween` animates component fields ([guide](../guide/engine/tweens.md)). It follows the particles' "state owned by a system, outside the World" pattern. Decisions:

- **Tweens are not components.** A `Tweens` resource owns every running tween, so there is no archetype churn per tween, no slot limit per entity and nothing in the GOD, levels or the inspector. Tweens are runtime behavior started by code; authored ambient motion (a `tween:Loop` component on a level object) waits for a real need.
- **Typed lenses, one pool per lens.** A `Lens[C, V]` holds a getter and a lerp; each lens gets a `pool[C, V]` of value rows with swap-remove, behind a small interface so `Tweens` runs all pools in one loop. Per row and tick: a component lookup through a `View1[C]` the pool holds (`GetOrNil`; the generic `ecs.GetCompOrNil[C]` would look the type up on every call), the ease, the lerp, a write through the getter. No strings, reflection or `unsafe`, and the value type is checked at compile time. Lens ids are process-wide, so a `Tweens` finds a pool by slice index; `Param(i)` returns prebuilt lenses.
- **Generational IDs.** An `ID` indexes a slot table that points to (pool, row); swap-remove updates the slot of the row it moves. Freeing a slot bumps its generation, so every call on a finished or killed tween is a no-op. Kill and finish only mark a row dead and free its ID; the pool removes dead rows in its next run, so nothing moves while a pool iterates.
- **Overwrite at start.** A map from (entity, lens) to ID, touched only when tweens start and end, kills the older tween on the same field. Additive tweens skip it.
- **Relative and Additive through the lerp.** A lens has no add function. For vector-like values `a + b - c` is an affine combination, so two lerps compute it (`lerp(c, lerp(a, b, 0.5), 2)`); relative targets and additive steps both use it. It is why lerps must extrapolate, and why both options are meant for numeric values, not colors.
- **Callbacks after the pools.** `OnDone`, sequence release and `DespawnOnDone` are queued in a reused slice and run after every pool has run, so a callback can start and kill tweens safely.
- **Schedule slot.** `tween.System` runs first in the camera stage: tweens started in `simulate` run in the same tick, the follow camera sees the result, `PrevPos` was snapshotted before, and the gated stage stops tweens under `TickPause`. The editor's Stop clears them.

Not built: authoring components and edit-mode preview, saving running tweens, Lua bindings, lenses over resources (the camera), keyframe tracks and path following.

## Nine-slice panels

`gfx.NineSlice` ([guide](../guide/engine/components.md#nineslice)) is one queue item (`KindNineSlice`), cut into pieces by the Renderer (`render/nine-slice.go`), not by the extract. Decisions:

- **Slice in `Draw`.** Only the Renderer knows the frame size, which a zero `Size` and the inset clamping need, and the sprite may still be loading (the placeholder is cut like any frame). The extract copies the component into one item, so a panel costs one queue slot and no per-piece work in the schedule.
- **Nine quads, one batch.** Each piece is a sub-rectangle of the sprite's texture through the same `addQuad` as sprites, so a panel and the sprites around it share a draw call. The sort key treats the kind like `KindSprite`, so panels group by texture.
- **Clamping.** Insets are clamped to the frame; when the panel is smaller than its borders they shrink in proportion and the center vanishes, so there are no gaps or overlaps at any size. Tiling crops the last repeat (a fractional last repeat squeezes the next whole source pixel into the remaining space).
- **Conservative culling.** The extract adds the panel's scaled diagonal to the culling margin, so no origin or rotation can cull a visible panel; it does not know the frame, so a zero `Size` culls by pivot.
- **`ScreenNineSlice` is runtime-only,** like `ScreenSprite`; `gfx:Panel` places a world panel in levels. Trim offsets, flips and shaders are not supported.

## Startup screen

The startup screen ([guide](../guide/engine/engine.md#startup-screen)) lives in `game` because it draws with Ebiten. Its timing is a small Ebiten-free state machine (`startupSeq`) tested on its own: a screen starts leaving once its `Duration` minus the fade has passed (or on a skip), and the last one only once the initial load is done, so a fast load ends exactly at `Duration`.

Holding the game reuses the `gate` that a Tool's `TickPause` uses: the gate is installed when a Tool is set or the startup screen is on, and while held it runs only the engine's input system, so device state stays current for the skip press. The press is read from the router's raw frame as the change from nothing held to something held. The screen is drawn in `DrawFinalScreen` at window resolution, so a large image stays sharp in a small logical resolution, and `Draw` skips the half-loaded game. Time is simulation time, which keeps tests deterministic. Everything is dropped when the last screen ends, so the steady-state tick stays 0 allocs; the readiness check walks scope closures and may allocate, only while the screen is up.

The default image is a JPEG re-encode of the original artwork (720x720, quality 85, about 50 KB; `images/weakpixel-screen.png` is a 640x640 palette PNG of about 165 KB for the README); `game/startup/README.md` says how to redo it.

## Rooms

Package `room` moves the game between levels through exits ([guide](../guide/engine/rooms.md)). It sits on top of `game` and is opt-in. Decisions:

- **Exits and spawns are GOD components**, not Tiled objects, so the editor validates and places them with prefabs and overrides; spawn names also match Tiled objects, and game code can still fire `GoTo` from Tiled areas.
- **The entering edge comes from `PrevPos`.** The engine snapshots `PrevPos` before the game's systems, and the director runs in `StagePost`, so `Contains(Pos) && !Contains(PrevPos)` is "walked in this tick" with no per-traveler state. Placing sets both, so a spawn inside an exit does not fire it.
- **Load once covered.** The target group loads when `Out` ends, not during it: its map spawns at its world position and would show through a half-covered old room. The swap (detach travelers with `Level.SetObjects`, unload, `OnEnter`) happens as soon as the group is ready; placing waits for every level of the new room, so `OnEnter` can add levels (`SetCurrent`) that hold the spawn.
- **Hold only simulate.** `Engine.Hold` skips `StageSimulate` alone, so tweens, the follow camera and input keep running and transitions can animate with them. It wraps the slot around the gate of the Tool and the startup screen, so either can hold the stage; a held tick costs one branch.
- **Shader transitions borrow a post pass** that the director appends at `Begin` and removes at `End`, and find again by shader handle if game code changed the chain.
- **The timing is a separate state machine** (`seq`, one event per tick), tested without an engine.

Not built: slide transitions with both rooms visible, preloading during `Out`, saved room state, an editor gizmo beyond the debug shape, Lua bindings.

## Collision

Package `collision` detects overlaps ([guide](../guide/engine/collision.md)); package `physics` moves bodies with the same `Collider` and `Space` ([guide](../guide/engine/physics.md)). Decisions:

- **Detection and physics are separate packages over one data model.** A trigger, a coin or a hit area needs only a `Collider`; a body adds a physics component. The shape and the filter live once.
- **"Mask is what I react to", one event per detecting side.** Game code never swaps A and B: the actor is always `Self`. Physics uses the same `Detects` rule for who is pushed.
- **No hash map in the broad phase.** The grid is a slice of packed `uint64` entries (row, column, proxy) sorted each tick: rebuilding reuses memory, sorting plain integers is several times faster than sorting structs with a comparison function, and a row's cells are consecutive keys, so a query does one binary search per row. A pair that shares several cells is reported only by the cell holding the top-left corner of the two bounds' intersection.
- **Statics are compared, not flagged.** The System compares each tick's static shapes with the last run's and rebuilds their grid only on a difference, so moving a "static" collider never needs an explicit call.
- **No Stay event by default.** Ongoing contact is state, read with `Space.Contacts`; a per-tick event for every held pair would fill the queue for floors and crowds. `Config.Stay` opts in, sent from the same merge as Enter and Exit.
- **Enter/Exit is a merge of two sorted key lists** (this run's detecting pairs against the last run's), no map. Contacts are sorted by `Self`, which also makes `Contacts(e)` a binary search.
- **Rays walk the broad phase in cell-long pieces** and stop at the first piece that ends behind the best hit, instead of querying the ray's bounding box (a long diagonal ray would test every collider in the square it spans).
- **The anim body is copied by `anim/animbody`, not read by `collision` and not written by `anim`,** so `collision` stays free of `anim` and `render` and `anim` stays free of `collision`; the fixed `body` is used, never per-frame boxes, so a solid box does not change shape mid-animation.

Not built: rotated shapes and polygons, slopes and partial tiles, tile grids at a map offset, per-frame hit and hurt boxes as proxies (several per entity, not entities), continuous detection, Lua bindings, named layers in the editor.

## Physics

Package `physics` moves bodies on top of `collision` ([guide](../guide/engine/physics.md)). Decisions:

- **Physics drives detection.** `collision.System` is split into `Detect` and `Publish`; physics detects once per substep and publishes once per tick, so events stay once per tick and detection code exists once.
- **Upright bodies, dynamic and kinematic only**. Dropping rotation removes the hardest part of a 2D solver; pixel-art bodies stay upright. Static is a `Collider` without a `Body`.
- **Detect, solve velocities, then move** (the Box2D order). Contacts found at the start of a substep are resolved on the velocities before positions move, so gravity never pushes a resting body into its floor; the cost is that contacts and flags lag positions by one substep.
- **Sequential impulses with warm starting, then position correction.** Accumulated impulses are kept per contact between substeps (a map keyed by pair and tile cell), so a stack settles with the default four passes. Position correction takes the largest push per direction and axis instead of the sum, so a box on two tile cells is not lifted twice.
- **Mirrored contacts become one constraint.** When both sides detect each other, collision reports two contacts; solving both would push the pair twice.
- **Internal tile edges are dropped** by checking the neighbor cell in the normal's direction, the classic fix for snagging on tile seams.
- **One-way contacts persist.** A one-way contact is accepted when the body is no deeper than it moves in a substep, and kept while it lasts, since position correction leaves a body that landed deep inside for a few substeps.
- **Riders get platform-relative velocity.** A dynamic body on a kinematic body moves by the platform's velocity and keeps its own velocity relative to it, switching frames when its ground changes. Game code that writes `Velocity` every tick then walks on a moving platform as on solid ground, and leaving it keeps the platform's momentum. A stack on a kinematic body shares its frame: `carrierOf` follows the grounds down the stack (at most 8 bodies, which also ends a cycle) to the kinematic body, so the bodies in it keep their relative velocities when the platform changes speed. Riders on dynamic bodies no platform carries use friction instead: zeroing a dynamic carrier's velocity in the contact would break momentum exchange in stacks. `MoveAndSlide` movers are carried from their `Touching.Ground`, as the last move wrote it, by the step of the body they stand on.
- **No per-body resolver**: a contact has two sides, and mixed resolvers in one pile do not converge. `Material`, `MoveAndSlide` and `PreSolve` cover per-object behavior; `Config.Resolver` replaces the solver for the whole World.
- **`Follow` instead of tweening positions**: tweens run after physics, so a tweened position is a teleport. `Follow` turns the tween's target into a velocity at the start of the tick.
- **`MoveAndSlide` is axis-separated and reads the Space only.** It is what character controllers in pixel-art games use (X then Y, stop flush), it works with detection alone, and it never touches the Space's proxies. It reads the candidates from the Space with a 16 unit margin and tests the current bounds of non-static colliders, because bodies move after detection. An overlap of up to one unit blocks (the mover stays where it is) so a body pushed into it or a platform rising into it does not let it fall through; deeper overlaps pass so it can always get out.
- **`MoveAndSlide` pushes by displacement, not impulse.** With `MoveOptions.Push`, it moves the pushed bodies' positions in the same call, through a sweep with each body's own collider, and leaves their velocity alone. An impulse would land one physics step later, by when the mover overlaps the body by more than the one unit it tolerates and walks through it. Leaving the velocity alone gives the usual platformer crate that stops when the player stops; friction and gravity still act on it in the solver. Mass enters as a speed share (`Push / (Push + M)`) instead of a threshold, so heavy crates feel heavy without a cutoff.

Not built: rotation, joints, sleeping, continuous collision, slopes, carrying riders on dynamic bodies no platform carries, bodies pushing `MoveAndSlide` movers, per-level gravity, a level-editor path mover, Lua bindings. The pile benchmark spends about a fifth of its time in map lookups (warm-start and impact maps); a slice-based contact cache is the next step if it matters.

## Scenes

`game.Scene` stacks the top-level screens of a game ([guide](../guide/engine/engine.md#scenes)). Decisions:

- **One World, scene-owned systems and entities.** Resources (camera, tweens, input router, assets) stay shared, and moving the hero between scenes needs nothing. Isolation comes from ownership and from which systems run, not from separate storage.
- **The root scene is the Engine's own systems.** `Engine.AddSystem` fills the root, which is always the bottom of the stack, so a game without scenes changes nothing. It follows the same rules as any scene below the top: a pause scene with `UpdateBelow: false` freezes it.
- **Applied between ticks.** `Push`, `Pop` and `Replace` queue; `Update` applies the queue at its start, after the Tool tick and before the schedule runs. A scene's runners are initialized outside the schedule (which can only append, and initializes what it was built with), with a context built for that call, so the first push of a scene before the very first tick runs `Init` before the root's.
- **A dispatcher per stage.** The schedule is built once, so each of the four game stage slots holds one dispatcher runner (after the engine's input system, tweens and follow camera) that ranges over a list rebuilt when the stack changes: the root's runners, then the active scenes', and for the input stage only those allowed by `InputBelow`. Active and editing lists (for a Tool's pause, split once per rebuild by `EditRunner`) are precomputed, so a tick allocates nothing. The dispatcher sits inside the existing gates (Tool pause, startup, `Hold`), which therefore apply unchanged.
- **Engine stages are not per scene.** Assets, snapshot, tweens, camera follow, animation, particles, extraction and audio run once, as under `Engine.Hold`. Per-scene time, tweens or animation freezing would need those stages to know the scene of an entity; deferred.
- **Drawing below by a layer floor.** Everything draws into one queue and an entity does not know its scene, so `DrawBelow: false` uses the scene's `Layer`: `Queue.Sort` drops the items below the floor of the topmost such scene (counted as culled), one compare per item and no change to the extract systems. The cost is the layer-band convention in the guide.
- **Rooms** take an optional scene (`room.Config.Scene`): the director is added to it and rooms load through it, so replacing the gameplay scene unloads the room. Editor and Tool need nothing: the editor opens levels in the Engine (root-owned) and scenes keep their stack while a Tool pauses.

Not built: one World per scene and scene-local resources, scene transitions (use `PostChain.Fade` or rooms), per-scene time, saving a popped scene's state, scenes in GOD files and Lua bindings.

## Hierarchy

`hierarchy.Parent` attaches an entity to a parent ([guide](../guide/engine/hierarchy.md)). Decisions:

- **`gfx.Transform` stays world space.** Extract, collision, physics and the camera read it unchanged; the child's `Transform` is derived data that a system overwrites. The cost is that a tween or a physics write on a child's `Transform` does not stick, and that colliders on children are a tick late.
- **Walk up, never order.** For each child the system collects the chain to the root in a fixed array and composes the `Local`s onto the root's `Transform`. It reads no intermediate `Transform`, so there is no depth tag, no sort and no child list to keep in sync with deletions: O(children x depth) with a depth cap of 32, 0 allocs. A cycle shows up as a chain that does not end and panics; documents are checked on load (`CheckCycles`) so data cannot cause it.
- **Layers walk too.** A relative ancestor's own layer is derived, so reading it would depend on visiting order; the walk adds its `LayerOffset` and goes on to the nearest ancestor whose layer is authored (absolute or root).
- **Y-sort by the root.** A child's `SortY` is the root's Y plus or minus 1/1024, stored by the system in `Parent` (`RootY`) and read by the sprite and nine-slice extracts through a second view, while the plain view excludes `Parent` so its loop is unchanged. A group of parts therefore sorts as one against other actors.
- **A stage of its own, ungated.** It sits between `post` and `anim`, so the placement sees the tick's movement, and runs in edit mode so the editor moves children with their parent. After a document spawns, the assets stage runs it once early (only when something spawned), so children are placed before the snapshot.
- **Absolute placement is a mode, not a missing component.** `Local.PlaceMode` (`PlaceAbsolute`) keeps `Local` inside `Parent`, so the mode toggles in place without an archetype move or a file migration, and it is an enum like `LayerMode` so an upright mode can be added later. A child without `Transform` cannot be the absolute case (everything that draws or collides reads `Transform`), and splitting `Local` into its own component was declined for the migration and archetype moves it costs.
- **Absolute children are placed once, and `Local` is never written.** The first placement composes `Local` as for a relative child, which is what makes prefab parts and level instances start in the right place without the prefab code knowing about the hierarchy (`god` does not import it). After that the `Transform` is authored, so the walk reads it as a root and the visiting order stays irrelevant. Mirroring `Local` from the `Transform` each tick was declined: `Local` would have two meanings, and `SetPlace(..., keepWorld)` gives the no-jump switch back to relative on demand.
- **Orphans are deleted by the system**, from the chain walk, after the loop, so a whole subtree goes in one pass without child lists.

Not built: running the propagation inside physics substeps, a physics warning for bodies on children, inheriting `FlipX`, ordered children, named paths, relationship pairs, editor support, Lua bindings.

## Not built yet

- Multiple cameras (split screen, minimap) as several viewports over one queue.
- Engine-level: input abstraction, variable timestep.
