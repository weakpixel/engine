package xmath

import "math"

// Circle is a circle with center C and radius R, e.g. a blast radius or a
// round trigger.
type Circle struct {
	C Vec2
	R float32
}

// Hit is the result of a ray or sweep test. T is the fraction of the ray
// where it hit (0 to 1), Point the contact point and Normal the unit normal
// of the surface that was hit. A ray that starts inside the shape reports
// T 0, the ray's start as Point and a zero Normal.
type Hit struct {
	T      float32
	Point  Vec2
	Normal Vec2
}

// ClosestOnSegment returns the point of the segment a-b nearest to p, e.g. to
// snap a character to a rope. A degenerate segment (a == b) returns a.
func ClosestOnSegment(p, a, b Vec2) Vec2 {
	ab := b.Sub(a)
	l2 := ab.LenSq()
	if l2 == 0 {
		return a
	}
	return a.Add(ab.Mul(Clamp01(p.Sub(a).Dot(ab) / l2)))
}

// SegmentDist returns the distance from p to the segment a-b, e.g. to pick a
// gizmo arrow or to test a laser beam against the player.
func SegmentDist(p, a, b Vec2) float32 { return p.Dist(ClosestOnSegment(p, a, b)) }

// SegmentsIntersect returns the crossing point of the segments a-b and c-d,
// e.g. line of sight across a wall edge. Parallel segments (including
// overlapping ones) do not intersect.
func SegmentsIntersect(a, b, c, d Vec2) (Vec2, bool) {
	r, s := b.Sub(a), d.Sub(c)
	den := r.Cross(s)
	if den == 0 {
		return Zero, false
	}
	ca := c.Sub(a)
	t, u := ca.Cross(s)/den, ca.Cross(r)/den
	if t < 0 || t > 1 || u < 0 || u > 1 {
		return Zero, false
	}
	return a.Add(r.Mul(t)), true
}

// RaySegment tests the ray from + t·d, t in 0..1, against the segment a-b,
// e.g. a hitscan bullet against wall segments. The Normal faces the ray's
// origin. A ray parallel to the segment does not hit it.
func RaySegment(from, d, a, b Vec2) (Hit, bool) {
	e := b.Sub(a)
	den := d.Cross(e)
	if den == 0 {
		return Hit{}, false
	}
	w := a.Sub(from)
	t, u := w.Cross(e)/den, w.Cross(d)/den
	if t < 0 || t > 1 || u < 0 || u > 1 {
		return Hit{}, false
	}
	n := e.Perp().Norm()
	if n.Dot(d) > 0 {
		n = n.Neg()
	}
	return Hit{T: t, Point: from.Add(d.Mul(t)), Normal: n}, true
}

// RayRect tests the ray from + t·d, t in 0..1, against r with the slab
// method, e.g. a grappling hook against crates. A ray that starts inside r
// reports T 0; one that only runs along an edge or touches a corner does not
// hit.
func RayRect(from, d Vec2, r Rect) (Hit, bool) {
	tmin, tmax := float32(math.Inf(-1)), float32(math.Inf(1))
	var n Vec2
	axes := [2]struct{ p, d, lo, hi float32 }{
		{from.X, d.X, r.Min.X, r.Max.X},
		{from.Y, d.Y, r.Min.Y, r.Max.Y},
	}
	for i, a := range axes {
		if a.d == 0 {
			if a.p <= a.lo || a.p >= a.hi {
				return Hit{}, false // parallel and outside or on the edge
			}
			continue
		}
		t1, t2 := (a.lo-a.p)/a.d, (a.hi-a.p)/a.d
		face := float32(-1) // entering through the low face
		if t1 > t2 {
			t1, t2, face = t2, t1, 1
		}
		if t1 > tmin {
			tmin = t1
			n = Vec2{}
			if i == 0 {
				n.X = face
			} else {
				n.Y = face
			}
		}
		tmax = min(tmax, t2)
	}
	if tmax <= tmin || tmax <= 0 || tmin > 1 {
		return Hit{}, false
	}
	if tmin < 0 {
		return Hit{Point: from}, true // starts inside
	}
	return Hit{T: tmin, Point: from.Add(d.Mul(tmin)), Normal: n}, true
}

// RayCircle tests the ray from + t·d, t in 0..1, against c, e.g. a sniper
// shot against a round enemy. A ray that starts inside reports T 0. A circle
// with a radius of 0 or less is never hit.
func RayCircle(from, d Vec2, c Circle) (Hit, bool) {
	if c.R <= 0 {
		return Hit{}, false
	}
	f := from.Sub(c.C)
	cc := f.LenSq() - c.R*c.R
	if cc < 0 {
		return Hit{Point: from}, true // starts inside
	}
	a := d.LenSq()
	b := 2 * f.Dot(d)
	disc := b*b - 4*a*cc
	if a == 0 || disc <= 0 {
		return Hit{}, false
	}
	t := (-b - Sqrt(disc)) / (2 * a)
	if t < 0 || t > 1 {
		return Hit{}, false
	}
	p := from.Add(d.Mul(t))
	return Hit{T: t, Point: p, Normal: p.Sub(c.C).Mul(1 / c.R)}, true
}

// CirclesOverlap reports whether a and b share an area (touching circles do
// not), e.g. a bullet against an asteroid.
func CirclesOverlap(a, b Circle) bool { return a.C.DistSq(b.C) < Sq(a.R+b.R) }

// CircleRectOverlap reports whether c and r share an area (touching does not
// count), e.g. a round player against a spike tile.
func CircleRectOverlap(c Circle, r Rect) bool {
	return c.C.DistSq(r.ClosestPoint(c.C)) < Sq(c.R)
}

// CirclePenetration returns the smallest move of b that separates it from a,
// e.g. for crowd units pushing each other apart. It reports false when the
// circles do not overlap. Circles with the same center are pushed along Right.
func CirclePenetration(a, b Circle) (Vec2, bool) {
	d := b.C.Sub(a.C)
	sum := a.R + b.R
	l2 := d.LenSq()
	if l2 >= sum*sum {
		return Zero, false
	}
	if l2 == 0 {
		return Right.Mul(sum), true
	}
	l := Sqrt(l2)
	return d.Mul((sum - l) / l), true
}

// SweepRect moves the box moving by d and reports where it first touches
// still, e.g. so fast bullets cannot tunnel through walls. Hit.T is the
// fraction of d travelled, Hit.Point the moving box's center at that moment
// and Hit.Normal the face of still that was hit. A box that already
// overlaps still reports T 0.
func SweepRect(moving Rect, d Vec2, still Rect) (Hit, bool) {
	return RayRect(moving.Center(), d, still.GrowV(moving.HalfSize()))
}

// PointInPolygon reports whether p lies inside the polygon with the given
// corners (even-odd rule, either winding), e.g. a Tiled polygon trigger
// zone. Fewer than three points contain nothing.
func PointInPolygon(p Vec2, poly []Vec2) bool {
	in := false
	for i, j := 0, len(poly)-1; i < len(poly); j, i = i, i+1 {
		a, b := poly[i], poly[j]
		if (a.Y > p.Y) != (b.Y > p.Y) && p.X < (b.X-a.X)*(p.Y-a.Y)/(b.Y-a.Y)+a.X {
			in = !in
		}
	}
	return in && len(poly) >= 3
}

// PolygonBounds returns the bounds of the points, e.g. the broadphase box of
// a polygon collider. No points give the zero Rect. Max is the largest
// corner, so a point on it lies outside the Rect (see [Rect]).
func PolygonBounds(poly []Vec2) Rect {
	if len(poly) == 0 {
		return Rect{}
	}
	r := Rect{poly[0], poly[0]}
	for _, p := range poly[1:] {
		r = r.Extend(p)
	}
	return r
}

// PolygonArea returns the signed area of the polygon, positive when the
// corners run clockwise on screen, e.g. to check the winding of an imported
// shape. Fewer than three points have area 0.
func PolygonArea(poly []Vec2) float32 {
	var sum float32
	for i, j := 0, len(poly)-1; i < len(poly); j, i = i, i+1 {
		sum += poly[j].Cross(poly[i])
	}
	return sum / 2
}

// Barycentric returns the weights (u, v, w), summing to 1, with
// p = u·a + v·b + w·c, e.g. to interpolate vertex colors in a custom mesh.
// The weights are all inside 0..1 when p lies inside the triangle. A
// degenerate triangle returns (1, 0, 0).
func Barycentric(p, a, b, c Vec2) (u, v, w float32) {
	v0, v1, v2 := b.Sub(a), c.Sub(a), p.Sub(a)
	den := v0.Cross(v1)
	if den == 0 {
		return 1, 0, 0
	}
	v = v2.Cross(v1) / den
	w = v0.Cross(v2) / den
	return 1 - v - w, v, w
}
