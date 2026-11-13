package collision

import (
	"cmp"
	"image"
	"math"

	"github.com/weakpixel/ecs"
	"github.com/weakpixel/engine/gfx"
)

// Contact is one overlap seen from the side that detects it: Self's Mask
// has a layer Other is on. When both detect each other there are two
// contacts, one per side, with opposite normals.
type Contact struct {
	// Self is the detecting entity, Other the detected one; Other is the
	// zero Entity when the contact is with a tile grid.
	Self, Other ecs.Entity
	// Normal is the unit vector that points from Self out of Other: the
	// direction to push Self.
	Normal gfx.Vec2
	// Depth is the penetration along Normal, >= 0.
	Depth float32
	// Tiles is the tile grid, for a contact with one; Cell is then the
	// solid cell.
	Tiles TileGrid
	Cell  image.Point
	// grid is the Space's id of Tiles (0 for none): it orders contacts and
	// keys tile pairs.
	grid uint32
}

// Enter is sent when Self starts detecting Other: once per detecting side
// and pair, and once per (collider, tile grid) however many cells touch.
// The Contact is the first one of the pair this tick.
type Enter struct{ Contact }

// Stay is sent every run in which Self still detects Other, after the run
// that sent their Enter: once per detecting side and pair, and once per
// (collider, tile grid). Only with Config.Stay set; otherwise read
// Space.Contacts. The Contact is the pair's first one this tick.
type Stay struct{ Contact }

// Exit is sent when Self stops detecting Other: they no longer overlap, the
// mask or filter no longer accepts the pair, the tile grid was removed, or
// either entity was deleted or lost its Collider (then the entity in the
// event is stale; check World.IsAlive).
type Exit struct {
	Self, Other ecs.Entity
	// Tiles is the tile grid of a tile pair, else nil.
	Tiles TileGrid
}

func cmpEntity(a, b ecs.Entity) int {
	if c := cmp.Compare(a.ID, b.ID); c != 0 {
		return c
	}
	return cmp.Compare(a.Generation, b.Generation)
}

// cmpPair orders contacts by pair: Self, Other, then grid.
func cmpPair(a, b *Contact) int {
	if c := cmpEntity(a.Self, b.Self); c != 0 {
		return c
	}
	if c := cmpEntity(a.Other, b.Other); c != 0 {
		return c
	}
	return cmp.Compare(a.grid, b.grid)
}

// cmpContact orders contacts by pair, then by cell.
func cmpContact(a, b Contact) int {
	if c := cmpPair(&a, &b); c != 0 {
		return c
	}
	if c := cmp.Compare(a.Cell.Y, b.Cell.Y); c != 0 {
		return c
	}
	return cmp.Compare(a.Cell.X, b.Cell.X)
}

// sortKey is cmpContact's order of a contact packed into integers, so that
// sorting moves 32 bytes and compares plain integers instead of moving and
// comparing whole Contacts. Cell coordinates are biased to be unsigned and
// are meant to fit an int32; i is the contact's index, which also makes the
// order total.
type sortKey struct {
	self, other uint64 // ID<<32 | Generation of the entity
	grid        uint64 // grid<<32 | cell Y
	cellX       uint32
	i           int32
}

func entityBits(e ecs.Entity) uint64 { return uint64(e.ID)<<32 | uint64(e.Generation) }

func newSortKey(c *Contact, i int) sortKey {
	return sortKey{
		self: entityBits(c.Self), other: entityBits(c.Other),
		grid:  uint64(c.grid)<<32 | uint64(uint32(int32(c.Cell.Y)+math.MinInt32)),
		cellX: uint32(int32(c.Cell.X) + math.MinInt32), i: int32(i),
	}
}

// cmpSortKey orders keys like cmpContact orders their contacts.
func cmpSortKey(a, b sortKey) int {
	switch {
	case a.self != b.self:
		return cmp.Compare(a.self, b.self)
	case a.other != b.other:
		return cmp.Compare(a.other, b.other)
	case a.grid != b.grid:
		return cmp.Compare(a.grid, b.grid)
	case a.cellX != b.cellX:
		return cmp.Compare(a.cellX, b.cellX)
	}
	return cmp.Compare(a.i, b.i)
}

// pairKey is one detecting pair of a tick, for the Enter/Exit diff.
type pairKey struct {
	self, other ecs.Entity
	grid        uint32
	tiles       TileGrid
}

func cmpKey(a, b *pairKey) int {
	if c := cmpEntity(a.self, b.self); c != 0 {
		return c
	}
	if c := cmpEntity(a.other, b.other); c != 0 {
		return c
	}
	return cmp.Compare(a.grid, b.grid)
}
