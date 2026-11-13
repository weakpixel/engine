package render

import (
	"image"
	"math"
	"testing"

	"github.com/weakpixel/engine/gfx"
)

func near(a, b gfx.Vec2) bool {
	return math.Abs(float64(a.X-b.X)) < 1e-3 && math.Abs(float64(a.Y-b.Y)) < 1e-3
}

var cameraCases = []struct {
	name string
	cam  Camera
}{
	{"identity", Camera{}},
	{"moved", Camera{Pos: gfx.Vec2{X: 100, Y: -50}}},
	{"zoomed", Camera{Pos: gfx.Vec2{X: 10, Y: 10}, Zoom: 2.5}},
	{"rotated", Camera{Pos: gfx.Vec2{X: -3, Y: 7}, Zoom: 1.5, Rotation: 0.7}},
	{"viewport", Camera{Zoom: 2, Rotation: -1.2, Viewport: image.Rect(100, 50, 500, 350)}},
}

func TestCameraRoundTrip(t *testing.T) {
	screen := image.Pt(640, 360)
	for _, tc := range cameraCases {
		t.Run(tc.name, func(t *testing.T) {
			for _, p := range []gfx.Vec2{{X: 0, Y: 0}, {X: 12.5, Y: -40}, {X: -300, Y: 999}} {
				got := tc.cam.ScreenToWorld(tc.cam.WorldToScreen(p, screen), screen)
				if !near(got, p) {
					t.Fatalf("round trip of %v = %v", p, got)
				}
			}
		})
	}
}

func TestCameraCenterAndBounds(t *testing.T) {
	screen := image.Pt(640, 360)
	for _, tc := range cameraCases {
		t.Run(tc.name, func(t *testing.T) {
			vp := tc.cam.ViewportRect(screen)
			center := gfx.Vec2{X: float32(vp.Min.X+vp.Max.X) / 2, Y: float32(vp.Min.Y+vp.Max.Y) / 2}
			if got := tc.cam.WorldToScreen(tc.cam.Pos, screen); !near(got, center) {
				t.Fatalf("Pos maps to %v, want the viewport center %v", got, center)
			}
			b := tc.cam.WorldBounds(screen).Grow(0.01)
			for _, c := range []gfx.Vec2{
				{X: float32(vp.Min.X), Y: float32(vp.Min.Y)}, {X: float32(vp.Max.X), Y: float32(vp.Max.Y)},
				{X: float32(vp.Min.X), Y: float32(vp.Max.Y)}, {X: float32(vp.Max.X), Y: float32(vp.Min.Y)},
			} {
				if w := tc.cam.ScreenToWorld(c, screen); !b.Contains(w) {
					t.Fatalf("bounds %v miss screen corner %v (world %v)", b, c, w)
				}
			}
		})
	}
}

func TestCameraWorldBoundsValues(t *testing.T) {
	c := Camera{Pos: gfx.Vec2{X: 100, Y: 100}, Zoom: 2}
	got := c.WorldBounds(image.Pt(200, 100))
	want := gfx.Rect{Min: gfx.Vec2{X: 50, Y: 75}, Max: gfx.Vec2{X: 150, Y: 125}}
	if got != want {
		t.Fatalf("WorldBounds = %v, want %v", got, want)
	}
}

func TestCameraAt(t *testing.T) {
	c := Camera{Pos: gfx.Vec2{X: 10}, PrevPos: gfx.Vec2{X: 0}}
	if got := c.At(0).Pos.X; got != 0 {
		t.Fatalf("At(0) = %v", got)
	}
	if got := c.At(1).Pos.X; got != 10 {
		t.Fatalf("At(1) = %v", got)
	}
	if got := c.At(0.25).Pos.X; got != 2.5 {
		t.Fatalf("At(0.25) = %v", got)
	}
}

// TestCameraPixelSnapped checks that a snapped camera puts the world on
// whole pixels (viewport center - Pos*Zoom is an integer), moves Pos by at
// most half a pixel, clears PixelSnap on the copy, and leaves rotated or
// unsnapped cameras alone.
func TestCameraPixelSnapped(t *testing.T) {
	screen := image.Pt(640, 361) // odd height: the center is on a half pixel
	for _, z := range []float32{1, 1.49, 2.27, 3, 7.3} {
		for i := range 200 {
			p := gfx.Vec2{X: 100 + float32(i)*0.037, Y: -50 + float32(i)*0.053}
			c := Camera{Pos: p, PrevPos: gfx.Vec2{X: 1, Y: 2}, Zoom: z, PixelSnap: true}.pixelSnapped(screen)
			for _, ax := range []struct{ pos, orig, center float32 }{{c.Pos.X, p.X, 320}, {c.Pos.Y, p.Y, 180.5}} {
				off := float64(ax.center) - float64(ax.pos)*float64(z)
				if math.Abs(off-math.Round(off)) > 1e-3 {
					t.Fatalf("zoom %v pos %v: offset %v is not a whole pixel", z, p, off)
				}
				if d := math.Abs(float64(ax.pos-ax.orig)) * float64(z); d > 0.5+1e-3 {
					t.Fatalf("zoom %v pos %v: moved %v pixels", z, p, d)
				}
			}
			if c.PixelSnap || c.PrevPos != (gfx.Vec2{X: 1, Y: 2}) {
				t.Fatalf("snapped copy has PixelSnap %v, PrevPos %v; want false and unchanged", c.PixelSnap, c.PrevPos)
			}
		}
	}
	p := gfx.Vec2{X: 10.3, Y: 4.7}
	for _, c := range []Camera{{Pos: p, Zoom: 1.5}, {Pos: p, Zoom: 1.5, PixelSnap: true, Rotation: 0.1}} {
		if got := c.pixelSnapped(screen); got != c {
			t.Errorf("%+v snapped to %+v, want unchanged", c, got)
		}
	}
}

// TestCameraPixelSnapPicking checks that the camera methods use the snapped
// view: a world point's sub-pixel phase on screen does not depend on the
// camera position, ScreenToWorld inverts WorldToScreen, and WorldBounds is
// the snapped view's rectangle.
func TestCameraPixelSnapPicking(t *testing.T) {
	screen := image.Pt(640, 360)
	frac := func(x float32) float64 { return float64(x) - math.Floor(float64(x)) }
	world := []gfx.Vec2{{X: 0, Y: 0}, {X: 12.3, Y: -40.7}, {X: -301.05, Y: 999.5}}
	for _, z := range []float32{1, 1.5, 3} {
		var phase []gfx.Vec2
		for i := range 50 {
			c := Camera{Pos: gfx.Vec2{X: 200 + float32(i)*0.137, Y: 90 - float32(i)*0.071}, Zoom: z, PixelSnap: true}
			for j, p := range world {
				s := c.WorldToScreen(p, screen)
				if i == 0 {
					phase = append(phase, s)
				} else if math.Abs(frac(s.X)-frac(phase[j].X)) > 1e-3 || math.Abs(frac(s.Y)-frac(phase[j].Y)) > 1e-3 {
					t.Fatalf("zoom %v camera %v: %v drawn at %v, sub-pixel phase differs from %v", z, c.Pos, p, s, phase[j])
				}
				if back := c.ScreenToWorld(s, screen); math.Abs(float64(back.X-p.X)) > 1e-3 || math.Abs(float64(back.Y-p.Y)) > 1e-3 {
					t.Fatalf("zoom %v camera %v: round trip of %v = %v", z, c.Pos, p, back)
				}
			}
			v := c.pixelSnapped(screen)
			if got, want := c.WorldBounds(screen), v.WorldBounds(screen); got != want {
				t.Fatalf("zoom %v camera %v: WorldBounds %v, want the snapped view's %v", z, c.Pos, got, want)
			}
		}
	}
}

// TestCameraDrawBounds checks that DrawBounds is the union of the views at
// PrevPos and Pos, and that it contains the view of every frame the Renderer
// can draw during the tick (blended, and rounded with PixelSnap).
func TestCameraDrawBounds(t *testing.T) {
	screen := image.Pt(200, 100)
	c := Camera{PrevPos: gfx.Vec2{X: 100, Y: 100}, Pos: gfx.Vec2{X: 130, Y: 90}, Zoom: 2}
	want := gfx.Rect{Min: gfx.Vec2{X: 50, Y: 65}, Max: gfx.Vec2{X: 180, Y: 125}}
	if got := c.DrawBounds(screen); got != want {
		t.Fatalf("DrawBounds = %v, want %v", got, want)
	}
	contains := func(outer, inner gfx.Rect) bool {
		const eps = 1e-3
		return inner.Min.X >= outer.Min.X-eps && inner.Min.Y >= outer.Min.Y-eps &&
			inner.Max.X <= outer.Max.X+eps && inner.Max.Y <= outer.Max.Y+eps
	}
	for _, z := range []float32{1, 1.49, 3} {
		for i := range 40 {
			c := Camera{
				PrevPos: gfx.Vec2{X: 50 + float32(i)*0.31, Y: 20 - float32(i)*0.17},
				Pos:     gfx.Vec2{X: 50 + float32(i)*0.31 + 2.6, Y: 20 - float32(i)*0.17 - 0.9},
				Zoom:    z, PixelSnap: true,
			}
			b := c.DrawBounds(screen)
			for _, alpha := range []float32{0, 0.1, 0.33, 0.5, 0.77, 1} {
				v := c.At(alpha).pixelSnapped(screen)
				if drawn := v.WorldBounds(screen); !contains(b, drawn) {
					t.Fatalf("zoom %v camera %+v alpha %v: drawn view %v outside DrawBounds %v", z, c, alpha, drawn, b)
				}
			}
		}
	}
}
