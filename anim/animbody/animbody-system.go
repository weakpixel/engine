// Package animbody keeps animated colliders in step with their animation: an
// entity whose collision.Collider has the shape collision.ShapeAnimBody takes
// its Offset and Size from the Body of the clip its gfx.Animator plays.
//
// It is opt-in, like collision itself: a game that uses anim-body colliders
// adds [System] to its schedule, and neither anim nor game imports collision.
package animbody

import (
	"github.com/weakpixel/asset"
	"github.com/weakpixel/ecs"
	"github.com/weakpixel/ecs/system"
	"github.com/weakpixel/engine/anim"
	"github.com/weakpixel/engine/collision"
	"github.com/weakpixel/engine/gfx"
)

// System writes, for every entity with a gfx.Animator and a collision.Collider
// of kind ShapeAnimBody, the collider's Offset and Size from the current
// clip's Body (the definition's fixed body, never the per-frame boxes, and
// unflipped), once the clip is loaded and only when they differ.
//
// It needs the *asset.Server resource. Add it to the simulate stage before the
// collision system, so the colliders are current when collisions are found.
// Run does not allocate.
type System struct {
	view  ecs.View2[gfx.Animator, collision.Collider]
	clips anim.ClipStore
}

// NewSystem returns the system for w.
func NewSystem(w *ecs.World) *System {
	return &System{view: ecs.NewView2[gfx.Animator, collision.Collider](w)}
}

// Init implements system.Initializer. It caches the Clip store, so register
// the loaders (anim.RegisterLoaders) and set the *asset.Server resource first.
func (s *System) Init(ctx *system.Context) {
	s.clips = anim.ClipStoreOf(ecs.GetResource[asset.Server](ctx.World))
}

// Run implements system.Runner.
func (s *System) Run(ctx *system.Context) {
	for _, c := range s.view.Each() {
		an, col := c.Unpack()
		if col.Shape != collision.ShapeAnimBody {
			continue
		}
		clip, state := s.clips.Lookup(an.Clip)
		if state != asset.Ready {
			continue
		}
		b := clip.Body
		off := gfx.Vec2{X: float32(b.X), Y: float32(b.Y)}
		size := gfx.Vec2{X: float32(b.W), Y: float32(b.H)}
		if col.Offset != off || col.Size != size {
			col.Offset, col.Size = off, size
		}
	}
}
