package physics

import (
	"testing"
	"time"

	"github.com/weakpixel/ecs"
	"github.com/weakpixel/ecs/system"
	"github.com/weakpixel/engine/gfx"
)

func TestSystemRunAllocs(t *testing.T) {
	tick := pile(t, 300, 300)
	for range 240 { // warm up: grow every buffer, map and event queue
		tick()
	}
	if a := testing.AllocsPerRun(50, tick); a != 0 {
		t.Errorf("System.Run allocs = %v, want 0", a)
	}
}

func TestMoveSystemAllocs(t *testing.T) {
	w := ecs.NewWorld()
	var sched system.Schedule
	sched.AddRunner(NewMoveSystem(w))
	for i := range 100 {
		w.NewEntity(gfx.Transform{}, Velocity{Vel: v(float32(i), 1)})
	}
	w.NewEntity(gfx.Transform{}, Velocity{}, Body{})
	sched.Run(w, time.Second/60)
	if a := testing.AllocsPerRun(50, func() { sched.Run(w, time.Second/60) }); a != 0 {
		t.Errorf("MoveSystem allocs = %v, want 0", a)
	}
}
