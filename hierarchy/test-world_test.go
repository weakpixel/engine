package hierarchy

import (
	"strings"
	"testing"
	"time"

	"github.com/weakpixel/ecs"
	"github.com/weakpixel/ecs/system"
	"github.com/weakpixel/engine/gfx"
)

const tick = time.Second / 60

// testWorld is a World with a hierarchy System run through a Schedule.
type testWorld struct {
	t     *testing.T
	w     *ecs.World
	sched system.Schedule
}

func newTestWorld(t *testing.T) *testWorld {
	w := ecs.NewWorld()
	tw := &testWorld{t: t, w: w}
	tw.sched.AddRunner(NewSystem(w))
	return tw
}

func (tw *testWorld) run(n int) {
	for range n {
		tw.sched.Run(tw.w, tick)
	}
}

// child spawns an entity attached to parent at offset pos.
func (tw *testWorld) child(parent ecs.Entity, pos gfx.Vec2, values ...any) ecs.Entity {
	vals := []any{gfx.Transform{}, Parent{Entity: parent, Local: Local{Pos: pos}}}
	return tw.w.NewEntity(append(vals, values...)...)
}

func (tw *testWorld) transform(e ecs.Entity) gfx.Transform {
	return *ecs.GetComp[gfx.Transform](tw.w, e)
}

func mustPanic(t *testing.T, want string, f func()) {
	t.Helper()
	defer func() {
		r := recover()
		if r == nil {
			t.Fatalf("expected panic containing %q", want)
		}
		if msg, _ := r.(string); !strings.Contains(msg, want) {
			t.Fatalf("panic %v does not contain %q", r, want)
		}
	}()
	f()
}
