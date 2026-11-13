package collision

import (
	"math/rand/v2"
	"strconv"
	"testing"
	"time"

	"github.com/weakpixel/ecs"
	"github.com/weakpixel/ecs/system"
	"github.com/weakpixel/engine/gfx"
)

// BenchmarkSystemRun measures one detection tick with n moving colliders and
// 10k static ones, spread uniformly or clustered in a small area.
func BenchmarkSystemRun(b *testing.B) {
	for _, n := range []int{1000, 10000} {
		for _, layout := range []struct {
			name string
			size float32
		}{{"uniform", 4000}, {"clustered", 800}} {
			b.Run(strconv.Itoa(n)+"/"+layout.name, func(b *testing.B) {
				_, tick := populate(b, n, 10000, layout.size)
				for range 10 {
					tick()
				}
				b.ReportAllocs()
				for b.Loop() {
					tick()
				}
			})
		}
	}
}

func BenchmarkRaycast(b *testing.B) {
	sys, tick := populate(b, 1000, 10000, 4000)
	tick()
	sp := sys.Space()
	b.ReportAllocs()
	for b.Loop() {
		sp.Raycast(v(0, 0), v(4000, 3900), AllLayers)
	}
}

// BenchmarkBullets measures one tick of a bullet-hell scene: 5,000 bullets
// that only detect enemies and walls, 200 enemies that detect bullets and
// 10k statics, all over 4000x4000. Bullet pairs share cells but never react.
func BenchmarkBullets(b *testing.B) {
	w := ecs.NewWorld()
	sys := NewSystem(w, Config{Stay: true})
	var sched system.Schedule
	sched.AddRunner(sys)
	r := rand.New(rand.NewPCG(7, 8))
	pos := func() gfx.Transform { return gfx.Transform{Pos: v(r.Float32()*4000, r.Float32()*4000)} }
	bullets := make([]ecs.Entity, 5000)
	for i := range bullets {
		bullets[i] = w.NewEntity(pos(), Collider{Size: v(4, 4), Layer: lBullet, Mask: LayersOf(lEnemy, lWall)})
	}
	for range 200 {
		w.NewEntity(pos(), Collider{Size: v(16, 16), Layer: lEnemy, Mask: LayersOf(lBullet)})
	}
	for range 10000 {
		w.NewEntity(pos(), Collider{Size: v(16, 16), Layer: lWall, Static: true})
	}
	ticks := 0
	tick := func() {
		d := float32(ticks%7) - 3
		for _, e := range bullets {
			ecs.GetComp[gfx.Transform](w, e).Pos.X += d
		}
		sched.Run(w, time.Second/60)
		ticks++
	}
	for range 10 {
		tick()
	}
	b.ReportAllocs()
	for b.Loop() {
		tick()
	}
}
