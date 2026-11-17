//go:build ebitentest

package render

import (
	"image"
	"image/color"
	"testing"

	"github.com/weakpixel/engine/gfx"
)

// drawItem draws one item onto a new 64x64 target and returns its pixels.
func drawItem(t *testing.T, r *Renderer, it Item) *image.RGBA {
	t.Helper()
	dst := newTarget(64, 64)
	var q Queue
	q.Push(it)
	r.Draw(dst, &q, &Camera{}, 1)
	out := image.NewRGBA(dst.Bounds())
	dst.ReadPixels(out.Pix)
	return out
}

// samePixels fails the test when more than tolerance pixels of got and want
// differ, or when want is empty.
func samePixels(t *testing.T, got, want *image.RGBA, tolerance int) {
	t.Helper()
	b := want.Bounds()
	drawn, wrong := 0, 0
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			g, w := got.RGBAAt(x, y), want.RGBAAt(x, y)
			if w.A != 0 {
				drawn++
			}
			if g != w {
				if wrong++; wrong > tolerance {
					t.Fatalf("pixel (%d,%d) = %v, want %v (%d pixels differ, %d allowed)", x, y, g, w, wrong, tolerance)
				}
			}
		}
	}
	if drawn == 0 {
		t.Fatal("nothing was drawn")
	}
}

// TestDrawShadedKindsPassThrough checks that a shader that returns the
// source pixel draws text, nine-slices and shapes exactly as without a
// shader: same pixels, same place, same tint.
func TestDrawShadedKindsPassThrough(t *testing.T) {
	e := newKindsEnv(t)
	variants := []struct {
		name string
		edit func(*Item)
	}{
		{"plain", func(*Item) {}},
		{"tinted", func(it *Item) { it.Color = gfx.RGBA(255, 128, 0, 255) }},
		{"half transparent", func(it *Item) { it.Color = gfx.RGBA(255, 255, 255, 128) }},
	}
	for name, item := range e.kindItems(gfx.Vec2{X: 24, Y: 24}) {
		// The slanted segments of a line or circle start at other float
		// coordinates in the scratch image, so a pixel that a segment only
		// touches can fall on the other side. Everything else is exact.
		tolerance := 0
		if name == "line" || name == "circle" {
			tolerance = 6
		}
		for _, v := range variants {
			// The edges of a shape overlap (a rect's corners, a circle's
			// segments), so an unshaded half-transparent shape is denser
			// there; the shaded one is tinted once, as a whole.
			if shape := name != "nine-slice" && name != "text"; shape && v.name == "half transparent" {
				continue
			}
			t.Run(name+"/"+v.name, func(t *testing.T) {
				it := item
				v.edit(&it)
				want := drawItem(t, e.r, it)
				it.Shader = e.pass
				samePixels(t, drawItem(t, e.r, it), want, tolerance)
			})
		}
	}
}

// TestDrawShadedNineSlicePassThrough covers the panel options: tiling, a
// pivot, the image's own size and two lines of text next to it.
func TestDrawShadedNineSlicePassThrough(t *testing.T) {
	e := newKindsEnv(t)
	base := e.kindItems(gfx.Vec2{X: 30, Y: 30})["nine-slice"]
	tests := []struct {
		name string
		edit func(*Item)
	}{
		{"tiled", func(it *Item) { it.Tile = true }},
		{"centered pivot", func(it *Item) { it.Origin = gfx.Vec2{X: 0.5, Y: 0.5} }},
		{"own size", func(it *Item) { it.Size = gfx.Vec2{} }},
		{"shrunk borders", func(it *Item) { it.Size = gfx.Vec2{X: 3, Y: 3} }},
		{"scaled", func(it *Item) { it.Scale = gfx.Vec2{X: 2, Y: 2}; it.Size = gfx.Vec2{X: 6, Y: 6} }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			it := base
			tc.edit(&it)
			want := drawItem(t, e.r, it)
			it.Shader = e.pass
			samePixels(t, drawItem(t, e.r, it), want, 0)
		})
	}
}

// TestDrawShadedTextLines checks a text of two lines: the shader gets both.
func TestDrawShadedTextLines(t *testing.T) {
	e := newKindsEnv(t)
	it := e.kindItems(gfx.Vec2{X: 8, Y: 8})["text"]
	it.Text = "Hi\nyo"
	want := drawItem(t, e.r, it)
	it.Shader = e.pass
	samePixels(t, drawItem(t, e.r, it), want, 0)
}

// TestDrawShadedPanelSeesWholeItem checks what the shader gets: the whole
// panel as one image, so a flash turns every piece white, and a rotation
// turns the shaded panel like the unshaded one.
func TestDrawShadedPanelSeesWholeItem(t *testing.T) {
	e := newKindsEnv(t)
	it := e.kindItems(gfx.Vec2{X: 20, Y: 20})["nine-slice"]
	it.Shader, it.Params = e.flash, [4]float32{1}
	got := drawItem(t, e.r, it)
	white := color.RGBA{255, 255, 255, 255}
	for _, p := range []image.Point{{20, 20}, {30, 21}, {39, 33}, {28, 27}, {21, 33}} {
		if c := got.RGBAAt(p.X, p.Y); c != white {
			t.Errorf("flashed panel pixel %v = %v, want white", p, c)
		}
	}
	if c := got.RGBAAt(41, 27); c.A != 0 {
		t.Errorf("pixel right of the panel = %v, want empty", c)
	}

	// A quarter turn around the top-left corner: the 20x14 panel now covers
	// x 6..20, y 20..40.
	it.Rot = 3.14159265 / 2
	got = drawItem(t, e.r, it)
	if c := got.RGBAAt(12, 30); c != white {
		t.Errorf("rotated panel pixel = %v, want white", c)
	}
	if c := got.RGBAAt(30, 26); c.A != 0 {
		t.Errorf("pixel where the unrotated panel was = %v, want empty", c)
	}
}

// TestDrawShadedMargin checks the transparent margin around a shaded item:
// a shader that fills its rectangle paints shadeMargin pixels beyond the
// item on every side, and nothing further out.
func TestDrawShadedMargin(t *testing.T) {
	e := newKindsEnv(t)
	red := color.RGBA{255, 0, 0, 255}
	for _, name := range []string{"nine-slice", "text", "rect"} {
		t.Run(name, func(t *testing.T) {
			it := e.kindItems(gfx.Vec2{X: 20, Y: 20})[name]
			it.Shader, it.Color = e.fill, gfx.RGBA(255, 0, 0, 255)
			got := drawItem(t, e.r, it)
			if c := got.RGBAAt(20-shadeMargin, 20-shadeMargin); c != red {
				t.Errorf("pixel at the margin's corner = %v, want red", c)
			}
			if c := got.RGBAAt(20-shadeMargin-1, 20); c.A != 0 {
				t.Errorf("pixel left of the margin = %v, want empty", c)
			}
			if c := got.RGBAAt(20, 20-shadeMargin-1); c.A != 0 {
				t.Errorf("pixel above the margin = %v, want empty", c)
			}
		})
	}
}

// TestDrawShadedKindsKeepOrder checks that two shaded items in one frame do
// not see each other in the scratch image and that draw order holds: the
// second item is drawn over the first, and an unshaded sprite between them
// stays between them.
func TestDrawShadedKindsKeepOrder(t *testing.T) {
	e := newKindsEnv(t)
	dst := newTarget(64, 64)
	one := gfx.Vec2{X: 1, Y: 1}
	var q Queue
	// A large red filled region, then a small green one inside it.
	q.Push(Item{Kind: KindRect, Space: ScreenSpace, Pos: gfx.Vec2{X: 10, Y: 10}, PrevPos: gfx.Vec2{X: 10, Y: 10},
		Scale: one, Size: gfx.Vec2{X: 30, Y: 30}, Shader: e.fill, Color: gfx.RGBA(255, 0, 0, 255)})
	q.Push(Item{Kind: KindRect, Space: ScreenSpace, Pos: gfx.Vec2{X: 20, Y: 20}, PrevPos: gfx.Vec2{X: 20, Y: 20},
		Scale: one, Size: gfx.Vec2{X: 4, Y: 4}, Shader: e.fill, Color: gfx.RGBA(0, 255, 0, 255)})
	e.r.Draw(dst, &q, &Camera{}, 1)
	if c := pixel(dst, 21, 21); c != (color.RGBA{0, 255, 0, 255}) {
		t.Errorf("pixel inside the second item = %v, want green", c)
	}
	if c := pixel(dst, 35, 35); c != (color.RGBA{255, 0, 0, 255}) {
		t.Errorf("pixel of the first item outside the second = %v, want red", c)
	}
	// The second, smaller region did not leave the first one's pixels in
	// the part of the scratch image it did not clear: draw the first again.
	var again Queue
	again.Push(*q.At(1))
	next := newTarget(64, 64)
	e.r.Draw(next, &again, &Camera{}, 1)
	if c := pixel(next, 35, 35); c.A != 0 {
		t.Errorf("pixel outside the small item = %v, want empty", c)
	}
}
