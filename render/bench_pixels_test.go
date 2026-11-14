//go:build ebitentest

package render

import (
	"testing"
	"testing/fstest"

	"github.com/weakpixel/engine/gfx"
)

// The Draw benchmarks need a running game: outside one, Ebiten queues every
// draw command without ever flushing it. Run with -tags ebitentest. Each
// has a batched and an unbatched (one DrawImage per quad) variant, and
// measures the CPU cost of Draw only: the GPU work runs outside the timer.

// BenchmarkDraw10k draws 10k rotated sprites of one texture over 3 layers.
func BenchmarkDraw10k(b *testing.B) {
	r, id := testRenderer(b)
	var q Queue
	for i := range 10_000 {
		it := q.Sprite(int16(i%3), id, gfx.Vec2{X: float32(i % 600), Y: float32(i % 340)}, gfx.Vec2{X: float32(i%600) - 1, Y: float32(i % 340)})
		it.Rot = 0.3
	}
	q.Sort(nil, r.stores.Sprites)
	benchDraw(b, r, &q)
}

// BenchmarkDrawTiles draws a 100x100 map of 16 different tiles of one atlas
// on one layer, like TilemapExtract queues them.
func BenchmarkDrawTiles(b *testing.B) {
	e, tiles := testSprites(b, 16)
	r := NewRenderer(StoresOf(e.srv))
	var q Queue
	for y := range 100 {
		for x := range 100 {
			p := gfx.Vec2{X: float32(x * 4), Y: float32(y * 4)}
			q.Sprite(0, tiles[(x*7+y*3)%16], p, p).Origin = topLeft
		}
	}
	q.Sort(nil, r.stores.Sprites)
	benchDraw(b, r, &q)
}

// BenchmarkDrawParticles draws 10k tinted, additive copies of one sprite.
func BenchmarkDrawParticles(b *testing.B) {
	r, id := testRenderer(b)
	var q Queue
	for i := range 10_000 {
		p := gfx.Vec2{X: float32(i*37%600) + 0.5, Y: float32(i*53%340) + 0.25}
		it := q.Sprite(2, id, p, p)
		it.Blend = BlendAdditive
		it.Color = gfx.RGBA(255, uint8(i), 64, uint8(100+i%150))
		it.Scale = gfx.Vec2{X: 0.5, Y: 0.5}
	}
	q.Sort(nil, r.stores.Sprites)
	benchDraw(b, r, &q)
}

// BenchmarkDrawShaded draws 10k rotated, tinted sprites through one shader
// on one layer: of one frame with one Params (particles of one effect), of
// 16 frames of one atlas (a shaded tile layer), and of one frame with 10
// different Params (flashing sprites).
func BenchmarkDrawShaded(b *testing.B) {
	files := fstest.MapFS{"fx/flash.kage": textFile(kageFlash)}
	e := newAssetEnv(b, files, 0)
	frames := e.loadSheet(files, "s", 16)
	sc := e.srv.NewScope("fx")
	flash := LoadShader(sc, "fx/flash.kage")
	e.settle(sc)
	r := NewRenderer(StoresOf(e.srv))
	for _, scene := range []struct {
		name   string
		frame  func(i int) int
		params func(i int) float32
	}{
		{"oneFrameOneParams", func(int) int { return 0 }, func(int) float32 { return 0.5 }},
		{"manyFrames", func(i int) int { return i % 16 }, func(int) float32 { return 0.5 }},
		{"manyParams", func(int) int { return 0 }, func(i int) float32 { return float32(i%10) / 10 }},
	} {
		b.Run(scene.name, func(b *testing.B) {
			var q Queue
			for i := range 10_000 {
				p := gfx.Vec2{X: float32(i*37%600) + 0.5, Y: float32(i*53%340) + 0.25}
				it := q.Sprite(0, frames[scene.frame(i)], p, p)
				it.Rot = 0.3
				it.Color = gfx.RGBA(255, 128, 128, 255)
				it.Shader = flash
				it.Params = [4]float32{scene.params(i)}
			}
			q.Sort(nil, r.stores.Sprites)
			benchDraw(b, r, &q)
		})
	}
}

func benchDraw(b *testing.B, r *Renderer, q *Queue) {
	dst := newTarget(640, 360)
	cam := &Camera{}
	for _, mode := range []struct {
		name    string
		noBatch bool
	}{{"batched", false}, {"unbatched", true}} {
		b.Run(mode.name, func(b *testing.B) {
			r.noBatch = mode.noBatch
			b.ReportAllocs()
			for b.Loop() {
				r.Draw(dst, q, cam, 0.5)
				// Reading a pixel makes Ebiten run the queued commands, so
				// they do not pile up over the loop; it is not part of the
				// measured cost of Draw.
				b.StopTimer()
				_ = dst.At(0, 0)
				b.StartTimer()
			}
			b.ReportMetric(float64(r.Stats().DrawCalls), "calls/op")
		})
	}
}
