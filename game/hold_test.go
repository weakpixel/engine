package game

import (
	"testing"

	"github.com/weakpixel/ecs"
	"github.com/weakpixel/ecs/system"
	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/tween"
)

// stageCounts counts the ticks each game stage ran in.
type stageCounts struct{ input, simulate, camera, post int }

func countStages(e *Engine) *stageCounts {
	c := &stageCounts{}
	e.AddSystem(StageInput, func(*system.Context) { c.input++ })
	e.AddSystem(StageSimulate, func(*system.Context) { c.simulate++ })
	e.AddSystem(StageCamera, func(*system.Context) { c.camera++ })
	e.AddSystem(StagePost, func(*system.Context) { c.post++ })
	return c
}

// TestHoldSkipsSimulate: a held tick skips StageSimulate only; tweens (in the
// camera stage) keep running.
func TestHoldSkipsSimulate(t *testing.T) {
	for _, tool := range []bool{false, true} {
		cfg := Config{}
		if tool {
			cfg.Tool = &testTool{}
		}
		e := newCameraEngine(t, cfg)
		c := countStages(e)
		en := ecs.NewEntity1(e.World(), gfx.Transform{})
		tween.To(e.Tweens(), en, tween.Pos, gfx.Vec2{X: 100}, 1)
		release := e.Hold()
		if !e.Held() {
			t.Fatal("Held() = false after Hold")
		}
		update(t, e, 2)
		if *c != (stageCounts{input: 2, camera: 2, post: 2}) {
			t.Fatalf("tool=%v: held ticks ran %+v, want no simulate", tool, *c)
		}
		if x := ecs.GetComp[gfx.Transform](e.World(), en).Pos.X; x <= 0 {
			t.Fatalf("tool=%v: tween did not advance while held (x = %v)", tool, x)
		}
		release()
		update(t, e, 1)
		if c.simulate != 1 || e.Held() {
			t.Fatalf("tool=%v: after release simulate ran %d times, Held() = %v", tool, c.simulate, e.Held())
		}
	}
}

// TestHoldNests: every Hold needs its own release, and a second call of a
// release does not release another hold.
func TestHoldNests(t *testing.T) {
	e := newCameraEngine(t, Config{})
	c := countStages(e)
	a, b := e.Hold(), e.Hold()
	a()
	a()
	update(t, e, 1)
	if c.simulate != 0 || !e.Held() {
		t.Fatalf("one hold left: simulate ran %d times, Held() = %v", c.simulate, e.Held())
	}
	b()
	update(t, e, 1)
	if c.simulate != 1 {
		t.Fatalf("all released: simulate ran %d times, want 1", c.simulate)
	}
}
