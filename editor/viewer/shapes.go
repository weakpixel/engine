package viewer

import (
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/weakpixel/engine/editor/toolshell"
	"github.com/weakpixel/engine/editor/viewkit"
	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/tiled"
	"github.com/weakpixel/engine/xmath"
)

// ellipseSegments is how many segments an ellipse outline has.
const ellipseSegments = 24

// pointArm is the half size in device-independent pixels of the cross drawn
// for a point object.
const pointArm = 4

// hitPx is how close in device-independent pixels the mouse must be to a
// polyline or a point to hit it: a little more than pointArm, so the whole
// cross picks its point.
const hitPx = 5

// poly is one outline in world coordinates.
type poly struct {
	pts    []gfx.Vec2
	closed bool
}

// localBox is the box of a rectangle-like object in its own frame, before
// rotation around the anchor: the anchor is the top-left corner, except for a
// tile object, whose anchor is the bottom-left corner.
func localBox(o *tiled.Object) gfx.Rect {
	w, h := float32(o.Width), float32(o.Height)
	if o.Shape == tiled.ShapeTile {
		return gfx.Rect{Min: gfx.Vec2{Y: -h}, Max: gfx.Vec2{X: w}}
	}
	return gfx.Rect{Max: gfx.Vec2{X: w, Y: h}}
}

// outlines returns the outline of o in world coordinates, where anchor is the
// world position of the object's X, Y (see tilemap.ObjectPos). A point has
// none: it is drawn as a cross (see [drawObject]).
func outlines(o *tiled.Object, anchor gfx.Vec2) []poly {
	place := func(local ...gfx.Vec2) []gfx.Vec2 {
		out := make([]gfx.Vec2, len(local))
		for i, p := range local {
			out[i] = anchor.Add(p.Rotate(xmath.Rad(float32(o.Rotation))))
		}
		return out
	}
	switch o.Shape {
	case tiled.ShapePoint:
		return nil
	case tiled.ShapePolygon, tiled.ShapePolyline:
		local := make([]gfx.Vec2, len(o.Points))
		for i, p := range o.Points {
			local[i] = gfx.Vec2{X: float32(p.X), Y: float32(p.Y)}
		}
		return []poly{{pts: place(local...), closed: o.Shape == tiled.ShapePolygon}}
	case tiled.ShapeEllipse:
		b := localBox(o)
		c, r := b.Min.Add(b.Max).Mul(0.5), b.Max.Sub(b.Min).Mul(0.5)
		local := make([]gfx.Vec2, ellipseSegments)
		for i := range local {
			sin, cos := math.Sincos(2 * math.Pi * float64(i) / ellipseSegments)
			local[i] = gfx.Vec2{X: c.X + r.X*float32(cos), Y: c.Y + r.Y*float32(sin)}
		}
		return []poly{{pts: place(local...), closed: true}}
	default: // rectangle, tile and text
		b := localBox(o)
		return []poly{{pts: place(b.Min, gfx.Vec2{X: b.Max.X, Y: b.Min.Y}, b.Max, gfx.Vec2{X: b.Min.X, Y: b.Max.Y}), closed: true}}
	}
}

// hit reports whether world position p hits o, which is anchored at anchor.
// tol is the reach in world units for the shapes that have no area (points,
// polylines) and the slack of zero-size rectangles. Rectangles, ellipses,
// tile objects and text hit by their box, polygons by point-in-polygon.
func hit(o *tiled.Object, anchor, p gfx.Vec2, tol float32) bool {
	local := p.Sub(anchor).Rotate(xmath.Rad(float32(-o.Rotation)))
	switch o.Shape {
	case tiled.ShapePoint:
		return math.Hypot(float64(local.X), float64(local.Y)) <= float64(tol)
	case tiled.ShapePolygon:
		return inPolygon(o.Points, local)
	case tiled.ShapePolyline:
		for i := 1; i < len(o.Points); i++ {
			if segmentDistance(o.Points[i-1], o.Points[i], local) <= float64(tol) {
				return true
			}
		}
		return false
	default:
		b := localBox(o)
		return b.Grow(tol).Contains(local)
	}
}

// inPolygon is the even-odd test of p against the polygon pts.
func inPolygon(pts []tiled.Point, p gfx.Vec2) bool {
	in := false
	px, py := float64(p.X), float64(p.Y)
	for i, j := 0, len(pts)-1; i < len(pts); j, i = i, i+1 {
		a, b := pts[i], pts[j]
		if (a.Y > py) != (b.Y > py) && px < (b.X-a.X)*(py-a.Y)/(b.Y-a.Y)+a.X {
			in = !in
		}
	}
	return in
}

// segmentDistance is the distance from p to the segment a-b.
func segmentDistance(a, b tiled.Point, p gfx.Vec2) float64 {
	px, py := float64(p.X)-a.X, float64(p.Y)-a.Y
	dx, dy := b.X-a.X, b.Y-a.Y
	t := 0.0
	if l2 := dx*dx + dy*dy; l2 > 0 {
		t = max(0, min(1, (px*dx+py*dy)/l2))
	}
	return math.Hypot(px-t*dx, py-t*dy)
}

// drawObject strokes o, anchored at world position anchor, in window pixels.
func drawObject(s *toolshell.Shell, screen ebiten.FinalScreen, o *tiled.Object, anchor gfx.Vec2, c color.RGBA, width float32) {
	if o.Shape == tiled.ShapePoint {
		p := s.WindowPoint(anchor)
		arm := pointArm * float32(viewkit.DeviceScale())
		s.StrokeLine(screen, p.X-arm, p.Y, p.X+arm, p.Y, width, c)
		s.StrokeLine(screen, p.X, p.Y-arm, p.X, p.Y+arm, width, c)
		return
	}
	for _, pl := range outlines(o, anchor) {
		pts := make([]gfx.Vec2, len(pl.pts))
		for i, p := range pl.pts {
			pts[i] = s.WindowPoint(p)
		}
		s.StrokePolyline(screen, pts, pl.closed, width, c)
	}
}
