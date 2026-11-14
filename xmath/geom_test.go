package xmath

import "testing"

func TestRayRect(t *testing.T) {
	// The collision.Space.Raycast cases against a 10x10 box at (20,-5),
	// and the same box tests the old collision.rayBox answered.
	box := RectXYWH(20, -5, 10, 10)
	tests := []struct {
		name    string
		from, d Vec2
		r       Rect
		ok      bool
		want    Hit
	}{
		{"nearest box", V(0, 0), V(100, 0), box, true, Hit{0.2, V(20, 0), V(-1, 0)}},
		{"too short", V(0, 0), V(10, 0), box, false, Hit{}},
		{"along an edge", V(0, -5), V(100, 0), box, false, Hit{}},
		{"along the far edge", V(0, 5), V(100, 0), box, false, Hit{}},
		{"from inside", V(25, 0), V(75, 0), box, true, Hit{0, V(25, 0), V(0, 0)}},
		{"from the right", V(100, 0), V(-100, 0), box, true, Hit{0.7, V(30, 0), V(1, 0)}},
		{"from above", V(25, -50), V(0, 100), box, true, Hit{0.45, V(25, -5), V(0, -1)}},
		{"from below", V(25, 50), V(0, -100), box, true, Hit{0.45, V(25, 5), V(0, 1)}},
		{"behind", V(40, 0), V(100, 0), box, false, Hit{}},
		{"missing", V(0, 20), V(100, 0), box, false, Hit{}},
		{"zero ray inside", V(25, 0), V(0, 0), box, true, Hit{0, V(25, 0), V(0, 0)}},
		{"zero ray outside", V(0, 0), V(0, 0), box, false, Hit{}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h, ok := RayRect(tc.from, tc.d, tc.r)
			if ok != tc.ok || (ok && (!near(h.T, tc.want.T, 1e-5) || !nearV(h.Point, tc.want.Point, 1e-4) || !nearV(h.Normal, tc.want.Normal, 1e-6))) {
				t.Errorf("got %+v %v, want %+v %v", h, ok, tc.want, tc.ok)
			}
		})
	}
}

func TestRayCircle(t *testing.T) {
	c := Circle{V(0, 40), 4}
	tests := []struct {
		name    string
		from, d Vec2
		c       Circle
		ok      bool
		want    Hit
	}{
		{"circle", V(0, 0), V(0, 100), c, true, Hit{0.36, V(0, 36), V(0, -1)}},
		{"too short", V(0, 0), V(0, 10), c, false, Hit{}},
		{"from inside", V(0, 41), V(0, 100), c, true, Hit{0, V(0, 41), V(0, 0)}},
		{"miss", V(10, 0), V(0, 100), c, false, Hit{}},
		{"grazing", V(4, 0), V(0, 100), c, false, Hit{}},
		{"away", V(0, 0), V(0, -100), c, false, Hit{}},
		{"zero ray outside", V(0, 0), V(0, 0), c, false, Hit{}},
		{"no radius", V(0, 0), V(0, 100), Circle{V(0, 40), 0}, false, Hit{}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			h, ok := RayCircle(tc.from, tc.d, tc.c)
			if ok != tc.ok || (ok && (!near(h.T, tc.want.T, 1e-5) || !nearV(h.Point, tc.want.Point, 1e-4) || !nearV(h.Normal, tc.want.Normal, 1e-6))) {
				t.Errorf("got %+v %v, want %+v %v", h, ok, tc.want, tc.ok)
			}
		})
	}
}

func TestSegments(t *testing.T) {
	a, b := V(0, 0), V(10, 0)
	checkV(t, "closest middle", ClosestOnSegment(V(4, 3), a, b), V(4, 0))
	checkV(t, "closest before", ClosestOnSegment(V(-4, 3), a, b), a)
	checkV(t, "closest after", ClosestOnSegment(V(14, 3), a, b), b)
	checkV(t, "closest degenerate", ClosestOnSegment(V(4, 3), a, a), a)
	checkF(t, "SegmentDist", SegmentDist(V(4, 3), a, b), 3)
	checkF(t, "SegmentDist end", SegmentDist(V(13, 4), a, b), 5)

	t.Run("SegmentsIntersect", func(t *testing.T) {
		p, ok := SegmentsIntersect(V(0, 0), V(10, 10), V(0, 10), V(10, 0))
		if !ok {
			t.Fatal("crossing segments do not intersect")
		}
		checkV(t, "cross", p, V(5, 5))
		for name, c := range map[string][4]Vec2{
			"parallel":     {V(0, 0), V(10, 0), V(0, 1), V(10, 1)},
			"collinear":    {V(0, 0), V(10, 0), V(5, 0), V(15, 0)},
			"short":        {V(0, 0), V(4, 4), V(0, 10), V(10, 0)},
			"degenerate":   {V(1, 1), V(1, 1), V(0, 10), V(10, 0)},
			"other is far": {V(0, 0), V(10, 0), V(20, -5), V(20, 5)},
		} {
			if _, ok := SegmentsIntersect(c[0], c[1], c[2], c[3]); ok {
				t.Errorf("%s: intersect reported", name)
			}
		}
	})
	t.Run("RaySegment", func(t *testing.T) {
		wallA, wallB := V(10, -5), V(10, 5)
		h, ok := RaySegment(V(0, 0), V(20, 0), wallA, wallB)
		if !ok {
			t.Fatal("no hit")
		}
		checkF(t, "T", h.T, 0.5)
		checkV(t, "Point", h.Point, V(10, 0))
		checkV(t, "Normal", h.Normal, V(-1, 0)) // faces the origin
		h, _ = RaySegment(V(20, 0), V(-20, 0), wallA, wallB)
		checkV(t, "Normal from the other side", h.Normal, V(1, 0))
		for name, c := range map[string][2]Vec2{
			"too short": {V(0, 0), V(5, 0)},
			"parallel":  {V(9, -9), V(0, 20)},
			"behind":    {V(0, 0), V(-20, 0)},
			"above":     {V(0, -9), V(20, 0)},
		} {
			if _, ok := RaySegment(c[0], c[1], wallA, wallB); ok {
				t.Errorf("%s: hit reported", name)
			}
		}
	})
}

func TestCircles(t *testing.T) {
	a := Circle{V(0, 0), 5}
	if !CirclesOverlap(a, Circle{V(8, 0), 5}) {
		t.Error("overlapping circles")
	}
	if CirclesOverlap(a, Circle{V(10, 0), 5}) {
		t.Error("touching circles overlap")
	}
	r := RectXYWH(10, 0, 10, 10)
	if !CircleRectOverlap(Circle{V(6, 5), 5}, r) {
		t.Error("circle reaching into the box")
	}
	if CircleRectOverlap(Circle{V(5, 5), 5}, r) {
		t.Error("circle touching the box")
	}
	if !CircleRectOverlap(Circle{V(15, 5), 1}, r) {
		t.Error("circle inside the box")
	}
	if CircleRectOverlap(Circle{V(2, 2), 3}, r) { // corner distance from (10,2): 8
		t.Error("circle far from the box")
	}
	t.Run("CirclePenetration", func(t *testing.T) {
		push, ok := CirclePenetration(a, Circle{V(8, 0), 5})
		if !ok {
			t.Fatal("no penetration")
		}
		checkV(t, "push", push, V(2, 0))
		push, ok = CirclePenetration(a, Circle{V(0, 0), 3})
		if !ok {
			t.Fatal("coincident circles do not penetrate")
		}
		checkV(t, "coincident", push, V(8, 0))
		if _, ok := CirclePenetration(a, Circle{V(10, 0), 5}); ok {
			t.Error("touching circles penetrate")
		}
	})
}

func TestSweepRect(t *testing.T) {
	still := RectXYWH(20, 0, 10, 10)
	moving := RectXYWH(0, 2, 4, 4) // center (2, 4), half size 2
	h, ok := SweepRect(moving, V(40, 0), still)
	if !ok {
		t.Fatal("no hit")
	}
	checkF(t, "T", h.T, 0.4) // right edge reaches x = 20 after 16 of 40
	checkV(t, "Point", h.Point, V(18, 4))
	checkV(t, "Normal", h.Normal, V(-1, 0))
	if _, ok := SweepRect(moving, V(10, 0), still); ok {
		t.Error("short sweep hits")
	}
	if _, ok := SweepRect(RectXYWH(0, 20, 4, 4), V(40, 0), still); ok {
		t.Error("sweep passing below hits")
	}
	if _, ok := SweepRect(RectXYWH(0, 6, 4, 4), V(40, 0), still); !ok {
		t.Error("sweep clipping the box misses")
	}
	if h, ok := SweepRect(RectXYWH(18, 2, 4, 4), V(40, 0), still); !ok || h.T != 0 {
		t.Errorf("already overlapping = %+v %v, want T 0", h, ok)
	}
}

func TestPolygon(t *testing.T) {
	// An L shape, clockwise on screen (y down).
	l := []Vec2{V(0, 0), V(10, 0), V(10, 5), V(5, 5), V(5, 10), V(0, 10)}
	tests := []struct {
		name string
		p    Vec2
		want bool
	}{
		{"inside the long arm", V(2, 8), true},
		{"inside the top", V(8, 2), true},
		{"in the notch", V(8, 8), false},
		{"outside", V(-1, 5), false},
		{"far", V(100, 100), false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := PointInPolygon(tc.p, l); got != tc.want {
				t.Errorf("PointInPolygon(%v) = %v, want %v", tc.p, got, tc.want)
			}
		})
	}
	if PointInPolygon(V(1, 1), l[:2]) || PointInPolygon(V(1, 1), nil) {
		t.Error("fewer than three points contain a point")
	}
	b := PolygonBounds(l)
	checkV(t, "bounds min", b.Min, V(0, 0))
	checkV(t, "bounds max", b.Max, V(10, 10))
	if PolygonBounds(nil) != (Rect{}) {
		t.Error("bounds of nothing")
	}
	checkF(t, "area clockwise", PolygonArea(l), 75)
	rev := []Vec2{l[5], l[4], l[3], l[2], l[1], l[0]}
	checkF(t, "area counter-clockwise", PolygonArea(rev), -75)
	checkF(t, "area of a line", PolygonArea(l[:2]), 0)
}

func TestBarycentric(t *testing.T) {
	a, b, c := V(0, 0), V(10, 0), V(0, 10)
	u, v, w := Barycentric(V(0, 0), a, b, c)
	checkF(t, "u at a", u, 1)
	checkF(t, "v at a", v, 0)
	checkF(t, "w at a", w, 0)
	u, v, w = Barycentric(V(5, 0), a, b, c)
	checkF(t, "u mid ab", u, 0.5)
	checkF(t, "v mid ab", v, 0.5)
	checkF(t, "w mid ab", w, 0)
	u, v, w = Barycentric(V(2, 3), a, b, c)
	checkF(t, "sum", u+v+w, 1)
	checkV(t, "reconstruct", a.Mul(u).Add(b.Mul(v)).Add(c.Mul(w)), V(2, 3))
	u, v, w = Barycentric(V(20, 20), a, b, a)
	if u != 1 || v != 0 || w != 0 {
		t.Errorf("degenerate = %v %v %v, want 1 0 0", u, v, w)
	}
}
