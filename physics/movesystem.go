package physics

import (
	"time"

	"github.com/weakpixel/ecs"
	"github.com/weakpixel/ecs/system"
	"github.com/weakpixel/engine/gfx"
)

// MoveSystem moves every entity with a gfx.Transform and a [Velocity] but no
// [Body] by Velocity x dt each tick: no gravity, no collision. Use it for
// bullets, drifting clouds and props; bodies are left to the physics
// [System]. It works without the physics System, for example next to a
// collision.System that reports bullet hits.
type MoveSystem struct {
	view ecs.View2[gfx.Transform, Velocity] // without Body: skipped per archetype
}

// NewMoveSystem returns the MoveSystem of w.
func NewMoveSystem(w *ecs.World) *MoveSystem {
	return &MoveSystem{view: ecs.NewView2[gfx.Transform, Velocity](w, ecs.Without[Body]())}
}

// Run implements system.Runner.
func (m *MoveSystem) Run(ctx *system.Context) {
	dt := float32(ctx.Time.Delta) / float32(time.Second)
	for _, c := range m.view.Each() {
		c.Comp1.Pos = c.Comp1.Pos.Add(c.Comp2.Vel.Mul(dt))
	}
}
