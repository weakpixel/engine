//go:build ebitentest

package render

import (
	"image/color"
	"testing"

	"github.com/weakpixel/engine/gfx"
)

// TestCanvasPaintAndDraw paints the red test sprite into a canvas and draws
// the canvas in world space, tinted and flipped.
func TestCanvasPaintAndDraw(t *testing.T) {
	e, sprites := testSprites(t, 1)
	st := StoresOf(e.srv)
	c := NewCanvases(st)
	// A 8x4 canvas, the 4x4 sprite in its left half.
	id := c.New(8, 4)
	var paint Queue
	paint.Sprite(0, sprites[0], gfx.Vec2{X: 2, Y: 2}, gfx.Vec2{X: 2, Y: 2})
	c.Paint(id, &paint)

	r := NewRenderer(st)
	r.SetCanvases(c)
	tests := []struct {
		name       string
		flip       bool
		red, empty int // x of a red and of an empty pixel at y 12
	}{
		{"plain", false, 11, 15},
		{"flipped", true, 15, 11},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dst := newTarget(32, 32)
			var q Queue
			// World (10, 10) is at screen (10, 10): the camera is centered.
			p := gfx.Vec2{X: 10, Y: 10}
			q.Push(Item{Kind: KindCanvas, Canvas: id, Pos: p, PrevPos: p, Scale: gfx.Vec2{X: 1, Y: 1},
				FlipX: tc.flip, Color: gfx.RGBA(255, 255, 255, 255)})
			if tc.flip {
				// Mirrored about the pivot: put it on the right edge, so the
				// canvas still covers x 10..18.
				q.At(0).Origin = gfx.Vec2{X: 1}
			}
			r.Draw(dst, &q, &Camera{Pos: gfx.Vec2{X: 16, Y: 16}, PrevPos: gfx.Vec2{X: 16, Y: 16}}, 1)
			if got := pixel(dst, tc.red, 12); got.R != 255 || got.A != 255 {
				t.Errorf("pixel %d,12 = %v, want red", tc.red, got)
			}
			if got := pixel(dst, tc.empty, 12); got.A != 0 {
				t.Errorf("pixel %d,12 = %v, want empty", tc.empty, got)
			}
			if got := pixel(dst, 9, 12); got.A != 0 {
				t.Errorf("pixel left of the canvas = %v, want empty", got)
			}
		})
	}
}

// TestDrawCanvasShader draws a canvas through a shader: Params.x 1 flashes
// it white, a zero or failed handle draws it unshaded.
func TestDrawCanvasShader(t *testing.T) {
	r, sprite, flash, failed := shadedRenderer(t)
	c := NewCanvases(r.stores)
	r.SetCanvases(c)
	id := c.New(4, 4)
	var paint Queue
	paint.Sprite(0, sprite, gfx.Vec2{X: 2, Y: 2}, gfx.Vec2{X: 2, Y: 2})
	c.Paint(id, &paint)

	tests := []struct {
		name   string
		shader gfx.ShaderHandle
		params [4]float32
		want   color.RGBA
		shaded int
	}{
		{"Params.x 1 flashes white", flash, [4]float32{1}, color.RGBA{255, 255, 255, 255}, 1},
		{"zero handle draws normally", gfx.ShaderHandle{}, [4]float32{1}, color.RGBA{255, 0, 0, 255}, 0},
		{"failed shader draws normally", failed, [4]float32{1}, color.RGBA{255, 0, 0, 255}, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dst := newTarget(16, 16)
			var q Queue
			q.Push(Item{Kind: KindCanvas, Canvas: id, Space: ScreenSpace, Pos: gfx.Vec2{X: 4, Y: 4}, PrevPos: gfx.Vec2{X: 4, Y: 4},
				Scale: gfx.Vec2{X: 1, Y: 1}, Shader: tc.shader, Params: tc.params})
			r.Draw(dst, &q, &Camera{}, 1)
			if got := pixel(dst, 5, 5); got != tc.want {
				t.Fatalf("canvas pixel = %v, want %v", got, tc.want)
			}
			if st := r.Stats(); st.ShaderDraws != tc.shaded {
				t.Fatalf("ShaderDraws = %d, want %d", st.ShaderDraws, tc.shaded)
			}
		})
	}
}
