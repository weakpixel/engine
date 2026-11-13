package camera

import (
	"testing"
	"time"

	"github.com/weakpixel/engine/gfx"
)

const tick = float32(1.0 / 60)

// run steps a fresh Rig (snapped to the origin) through the goals, the
// target moving at vel, and returns the follow positions.
func run(r *Rig, vel gfx.Vec2, goals ...gfx.Vec2) []gfx.Vec2 {
	r.reset(gfx.Vec2{})
	out := make([]gfx.Vec2, len(goals))
	for i, g := range goals {
		out[i] = r.step(g, vel, tick)
	}
	return out
}

func TestPipelinePresets(t *testing.T) {
	goal := vec(100, 0)
	tests := []struct {
		name string
		rig  Rig
		want gfx.Vec2 // after one tick toward goal from the origin
	}{
		{"zero is default", Rig{}, goal},
		{"default", Rig{Mode: FollowDefault}, goal},
		{"smooth", Rig{Mode: FollowSmooth}, smoothStep(gfx.Vec2{}, goal, tick, 0.15)},
		{"smooth custom time", Rig{Mode: FollowSmooth, Smooth: Smooth{Time: time.Second}},
			smoothStep(gfx.Vec2{}, goal, tick, 1)},
		{"deadzone", Rig{Mode: FollowDeadZone}, vec(68, 0)},
		{"deadzone custom", Rig{Mode: FollowDeadZone, DeadZone: DeadZone{HalfSize: vec(10, 10)}}, vec(90, 0)},
		{"spring", Rig{Mode: FollowSpring}, func() gfx.Vec2 {
			p, _ := springStep(gfx.Vec2{}, gfx.Vec2{}, goal, tick, 2, 1)
			return p
		}()},
		// Standing target: the look offset stays zero, so it is plain smooth.
		{"lookahead standing", Rig{Mode: FollowLookAhead}, smoothStep(gfx.Vec2{}, goal, tick, 0.15)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := tt.rig
			if got := run(&r, gfx.Vec2{}, goal)[0]; !near(got, tt.want, 1e-4) {
				t.Errorf("one tick: %v, want %v", got, tt.want)
			}
		})
	}
}

// TestPipelineLookAheadLeads checks that a moving target puts the camera
// ahead of it once settled.
func TestPipelineLookAheadLeads(t *testing.T) {
	r := Rig{Mode: FollowLookAhead}
	r.reset(gfx.Vec2{})
	vel := vec(60, 0)
	var goal, follow gfx.Vec2
	for range 600 {
		goal = goal.Add(vel.Mul(tick))
		follow = r.step(goal, vel, tick)
	}
	// Smooth lag behind a moving goal is vel*tau = 9; the look-ahead is 48.
	if lead := follow.X - goal.X; lead < 30 || lead > 48 {
		t.Errorf("camera leads the target by %v, want 48 minus the smoothing lag", lead)
	}
}

func TestPipelineComposition(t *testing.T) {
	t.Run("smooth + dead zone", func(t *testing.T) {
		r := Rig{Mode: FollowSmooth, DeadZone: DeadZone{On: true}}
		// Inside the window: nothing moves.
		if got := run(&r, gfx.Vec2{}, vec(20, 10))[0]; got != (gfx.Vec2{}) {
			t.Errorf("goal inside the window moved the camera to %v", got)
		}
		// Past the edge: the window moves to 68 and the camera eases toward it.
		got := run(&r, gfx.Vec2{}, vec(100, 0))[0]
		if want := smoothStep(gfx.Vec2{}, vec(68, 0), tick, 0.15); !near(got, want, 1e-4) {
			t.Errorf("one tick: %v, want easing toward the window edge %v", got, want)
		}
		if r.state.window != vec(68, 0) {
			t.Errorf("window %v, want (68,0)", r.state.window)
		}
	})
	t.Run("spring + look-ahead", func(t *testing.T) {
		r := Rig{Mode: FollowSpring, LookAhead: LookAhead{On: true}}
		r.reset(gfx.Vec2{})
		for range 600 {
			r.step(gfx.Vec2{}, vec(100, 0), tick)
		}
		if !near(r.state.follow, vec(48, 0), 0.01) {
			t.Errorf("spring settled at %v, want the look-ahead point (48,0)", r.state.follow)
		}
	})
	t.Run("dead zone + look-ahead", func(t *testing.T) {
		r := Rig{Mode: FollowDeadZone, LookAhead: LookAhead{On: true}}
		r.reset(gfx.Vec2{})
		for range 600 {
			r.step(gfx.Vec2{}, vec(0, 100), tick)
		}
		// The look goal is (0,48); the window's bottom edge is 24 below center.
		if !near(r.state.follow, vec(0, 24), 0.01) {
			t.Errorf("settled at %v, want the window edge on the look point (0,24)", r.state.follow)
		}
	})
	t.Run("stage off stays neutral", func(t *testing.T) {
		r := Rig{Mode: FollowSmooth}
		run(&r, vec(100, 0), vec(500, 0))
		if r.state.look != (gfx.Vec2{}) || r.state.window != vec(500, 0) {
			t.Errorf("look %v window %v, want zero and on the goal", r.state.look, r.state.window)
		}
	})
}

func TestPipelineZeroDt(t *testing.T) {
	r := Rig{Mode: FollowSmooth}
	r.reset(vec(1, 1))
	if got := r.step(vec(100, 100), vec(9, 9), 0); got != vec(1, 1) {
		t.Errorf("dt 0 moved the camera to %v", got)
	}
}

func TestPipelineUnknownMode(t *testing.T) {
	r := Rig{Mode: "zoomy"}
	mustPanic(t, `camera: unknown follow mode "zoomy"`, func() { r.step(gfx.Vec2{}, gfx.Vec2{}, tick) })
	mustPanic(t, `camera: unknown follow mode "zoomy"`, func() { r.Mode.Check() })
	for _, m := range []FollowMode{"", FollowDefault, FollowSmooth, FollowDeadZone, FollowSpring, FollowLookAhead} {
		m.Check()
	}
	if FollowMode("").String() != "default" || FollowSpring.String() != "spring" {
		t.Error("String")
	}
}
