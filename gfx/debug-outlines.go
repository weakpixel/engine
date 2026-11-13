package gfx

import "iter"

// DebugOutline is one outline the debug overlay draws in world space: the
// bounds of a collider, or a circle.
type DebugOutline struct {
	// Circle selects a circle of Radius around Center; otherwise the
	// outline is the rectangle Bounds.
	Circle bool
	// Bounds is the rectangle, in world units.
	Bounds Rect
	// Center and Radius describe the circle, in world units.
	Center Vec2
	Radius float32
	// Color is the outline color.
	Color Color
}

// DebugOutlines is a World resource: a source of outlines for the debug
// overlay (extract.DebugExtract), set by the package that owns the shapes
// (collision sets it for its Space). It lets the overlay draw shapes it
// knows nothing about, so extract needs no import of them.
type DebugOutlines struct {
	// Outlines yields the outlines of the last run. The yielded values are
	// copies. It may be nil.
	Outlines iter.Seq[DebugOutline]
}
