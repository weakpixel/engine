# Rendering (`extract` + `render`)

```
components --extract (Update)--> render.Queue --Renderer.Draw (Draw)--> screen
```

## Extract systems

Each system caches resources in `Init` and pushes `render.Item`s in `Run` (0 allocs).

| System | Queues | Notes |
| --- | --- | --- |
| `NewAssetSystem(w)` | | runs `Server.Update`; publishes `*asset.Events` |
| `NewSnapshotTransforms(w)` | | `PrevPos = Pos`; **first** stage of the tick. Use `.Run` |
| `ResetQueue` | | empties the queue; plain func |
| `tilemap.NewTilemapExtract(w)` | world tiles | `tilemap.TileLayer`: one item per visible cell, or per visible chunk of a `Chunked` layer, culled, parallax, animated tiles; **before** `SpriteExtract` ([tilemaps.md](tilemaps.md)) |
| `NewSpriteExtract(w)` | world sprites | culled against the camera (counted in `Queue.Culled`); skips `Hidden`; field `Margin` |
| `NewNineSliceExtract(w)` | world nine-slice panels | culled by pivot with the panel's size added; skips `Hidden`; field `Margin` ([components.md](components.md#nineslice)) |
| `NewScreenSpriteExtract(w)` | screen sprites | no culling |
| `NewScreenNineSliceExtract(w)` | screen nine-slice panels | no culling |
| `NewLabelExtract(w)` | screen text | no culling |
| `NewDebugExtract(w)` | world shapes | from `gfx.DebugShape`; culled and counted like sprites |
| `NewSortQueue(w)` | | sorts; **last** |

```go
sprites := extract.NewSpriteExtract(w)
sprites.Margin = 32 // about half the largest sprite, so edges don't pop
s.AddRunner(sprites, extract.NewLabelExtract(w))
```

### Writing your own

1. A struct with a prebuilt view and a `resources` value.
2. `Init` caches resources; `Run` loops the view and calls `queue.Push` (no lookups or allocations). If it culls, it culls against `cam.DrawBounds(screen.Size)` (see [Camera](#camera)) and reports the number of skipped items once per `Run` with `queue.AddCulled(n)`.
3. Add it with `AddRunner` between `ResetQueue` and `SortQueue` (with `game.Engine`: `AddRunner(game.StagePost, ...)`).
4. Add an extract test and a check in `TestTickNoAllocs` (`extract`).

```go
q.Push(render.Item{Kind: render.KindRect, Space: render.WorldSpace, Pos: p, PrevPos: p,
	Size: gfx.Vec2{X: 10, Y: 10}, Color: gfx.RGBA(255, 0, 0, 255)})
```

`Queue` API: `Reset`, `Push(Item)`, `Sprite(layer, id, pos, prev) *Item` (push a sprite and tweak it), `Len`, `At(i)`, `Sort(cfg, sprites)`, `AddCulled(n)`/`Culled()` (the tick's culled count, cleared by `Reset`). Item `Kind`, `Space` and `Blend` values are in [kinds.md](kinds.md).

## Draw order

1. `Layer`, ascending.
2. `SortY`, but only in layers marked `YSort`.
3. The sprite's shader (unshaded sprites first), then its texture slot: this keeps batches together, because the renderer only batches consecutive quads with the same texture and blend mode (see [Batching](#batching)). Placeholders share texture slot 0 and sort first, together with [canvas](#canvases) items. Needs `*render.Stores`.
4. Equal items keep insertion order. Tiles are queued before sprites, so in a layer without `YSort` they draw under the sprites of the same layer. A tile's `SortY` is the bottom of its cell.

```go
layers := &render.LayerConfig{}
layers.Set(1, render.LayerOptions{YSort: true}) // top-down actors: lower on screen = in front
ecs.SetResource(w, layers)
```

Configure layers before the first tick. For Y-sorted sprites, `SortY` is the entity's `Pos.Y`; a sprite or panel with a `hierarchy.Parent` uses its root's Y, nudged to the front or back of it ([hierarchy.md](hierarchy.md#y-sorted-layers)).

## Camera

A plain value you write each tick; the renderer reads a copy. To follow an entity without writing a camera system, use the follow camera: [camera.md](camera.md).

```go
cam := &render.Camera{Zoom: 2, Pos: gfx.Vec2{X: 400, Y: 300}}
// in your camera system, BEFORE moving it:
cam.PrevPos = cam.Pos
cam.Pos = target
```

| Field | |
| --- | --- |
| `Pos` / `PrevPos` | world point at the viewport center |
| `Zoom` | world units to pixels; 0 = 1 |
| `Rotation` | radians, clockwise |
| `Viewport` | screen region to draw into; zero = whole screen |
| `PixelSnap` | round the drawn view to whole pixels; for pixel art, see [Pixel-perfect pixel art](#pixel-perfect-pixel-art) |

Methods (all take the logical screen size, `image.Point`, except `At`): `WorldToScreen`, `ScreenToWorld` (mouse picking), `WorldBounds` (the view at `Pos`), `DrawBounds` (culling: the union of the views at `PrevPos` and `Pos`, which covers every frame drawn during the tick), `ViewportRect`, `At(alpha)` (blended copy). With `PixelSnap`, `WorldToScreen`, `ScreenToWorld`, `WorldBounds` and `DrawBounds` use the rounded view, so picking and culling match the drawn pixels.

```go
mouse := cam.ScreenToWorld(gfx.Vec2{X: float32(mx), Y: float32(my)}, screen.Size)
```

### Pixel-perfect pixel art

Nearest-neighbour sprites at a fractional `Zoom` draw some texels one pixel wide and others two. If the camera also sits at a sub-pixel position, which texels get the extra pixel changes from frame to frame while it moves, and tiles seem to swim. It is worse when zoomed out, where one pixel more is a larger share of a texel. For pixel art:

- Set `PixelSnap`. The renderer rounds the view once per frame, after blending, so that viewport center − `Pos`·`Zoom` is a whole number of pixels (the view moves by at most half a pixel per axis). Every world point then keeps its sub-pixel phase on screen, and the world moves as a whole. It is ignored while `Rotation` is not 0.
- Prefer whole-number `Zoom` values. With `PixelSnap` a fractional zoom no longer shimmers, but its uneven texels stay visible, and the world scrolls by uneven pixel counts (6, 7, 6, 7…).
- Use `Layout.Integer` (see [Layout](#layout)) so the logical screen scales to the window by a whole number too.

## Interpolation

The simulation runs at a fixed TPS; `Draw` at the display rate. Items and the camera carry `PrevPos` and `Pos`, and the renderer draws at `PrevPos.Lerp(Pos, alpha)`.

- `SnapshotTransforms` must be the first stage.
- A camera system must copy `PrevPos = Pos` before moving.
- Spawn entities with `PrevPos = Pos`.
- With `Camera.PixelSnap`, the blended camera is rounded after blending, once per frame. Rounding `Pos` in the tick would not help: blending two rounded positions gives a sub-pixel position again.
- Extract systems cull against `Camera.DrawBounds`, the views at both `PrevPos` and `Pos`, so the trailing screen edge isn't missing anything while the camera moves.

## Layout

```go
func (g *Game) Layout(w, h int) (int, int) {
	lw, lh := render.Layout{Width: 320, Height: 180, Integer: true}.Size(w, h)
	g.screen.Size = image.Pt(lw, lh)
	return lw, lh
}
```

`Integer` scales by a whole number for pixel-perfect art: the logical size grows to fill the window at the largest integer scale that fits `Width` x `Height`.

## Renderer

```go
r := render.NewRenderer(render.StoresOf(srv))
r.Draw(screen, queue, cam, alpha)
r.SetGroups(groups) // optional: layer groups, see below
r.SetPost(post)     // optional: post-processing, see below
r.SetCanvases(c)    // optional: pre-rendered images, see below
r.Stats() // see the table below
```

Sprites (shaded or not) and shapes reuse draw options (0 allocs). Shapes are scaled copies of a 1x1 image. Text may allocate. Tests that read pixels need `-tags ebitentest` and a display.

### Batching

The renderer batches for you; there is nothing to enable. Unshaded sprites, tiles, particles, the nine pieces of a nine-slice panel and the edges of rects, lines and circles are quads, and consecutive quads with the same target, texture and blend mode are drawn with one `DrawTriangles` call instead of one `DrawImage` call each. Tint, position, rotation, scale and flips do not break a batch. To keep batches long:

- Put sprites that are drawn together into one atlas: all frames of one texture batch, all tiles of one tileset too.
- Keep additive or multiply sprites (particles, lights; see [particles.md](particles.md)) on their own layer, so they do not alternate with normal ones.
- Text and layer group boundaries end the current batch. Shaded sprites batch among themselves: consecutive ones with the same shader, frame, blend mode and `Params` are one draw call (particles of one effect, for example), and a different frame or `Params` starts a new one ([shaders guide](shaders.md#performance)). In a Y-sorted layer, sprites of different textures interleave by Y and batch less.

`Stats.Quads` counts the batched unshaded quads, `Stats.ShadedQuads` the shaded ones, and `Stats.DrawCalls` the calls they took: 10,000 tiles of one tileset are one draw call.

## Canvases

A `render.Canvases` holds offscreen images that a system paints once and then draws as one `KindCanvas` item per frame, instead of re-queuing what they show. `tilemap` uses it for [chunked layers](tilemaps.md#chunk-pre-rendering). `game.Engine` creates one (`Engine.Canvases()`, also a World resource) and gives it to its renderer; without the engine:

```go
canvases := render.NewCanvases(render.StoresOf(srv))
ecs.SetResource(w, canvases)
r.SetCanvases(canvases)

id := canvases.New(256, 256)  // transparent; reuses a freed image of that size
canvases.Paint(id, &paintQueue) // clears it, draws the queue now, unsorted; (0,0) is the top-left corner
q.Push(render.Item{Kind: render.KindCanvas, Canvas: id, Pos: p, PrevPos: p, Scale: gfx.Vec2{X: 1, Y: 1}})
canvases.Free(id)               // the ID and its copies go stale
```

- `Paint` draws the items like `Draw` with a camera centered on the canvas, so world and screen space both are canvas pixels; interpolation is off. Sprites that are not ready paint their placeholder, so paint once they are ready. It runs on the game goroutine, in `Update` or `Draw`.
- A canvas item uses `Pos`, `PrevPos`, `Rot`, `Scale`, `FlipX/Y`, `Color`, `Blend` and `Origin` (a fraction of the canvas size; zero is the top-left corner). It ignores `Shader`. A zero or stale ID, or a renderer without canvases, draws nothing.
- Each canvas is its own image, so each canvas item is its own batch (Ebiten may still merge canvases that share one of its internal atlas pages).
- `Len()` and `Bytes()` (4 bytes per pixel) report what is allocated. Up to 16 freed images are kept for reuse by `New`.
- Panics: `render: canvas size` (`New` with a non-positive side), `render: stale canvas` (`Paint` with a zero or freed ID).

## Stats and the overlay

`Renderer.Stats()` describes the last `Draw`:

| Field | Counts |
| --- | --- |
| `Items` | Queue items drawn. |
| `DrawCalls` | Ebiten draw calls issued; a batch of quads is one (Ebiten may merge consecutive calls into fewer GPU batches). |
| `Quads` | Unshaded quads drawn through batches: one per unshaded sprite, one per edge or segment of a rect, line or circle. `Quads` much larger than `DrawCalls` means batching works. |
| `ShadedQuads` | Sprites and canvases drawn through their shader, one quad each. |
| `ShaderDraws` | Draw calls that went through a shader: batches of shaded quads, shaded layer group composites and post passes. |
| `Groups` | Layer groups composited. |
| `PostPasses` | Post-processing passes that ran, the final pass of `DrawFinal` included ([post-processing](shaders.md#post-processing)). |
| `Placeholders` | Sprite items that drew the placeholder sprite: zero, stale, loading or failed handle. A count that stays above zero usually means a wrong asset key. Text with the default font does not count. |
| `Culled` | Items the extract systems skipped because they were outside the camera bounds (`Queue.Culled`). `SpriteExtract` and `DebugExtract` report it; `TilemapExtract` never visits cells outside the view, so off-screen tiles do not count. Hidden sprites are not culled. |

`render.StatsOverlay` draws those numbers, plus FPS and TPS, as a small text panel with Ebiten's debug font (no font asset needed):

```go
var overlay render.StatsOverlay // zero value: top-left corner; set Pos to move it
// at the end of Draw, after r.Draw:
overlay.Draw(screen, r.Stats())
```

`game.Engine` draws one when `Config.StatsOverlay` is set or after `SetStatsOverlay(true)`. The overlay is a debug aid: it allocates one string per frame. `Renderer.Draw` itself stays allocation-free. [Screenshots](engine.md#screenshots) never contain the overlay.

## Shaders

Sprites can be drawn through a Kage shader: set `Sprite.Shader` (or `Item.Shader`) and drive it with `Sprite.Params`. Unshaded sprites sort first in a layer, and shaded ones group by shader. The [shaders guide](shaders.md) covers writing shaders, examples and costs.

## Layer groups

A `render.GroupConfig` draws a range of layers into an offscreen image and composites that image onto the layers below, optionally through a shader and with a blend mode. That is how a shader applies to a whole layer (water, fog) and how light maps work. `game.Engine` creates one: `e.Groups()`. A group with `NoComposite` is never composited; post passes read it with `render.TargetGroup` (masks, displacement and light maps). Details and examples: [shaders guide](shaders.md#layer-groups).

## Post-processing

A `render.PostChain` runs full-screen shader passes over the finished frame, and an optional final pass at window resolution through `Renderer.DrawFinal` (for `ebiten.FinalScreenDrawer`). `game.Engine` creates one and wires both: `e.Post()`. `PostChain.Fade` covers the frame with a color (screen fades and flashes without a shader). Passes can also read several images and write offscreen targets (`PostChain.AddBloom` is built on that). Details and examples: [shaders guide](shaders.md#post-processing). A `game.ShotGame` [screenshot](engine.md#screenshots) contains the passes and the fade, a `game.ShotWindow` one the final pass too.
