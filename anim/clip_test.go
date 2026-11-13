package anim

import (
	"slices"
	"testing"
	"time"

	"github.com/weakpixel/engine/gfx"
)

const ms = time.Millisecond

// clipOf builds a clip with the given frame durations in ms.
func clipOf(mode LoopMode, durs ...int) *Clip {
	c := &Clip{Loop: mode}
	for _, d := range durs {
		c.Frames = append(c.Frames, Frame{Duration: time.Duration(d) * ms})
		c.Total += time.Duration(d) * ms
	}
	return c
}

func TestFrameAt(t *testing.T) {
	type step struct {
		at    int // ms
		frame int
		done  bool
	}
	tests := []struct {
		name  string
		clip  *Clip
		steps []step
	}{
		{"loop uneven", clipOf(Loop, 100, 200, 100), []step{
			{0, 0, false}, {99, 0, false}, {100, 1, false}, {299, 1, false}, {300, 2, false},
			{399, 2, false}, {400, 0, false}, {650, 1, false}, {1_000_000_000, 0, false},
		}},
		{"once stops on last", clipOf(Once, 100, 200), []step{
			{0, 0, false}, {100, 1, false}, {299, 1, false}, {300, 1, true}, {5000, 1, true},
		}},
		{"pingpong 4 frames", clipOf(PingPong, 100, 100, 100, 100), []step{
			{0, 0, false}, {100, 1, false}, {200, 2, false}, {300, 3, false},
			{400, 2, false}, {500, 1, false}, {600, 0, false}, {699, 0, false}, {700, 1, false},
		}},
		{"pingpong uneven", clipOf(PingPong, 50, 100, 200, 50), []step{
			{0, 0, false}, {50, 1, false}, {150, 2, false}, {350, 3, false},
			{400, 2, false}, {599, 2, false}, {600, 1, false}, {699, 1, false},
			// period = 2*400 - 50 - 50 = 700
			{700, 0, false},
		}},
		{"pingpong two frames", clipOf(PingPong, 100, 100), []step{
			{0, 0, false}, {100, 1, false}, {200, 0, false}, {300, 1, false},
		}},
		{"single frame loop", clipOf(Loop, 80), []step{{0, 0, false}, {1000, 0, false}}},
		{"single frame once", clipOf(Once, 80), []step{{0, 0, false}, {80, 0, true}}},
		{"single frame pingpong", clipOf(PingPong, 80), []step{{0, 0, false}, {1000, 0, false}}},
		{"negative time", clipOf(Loop, 100, 100), []step{{-50, 0, false}}},
		{"empty", &Clip{}, []step{{0, 0, true}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, s := range tt.steps {
				f, done := tt.clip.FrameAt(time.Duration(s.at) * ms)
				if f != s.frame || done != s.done {
					t.Errorf("FrameAt(%dms) = (%d, %v), want (%d, %v)", s.at, f, done, s.frame, s.done)
				}
			}
		})
	}
}

func TestFrameAtNoAllocs(t *testing.T) {
	c := clipOf(PingPong, 100, 100, 100, 100)
	var sink int
	if n := testing.AllocsPerRun(100, func() { f, _ := c.FrameAt(450 * ms); sink += f }); n != 0 {
		t.Fatalf("FrameAt allocs/op = %v, want 0", n)
	}
	_ = sink
}

func TestShapesAt(t *testing.T) {
	hit := gfx.Box{Kind: gfx.BoxHit, ID: "slash", Rect: gfx.IRect{X: 1, Y: 2, W: 3, H: 4}}
	hurt := gfx.Box{Kind: gfx.BoxHurt, Rect: gfx.IRect{X: -4, Y: -13, W: 8, H: 13}}
	boxes := []gfx.Box{hurt, hurt, hit}
	c := clipOf(Once, 100, 100)
	c.Frames[0].Boxes = boxes[0:1]
	c.Frames[1].Boxes = boxes[1:3]
	tests := []struct {
		frame int
		want  []gfx.Box
	}{
		{-1, nil},
		{0, []gfx.Box{hurt}},
		{1, []gfx.Box{hurt, hit}},
		{2, nil},
	}
	for _, tt := range tests {
		if got := c.ShapesAt(tt.frame); !slices.Equal(got, tt.want) {
			t.Errorf("ShapesAt(%d) = %v, want %v", tt.frame, got, tt.want)
		}
	}
	if got := clipOf(Loop, 100).ShapesAt(0); got != nil {
		t.Errorf("clip without boxes: ShapesAt(0) = %v, want nil", got)
	}
	var sink int
	if n := testing.AllocsPerRun(100, func() { sink += len(c.ShapesAt(1)) }); n != 0 {
		t.Fatalf("ShapesAt allocs/op = %v, want 0", n)
	}
	_ = sink
}
