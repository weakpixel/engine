package camera

import (
	"testing"

	"github.com/weakpixel/engine/gfx"
)

func TestLookAhead(t *testing.T) {
	const dist, tau, minSpeed = 48, 0.3, 1
	settle := func(look, vel gfx.Vec2) gfx.Vec2 {
		for range 600 {
			look = lookStep(look, vel, 1.0/60, dist, tau, minSpeed)
		}
		return look
	}
	tests := []struct {
		name string
		from gfx.Vec2
		vel  gfx.Vec2
		want gfx.Vec2
	}{
		{"moving right", gfx.Vec2{}, vec(120, 0), vec(48, 0)},
		{"moving diagonally", gfx.Vec2{}, vec(-30, 40), vec(-28.8, 38.4)},
		{"speed is irrelevant", gfx.Vec2{}, vec(0, 5000), vec(0, 48)},
		{"reverses", vec(48, 0), vec(-60, 0), vec(-48, 0)},
		{"below min speed returns", vec(48, 0), vec(0.5, 0), gfx.Vec2{}},
		{"standing returns", vec(0, -48), gfx.Vec2{}, gfx.Vec2{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := settle(tt.from, tt.vel); !near(got, tt.want, 0.01) {
				t.Errorf("settled at %v, want %v", got, tt.want)
			}
		})
	}
	t.Run("eases", func(t *testing.T) {
		got := lookStep(gfx.Vec2{}, vec(100, 0), tau, dist, tau, minSpeed)
		if !near(got, vec(48*0.6321, 0), 0.01) {
			t.Errorf("after one time constant %v, want 63%% of the distance", got)
		}
	})
	t.Run("zero dt", func(t *testing.T) {
		if got := lookStep(vec(3, 4), vec(100, 0), 0, dist, tau, minSpeed); got != vec(3, 4) {
			t.Errorf("dt 0 moved the look offset to %v", got)
		}
	})
	t.Run("velocity", func(t *testing.T) {
		if got := velocity(vec(10, 10), vec(12, 9), 0.5); got != vec(4, -2) {
			t.Errorf("velocity = %v, want (4,-2)", got)
		}
	})
}
