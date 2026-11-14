package extmod_test

import (
	"testing"

	"extmod"

	"github.com/weakpixel/ecs"
	"github.com/weakpixel/engine/gfx"
)

func TestThing(t *testing.T) {
	world := ecs.NewWorld()
	things := extmod.NewThingView(world)

	e := things.NewEntity(extmod.Thing{
		Pos:       &extmod.Pos{X: 1, Y: 2},
		Transform: &gfx.Transform{Pos: gfx.Vec2{X: 3, Y: 4}},
	})

	got := things.Get(e)
	if got.Pos.X != 1 || got.Transform.Pos.X != 3 {
		t.Fatalf("Get = %+v", got)
	}
}
