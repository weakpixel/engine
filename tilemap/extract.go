package tilemap

import (
	"math"
	"time"

	"github.com/weakpixel/asset"
	"github.com/weakpixel/ecs"
	"github.com/weakpixel/ecs/system"
	"github.com/weakpixel/engine/extract"
	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/render"
	"github.com/weakpixel/engine/tiled"
)

// TilemapExtract queues the visible tiles of every [TileLayer] entity: one
// world-space sprite item per non-empty cell inside the camera bounds, in
// the map's render order, and one item per visible repetition of an image
// layer. Run it in the render stage after extract.ResetQueue and before
// extract.SpriteExtract, so that in a layer without Y-sorting tiles draw
// under the sprites of the same layer (insertion order breaks sort ties).
//
// Placement follows Tiled: a tile sits on the bottom-left corner of its cell,
// shifted by the tileset's tileoffset; flip flags become the item's rotation
// and mirroring about the tile's center. A diagonal flip of a non-square tile
// therefore rotates about its center, where Tiled keeps the bottom-left
// corner. Item.SortY is the bottom of the cell, so tiles also work in
// Y-sorted layers. Parallax uses Tiled's model,
// offset = (camera center - map position - ParallaxOrigin) * (1 - parallax),
// with Camera.PrevPos for Item.PrevPos so camera interpolation stays smooth.
//
// Animated tiles share one clock, the sum of the tick deltas since the
// system started, so every cell with the same tile shows the same frame.
// Hidden layers, maps that are not Ready and layers whose index is out of
// range push nothing.
//
// A layer with [TileLayer.Chunked] set queues one [render.KindCanvas] item
// per visible chunk of chunkCells x chunkCells cells instead, its static
// tiles pre-rendered into a render.Canvases canvas the first time the chunk
// is visible, plus one sprite item per visible animated tile (see
// chunks.go). Chunks not drawn for a while are freed.
//
// Run does not allocate, except while it pre-renders a chunk (the queue
// grows to its steady size during the first ticks).
//
// It needs the resources *render.Queue, *render.Camera, *extract.Screen and
// *asset.Server (with [RegisterLoaders] run on it). Chunked layers also need
// *render.Canvases, and read the optional *render.LayerConfig: without
// canvases, and in a Y-sorted render layer, they draw tile by tile.
type TilemapExtract struct {
	view    ecs.View2[TileLayer, gfx.Transform]
	maps    MapStore
	sprites render.SpriteStore
	queue   *render.Queue
	cam     *render.Camera
	screen  *extract.Screen
	layers  *render.LayerConfig // nil: no layer is Y-sorted
	clock   time.Duration
	chunks  chunkCache
}

// NewTilemapExtract returns the system for w.
func NewTilemapExtract(w *ecs.World) *TilemapExtract {
	return &TilemapExtract{view: ecs.NewView2[TileLayer, gfx.Transform](w)}
}

// Init implements system.Initializer. It caches the resources and the
// stores, so Run does no lookups.
func (s *TilemapExtract) Init(ctx *system.Context) {
	s.queue = ecs.GetResource[render.Queue](ctx.World)
	s.cam = ecs.GetResource[render.Camera](ctx.World)
	s.screen = ecs.GetResource[extract.Screen](ctx.World)
	s.layers = ecs.GetResourceOrNil[render.LayerConfig](ctx.World)
	srv := ecs.GetResource[asset.Server](ctx.World)
	s.maps = MapStoreOf(srv)
	s.sprites = render.SpriteStoreOf(srv)
	s.chunks.init(ecs.GetResourceOrNil[render.Canvases](ctx.World))
}

// Run implements system.Runner.
func (s *TilemapExtract) Run(ctx *system.Context) {
	s.clock += ctx.Time.Delta
	s.chunks.beginTick()
	// Everything this tick's frames can show: the view at PrevPos and at
	// Pos, plus the pixel snap (see render.Camera.DrawBounds).
	bounds := s.cam.DrawBounds(s.screen.Size)
	for _, c := range s.view.Each() {
		tl, tr := c.Unpack()
		if tl.Hidden {
			continue
		}
		m, state := s.maps.Lookup(tl.Map)
		if state != asset.Ready || tl.Index < 0 || int(tl.Index) >= len(m.Layers) {
			continue
		}
		l := &m.Layers[tl.Index]
		if tl.follow {
			s.syncShader(tl, l)
		}
		pl := s.place(m, l, tr)
		switch l.Kind {
		case Tiles:
			s.pushTiles(m, l, tl, pl, bounds)
		case Image:
			s.pushImage(l, tl, pl, bounds)
		}
	}
	s.chunks.endTick()
}

// syncShader lets a layer made by Spawn follow its map's hot reloads: when the
// map has a new version, Shader and Params become the new layer's, except a
// field that no longer equals the value Spawn (or the last sync) copied in,
// which game code has set and keeps. The version is read on the first tick a
// layer is seen, so a reload before that is not noticed. It does not allocate.
func (s *TilemapExtract) syncShader(tl *TileLayer, l *Layer) {
	version := s.maps.Generic().Version(asset.HandleOf[Map](tl.Map.Ref))
	if tl.mapVersion == version {
		return
	}
	first := tl.mapVersion == 0
	tl.mapVersion = version
	if first {
		return
	}
	if tl.Shader == tl.srcShader {
		tl.Shader = l.Shader
	}
	if tl.Params == tl.srcParams {
		tl.Params = l.Params
	}
	tl.srcShader, tl.srcParams = l.Shader, l.Params
}

// placement is where a layer's top-left corner is in the world this tick and
// the last one, parallax included.
type placement struct {
	pos, prev gfx.Vec2
}

// delta is the movement of the layer over the tick, added to every item's
// position to get its PrevPos.
func (p placement) delta() gfx.Vec2 { return p.prev.Sub(p.pos) }

func (s *TilemapExtract) place(m *Map, l *Layer, tr *gfx.Transform) placement {
	at := func(mapPos, cam gfx.Vec2) gfx.Vec2 {
		d := cam.Sub(mapPos).Sub(m.ParallaxOrigin)
		return mapPos.Add(l.Offset).Add(gfx.Vec2{X: d.X * (1 - l.Parallax.X), Y: d.Y * (1 - l.Parallax.Y)})
	}
	return placement{pos: at(tr.Pos, s.cam.Pos), prev: at(tr.PrevPos, s.cam.PrevPos)}
}

// cellRange returns the cells [a, b) of one axis that can draw into the
// visible range [lo, hi) (layer-local pixels): cell c covers
// [c*size, (c+1)*size), widened by the map's largest tile overhang. The
// result is clamped to the layer's grid [origin, origin+n).
func cellRange(lo, hi, size, overhang float32, origin, n int) (int, int) {
	a := int(math.Floor(float64((lo - overhang) / size)))
	b := int(math.Ceil(float64((hi + overhang) / size)))
	return max(a, origin), min(b, origin+n)
}

// visibleCells returns the cells [x0, x1) x [y0, y1) of tile layer l that
// can draw into bounds, empty when none can.
func visibleCells(m *Map, l *Layer, pl placement, bounds gfx.Rect) (x0, x1, y0, y1 int) {
	tw, th := float32(m.TileW), float32(m.TileH)
	x0, x1 = cellRange(bounds.Min.X-pl.pos.X, bounds.Max.X-pl.pos.X, tw, m.overhang, l.Origin.X, l.W)
	y0, y1 = cellRange(bounds.Min.Y-pl.pos.Y, bounds.Max.Y-pl.pos.Y, th, m.overhang, l.Origin.Y, l.H)
	return
}

// renderOrder returns how to walk [a, b) along one axis: from start to end
// (exclusive) in steps of step, backwards when reverse is set.
func renderOrder(a, b int, reverse bool) (start, end, step int) {
	if reverse {
		return b - 1, a - 1, -1
	}
	return a, b, 1
}

// Render order: rows top to bottom ("down") or bottom to top ("up"), cells
// left to right ("right") or right to left ("left").
func upward(o RenderOrder) bool   { return o == RightUp || o == LeftUp }
func leftward(o RenderOrder) bool { return o == LeftDown || o == LeftUp }

func (s *TilemapExtract) pushTiles(m *Map, l *Layer, tl *TileLayer, pl placement, bounds gfx.Rect) {
	x0, x1, y0, y1 := visibleCells(m, l, pl, bounds)
	if x0 >= x1 || y0 >= y1 {
		return
	}
	if s.chunked(tl) {
		s.pushChunks(m, l, tl, pl, x0, x1, y0, y1)
		return
	}
	s.pushCells(m, l, tl, pl, x0, x1, y0, y1)
}

// pushCells queues one item per non-empty cell of [x0, x1) x [y0, y1), in
// the map's render order.
func (s *TilemapExtract) pushCells(m *Map, l *Layer, tl *TileLayer, pl placement, x0, x1, y0, y1 int) {
	ys, ye, dy := renderOrder(y0, y1, upward(m.RenderOrder))
	xs, xe, dx := renderOrder(x0, x1, leftward(m.RenderOrder))
	delta := pl.delta()
	for cy := ys; cy != ye; cy += dy {
		row := (cy - l.Origin.Y) * l.W
		for cx := xs; cx != xe; cx += dx {
			if raw := l.GIDs[row+cx-l.Origin.X]; raw != 0 {
				s.pushCell(m, tl, pl.pos, delta, raw, cx, cy)
			}
		}
	}
}

// pushCell queues the tile raw (a GID with flip flags) of cell (cx, cy) of a
// layer whose top-left corner is at origin.
func (s *TilemapExtract) pushCell(m *Map, tl *TileLayer, origin, delta gfx.Vec2, raw uint32, cx, cy int) {
	gid, flip := tiled.SplitGID(raw)
	// Checked against the GID table when the map was loaded, so the index is
	// in range; the bounds check stays as a cheap guard.
	if gid >= uint32(len(m.tiles)) {
		return
	}
	t := &m.tiles[gid]
	pos, bottom := tilePos(m, t, origin, cx, cy)
	rot, fx, fy := orient(flip)
	s.queue.Push(render.Item{
		Kind: render.KindSprite, Layer: tl.Layer, SortY: bottom,
		Sprite: t.FrameAt(s.clock), Pos: pos, PrevPos: pos.Add(delta),
		Rot: rot, Scale: t.Scale, Origin: center, FlipX: fx, FlipY: fy, Color: tl.Tint,
		Shader: tl.Shader, Params: tl.Params,
	})
}

// tilePos returns where tile t of cell (cx, cy) is drawn, for a layer whose
// top-left corner is at origin: the center of the tile's image (items are
// drawn about it) and the bottom of the cell. The tile's bottom-left corner
// sits on the cell's, plus the tileset offset.
func tilePos(m *Map, t *Tile, origin gfx.Vec2, cx, cy int) (pos gfx.Vec2, bottom float32) {
	w, h := t.Size.X*t.Scale.X, t.Size.Y*t.Scale.Y
	bottom = origin.Y + float32(cy+1)*float32(m.TileH)
	return gfx.Vec2{
		X: origin.X + float32(cx)*float32(m.TileW) + t.Offset.X + w/2,
		Y: bottom + t.Offset.Y - h/2,
	}, bottom
}

// center is the item pivot of tiles and image layers: positions are computed
// for the image's center, which also makes flips and rotations stay in place.
var center = gfx.Vec2{X: 0.5, Y: 0.5}

func (s *TilemapExtract) pushImage(l *Layer, tl *TileLayer, pl placement, bounds gfx.Rect) {
	if l.Image.IsZero() || l.ImageSize.X <= 0 || l.ImageSize.Y <= 0 {
		return
	}
	w, h := l.ImageSize.X, l.ImageSize.Y
	// Without repeat, the image is one copy at the layer's corner; with it,
	// copies tile the visible bounds along that axis.
	i0, i1 := 0, 1
	if l.RepeatX {
		i0 = int(math.Floor(float64((bounds.Min.X - pl.pos.X) / w)))
		i1 = int(math.Floor(float64((bounds.Max.X-pl.pos.X)/w))) + 1
	}
	j0, j1 := 0, 1
	if l.RepeatY {
		j0 = int(math.Floor(float64((bounds.Min.Y - pl.pos.Y) / h)))
		j1 = int(math.Floor(float64((bounds.Max.Y-pl.pos.Y)/h))) + 1
	}
	delta := pl.delta()
	for j := j0; j < j1; j++ {
		for i := i0; i < i1; i++ {
			min := gfx.Vec2{X: pl.pos.X + float32(i)*w, Y: pl.pos.Y + float32(j)*h}
			if min.X >= bounds.Max.X || min.Y >= bounds.Max.Y || min.X+w <= bounds.Min.X || min.Y+h <= bounds.Min.Y {
				continue
			}
			pos := gfx.Vec2{X: min.X + w/2, Y: min.Y + h/2}
			s.queue.Push(render.Item{
				Kind: render.KindSprite, Layer: tl.Layer, SortY: min.Y + h,
				Sprite: l.Image, Pos: pos, PrevPos: pos.Add(delta),
				Scale: gfx.Vec2{X: 1, Y: 1}, Origin: center, Color: tl.Tint,
				Shader: tl.Shader, Params: tl.Params,
			})
		}
	}
}
