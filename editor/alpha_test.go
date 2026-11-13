package editor

import (
	"fmt"
	"image"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/weakpixel/engine/gfx"
)

// TestPickUsesVisibleArt: a sprite with transparent margins is picked, and
// outlined, by its art only. box.png is 16x8; the art is its inner 8x4.
func TestPickUsesVisibleArt(t *testing.T) {
	f := newFixture(t, Config{})
	reads := 0
	f.s.alpha.read = func(*ebiten.Image) (image.Rectangle, bool) {
		reads++
		return image.Rect(4, 2, 12, 6), true
	}
	f.open(doc(visual("a", 0, 0, 0)))
	f.settle()

	if r, _ := f.s.Bounds("a"); r != (gfx.Rect{Min: gfx.Vec2{X: -4, Y: -2}, Max: gfx.Vec2{X: 4, Y: 2}}) {
		t.Errorf("Bounds(a) = %v, want the art only", r)
	}
	if id, ok := f.s.Pick(gfx.Vec2{X: -7, Y: 0}); ok {
		t.Errorf("clicking the transparent margin picked %q", id)
	}
	if id, ok := f.s.Pick(gfx.Vec2{X: 3, Y: 1}); !ok || id != "a" {
		t.Errorf("clicking the art picked %q, %v", id, ok)
	}
	if reads != 1 {
		t.Errorf("pixels read %d times, want once (cached per sprite)", reads)
	}
}

// TestPickFallsBackToFrame: when the pixels cannot be read, the frame rect
// is used and the read is retried later.
func TestPickFallsBackToFrame(t *testing.T) {
	f := newFixture(t, Config{})
	readable := false
	f.s.alpha.read = func(*ebiten.Image) (image.Rectangle, bool) {
		return image.Rect(4, 2, 12, 6), readable
	}
	f.open(doc(visual("a", 0, 0, 0)))
	f.settle()
	if id, ok := f.s.Pick(gfx.Vec2{X: -7, Y: 0}); !ok || id != "a" {
		t.Fatalf("unreadable pixels: picked %q, %v; want the whole frame", id, ok)
	}
	readable = true
	if _, ok := f.s.Pick(gfx.Vec2{X: -7, Y: 0}); ok {
		t.Error("after the pixels became readable the margin is still picked")
	}
}

// panel is a gfx:Panel object whose NineSlice has the given size and origin.
func panel(id string, x, y, w, h, ox, oy float32) string {
	return fmt.Sprintf(`{"id":%q,"group":"gfx","class":"Panel","components":{`+
		`"gfx:Transform":{"Pos":{"X":%g,"Y":%g},"PrevPos":{"X":%g,"Y":%g},"Rot":0,"Scale":{"X":1,"Y":1}},`+
		`"gfx:NineSlice":{"ID":"box.png","Insets":{"Left":2,"Top":2,"Right":2,"Bottom":2},"Size":{"X":%g,"Y":%g},"Layer":0,"Origin":{"X":%g,"Y":%g},"Tint":0,"Blend":0,"Tile":false,"Hidden":false}}}`,
		id, x, y, x, y, w, h, ox, oy)
}

// TestPickNineSlice: a panel is picked and outlined by its whole rectangle
// (Size from the Origin pivot), and a zero Size is the frame's (box.png is
// 16x8).
func TestPickNineSlice(t *testing.T) {
	f := newFixture(t, Config{})
	f.open(doc(panel("p", 10, 10, 40, 20, 0, 0), panel("q", 100, 100, 0, 0, 0.5, 0.5)))
	f.settle()

	if r, _ := f.s.Bounds("p"); r != (gfx.Rect{Min: gfx.Vec2{X: 10, Y: 10}, Max: gfx.Vec2{X: 50, Y: 30}}) {
		t.Errorf("Bounds(p) = %v", r)
	}
	if r, _ := f.s.Bounds("q"); r != (gfx.Rect{Min: gfx.Vec2{X: 92, Y: 96}, Max: gfx.Vec2{X: 108, Y: 104}}) {
		t.Errorf("Bounds(q) = %v, want the frame size around the pivot", r)
	}
	for _, c := range []struct {
		p  gfx.Vec2
		id string
	}{{gfx.Vec2{X: 30, Y: 20}, "p"}, {gfx.Vec2{X: 10.5, Y: 29.5}, "p"}, {gfx.Vec2{X: 100, Y: 100}, "q"}, {gfx.Vec2{X: 51, Y: 20}, ""}, {gfx.Vec2{X: 30, Y: 31}, ""}} {
		if id, _ := f.s.Pick(c.p); id != c.id {
			t.Errorf("Pick(%v) = %q, want %q", c.p, id, c.id)
		}
	}
}

// TestPickSmallObject: an object a pixel wide on screen is picked within
// minPickPx device-independent pixels around it, scaled by the display.
func TestPickSmallObject(t *testing.T) {
	f := newFixture(t, Config{})
	f.s.alpha.read = func(*ebiten.Image) (image.Rectangle, bool) {
		return image.Rect(8, 4, 9, 5), true // one visible pixel
	}
	f.open(doc(visual("a", 0, 0, 0)))
	f.settle()
	r, _ := f.s.Bounds("a")
	c := gfx.Vec2{X: (r.Min.X + r.Max.X) / 2, Y: (r.Min.Y + r.Max.Y) / 2}
	unit := f.s.windowRect(gfx.Rect{Max: gfx.Vec2{X: 1, Y: 1}})
	px := unit.Max.X - unit.Min.X // window pixels per world unit
	for _, scale := range []float64{1, 2} {
		old := deviceScale
		deviceScale = func() float64 { return scale }
		reach := minPickPx * float32(scale) / 2 / px // world units from the center
		if id, ok := f.s.Pick(c.Add(gfx.Vec2{X: reach * 0.9})); !ok || id != "a" {
			t.Errorf("scale %g: a click %g px beside the pixel picked %q, %v", scale, reach*0.9*px, id, ok)
		}
		if id, ok := f.s.Pick(c.Add(gfx.Vec2{X: reach * 1.1})); ok {
			t.Errorf("scale %g: a click %g px away picked %q", scale, reach*1.1*px, id)
		}
		deviceScale = old
	}
}
