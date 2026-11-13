package gfx_test

import (
	"testing"

	"github.com/weakpixel/engine/gfx"
)

func TestBoxWorld(t *testing.T) {
	// 10 wide, 4 high, starting 2 right of and 6 above the pivot.
	box := gfx.Box{Kind: gfx.BoxHit, Rect: gfx.IRect{X: 2, Y: -6, W: 10, H: 4}}
	at := gfx.Vec2{X: 100, Y: 50}
	rect := func(x0, y0, x1, y1 float32) gfx.Rect {
		return gfx.Rect{Min: gfx.Vec2{X: x0, Y: y0}, Max: gfx.Vec2{X: x1, Y: y1}}
	}
	tests := []struct {
		name         string
		scale        gfx.Vec2
		flipX, flipY bool
		want         gfx.Rect
	}{
		{"zero scale means 1", gfx.Vec2{}, false, false, rect(102, 44, 112, 48)},
		{"unit scale", gfx.Vec2{X: 1, Y: 1}, false, false, rect(102, 44, 112, 48)},
		{"scale 2", gfx.Vec2{X: 2, Y: 3}, false, false, rect(104, 32, 124, 44)},
		// x' = -(2 + 10) = -12, so the box spans -12..-2 around the pivot.
		{"flipX", gfx.Vec2{}, true, false, rect(88, 44, 98, 48)},
		// y' = -(-6 + 4) = 2: below the pivot.
		{"flipY", gfx.Vec2{}, false, true, rect(102, 52, 112, 56)},
		{"flip both scaled", gfx.Vec2{X: 2, Y: 2}, true, true, rect(76, 54, 96, 62)},
		{"negative scale mirrors like a flip", gfx.Vec2{X: -1, Y: 1}, false, false, rect(88, 44, 98, 48)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tr := gfx.Transform{Pos: at, Scale: tt.scale}
			if got := box.World(&tr, tt.flipX, tt.flipY); got != tt.want {
				t.Fatalf("World = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestBoxWorldNoAllocs(t *testing.T) {
	box := gfx.Box{Kind: gfx.BoxHurt, ID: "body", Rect: gfx.IRect{X: -4, Y: -13, W: 8, H: 13}}
	tr := gfx.Transform{Pos: gfx.Vec2{X: 1, Y: 2}}
	var sink gfx.Rect
	if n := testing.AllocsPerRun(100, func() { sink = box.World(&tr, true, false) }); n != 0 {
		t.Fatalf("World allocs/op = %v, want 0", n)
	}
	_ = sink
}

func TestBoxKindString(t *testing.T) {
	if gfx.BoxHurt.String() != "hurt" || gfx.BoxHit.String() != "hit" {
		t.Fatalf("names = %q, %q", gfx.BoxHurt, gfx.BoxHit)
	}
}
