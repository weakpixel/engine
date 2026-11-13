// +game:group:name=enum
//
// Package enum covers +game:enum: an integer type whose constants become
// the x-enum of its schema wherever encoding/json reaches it.
package enum

// +game:object
type Emitter struct {
	// +game:component
	Shape *Shape
}

// Kind is the shape of an emitter.
//
// +game:enum
type Kind uint16

// Kind values. Unexported and other-typed constants are not members.
const (
	// KindPoint spawns at the origin.
	KindPoint  Kind = iota
	KindCircle      // KindCircle spawns inside a circle.
	KindRect

	kindHidden Kind = 9
)

// Untyped is not a member of Kind.
const Untyped = 5

// Plain is an integer type without the marker; it stays a number.
type Plain int

// Shape uses Kind directly, through a pointer, a slice and a map.
type Shape struct {
	Kind   Kind
	Maybe  *Kind
	Many   []Kind
	ByName map[string]Kind
	Size   Plain
}
