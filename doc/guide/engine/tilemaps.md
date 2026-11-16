# Tilemaps (Tiled)

Author levels in [Tiled](https://www.mapeditor.org/) and load its **JSON formats** as assets: maps (`*.tmj`), external tilesets (`*.tsj`) and worlds (`*.world`). Tile layers and image layers draw through the render queue (flips, animated tiles, offsets, opacity and tint, parallax, culling, 0 allocs per tick). Everything else in a map (object layers, typed custom properties, per-tile collision shapes, classes) is **read-only data** for your code: the engine gives it no meaning, apart from an optional solid-cell lookup built from the collision shapes.

Two packages, both in the engine module:

| Package | Job |
| --- | --- |
| `tiled` | The file model and decoding of the three formats, plus the checks one file allows. Standard library only, so tools can use it without Ebiten. |
| `tilemap` | The asset types `Tileset`, `Map`, `World` with loaders and handles, the `TileLayer` component, `Spawn`/`SpawnWorld`, `TilemapExtract`, and the map data API. |

## Quick start

With `game.Engine` the loaders and the extract are already wired:

```go
e.Start(func(e *game.Engine) {
	level, err := e.LoadMap("world-1.world", tilemap.SpawnOptions{}) // or "level-1.tmj"
	if err != nil {
		log.Fatal(err)
	}
	// level.Spawned() turns true in the tick the world is loaded; see below.
})
```

Without it: `tilemap.RegisterLoaders(srv)` after `render.RegisterLoaders`, load a map, spawn it once it is ready, and add `tilemap.NewTilemapExtract(w)` to the render stage **before** `extract.NewSpriteExtract(w)`:

```go
render.RegisterLoaders(srv, render.LoaderOptions{})
tilemap.RegisterLoaders(srv) // after render: tilesets use render.Atlas

h, err := tilemap.LoadMapSync(srv.Global(), "world-1/level-1.tmj")
layers, err := tilemap.Spawn(w, tilemap.MapStoreOf(srv), h, tilemap.SpawnOptions{BaseLayer: 0})

s.AddStage("render-prepare", extract.ResetQueue)
s.AddRunner(tilemap.NewTilemapExtract(w), extract.NewSpriteExtract(w))
```

`TilemapExtract` needs the resources `*render.Queue`, `*render.Camera`, `*extract.Screen` and `*asset.Server`. [Chunked layers](#chunk-pre-rendering) also need `*render.Canvases`, given to the renderer with `SetCanvases`; `game.Engine` sets both up.

## What is supported

| Feature | Support |
| --- | --- |
| Orientation | `orthogonal` only. Isometric, staggered and hexagonal are load errors. |
| Map size | finite, and infinite (chunked) maps |
| Layer data | JSON array (CSV), base64 uncompressed, base64 + zlib, base64 + gzip. zstd is a load error. |
| Tilesets | external `*.tsj` (single image or image collection); embedded image collections. An **embedded single-image tileset is a load error**: export it to a `.tsj` file (Tiled: Tileset > Export Tileset As). |
| Tile flips | horizontal, vertical, diagonal (rotations), see [Flips](#flips) |
| Tile animations | yes, one clock for the whole engine |
| Tileset options | `tileoffset`, `tilerendersize` `tile` and `grid` with `fillmode: stretch`. `preserve-aspect-fit` is a load error. |
| Image layers | offset, opacity, tint, parallax, `repeatx`, `repeaty` |
| Shader properties | `shader` and `shaderParams` on a tile or image layer, see [shaders](shaders.md#tile-layers) |
| Group layers | offset, opacity, tint, parallax and visibility compose into the children |
| Layer options | offset, opacity, `tintcolor`, parallax, `visible`; map `renderorder` (all four) and `parallaxoriginx/y` |
| Objects | every shape (rectangle, point, ellipse, polygon, polyline, text, tile object) as data. Object templates (`template`) are a load error: detach them in Tiled. |
| Custom properties | every type (string, int, float, bool, color, file, object, class) as data. Defaults of custom types from the `.tiled-project` are **not** applied (the project file is not read). |
| Per-tile collision | the tile's object group, as data (`Tile.Shapes`), plus a solid-cell lookup built from it ([collision grid](#collision-grid)) |
| Worlds | `*.world` with a `maps` list. Worlds with `patterns` are a load error. |
| Other | `transparentcolor` (color keys on tileset or layer images) is a load error: use an image with alpha. A format `version` whose major part is not 1 is a load error. Wang sets, terrains and editor settings are ignored. |

Unsupported features are always errors, never silently dropped. Each error names the feature, and all problems of a file are reported at once (`errors.Join`), for example:

```
asset: load tilemap.Map forest/a.tmj: unsupported Tiled feature: orientation: "isometric" (only "orthogonal" is supported)
layer "entities/props": object 7: unsupported Tiled feature: object templates: "door.tx"; detach the template in Tiled
```

Unknown fields are ignored: Tiled adds fields between versions.

## Asset kinds and keys

| Kind | Key | Load with | Handle | Depends on |
| --- | --- | --- | --- | --- |
| `tilemap.Tileset` | `"tiles/temple.tsj"` | `tilemap.LoadTileset` | `tilemap.TilesetHandle` | `render.Atlas` of the same key (single image) or one `render.Sprite` per tile image (collection) |
| `tilemap.Map` | `"level-1.tmj"` | `tilemap.LoadMap` | `tilemap.MapHandle` | its external tilesets, image-layer images, embedded collection images |
| `tilemap.World` | `"world-1.world"` | `tilemap.LoadWorld` | `tilemap.WorldHandle` | its maps |

Each has `Load…`, `Load…Sync`, `Hold…` and a store facade (`tilemap.MapStoreOf(srv).Get(h)`), like the other kinds ([assets.md](assets.md#game-facing-handles)). Paths inside the files are relative to the file, as Tiled writes them. A tileset shared by several maps loads once. Image-layer images load even when the layer is invisible, because the game may show it later.

A single-image tileset is also an **atlas**: `render.LoadAtlas(sc, "tiles/temple.tsj")` cuts it by `tilewidth`/`tileheight`, `margin`, `spacing` and `columns`, and `render.LoadSprite(sc, "tiles/temple.tsj#92")` is the sprite of local tile 92. The `Tileset` asset takes its sprites from there. Loading a collection tileset as an atlas fails ("collection tileset has no single image; load it as tilemap.Tileset").

## The Map value

A `Map` is self-contained and read-only. Pointers into it follow the `Store.Get` rule (valid until the next `Server.Update`).

| Field | |
| --- | --- |
| `Width`, `Height`, `TileW`, `TileH` | size in cells, cell size in pixels |
| `Infinite`, `RenderOrder`, `ParallaxOrigin`, `Background`, `Class`, `Props` | map attributes (`Background` is not drawn by the engine) |
| `Layers []Layer` | **every** layer, flattened in draw order: depth-first, bottom to top, a group before its children |
| `Tilesets []MapTileset` | `FirstGID`, `Tileset` (zero for embedded ones), `Name` |

A `Layer` has `Kind` (`Tiles`, `Objects`, `Image`, `Group`), `Name`, `Path` (`"entity-group/entities"`), `Parent` (index of its group or -1), `Class`, `Props` (its own; Tiled does not inherit properties) and the **effective** `Offset`, `Opacity`, `Tint`, `Parallax` and `Visible`, composed with every group it is in. Per kind:

- tile layers: `Origin`, `W`, `H`, `GIDs` (raw, flag bits included). An infinite map's chunks are merged into one dense grid per layer covering all of them; `Origin` is the cell of `GIDs[0]` and can be negative. The memory cost is `W*H*4` bytes per layer, also for sparse maps.
- object layers: `Objects []tiled.Object` and `DrawOrder`.
- image layers: `Image` (a `gfx.SpriteHandle`), `ImageSize`, `RepeatX`, `RepeatY`.

### Query API

```go
m := tilemap.MapStoreOf(srv).Get(h)

l, ok := m.LayerByName("main")
l, ok = m.LayerByPath("entity-group/entities")                    // the first in draw order
l, ok = m.LayerByPathKind("entity-group/entities", tilemap.Objects) // Tiled allows equal paths

tile, ok := m.Tile(gid)            // O(1): the map keeps a GID table
gid, flip, ok := m.Cell(l, cx, cy) // map cells
gid, flip, ok = m.CellAt(l, pos)   // map pixels (layer offset applied)

for layer, obj := range m.Objects() { ... }
for layer, obj := range m.ObjectsOfClass("baby-chick") { ... }
layer, obj, ok := m.FindObject("player")
world := tilemap.ObjectPos(layer, obj, mapPos) // mapPos + layer offset + obj.X, obj.Y
r := m.Bounds()                                // map pixels: (0,0)-(Width*TileW, Height*TileH)
```

`Bounds` of an infinite map is the union of its tile layers' cells (which can start at negative coordinates), or `Width x Height` cells if it has none. Layer offsets are not included. Add the map's world position to place it; `game.Level.Bounds()` does that for every map of a level, for [camera bounds](camera.md#bounds).

A `Tile` has `Sprite`, `Size`, `Offset` (the tileset's `tileoffset`), `Scale` (`tilerendersize: grid`), `Class`, `Props`, `Shapes` (collision shapes in tile pixels) and `Anim`/`AnimTotal`. `tile.FrameAt(clock)` is the sprite shown at a time. Tiles share their slices with the tileset: don't modify them.

### Collision grid

`NewCollisionGrid` turns the tiles' collision shapes into a solid-cell lookup for the broad phase of tile collision. Build it once the map is Ready:

```go
ct, _ := m.LayerByName("collider-tiles")
grid := tilemap.NewCollisionGrid(m, ct) // no layers: every tile layer, hidden ones included

grid.At(cx, cy)            // tilemap.CollisionNone, CollisionPartial or CollisionFull
grid.Solid(cx, cy)         // At != CollisionNone
grid.SolidAt(pos)          // map pixels; CollisionAt(pos) and CellOf(pos) too
grid.CellRect(cx, cy)      // the cell in map pixels
for cell, c := range grid.SolidIn(box) { ... } // solid cells a box (map pixels) overlaps
grid.Set(cx, cy, tilemap.CollisionNone)         // a wall broke; the map is unchanged
```

- Cells are map cells and positions map pixels, the space of objects. Each layer's `Offset` is applied when the grid is built, so `grid.SolidAt(pos)` takes no layer, unlike `CellAt`. Parallax is ignored.
- Each shape is placed the way the tile is drawn (bottom-left anchor, `tileoffset`, grid scale, flips) and marks every cell its bounding box overlaps. **`CollisionFull`** is a cell covered by one rectangle (rotation a multiple of 90°); **`CollisionPartial`** is any other overlap, where the narrow phase against `Tile.Shapes` is yours. A tall tile's shapes mark the cells above it; shapes outside the layers' grids are dropped.
- Edges within 1/64 px of a cell boundary count as on it, so Tiled's rounding noise (a `y` of `-0.0037`) does not spill into the next cell.
- A `*CollisionGrid` is a `collision.TileGrid`: `space.AddTiles(grid, collision.TileOptions{Layer: wall})` gives colliders contacts and events with its solid cells ([collision.md](collision.md#tile-grids)).
- The grid is a copy (one byte per cell): it outlives the map's pointers, and `Set` changes only the grid. `NewCollisionGrid` is O(cells) and panics with `tilemap: NewCollisionGrid: ` for a layer that is not a tile layer of the map. Lookups are O(1) and do not allocate; `SolidIn` allocates only its iterator closure.

A `tiled.Object` has `ID`, `Name`, `Class`, `Shape` (`ShapeRect`, `ShapePoint`, `ShapeEllipse`, `ShapePolygon`, `ShapePolyline`, `ShapeText`, `ShapeTile`), `X`, `Y`, `Width`, `Height`, `Rotation` (degrees), `Visible`, `Points`, `GID`/`Flip` (tile objects), `Text` and `Props`.

**Class vs type.** Tiled 1.9 renamed an object's and a tile's `type` to `class`, but files saved for compatibility still write `type`. `Class` is `class` when set, else `type`.

### Properties

`tiled.Properties` is an ordered list with typed getters that return `(value, ok)`; `ok` is false when the property is missing or has another type:

```go
mode, ok := layer.Props.String("layer-render-mode")
mass, ok := obj.Props.Float("mass")    // an "int" property widens to float64
static, ok := tile.Props.Bool("static")
c, ok := props.Color("glow")           // tiled.Color; gfx.Color(c) converts
ref, ok := props.Object("target")      // an object ID
stats, ok := props.Class("stats")      // the members the file sets, by name
```

Getters: `String`, `File`, `Int`, `Float`, `Bool`, `Color`, `Object`, `Class`, and `Get` for the raw `Property{Name, Type, PropertyType, Value}`.

## Placement and coordinates

- A map spawned at `Pos` has its top-left corner there. Map pixels plus `Pos` are world units.
- A tile sits on the **bottom-left corner** of its cell (Tiled's anchor), shifted by the tileset's `tileoffset`. A tile taller than the cell reaches up into the cells above, as in Tiled.
- A layer's `Offset` (composed with its groups) shifts it.
- **Parallax** follows Tiled: the layer is shifted by `(camera center − map position − ParallaxOrigin) × (1 − parallax)`. `Camera.Pos` is the camera center; the previous tick's shift uses `Camera.PrevPos`, so interpolation stays smooth.
- Moving a layer entity's `Transform.Pos` moves the layer. `Transform.Rot` and `Scale` are ignored.
- Objects are not shifted by parallax: they are gameplay data. `ObjectPos` gives an object's world position.

### Flips

The flip bits of a GID (`tiled.FlipH`, `FlipV`, `FlipD`; `tiled.SplitGID` separates them) become the item's rotation and mirroring about the tile's center, so a flipped tile stays in its cell. One deviation from Tiled: **a diagonal flip of a non-square tile rotates about its center**, while Tiled keeps the bottom-left corner. Square tiles look identical. The hexagonal 120° bit is ignored.

### Animated tiles

All cells with the same tile show the same frame, as in Tiled: `TilemapExtract` keeps one clock (the sum of the tick deltas since it started) and picks the frame from it.

## Spawning and draw order

`tilemap.Spawn(w, maps, h, opts)` creates one entity per **tile and image layer**, in draw order, with a `gfx.Transform` and a `tilemap.TileLayer{Map, Index, Layer, Hidden, Chunked, Tint, Shader, Params}`. Object and group layers create no entity. The map must be Ready, otherwise `Spawn` returns an error and creates nothing. `SpawnWorld(w, worlds, maps, h, opts)` spawns every map of a world at `opts.Pos` plus the map's position. Deleting the entities is your job (`game.Engine.UnloadLevel` does it for levels from `LoadMap`).

- A layer invisible in Tiled spawns with `Hidden: true`. Toggling `TileLayer.Hidden` at run time is a field write, as for `gfx.Sprite`.
- `TileLayer.Tint` is the layer's effective tint with its opacity folded into the alpha.
- `TileLayer.Shader` and `Params` come from the layer properties `shader` (a `.kage` key relative to the map) and `shaderParams` (up to four comma-separated numbers). A missing shader file or a bad `shaderParams` fails the map load. Shaded layers should be `Chunked`: one draw per chunk instead of one per tile, see [shaders](shaders.md#tile-layers). When the map is hot-reloaded, a layer made by `Spawn` takes the new layer's `Shader` and `Params`, except a field game code has written since (it is compared with the value `Spawn` copied in); [hot reload](shaders.md#hot-reload).
- The default render layer is `opts.BaseLayer + drawIndex`, where `drawIndex` counts drawable layers bottom to top from 0. Each map of a world starts again at `BaseLayer`, so equal layers of neighboring maps share a render layer.
- `opts.LayerFor(m, layer, drawIndex) (renderLayer, spawn)` overrides both: skip a layer or put it anywhere. `m.Layers[layer.Parent]` is its group.
- `opts.Chunked` spawns every tile layer with `TileLayer.Chunked`: see [Chunk pre-rendering](#chunk-pre-rendering).

`TilemapExtract` pushes one `render.Item` per non-empty, visible cell, in the map's `renderorder`, with `SortY` at the bottom of the cell. It runs **before** `SpriteExtract`, so in a layer without Y-sorting tiles draw under the sprites of the same layer. In a `YSort` layer, tiles and sprites sort by their bottom edge: a hero whose pivot is at the feet walks behind a wall tile whose cell is lower on screen. Tiles of one tileset share a texture, so they batch. Maps that are loading or failed draw nothing; a tile whose sprite is not ready draws the sprite placeholder.

Culling uses the camera bounds, widened by the largest distance any tile of the map reaches outside its cell (tall tiles, offsets), so oversized tiles at the edge do not pop.

## Chunk pre-rendering

A tile layer queues one item per visible tile. Batching makes that one draw call per tileset, but every item still costs its extract, sort and draw work on the CPU, about 0.2 µs each. For large static layers, set `TileLayer.Chunked` (or spawn with `SpawnOptions{Chunked: true}`): the layer is cut into chunks of 16x16 cells, the static tiles of a chunk are painted into one image the first time the chunk is visible, and from then on the chunk is **one** `render.KindCanvas` item. On the benchmark's 256x256 map that is 32 items instead of 3,840 and a frame about 9x cheaper on the CPU (see [Costs](#costs)).

```go
level, err := e.LoadMap("level-1.tmj", tilemap.SpawnOptions{Chunked: true})
// or per layer, after spawning: ecs.GetComp[tilemap.TileLayer](w, layer).Chunked = true
```

- **Same picture.** A chunk is painted with the renderer itself, tile by tile in render order, with flips, tile offsets and tiles reaching outside their cell (the chunk image has a margin for them). `TestChunkedMatchesTiles` (`-tags ebitentest`) checks that chunked and per-tile frames are pixel-identical, at sub-pixel camera positions too. The layer's `Tint`, `Hidden`, parallax and the entity's `Transform` apply to the chunk items like to tiles.
- **Animated tiles** stay out of the chunk image and are queued one by one, after their chunk. Canvas items sort before the sprites of their render layer, so they stay under the animated tiles.
- **When it draws tile by tile instead:** in a render layer configured with `YSort` (a chunk has one `SortY`, its bottom, so its tiles could not sort against sprites), without a `*render.Canvases` resource, for a chunk whose tile sprites are still loading, and for chunks over the bake budget: at most 8 chunks are painted per tick, so a level start or a camera jump spreads the work over a few ticks instead of one long frame. Image layers ignore `Chunked`. **Shaders:** a layer shader is not baked into the chunk image; it runs on the chunk item (and the animated tiles) every frame.
- **Memory.** A chunk is 256x256 px with 16 px tiles (4 bytes per pixel, 256 KiB), fewer cells per chunk for tiles above 128 px. Chunks are made when first visible and freed after 120 ticks (2 s at 60 TPS) without being drawn, so memory follows what the camera saw recently, not the map size; `Canvases.Len()` and `Bytes()` report it. Freed images are kept for reuse (up to 16), so scrolling does not allocate an image per new chunk.
- **Static cells.** The engine has no API to change cells; a map reloaded by hot reload (directly, or because its tileset or texture changed) gets a new `Version`, and its chunks are painted again. Two entities that draw the same layer of the same map share its chunks.
- **Draw order between chunks.** Chunks draw in the map's render order, each one's tiles in render order. Only tiles that reach outside their cell into a neighbor chunk can overlap in a different order than per tile; keep oversized tiles on a layer of their own if that matters.

## Properties are yours

The engine interprets no custom property. Give your own conventions meaning in `LayerFor` (it picks a render layer per Tiled layer, so a property can route a tile layer to a Y-sorted layer) and in your systems (a property can hide a layer unless a debug overlay is on).

There are no spawn hooks and no physics; the [collision grid](#collision-grid) is a lookup, not a solver.

### Where to place objects

Place **Game Objects** (anything that becomes an entity: characters, pickups, props) in a GOD level (`*.level.json`) laid over the map, not in a Tiled object layer. The level editor shows the map under the level, validates every component against its schema and handles prefabs and overrides; Tiled objects are untyped and the engine never turns them into entities. The demo does this: its villagers are in `world-1/level-1.level.json`, on top of the Tiled world. `LoadMap` and `LoadLevelBase` spawn `<map>.level.json` with its map by default (see [engine.md](engine.md#tiled-maps)); `SpawnOptions.NoDocuments` loads the map alone.

Room exits and spawn points are Game Objects too (`room:ExitArea`, `room:SpawnPoint`, see [rooms.md](rooms.md)); a spawn name also matches a named Tiled object.

Use Tiled objects for **map data** that is not an entity: regions, trigger areas, paths and named points. Read them in a system once the level has spawned (`Level.Spawned()`), with the [query API](#query-api); the hero starts at the `player` object this way.

Preview a map, a tileset or a world with `wpix-editor view <file>`: layers toggle, objects are outlined and named, tile ids, collision shapes and animations show on a tileset, and a world shows its maps with the levels that use them.

## Costs

| | |
| --- | --- |
| Load | decode on a worker (JSON, base64, zlib/gzip); `Finish` flattens layers, builds the GID table and checks every cell |
| Per tick | `TilemapExtract.Run`: proportional to the visible cells, or to the visible chunks of [chunked layers](#chunk-pre-rendering), 0 allocs (painting a new chunk allocates) |
| Memory | one dense `uint32` grid per tile layer, one `Tile` per GID; 4 bytes per pixel of every chunk image painted in the last 120 ticks |

`BenchmarkTilemapExtract` measures `Run` alone; `BenchmarkTilemapFrame` (`-tags ebitentest`, needs a display) measures the CPU side of a frame: `Run`, `Queue.Sort` and `Renderer.Draw` onto 640x360. Scenes: `level` is the demo's level 2 at zoom 3, `large` a 256x256 map with 4 full layers of 16 px tiles at zoom 1. Measured on an Intel Xeon @ 2.80 GHz (4 cores), steady state:

| Scene | Items | `Run` | Frame |
| --- | --- | --- | --- |
| `level` | 242 | ≈ 5.5 µs | ≈ 86 µs |
| `level-chunked` | 8 | ≈ 0.8 µs | ≈ 38 µs |
| `large` | 3,840 | ≈ 74 µs | ≈ 955 µs |
| `large-chunked` | 32 | ≈ 1.0 µs | ≈ 107 µs |

Medians of 8 runs; the frame numbers vary by ±15-40% because the benchmark reads a pixel back between frames. Chunked frames issue one draw call per chunk (8 and 32) instead of one per tileset (2 and 1), which costs less than the items they replace.
