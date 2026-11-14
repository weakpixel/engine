package render

import (
	"math/rand/v2"
	"testing"
)

func BenchmarkSort10k(b *testing.B) {
	rng := rand.New(rand.NewPCG(1, 2))
	e, sprites := testSprites(b, 8)
	st := StoresOf(e.srv).Sprites
	var q Queue
	cfg := &LayerConfig{}
	cfg.Set(1, LayerOptions{YSort: true})
	for range 10_000 {
		q.Push(Item{Layer: int16(rng.IntN(3)), SortY: rng.Float32() * 1000, Sprite: sprites[rng.IntN(8)]})
	}
	// Sorting an already sorted queue is the common case (little changes
	// between ticks), so shuffle the items before every iteration instead.
	items := append([]Item(nil), q.items...)
	b.ReportAllocs()
	for b.Loop() {
		b.StopTimer()
		copy(q.items, items)
		b.StartTimer()
		q.Sort(cfg, st)
	}
}
