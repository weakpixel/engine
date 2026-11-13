package fxedit

import "github.com/weakpixel/engine/gfx"

// Visibility says which optional fields of the panel show for an emitter:
// fields appear only when they act (roadmap M4c). The rules live here, in Go;
// when the schema gets field hints they move there.
type Visibility struct {
	// Change shows End color and End scale (change over life is on).
	Change bool
	// Loop shows the Loop checkbox (a cycle has a Duration).
	Loop bool
	// Radius shows the circle's radius, Size the rect's width and height.
	Radius, Size bool
	// Shader shows the shader parameters (a shader is set).
	Shader bool
}

// Visible returns what shows for e.
func Visible(e Emitter) Visibility {
	return Visibility{
		Change: ChangesOverLife(e),
		Loop:   e.Duration > 0,
		Radius: e.Shape == gfx.EmitCircle,
		Size:   e.Shape == gfx.EmitRect,
		Shader: e.Shader != "",
	}
}
