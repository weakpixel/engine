package hierarchy

import (
	"github.com/weakpixel/ecs"
	"github.com/weakpixel/engine/gfx"
)

// LayerMode says how a child's draw layer follows its parent's.
//
// +game:enum
type LayerMode uint8

// Layer modes.
const (
	// LayerRelative sets the child's layers to the parent's layer plus
	// [Local.LayerOffset] every tick.
	LayerRelative LayerMode = iota
	// LayerAbsolute leaves the child's layers alone.
	LayerAbsolute
)

// YSortSide says where a child sorts relative to its root in a Y-sorted
// layer.
//
// +game:enum
type YSortSide uint8

// Y-sort sides.
const (
	// YSortInFront sorts the child just in front of its root.
	YSortInFront YSortSide = iota
	// YSortBehind sorts the child just behind its root.
	YSortBehind
)

// Local is the placement of a child relative to its parent.
type Local struct {
	// Pos is the offset in the parent's space: scaled and rotated by the
	// parent's world Scale and Rot.
	Pos gfx.Vec2
	// Rot is added to the parent's rotation, in radians. Under a mirrored
	// parent (one negative scale axis) it is subtracted.
	//
	// +game:editor:unit=deg,step=5
	Rot float32
	// Scale multiplies the parent's scale. Zero means (1, 1).
	//
	// +game:default:X=1,Y=1
	Scale gfx.Vec2
	// LayerOffset is added to the parent's layer with LayerRelative.
	LayerOffset int16
	// LayerMode says how the child's layers follow the parent's.
	LayerMode LayerMode
	// YSortSide says where the child sorts relative to its root in a
	// Y-sorted layer.
	YSortSide YSortSide
}

// Parent attaches its entity to Entity. The entity also needs a
// gfx.Transform: the [System] overwrites it each tick with the world
// transform computed from the parent chain, so a serialized Transform of a
// child is derived data. Use [Attach] from code.
//
// +game:component
type Parent struct {
	// Entity is the parent. In a GOD level it is the id of an object of the
	// same document.
	Entity ecs.Entity
	// Local is the placement relative to the parent.
	Local Local

	// placed is set once the System has written the Transform, so the first
	// placement can also set PrevPos.
	placed bool
	// rootY is the Y of the root of the chain, for Y-sorting.
	rootY float32
}

// RootY returns the world Y of the root of the child's parent chain as of
// the last [System] run. The sprite extract uses it, plus or minus a small
// step, as the child's sort key.
func (p *Parent) RootY() float32 { return p.rootY }
