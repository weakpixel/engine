//go:build ebitentest

package render

import (
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

func TestBloomGlow(t *testing.T) {
	r, q, _ := postPixelsWith(t, nil)
	draw := func(o BloomOptions) *ebiten.Image {
		var post PostChain
		post.AddBloom(o)
		r.SetPost(&post)
		dst := newTarget(32, 32)
		r.Draw(dst, q, &Camera{}, 1)
		return dst
	}

	dst := draw(BloomOptions{})
	// The red sprite (0..3) glows around itself and nowhere else.
	if got := pixel(dst, 1, 1); got.R != 255 {
		t.Fatalf("sprite = %v, want it to stay red", got)
	}
	if got := pixel(dst, 5, 1); got.R == 0 || got.G != 0 || got.B != 0 {
		t.Fatalf("next to the sprite = %v, want a red glow", got)
	}
	if got := pixel(dst, 28, 28); got != (color.RGBA{}) {
		t.Fatalf("far from the sprite = %v, want no glow", got)
	}

	// A threshold above the sprite's brightness leaves the frame alone.
	dst = draw(BloomOptions{Threshold: 1})
	if got := pixel(dst, 5, 1); got != (color.RGBA{}) {
		t.Fatalf("threshold 1: next to the sprite = %v, want no glow", got)
	}
}
