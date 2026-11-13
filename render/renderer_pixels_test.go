//go:build ebitentest

// Tests that read pixels back. Ebiten only allows that while a game is
// running, so they need a window (a display): run them with
//
//	go test -tags ebitentest ./render
package render

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
	"testing/fstest"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/weakpixel/engine/gfx"
)

// pixel reads one pixel. The tests that call it need Ebiten's graphics
// context, see TestMain in main_test.go.
func pixel(img *ebiten.Image, x, y int) color.RGBA {
	return color.RGBAModel.Convert(img.At(x, y)).(color.RGBA)
}

func TestDrawSpriteInterpolation(t *testing.T) {
	r, id := testRenderer(t)
	tests := []struct {
		name  string
		alpha float64
		wantX int
	}{
		{"alpha 0 draws at PrevPos", 0, 10},
		{"alpha 1 draws at Pos", 1, 30},
		{"alpha 0.5 draws between", 0.5, 20},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dst := newTarget(64, 16)
			var q Queue
			q.Sprite(0, id, gfx.Vec2{X: 30, Y: 4}, gfx.Vec2{X: 10, Y: 4})
			q.At(0).Space = ScreenSpace
			q.At(0).Origin = topLeft
			r.Draw(dst, &q, &Camera{}, tc.alpha)
			if got := pixel(dst, tc.wantX+1, 5); got.R != 255 {
				t.Fatalf("pixel at %d,5 = %v, want red", tc.wantX+1, got)
			}
			if got := pixel(dst, tc.wantX-2, 5); got.R != 0 {
				t.Fatalf("pixel left of the sprite = %v, want empty", got)
			}
		})
	}
}

func TestDrawWorldSpaceUsesCamera(t *testing.T) {
	r, id := testRenderer(t)
	dst := newTarget(64, 64)
	var q Queue
	q.Sprite(0, id, gfx.Vec2{X: 100, Y: 100}, gfx.Vec2{X: 100, Y: 100}).Origin = topLeft
	// The camera looks at the sprite, so it is drawn at the screen center.
	r.Draw(dst, &q, &Camera{Pos: gfx.Vec2{X: 100, Y: 100}, PrevPos: gfx.Vec2{X: 100, Y: 100}}, 1)
	if got := pixel(dst, 32, 32); got.R != 255 {
		t.Fatalf("center pixel = %v, want red", got)
	}
	if got := pixel(dst, 5, 5); got.R != 0 {
		t.Fatalf("corner pixel = %v, want empty", got)
	}
}

func TestDrawTintAndShapes(t *testing.T) {
	r, id := testRenderer(t)
	dst := newTarget(64, 64)
	var q Queue
	q.Sprite(0, id, gfx.Vec2{X: 2, Y: 2}, gfx.Vec2{X: 2, Y: 2}).Space = ScreenSpace
	q.At(0).Origin = topLeft
	q.At(0).Color = gfx.RGBA(0, 255, 0, 255) // red sprite * green tint = black
	q.Push(Item{Kind: KindRect, Space: ScreenSpace, Pos: gfx.Vec2{X: 20, Y: 20}, PrevPos: gfx.Vec2{X: 20, Y: 20},
		Scale: gfx.Vec2{X: 1, Y: 1}, Size: gfx.Vec2{X: 10, Y: 10}})
	q.Push(Item{Kind: KindLine, Space: ScreenSpace, Pos: gfx.Vec2{X: 40, Y: 40}, PrevPos: gfx.Vec2{X: 40, Y: 40},
		Scale: gfx.Vec2{X: 1, Y: 1}, Size: gfx.Vec2{X: 10, Y: 0}})
	q.Push(Item{Kind: KindCircle, Space: ScreenSpace, Pos: gfx.Vec2{X: 15, Y: 45}, PrevPos: gfx.Vec2{X: 15, Y: 45},
		Scale: gfx.Vec2{X: 1, Y: 1}, Size: gfx.Vec2{X: 8}})
	r.Draw(dst, &q, &Camera{}, 1)

	if got := pixel(dst, 3, 3); got.R != 0 {
		t.Fatalf("tinted sprite pixel = %v, want red channel removed", got)
	}
	if got := pixel(dst, 25, 20); got.A != 255 {
		t.Fatalf("rect top edge = %v, want drawn", got)
	}
	if got := pixel(dst, 25, 25); got.A != 0 {
		t.Fatalf("rect interior = %v, want empty (outline only)", got)
	}
	if got := pixel(dst, 45, 40); got.A != 255 {
		t.Fatalf("line = %v, want drawn", got)
	}
	if got := pixel(dst, 22, 45); got.A != 255 {
		t.Fatalf("circle right extreme (inside the radius) = %v, want drawn", got)
	}
	// One batch for the sprite, one for all shape segments (white image).
	if st := r.Stats(); st.Items != 4 || st.DrawCalls != 2 || st.Quads != 1+4+1+circleSegments {
		t.Fatalf("Stats = %+v", st)
	}
}

func TestDrawSpriteShader(t *testing.T) {
	r, sprite, flash, failed := shadedRenderer(t)
	tests := []struct {
		name   string
		shader gfx.ShaderHandle
		params [4]float32
		tint   gfx.Color
		want   color.RGBA
	}{
		{"Params.x 1 flashes white", flash, [4]float32{1}, 0, color.RGBA{255, 255, 255, 255}},
		{"Params.x 0 keeps the sprite", flash, [4]float32{}, 0, color.RGBA{255, 0, 0, 255}},
		{"tint reaches the shader", flash, [4]float32{1}, gfx.RGBA(0, 255, 0, 255), color.RGBA{0, 255, 0, 255}},
		{"failed shader draws normally", failed, [4]float32{1}, 0, color.RGBA{255, 0, 0, 255}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dst := newTarget(16, 16)
			var q Queue
			it := q.Sprite(0, sprite, gfx.Vec2{X: 4, Y: 4}, gfx.Vec2{X: 4, Y: 4})
			it.Space, it.Origin = ScreenSpace, topLeft
			it.Shader, it.Params, it.Color = tc.shader, tc.params, tc.tint
			r.Draw(dst, &q, &Camera{}, 1)
			if got := pixel(dst, 5, 5); got != tc.want {
				t.Fatalf("sprite pixel = %v, want %v", got, tc.want)
			}
			// The shader draws the sprite's rectangle and nothing else.
			if got := pixel(dst, 9, 5); got.A != 0 {
				t.Fatalf("pixel right of the sprite = %v, want empty", got)
			}
		})
	}
}

// TestDrawSpriteShaderAtlasFrame checks that a shader samples the item's
// own frame of an atlas, not the atlas origin: imageSrc0 is the frame's
// sub-image.
func TestDrawSpriteShaderAtlasFrame(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 8, 4))
	for y := range 4 {
		for x := range 8 {
			c := color.RGBA{R: 255, A: 255}
			if x >= 4 {
				c = color.RGBA{B: 255, A: 255}
			}
			img.SetRGBA(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	files := fstest.MapFS{
		"fx/flash.kage": textFile(kageFlash),
		"rb.png":        {Data: buf.Bytes()},
		"rb.sheet.json": textFile(`{"image":"rb.png","frameW":4,"frameH":4}`),
	}
	e := newAssetEnv(t, files, 0)
	sc := e.srv.NewScope("s")
	blue := LoadSprite(sc, "rb.sheet.json#1")
	flash := LoadShader(sc, "fx/flash.kage")
	e.settle(sc)
	r := NewRenderer(StoresOf(e.srv))

	dst := newTarget(16, 16)
	var q Queue
	it := q.Sprite(0, blue, gfx.Vec2{}, gfx.Vec2{})
	it.Space, it.Origin, it.Shader = ScreenSpace, topLeft, flash
	r.Draw(dst, &q, &Camera{}, 1)
	if got := pixel(dst, 1, 1); got != (color.RGBA{B: 255, A: 255}) {
		t.Fatalf("pixel = %v, want the blue frame", got)
	}
}
