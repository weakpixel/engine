# Engine and renderer design

Why the engine is split into plain-data components, extract systems and an ecs-free renderer, and what the first version leaves out.
The API is in the [engine guide](../guide/engine/README.md); the package layering in [architecture.md](packages.md#engine-runtime-packages).

## Three parts

| Part | Knows about | Runs in | Job |
| --- | --- | --- | --- |
| Components (`gfx`) | plain data, asset handles | n/a | What an entity looks like: pointer-free, handles instead of images |
| Extract systems (`extract`, `anim`) | `ecs` + `render` | `Update`, last stages of the schedule | Walk views, cull, write `render.Item`s into a `render.Queue` resource |
| Renderer (`render`) | Ebiten, not `ecs` | `Draw` | Draw the sorted queue |

Why split:

- **Ebiten's loop forces a handoff anyway.**
  `Update` runs zero or more times per `Draw`, and the World must not be touched in `Draw`.
  A queue built at the end of `Update` is that handoff.
  Ebiten calls both on one goroutine, so no double buffering is needed.
- **The renderer stays testable and swappable.**
  It takes a list of plain structs: sorting and culling are unit-tested without a World, and a queue can be replayed in a debug tool.
- **Ebiten stays out of `ecs`**, and out of `gfx` and `asset`, so tools and servers never compile it.
- **One place owns draw order and batching.**
  Systems say "draw sprite X on layer 3 at (x, y)"; only the renderer decides order and batching.

What is *not* split: deciding what is visible (culling, `Sprite.Hidden`) happens in extract, where the data is.

## The frame contract

```
Update (fixed TPS): assets → snapshot PrevPos → game stages → animation → reset queue → extract → sort
Draw (display rate): Renderer.Draw(screen, queue, camera, alpha)
```

- `AssetSystem` runs first so loaded assets are committed before anything reads them.
- `SnapshotTransforms` copies `Pos` to `PrevPos` before anything moves; `Draw` blends `PrevPos.Lerp(Pos, alpha)` so a 60 TPS simulation looks smooth on a 144 Hz display without running more ticks.
- **Pixel snap after blending, in `Draw`.**
  `Camera.PixelSnap` rounds the blended view once per frame so that viewport center − `Pos`·`Zoom` is an integer.
  It has to happen after blending: rounding `Pos` in the tick and blending two rounded positions gives a sub-pixel view again.
  The camera methods (`WorldToScreen`, `ScreenToWorld`, `WorldBounds`) round the same way, so picking matches the drawn pixels; `Draw` clears `PixelSnap` on its rounded copy so per-item transforms don't round again.
  It is opt-in (the zero value keeps the exact view for non-pixel-art and rotated cameras) and ignored with a rotation.
- **Culling covers the whole tick.**
  `Draw` shows any view between `PrevPos` and `Pos`, so extract culls against `Camera.DrawBounds`, the union of both views.
  Culling against `Pos` alone dropped a sliver at the trailing edge while the camera moved.
  No margin is needed for `PixelSnap`: rounding is monotonic, so the rounded blended view lies between the two rounded end views.
- `game.Engine` fixes this order and builds the schedule on the first `Update`, so user systems can only go into the four slots between snapshot and animation ([engine.md](../guide/engine/engine.md#tick-order)).
- Animation is not a renderer feature: `AnimationSystem` writes `Sprite.ID` (and `Origin` for pivoted clips); the renderer only ever sees the current sprite.

## Items and sorting

- `render.Item` is a fat value (136 bytes): kind, space, layer, sort Y, sprite handle, `Pos`/`PrevPos`, rotation, scale, origin, flips, size, color, blend, shader handle and params, text, font, nine-slice insets.
  The shader fields took it from 96 bytes, which made `BenchmarkTilemapExtract/large` (a pure item-copy loop) about 17% slower.
  Fine for tens of thousands of items and simple to fill.
  If profiles say otherwise, sort a packed `uint64` key slice instead of moving items.
- The queue is reused every tick (`Reset` keeps capacity), so steady state is 0 allocs.
- Sort order: layer, then `SortY` in Y-sorted layers, then the sprite's shader and texture (so draws from one atlas batch together, see [Batching](#batching)), then insertion order.
  The key is one `uint64` per item (shader slot high, texture slot low), computed once per sort.
  The texture key needs the sprite store, hence the optional `*render.Stores` resource.
- `Item.Scale` is used as is: the "zero means 1" rule of `Transform.Scale` is resolved in extract, not in the renderer.

## Zero values

Every zero-valued component draws something sensible: packed `gfx.Color` 0 is opaque white (so the zero tint is "no tint"), `Transform.Scale` zero is (1, 1), `Camera.Zoom` zero is 1, the zero handle is the placeholder, and the zero `Sprite.Origin` is the sprite's default pivot.
That is why top-left needs a tiny non-zero origin.
A packed `uint32` color (instead of `ebiten.ColorScale`) keeps `gfx` free of Ebiten.

## Placeholders

Components hold handles, and the renderer resolves them at draw time through read-only stores.
A handle that is zero, missing, loading, failed or stale draws the placeholder: a magenta checker in development, nothing with `TransparentPlaceholders` in release.
Because the queue holds handles, not images, a freshly loaded texture shows up on the next `Draw` with no extra work.

## Batching

Sprites (tiles and particles included), canvases and the segments of shapes are textured quads.
`Draw` collects consecutive unshaded quads that share the target image, the source texture and the blend mode, and draws them with one `DrawTriangles32` call; consecutive shaded quads that share the target, the source image, the blend mode, the shader and `Params` go into one `DrawTrianglesShader32` call (`render/batch.go`).
Decisions:

- **Batch in the renderer, not in a separate item kind.**
  Every system already queues one item per sprite or tile; batching them where they are drawn gives tilemaps, particles and ordinary sprites the speedup without a new API, and keeps draw order exactly as sorted, because triangles of one call draw in index order.
- **Why, when Ebiten merges `DrawImage` calls anyway.**
  Ebiten turns consecutive `DrawImage` calls into one GPU command, but each call still pays its own CPU setup.
  With 10k quads, that setup is most of `Draw`: `BenchmarkDraw10k`, `BenchmarkDrawTiles` and `BenchmarkDrawParticles` (`-tags ebitentest`, batched against unbatched) measure about 5x less CPU time per `Draw`.
- **Atlas frames are drawn from their texture.**
  A frame's `Image` is a sub-image, and different sub-images are different source images; the batch uses the whole texture (`Sprite.sheet`) with the frame's rectangle as source coordinates, so all frames of one atlas, and so all tiles of one tileset, share a batch.
  Both paths use Ebiten's unchecked address mode, so they sample the same pixels.
- **Same pixels as `DrawImage`.**
  Corners are computed with the float32 matrix and the destination snapping that `DrawImage` uses (`snapPixel`, a copy of Ebiten's `adjustDestinationPixel`); `DrawTriangles` does not snap on its own, and without it half-pixel positions covered different pixels.
  `TestDrawBatchedMatchesUnbatched` draws a scene both ways and compares every pixel.
- **Shaded batches use the frame, not the sheet.**
  A shader must see its frame as `imageSrc0`, so a shaded batch draws from the frame's sub-image and different frames of one atlas are different batches.
  Ebiten could not merge them on the GPU anyway: it passes every source region to the shader as uniforms and merges only draws with equal uniforms, so shaded quads of different frames, or with different `Params`, are one GPU draw each (and each such draw allocates inside Ebiten).
  What the shaded batch saves is CPU: for 10k shaded sprites of one frame and one `Params` (particles of one effect), `BenchmarkDrawShaded` measures about 6x less CPU per `Draw`; where every sprite starts a new batch it costs about 5% more than one `DrawRectShader` per sprite (building the vertices on our side).
  `TestDrawShadedMatchesDrawRectShader` compares every pixel with `DrawRectShader`.
- **One pending batch.**
  Unshaded and shaded quads share the batch and its buffers; a quad with another key flushes it, so draw order stays exactly as sorted.
- **What breaks a batch.**
  Another target (a layer group), another texture (for shaded quads: another frame), another blend mode, shader or `Params`, a switch between shaded and unshaded, and every item that is not a quad: text flushes the pending batch before it draws.
  The shader/texture sort key keeps those breaks rare inside a layer; Y-sorted layers that mix textures batch less.
- **Not built: sheet batching for shaders.**
  Drawing shaded frames from the whole sheet, with the frame rectangle or `Params` in the vertices' custom values, would merge them on the GPU too, but it changes what `imageSrc0` means to every shader.
  Chunked tile layers already run a layer shader once per chunk.
- **Reused buffers.**
  The vertex slice and the fixed index pattern grow to the largest batch and are reused, so batching allocates nothing in steady state.
  Tint goes into the vertex colors (premultiplied, like `DrawImage`'s `ColorScale`).

## Stats

How to read them: [rendering guide](../guide/engine/rendering.md#stats-and-the-overlay).
Decisions:

- **The culled count travels in the `Queue`.**
  Culling happens in extract, which the renderer never sees, so extract systems add their totals with `Queue.AddCulled` (once per `Run`, not per item) and `Draw` copies `Queue.Culled` into `Stats`.
  `Reset` clears it with the items, so it is always the count of the tick being drawn.
- **Placeholders are one pointer compare.**
  `asset.Store.Get` returns the placeholder slot's pointer for every handle that is not a ready asset, so `Draw` fetches that pointer once (the zero handle) and compares.
  No `Lookup`, no state read per item.
- **The overlay lives in `render`, not `game`.**
  It needs only `Stats` and Ebiten, so games without `game.Engine` can use it.
  It uses Ebiten's debug font so it works before any font loads, and it may allocate (one string per frame) because it is off in release builds; `Renderer.Draw` stays 0 allocs.

## Canvases

`render.Canvases` exists so that packages outside `render` can pre-render without importing Ebiten: `tilemap` builds a `Queue` of tiles and asks `Canvases.Paint` to draw it into an offscreen image, then queues `KindCanvas` items that name the image by `CanvasID`.
Decisions:

- **Not an asset.**
  Canvases are made and freed by systems at run time, not loaded from keys, so they have their own generational ID (8 bytes, pointer-free like a handle) instead of a slot in the asset server.
- **Painted with a `Renderer`.**
  `Canvases` owns a private renderer, created by the first `Paint`, so a canvas looks exactly like the items drawn one by one (same placement, flips, snapping and batching).
  `tilemap`'s pixel test relies on that.
- **Painted in `Update`.**
  Ebiten allows drawing into offscreen images outside `Draw`, and the extract that knows what to paint runs in `Update`; deferring the paint to `Draw` would need a second queue.
- **Sort key 0.**
  Canvas items sort with the placeholder texture, before the sprites of their layer, and keep their insertion order among themselves (the chunk render order).

The tilemap side (chunk cache, bake budget, eviction, hot reload) is in `tilemap/chunks.go`; its behavior is in the [tilemaps guide](../guide/engine/tilemaps.md#chunk-pre-rendering).

## Costs

Sprites (shaded or not) and shapes reuse draw options (`GeoM.Reset()` per item), cached sub-images and the batch buffers; shapes are scaled copies of a 1x1 image, so they allocate nothing.
Text may allocate (Ebiten's text layout).
Measured numbers: [engine guide](../guide/engine/README.md#performance).
`TestTickNoAllocs` asserts the whole render stage is 0 allocs; a new extract system gets a check there.

## Nine-slice panels

`gfx.NineSlice` ([guide](../guide/engine/components.md#nineslice)) is one queue item (`KindNineSlice`), cut into pieces by the Renderer (`render/nine-slice.go`), not by the extract.
Decisions:

- **Slice in `Draw`.**
  Only the Renderer knows the frame size, which a zero `Size` and the inset clamping need, and the sprite may still be loading (the placeholder is cut like any frame).
  The extract copies the component into one item, so a panel costs one queue slot and no per-piece work in the schedule.
- **Nine quads, one batch.**
  Each piece is a sub-rectangle of the sprite's texture through the same `addQuad` as sprites, so a panel and the sprites around it share a draw call.
  The sort key treats the kind like `KindSprite`, so panels group by texture.
- **Clamping.**
  Insets are clamped to the frame; when the panel is smaller than its borders they shrink in proportion and the center vanishes, so there are no gaps or overlaps at any size.
  Tiling crops the last repeat (a fractional last repeat squeezes the next whole source pixel into the remaining space).
- **Conservative culling.**
  The extract adds the panel's scaled diagonal to the culling margin, so no origin or rotation can cull a visible panel; it does not know the frame, so a zero `Size` culls by pivot.
- **`ScreenNineSlice` is runtime-only,** like `ScreenSprite`; `gfx:Panel` places a world panel in levels.
  Trim offsets, flips and shaders are not supported.

## Not built yet

- Multiple cameras (split screen, minimap) as several viewports over one queue.
- Engine-level: variable timestep.
