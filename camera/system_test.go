package camera

import (
	"image"
	"testing"
	"time"

	"github.com/weakpixel/ecs"
	"github.com/weakpixel/ecs/system"
	"github.com/weakpixel/engine/extract"
	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/render"
)

// scene is a World with the camera resources and an initialized System.
// tick does what game.Engine does around it: snapshot the camera and every
// Transform, then run the system.
type scene struct {
	w   *ecs.World
	cam *render.Camera
	rig *Rig
	sys *System
	ctx *system.Context
}

func newScene(t *testing.T, rig *Rig) *scene {
	t.Helper()
	w := ecs.NewWorld()
	s := &scene{w: w, cam: &render.Camera{}, rig: rig}
	ecs.SetResource(w, s.cam)
	ecs.SetResource(w, &extract.Screen{Size: image.Pt(320, 180)})
	if rig != nil {
		ecs.SetResource(w, rig)
	}
	s.sys = NewSystem(w)
	s.ctx = &system.Context{World: w, Time: system.Time{Delta: time.Second / 60}}
	s.sys.Init(s.ctx)
	s.rig = s.sys.rig
	return s
}

// spawn creates a target at pos.
func (s *scene) spawn(pos gfx.Vec2, t Target) ecs.Entity {
	return ecs.NewEntity2(s.w, t, gfx.Transform{Pos: pos, PrevPos: pos})
}

// move sets e's position for the next tick (Pos only; tick snapshots).
func (s *scene) move(e ecs.Entity, pos gfx.Vec2) { ecs.GetComp[gfx.Transform](s.w, e).Pos = pos }

func (s *scene) tick() {
	s.cam.PrevPos = s.cam.Pos
	s.sys.Run(s.ctx)
	for _, c := range ecs.NewView1[gfx.Transform](s.w).Each() {
		c.PrevPos = c.Pos
	}
}

func TestSystemNoTargetLeavesCamera(t *testing.T) {
	s := newScene(t, &Rig{Mode: FollowSmooth, Bounds: gfx.Rect{Max: vec(10, 10)}})
	s.cam.Pos = vec(-500, 777)
	ecs.NewEntity1(s.w, gfx.Transform{Pos: vec(1, 1)}) // not a target
	for range 3 {
		s.tick()
	}
	if s.cam.Pos != vec(-500, 777) {
		t.Errorf("camera moved to %v without a target", s.cam.Pos)
	}
}

func TestSystemCreatesRig(t *testing.T) {
	s := newScene(t, nil)
	if got := ecs.GetResourceOrNil[Rig](s.w); got == nil || got != s.rig {
		t.Fatalf("Rig resource %p, system uses %p", got, s.rig)
	}
	s.spawn(vec(4, 5), Target{})
	s.tick()
	if s.cam.Pos != vec(4, 5) {
		t.Errorf("zero Rig: camera %v, want hard follow (4,5)", s.cam.Pos)
	}
}

func TestSystemPicksTarget(t *testing.T) {
	tests := []struct {
		name  string
		specs []Target
		want  gfx.Vec2 // targets sit at (10*i, 0)
	}{
		{"single", []Target{{}}, vec(0, 0)},
		{"priority wins", []Target{{Priority: 1}, {Priority: 5}, {Priority: -2}}, vec(10, 0)},
		{"tie goes to lowest ID", []Target{{Priority: 3}, {Priority: 3}}, vec(0, 0)},
		{"offset", []Target{{Offset: vec(0, -16)}}, vec(0, -16)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newScene(t, &Rig{})
			for i, sp := range tt.specs {
				s.spawn(vec(float32(10*i), 0), sp)
			}
			s.tick()
			if s.cam.Pos != tt.want {
				t.Errorf("camera %v, want %v", s.cam.Pos, tt.want)
			}
		})
	}
	t.Run("tie-break survives archetype moves", func(t *testing.T) {
		type marker struct{ _ int }
		s := newScene(t, &Rig{})
		a := s.spawn(vec(1, 0), Target{})
		s.spawn(vec(2, 0), Target{})
		// a moves to a later archetype, so view order puts it last.
		ecs.SetComp(s.w, a, marker{})
		s.tick()
		if s.cam.Pos != vec(1, 0) {
			t.Errorf("camera %v, want the lowest ID (1,0)", s.cam.Pos)
		}
	})
}

func TestSystemSnap(t *testing.T) {
	t.Run("first target snaps and sets PrevPos", func(t *testing.T) {
		s := newScene(t, &Rig{Mode: FollowSmooth})
		s.tick()
		s.spawn(vec(300, 200), Target{})
		s.tick()
		if s.cam.Pos != vec(300, 200) || s.cam.PrevPos != s.cam.Pos {
			t.Errorf("Pos %v PrevPos %v, want both on the target", s.cam.Pos, s.cam.PrevPos)
		}
	})
	t.Run("target returns after an absence", func(t *testing.T) {
		s := newScene(t, &Rig{Mode: FollowSmooth})
		e := s.spawn(vec(0, 0), Target{})
		s.tick()
		s.w.DeleteEntity(e)
		s.tick()
		s.spawn(vec(900, 0), Target{})
		s.tick()
		if s.cam.Pos != vec(900, 0) || s.cam.PrevPos != s.cam.Pos {
			t.Errorf("Pos %v PrevPos %v, want a snap to the new target", s.cam.Pos, s.cam.PrevPos)
		}
	})
	t.Run("target switch does not snap", func(t *testing.T) {
		s := newScene(t, &Rig{Mode: FollowSmooth})
		s.spawn(vec(0, 0), Target{})
		s.tick()
		s.spawn(vec(500, 0), Target{Priority: 1})
		s.tick()
		if x := s.cam.Pos.X; x <= 0 || x >= 500 {
			t.Errorf("camera X %v, want easing between the targets", x)
		}
	})
	t.Run("Snap resets the state", func(t *testing.T) {
		s := newScene(t, &Rig{Mode: FollowSpring, LookAhead: LookAhead{On: true}, DeadZone: DeadZone{On: true}})
		e := s.spawn(vec(0, 0), Target{})
		s.tick()
		for i := range 20 {
			s.move(e, vec(float32(i*10), 0))
			s.tick()
		}
		if s.rig.state.vel == (gfx.Vec2{}) || s.rig.state.look == (gfx.Vec2{}) {
			t.Fatal("test setup: expected spring velocity and look offset")
		}
		s.move(e, vec(5000, 5000))
		s.rig.Snap()
		s.tick()
		st := s.rig.state
		if s.cam.Pos != vec(5000, 5000) || s.cam.PrevPos != s.cam.Pos ||
			st.vel != (gfx.Vec2{}) || st.look != (gfx.Vec2{}) || st.window != vec(5000, 5000) || st.snap {
			t.Errorf("after Snap: cam %v/%v state %+v", s.cam.Pos, s.cam.PrevPos, st)
		}
	})
	t.Run("TeleportDistance", func(t *testing.T) {
		s := newScene(t, &Rig{Mode: FollowSmooth, TeleportDistance: 100})
		e := s.spawn(vec(0, 0), Target{})
		s.tick()
		s.move(e, vec(90, 0))
		s.tick()
		if s.cam.Pos.X >= 90 {
			t.Errorf("a short move snapped: %v", s.cam.Pos)
		}
		s.move(e, vec(1000, 0))
		s.tick()
		if s.cam.Pos != vec(1000, 0) || s.cam.PrevPos != s.cam.Pos {
			t.Errorf("a long move did not snap: %v / %v", s.cam.Pos, s.cam.PrevPos)
		}
	})
}

func TestSystemBounds(t *testing.T) {
	bounds := gfx.Rect{Max: vec(1000, 1000)} // view is 320x180
	t.Run("clamped", func(t *testing.T) {
		s := newScene(t, &Rig{Bounds: bounds})
		s.spawn(vec(0, 2000), Target{})
		s.tick()
		if s.cam.Pos != vec(160, 910) {
			t.Errorf("camera %v, want (160,910)", s.cam.Pos)
		}
	})
	t.Run("clamp does not feed back", func(t *testing.T) {
		s := newScene(t, &Rig{Mode: FollowSmooth, Bounds: bounds})
		e := s.spawn(vec(500, 500), Target{})
		s.tick()
		s.move(e, vec(-400, 500))
		for range 120 {
			s.tick()
		}
		if s.cam.Pos.X != 160 || !near(s.rig.state.follow, vec(-400, 500), 0.1) {
			t.Fatalf("camera %v follow %v, want clamped camera, unclamped follow", s.cam.Pos, s.rig.state.follow)
		}
		// Back inside: the camera leaves the edge on the very next tick,
		// easing from the unclamped follow position.
		s.move(e, vec(500, 500))
		s.tick()
		want := clampToBounds(smoothStep(vec(-400, 500), vec(500, 500), 1.0/60, 0.15), vec(160, 90), bounds)
		if !near(s.cam.Pos, want, 0.1) {
			t.Errorf("camera %v, want %v", s.cam.Pos, want)
		}
	})
}

// TestSystemLaterOffsetDoesNotAccumulate adds a shake after the system each
// tick; the follow must not drift.
func TestSystemLaterOffsetDoesNotAccumulate(t *testing.T) {
	s := newScene(t, &Rig{Mode: FollowSmooth})
	s.spawn(vec(100, 100), Target{})
	for range 60 {
		s.tick()
		s.cam.Pos = s.cam.Pos.Add(vec(5, 0)) // shake
	}
	if !near(s.rig.state.follow, vec(100, 100), 1e-3) {
		t.Errorf("follow drifted to %v", s.rig.state.follow)
	}
}

func TestSystemVelocityDrivesLookAhead(t *testing.T) {
	s := newScene(t, &Rig{Mode: FollowLookAhead})
	e := s.spawn(vec(0, 0), Target{})
	s.tick()
	for i := range 300 {
		s.move(e, vec(float32(i+1)*2, 0)) // 120 units/s
		s.tick()
	}
	if s.rig.state.look.X < 47 {
		t.Errorf("look offset %v, want about 48 to the right", s.rig.state.look)
	}
}

func TestSystemUnknownMode(t *testing.T) {
	s := newScene(t, &Rig{Mode: "orbit"})
	s.spawn(vec(0, 0), Target{})
	mustPanic(t, `camera: unknown follow mode "orbit"`, s.tick)
}

func TestSystemRunDoesNotAllocate(t *testing.T) {
	if testing.CoverMode() != "" || raceEnabled {
		t.Skip("instrumentation allocates")
	}
	for _, mode := range []FollowMode{FollowDefault, FollowSmooth, FollowDeadZone, FollowSpring, FollowLookAhead} {
		s := newScene(t, &Rig{Mode: mode, Bounds: gfx.Rect{Max: vec(2000, 2000)}})
		e := s.spawn(vec(0, 0), Target{})
		s.spawn(vec(50, 0), Target{Priority: -1})
		s.rig.Shake.MaxAngle = 0.05
		s.sys.Run(s.ctx) // warm up: snap
		x := float32(0)
		if n := testing.AllocsPerRun(100, func() {
			x += 3
			s.move(e, vec(x, x/2))
			s.rig.AddTrauma(0.1) // keep it shaking
			s.sys.Run(s.ctx)
		}); n != 0 {
			t.Errorf("%s: Run allocates %v per tick", mode, n)
		}
	}
}
