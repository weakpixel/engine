package game

import (
	"testing"

	"github.com/weakpixel/ecs"
	"github.com/weakpixel/ecs/system"
	"github.com/weakpixel/engine/camera"
	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/tween"
)

func TestTweensResource(t *testing.T) {
	e := newCameraEngine(t, Config{})
	if tw := ecs.GetResource[tween.Tweens](e.World()); tw != e.Tweens() || tw == nil {
		t.Fatalf("Tweens resource %p, Engine.Tweens %p", tw, e.Tweens())
	}
}

// TestTweenBeforeFollowCamera: a tween a game system starts runs in the same
// tick, before the follow camera reads its target.
func TestTweenBeforeFollowCamera(t *testing.T) {
	e := newCameraEngine(t, Config{})
	en := ecs.NewEntity2(e.World(), camera.Target{}, gfx.Transform{})
	started := false
	e.AddSystem(StageSimulate, func(ctx *system.Context) {
		if !started {
			tween.To(ecs.GetResource[tween.Tweens](ctx.World), en, tween.Pos, gfx.Vec2{X: 60, Y: 20}, 0)
			started = true
		}
	})
	update(t, e, 1)
	if p := e.Camera().Pos; p != (gfx.Vec2{X: 60, Y: 20}) {
		t.Errorf("camera %v after the first tick, want the tweened target (60, 20)", p)
	}
}

func TestTweensPausedByTool(t *testing.T) {
	tool := &testTool{script: []TickMode{TickRun, TickPause, TickPause, TickPause, TickRun}}
	e := newCameraEngine(t, Config{Tool: tool})
	en := ecs.NewEntity1(e.World(), gfx.Transform{})
	tween.To(e.Tweens(), en, tween.Pos, gfx.Vec2{X: 100}, 1)
	update(t, e, 1)
	x := ecs.GetComp[gfx.Transform](e.World(), en).Pos.X
	if x <= 0 {
		t.Fatalf("x = %v after a running tick", x)
	}
	update(t, e, 3)
	if got := ecs.GetComp[gfx.Transform](e.World(), en).Pos.X; got != x {
		t.Fatalf("x moved from %v to %v under TickPause", x, got)
	}
	update(t, e, 1)
	if got := ecs.GetComp[gfx.Transform](e.World(), en).Pos.X; got <= x {
		t.Errorf("x = %v after resuming, want more than %v", got, x)
	}
}

func TestTickNoAllocsWithTweens(t *testing.T) {
	if testing.CoverMode() != "" || raceEnabled {
		t.Skip("instrumentation allocates")
	}
	e := newCameraEngine(t, Config{Width: 64, Height: 32})
	tw := e.Tweens()
	for i := range 20 {
		en := ecs.NewEntity2(e.World(), gfx.Transform{Pos: gfx.Vec2{X: float32(i)}}, gfx.Sprite{})
		tween.To(tw, en, tween.Pos, gfx.Vec2{X: 50, Y: 20}, 0.5).Repeat(-1).Yoyo().Ease(tween.OutBack)
		tween.To(tw, en, tween.Tint, gfx.RGBA(255, 255, 255, 0), 0.3).Repeat(-1)
		tween.To(tw, en, tween.Param(0), 1, 0.2).Repeat(-1).Yoyo()
	}
	update(t, e, 10)
	if n := testing.AllocsPerRun(20, func() { e.Update() }); n != 0 {
		t.Fatalf("allocs per tick = %v, want 0", n)
	}
}
