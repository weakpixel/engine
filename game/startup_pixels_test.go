//go:build ebitentest

package game

import (
	"image"
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// TestStartupComposePixels reads real pixels of a composed startup screen:
// the image fits the window (aspect kept, centred) on the Background. Run
// with -tags ebitentest and a display (xvfb-run).
func TestStartupComposePixels(t *testing.T) {
	red := image.NewRGBA(image.Rect(0, 0, 8, 4))
	for y := range 4 {
		for x := range 8 {
			red.Set(x, y, color.RGBA{R: 255, A: 255})
		}
	}
	blue := color.RGBA{B: 255, A: 255}
	e, _, _ := newStartupEngine(t, Config{Startup: &StartupConfig{Screens: []StartupScreen{{Image: red, Background: blue}}}})
	target := e.startup.compose(e, image.Pt(32, 32))
	// 8x4 scaled by 4 to 32x16, centred: rows 8..23.
	for _, tt := range []struct {
		x, y int
		want color.RGBA
	}{{16, 2, blue}, {16, 29, blue}, {2, 12, color.RGBA{R: 255, A: 255}}, {29, 20, color.RGBA{R: 255, A: 255}}} {
		if got := color.RGBAModel.Convert(target.At(tt.x, tt.y)).(color.RGBA); got != tt.want {
			t.Errorf("pixel (%d,%d) = %v, want %v", tt.x, tt.y, got, tt.want)
		}
	}

	var state StartupState
	e2, _, _ := newStartupEngine(t, Config{Startup: &StartupConfig{Screens: []StartupScreen{{
		Draw: func(dst *ebiten.Image, s StartupState) {
			state = s
			dst.Set(1, 1, color.White)
		},
	}}}})
	target = e2.startup.compose(e2, image.Pt(10, 6))
	if b := target.Bounds(); b.Dx() != 10 || b.Dy() != 6 {
		t.Fatalf("Draw target is %v, want the window size", b)
	}
	if got := color.RGBAModel.Convert(target.At(1, 1)).(color.RGBA); got != (color.RGBA{255, 255, 255, 255}) {
		t.Errorf("Draw did not draw: %v", got)
	}
	if state.Screens != 1 || state.Duration != defaultStartupDuration {
		t.Errorf("state = %+v", state)
	}
}
