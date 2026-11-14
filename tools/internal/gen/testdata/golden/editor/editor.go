// +game:group:name=editor
//
// Package editor covers +game:editor: hints for how an editor shows and edits
// a field. They are schema data only; decoding and validation ignore them.
package editor

// +game:object
type Lamp struct {
	// +game:component
	Light *Light
}

// Shape is the area a light covers.
//
// +game:enum
type Shape uint8

// Shapes.
const (
	ShapePoint Shape = iota
	ShapeBox
	ShapeDisc
)

// Rect is a size.
type Rect struct{ W, H float32 }

// Light shows each hint.
type Light struct {
	// Tint is the light color.
	//
	// +game:editor:color=true,order=1
	Tint [4]float32
	// Packed is the same color as 0xRRGGBBAA.
	//
	// +game:editor:color=true
	Packed uint32
	// Power is how bright the light is.
	//
	// +game:editor:name="Brightness",min=0,max=10,step=0.5,order=0
	Power float32
	// Angle is in radians.
	//
	// +game:editor:unit=deg,min=0,max=360,step=5
	Angle float32
	// Fade is the fade time.
	//
	// +game:editor:unit=s,min=0
	Fade float32
	// Shape is the covered area.
	Shape Shape
	// Size is the box size.
	//
	// +game:editor:showIf=Shape=ShapeBox,min=1
	Size Rect
	// Radius is the disc radius.
	//
	// +game:editor:showIf=Shape=ShapeDisc
	Radius float32
	// Flicker turns the flicker on.
	Flicker bool
	// Rate is the flicker speed.
	//
	// +game:editor:showIf=Flicker
	Rate float32
	// Steady is the flicker-free brightness.
	//
	// +game:editor:showIf="!Flicker"
	Steady float32
	// Cache is internal bookkeeping.
	//
	// +game:editor:hidden=true
	Cache int32
}
