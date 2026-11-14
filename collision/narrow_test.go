package collision

import (
	"testing"

	"github.com/weakpixel/ecs"
	"github.com/weakpixel/engine/gfx"
)

func v(x, y float32) gfx.Vec2 { return gfx.Vec2{X: x, Y: y} }

func rect(x0, y0, x1, y1 float32) gfx.Rect { return gfx.Rect{Min: v(x0, y0), Max: v(x1, y1)} }

var noCustom = &narrow{}

func shapeAt(pos gfx.Vec2, c Collider) Shape {
	sh, _ := makeShape(ecs.Entity{}, &gfx.Transform{Pos: pos}, &c, noCustom)
	return sh
}

func TestBounds(t *testing.T) {
	tests := []struct {
		name   string
		t      gfx.Transform
		c      Collider
		want   gfx.Rect
		radius float32
	}{
		{"rect", gfx.Transform{Pos: v(10, 20)}, Collider{Offset: v(-4, -13), Size: v(8, 13)}, rect(6, 7, 14, 20), 0},
		{"rect scaled", gfx.Transform{Pos: v(10, 20), Scale: v(2, 2)}, Collider{Offset: v(-4, -13), Size: v(8, 13)}, rect(2, -6, 18, 20), 0},
		{"rect mirrored", gfx.Transform{Pos: v(10, 0), Scale: v(-1, 1)}, Collider{Offset: v(2, 0), Size: v(4, 4)}, rect(4, 0, 8, 4), 0},
		{"rotation ignored", gfx.Transform{Pos: v(0, 0), Rot: 1}, Collider{Size: v(4, 2)}, rect(0, 0, 4, 2), 0},
		{"circle", gfx.Transform{Pos: v(5, 5)}, Collider{Shape: ShapeCircle, Offset: v(1, 0), Size: v(3, 0)}, rect(3, 2, 9, 8), 3},
		{"circle mirrored", gfx.Transform{Pos: v(5, 5), Scale: v(-2, 2)}, Collider{Shape: ShapeCircle, Offset: v(1, 0), Size: v(1, 0)}, rect(1, 3, 5, 7), 2},
		{"anim body", gfx.Transform{Pos: v(10, 20)}, Collider{Shape: ShapeAnimBody, Offset: v(-4, -13), Size: v(8, 13)}, rect(6, 7, 14, 20), 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			sh, ok := makeShape(ecs.Entity{}, &tc.t, &tc.c, noCustom)
			if !ok {
				t.Fatal("shape skipped")
			}
			if sh.Bounds != tc.want || sh.Radius != tc.radius {
				t.Errorf("bounds %v radius %v, want %v %v", sh.Bounds, sh.Radius, tc.want, tc.radius)
			}
			if tc.c.Shape == ShapeAnimBody && sh.Kind != ShapeRect {
				t.Errorf("anim body kind %v, want rect", sh.Kind)
			}
		})
	}
	if _, ok := makeShape(ecs.Entity{}, &gfx.Transform{}, &Collider{Shape: ShapeAnimBody}, noCustom); ok {
		t.Error("zero-size anim body should be skipped")
	}
	mustPanic(t, "collision: unknown shape kind custom(200)", func() {
		makeShape(ecs.Entity{}, &gfx.Transform{}, &Collider{Shape: 200}, noCustom)
	})
}

func TestNarrowPhase(t *testing.T) {
	box := func(x, y, w, h float32) Shape { return shapeAt(v(x, y), Collider{Size: v(w, h)}) }
	circle := func(x, y, r float32) Shape { return shapeAt(v(x, y), Collider{Shape: ShapeCircle, Size: v(r, 0)}) }
	tests := []struct {
		name        string
		self, other Shape
		ok          bool
		normal      gfx.Vec2
		depth       float32
	}{
		{"rects touching", box(0, 0, 10, 10), box(10, 0, 10, 10), false, gfx.Vec2{}, 0},
		{"rect left of rect", box(0, 0, 10, 10), box(8, 0, 10, 10), true, v(-1, 0), 2},
		{"rect right of rect", box(8, 0, 10, 10), box(0, 0, 10, 10), true, v(1, 0), 2},
		{"rect on floor", box(0, 0, 10, 10), box(-20, 9, 50, 10), true, v(0, -1), 1},
		{"circles touching", circle(0, 0, 2), circle(4, 0, 2), false, gfx.Vec2{}, 0},
		{"circles", circle(0, 0, 2), circle(3, 0, 2), true, v(-1, 0), 1},
		{"circles same center", circle(0, 0, 2), circle(0, 0, 1), true, v(0, -1), 3},
		{"circle above rect", circle(5, -1, 2), box(0, 0, 10, 10), true, v(0, -1), 1},
		{"circle touching rect", circle(5, -2, 2), box(0, 0, 10, 10), false, gfx.Vec2{}, 0},
		{"circle inside rect", circle(9, 5, 2), box(0, 0, 10, 10), true, v(1, 0), 3},
		{"rect under circle", box(0, 0, 10, 10), circle(5, -1, 2), true, v(0, 1), 1},
	}
	var n narrow
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			nrm, d, ok := n.test(&tc.self, &tc.other)
			if ok != tc.ok || nrm != tc.normal || d != tc.depth {
				t.Errorf("got %v %v %v, want %v %v %v", nrm, d, ok, tc.normal, tc.depth, tc.ok)
			}
		})
	}
}

func TestNarrowPhaseMissingPair(t *testing.T) {
	var n narrow
	a, b := Shape{Kind: ShapeCustom}, Shape{Kind: ShapeRect}
	mustPanic(t, "collision: no narrow phase for shapes custom(128) and rect", func() { n.test(&a, &b) })
}

func TestShapeOf(t *testing.T) {
	tr := &gfx.Transform{Pos: gfx.Vec2{X: 10, Y: 20}, Scale: gfx.Vec2{X: 2, Y: 1}}
	sh, ok := ShapeOf(tr, &Collider{Shape: ShapeCircle, Offset: gfx.Vec2{X: 1, Y: 1}, Size: gfx.Vec2{X: 3}})
	if !ok || sh.Kind != ShapeCircle || sh.Center != (gfx.Vec2{X: 12, Y: 21}) || sh.Radius != 6 {
		t.Errorf("circle: %+v, %v", sh, ok)
	}
	sh, ok = ShapeOf(tr, &Collider{Shape: ShapeAnimBody, Size: gfx.Vec2{X: 4, Y: 2}})
	if !ok || sh.Kind != ShapeRect || sh.Bounds.Max != (gfx.Vec2{X: 18, Y: 22}) {
		t.Errorf("anim body: %+v, %v", sh, ok)
	}
	if _, ok := ShapeOf(tr, &Collider{Shape: ShapeAnimBody}); ok {
		t.Error("anim body without a size has a shape")
	}
	if _, ok := ShapeOf(tr, &Collider{Shape: ShapeKind(99)}); ok {
		t.Error("custom kind has a shape")
	}
}
