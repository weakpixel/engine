package hierarchy

import (
	"math"
	"testing"

	"github.com/weakpixel/engine/gfx"
)

func near(a, b float32) bool { return math.Abs(float64(a-b)) < 1e-4 }

func nearT(a, b gfx.Transform) bool {
	return near(a.Pos.X, b.Pos.X) && near(a.Pos.Y, b.Pos.Y) && near(a.Rot, b.Rot) &&
		near(a.Scale.X, b.Scale.X) && near(a.Scale.Y, b.Scale.Y)
}

func v(x, y float32) gfx.Vec2 { return gfx.Vec2{X: x, Y: y} }

func TestCompose(t *testing.T) {
	half := float32(math.Pi / 2)
	tests := []struct {
		name   string
		parent gfx.Transform
		local  Local
		want   gfx.Transform
	}{
		{"offset only", gfx.Transform{Pos: v(10, 20)}, Local{Pos: v(1, 2)},
			gfx.Transform{Pos: v(11, 22), Scale: v(1, 1)}},
		{"rotated parent", gfx.Transform{Pos: v(10, 0), Rot: half}, Local{Pos: v(1, 0), Rot: 0.5},
			gfx.Transform{Pos: v(10, 1), Rot: half + 0.5, Scale: v(1, 1)}},
		{"scaled parent", gfx.Transform{Scale: v(2, 3)}, Local{Pos: v(1, 1), Scale: v(2, 2)},
			gfx.Transform{Pos: v(2, 3), Scale: v(4, 6)}},
		{"zero scale is one", gfx.Transform{Pos: v(1, 1)}, Local{Pos: v(1, 1)},
			gfx.Transform{Pos: v(2, 2), Scale: v(1, 1)}},
		{"mirrored parent", gfx.Transform{Pos: v(5, 0), Rot: 1, Scale: v(-1, 1)}, Local{Pos: v(2, 0), Rot: 0.25},
			gfx.Transform{Pos: v(5, 0).Add(v(-2, 0).Rotate(1)), Rot: 0.75, Scale: v(-1, 1)}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Compose(tt.parent, tt.local); !nearT(got, tt.want) {
				t.Fatalf("compose = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestLocalOf(t *testing.T) {
	parents := []gfx.Transform{
		{},
		{Pos: v(3, -4), Rot: 0.7},
		{Pos: v(1, 2), Rot: -1.1, Scale: v(2, 0.5)},
		{Pos: v(-5, 5), Rot: 2, Scale: v(-1, 1)},
		{Pos: v(0, 1), Rot: 0.3, Scale: v(-2, -2)},
	}
	worlds := []gfx.Transform{
		{Pos: v(10, 20), Rot: 0.2, Scale: v(1, 1)},
		{Pos: v(-3, 8), Rot: -2, Scale: v(3, 0.25)},
		{Pos: v(0, 0)},
	}
	for _, p := range parents {
		for _, w := range worlds {
			got := Compose(p, LocalOf(p, w))
			want := w
			want.Scale = scaleOrOne(w.Scale)
			if !nearT(got, want) {
				t.Errorf("parent %+v world %+v: round trip %+v", p, w, got)
			}
		}
	}
}
