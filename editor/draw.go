package editor

import (
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/weakpixel/engine/gfx"
)

// selectionColor is the outline of selected objects.
var selectionColor = color.RGBA{R: 255, G: 200, B: 40, A: 255}

// windowRect maps a world rectangle to window pixels through the current
// camera and the final-screen transform. Rotation of the camera is ignored
// (the corners are mapped and re-normalized).
func (s *Session) windowRect(r gfx.Rect) gfx.Rect {
	cam, size := s.e.Camera(), s.screenSize()
	a := s.e.LogicalToWindow(cam.WorldToScreen(r.Min, size))
	b := s.e.LogicalToWindow(cam.WorldToScreen(r.Max, size))
	return normRect(a, b)
}

// drawSelection outlines every selected object in window pixels, one pixel
// outside its bounds, and at least as large as the area that picks it.
func (s *Session) drawSelection(screen ebiten.FinalScreen) {
	least := minPickPx * float32(deviceScale())
	for _, id := range s.selection {
		r, ok := s.Bounds(id)
		if !ok {
			continue
		}
		s.strokeRect(screen, atLeast(s.windowRect(r), least).Grow(1), selectionColor)
	}
}

// strokeRect outlines the window-pixel rectangle r, one pixel wide, inside r.
func (s *Session) strokeRect(screen ebiten.FinalScreen, r gfx.Rect, c color.RGBA) {
	w, h := r.Max.X-r.Min.X, r.Max.Y-r.Min.Y
	s.fillRect(screen, r.Min.X, r.Min.Y, w, 1, c)
	s.fillRect(screen, r.Min.X, r.Max.Y-1, w, 1, c)
	s.fillRect(screen, r.Min.X, r.Min.Y, 1, h, c)
	s.fillRect(screen, r.Max.X-1, r.Min.Y, 1, h, c)
}

// fillRect fills a window-pixel rectangle with c.
func (s *Session) fillRect(screen ebiten.FinalScreen, x, y, w, h float32, c color.RGBA) {
	if s.pixel == nil {
		// ebiten.FinalScreen only offers DrawImage and friends (vector
		// needs an *ebiten.Image), so lines are a scaled white pixel.
		s.pixel = ebiten.NewImage(1, 1)
		s.pixel.Fill(color.White)
	}
	var op ebiten.DrawImageOptions
	op.GeoM.Scale(float64(w), float64(h))
	op.GeoM.Translate(float64(x), float64(y))
	op.ColorScale.ScaleWithColor(c)
	screen.DrawImage(s.pixel, &op)
}
