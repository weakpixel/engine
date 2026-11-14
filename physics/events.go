package physics

import (
	"github.com/weakpixel/ecs"
	"github.com/weakpixel/engine/collision"
	"github.com/weakpixel/engine/gfx"
)

// Impact is sent when a solid contact starts (not for sensors), once per
// detecting side and pair like collision.Enter, and once per (body, tile
// grid). Speed lets landing dust or hit sounds scale with the hit.
type Impact struct {
	// Self is the detecting entity, Other the one it hit; Other is zero for
	// a tile grid, then Tiles is set.
	Self, Other ecs.Entity
	Tiles       collision.TileGrid
	// Normal points from Self out of Other.
	Normal gfx.Vec2
	// Speed is the relative speed along the normal when the contact
	// started, >= 0.
	Speed float32
}
