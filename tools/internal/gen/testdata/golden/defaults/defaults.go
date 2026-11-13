// +game:group:name=defaults
//
// Package defaults covers +game:default: the value an editor writes when it
// creates a component. Decoding never applies it.
package defaults

import "github.com/weakpixel/engine/tools/internal/gen/testdata/golden/defaults/paint"

// +game:object
type Label struct {
	// +game:component
	Text *Text
}

// Mode is how text is drawn.
//
// +game:enum
type Mode uint8

// Modes.
const (
	ModePlain Mode = iota
	ModeBold
)

// Tile is a size constant usable in a default.
const Tile = 16

// Text shows the field forms.
type Text struct {
	// +game:default="Hello, world"
	Caption string
	// +game:default=1.5
	Size float64
	// +game:default=true
	Visible bool
	// +game:default=ModeBold
	Mode Mode
	// +game:default=paint.Red
	Color paint.Color
	// +game:default=2 * Tile
	Step int32
	// +game:default=0.1
	Alpha float32
	// +game:default=3
	Maybe *int
	// +game:default={1, 2, 3}
	Steps []int
	// +game:default={"a, b", "c"}
	Tags []string
	// +game:default={1.5, 2.5}
	Pair [2]float64
	// +game:default:walk=2,run=4
	Speeds map[string]float32
	// +game:default:1="one"
	// +game:default:2="two"
	Names map[int]string

	// Pos keeps the inner defaults of paint.Vec.
	Pos paint.Vec
	// +game:default:X=10
	Override paint.Vec
	// +game:default:X=0,Y=0
	Zeroed paint.Vec
	// +game:default:Offset.X=2
	// +game:default:Offset.Y=-1
	Deep Shape
}

// Shape nests a vector.
type Shape struct {
	Offset paint.Vec
	// +game:default:Inner.N=7
	Rest Frame
}

// Frame is two levels down.
type Frame struct {
	Inner struct{ N int }
}
