package collision

import (
	"image"
	"iter"

	"github.com/weakpixel/engine/gfx"
)

// TileGrid is a grid of solid cells, such as a *tilemap.CollisionGrid.
// Cell (0, 0) starts at world (0, 0) and every cell has the size of
// CellRect(0, 0). A solid cell counts as a full box, also when the tile's
// shape covers only part of it.
type TileGrid interface {
	// Solid reports whether cell (cx, cy) blocks.
	Solid(cx, cy int) bool
	// CellRect returns the world rectangle of cell (cx, cy).
	CellRect(cx, cy int) gfx.Rect
	// CellOf returns the cell that contains pos.
	CellOf(pos gfx.Vec2) (cx, cy int)
}

// TileOptions place a tile grid in a [Space].
type TileOptions struct {
	// Layer is the layer set of every cell; zero is DefaultLayer.
	Layer Layers
	// OneWay makes every cell block from one side only (for physics).
	OneWay OneWay
}

// tileGrid is a grid added to a Space.
type tileGrid struct {
	grid  TileGrid
	id    uint32 // > 0, in the order grids were added
	layer Layers
	opts  TileOptions
}

// AddTiles adds grid to the space: colliders whose Mask has the grid's layer
// get a contact per overlapped solid cell and one Enter and Exit per grid.
// Grids never detect anything. The grid is read each tick, so cells changed
// in place (a broken wall) count from the next run on.
//
// Panics with "collision: tile grid already added" if grid is in the space,
// and "collision: nil tile grid" if it is nil.
func (s *Space) AddTiles(grid TileGrid, opts TileOptions) {
	if grid == nil {
		panic("collision: nil tile grid")
	}
	if s.tileIndex(grid) >= 0 {
		panic("collision: tile grid already added")
	}
	layer := opts.Layer
	if layer == 0 {
		layer = DefaultLayer
	}
	s.nextGrid++
	s.tiles = append(s.tiles, tileGrid{grid: grid, id: s.nextGrid, layer: layer, opts: opts})
}

// RemoveTiles removes grid from the space; its pairs end with an Exit on the
// next run. It reports false if grid was not in the space.
func (s *Space) RemoveTiles(grid TileGrid) bool {
	i := s.tileIndex(grid)
	if i < 0 {
		return false
	}
	s.tiles = append(s.tiles[:i], s.tiles[i+1:]...)
	return true
}

// Tiles returns the options grid was added with, and false if it is not in
// the space.
func (s *Space) Tiles(grid TileGrid) (TileOptions, bool) {
	if i := s.tileIndex(grid); i >= 0 {
		return s.tiles[i].opts, true
	}
	return TileOptions{}, false
}

// TileGrids yields the tile grids of the space with the options they were
// added with, in the order they were added. Do not add or remove grids
// while ranging.
func (s *Space) TileGrids() iter.Seq2[TileGrid, TileOptions] {
	return func(yield func(TileGrid, TileOptions) bool) {
		for i := range s.tiles {
			if !yield(s.tiles[i].grid, s.tiles[i].opts) {
				return
			}
		}
	}
}

func (s *Space) tileIndex(grid TileGrid) int {
	for i := range s.tiles {
		if s.tiles[i].grid == grid {
			return i
		}
	}
	return -1
}

// cellsOf returns the cell range [x0, x1] x [y0, y1] of the cells that b
// touches in grid.
func cellsOf(grid TileGrid, b gfx.Rect) (x0, y0, x1, y1 int) {
	x0, y0 = grid.CellOf(b.Min)
	x1, y1 = grid.CellOf(b.Max)
	return
}

// tileContacts appends the contacts of the dynamic shape sh with the solid
// cells of t. Cells that sh only touches are skipped by the narrow phase.
func (s *Space) tileContacts(sh *Shape, t *tileGrid) {
	x0, y0, x1, y1 := cellsOf(t.grid, sh.Bounds)
	for cy := y0; cy <= y1; cy++ {
		for cx := x0; cx <= x1; cx++ {
			if !t.grid.Solid(cx, cy) {
				continue
			}
			// The cell lives in the Space: a local would escape to the heap,
			// because test may hand it to a custom NarrowFunc.
			s.cell = rectShape(t.grid.CellRect(cx, cy))
			if n, d, ok := s.narrow.test(sh, &s.cell); ok {
				s.contacts = append(s.contacts, Contact{
					Self: sh.Entity, Normal: n, Depth: d,
					Tiles: t.grid, Cell: image.Point{X: cx, Y: cy}, grid: t.id,
				})
			}
		}
	}
}
