package camera

import "github.com/weakpixel/engine/gfx"

// Target marks the entity the camera follows; the entity also needs a
// gfx.Transform. It is a runtime-only component, not part of any Game
// Object schema: add it with ecs.SetComp.
//
// With several targets the highest Priority wins, and among equal
// priorities the entity with the lowest ecs.Entity ID (stable, unlike view
// order, which changes when entities move between archetypes). With none the
// System leaves the camera alone.
type Target struct {
	// Offset is added to the target's position: the camera aims at
	// Transform.Pos + Offset, for example to frame a character's head.
	Offset gfx.Vec2
	// Priority picks among several targets; higher wins.
	Priority int16
}
