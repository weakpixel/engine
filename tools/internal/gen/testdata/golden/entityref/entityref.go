// +game:group:name=entityref
//
// Package entityref covers ecs.Entity fields: they become entity references
// ({"type":"string","format":"entity","nullable":true}) wherever encoding/json
// reaches them.
package entityref

import "github.com/weakpixel/ecs"

// +game:object
type Follower struct {
	// +game:component
	Links *Links
}

// Links holds every shape an entity reference can take.
type Links struct {
	Target   ecs.Entity
	Owner    *ecs.Entity
	Group    []ecs.Entity
	Pair     [2]ecs.Entity
	ByName   map[string]ecs.Entity
	Nested   Nested
	Distance float64
}

type Nested struct {
	Parent ecs.Entity
}
