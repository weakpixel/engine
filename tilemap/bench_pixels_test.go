//go:build ebitentest

package tilemap

import (
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/weakpixel/engine/render"
)

// BenchmarkTilemapFrame measures the CPU cost of a whole frame of a map, on
// the scenes of BenchmarkTilemapExtract: TilemapExtract.Run, Queue.Sort and
// Renderer.Draw onto a 640x360 image. The GPU work runs outside the timer.
// It needs a running game: run it with -tags ebitentest (and a display).
func BenchmarkTilemapFrame(b *testing.B) {
	for _, sc := range benchScenes {
		for _, chunked := range []bool{false, true} {
			name := sc.name
			if chunked {
				name += "-chunked"
			}
			b.Run(name, func(b *testing.B) {
				e := sc.env(b, chunked)
				e.warmUp()
				st := render.StoresOf(e.srv)
				r := render.NewRenderer(st)
				r.SetCanvases(e.canvases)
				dst := ebiten.NewImage(640, 360)
				b.ReportAllocs()
				for b.Loop() {
					e.run(16 * time.Millisecond)
					e.queue.Sort(nil, st.Sprites)
					r.Draw(dst, e.queue, e.cam, 1)
					// Reading a pixel makes Ebiten run the queued commands,
					// so they do not pile up over the loop.
					b.StopTimer()
					_ = dst.At(0, 0)
					b.StartTimer()
				}
				b.ReportMetric(float64(e.queue.Len()), "items/op")
				b.ReportMetric(float64(r.Stats().DrawCalls), "calls/op")
			})
		}
	}
}
