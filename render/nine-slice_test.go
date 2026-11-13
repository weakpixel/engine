package render

import (
	"image"
	"testing"

	"github.com/weakpixel/engine/gfx"
)

type piece struct {
	src image.Rectangle
	dst gfx.Rect
}

func slices9(frame image.Point, in gfx.Insets, size gfx.Vec2, tile bool) []piece {
	var out []piece
	nineSlice(frame, in, size, tile, func(src image.Rectangle, dst gfx.Rect) { out = append(out, piece{src, dst}) })
	return out
}

func rect(x0, y0, x1, y1 float32) gfx.Rect {
	return gfx.Rect{Min: gfx.Vec2{X: x0, Y: y0}, Max: gfx.Vec2{X: x1, Y: y1}}
}

func TestNineSlice(t *testing.T) {
	frame := image.Pt(24, 24)
	in := gfx.Insets{Left: 6, Top: 6, Right: 6, Bottom: 6}

	t.Run("stretch", func(t *testing.T) {
		got := slices9(frame, in, gfx.Vec2{X: 100, Y: 50}, false)
		if len(got) != 9 {
			t.Fatalf("%d pieces, want 9", len(got))
		}
		// Row-major: top-left corner, top edge, top-right corner, ...
		want := []piece{
			{image.Rect(0, 0, 6, 6), rect(0, 0, 6, 6)},
			{image.Rect(6, 0, 18, 6), rect(6, 0, 94, 6)},
			{image.Rect(18, 0, 24, 6), rect(94, 0, 100, 6)},
			{image.Rect(0, 6, 6, 18), rect(0, 6, 6, 44)},
			{image.Rect(6, 6, 18, 18), rect(6, 6, 94, 44)},
			{image.Rect(18, 6, 24, 18), rect(94, 6, 100, 44)},
			{image.Rect(0, 18, 6, 24), rect(0, 44, 6, 50)},
			{image.Rect(6, 18, 18, 24), rect(6, 44, 94, 50)},
			{image.Rect(18, 18, 24, 24), rect(94, 44, 100, 50)},
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("piece %d = %+v, want %+v", i, got[i], want[i])
			}
		}
	})
	t.Run("zero size is the frame size", func(t *testing.T) {
		got := slices9(frame, in, gfx.Vec2{}, false)
		for i, p := range got {
			if sw := float32(p.src.Dx()); p.dst.Max.X-p.dst.Min.X != sw {
				t.Errorf("piece %d stretched: %+v", i, p)
			}
		}
	})
	t.Run("zero insets is one piece", func(t *testing.T) {
		got := slices9(frame, gfx.Insets{}, gfx.Vec2{X: 48, Y: 12}, false)
		if len(got) != 1 || got[0].src != image.Rect(0, 0, 24, 24) || got[0].dst != rect(0, 0, 48, 12) {
			t.Fatalf("got %+v", got)
		}
	})
	t.Run("insets clamp to the frame", func(t *testing.T) {
		got := slices9(frame, gfx.Insets{Left: 20, Right: 20}, gfx.Vec2{X: 48, Y: 24}, false)
		// left takes 20, right the remaining 4; the center is empty.
		if len(got) != 2 || got[0].src != image.Rect(0, 0, 20, 24) || got[1].src != image.Rect(20, 0, 24, 24) {
			t.Fatalf("got %+v", got)
		}
	})
	t.Run("small size shrinks the borders", func(t *testing.T) {
		got := slices9(frame, in, gfx.Vec2{X: 6, Y: 24}, false)
		var covered float32
		for _, p := range got {
			if p.dst.Max.X > 6 || p.dst.Min.X < 0 {
				t.Errorf("piece outside the panel: %+v", p)
			}
			if p.src.Min.Y == 6 { // the middle row: every column once
				covered += p.dst.Max.X - p.dst.Min.X
			}
		}
		if covered != 6 {
			t.Fatalf("middle row covers %v, want 6", covered)
		}
	})
	t.Run("tile repeats and crops", func(t *testing.T) {
		got := slices9(image.Pt(8, 8), gfx.Insets{Left: 2, Top: 2, Right: 2, Bottom: 2}, gfx.Vec2{X: 17, Y: 8}, true)
		// The center is 17-4 = 13 px wide: three 4 px tiles and a 1 px crop.
		var centerW []float32
		for _, p := range got {
			if p.src.Min.X == 2 && p.src.Min.Y == 2 {
				centerW = append(centerW, p.dst.Max.X-p.dst.Min.X)
			}
		}
		want := []float32{4, 4, 4, 1}
		if len(centerW) != len(want) {
			t.Fatalf("center tiles %v, want %v", centerW, want)
		}
		for i := range want {
			if centerW[i] != want[i] {
				t.Fatalf("center tiles %v, want %v", centerW, want)
			}
		}
	})
	t.Run("no allocations", func(t *testing.T) {
		n := 0
		allocs := testing.AllocsPerRun(100, func() {
			nineSlice(frame, in, gfx.Vec2{X: 100, Y: 50}, true, func(image.Rectangle, gfx.Rect) { n++ })
		})
		if allocs != 0 {
			t.Fatalf("%v allocs, want 0", allocs)
		}
	})
}
