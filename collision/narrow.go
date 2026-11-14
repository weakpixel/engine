package collision

import "github.com/weakpixel/engine/gfx"

// NarrowFunc tests two shapes for overlap. normal is the unit vector that
// points from self out of other (the direction to push self), depth the
// penetration along it (>= 0). Shapes that only touch do not overlap.
type NarrowFunc func(self, other *Shape) (normal gfx.Vec2, depth float32, ok bool)

// pairKind indexes the custom narrow-phase table.
type pairKind struct{ a, b ShapeKind }

// narrow holds the narrow phase of a Space: the built-in pairs are a switch,
// custom pairs a map, filled by RegisterPair.
type narrow struct {
	bounds map[ShapeKind]BoundsFunc
	pairs  map[pairKind]NarrowFunc
}

// boundsOf returns the bounds function of a custom kind, or nil.
func (n *narrow) boundsOf(k ShapeKind) BoundsFunc { return n.bounds[k] }

// test runs the narrow phase for a and b. Both kinds are ShapeRect,
// ShapeCircle or registered.
func (n *narrow) test(a, b *Shape) (gfx.Vec2, float32, bool) {
	switch {
	case a.Kind == ShapeRect && b.Kind == ShapeRect:
		return rectRect(a, b)
	case a.Kind == ShapeCircle && b.Kind == ShapeCircle:
		return circleCircle(a, b)
	case a.Kind == ShapeCircle && b.Kind == ShapeRect:
		return circleRect(a, b)
	case a.Kind == ShapeRect && b.Kind == ShapeCircle:
		nrm, d, ok := circleRect(b, a)
		return nrm.Mul(-1), d, ok
	}
	if f := n.pairs[pairKind{a.Kind, b.Kind}]; f != nil {
		return f(a, b)
	}
	panic("collision: no narrow phase for shapes " + a.Kind.String() + " and " + b.Kind.String())
}

func isBuiltin(k ShapeKind) bool { return k < ShapeCustom }

// rectRect separates two boxes along the axis of least overlap.
func rectRect(a, b *Shape) (gfx.Vec2, float32, bool) {
	ra, rb := a.Bounds, b.Bounds
	ox := min(ra.Max.X, rb.Max.X) - max(ra.Min.X, rb.Min.X)
	oy := min(ra.Max.Y, rb.Max.Y) - max(ra.Min.Y, rb.Min.Y)
	if ox <= 0 || oy <= 0 {
		return gfx.Vec2{}, 0, false
	}
	if ox < oy {
		if a.Center.X < b.Center.X {
			return gfx.Vec2{X: -1}, ox, true
		}
		return gfx.Vec2{X: 1}, ox, true
	}
	if a.Center.Y < b.Center.Y {
		return gfx.Vec2{Y: -1}, oy, true
	}
	return gfx.Vec2{Y: 1}, oy, true
}

func circleCircle(a, b *Shape) (gfx.Vec2, float32, bool) {
	d := a.Center.Sub(b.Center)
	r := a.Radius + b.Radius
	if d.X*d.X+d.Y*d.Y >= r*r {
		return gfx.Vec2{}, 0, false
	}
	dist := d.Len()
	if dist == 0 {
		return gfx.Vec2{Y: -1}, r, true // same center: push up
	}
	return d.Mul(1 / dist), r - dist, true
}

// circleRect tests circle c against box r; the normal pushes the circle out.
func circleRect(c, r *Shape) (gfx.Vec2, float32, bool) {
	b := r.Bounds
	p := c.Center
	q := gfx.Vec2{X: min(max(p.X, b.Min.X), b.Max.X), Y: min(max(p.Y, b.Min.Y), b.Max.Y)}
	if q != p {
		d := p.Sub(q)
		dd := d.X*d.X + d.Y*d.Y
		if dd >= c.Radius*c.Radius {
			return gfx.Vec2{}, 0, false
		}
		dist := d.Len()
		return d.Mul(1 / dist), c.Radius - dist, true
	}
	// The center is inside the box: leave through the nearest edge.
	left, right := p.X-b.Min.X, b.Max.X-p.X
	up, down := p.Y-b.Min.Y, b.Max.Y-p.Y
	n, e := gfx.Vec2{X: -1}, left
	if right < e {
		n, e = gfx.Vec2{X: 1}, right
	}
	if up < e {
		n, e = gfx.Vec2{Y: -1}, up
	}
	if down < e {
		n, e = gfx.Vec2{Y: 1}, down
	}
	return n, e + c.Radius, true
}
