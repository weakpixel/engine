package animedit

import (
	"reflect"
	"testing"

	"github.com/weakpixel/engine/anim/animfile"
	"github.com/weakpixel/engine/gfx"
)

// The boxes the editor draws are the boxes the loader hands out: for every
// frame of every clip, the unreplaced ActiveBoxes equal Resolve's.
func TestActiveBoxesEqualResolve(t *testing.T) {
	f, sheet := heroFile(t), heroSheet(t)
	// A second hurt shape shows the replace rule on more than one frame, and
	// an all-frames hit shape the nil range.
	if _, err := AddBox(f, "run", KindHit, 0, animfile.Rect{X: 9, Y: 9, W: 1, H: 1}); err != nil {
		t.Fatal(err)
	}
	if err := SetShapeFrames(f, "run", BoxRef{Index: 2}, true, 0, nil); err != nil {
		t.Fatal(err)
	}
	res, err := animfile.Resolve(f, sheet.PlayOrder())
	if err != nil {
		t.Fatal(err)
	}
	sawReplaced := false
	for _, rc := range res.Clips {
		n := len(rc.Durations)
		for frame := range n {
			var got []animfile.Box
			for _, b := range ActiveBoxes(f, rc.Name, frame, n) {
				if b.Replaced {
					sawReplaced = true
					continue
				}
				kind := animfile.BoxHurt
				if b.Kind == KindHit {
					kind = animfile.BoxHit
				}
				got = append(got, animfile.Box{Kind: kind, ID: b.ID, Rect: b.Rect})
			}
			var want []animfile.Box
			if rc.Boxes != nil {
				want = rc.Boxes[frame]
			}
			if !reflect.DeepEqual(got, want) {
				t.Errorf("clip %q frame %d: boxes %v, want %v", rc.Name, frame, got, want)
			}
		}
	}
	if !sawReplaced {
		t.Error("the fixture never replaced a body box")
	}
}

func TestPickOrder(t *testing.T) {
	f := heroFile(t)
	boxes := ActiveBoxes(f, "run", 1, 4) // body + slash (frames 1..2)
	if len(boxes) != 2 {
		t.Fatalf("boxes = %v", boxes)
	}
	// Slash covers x 1..5, y -3..-1; body covers x -1..1, y -3..0.
	if ref, ok := Pick(boxes, gfx.Vec2{X: 2, Y: -2}); !ok || ref != (BoxRef{Index: 0}) {
		t.Errorf("pick in the slash = %+v, %v", ref, ok)
	}
	if ref, ok := Pick(boxes, gfx.Vec2{X: 0, Y: -1}); !ok || !ref.Body {
		t.Errorf("pick in the body = %+v, %v", ref, ok)
	}
	if _, ok := Pick(boxes, gfx.Vec2{X: 50, Y: 50}); ok {
		t.Error("picked empty space")
	}
	// On frame 3 the hurt shape replaces the body: the body is not pickable.
	boxes = ActiveBoxes(f, "run", 3, 4)
	if !boxes[0].Replaced {
		t.Fatalf("body not replaced on frame 3: %v", boxes)
	}
	if ref, ok := Pick(boxes, gfx.Vec2{X: -0.5, Y: -2.5}); ok && ref.Body {
		t.Error("picked a replaced body box")
	}
}

func TestHandleAt(t *testing.T) {
	r := animfile.Rect{X: 10, Y: 20, W: 8, H: 4}
	tests := []struct {
		p    gfx.Vec2
		want Handle
	}{
		{gfx.Vec2{X: 10, Y: 20}, HandleNW}, {gfx.Vec2{X: 14, Y: 20}, HandleN}, {gfx.Vec2{X: 18, Y: 20}, HandleNE},
		{gfx.Vec2{X: 18, Y: 22}, HandleE}, {gfx.Vec2{X: 18, Y: 24}, HandleSE}, {gfx.Vec2{X: 14, Y: 24}, HandleS},
		{gfx.Vec2{X: 10, Y: 24}, HandleSW}, {gfx.Vec2{X: 10, Y: 22}, HandleW},
		{gfx.Vec2{X: 10.5, Y: 20.5}, HandleNW}, {gfx.Vec2{X: 14, Y: 22}, HandleNone}, {gfx.Vec2{X: 30, Y: 30}, HandleNone},
	}
	for _, tc := range tests {
		if got := HandleAt(r, tc.p, 1); got != tc.want {
			t.Errorf("HandleAt(%v) = %v, want %v", tc.p, got, tc.want)
		}
	}
}

func TestResizeRect(t *testing.T) {
	r := animfile.Rect{X: 0, Y: 0, W: 10, H: 10}
	tests := []struct {
		name   string
		h      Handle
		dx, dy int32
		want   animfile.Rect
	}{
		{"se grows", HandleSE, 3, 2, animfile.Rect{W: 13, H: 12}},
		{"nw moves origin", HandleNW, 2, 3, animfile.Rect{X: 2, Y: 3, W: 8, H: 7}},
		{"e only x", HandleE, 4, 9, animfile.Rect{W: 14, H: 10}},
		{"n only y", HandleN, 9, -4, animfile.Rect{Y: -4, W: 10, H: 14}},
		{"w past the edge stops at 1", HandleW, 50, 0, animfile.Rect{X: 9, W: 1, H: 10}},
		{"s past the edge stops at 1", HandleS, 0, -50, animfile.Rect{W: 10, H: 1}},
		{"none", HandleNone, 5, 5, r},
	}
	for _, tc := range tests {
		if got := ResizeRect(r, tc.h, tc.dx, tc.dy); got != tc.want {
			t.Errorf("%s: %+v, want %+v", tc.name, got, tc.want)
		}
	}
	if got := MoveRect(r, -2, 5); got != (animfile.Rect{X: -2, Y: 5, W: 10, H: 10}) {
		t.Errorf("MoveRect = %+v", got)
	}
}

func TestRectBetween(t *testing.T) {
	if got := RectBetween(gfx.Vec2{X: 5.4, Y: 7.6}, gfx.Vec2{X: 1.2, Y: 2.1}); got != (animfile.Rect{X: 1, Y: 2, W: 4, H: 6}) {
		t.Errorf("RectBetween = %+v", got)
	}
	if got := RectBetween(gfx.Vec2{X: 5.1, Y: 5.1}, gfx.Vec2{X: 5.3, Y: 5.2}); got.W != 0 || got.H != 0 {
		t.Errorf("a click is %+v, want an empty rect", got)
	}
}

func TestFlip(t *testing.T) {
	r := animfile.Rect{X: 2, Y: -3, W: 5, H: 2}
	if got := Flip(r); got != (animfile.Rect{X: -7, Y: -3, W: 5, H: 2}) {
		t.Errorf("Flip = %+v", got)
	}
	if Flip(Flip(r)) != r {
		t.Error("Flip is not its own inverse")
	}
}
