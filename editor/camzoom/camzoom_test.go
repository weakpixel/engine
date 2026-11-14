package camzoom

import (
	"image"
	"math"
	"testing"

	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/render"
)

var screen = image.Pt(320, 180)

func near(a, b float32) bool { return math.Abs(float64(a-b)) < 1e-3 }

func TestEaseKeepsCursorPoint(t *testing.T) {
	cur := gfx.Vec2{X: 250, Y: 40}
	cam := render.Camera{Pos: gfx.Vec2{X: 10, Y: -5}, Zoom: 1}
	under := cam.ScreenToWorld(cur, screen)
	var z Zoom
	z.Wheel(&cam, 2)
	want := float32(math.Pow(Step, 2))
	prev := cam.Zoom
	for i := range 200 {
		z.Update(&cam, cur, screen)
		if i == 0 && (cam.Zoom <= prev || cam.Zoom >= want) {
			t.Fatalf("first tick zoom = %v, want between %v and %v (eased)", cam.Zoom, prev, want)
		}
		if cam.Zoom < prev {
			t.Fatalf("zoom went back from %v to %v", prev, cam.Zoom)
		}
		prev = cam.Zoom
		if got := cam.ScreenToWorld(cur, screen); !near(got.X, under.X) || !near(got.Y, under.Y) {
			t.Fatalf("tick %d: world under the cursor moved from %v to %v", i, under, got)
		}
	}
	if cam.Zoom != want {
		t.Errorf("zoom = %v, want to settle at %v", cam.Zoom, want)
	}
}

func TestSmallDeltasAreSmall(t *testing.T) {
	cam := render.Camera{Zoom: 1}
	var z Zoom
	z.Wheel(&cam, 0.05) // a trackpad tick
	for range 100 {
		z.Update(&cam, gfx.Vec2{}, screen)
	}
	if want := float32(math.Pow(Step, 0.05)); !near(cam.Zoom, want) || cam.Zoom > 1.01 {
		t.Errorf("zoom = %v, want %v", cam.Zoom, want)
	}
}

func TestLimitsAndOtherWriters(t *testing.T) {
	cam := render.Camera{Zoom: 1}
	var z Zoom
	for range 50 {
		z.Wheel(&cam, 100) // clamped per tick
		z.Update(&cam, gfx.Vec2{}, screen)
	}
	if cam.Zoom != Max {
		t.Errorf("zoom = %v, want clamped to %v", cam.Zoom, float32(Max))
	}
	// Something else (framing, the 1 key) sets the zoom: no pull back.
	z.Wheel(&cam, -1)
	cam.Zoom = 2
	z.Update(&cam, gfx.Vec2{}, screen)
	if cam.Zoom != 2 {
		t.Errorf("zoom = %v after an outside write, want 2", cam.Zoom)
	}
	cam.Zoom = 0 // zero means 1
	z.Wheel(&cam, 0)
	z.Update(&cam, gfx.Vec2{}, screen)
	if cam.Zoom != 0 {
		t.Errorf("zoom = %v, want untouched", cam.Zoom)
	}
}
