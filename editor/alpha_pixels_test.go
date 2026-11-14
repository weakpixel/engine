//go:build ebitentest

package editor

import (
	"image"
	"image/color"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"
)

// TestReadOpaque reads real pixels. It runs from inside Update (see
// TestMain), the only place Ebiten allows it. Run with -tags ebitentest and
// a display (xvfb-run).
func TestReadOpaque(t *testing.T) {
	img := ebiten.NewImage(16, 8)
	img.Set(5, 3, color.RGBA{A: 255})
	img.Set(9, 4, color.RGBA{R: 255, A: 255})
	r, ok := readOpaque(img)
	if want := image.Rect(5, 3, 10, 5); !ok || r != want {
		t.Errorf("readOpaque = %v, %v; want %v", r, ok, want)
	}
	if _, ok := readOpaque(ebiten.NewImage(4, 4)); ok {
		t.Error("a transparent image has no art")
	}
}
