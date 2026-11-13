package hierarchy

import (
	"fmt"
	"testing"

	"github.com/weakpixel/ecs"
	"github.com/weakpixel/ecs/system"
	"github.com/weakpixel/engine/gfx"
)

// forest builds roots with chains of the given depth and about n children in
// all, each with a Sprite so the layer path runs too.
func forest(n, depth int) (*ecs.World, *System) {
	w := ecs.NewWorld()
	for range n / depth {
		e := w.NewEntity(gfx.Transform{Pos: v(10, 10)}, gfx.Sprite{Layer: 1})
		for range depth {
			e = w.NewEntity(gfx.Transform{}, gfx.Sprite{},
				Parent{Entity: e, Local: Local{Pos: v(1, 1), Rot: 0.1, LayerOffset: 1}})
		}
	}
	return w, NewSystem(w)
}

func TestSystemAllocs(t *testing.T) {
	for _, depth := range []int{1, 3} {
		t.Run(fmt.Sprintf("depth%d", depth), func(t *testing.T) {
			w, sys := forest(999, depth)
			ctx := &system.Context{World: w}
			sys.Run(ctx) // warm up: first run sizes the delete slice
			if got := testing.AllocsPerRun(20, func() { sys.Run(ctx) }); got != 0 {
				t.Fatalf("System.Run allocates %v per run, want 0", got)
			}
		})
	}
}

func BenchmarkSystem(b *testing.B) {
	for _, depth := range []int{1, 3} {
		b.Run(fmt.Sprintf("1000children/depth%d", depth), func(b *testing.B) {
			w, sys := forest(999, depth)
			ctx := &system.Context{World: w}
			b.ReportAllocs()
			for b.Loop() {
				sys.Run(ctx)
			}
		})
	}
}
