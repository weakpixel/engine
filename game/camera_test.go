package game

import (
	"testing"

	"github.com/weakpixel/ecs"
	"github.com/weakpixel/ecs/system"
	"github.com/weakpixel/engine/camera"
	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/input"
	"github.com/weakpixel/engine/render"
)

func newCameraEngine(t *testing.T, cfg Config) *Engine {
	t.Helper()
	cfg.NoAudio = true
	cfg.Input.Source = &input.ManualSource{}
	e := New(cfg)
	t.Cleanup(e.Close)
	return e
}

func update(t *testing.T, e *Engine, n int) {
	t.Helper()
	for range n {
		if err := e.Update(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestCameraRigResource(t *testing.T) {
	e := newCameraEngine(t, Config{CameraRig: camera.Rig{Mode: camera.FollowSpring, TeleportDistance: 50}})
	rig := ecs.GetResource[camera.Rig](e.World())
	if rig != e.CameraRig() || rig.Mode != camera.FollowSpring || rig.TeleportDistance != 50 {
		t.Fatalf("Rig resource %+v (%p), CameraRig %p", rig, rig, e.CameraRig())
	}
}

func TestCameraNewPanicsOnUnknownMode(t *testing.T) {
	mustPanic(t, `camera: unknown follow mode "wobble"`, func() {
		New(Config{NoAudio: true, CameraRig: camera.Rig{Mode: "wobble"}})
	})
}

func TestCameraNoTargetUntouched(t *testing.T) {
	e := newCameraEngine(t, Config{Camera: render.Camera{Pos: gfx.Vec2{X: 7, Y: 9}}})
	ecs.NewEntity1(e.World(), gfx.Transform{Pos: gfx.Vec2{X: 100}})
	update(t, e, 5)
	if e.Camera().Pos != (gfx.Vec2{X: 7, Y: 9}) {
		t.Errorf("camera moved to %v without a target", e.Camera().Pos)
	}
}

// TestCameraFollowsInEachMode spawns a target, moves it, and expects the
// camera to start toward it on the next tick and to settle on it.
func TestCameraFollowsInEachMode(t *testing.T) {
	goal := gfx.Vec2{X: 200, Y: 100}
	tests := []struct {
		mode   camera.FollowMode
		settle gfx.Vec2 // where the camera ends up (dead zone: on the window edge)
	}{
		{camera.FollowDefault, goal},
		{camera.FollowSmooth, goal},
		{camera.FollowDeadZone, gfx.Vec2{X: 200 - camera.DefaultDeadZone.X, Y: 100 - camera.DefaultDeadZone.Y}},
		{camera.FollowSpring, goal},
		{camera.FollowLookAhead, goal},
	}
	for _, tt := range tests {
		t.Run(tt.mode.String(), func(t *testing.T) {
			e := newCameraEngine(t, Config{CameraRig: camera.Rig{Mode: tt.mode}})
			en := ecs.NewEntity2(e.World(), camera.Target{}, gfx.Transform{})
			update(t, e, 1)
			if e.Camera().Pos != (gfx.Vec2{}) {
				t.Fatalf("first tick: camera %v, want snapped to the target", e.Camera().Pos)
			}
			ecs.GetComp[gfx.Transform](e.World(), en).Pos = goal
			update(t, e, 1)
			if p := e.Camera().Pos; p.X <= 0 || p.Y <= 0 {
				t.Fatalf("camera %v did not start toward %v", p, goal)
			}
			update(t, e, 600)
			p := e.Camera().Pos
			if d := p.Sub(tt.settle); d.X*d.X+d.Y*d.Y > 0.01 {
				t.Errorf("camera settled at %v, want %v", p, tt.settle)
			}
		})
	}
}

func TestCameraUserSystemRunsAfter(t *testing.T) {
	e := newCameraEngine(t, Config{})
	ecs.NewEntity2(e.World(), camera.Target{}, gfx.Transform{Pos: gfx.Vec2{X: 40, Y: 30}})
	var seen gfx.Vec2
	e.AddSystem(StageCamera, func(*system.Context) {
		seen = e.Camera().Pos
		e.Camera().Pos = e.Camera().Pos.Add(gfx.Vec2{X: 1}) // shake
	})
	update(t, e, 3)
	if seen != (gfx.Vec2{X: 40, Y: 30}) {
		t.Errorf("user system saw %v, want the followed position", seen)
	}
	if e.Camera().Pos != (gfx.Vec2{X: 41, Y: 30}) {
		t.Errorf("camera %v, want the follow plus the shake, not accumulated", e.Camera().Pos)
	}
}

func TestCameraPausedByTool(t *testing.T) {
	tool := &testTool{script: []TickMode{TickRun, TickPause, TickPause, TickRun}}
	e := newCameraEngine(t, Config{Tool: tool})
	en := ecs.NewEntity2(e.World(), camera.Target{}, gfx.Transform{})
	update(t, e, 1)
	ecs.GetComp[gfx.Transform](e.World(), en).Pos = gfx.Vec2{X: 50}
	update(t, e, 2)
	if e.Camera().Pos != (gfx.Vec2{}) {
		t.Errorf("camera moved to %v while paused", e.Camera().Pos)
	}
	update(t, e, 1)
	if e.Camera().Pos != (gfx.Vec2{X: 50}) {
		t.Errorf("camera %v after resuming, want (50,0)", e.Camera().Pos)
	}
}
