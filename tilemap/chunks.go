package tilemap

import (
	"math"

	"github.com/weakpixel/asset"
	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/render"
	"github.com/weakpixel/engine/tiled"
)

// This file pre-renders chunked tile layers (TileLayer.Chunked). A layer is
// cut into square chunks of cells; the first time a chunk is visible, its
// static tiles are painted into a render.Canvases canvas, and from then on
// the chunk is one KindCanvas item instead of one item per tile. Animated
// tiles are left out of the canvas and queued one by one on top of it.
//
// Chunks are cached per map layer (map handle and layer index), so two
// entities that draw the same layer share them. A chunk that has not been
// drawn for chunkKeep ticks is freed, which bounds the GPU memory to roughly
// what the camera saw recently; a new map version (hot reload, which also
// follows tileset and texture reloads) drops the layer's chunks.

const (
	// chunkCells is the side of a chunk in cells. With 16 px tiles a chunk
	// is 256 px, so a 640x360 view at zoom 1 shows at most 4x3 of them per
	// layer.
	chunkCells = 16
	// maxChunkPixels bounds the side of a chunk in pixels (padding aside):
	// maps with big tiles get fewer cells per chunk.
	maxChunkPixels = 2048
	// bakeBudget is how many chunks Run pre-renders per tick at most.
	// Visible chunks beyond it draw tile by tile until a later tick bakes
	// them, so a level start or a camera jump spreads the painting over a
	// few ticks instead of one long frame.
	bakeBudget = 8
	// chunkKeep is how many ticks a chunk stays baked after it was last
	// drawn (2 s at 60 TPS), and how often unused layers are dropped.
	chunkKeep = 120
)

// chunkCache holds the chunks of every chunked layer drawn recently.
type chunkCache struct {
	canvases *render.Canvases // nil: chunking is off
	layers   map[chunkKey]*layerChunks
	// live lists the baked chunks, so the eviction sweep visits only them,
	// not every chunk of every cached layer.
	live []liveChunk
	// paint is the reused queue a chunk's tiles are painted from.
	paint  render.Queue
	tick   uint64
	budget int
}

type chunkKey struct {
	m     MapHandle
	layer int32
}

// layerChunks are the chunks of one map layer, row by row.
type layerChunks struct {
	version    uint32 // the map version the chunks were made from
	side       int    // cells per chunk side
	cols, rows int
	// pad is the margin around a chunk's cells, in pixels, that holds the
	// parts of its tiles reaching outside them (the map's overhang).
	pad    int
	chunks []chunk
	seen   uint64
}

type chunkState uint8

const (
	chunkUnbaked chunkState = iota
	chunkBaked
	chunkEmpty // no static tile: nothing to paint, only animated ones
)

type chunk struct {
	state  chunkState
	canvas render.CanvasID
	// anim holds the indices in Layer.GIDs of the chunk's animated cells, in
	// render order. Filled when the chunk is baked; the slice is reused.
	anim []int32
	seen uint64
}

type liveChunk struct {
	lc     *layerChunks
	i      int32
	canvas render.CanvasID // tells a live entry from a chunk baked again
}

func (c *chunkCache) init(canvases *render.Canvases) {
	c.canvases = canvases
	c.layers = map[chunkKey]*layerChunks{}
}

func (c *chunkCache) beginTick() {
	c.tick++
	c.budget = bakeBudget
}

// endTick frees the chunks not drawn for chunkKeep ticks and, every
// chunkKeep ticks, forgets the layers not drawn for that long.
func (c *chunkCache) endTick() {
	for i := 0; i < len(c.live); {
		e := c.live[i]
		if int(e.i) < len(e.lc.chunks) {
			ch := &e.lc.chunks[e.i]
			if ch.state == chunkBaked && ch.canvas == e.canvas {
				if c.tick-ch.seen <= chunkKeep {
					i++
					continue
				}
				c.canvases.Free(ch.canvas)
				ch.state, ch.canvas = chunkUnbaked, render.CanvasID{}
			}
		}
		// Swap-remove: the order of live does not matter.
		last := len(c.live) - 1
		c.live[i] = c.live[last]
		c.live = c.live[:last]
	}
	if c.tick%chunkKeep == 0 {
		for k, lc := range c.layers {
			if c.tick-lc.seen > chunkKeep {
				c.freeLayer(lc)
				delete(c.layers, k)
			}
		}
	}
}

// layer returns the chunks of layer index of map h (m, l), creating them or,
// when the map has a new version, dropping the old ones.
func (c *chunkCache) layer(maps MapStore, h MapHandle, index int32, m *Map, l *Layer) *layerChunks {
	key := chunkKey{h, index}
	version := maps.Generic().Version(asset.HandleOf[Map](h.Ref))
	lc := c.layers[key]
	switch {
	case lc == nil:
		lc = &layerChunks{}
		c.layers[key] = lc
		lc.reset(m, l, version)
	case lc.version != version:
		c.freeLayer(lc)
		lc.reset(m, l, version)
	}
	lc.seen = c.tick
	return lc
}

func (lc *layerChunks) reset(m *Map, l *Layer, version uint32) {
	lc.version = version
	lc.side = max(1, min(chunkCells, maxChunkPixels/max(m.TileW, m.TileH, 1)))
	lc.cols = (l.W + lc.side - 1) / lc.side
	lc.rows = (l.H + lc.side - 1) / lc.side
	lc.pad = int(math.Ceil(float64(m.overhang)))
	lc.chunks = make([]chunk, lc.cols*lc.rows)
}

// freeLayer frees the canvases of lc's baked chunks. Their live entries are
// dropped by the next sweep.
func (c *chunkCache) freeLayer(lc *layerChunks) {
	for i := range lc.chunks {
		if ch := &lc.chunks[i]; ch.state == chunkBaked {
			c.canvases.Free(ch.canvas)
			ch.state, ch.canvas = chunkUnbaked, render.CanvasID{}
		}
	}
}

// chunked reports whether tl draws by chunks this tick: it asks for it, there
// are canvases to paint into, and its render layer is not Y-sorted (a chunk
// has one SortY, so its tiles could not sort against the sprites).
func (s *TilemapExtract) chunked(tl *TileLayer) bool {
	return tl.Chunked && s.chunks.canvases != nil && !s.layers.Options(tl.Layer).YSort
}

// pushChunks queues the chunks of l that hold visible cells [x0, x1) x
// [y0, y1), in the map's render order: one canvas item per baked chunk plus
// its visible animated tiles. A chunk that cannot be baked this tick (its
// sprites are loading, or the bake budget is spent) draws tile by tile.
func (s *TilemapExtract) pushChunks(m *Map, l *Layer, tl *TileLayer, pl placement, x0, x1, y0, y1 int) {
	lc := s.chunks.layer(s.maps, tl.Map, tl.Index, m, l)
	side := lc.side
	gx0, gx1 := (x0-l.Origin.X)/side, (x1-1-l.Origin.X)/side+1
	gy0, gy1 := (y0-l.Origin.Y)/side, (y1-1-l.Origin.Y)/side+1
	ys, ye, dy := renderOrder(gy0, gy1, upward(m.RenderOrder))
	xs, xe, dx := renderOrder(gx0, gx1, leftward(m.RenderOrder))
	delta := pl.delta()
	for gy := ys; gy != ye; gy += dy {
		cy0 := l.Origin.Y + gy*side
		cy1 := min(cy0+side, l.Origin.Y+l.H)
		for gx := xs; gx != xe; gx += dx {
			cx0 := l.Origin.X + gx*side
			cx1 := min(cx0+side, l.Origin.X+l.W)
			i := gy*lc.cols + gx
			ch := &lc.chunks[i]
			ch.seen = s.chunks.tick
			if ch.state == chunkUnbaked && !s.bake(m, l, lc, int32(i), cx0, cx1, cy0, cy1) {
				s.pushCells(m, l, tl, pl, max(cx0, x0), min(cx1, x1), max(cy0, y0), min(cy1, y1))
				continue
			}
			if ch.state == chunkBaked {
				top := pl.pos.Add(gfx.Vec2{X: float32(cx0*m.TileW - lc.pad), Y: float32(cy0*m.TileH - lc.pad)})
				s.queue.Push(render.Item{
					Kind: render.KindCanvas, Layer: tl.Layer, SortY: pl.pos.Y + float32(cy1*m.TileH),
					Canvas: ch.canvas, Pos: top, PrevPos: top.Add(delta),
					Scale: gfx.Vec2{X: 1, Y: 1}, Color: tl.Tint,
					// The shader runs every frame on the chunk; bake never sets it.
					Shader: tl.Shader, Params: tl.Params,
				})
			}
			for _, idx := range ch.anim {
				cx, cy := l.Origin.X+int(idx)%l.W, l.Origin.Y+int(idx)/l.W
				if cx >= x0 && cx < x1 && cy >= y0 && cy < y1 {
					s.pushCell(m, tl, pl.pos, delta, l.GIDs[idx], cx, cy)
				}
			}
		}
	}
}

// bake pre-renders chunk i of lc, the cells [cx0, cx1) x [cy0, cy1) of l,
// and reports whether it is baked (or empty) now. It returns false, and
// leaves the chunk unbaked, while one of the static tiles' sprites is still
// loading or when the tick's bake budget is spent.
func (s *TilemapExtract) bake(m *Map, l *Layer, lc *layerChunks, i int32, cx0, cx1, cy0, cy1 int) bool {
	ch := &lc.chunks[i]
	ch.anim = ch.anim[:0]
	static, settled := 0, true
	eachCell(m, l, cx0, cx1, cy0, cy1, func(idx int, t *Tile, _ tiled.Flip, _, _ int) {
		switch {
		case t.AnimTotal > 0:
			ch.anim = append(ch.anim, int32(idx))
		default:
			static++
			settled = settled && s.spriteSettled(t.Sprite)
		}
	})
	if !settled {
		return false
	}
	if static == 0 {
		ch.state = chunkEmpty
		return true
	}
	if s.chunks.budget == 0 {
		return false
	}
	s.chunks.budget--

	q := &s.chunks.paint
	q.Reset()
	// Tiles are placed like pushCell places them, relative to the chunk's
	// top-left corner minus the padding.
	origin := gfx.Vec2{X: float32(lc.pad - cx0*m.TileW), Y: float32(lc.pad - cy0*m.TileH)}
	eachCell(m, l, cx0, cx1, cy0, cy1, func(_ int, t *Tile, flip tiled.Flip, cx, cy int) {
		if t.AnimTotal > 0 {
			return
		}
		pos, _ := tilePos(m, t, origin, cx, cy)
		rot, fx, fy := orient(flip)
		q.Push(render.Item{
			Kind: render.KindSprite, Sprite: t.Sprite, Pos: pos, PrevPos: pos,
			Rot: rot, Scale: t.Scale, Origin: center, FlipX: fx, FlipY: fy,
		})
	})
	w := (cx1-cx0)*m.TileW + 2*lc.pad
	h := (cy1-cy0)*m.TileH + 2*lc.pad
	ch.canvas = s.chunks.canvases.New(w, h)
	s.chunks.canvases.Paint(ch.canvas, q)
	q.Reset()
	ch.state = chunkBaked
	s.chunks.live = append(s.chunks.live, liveChunk{lc: lc, i: i, canvas: ch.canvas})
	return true
}

// eachCell calls f for every non-empty cell of [cx0, cx1) x [cy0, cy1) of
// l, in the map's render order, with the cell's index in l.GIDs, its tile and
// flip flags.
func eachCell(m *Map, l *Layer, cx0, cx1, cy0, cy1 int, f func(idx int, t *Tile, flip tiled.Flip, cx, cy int)) {
	ys, ye, dy := renderOrder(cy0, cy1, upward(m.RenderOrder))
	xs, xe, dx := renderOrder(cx0, cx1, leftward(m.RenderOrder))
	for cy := ys; cy != ye; cy += dy {
		row := (cy - l.Origin.Y) * l.W
		for cx := xs; cx != xe; cx += dx {
			idx := row + cx - l.Origin.X
			gid, flip := tiled.SplitGID(l.GIDs[idx])
			if gid != 0 && gid < uint32(len(m.tiles)) {
				f(idx, &m.tiles[gid], flip, cx, cy)
			}
		}
	}
}

// spriteSettled reports whether a sprite is done loading: ready, failed or
// missing (the last two draw the placeholder for good, so waiting for them
// would keep the chunk unbaked forever).
func (s *TilemapExtract) spriteSettled(h gfx.SpriteHandle) bool {
	switch _, st := s.sprites.Lookup(h); st {
	case asset.Queued, asset.Decoding, asset.Waiting:
		return false
	}
	return true
}
