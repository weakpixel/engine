package camera

import (
	"math"
	"testing"
	"time"

	"github.com/weakpixel/engine/gfx"
)

const tickDt = float32(1.0 / 60)

func TestShakeTrauma(t *testing.T) {
	r := &Rig{}
	r.AddTrauma(0.7)
	r.AddTrauma(0.7)
	if r.Trauma() != 1 {
		t.Errorf("trauma %v after 0.7+0.7, want 1", r.Trauma())
	}
	r.AddTrauma(-3)
	if r.Trauma() != 0 {
		t.Errorf("trauma %v after -3, want 0", r.Trauma())
	}
	r.AddTrauma(0.5)
	r.StopShake()
	if r.Trauma() != 0 {
		t.Errorf("trauma %v after StopShake, want 0", r.Trauma())
	}
}

func TestShakeStep(t *testing.T) {
	t.Run("decays to exactly zero", func(t *testing.T) {
		s := shakeState{trauma: 1}
		var off gfx.Vec2
		moved := false
		for range 61 { // decay 1/s: settled after 60 ticks
			off, _ = s.step(Shake{}, tickDt)
			moved = moved || off != (gfx.Vec2{})
		}
		if !moved {
			t.Error("full trauma never moved the camera")
		}
		if s.trauma != 0 || off != (gfx.Vec2{}) || s.time != 0 {
			t.Errorf("after 61 ticks: trauma %v, offset %v, time %v; want all zero", s.trauma, off, s.time)
		}
	})
	t.Run("exponent and max offset bound the amplitude", func(t *testing.T) {
		c := Shake{MaxOffset: vec(10, 4), Decay: 1e-9} // no noticeable decay
		s := shakeState{trauma: 0.5}
		for range 600 {
			off, _ := s.step(c, tickDt)
			if math.Abs(float64(off.X)) > 2.5 || math.Abs(float64(off.Y)) > 1 {
				t.Fatalf("offset %v at trauma 0.5, want within 0.25*MaxOffset", off)
			}
		}
	})
	t.Run("angle only with MaxAngle", func(t *testing.T) {
		s := shakeState{trauma: 1}
		for range 10 {
			if _, a := s.step(Shake{}, tickDt); a != 0 {
				t.Fatalf("angle %v with MaxAngle 0", a)
			}
		}
		s = shakeState{trauma: 1}
		turned := false
		for range 10 {
			_, a := s.step(Shake{MaxAngle: 0.1}, tickDt)
			if math.Abs(float64(a)) > 0.1 {
				t.Fatalf("angle %v beyond MaxAngle", a)
			}
			turned = turned || a != 0
		}
		if !turned {
			t.Error("MaxAngle 0.1 never rotated")
		}
	})
	t.Run("Off keeps trauma but does not move", func(t *testing.T) {
		s := shakeState{trauma: 1}
		off, a := s.step(Shake{Off: true}, tickDt)
		if off != (gfx.Vec2{}) || a != 0 {
			t.Errorf("Off: offset %v angle %v, want zero", off, a)
		}
		if s.trauma <= 0 || s.trauma >= 1 {
			t.Errorf("Off: trauma %v, want it decaying from 1", s.trauma)
		}
	})
	t.Run("zero dt changes nothing", func(t *testing.T) {
		s := shakeState{trauma: 0.5, time: 0.2, offset: vec(1, 2), angle: 0.3}
		off, a := s.step(Shake{}, 0)
		if off != vec(1, 2) || a != 0.3 || s.trauma != 0.5 || s.time != 0.2 {
			t.Errorf("dt 0 changed the state: %+v, returned %v %v", s, off, a)
		}
	})
}

func TestSystemShake(t *testing.T) {
	settle := func(s *scene) {
		for range 70 {
			s.tick()
		}
	}
	t.Run("ends on the follow position in every mode", func(t *testing.T) {
		for _, mode := range []FollowMode{FollowDefault, FollowSmooth, FollowDeadZone, FollowSpring, FollowLookAhead} {
			s := newScene(t, &Rig{Mode: mode})
			s.spawn(vec(100, 50), Target{})
			s.tick()
			s.rig.AddTrauma(1)
			s.tick()
			s.tick()
			if s.cam.Pos == vec(100, 50) {
				t.Errorf("%s: camera did not shake", mode)
			}
			if s.rig.state.follow != vec(100, 50) {
				t.Errorf("%s: shake moved the follow position to %v", mode, s.rig.state.follow)
			}
			settle(s)
			if s.cam.Pos != vec(100, 50) {
				t.Errorf("%s: camera %v after the shake, want (100,50)", mode, s.cam.Pos)
			}
		}
	})
	t.Run("no target: no drift", func(t *testing.T) {
		s := newScene(t, &Rig{Shake: Shake{MaxAngle: 0.1}})
		s.cam.Pos, s.cam.Rotation = vec(-30, 40), 0.5
		s.rig.AddTrauma(1)
		s.tick()
		s.tick()
		if s.cam.Pos == vec(-30, 40) {
			t.Error("camera without a target did not shake")
		}
		settle(s)
		if !near(s.cam.Pos, vec(-30, 40), 1e-4) || math.Abs(float64(s.cam.Rotation-0.5)) > 1e-6 {
			t.Errorf("camera %v rotation %v after the shake, want (-30,40) and 0.5", s.cam.Pos, s.cam.Rotation)
		}
	})
	t.Run("InBounds at an edge", func(t *testing.T) {
		// The view is 320x180 at zoom 1; bounds fit it exactly, so the
		// clamped camera sits at the bounds' center.
		bounds := gfx.Rect{Max: vec(320, 180)}
		for _, inBounds := range []bool{false, true} {
			s := newScene(t, &Rig{Bounds: bounds, Shake: Shake{InBounds: inBounds}})
			s.spawn(vec(0, 0), Target{})
			s.tick()
			s.rig.AddTrauma(1)
			left := false
			for range 30 {
				s.tick()
				left = left || s.cam.Pos != vec(160, 90)
			}
			if left == inBounds {
				t.Errorf("InBounds %v: view left the bounds %v, want %v", inBounds, left, !inBounds)
			}
		}
	})
	t.Run("snap keeps the shake and PrevPos", func(t *testing.T) {
		s := newScene(t, &Rig{})
		s.rig.AddTrauma(1)
		s.ctx.Time.Delta = 70 * time.Millisecond // noise time 1.4: off the lattice, where it is 0
		s.spawn(vec(10, 10), Target{})
		s.tick() // first tick with a target snaps
		if s.cam.PrevPos != s.cam.Pos || s.cam.Pos == vec(10, 10) {
			t.Errorf("snap: Pos %v PrevPos %v, want equal and shaken", s.cam.Pos, s.cam.PrevPos)
		}
		if s.rig.Trauma() == 0 {
			t.Error("the snap stopped the shake")
		}
	})
}
