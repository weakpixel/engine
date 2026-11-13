// +game:group:name=docs
//
// Package docs checks that Go doc comments reach the JSON definition.
package docs

// Common is shared by the objects of the package.
//
// +game:object
type Common struct {
	// Where the object is.
	// +game:component
	Transform *Transform
}

// Hero is the player character.
// +game:object
type Hero struct {
	// +game:embed
	Common

	// Stores the hero's health.
	// +game:component
	Health *Health

	// +game:component
	Plain *Plain // Trailing comment of the field.
}

// Transform is a position in the world.
type Transform struct {
	// Horizontal position.
	X float64
	Y float64 // Vertical position.
}

// Health is hit points.
//
// It never drops below zero.
// +game:component
type Health struct {
	Current, Max int
}

type Plain struct{ V int }
