package tween

import (
	"testing"

	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/xmath"
)

func TestEaseEndpoints(t *testing.T) {
	for e := range numEases {
		t.Run(e.String(), func(t *testing.T) {
			if got := e.At(0); got != 0 {
				t.Errorf("At(0) = %v, want 0", got)
			}
			if got := e.At(1); got != 1 {
				t.Errorf("At(1) = %v, want 1", got)
			}
			// The raw curve, without the clamping of At, also hits both
			// ends (within float error), so At has no jump near them.
			f := easeFuncs[e]
			if e != Step && (xmath.Abs(f(0)) > 1e-3 || xmath.Abs(f(1)-1) > 1e-3) {
				t.Errorf("curve(0), curve(1) = %v, %v", f(0), f(1))
			}
		})
	}
}

func TestEaseMonotonic(t *testing.T) {
	for _, e := range []Ease{Linear, InQuad, OutQuad, InOutQuad, InCubic, OutCubic, InOutCubic,
		InSine, OutSine, InOutSine, InExpo, OutExpo, InOutExpo, InBounce, OutBounce, Step} {
		if e == InBounce || e == OutBounce {
			continue // bounces go back down between hops
		}
		prev := e.At(0)
		for i := 1; i <= 100; i++ {
			v := e.At(float32(i) / 100)
			if v < prev {
				t.Errorf("%v not monotonic at %v: %v < %v", e, float32(i)/100, v, prev)
				break
			}
			prev = v
		}
	}
}

func TestEaseShapes(t *testing.T) {
	tests := []struct {
		e    Ease
		t    float32
		want float32
	}{
		{Linear, 0.25, 0.25},
		{InQuad, 0.5, 0.25},
		{OutQuad, 0.5, 0.75},
		{InOutQuad, 0.25, 0.125},
		{InOutQuad, 0.75, 0.875},
		{InCubic, 0.5, 0.125},
		{Step, 0.999, 0},
	}
	for _, tc := range tests {
		if got := tc.e.At(tc.t); xmath.Abs(got-tc.want) > 1e-6 {
			t.Errorf("%v.At(%v) = %v, want %v", tc.e, tc.t, got, tc.want)
		}
	}
	if InBack.At(0.2) >= 0 {
		t.Error("InBack should undershoot below 0")
	}
	if OutBack.At(0.8) <= 1 {
		t.Error("OutBack should overshoot above 1")
	}
	if Ease(200).At(0.3) != 0.3 || Ease(200).String() != "invalid" {
		t.Error("an invalid Ease should be linear and print as invalid")
	}
}

func TestLerp(t *testing.T) {
	if got := LerpFloat(0.1, 0.7, 1); got != 0.7 {
		t.Errorf("LerpFloat end = %v, want exactly 0.7", got)
	}
	if got := LerpFloat(0.1, 0.7, 0); got != 0.1 {
		t.Errorf("LerpFloat start = %v, want exactly 0.1", got)
	}
	if got := LerpFloat(0, 10, 1.5); got != 15 {
		t.Errorf("LerpFloat extrapolates to %v, want 15", got)
	}
	if got := LerpVec2(gfx.Vec2{X: 1, Y: 2}, gfx.Vec2{X: 3, Y: 6}, 0.5); got != (gfx.Vec2{X: 2, Y: 4}) {
		t.Errorf("LerpVec2 = %v", got)
	}
	if got := LerpFloat4([4]float32{0, 1, 2, 3}, [4]float32{4, 5, 6, 7}, 0.25); got != [4]float32{1, 2, 3, 4} {
		t.Errorf("LerpFloat4 = %v", got)
	}
}

func TestLerpColor(t *testing.T) {
	red, blue := gfx.RGBA(255, 0, 0, 255), gfx.RGBA(0, 0, 255, 128)
	tests := []struct {
		name string
		a, b gfx.Color
		t    float32
		want gfx.Color
	}{
		{"start exact", red, blue, 0, red},
		{"end exact", red, blue, 1, blue},
		{"per channel", red, blue, 0.5, gfx.RGBA(128, 0, 128, 192)},
		{"zero is white", 0, gfx.RGBA(0, 0, 0, 255), 0.5, gfx.RGBA(128, 128, 128, 255)},
		{"overshoot clamps", gfx.RGBA(0, 0, 0, 255), gfx.RGBA(200, 0, 0, 255), 2, gfx.RGBA(255, 0, 0, 255)},
		{"transparent black is not zero", gfx.RGBA(0, 0, 2, 10), gfx.RGBA(0, 0, 4, 20), -1, gfx.RGBA(0, 0, 1, 0)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := LerpColor(tc.a, tc.b, tc.t); got != tc.want {
				t.Errorf("LerpColor = %08x, want %08x", uint32(got), uint32(tc.want))
			}
		})
	}
}
