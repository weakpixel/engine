package collision

import (
	"image"
	"iter"
	"math"

	"github.com/weakpixel/ecs"
	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/xmath"
)

// QueryPoint yields the colliders of the last run that contain p (as
// gfx.Rect.Contains for boxes, the open disc for circles) and are on a
// layer of mask. A custom shape counts by its bounds. Tile grids are not
// searched: use the grid's own lookups. Yields no entity twice; does not
// allocate when ranged over directly.
func (s *Space) QueryPoint(p gfx.Vec2, mask Layers) iter.Seq[ecs.Entity] {
	return func(yield func(ecs.Entity) bool) { s.query(gfx.Rect{Min: p, Max: p}, mask, yield) }
}

// QueryRect yields the colliders of the last run that overlap r (share an
// area with it; touching is not enough) and are on a layer of mask. Same
// rules as [Space.QueryPoint].
func (s *Space) QueryRect(r gfx.Rect, mask Layers) iter.Seq[ecs.Entity] {
	return func(yield func(ecs.Entity) bool) { s.query(r, mask, yield) }
}

// QueryShapes yields the placed colliders of the last run whose bounds
// overlap r and that are on a layer of mask, dynamic ones first: the shapes
// a mover tests itself against (see package physics). The shapes belong to
// the Space and are valid until the next run. Tile grids are not searched
// ([Space.TileGrids]). Does not allocate when ranged over directly.
func (s *Space) QueryShapes(r gfx.Rect, mask Layers) iter.Seq[*Shape] {
	return func(yield func(*Shape) bool) {
		buf := s.borrow()
		defer func() { s.giveBack(buf) }()
		for _, set := range [2]struct {
			bp     Broadphase
			shapes []Shape
		}{{s.dyn, s.dynamic}, {s.stat, s.statics}} {
			buf = set.bp.Query(r, buf[:0])
			for _, p := range buf {
				sh := &set.shapes[p]
				if sh.Layer.Any(mask) && sh.Bounds.Overlaps(r) && !yield(sh) {
					return
				}
			}
		}
	}
}

// borrow returns an empty query buffer; nested queries each get their own.
func (s *Space) borrow() []Proxy {
	if n := len(s.queryBuf); n > 0 {
		b := s.queryBuf[n-1]
		s.queryBuf = s.queryBuf[:n-1]
		return b[:0]
	}
	return nil
}

func (s *Space) giveBack(b []Proxy) { s.queryBuf = append(s.queryBuf, b) }

func (s *Space) query(q gfx.Rect, mask Layers, yield func(ecs.Entity) bool) {
	buf := s.borrow()
	defer func() { s.giveBack(buf) }()
	for _, set := range [2]struct {
		bp     Broadphase
		shapes []Shape
	}{{s.dyn, s.dynamic}, {s.stat, s.statics}} {
		buf = set.bp.Query(q, buf[:0])
		for _, p := range buf {
			sh := &set.shapes[p]
			if sh.Layer.Any(mask) && shapeHits(sh, q) && !yield(sh.Entity) {
				return
			}
		}
	}
}

// shapeHits is the exact test of a query rectangle against a shape whose
// bounds intersect it.
func shapeHits(sh *Shape, q gfx.Rect) bool {
	if sh.Kind != ShapeCircle {
		return true // boxes and custom shapes count by their bounds
	}
	c := sh.Center
	d := c.Sub(gfx.Vec2{X: min(max(c.X, q.Min.X), q.Max.X), Y: min(max(c.Y, q.Min.Y), q.Max.Y)})
	return d.X*d.X+d.Y*d.Y < sh.Radius*sh.Radius
}

// RayHit is where a ray first hits a collider or a solid tile.
type RayHit struct {
	// Entity is the collider hit; the zero Entity for a tile.
	Entity ecs.Entity
	// Tiles and Cell are the grid and cell hit, for a tile.
	Tiles TileGrid
	Cell  image.Point
	// Point is where the ray enters the shape, Normal the unit normal of the
	// face it enters through. A ray that starts inside a shape hits it at
	// its start with a zero Normal.
	Point, Normal gfx.Vec2
	// Fraction is the distance to Point as a fraction of the ray, in [0, 1].
	Fraction float32
}

// Raycast returns the nearest hit of the segment from -> to with the
// colliders of the last run on a layer of mask and the cells of the tile
// grids on such a layer. A ray that only grazes a shape (runs along an edge
// or touches a corner) does not hit it. A custom shape is hit by its
// bounds. It does not allocate once warm.
func (s *Space) Raycast(from, to gfx.Vec2, mask Layers) (RayHit, bool) {
	d := to.Sub(from)
	best := RayHit{Fraction: float32(math.Inf(1))}
	for i := range s.tiles {
		if g := &s.tiles[i]; g.layer.Any(mask) {
			if h, ok := rayTiles(g.grid, from, d); ok && h.Fraction < best.Fraction {
				best = h
			}
		}
	}
	// Query the broad phases piece by piece along the ray, one cell long
	// each, instead of with the bounds of the whole ray: a long diagonal ray
	// would otherwise test every collider in the square it spans. A hit
	// entered before the end of a piece has bounds that reach into that
	// piece or an earlier one, so the walk stops at the first piece that
	// ends behind the best hit.
	steps := int(min(math.Ceil(float64(d.Len()/s.cellSize)), 1<<16))
	steps = max(steps, 1)
	buf := s.borrow()
	for k := range steps {
		t0, t1 := float32(k)/float32(steps), float32(k+1)/float32(steps)
		if t0 > best.Fraction {
			break
		}
		a, b := from.Add(d.Mul(t0)), from.Add(d.Mul(t1))
		q := gfx.Rect{
			Min: gfx.Vec2{X: min(a.X, b.X), Y: min(a.Y, b.Y)},
			Max: gfx.Vec2{X: max(a.X, b.X), Y: max(a.Y, b.Y)},
		}
		buf = s.rayShapes(s.dyn, s.dynamic, q, from, d, mask, &best, buf)
		buf = s.rayShapes(s.stat, s.statics, q, from, d, mask, &best, buf)
	}
	s.giveBack(buf)
	if math.IsInf(float64(best.Fraction), 1) {
		return RayHit{}, false
	}
	best.Point = from.Add(d.Mul(best.Fraction))
	return best, true
}

// rayShapes tests the ray against the shapes of bp whose bounds intersect
// q and keeps the nearest hit in best. buf is scratch.
func (s *Space) rayShapes(bp Broadphase, shapes []Shape, q gfx.Rect, from, d gfx.Vec2, mask Layers, best *RayHit, buf []Proxy) []Proxy {
	buf = bp.Query(q, buf[:0])
	for _, p := range buf {
		sh := &shapes[p]
		if !sh.Layer.Any(mask) {
			continue
		}
		var h xmath.Hit
		var ok bool
		if sh.Kind == ShapeCircle {
			h, ok = xmath.RayCircle(from, d, xmath.Circle{C: sh.Center, R: sh.Radius})
		} else {
			h, ok = xmath.RayRect(from, d, sh.Bounds)
		}
		if ok && h.T < best.Fraction {
			*best = RayHit{Entity: sh.Entity, Normal: h.Normal, Fraction: h.T}
		}
	}
	return buf
}

// rayTiles walks the cells of grid along the ray (a grid DDA, Amanatides
// and Woo) and returns the first solid cell the ray enters.
func rayTiles(grid TileGrid, from, d gfx.Vec2) (RayHit, bool) {
	cell := grid.CellRect(0, 0)
	size := cell.Max.Sub(cell.Min)
	cx, cy := grid.CellOf(from)
	ex, ey := grid.CellOf(from.Add(d))
	stepX, tMaxX, tDeltaX := ddaAxis(from.X, d.X, cell.Min.X, size.X, cx)
	stepY, tMaxY, tDeltaY := ddaAxis(from.Y, d.Y, cell.Min.Y, size.Y, cy)
	// Every step moves one cell towards the end cell, so this bounds the
	// walk even when rounding keeps it from landing exactly on (ex, ey).
	for range xmath.Abs(ex-cx) + xmath.Abs(ey-cy) + 2 {
		if grid.Solid(cx, cy) {
			if h, ok := xmath.RayRect(from, d, grid.CellRect(cx, cy)); ok {
				return RayHit{Tiles: grid, Cell: image.Point{X: cx, Y: cy}, Normal: h.Normal, Fraction: h.T}, true
			}
		}
		if tMaxX < tMaxY {
			if tMaxX > 1 {
				break
			}
			cx += stepX
			tMaxX += tDeltaX
		} else {
			if tMaxY > 1 {
				break
			}
			cy += stepY
			tMaxY += tDeltaY
		}
	}
	return RayHit{}, false
}

// ddaAxis returns the step direction, the ray fraction at which the ray
// crosses into the next cell, and the fraction per cell, for one axis. The
// grid's cells start at origin and are size wide; c is the start cell.
func ddaAxis(p, d, origin, size float32, c int) (step int, tMax, tDelta float32) {
	inf := float32(math.Inf(1))
	switch {
	case d > 0:
		next := origin + float32(c+1)*size
		return 1, (next - p) / d, size / d
	case d < 0:
		next := origin + float32(c)*size
		return -1, (next - p) / d, -size / d
	}
	return 0, inf, inf
}

func absInt(n int) int {
	if n < 0 {
		return -n
	}
	return n
}
