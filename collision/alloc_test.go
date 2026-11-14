package collision

import (
	"math/rand/v2"
	"testing"
	"time"

	"github.com/weakpixel/ecs"
	"github.com/weakpixel/ecs/system"
	"github.com/weakpixel/engine/gfx"
)

// populate spawns n dynamic colliders and statics static ones, all in a
// square of side size, plus a tile floor. tick moves the dynamic ones a
// little, so pairs start and end, and runs the system through a Schedule,
// which ages the event queues.
func populate(tb testing.TB, n, statics int, size float32) (sys *System, tick func()) {
	w := ecs.NewWorld()
	sys = NewSystem(w, Config{Stay: true}) // the costlier setting
	var sched system.Schedule
	sched.AddRunner(sys)
	r := rand.New(rand.NewPCG(5, 6))
	dyn := make([]ecs.Entity, n)
	for i := range dyn {
		c := Collider{Size: v(8, 12), Layer: lEnemy, Mask: LayersOf(lEnemy, lWall)}
		if i%4 == 0 {
			c.Shape, c.Size = ShapeCircle, v(5, 0)
		}
		dyn[i] = w.NewEntity(gfx.Transform{Pos: v(r.Float32()*size, r.Float32()*size)}, c)
	}
	for range statics {
		w.NewEntity(gfx.Transform{Pos: v(r.Float32()*size, r.Float32()*size)}, Collider{Size: v(16, 16), Layer: lWall, Static: true})
	}
	sys.Space().AddTiles(floor(int(size/16), -1, int(size/16)+1), TileOptions{Layer: lWall})
	ticks := 0
	return sys, func() {
		d := float32(ticks%7) - 3
		for _, e := range dyn {
			ecs.GetComp[gfx.Transform](w, e).Pos.X += d
		}
		sched.Run(w, time.Second/60)
		ticks++
	}
}

func TestSystemRunAllocs(t *testing.T) {
	_, run := populate(t, 300, 300, 400)
	for range 20 { // warm up: grow every buffer and the event queues
		run()
	}
	if a := testing.AllocsPerRun(50, run); a != 0 {
		t.Errorf("System.Run allocs = %v, want 0", a)
	}
}

func TestQueryAllocs(t *testing.T) {
	sys, run := populate(t, 300, 300, 400)
	run()
	sp := sys.Space()
	e := sp.AllContacts()[0].Self
	n := 0
	tests := map[string]func(){
		"Contacts": func() { n += len(sp.Contacts(e)) },
		"QueryRect": func() {
			for range sp.QueryRect(rect(100, 100, 200, 200), AllLayers) {
				n++
			}
		},
		"QueryPoint": func() {
			for range sp.QueryPoint(v(150, 150), AllLayers) {
				n++
			}
		},
		"Raycast": func() {
			if _, ok := sp.Raycast(v(0, 0), v(400, 420), AllLayers); ok {
				n++
			}
		},
	}
	for name, f := range tests {
		t.Run(name, func(t *testing.T) {
			f()
			if a := testing.AllocsPerRun(50, f); a != 0 {
				t.Errorf("allocs = %v, want 0", a)
			}
		})
	}
}
