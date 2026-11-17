package game

import (
	"testing"

	"github.com/weakpixel/ecs"
	"github.com/weakpixel/ecs/system"
	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/input"
	"github.com/weakpixel/engine/render"
)

// shapeExtract queues one rect per Run on a layer below the sprite's.
type shapeExtract struct {
	queue       *render.Queue
	inits, runs int
}

func (s *shapeExtract) Init(ctx *system.Context) {
	s.queue = ecs.GetResource[render.Queue](ctx.World)
	s.inits++
}

func (s *shapeExtract) Run(*system.Context) {
	s.runs++
	s.queue.Push(render.Item{Kind: render.KindRect, Layer: -1, Scale: gfx.Vec2{X: 1, Y: 1}, Size: gfx.Vec2{X: 4, Y: 4}})
}

// TestAddExtract: an AddExtract runner is initialized once, runs every tick
// even while a Tool pauses the game, and its items are sorted with the
// engine's (the rect on layer -1 comes before the sprite on layer 0).
func TestAddExtract(t *testing.T) {
	tool := &testTool{script: []TickMode{TickRun, TickPause, TickPause}}
	e := New(Config{NoAudio: true, Tool: tool, Input: InputConfig{Source: &input.ManualSource{}}})
	defer e.Close()
	x := &shapeExtract{}
	e.AddExtract(x)
	e.Start(func(e *Engine) {
		ecs.NewEntity2(e.World(), gfx.Transform{}, gfx.Sprite{})
	})
	for range 3 {
		if err := e.Update(); err != nil {
			t.Fatal(err)
		}
	}
	if x.inits != 1 || x.runs != 3 {
		t.Fatalf("inits=%d runs=%d, want 1 and 3", x.inits, x.runs)
	}
	q := ecs.GetResource[render.Queue](e.World())
	if q.Len() != 2 {
		t.Fatalf("queue has %d items, want the rect and the sprite", q.Len())
	}
	if q.At(0).Kind != render.KindRect || q.At(1).Kind != render.KindSprite {
		t.Fatalf("queue order %d, %d; want the rect first", q.At(0).Kind, q.At(1).Kind)
	}
}

func TestAddExtractPanics(t *testing.T) {
	e := New(Config{NoAudio: true})
	defer e.Close()
	mustPanic(t, "game: nil system", func() { e.AddExtract(nil) })
	e.Update()
	mustPanic(t, "game: AddSystem after the first Update", func() { e.AddExtract(&shapeExtract{}) })
}
