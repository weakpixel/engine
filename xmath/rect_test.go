package xmath

import "testing"

func TestRectBasics(t *testing.T) {
	r := RectXYWH(10, 20, 30, 40)
	checkV(t, "Size", r.Size(), V(30, 40))
	checkV(t, "Center", r.Center(), V(25, 40))
	checkV(t, "HalfSize", r.HalfSize(), V(15, 20))
	checkF(t, "W", r.W(), 30)
	checkF(t, "H", r.H(), 40)
	if got := RectCentered(V(25, 40), V(30, 40)); got != r {
		t.Errorf("RectCentered = %v, want %v", got, r)
	}
	if got := RectPoints(V(40, 60), V(10, 20)); got != r {
		t.Errorf("RectPoints = %v, want %v", got, r)
	}
	if r.Empty() || !(Rect{}).Empty() || !RectXYWH(0, 0, -1, 5).Empty() {
		t.Error("Empty")
	}
	if c := r.Corners(); c[0] != V(10, 20) || c[1] != V(40, 20) || c[2] != V(40, 60) || c[3] != V(10, 60) {
		t.Errorf("Corners = %v", c)
	}
}

func TestRectContains(t *testing.T) {
	r := RectXYWH(0, 0, 10, 10)
	tests := []struct {
		p    Vec2
		want bool
	}{
		{V(0, 0), true}, {V(9.99, 9.99), true}, {V(10, 5), false}, {V(5, 10), false}, {V(-0.1, 5), false},
	}
	for _, tt := range tests {
		if got := r.Contains(tt.p); got != tt.want {
			t.Errorf("Contains(%v) = %v, want %v", tt.p, got, tt.want)
		}
	}
	if !r.ContainsRect(RectXYWH(2, 2, 8, 8)) || r.ContainsRect(RectXYWH(2, 2, 9, 8)) {
		t.Error("ContainsRect")
	}
}

func TestRectOverlap(t *testing.T) {
	a := RectXYWH(0, 0, 10, 10)
	tests := []struct {
		name string
		b    Rect
		want bool
	}{
		{"inside", RectXYWH(2, 2, 2, 2), true},
		{"partial", RectXYWH(8, 8, 5, 5), true},
		{"touching edge", RectXYWH(10, 0, 5, 5), false},
		{"apart", RectXYWH(20, 20, 1, 1), false},
	}
	for _, tt := range tests {
		if got := a.Overlaps(tt.b); got != tt.want {
			t.Errorf("%s: Overlaps = %v, want %v", tt.name, got, tt.want)
		}
		if got := tt.b.Overlaps(a); got != tt.want {
			t.Errorf("%s: reversed Overlaps = %v, want %v", tt.name, got, tt.want)
		}
	}
	if got := a.Intersect(RectXYWH(8, 8, 5, 5)); got != RectXYWH(8, 8, 2, 2) {
		t.Errorf("Intersect = %v", got)
	}
	if got := a.Intersect(RectXYWH(20, 20, 1, 1)); got != (Rect{}) {
		t.Errorf("Intersect of apart = %v, want zero", got)
	}
	if got := a.Union(RectXYWH(20, 20, 1, 1)); got != (Rect{V(0, 0), V(21, 21)}) {
		t.Errorf("Union = %v", got)
	}
	if got := a.Extend(V(-5, 3)); got != (Rect{V(-5, 0), V(10, 10)}) {
		t.Errorf("Extend = %v", got)
	}
}

func TestRectTransforms(t *testing.T) {
	r := RectXYWH(0, 0, 10, 10)
	if got := r.Translate(V(5, -5)); got != RectXYWH(5, -5, 10, 10) {
		t.Errorf("Translate = %v", got)
	}
	if got := r.Grow(2); got != RectXYWH(-2, -2, 14, 14) {
		t.Errorf("Grow = %v", got)
	}
	if got := r.Grow(-2); got != RectXYWH(2, 2, 6, 6) {
		t.Errorf("Grow(-2) = %v", got)
	}
	if got := r.GrowV(V(1, 3)); got != RectXYWH(-1, -3, 12, 16) {
		t.Errorf("GrowV = %v", got)
	}
	if got := r.Lerp(RectXYWH(10, 10, 10, 10), 0.5); got != RectXYWH(5, 5, 10, 10) {
		t.Errorf("Lerp = %v", got)
	}
}

func TestRectPoints(t *testing.T) {
	r := RectXYWH(0, 0, 10, 10)
	checkV(t, "ClampPoint outside", r.ClampPoint(V(-5, 20)), V(0, 10))
	checkV(t, "ClosestPoint inside", r.ClosestPoint(V(3, 4)), V(3, 4))
	// Camera bounds: clamped when the view fits, centered when it does not.
	checkV(t, "ClampCenter fits", r.ClampCenter(V(0, 5), V(2, 2)), V(2, 5))
	checkV(t, "ClampCenter too big", r.ClampCenter(V(0, 5), V(6, 2)), V(5, 5))
}

func TestRectPenetration(t *testing.T) {
	r := RectXYWH(0, 0, 10, 10)
	tests := []struct {
		name string
		o    Rect
		want Vec2
	}{
		{"apart", RectXYWH(20, 0, 5, 5), Zero},
		{"touching", RectXYWH(10, 0, 5, 5), Zero},
		{"from the right", RectXYWH(8, 2, 5, 5), V(2, 0)},
		{"from the left", RectXYWH(-3, 2, 5, 5), V(-2, 0)},
		{"from below", RectXYWH(2, 9, 5, 5), V(0, 1)},
		{"from above", RectXYWH(2, -4, 5, 5), V(0, -1)},
	}
	for _, tt := range tests {
		got := r.Penetration(tt.o)
		checkV(t, tt.name, got, tt.want)
		// Applying the push separates the boxes.
		if r.Overlaps(tt.o.Translate(got)) {
			t.Errorf("%s: still overlapping after the push", tt.name)
		}
	}
}
