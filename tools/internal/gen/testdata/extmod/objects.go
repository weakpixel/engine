package extmod

import "github.com/weakpixel/engine/gfx"

// Pos is a position local to the extension module.
type Pos struct{ X, Y float64 }

// Thing mixes a local component with one of the engine.
// +game:object
type Thing struct {
	// +game:component
	Pos *Pos

	// Position as the engine sees it.
	// +game:component
	Transform *gfx.Transform
}
