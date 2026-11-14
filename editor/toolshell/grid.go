package toolshell

import (
	"image/color"
	"math"
)

// The colours are premultiplied (color.RGBA), so a translucent white has
// R=G=B=A; a component above A would saturate to opaque.
var (
	gridColor      = color.RGBA{R: 18, G: 18, B: 18, A: 18}
	gridMajorColor = color.RGBA{R: 40, G: 40, B: 40, A: 40}
)

const (
	// gridMajorEvery makes every n-th grid line brighter.
	gridMajorEvery = 8
	// minGridPixels is the smallest grid step in window pixels that is
	// drawn; a denser grid would only be noise.
	minGridPixels = 4
)

// Grid is the grid drawn over the viewport. The lines are the multiples of
// Cell in world units, shifted by Offset.
type Grid struct {
	On bool
	// Cell is the cell size in world units.
	Cell float32
	// OffsetX and OffsetY shift the lines, in world units.
	OffsetX, OffsetY float32
}

// GridLine is one line of a grid along one axis.
type GridLine struct {
	// At is the line's position in window pixels along the axis.
	At float32
	// Major is set for every eighth line, counted from the grid origin.
	Major bool
}

// GridLines returns the lines of a grid along one axis that fall inside
// 0..length (window pixels). origin is the window position of line 0 and
// step the distance between lines in window pixels. It returns nil when step
// is below 4 pixels, since a denser grid would only be noise.
func GridLines(origin, step, length float32) []GridLine {
	if step < minGridPixels {
		return nil
	}
	first := int(math.Floor(float64(-origin / step)))
	last := int(math.Ceil(float64((length - origin) / step)))
	lines := make([]GridLine, 0, last-first+1)
	for k := first; k <= last; k++ {
		lines = append(lines, GridLine{At: origin + float32(k)*step, Major: k%gridMajorEvery == 0})
	}
	return lines
}

// CoarseStep returns step (window pixels) times 8 as often as needed to
// reach 4 pixels, so a zoomed-out view (a whole world) still shows a grid:
// every eighth line, then every 64th, and so on. The lines stay on the
// multiples of the cell. A step of 0 or less is returned unchanged.
func CoarseStep(step float32) float32 {
	if step <= 0 {
		return step
	}
	for step < minGridPixels {
		step *= gridMajorEvery
	}
	return step
}
