package collision

import (
	"strconv"

	"github.com/weakpixel/engine/gfx"
)

// ShapeKind selects the shape of a [Collider]. Values from [ShapeCustom] on
// are the game's own: register them with [Space.RegisterShape] and
// [Space.RegisterPair]. The set is open, so it is a plain number in GOD.
type ShapeKind uint8

// Shape kinds.
const (
	// ShapeRect is a rectangle of Size whose top-left corner is
	// Transform.Pos + Offset.
	ShapeRect ShapeKind = iota
	// ShapeCircle is a circle of radius Size.X around Transform.Pos +
	// Offset.
	ShapeCircle
	// ShapeAnimBody is a rectangle like ShapeRect whose Offset and Size the
	// anim.AnimationSystem writes from the body of the entity's animation
	// definition (the union of its body rects). The collider is skipped
	// while Size is zero (no clip loaded yet).
	ShapeAnimBody

	// ShapeCustom is the first kind a game may define.
	ShapeCustom ShapeKind = 128
)

var shapeNames = [...]string{"rect", "circle", "anim-body"}

// String returns "rect", "circle", "anim-body" or "custom(<n>)".
func (k ShapeKind) String() string {
	if int(k) < len(shapeNames) {
		return shapeNames[k]
	}
	if k >= ShapeCustom {
		return "custom(" + strconv.Itoa(int(k)) + ")"
	}
	return "invalid(" + strconv.Itoa(int(k)) + ")"
}

// OneWay makes a collider or a tile grid block only from one side. Package
// collision stores it and reports it; package physics applies it.
//
// +game:enum
type OneWay uint8

// One-way directions. The direction names the blocking face.
const (
	// OneWayNone blocks from every side.
	OneWayNone OneWay = iota
	// OneWayUp blocks only bodies coming from above, moving down onto it: a
	// platform you can jump through from below.
	OneWayUp
	// OneWayDown blocks only bodies coming from below.
	OneWayDown
	// OneWayLeft blocks only bodies coming from the left.
	OneWayLeft
	// OneWayRight blocks only bodies coming from the right.
	OneWayRight
)

// Collider gives an entity with a gfx.Transform a collision shape. The
// shape is axis-aligned: Transform.Scale scales Offset and Size (a negative
// scale mirrors), Transform.Rot is ignored.
//
// +game:component
type Collider struct {
	// Shape is ShapeRect, ShapeCircle, ShapeAnimBody or a custom kind.
	Shape ShapeKind
	// Offset is added to Transform.Pos: the top-left corner of a rect, the
	// center of a circle.
	Offset gfx.Vec2
	// Size is the width and height of a rect, the radius (in X) of a
	// circle.
	Size gfx.Vec2
	// Layer is the bitmask of layers this collider is on. Zero means
	// DefaultLayer (layer 0).
	Layer Layers
	// Mask is the bitmask of layers this collider reacts to: it detects the
	// colliders on them. Zero reacts to nothing (a coin, a wall).
	Mask Layers
	// Static marks a collider that does not move on its own. Static pairs
	// are never tested; a static collider that is moved anyway is picked up
	// on the next tick.
	Static bool
	// Sensor reports contacts but never pushes (for physics; detection
	// treats every collider the same).
	Sensor bool
	// OneWay blocks only from one side (for physics): 0 every side, 1 up,
	// 2 down, 3 left, 4 right.
	OneWay OneWay
}
