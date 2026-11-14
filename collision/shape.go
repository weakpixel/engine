package collision

import (
	"github.com/weakpixel/ecs"
	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/xmath"
)

// Shape is a collider placed in the world, as the broad and narrow phases
// see it. The [Space] builds one per collider each tick; custom narrow-phase
// functions ([NarrowFunc]) receive two of them.
type Shape struct {
	// Entity owns the collider; the zero Entity for a tile cell.
	Entity ecs.Entity
	// Kind is the collider's kind, with ShapeAnimBody turned into
	// ShapeRect.
	Kind ShapeKind
	// Bounds is the axis-aligned box around the shape, in world units.
	Bounds gfx.Rect
	// Center is the circle's center, or the middle of Bounds for other
	// kinds.
	Center gfx.Vec2
	// Radius is the circle's radius in world units, zero for other kinds.
	Radius float32
	// Layer is the collider's layer set, with the zero-Layer default
	// applied; Mask is its mask.
	Layer, Mask Layers
	// Pos and Scale are the entity's Transform.Pos and Transform.Scale (zero
	// Scale is (1, 1)), for custom kinds that need them.
	Pos, Scale gfx.Vec2
	// Collider is a copy of the entity's collider.
	Collider Collider
}

// BoundsFunc returns the world bounds of a custom-kind collider. t.Scale is
// already (1, 1) when the Transform's is zero.
type BoundsFunc func(t *gfx.Transform, c *Collider) gfx.Rect

// scaleOf returns t.Scale, with the zero value meaning (1, 1).
func scaleOf(t *gfx.Transform) gfx.Vec2 {
	if t.Scale == (gfx.Vec2{}) {
		return gfx.Vec2{X: 1, Y: 1}
	}
	return t.Scale
}

// span maps the interval [p, p+size) by scale s and moves it to pos. A
// negative scale mirrors it, so the ends are swapped to keep lo <= hi (as
// gfx.Box.World does).
func span(pos, p, size, s float32) (lo, hi float32) {
	lo, hi = pos+p*s, pos+(p+size)*s
	if lo > hi {
		lo, hi = hi, lo
	}
	return lo, hi
}

// makeShape places c at t, with the bounds of custom kinds from n. ok is
// false for a collider that is skipped (an anim-body collider without a size
// yet).
//
// Panics with "collision: unknown shape kind <k>: ..." for a kind that is
// neither built in nor registered.
func makeShape(e ecs.Entity, t *gfx.Transform, c *Collider, n *narrow) (sh Shape, ok bool) {
	s := scaleOf(t)
	sh = Shape{
		Entity: e, Kind: c.Shape,
		Layer: c.LayerOrDefault(), Mask: c.Mask,
		Pos: t.Pos, Scale: s, Collider: *c,
	}
	switch c.Shape {
	case ShapeAnimBody:
		if c.Size == (gfx.Vec2{}) {
			return sh, false
		}
		sh.Kind = ShapeRect
		fallthrough
	case ShapeRect:
		x0, x1 := span(t.Pos.X, c.Offset.X, c.Size.X, s.X)
		y0, y1 := span(t.Pos.Y, c.Offset.Y, c.Size.Y, s.Y)
		sh.Bounds = gfx.Rect{Min: gfx.Vec2{X: x0, Y: y0}, Max: gfx.Vec2{X: x1, Y: y1}}
		sh.Center = sh.Bounds.Center()
	case ShapeCircle:
		sh.Center = gfx.Vec2{X: t.Pos.X + c.Offset.X*s.X, Y: t.Pos.Y + c.Offset.Y*s.Y}
		sh.Radius = c.Size.X * xmath.Abs(s.X)
		sh.Bounds = gfx.Rect{
			Min: gfx.Vec2{X: sh.Center.X - sh.Radius, Y: sh.Center.Y - sh.Radius},
			Max: gfx.Vec2{X: sh.Center.X + sh.Radius, Y: sh.Center.Y + sh.Radius},
		}
	default:
		f := n.boundsOf(c.Shape)
		if f == nil {
			panic("collision: unknown shape kind " + c.Shape.String() + ": register it with Space.RegisterShape")
		}
		ts := *t
		ts.Scale = s
		sh.Bounds = f(&ts, c)
		sh.Center = sh.Bounds.Center()
	}
	return sh, true
}

// ShapeOf places c at t like the [Space] does each tick, for tools that draw
// colliders without running one (the level editor). ok is false for a custom
// kind (the Space asks its registered bounds function) and for an anim-body
// collider without a size yet.
func ShapeOf(t *gfx.Transform, c *Collider) (sh Shape, ok bool) {
	switch c.Shape {
	case ShapeRect, ShapeCircle, ShapeAnimBody:
		return makeShape(ecs.Entity{}, t, c, nil)
	}
	return Shape{}, false
}

// rectShape is the shape of a tile cell.
func rectShape(r gfx.Rect) Shape {
	return Shape{Kind: ShapeRect, Bounds: r, Center: r.Center()}
}

// intersects is [gfx.Rect.Overlaps], except that an axis on which q has zero size
// matches when q's coordinate lies in [a.Min, a.Max): a point query hits a
// box as gfx.Rect.Contains says.
func intersects(a, q gfx.Rect) bool {
	return axisHit(a.Min.X, a.Max.X, q.Min.X, q.Max.X) && axisHit(a.Min.Y, a.Max.Y, q.Min.Y, q.Max.Y)
}

func axisHit(a0, a1, q0, q1 float32) bool {
	if q0 == q1 {
		return a0 <= q0 && q0 < a1
	}
	return a0 < q1 && q0 < a1
}
