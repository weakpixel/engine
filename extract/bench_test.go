package extract_test

import (
	"fmt"
	"testing"

	"github.com/weakpixel/engine/gfx"
)

// BenchmarkExtract measures a whole render-stage tick (snapshot, reset,
// four extract systems, sort) over n sprites on 3 layers, about 90% of them
// inside the camera.
func BenchmarkExtract(b *testing.B) {
	for _, n := range []int{1_000, 10_000} {
		b.Run(fmt.Sprintf("%dk", n/1000), func(b *testing.B) {
			s := newScene(b)
			for i := range n {
				s.spawn(gfx.Vec2{X: float32(i%110) - 55, Y: float32(i % 70)},
					gfx.Sprite{ID: s.h(i%5 + 1), Layer: int16(i % 3)})
			}
			s.tick() // Init and warm-up
			b.ReportAllocs()
			for b.Loop() {
				s.tick()
			}
		})
	}
}
