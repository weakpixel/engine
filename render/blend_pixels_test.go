//go:build ebitentest

package render

import (
	"image/color"
	"testing"

	"github.com/weakpixel/engine/gfx"
)

// nearByte reports whether a and b differ by at most 3 (rounding in blending).
func nearByte(a, b uint8) bool { return int(a)-int(b) <= 3 && int(b)-int(a) <= 3 }

// TestDrawBlendModes draws a source of red 80 over a background of red 200
// with each mode and checks the red channel; the pixel next to the source
// must stay the background.
func TestDrawBlendModes(t *testing.T) {
	r, sprite, _, _ := groupPixels(t)
	tests := []struct {
		name  string
		blend Blend
		wantR uint8
	}{
		{"normal", BlendNormal, 80},
		{"additive", BlendAdditive, 255},
		{"screen", BlendScreen, 217}, // 80 + 200*(1-80/255)
		{"subtract", BlendSubtract, 120},
		{"lighten", BlendLighten, 200},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dst := newTarget(16, 16)
			var q Queue
			put(&q, sprite, 0, 0, 0).Color = gfx.RGBA(200, 255, 255, 255)
			put(&q, sprite, 0, 4, 0).Color = gfx.RGBA(200, 255, 255, 255)
			src := put(&q, sprite, 1, 0, 0)
			src.Color, src.Blend = gfx.RGBA(80, 255, 255, 255), tc.blend
			q.Sort(nil, r.stores.Sprites)
			r.Draw(dst, &q, &Camera{}, 1)
			if got := pixel(dst, 1, 1); !nearByte(got.R, tc.wantR) || got.G != 0 || got.A != 255 {
				t.Errorf("blended = %v, want R~%d", got, tc.wantR)
			}
			if got := pixel(dst, 5, 1); !nearByte(got.R, 200) || got.A != 255 {
				t.Errorf("untouched = %v, want R~200", got)
			}
		})
	}
}

// TestDrawEraseMask is the fog recipe: a group cleared to an opaque dark
// color covers the scene, and an item with BlendErase inside it cuts a hole.
func TestDrawEraseMask(t *testing.T) {
	r, sprite, _, _ := groupPixels(t)
	var g GroupConfig
	g.Add(GroupOptions{From: 10, To: 10, Clear: color.RGBA{0, 0, 255, 255}})
	r.SetGroups(&g)

	dst := newTarget(16, 16)
	var q Queue
	put(&q, sprite, 0, 0, 0)
	put(&q, sprite, 0, 4, 0)
	put(&q, sprite, 10, 0, 0).Blend = BlendErase
	q.Sort(nil, r.stores.Sprites)
	r.Draw(dst, &q, &Camera{}, 1)

	if got := pixel(dst, 1, 1); got != (color.RGBA{R: 255, A: 255}) {
		t.Errorf("hole = %v, want the scene (red)", got)
	}
	if got := pixel(dst, 5, 1); got != (color.RGBA{B: 255, A: 255}) {
		t.Errorf("fog = %v, want blue", got)
	}
}
