package tilemap

import (
	"image"
	"iter"
	"math"

	"github.com/weakpixel/engine/collision"
	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/tiled"
)

// A CollisionGrid is a collision.TileGrid: add it to a collision.Space with
// Space.AddTiles.
var _ collision.TileGrid = (*CollisionGrid)(nil)

// Collision is what a cell of a [CollisionGrid] holds.
type Collision uint8

// Collision values.
const (
	// CollisionNone means no collision shape touches the cell.
	CollisionNone Collision = iota
	// CollisionPartial means some shape overlaps the cell without covering
	// it: test against the shapes yourself (Tile.Shapes of the tiles in and
	// around the cell).
	CollisionPartial
	// CollisionFull means one unrotated rectangle covers the whole cell.
	CollisionFull
)

var collisionNames = [...]string{"none", "partial", "full"}

// String returns "none", "partial" or "full".
func (c Collision) String() string {
	if int(c) < len(collisionNames) {
		return collisionNames[c]
	}
	return "invalid"
}

// Solid reports whether c is not CollisionNone.
func (c Collision) Solid() bool { return c != CollisionNone }

// CollisionGrid is a solid-cell lookup built from the collision shapes of a
// [Map]'s tiles ([Tile].Shapes): one [Collision] per map cell, for the
// broad phase of tile collision. Build it with [NewCollisionGrid] once the
// map is Ready.
//
// Cells are map cells and positions are map pixels, as for objects: cell
// (cx, cy) covers [cx*CellSize.X, (cx+1)*CellSize.X) and the same in Y.
// Unlike [Map.CellAt], no layer offset is subtracted: the offset of each
// source layer is applied when the grid is built.
//
// A grid is a copy: it stays valid after the map is unloaded or reloaded,
// and changing it with [CollisionGrid.Set] (a broken wall) does not touch
// the map. Lookups are O(1) and do not allocate.
type CollisionGrid struct {
	// Origin is the cell of the grid's first entry; non-zero only for
	// infinite maps. W and H are the grid size in cells: the union of the
	// source layers' grids. Cells outside it are CollisionNone.
	Origin image.Point
	W, H   int
	// CellSize is the map's cell size in pixels.
	CellSize gfx.Vec2

	cells []Collision
}

// collisionEps is how far, in pixels, a shape may miss or overshoot a cell
// edge and still count as ending on it. Tiled's editor leaves rounding noise
// in shapes (a y of -0.0037 for a shape drawn at 0), which must not spill
// into the neighbor cell or turn a full cell into a partial one.
const collisionEps = 1.0 / 64

// NewCollisionGrid builds the grid of map m from the given tile layers, or
// from every tile layer of m when none are given. Hidden layers count: a
// layer of collider tiles is usually hidden.
//
// Each shape of the tile in a cell is placed the way [TilemapExtract] draws
// the tile (bottom-left anchor, tileset offset, grid scale, flip flags about
// the tile's center, then the layer's Offset), and marks every cell its
// bounding box overlaps: CollisionFull where it is an unrotated rectangle
// covering the cell, CollisionPartial elsewhere. A tile reaching outside its
// cell therefore marks its neighbors; parts of shapes outside the grid are
// dropped. Points, polylines and text count by their bounding box, like
// rectangles, ellipses and polygons. Animated tiles use the shapes of the
// tile itself (Tiled's rule). Parallax is ignored.
//
// It panics if a layer is not a tile layer of m. The cost is proportional to
// the cells of the layers; the grid holds one byte per cell.
func NewCollisionGrid(m *Map, layers ...*Layer) *CollisionGrid {
	if len(layers) == 0 {
		for i := range m.Layers {
			if m.Layers[i].Kind == Tiles {
				layers = append(layers, &m.Layers[i])
			}
		}
	}
	g := &CollisionGrid{CellSize: gfx.Vec2{X: float32(m.TileW), Y: float32(m.TileH)}}
	var bounds image.Rectangle
	for _, l := range layers {
		if !m.ownsLayer(l) {
			panic("tilemap: NewCollisionGrid: layer is not a layer of the map")
		}
		if l.Kind != Tiles {
			panic("tilemap: NewCollisionGrid: layer " + l.Path + " is not a tile layer (" + l.Kind.String() + ")")
		}
		bounds = bounds.Union(image.Rect(l.Origin.X, l.Origin.Y, l.Origin.X+l.W, l.Origin.Y+l.H))
	}
	g.Origin, g.W, g.H = bounds.Min, bounds.Dx(), bounds.Dy()
	g.cells = make([]Collision, g.W*g.H)
	for _, l := range layers {
		g.addLayer(m, l)
	}
	return g
}

// ownsLayer reports whether l points into m.Layers.
func (m *Map) ownsLayer(l *Layer) bool {
	for i := range m.Layers {
		if &m.Layers[i] == l {
			return true
		}
	}
	return false
}

func (g *CollisionGrid) addLayer(m *Map, l *Layer) {
	tw, th := float64(m.TileW), float64(m.TileH)
	for y := range l.H {
		for x := range l.W {
			gid, flip := tiled.SplitGID(l.GIDs[y*l.W+x])
			t, ok := m.Tile(gid)
			if !ok || len(t.Shapes) == 0 {
				continue
			}
			// The tile's top-left corner in map pixels: its bottom-left
			// corner sits on the cell's, plus the tileset and layer offsets.
			cx, cy := float64(l.Origin.X+x), float64(l.Origin.Y+y)
			sx, sy := float64(t.Scale.X), float64(t.Scale.Y)
			w, h := float64(t.Size.X)*sx, float64(t.Size.Y)*sy
			left := cx*tw + float64(t.Offset.X+l.Offset.X)
			top := (cy+1)*th + float64(t.Offset.Y+l.Offset.Y) - h
			for i := range t.Shapes {
				r := shapeBounds(&t.Shapes[i])
				r = scaleRect(r, sx, sy)
				r = flipRect(r, flip, w, h)
				r = r.Add(vec2f{left, top})
				g.mark(r, isAxisRect(&t.Shapes[i]))
			}
		}
	}
}

// rectf is an axis-aligned rectangle in float64 pixels; float32 would lose
// the sub-pixel noise collisionEps is meant to absorb on large maps.
type rectf struct{ x0, y0, x1, y1 float64 }

type vec2f struct{ x, y float64 }

func (r rectf) Add(v vec2f) rectf { return rectf{r.x0 + v.x, r.y0 + v.y, r.x1 + v.x, r.y1 + v.y} }

// isAxisRect reports whether o is a rectangle whose bounding box is the
// shape itself, so it can mark cells full.
func isAxisRect(o *tiled.Object) bool {
	return o.Shape == tiled.ShapeRect && math.Mod(o.Rotation, 90) == 0
}

// shapeBounds returns the bounding box of o in tile pixels. Rotation is
// clockwise in degrees about X, Y, as in Tiled.
func shapeBounds(o *tiled.Object) rectf {
	var pts []vec2f
	switch o.Shape {
	case tiled.ShapePolygon, tiled.ShapePolyline:
		pts = make([]vec2f, len(o.Points))
		for i, p := range o.Points {
			pts[i] = vec2f{p.X, p.Y}
		}
	case tiled.ShapeTile:
		// Tile objects are anchored at their bottom-left corner.
		pts = []vec2f{{0, -o.Height}, {o.Width, -o.Height}, {0, 0}, {o.Width, 0}}
	default:
		pts = []vec2f{{0, 0}, {o.Width, 0}, {0, o.Height}, {o.Width, o.Height}}
	}
	sin, cos := math.Sincos(o.Rotation * math.Pi / 180)
	if o.Rotation == 0 {
		sin, cos = 0, 1 // exact, without rounding noise from Sincos
	}
	r := rectf{math.Inf(1), math.Inf(1), math.Inf(-1), math.Inf(-1)}
	for _, p := range pts {
		x, y := o.X+p.x*cos-p.y*sin, o.Y+p.x*sin+p.y*cos
		r = rectf{min(r.x0, x), min(r.y0, y), max(r.x1, x), max(r.y1, y)}
	}
	if len(pts) == 0 {
		r = rectf{o.X, o.Y, o.X, o.Y}
	}
	return r
}

func scaleRect(r rectf, sx, sy float64) rectf {
	return rectf{r.x0 * sx, r.y0 * sy, r.x1 * sx, r.y1 * sy}
}

// flipRect applies a cell's flip flags to r, a box in a w x h tile: Tiled's
// transpose (FlipD), then FlipH and FlipV, about the tile's center. That is
// the matrix orient turns into the renderer's rotation and mirroring, so the
// shapes follow the drawn tile, including the center rotation of a
// diagonally flipped non-square tile.
func flipRect(r rectf, f tiled.Flip, w, h float64) rectf {
	cx, cy := w/2, h/2
	// Relative to the center.
	x0, y0, x1, y1 := r.x0-cx, r.y0-cy, r.x1-cx, r.y1-cy
	if f&tiled.FlipD != 0 {
		x0, y0, x1, y1 = y0, x0, y1, x1
	}
	if f&tiled.FlipH != 0 {
		x0, x1 = -x1, -x0
	}
	if f&tiled.FlipV != 0 {
		y0, y1 = -y1, -y0
	}
	return rectf{x0 + cx, y0 + cy, x1 + cx, y1 + cy}
}

// mark records the shape with bounding box r (map pixels).
func (g *CollisionGrid) mark(r rectf, axisRect bool) {
	tw, th := float64(g.CellSize.X), float64(g.CellSize.Y)
	x0, x1 := overlapRange(r.x0, r.x1, tw)
	y0, y1 := overlapRange(r.y0, r.y1, th)
	for cy := y0; cy < y1; cy++ {
		for cx := x0; cx < x1; cx++ {
			if i, ok := g.index(cx, cy); ok && g.cells[i] == CollisionNone {
				g.cells[i] = CollisionPartial
			}
		}
	}
	if !axisRect {
		return
	}
	x0, x1 = coverRange(r.x0, r.x1, tw)
	y0, y1 = coverRange(r.y0, r.y1, th)
	for cy := y0; cy < y1; cy++ {
		for cx := x0; cx < x1; cx++ {
			if i, ok := g.index(cx, cy); ok {
				g.cells[i] = CollisionFull
			}
		}
	}
}

// overlapRange returns the cells [a, b) of one axis that [lo, hi] overlaps.
// Edges within collisionEps of a cell boundary end on it, so a shape that
// only touches a cell does not mark it; a zero-size shape marks the cell it
// is in.
func overlapRange(lo, hi, size float64) (int, int) {
	if hi-lo > 2*collisionEps {
		lo, hi = lo+collisionEps, hi-collisionEps
	}
	a := int(math.Floor(lo / size))
	b := int(math.Floor(hi/size)) + 1
	if hi > lo && hi == float64(b-1)*size {
		b-- // hi on a boundary: the cell after it is only touched
	}
	return a, max(b, a+1)
}

// coverRange returns the cells [a, b) of one axis that [lo, hi] covers,
// with collisionEps of slack at both ends.
func coverRange(lo, hi, size float64) (int, int) {
	a := int(math.Ceil((lo - collisionEps) / size))
	b := int(math.Floor((hi + collisionEps) / size))
	return a, b
}

// index returns the position of cell (cx, cy) in g.cells.
func (g *CollisionGrid) index(cx, cy int) (int, bool) {
	x, y := cx-g.Origin.X, cy-g.Origin.Y
	if x < 0 || y < 0 || x >= g.W || y >= g.H {
		return 0, false
	}
	return y*g.W + x, true
}

// At returns the collision of cell (cx, cy); CollisionNone outside the grid.
func (g *CollisionGrid) At(cx, cy int) Collision {
	if i, ok := g.index(cx, cy); ok {
		return g.cells[i]
	}
	return CollisionNone
}

// Solid reports whether any collision shape touches cell (cx, cy).
func (g *CollisionGrid) Solid(cx, cy int) bool { return g.At(cx, cy) != CollisionNone }

// CellOf returns the cell that contains pos, in map pixels.
func (g *CollisionGrid) CellOf(pos gfx.Vec2) (cx, cy int) {
	cx = int(math.Floor(float64(pos.X) / float64(g.CellSize.X)))
	cy = int(math.Floor(float64(pos.Y) / float64(g.CellSize.Y)))
	return cx, cy
}

// CollisionAt returns the collision of the cell that contains pos, in map
// pixels.
func (g *CollisionGrid) CollisionAt(pos gfx.Vec2) Collision {
	return g.At(g.CellOf(pos))
}

// SolidAt reports whether the cell that contains pos, in map pixels, is
// solid.
func (g *CollisionGrid) SolidAt(pos gfx.Vec2) bool { return g.CollisionAt(pos) != CollisionNone }

// CellRect returns the rectangle of cell (cx, cy) in map pixels.
func (g *CollisionGrid) CellRect(cx, cy int) gfx.Rect {
	min := gfx.Vec2{X: float32(cx) * g.CellSize.X, Y: float32(cy) * g.CellSize.Y}
	return gfx.Rect{Min: min, Max: min.Add(g.CellSize)}
}

// Set changes the collision of cell (cx, cy), for maps that change at run
// time. It reports false, and changes nothing, outside the grid.
func (g *CollisionGrid) Set(cx, cy int, c Collision) bool {
	i, ok := g.index(cx, cy)
	if ok {
		g.cells[i] = c
	}
	return ok
}

// SolidIn yields the solid cells that r (map pixels, half-open as
// gfx.Rect.Contains) overlaps, row by row, with their collision: the cells
// to test a moving box against. The only allocation is the iterator's
// closure.
func (g *CollisionGrid) SolidIn(r gfx.Rect) iter.Seq2[image.Point, Collision] {
	return func(yield func(image.Point, Collision) bool) {
		if r.Max.X <= r.Min.X || r.Max.Y <= r.Min.Y {
			return
		}
		x0, y0 := g.CellOf(r.Min)
		// Max is exclusive: a box ending exactly on a cell edge does not
		// reach into the next cell.
		x1 := int(math.Ceil(float64(r.Max.X) / float64(g.CellSize.X)))
		y1 := int(math.Ceil(float64(r.Max.Y) / float64(g.CellSize.Y)))
		x0, y0 = max(x0, g.Origin.X), max(y0, g.Origin.Y)
		x1, y1 = min(x1, g.Origin.X+g.W), min(y1, g.Origin.Y+g.H)
		for cy := y0; cy < y1; cy++ {
			row := (cy - g.Origin.Y) * g.W
			for cx := x0; cx < x1; cx++ {
				if c := g.cells[row+cx-g.Origin.X]; c != CollisionNone && !yield(image.Point{X: cx, Y: cy}, c) {
					return
				}
			}
		}
	}
}
