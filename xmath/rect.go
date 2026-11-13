package xmath

// Rect is an axis-aligned rectangle from Min (inclusive) to Max (exclusive).
// It serializes as {"Min": {..}, "Max": {..}}, like [Vec2] without a
// MarshalJSON.
type Rect struct {
	Min, Max Vec2
}

// RectXYWH returns the rectangle with corner (x, y) and size (w, h), e.g. a
// Tiled object's box.
func RectXYWH(x, y, w, h float32) Rect { return Rect{Vec2{x, y}, Vec2{x + w, y + h}} }

// RectCentered returns the rectangle of the given size around c, e.g. a
// hitbox around the player's feet.
func RectCentered(c, size Vec2) Rect {
	h := size.Mul(0.5)
	return Rect{c.Sub(h), c.Add(h)}
}

// RectPoints returns the rectangle spanning two points in any order, e.g. a
// mouse drag selection.
func RectPoints(a, b Vec2) Rect { return Rect{a.Min(b), a.Max(b)} }

// Size returns Max - Min, e.g. to scale a background to fill r.
func (r Rect) Size() Vec2 { return r.Max.Sub(r.Min) }

// W returns the width, e.g. to center text: x := r.Min.X + (r.W()-textW)/2.
func (r Rect) W() float32 { return r.Max.X - r.Min.X }

// H returns the height.
func (r Rect) H() float32 { return r.Max.Y - r.Min.Y }

// Center returns the middle point, e.g. to spawn loot at a chest's center.
func (r Rect) Center() Vec2 { return r.Min.Add(r.Max).Mul(0.5) }

// HalfSize returns Size / 2, e.g. the extents for an AABB collision test.
func (r Rect) HalfSize() Vec2 { return r.Size().Mul(0.5) }

// Empty reports whether r has no area (width or height of 0 or less), e.g. to
// skip drawing a collapsed panel.
func (r Rect) Empty() bool { return r.W() <= 0 || r.H() <= 0 }

// Contains reports whether p lies inside r (Min in, Max out), e.g. a button
// hover test.
func (r Rect) Contains(p Vec2) bool {
	return p.X >= r.Min.X && p.X < r.Max.X && p.Y >= r.Min.Y && p.Y < r.Max.Y
}

// ContainsRect reports whether o lies fully inside r, e.g. "is the room fully
// on screen".
func (r Rect) ContainsRect(o Rect) bool {
	return o.Min.X >= r.Min.X && o.Max.X <= r.Max.X && o.Min.Y >= r.Min.Y && o.Max.Y <= r.Max.Y
}

// Overlaps reports whether r and o share an area; rectangles that only touch
// along an edge do not overlap. E.g. player against a coin box.
func (r Rect) Overlaps(o Rect) bool {
	return r.Min.X < o.Max.X && o.Min.X < r.Max.X && r.Min.Y < o.Max.Y && o.Min.Y < r.Max.Y
}

// Intersect returns the region r and o share, or the zero Rect when they do
// not overlap, e.g. to clip a sprite to the viewport.
func (r Rect) Intersect(o Rect) Rect {
	if !r.Overlaps(o) {
		return Rect{}
	}
	return Rect{r.Min.Max(o.Min), r.Max.Min(o.Max)}
}

// Union returns the smallest rectangle holding both, e.g. the bounds of all
// selected objects.
func (r Rect) Union(o Rect) Rect { return Rect{r.Min.Min(o.Min), r.Max.Max(o.Max)} }

// Extend returns r grown to include p, e.g. to build level bounds from points.
func (r Rect) Extend(p Vec2) Rect { return Rect{r.Min.Min(p), r.Max.Max(p)} }

// Translate returns r moved by d, e.g. a hitbox that follows its entity.
func (r Rect) Translate(d Vec2) Rect { return Rect{r.Min.Add(d), r.Max.Add(d)} }

// Grow returns r expanded by m on every side; a negative m shrinks it, e.g. a
// pickup radius margin.
func (r Rect) Grow(m float32) Rect { return r.GrowV(Splat(m)) }

// GrowV returns r expanded by m.X left and right and m.Y above and below,
// e.g. a camera dead zone of 32×16.
func (r Rect) GrowV(m Vec2) Rect { return Rect{r.Min.Sub(m), r.Max.Add(m)} }

// ClosestPoint returns the point of r nearest to p, e.g. for a circle against
// a box or a magnet pull to a platform.
func (r Rect) ClosestPoint(p Vec2) Vec2 { return r.ClampPoint(p) }

// ClampPoint returns p clamped into r, e.g. to keep the player inside the
// arena. An empty r returns Min.
func (r Rect) ClampPoint(p Vec2) Vec2 {
	return Vec2{max(r.Min.X, min(p.X, r.Max.X)), max(r.Min.Y, min(p.Y, r.Max.Y))}
}

// ClampCenter returns c moved so a box of half-size half around it stays
// inside r. On an axis where the box does not fit, it returns the center of
// r. E.g. camera bounds.
func (r Rect) ClampCenter(c, half Vec2) Vec2 {
	return Vec2{
		clampAxis(c.X, half.X, r.Min.X, r.Max.X),
		clampAxis(c.Y, half.Y, r.Min.Y, r.Max.Y),
	}
}

func clampAxis(p, half, lo, hi float32) float32 {
	if hi-lo <= 2*half {
		return (lo + hi) / 2
	}
	return min(max(p, lo+half), hi-half)
}

// Penetration returns the smallest push that moves o out of r, or Zero when
// they do not overlap, e.g. a simple box collision response. It pushes along
// the axis of least overlap, away from r's center; ties go to X.
func (r Rect) Penetration(o Rect) Vec2 {
	if !r.Overlaps(o) {
		return Zero
	}
	// Overlap on each axis, and which way o must move to leave.
	px := min(r.Max.X-o.Min.X, o.Max.X-r.Min.X)
	py := min(r.Max.Y-o.Min.Y, o.Max.Y-r.Min.Y)
	d := o.Center().Sub(r.Center())
	if px <= py {
		return Vec2{px * signNonZero(d.X), 0}
	}
	return Vec2{0, py * signNonZero(d.Y)}
}

// signNonZero is Sign, but 1 for 0 so a push always has a direction.
func signNonZero(x float32) float32 {
	if x < 0 {
		return -1
	}
	return 1
}

// Lerp blends the corners of two rectangles, e.g. an animated selection frame.
func (r Rect) Lerp(o Rect, t float32) Rect {
	return Rect{r.Min.Lerp(o.Min, t), r.Max.Lerp(o.Max, t)}
}

// Corners returns the corners in the order top-left, top-right, bottom-right,
// bottom-left on screen, e.g. to draw a debug outline.
func (r Rect) Corners() [4]Vec2 {
	return [4]Vec2{r.Min, {r.Max.X, r.Min.Y}, r.Max, {r.Min.X, r.Max.Y}}
}
