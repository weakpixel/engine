package tween

import (
	"testing"

	"github.com/weakpixel/engine/gfx"
)

// BenchmarkTweens measures one Update with 10,000 running tweens that repeat
// forever, so the set stays the same across iterations.
func BenchmarkTweens(b *testing.B) {
	b.Run("pos-10k", func(b *testing.B) {
		w, tw := newTweens()
		for i := range 10_000 {
			e := w.NewEntity(gfx.Transform{Pos: gfx.Vec2{X: float32(i)}})
			To(tw, e, Pos, gfx.Vec2{X: 100, Y: 50}, 1).Repeat(-1).Yoyo().Ease(InOutSine)
		}
		benchUpdate(b, tw)
	})
	b.Run("mixed-10k-on-2.5k", func(b *testing.B) {
		w, tw := newTweens()
		for i := range 2_500 {
			e := w.NewEntity(gfx.Transform{Pos: gfx.Vec2{X: float32(i)}}, gfx.Sprite{})
			To(tw, e, Pos, gfx.Vec2{X: 100, Y: 50}, 1).Repeat(-1).Yoyo().Ease(InOutSine)
			To(tw, e, Scale, gfx.Vec2{X: 2, Y: 2}, 0.5).Repeat(-1).Ease(OutBack)
			To(tw, e, Tint, gfx.RGBA(255, 0, 0, 0), 0.7).Repeat(-1).Yoyo()
			To(tw, e, Param(0), 1, 0.3).Repeat(-1).Yoyo()
		}
		benchUpdate(b, tw)
	})
}

func benchUpdate(b *testing.B, tw *Tweens) {
	tw.Update(1.0 / 60) // start them all
	b.ReportAllocs()
	for b.Loop() {
		tw.Update(1.0 / 60)
	}
	if tw.Len() != 10_000 {
		b.Fatalf("running %d", tw.Len())
	}
}
