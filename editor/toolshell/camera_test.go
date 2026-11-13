package toolshell

import (
	"image"
	"testing"

	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/input"
	"github.com/weakpixel/engine/render"
)

var testScreen = image.Pt(320, 180)

func near(a, b float32) bool { d := a - b; return d > -0.001 && d < 0.001 }

func TestZoomAt(t *testing.T) {
	cur := gfx.Vec2{X: 250, Y: 40}
	cam := render.Camera{Pos: gfx.Vec2{X: 10, Y: -5}, Zoom: 1}
	under := cam.ScreenToWorld(cur, testScreen)
	ZoomAt(&cam, cur, testScreen, 2)
	if !near(cam.Zoom, 1.21) {
		t.Errorf("zoom = %v, want 1.1^2", cam.Zoom)
	}
	if got := cam.ScreenToWorld(cur, testScreen); !near(got.X, under.X) || !near(got.Y, under.Y) {
		t.Errorf("world under the cursor moved from %v to %v", under, got)
	}
	ZoomAt(&cam, cur, testScreen, 100)
	if cam.Zoom != maxZoom {
		t.Errorf("zoom = %v, want clamped to %v", cam.Zoom, float32(maxZoom))
	}
	ZoomAt(&cam, cur, testScreen, -1000)
	if cam.Zoom != minZoom {
		t.Errorf("zoom = %v, want clamped to %v", cam.Zoom, float32(minZoom))
	}
}

func TestFrame(t *testing.T) {
	cam := render.Camera{Pos: gfx.Vec2{X: 99, Y: 99}}
	FrameRect(&cam, gfx.Rect{Min: gfx.Vec2{X: 0, Y: 0}, Max: gfx.Vec2{X: 64, Y: 32}}, testScreen, image.Rectangle{})
	if cam.Pos != (gfx.Vec2{X: 32, Y: 16}) {
		t.Errorf("pos = %v, want the rect center", cam.Pos)
	}
	// 320/64 = 5 is the tighter... 180/32 = 5.625, so width limits: 5*0.9.
	if !near(cam.Zoom, 4.5) {
		t.Errorf("zoom = %v, want 4.5", cam.Zoom)
	}
	cam.Viewport = image.Rect(0, 0, 160, 180)
	FrameRect(&cam, gfx.Rect{Max: gfx.Vec2{X: 64, Y: 32}}, testScreen, image.Rectangle{})
	if !near(cam.Zoom, 2.25) {
		t.Errorf("zoom with a half viewport = %v, want 2.25", cam.Zoom)
	}
	cam.Viewport = image.Rectangle{}
	free := image.Rect(40, 0, 200, 180)
	FrameRect(&cam, gfx.Rect{Max: gfx.Vec2{X: 64, Y: 32}}, testScreen, free)
	if c := cam.WorldToScreen(gfx.Vec2{X: 32, Y: 16}, testScreen); !near(c.X, 120) || !near(c.Y, 90) || !near(cam.Zoom, 2.25) {
		t.Errorf("framed in %v: center at %v, zoom %v; want (120, 90), 2.25", free, c, cam.Zoom)
	}
	FrameRect(&cam, gfx.Rect{Max: gfx.Vec2{X: 1e6, Y: 1e6}}, testScreen, image.Rectangle{})
	if cam.Zoom != minZoom {
		t.Errorf("zoom = %v, want clamped to %v", cam.Zoom, float32(minZoom))
	}
}

func TestPan(t *testing.T) {
	var c cameraInput
	cam := render.Camera{Zoom: 2}
	m := input.MouseFrame{X: 100, Y: 100}
	m.SetDown(input.MouseMiddle, true)
	c.update(&cam, testScreen, &m, false) // press: no movement yet
	if cam.Pos != (gfx.Vec2{}) {
		t.Fatalf("pos moved on press: %v", cam.Pos)
	}
	m.X, m.Y = 120, 90
	c.update(&cam, testScreen, &m, false)
	// The cursor moved (+20,-10) screen pixels at zoom 2: the world moves with it.
	if cam.Pos != (gfx.Vec2{X: -10, Y: 5}) {
		t.Errorf("pos = %v, want (-10, 5)", cam.Pos)
	}
	// Left button pans only with the modifier.
	m = input.MouseFrame{X: 0, Y: 0}
	m.SetDown(input.MouseLeft, true)
	c.update(&cam, testScreen, &m, true)
	m.X = 20
	c.update(&cam, testScreen, &m, false)
	if cam.Pos.X != -10 {
		t.Errorf("left drag without a modifier panned: %v", cam.Pos)
	}
	c.update(&cam, testScreen, &m, true)
	m.X = 40
	c.update(&cam, testScreen, &m, true)
	if cam.Pos.X != -20 {
		t.Errorf("left drag with a modifier: pos.X = %v, want -20", cam.Pos.X)
	}
}

func TestCoarseStep(t *testing.T) {
	for _, tt := range []struct{ in, want float32 }{{16, 16}, {4, 4}, {3, 24}, {0.25, 16}, {0.03, 15.36}, {0, 0}} {
		if got := CoarseStep(tt.in); !near(got, tt.want) {
			t.Errorf("CoarseStep(%v) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestGridLines(t *testing.T) {
	if got := GridLines(0, 3, 100); got != nil {
		t.Errorf("a 3 px step drew %d lines, want none", len(got))
	}
	lines := GridLines(10, 20, 100)
	// k = -1..5 would be -10..110; the loop includes the edge lines.
	if len(lines) == 0 || lines[0].At != -10 || lines[len(lines)-1].At != 110 {
		t.Fatalf("lines = %v, want from -10 to 110", lines)
	}
	for _, l := range lines {
		wantMajor := l.At == 10
		if k := (l.At - 10) / 20; k == 0 {
			wantMajor = true
		}
		if l.Major != wantMajor {
			t.Errorf("line at %v: Major = %v, want %v", l.At, l.Major, wantMajor)
		}
	}
	eight := GridLines(0, 10, 200)
	if !eight[8].Major || eight[7].Major || eight[9].Major {
		t.Errorf("major lines are not every 8th: %v", eight[:10])
	}
}
