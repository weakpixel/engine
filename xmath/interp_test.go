package xmath

import "testing"

func TestInterp(t *testing.T) {
	tests := []struct {
		name string
		got  float32
		want float32
	}{
		{"Lerp mid", Lerp(10, 20, 0.5), 15},
		{"Lerp extrapolates", Lerp(0, 10, 2), 20},
		{"Lerp end exact", Lerp(0.1, 0.7, 1), 0.7},
		{"Lerp start exact", Lerp(0.1, 0.7, 0), 0.1},
		{"InvLerp", InvLerp(10, 20, 15), 0.5},
		{"InvLerp a==b", InvLerp(5, 5, 9), 0},
		{"Remap", Remap(5, 0, 10, 100, 200), 150},
		{"Remap a0==a1", Remap(5, 3, 3, 100, 200), 100},
		{"RemapClamped", RemapClamped(50, 0, 10, 100, 200), 200},
		{"Clamp01 low", Clamp01(-1), 0},
		{"Clamp01 high", Clamp01(2), 1},
		{"ClampAbs", ClampAbs(-9, 5), -5},
		{"ClampAbs no limit", ClampAbs(-9, 0), -9},
		{"SmoothStep mid", SmoothStep(0, 10, 5), 0.5},
		{"SmoothStep below", SmoothStep(0, 10, -1), 0},
		{"SmoothStep equal edges", SmoothStep(3, 3, 4), 1},
		{"SmootherStep mid", SmootherStep(0, 10, 5), 0.5},
		{"Step below", Step(5, 4), 0},
		{"Step at", Step(5, 5), 1},
		{"PingPong up", PingPong(3, 10), 3},
		{"PingPong down", PingPong(13, 10), 7},
		{"PingPong wraps", PingPong(23, 10), 3},
		{"Snap", Snap(13, 8), 16},
		{"Snap zero step", Snap(13, 0), 13},
		{"SnapFloor", SnapFloor(15.9, 8), 8},
		{"SnapFloor negative", SnapFloor(-1, 8), -8},
		{"Fract negative", Fract(-0.25), 0.75},
		{"Repeat negative", Repeat(-1, 4), 3},
		{"Repeat zero length", Repeat(3, 0), 0},
	}
	for _, tt := range tests {
		checkF(t, tt.name, tt.got, tt.want)
	}
}

func TestWrap(t *testing.T) {
	if got := Wrap(-1, 0, 10); got != 9 {
		t.Errorf("Wrap(-1,0,10) = %v, want 9", got)
	}
	if got := Wrap(25, 0, 10); got != 5 {
		t.Errorf("Wrap(25,0,10) = %v, want 5", got)
	}
	if got := Wrap(-7, -5, 5); got != 3 {
		t.Errorf("Wrap(-7,-5,5) = %v, want 3", got)
	}
	if got := Wrap(10, 0, 10); got != 0 {
		t.Errorf("Wrap(hi) = %v, want lo", got)
	}
	checkF(t, "Wrap float", Wrap(float32(-0.5), 0, 2), 1.5)
	if got := Wrap(float32(-1e-9), 0, 1); got >= 1 || got < 0 {
		t.Errorf("Wrap tiny negative = %v, want in [0,1)", got)
	}
	if got := Wrap(3, 5, 5); got != 5 {
		t.Errorf("Wrap of empty range = %v, want lo", got)
	}
}
