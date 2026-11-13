// +game:group:name=zero
//
// Package zero covers an object with 0 required components (only an
// optional one): the untyped world.Query() path with no IDs.
package zero

// +game:object
type Ghost struct {
	// +game:component
	// +game:optional
	Tag *Tag
}

type Tag struct{ Name string }
