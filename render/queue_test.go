package render

import (
	"math"
	"testing"

	"github.com/weakpixel/engine/gfx"
)

func TestQueuePushResetLen(t *testing.T) {
	_, sprites := testSprites(t, 1)
	var q Queue
	q.Push(Item{Layer: 1, Text: "hello"})
	it := q.Sprite(2, sprites[0], gfx.Vec2{X: 1, Y: 2}, gfx.Vec2{})
	it.FlipX = true
	if q.Len() != 2 {
		t.Fatalf("Len = %d, want 2", q.Len())
	}
	if got := q.At(1); got.Sprite != sprites[0] || !got.FlipX || got.Scale != (gfx.Vec2{X: 1, Y: 1}) {
		t.Fatalf("At(1) = %+v", got)
	}
	q.Reset()
	if q.Len() != 0 {
		t.Fatalf("Len after Reset = %d", q.Len())
	}
	if cap(q.items) == 0 {
		t.Fatal("Reset dropped the capacity")
	}
}

func TestQueueCulled(t *testing.T) {
	var q Queue
	q.AddCulled(3)
	q.AddCulled(4)
	if n := q.Culled(); n != 7 {
		t.Fatalf("Culled = %d, want 7", n)
	}
	q.Reset()
	if n := q.Culled(); n != 0 {
		t.Fatalf("Culled after Reset = %d, want 0", n)
	}
}

func TestQueueNoAllocs(t *testing.T) {
	e, sprites := testSprites(t, 7)
	st := StoresOf(e.srv).Sprites
	var q Queue
	var cfg LayerConfig
	cfg.Set(1, LayerOptions{YSort: true})
	fill := func() {
		q.Reset()
		for i := range 1000 {
			q.Sprite(int16(i%3), sprites[i%7], gfx.Vec2{X: float32(i), Y: float32(i % 13)}, gfx.Vec2{}).SortY = float32(i % 13)
		}
		q.Sort(&cfg, st)
	}
	fill() // warm up: grow the slices once
	if n := testing.AllocsPerRun(50, fill); n != 0 {
		t.Fatalf("allocs per Reset+Push+Sort = %v, want 0", n)
	}
}

func TestQueueMinLayer(t *testing.T) {
	var q Queue
	push := func() {
		q.Reset()
		for _, l := range []int16{5, -1, 0, 3, -7} {
			q.Push(Item{Layer: l})
		}
	}
	layers := func() (got []int16) {
		for i := range q.Len() {
			got = append(got, q.At(i).Layer)
		}
		return got
	}

	push()
	q.Sort(nil, SpriteStore{})
	if got := layers(); len(got) != 5 || q.Culled() != 0 {
		t.Fatalf("no floor: layers %v, culled %d", got, q.Culled())
	}

	q.SetMinLayer(0)
	push()
	q.Sort(nil, SpriteStore{})
	if got := layers(); len(got) != 3 || got[0] != 0 || got[1] != 3 || got[2] != 5 || q.Culled() != 2 {
		t.Fatalf("floor 0: layers %v, culled %d, want [0 3 5] and 2", got, q.Culled())
	}

	q.SetMinLayer(math.MinInt16)
	push()
	q.Sort(nil, SpriteStore{})
	if q.Len() != 5 {
		t.Fatalf("floor removed: Len = %d, want 5", q.Len())
	}
}
