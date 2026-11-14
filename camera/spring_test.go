package camera

import (
	"testing"

	"github.com/weakpixel/engine/gfx"
)

// simulateSpring runs a spring from pos 0, velocity 0 toward goal for
// seconds at tps and returns the final position and the largest X reached.
func simulateSpring(goal gfx.Vec2, seconds float32, tps int, freq, zeta float32) (gfx.Vec2, float32) {
	var pos, vel gfx.Vec2
	peak := float32(0)
	dt := 1 / float32(tps)
	for range int(seconds * float32(tps)) {
		pos, vel = springStep(pos, vel, goal, dt, freq, zeta)
		peak = max(peak, pos.X)
	}
	return pos, peak
}

func TestSpring(t *testing.T) {
	goal := vec(100, -50)
	tests := []struct {
		name      string
		zeta      float32
		overshoot bool
	}{
		{"critical", 1, false},
		{"under-damped", 0.3, true},
		{"over-damped", 2.5, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pos, peak := simulateSpring(goal, 10, 60, 2, tt.zeta)
			if !near(pos, goal, 0.01) {
				t.Errorf("after 10 s at %v, want the goal %v", pos, goal)
			}
			if over := peak > goal.X+0.01; over != tt.overshoot {
				t.Errorf("peak %v (goal %v): overshoot = %v, want %v", peak, goal.X, over, tt.overshoot)
			}
		})
	}
}

func TestSpringZeroDt(t *testing.T) {
	pos, vel := springStep(vec(1, 2), vec(3, 4), vec(9, 9), 0, 2, 1)
	if pos != vec(1, 2) || vel != vec(3, 4) {
		t.Errorf("dt 0 changed the spring: %v %v", pos, vel)
	}
}

// TestSpringLargeDtStable takes huge steps: an exact solution never blows up.
func TestSpringLargeDtStable(t *testing.T) {
	for _, zeta := range []float32{0.2, 1, 3} {
		var pos, vel gfx.Vec2
		for range 20 {
			pos, vel = springStep(pos, vel, vec(100, 100), 5, 2, zeta)
		}
		if !near(pos, vec(100, 100), 0.01) {
			t.Errorf("zeta %v: %v after big steps, want the goal", zeta, pos)
		}
	}
}

// TestSpringTickRateIndependent compares the step response at 30, 60 and
// 120 ticks per second part way through (where it is still moving).
func TestSpringTickRateIndependent(t *testing.T) {
	for _, zeta := range []float32{0.4, 1, 2} {
		ref, _ := simulateSpring(vec(100, 0), 0.4, 30, 2, zeta)
		for _, tps := range []int{60, 120} {
			got, _ := simulateSpring(vec(100, 0), 0.4, tps, 2, zeta)
			if !near(got, ref, 0.01) {
				t.Errorf("zeta %v: %d TPS gives %v, 30 TPS %v", zeta, tps, got, ref)
			}
		}
	}
}
