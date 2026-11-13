package physics

import (
	"github.com/weakpixel/engine/collision"
	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/xmath"
)

// addTileContact adds the contact of body a with a solid tile cell.
//
// Internal edges: a normal that points into a neighboring solid cell would
// push the body out through a face that is inside the wall. A box sliding
// along a floor of tiles gets such a contact at every seam (its front edge
// overlaps the next cell less than its bottom does) and would snag, so that
// part of the normal is dropped.
func (s *System) addTileContact(c *collision.Contact, a int32) {
	b := &s.bodies[a]
	if b.col == nil || b.col.Sensor || b.inv == (gfx.Vec2{}) {
		return
	}
	n, ok := outerNormal(c.Tiles, c.Cell.X, c.Cell.Y, c.Normal)
	if !ok {
		return
	}
	tc := Contact{Contact: *c, a: a, b: -1, aDet: true, invA: b.inv}
	tc.Normal = n
	g := s.gridOf(c.Tiles)
	if g.opts.OneWay != collision.OneWayNone {
		_, held := s.warm[tc.key()]
		if !s.oneWayBlocks(a, -1, g.opts.OneWay, n, c.Depth, held) {
			return
		}
	}
	s.mix(&tc, &b.mat, &g.mat)
	s.contacts = append(s.contacts, tc)
}

// outerNormal drops the parts of n that point into solid neighbors of cell
// (cx, cy) and reports false when nothing is left.
func outerNormal(g collision.TileGrid, cx, cy int, n gfx.Vec2) (gfx.Vec2, bool) {
	const eps = 1e-6
	if sx := int(xmath.SignEps(n.X, eps)); sx != 0 && g.Solid(cx+sx, cy) {
		n.X = 0
	}
	if sy := int(xmath.SignEps(n.Y, eps)); sy != 0 && g.Solid(cx, cy+sy) {
		n.Y = 0
	}
	return n.NormOk()
}

// gridOf returns the info of a tile grid of this tick.
func (s *System) gridOf(g collision.TileGrid) *gridInfo {
	for i := range s.grids {
		if s.grids[i].grid == g {
			return &s.grids[i]
		}
	}
	// A grid added during the tick.
	opts, _ := s.space.Tiles(g)
	m, ok := s.tileMats[g]
	if !ok {
		m = s.defMat
	}
	s.grids = append(s.grids, gridInfo{grid: g, opts: opts, mat: m})
	return &s.grids[len(s.grids)-1]
}
