//go:build ebitentest

package render

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
	"testing/fstest"

	"github.com/weakpixel/engine/gfx"
)

// panelPixels loads a 6x6 panel image with a 2 px border: red corners, green
// edges, a blue center, and a white pixel at the first center column so that
// tiling is visible.
func panelPixels(t *testing.T) (*Renderer, gfx.SpriteHandle) {
	t.Helper()
	red, green, blue, white := color.RGBA{255, 0, 0, 255}, color.RGBA{0, 255, 0, 255}, color.RGBA{0, 0, 255, 255}, color.RGBA{255, 255, 255, 255}
	img := image.NewRGBA(image.Rect(0, 0, 6, 6))
	for y := range 6 {
		for x := range 6 {
			c := blue
			switch inX, inY := x < 2 || x >= 4, y < 2 || y >= 4; {
			case inX && inY:
				c = red
			case inX || inY:
				c = green
			}
			img.SetRGBA(x, y, c)
		}
	}
	img.SetRGBA(2, 2, white)
	files := fstest.MapFS{"panel.png": {Data: encodePNG(t, img)}}
	e := newAssetEnv(t, files, 0)
	sc := e.srv.NewScope("ui")
	h := LoadSprite(sc, "panel.png")
	e.settle(sc)
	return NewRenderer(StoresOf(e.srv)), h
}

func encodePNG(t *testing.T, img image.Image) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func panelItem(h gfx.SpriteHandle, size gfx.Vec2, tile bool) Item {
	return Item{
		Kind: KindNineSlice, Space: ScreenSpace, Sprite: h, Scale: gfx.Vec2{X: 1, Y: 1},
		Size: size, Insets: gfx.Insets{Left: 2, Top: 2, Right: 2, Bottom: 2}, Tile: tile,
	}
}

func TestDrawNineSliceCorners(t *testing.T) {
	r, h := panelPixels(t)
	dst := newTarget(40, 24)
	var q Queue
	q.Push(panelItem(h, gfx.Vec2{X: 20, Y: 12}, false))
	q.Sort(nil, r.stores.Sprites)
	r.Draw(dst, &q, &Camera{}, 1)

	red, green, blue := color.RGBA{255, 0, 0, 255}, color.RGBA{0, 255, 0, 255}, color.RGBA{0, 0, 255, 255}
	for _, c := range []struct {
		x, y int
		want color.RGBA
	}{
		{0, 0, red}, {1, 1, red}, {18, 0, red}, {19, 1, red}, {0, 10, red}, {19, 11, red}, // corners keep their 2x2 size
		{2, 0, green}, {10, 1, green}, {0, 5, green}, {19, 6, green}, {10, 11, green}, // edges
		{10, 6, blue}, {17, 9, blue}, // center
		{20, 6, color.RGBA{}}, {10, 12, color.RGBA{}}, // nothing outside the panel
	} {
		if got := pixel(dst, c.x, c.y); got != c.want {
			t.Errorf("pixel %d,%d = %v, want %v", c.x, c.y, got, c.want)
		}
	}
	// The white pixel of the center's first column is stretched, not tiled.
	if got := pixel(dst, 2, 2); got != (color.RGBA{255, 255, 255, 255}) {
		t.Errorf("center start = %v, want white", got)
	}
	if got := pixel(dst, 3, 2); got != (color.RGBA{255, 255, 255, 255}) {
		t.Errorf("stretched white = %v, want white", got)
	}
	if st := r.Stats(); st.DrawCalls != 1 || st.Quads != 9 {
		t.Errorf("stats %+v: want one draw call of 9 quads", st)
	}
}

func TestDrawNineSliceTile(t *testing.T) {
	r, h := panelPixels(t)
	dst := newTarget(40, 24)
	var q Queue
	// The center spans x 2..15 and y 2..6: the 2x2 center image repeats every
	// 2 px and the last column is cropped to 1 px.
	q.Push(panelItem(h, gfx.Vec2{X: 17, Y: 8}, true))
	q.Sort(nil, r.stores.Sprites)
	r.Draw(dst, &q, &Camera{}, 1)

	white, blue, green := color.RGBA{255, 255, 255, 255}, color.RGBA{0, 0, 255, 255}, color.RGBA{0, 255, 0, 255}
	for _, c := range []struct {
		x, y int
		want color.RGBA
	}{
		{2, 2, white}, {4, 2, white}, {6, 2, white}, {14, 2, white}, {2, 4, white}, {8, 4, white}, // one white pixel per repeat
		{3, 2, blue}, {3, 3, blue}, {5, 5, blue}, {14, 3, blue},
		{15, 2, green}, {16, 5, green}, // right edge starts where the cropped repeat ends
	} {
		if got := pixel(dst, c.x, c.y); got != c.want {
			t.Errorf("pixel %d,%d = %v, want %v", c.x, c.y, got, c.want)
		}
	}
}
