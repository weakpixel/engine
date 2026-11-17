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

// TestBloomFromGroup checks BloomOptions.Src: with a layer group as the
// source only that group's sprite glows, although a sprite outside it is
// just as bright, and both still show.
func TestBloomFromGroup(t *testing.T) {
	r, q, _ := postPixelsWith(t, nil)
	sprite := q.At(0).Sprite
	put(q, sprite, 1, 20, 0) // layer 1, in the group
	q.Sort(nil, r.stores.Sprites)
	var g GroupConfig
	glow := g.Add(GroupOptions{From: 1, To: 1})
	r.SetGroups(&g)
	var post PostChain
	post.AddBloom(BloomOptions{Src: TargetGroup(glow)})
	r.SetPost(&post)
	dst := newTarget(32, 32)
	r.Draw(dst, q, &Camera{}, 1)

	for _, x := range []int{1, 21} {
		if got := pixel(dst, x, 1); got.R != 255 {
			t.Fatalf("sprite at x %d = %v, want red", x, got)
		}
	}
	if got := pixel(dst, 25, 1); got.R == 0 {
		t.Fatalf("next to the group's sprite = %v, want a red glow", got)
	}
	if got := pixel(dst, 5, 1); got != (color.RGBA{}) {
		t.Fatalf("next to the sprite outside the group = %v, want no glow", got)
	}
}
