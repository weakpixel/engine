package particles

import (
	"image"
	"testing"

	"github.com/weakpixel/ecs"
	"github.com/weakpixel/engine/extract"
	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/particles/fxfile"
	"github.com/weakpixel/engine/render"
)

// benchScene fills emitters emitters so that each holds about perEmitter
// live particles in the steady state (Rate x life).
func benchScene(b *testing.B, emitters, perEmitter int) (*ecs.World, *System, *Extract) {
	b.Helper()
	w, s, _ := newTestSystem()
	x := benchExtract(w, s)
	for range emitters {
		spawnEmitter(w, fxfile.Emitter{
			Rate: float32(perEmitter), LifeMin: 1, LifeMax: 1, SpeedMax: 20, Spread: 6,
			Shape: gfx.EmitCircle, ShapeSize: gfx.Vec2{X: 4}, Max: uint16(perEmitter + 16),
			EndScale: 0.5, FadeOut: 0.3,
		})
	}
	for range 90 { // warm up past one full life
		s.Run(ctxOf(w))
	}
	return w, s, x
}

// benchExtract builds the Extract with the resources the tests use, with a
// camera that sees the emitters at the origin.
func benchExtract(w *ecs.World, s *System) *Extract {
	ecs.SetResource(w, &render.Queue{})
	ecs.SetResource(w, &render.Camera{Zoom: 1})
	ecs.SetResource(w, &extract.Screen{Size: image.Pt(200, 200)})
	x := NewExtract(w, s)
	x.Init(ctxOf(w))
	return x
}

func benchmarkSystem(b *testing.B, emitters, per int) {
	w, s, _ := benchScene(b, emitters, per)
	ctx := ctxOf(w)
	b.ReportAllocs()
	for b.Loop() {
		s.Run(ctx)
	}
}

func benchmarkExtract(b *testing.B, emitters, per int) {
	w, _, x := benchScene(b, emitters, per)
	ctx := ctxOf(w)
	b.ReportAllocs()
	for b.Loop() {
		x.queue.Reset()
		x.Run(ctx)
	}
	b.ReportMetric(float64(x.queue.Len()), "particles")
}

func BenchmarkParticleSystem(b *testing.B) {
	b.Run("100x100", func(b *testing.B) { benchmarkSystem(b, 100, 100) })
	b.Run("1x10k", func(b *testing.B) { benchmarkSystem(b, 1, 10000) })
}

func BenchmarkParticleExtract(b *testing.B) {
	b.Run("100x100", func(b *testing.B) { benchmarkExtract(b, 100, 100) })
	b.Run("1x10k", func(b *testing.B) { benchmarkExtract(b, 1, 10000) })
}
