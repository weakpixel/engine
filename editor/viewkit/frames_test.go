package viewkit

import (
	"testing"

	"github.com/weakpixel/engine/gfx"
)

func TestLabelMode(t *testing.T) {
	f := Frame{Index: 3, Name: "run_3"}
	grid := Frame{Index: 3, Name: "3"}
	for _, c := range []struct {
		mode LabelMode
		f    Frame
		want string
	}{
		{LabelBoth, f, "3 run_3"}, {LabelBoth, grid, "3"}, {LabelIndex, f, "3"},
		{LabelName, f, "run_3"}, {LabelName, Frame{Index: 2}, "2"}, {LabelNone, f, ""},
	} {
		if got := c.mode.Text(c.f); got != c.want {
			t.Errorf("mode %v text(%+v) = %q, want %q", c.mode, c.f, got, c.want)
		}
	}
}

func TestFrameAtPrefersTheSmallerFrame(t *testing.T) {
	r := func(x0, y0, x1, y1 float32) gfx.Rect {
		return gfx.Rect{Min: gfx.Vec2{X: x0, Y: y0}, Max: gfx.Vec2{X: x1, Y: y1}}
	}
	frames := []Frame{{Rect: r(0, 0, 32, 32)}, {Rect: r(8, 8, 16, 16)}}
	for _, c := range []struct {
		p    gfx.Vec2
		want int
	}{
		{gfx.Vec2{X: 10, Y: 10}, 1}, {gfx.Vec2{X: 1, Y: 1}, 0}, {gfx.Vec2{X: 32, Y: 1}, -1}, {gfx.Vec2{X: -1, Y: 0}, -1},
	} {
		if got := FrameAt(frames, c.p); got != c.want {
			t.Errorf("FrameAt(%v) = %d, want %d", c.p, got, c.want)
		}
	}
}
