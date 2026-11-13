package editor

import (
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/weakpixel/engine/gfx"
)

// The colours are premultiplied (color.RGBA), so a translucent white has
// R=G=B=A; a component above A would saturate to opaque.
var (
	gridColor      = color.RGBA{R: 18, G: 18, B: 18, A: 18}
	gridMajorColor = color.RGBA{R: 40, G: 40, B: 40, A: 40}
	axisXColor     = color.RGBA{R: 172, G: 52, B: 52, A: 190}
	axisYColor     = color.RGBA{R: 60, G: 149, B: 67, A: 190}
)

const (
	// gridMajorEvery makes every n-th grid line brighter.
	gridMajorEvery = 8
	// minGridPixels is the smallest grid step in window pixels that is
	// drawn; a denser grid would only be noise.
	minGridPixels = 4
)

// drawStage draws the grid and the origin axes of a [Stage] in window
// pixels, over the game's picture. The grid lines are the multiples of
// Stage.Grid; the axes (X red, Y green) cross at the world origin.
func (s *Session) drawStage(screen ebiten.FinalScreen) {
	b := screen.Bounds()
	w, h := float32(b.Dx()), float32(b.Dy())
	// The window position of the world origin and the grid step in pixels
	// come from mapping one grid cell.
	step := s.cfg.Stage.Grid
	if step <= 0 {
		step = 1
	}
	cell := s.windowRect(gfx.Rect{Max: gfx.Vec2{X: step, Y: step}})
	ox, oy := cell.Min.X, cell.Min.Y
	px, py := cell.Max.X-cell.Min.X, cell.Max.Y-cell.Min.Y
	if s.cfg.Stage.Grid > 0 && px >= minGridPixels && py >= minGridPixels {
		lines := func(origin, size, length float32, vertical bool) {
			first := int(math.Floor(float64(-origin / size)))
			last := int(math.Ceil(float64((length - origin) / size)))
			for k := first; k <= last; k++ {
				c := gridColor
				if k%gridMajorEvery == 0 {
					c = gridMajorColor
				}
				at := origin + float32(k)*size
				if vertical {
					s.fillRect(screen, at, 0, 1, h, c)
				} else {
					s.fillRect(screen, 0, at, w, 1, c)
				}
			}
		}
		lines(ox, px, w, true)
		lines(oy, py, h, false)
	}
	s.fillRect(screen, 0, oy, w, 1, axisXColor)
	s.fillRect(screen, ox, 0, 1, h, axisYColor)
}

// InStage reports whether the session is a prefab [Stage].
func (s *Session) InStage() bool { return s.cfg.Stage != nil }

// SetGrid sets the grid step of a prefab [Stage] in world units; 0 or less
// draws no grid. It does nothing in a level session.
func (s *Session) SetGrid(grid float32) {
	if s.cfg.Stage != nil {
		s.cfg.Stage.Grid = max(grid, 0)
	}
}
