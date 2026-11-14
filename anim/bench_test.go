package anim

import (
	"fmt"
	"testing"
	"time"

	"github.com/weakpixel/engine/gfx"
)

// BenchmarkAnimation is one AnimationSystem tick over n animated entities
// that all play the 4-frame "run" clip.
func BenchmarkAnimation(b *testing.B) {
	for _, n := range []int{1_000, 10_000} {
		b.Run(fmt.Sprintf("%dk", n/1000), func(b *testing.B) {
			s := newAnimScene(b)
			for range n {
				s.spawn(gfx.Animator{Clip: s.clip("run")})
			}
			s.tick(time.Millisecond) // runs Init
			b.ReportAllocs()
			b.ResetTimer()
			for range b.N {
				s.tick(16 * time.Millisecond)
			}
		})
	}
}
