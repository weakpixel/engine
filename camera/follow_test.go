package camera

import (
	"testing"

	"github.com/weakpixel/engine/gfx"
)

func TestDefault(t *testing.T) {
	r := &Rig{}
	r.reset(vec(0, 0))
	for _, goal := range []gfx.Vec2{vec(10, -5), vec(-300, 42), vec(0, 0)} {
		if got := r.step(goal, gfx.Vec2{}, 1.0/60); got != goal {
			t.Errorf("step(%v) = %v, want the goal", goal, got)
		}
	}
}

func TestSmooth(t *testing.T) {
	const tau = 0.15
	tests := []struct {
		name     string
		pos, gl  gfx.Vec2
		dt       float32
		want     gfx.Vec2
		exactish float32
	}{
		{"at goal stays", vec(5, 5), vec(5, 5), 1.0 / 60, vec(5, 5), 0},
		{"zero dt no move", vec(0, 0), vec(100, 0), 0, vec(0, 0), 0},
		{"negative dt no move", vec(0, 0), vec(100, 0), -1, vec(0, 0), 0},
		// After one time constant 1 - 1/e ≈ 63.2% of the distance is covered.
		{"one time constant", vec(0, 0), vec(100, -100), tau, vec(63.212, -63.212), 0.01},
		{"long dt arrives", vec(0, 0), vec(100, 0), 100, vec(100, 0), 0.001},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := smoothStep(tt.pos, tt.gl, tt.dt, tau); !near(got, tt.want, tt.exactish) {
				t.Errorf("smoothStep = %v, want %v", got, tt.want)
			}
		})
	}
}

// TestSmoothTickRateIndependent eases toward a static goal for 0.5 s at 30,
// 60 and 120 ticks per second; the results must agree.
func TestSmoothTickRateIndependent(t *testing.T) {
	goal := vec(200, -80)
	var results []gfx.Vec2
	for _, tps := range []int{30, 60, 120} {
		pos := gfx.Vec2{}
		for range tps / 2 {
			pos = smoothStep(pos, goal, 1/float32(tps), 0.15)
		}
		results = append(results, pos)
	}
	for i, r := range results[1:] {
		if !near(r, results[0], 0.01) {
			t.Errorf("tps result %d = %v, want %v (30 TPS)", i+1, r, results[0])
		}
	}
}

func TestDeadZone(t *testing.T) {
	half := vec(32, 24)
	window := vec(100, 100)
	tests := []struct {
		name string
		goal gfx.Vec2
		want gfx.Vec2
	}{
		{"center", vec(100, 100), vec(100, 100)},
		{"inside", vec(120, 80), vec(100, 100)},
		{"on edge", vec(132, 124), vec(100, 100)},
		{"past right", vec(140, 100), vec(108, 100)},
		{"past left", vec(50, 100), vec(82, 100)},
		{"past bottom", vec(100, 130), vec(100, 106)},
		{"past top", vec(100, 60), vec(100, 84)},
		{"corner", vec(200, 200), vec(168, 176)},
		{"jump", vec(-1000, 5000), vec(-968, 4976)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := deadZoneStep(window, tt.goal, half); got != tt.want {
				t.Errorf("deadZoneStep(%v) = %v, want %v", tt.goal, got, tt.want)
			}
		})
	}
}
