package viewkit

import "testing"

// withScale sets the display scale factor for the test.
func withScale(t *testing.T, f float64) {
	t.Helper()
	old := DeviceScale
	DeviceScale = func() float64 { return f }
	t.Cleanup(func() { DeviceScale = old })
}

func TestLabelFollowsDeviceScale(t *testing.T) {
	var c Labels
	withScale(t, 1)
	b1 := c.image("player").Bounds()
	if b1.Dy() < LabelSize {
		t.Fatalf("label is %d px high at scale 1, want at least %d", b1.Dy(), LabelSize)
	}
	withScale(t, 2)
	b2 := c.image("player").Bounds()
	if b2.Dy()*10 < b1.Dy()*18 || b2.Dx()*10 < b1.Dx()*18 { // text metrics do not scale exactly
		t.Fatalf("label is %v at scale 2, want about twice %v", b2.Size(), b1.Size())
	}
}

func TestLabelFits(t *testing.T) {
	var c Labels
	withScale(t, 2)
	w := c.Width("a")
	for _, tc := range []struct {
		box  float32
		want bool
	}{
		{MinLabelPx, false},                // under the scaled minimum
		{2 * MinLabelPx, w < 2*MinLabelPx}, // at it
		{w, false},                         // as wide as the label
		{max(w, 2*MinLabelPx) + 1, true},
	} {
		if got := c.Fits("a", tc.box); got != tc.want {
			t.Errorf("fits(%q, %g) = %v, want %v (label %g px)", "a", tc.box, got, tc.want, w)
		}
	}
}
