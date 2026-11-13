package physics

import (
	"fmt"
	"math/rand/v2"
	"testing"
	"time"

	"github.com/weakpixel/ecs"
	"github.com/weakpixel/ecs/system"
	"github.com/weakpixel/engine/collision"
	"github.com/weakpixel/engine/gfx"
)

// pile builds n dynamic boxes and circles above a tile floor, plus
// statics static colliders scattered over the floor area and a kinematic
// platform, and returns a function that runs one tick through a Schedule
// (which ages the event queues).
func pile(tb testing.TB, n, statics int) (tick func()) {
	w := ecs.NewWorld()
	sys := NewSystem(w, Config{Gravity: v(0, 980)})
	var sched system.Schedule
	sched.AddRunner(sys)
	r := rand.New(rand.NewPCG(1, 2))
	const width = 1600
	g := newGrid().row(40, -1, width/16+1) // floor top at y=640
	sys.Space().AddTiles(g, collision.TileOptions{Layer: lWall})
	for range statics {
		w.NewEntity(gfx.Transform{Pos: v(r.Float32()*width, 200+r.Float32()*400)},
			collision.Collider{Size: v(16, 8), Layer: lWall, Static: true, OneWay: collision.OneWayUp})
	}
	for i := range n {
		c := collision.Collider{Size: v(10, 10), Layer: lBody, Mask: collision.LayersOf(lBody, lWall)}
		if i%4 == 0 {
			c.Shape, c.Size = collision.ShapeCircle, v(5, 0)
		}
		w.NewEntity(gfx.Transform{Pos: v(r.Float32()*width, r.Float32()*600)}, c,
			Body{GravityScale: 1}, Velocity{}, Touching{})
	}
	plat := w.NewEntity(gfx.Transform{Pos: v(100, 500)}, collision.Collider{Size: v(64, 8), Layer: lWall},
		Body{Kind: Kinematic}, Velocity{}, Follow{})
	ticks := 0
	return func() {
		f := ecs.GetComp[Follow](w, plat)
		f.Target.X = 100 + float32(ticks%120)
		sched.Run(w, time.Second/60)
		ticks++
	}
}

// BenchmarkRun measures one physics tick: 1k bodies falling into a pile on
// a tile floor, and 100 bodies among 1k static one-way colliders.
func BenchmarkRun(b *testing.B) {
	for _, tc := range []struct {
		name            string
		bodies, statics int
	}{{"1k-pile", 1000, 0}, {"100-on-1k-static", 100, 1000}} {
		b.Run(tc.name, func(b *testing.B) {
			tick := pile(b, tc.bodies, tc.statics)
			for range 120 { // let the pile form
				tick()
			}
			b.ReportAllocs()
			for b.Loop() {
				tick()
			}
		})
	}
}

// velocityWorld builds n Velocity entities, a bodyShare fraction of them bodies
// (Body and Collider), the rest spread over three archetypes: Transform and
// Velocity, plus a Collider, plus a Sprite.
func velocityWorld(n int, bodyShare float32) *ecs.World {
	w := ecs.NewWorld()
	bodies := int(float32(n) * bodyShare)
	for i := range n {
		t := gfx.Transform{Pos: v(float32(i), 0)}
		vel := Velocity{Vel: v(1, 2)}
		switch {
		case i < bodies:
			w.NewEntity(t, vel, Body{}, collision.Collider{Size: v(4, 4)})
		case i%3 == 0:
			w.NewEntity(t, vel)
		case i%3 == 1:
			w.NewEntity(t, vel, collision.Collider{Size: v(4, 4)})
		default:
			w.NewEntity(t, vel, gfx.Sprite{})
		}
	}
	return w
}

// BenchmarkMoveSystem measures MoveSystem.Run over Velocity entities of which
// a share are bodies, which its view skips per archetype. The variant=today
// name keeps the cases comparable with bench/exclusion-before.txt.
func BenchmarkMoveSystem(b *testing.B) {
	ctx := &system.Context{}
	ctx.Time.Delta = time.Second / 60
	for _, n := range []int{1000, 10000} {
		for _, share := range []struct {
			name string
			f    float32
		}{{"0pct", 0}, {"25pct", 0.25}} {
			name := func(variant string) string {
				return fmt.Sprintf("n=%d/bodies=%s/variant=%s", n, share.name, variant)
			}
			b.Run(name("today"), func(b *testing.B) {
				m := NewMoveSystem(velocityWorld(n, share.f))
				m.Run(ctx)
				b.ReportAllocs()
				for b.Loop() {
					m.Run(ctx)
				}
			})
		}
	}
}
