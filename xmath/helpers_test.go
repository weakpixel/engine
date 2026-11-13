package xmath

import "testing"

// near reports whether a and b differ by at most eps.
func near(a, b, eps float32) bool { return Abs(a-b) <= eps }

func nearV(a, b Vec2, eps float32) bool { return near(a.X, b.X, eps) && near(a.Y, b.Y, eps) }

func checkF(t *testing.T, name string, got, want float32) {
	t.Helper()
	if !near(got, want, 1e-5) {
		t.Errorf("%s = %v, want %v", name, got, want)
	}
}

func checkV(t *testing.T, name string, got, want Vec2) {
	t.Helper()
	if !nearV(got, want, 1e-5) {
		t.Errorf("%s = %v, want %v", name, got, want)
	}
}
