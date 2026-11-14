package viewer

import "testing"

// withScale sets the display scale factor for the test.
func withScale(t *testing.T, f float64) {
	t.Helper()
	old := deviceScale
	deviceScale = func() float64 { return f }
	t.Cleanup(func() { deviceScale = old })
}

func TestLabelFollowsDeviceScale(t *testing.T) {
	var c labelCache
	withScale(t, 1)
	b1 := c.image("player").Bounds()
	if b1.Dy() < labelSize {
		t.Fatalf("label is %d px high at scale 1, want at least %d", b1.Dy(), labelSize)
	}
	withScale(t, 2)
	b2 := c.image("player").Bounds()
	if b2.Dy()*10 < b1.Dy()*18 || b2.Dx()*10 < b1.Dx()*18 { // text metrics do not scale exactly
		t.Fatalf("label is %v at scale 2, want about twice %v", b2.Size(), b1.Size())
	}
}

func TestLabelFits(t *testing.T) {
	var c labelCache
	withScale(t, 2)
	w := c.width("a")
	for _, tc := range []struct {
		box  float32
		want bool
	}{
		{minLabelPx, false},                // under the scaled minimum
		{2 * minLabelPx, w < 2*minLabelPx}, // at it
		{w, false},                         // as wide as the label
		{max(w, 2*minLabelPx) + 1, true},
	} {
		if got := c.fits("a", tc.box); got != tc.want {
			t.Errorf("fits(%q, %g) = %v, want %v (label %g px)", "a", tc.box, got, tc.want, w)
		}
	}
}
