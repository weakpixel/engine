package xmath

import "testing"

var curves = map[string]EaseFunc{
	"Linear": EaseLinear, "InQuad": EaseInQuad, "OutQuad": EaseOutQuad, "InOutQuad": EaseInOutQuad,
	"InCubic": EaseInCubic, "OutCubic": EaseOutCubic, "InOutCubic": EaseInOutCubic,
	"InSine": EaseInSine, "OutSine": EaseOutSine, "InOutSine": EaseInOutSine,
	"InExpo": EaseInExpo, "OutExpo": EaseOutExpo, "InOutExpo": EaseInOutExpo,
	"InBack": EaseInBack, "OutBack": EaseOutBack, "InOutBack": EaseInOutBack,
	"InElastic": EaseInElastic, "OutElastic": EaseOutElastic,
	"InBounce": EaseInBounce, "OutBounce": EaseOutBounce,
}

func TestEaseEndpoints(t *testing.T) {
	for name, f := range curves {
		if got := f(0); !near(got, 0, 1e-3) {
			t.Errorf("%s(0) = %v, want 0", name, got)
		}
		if got := f(1); !near(got, 1, 1e-3) {
			t.Errorf("%s(1) = %v, want 1", name, got)
		}
	}
}

func TestEaseShapes(t *testing.T) {
	checkF(t, "InQuad", EaseInQuad(0.5), 0.25)
	checkF(t, "OutQuad", EaseOutQuad(0.5), 0.75)
	checkF(t, "InOutQuad low", EaseInOutQuad(0.25), 0.125)
	checkF(t, "InOutQuad high", EaseInOutQuad(0.75), 0.875)
	checkF(t, "InCubic", EaseInCubic(0.5), 0.125)
	if EaseInBack(0.2) >= 0 {
		t.Error("InBack should undershoot below 0")
	}
	if EaseOutBack(0.8) <= 1 {
		t.Error("OutBack should overshoot above 1")
	}
}

func TestReverseMirror(t *testing.T) {
	out := Reverse(EaseInQuad)
	for _, x := range []float32{0, 0.3, 0.5, 1} {
		checkF(t, "Reverse(InQuad)", out(x), EaseOutQuad(x))
	}
	inout := Mirror(EaseInCubic)
	for _, x := range []float32{0, 0.2, 0.5, 0.8, 1} {
		checkF(t, "Mirror(InCubic)", inout(x), EaseInOutCubic(x))
	}
}
