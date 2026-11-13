package tileset

import (
	"errors"
	"fmt"
	"image"
)

// Align places a sub-texture's frame inside a larger cell.
type Align int

// The alignments of -align.
const (
	TopLeft Align = iota
	Center
	BottomLeft
)

// ParseAlign returns the Align named by s: topleft, center or bottomleft.
func ParseAlign(s string) (Align, error) {
	switch s {
	case "topleft":
		return TopLeft, nil
	case "center":
		return Center, nil
	case "bottomleft":
		return BottomLeft, nil
	}
	return 0, fmt.Errorf("unknown alignment %q (want topleft, center or bottomleft)", s)
}

// Layout is the uniform grid the tiles are packed into.
type Layout struct {
	Tile            image.Point // cell size
	Columns, Count  int
	Margin, Spacing int
}

// CellSize returns the smallest cell that holds every frame of the atlas.
func CellSize(a *Atlas) image.Point {
	var c image.Point
	for _, s := range a.SubTextures {
		c.X, c.Y = max(c.X, s.Frame.X), max(c.Y, s.Frame.Y)
	}
	return c
}

// NewLayout lays out a's tiles in cells of size tile (zero: CellSize), columns
// per row (capped at the tile count). A frame larger than the cell is an
// error listing all offenders.
func NewLayout(a *Atlas, tile image.Point, columns, margin, spacing int) (Layout, error) {
	if tile == (image.Point{}) {
		tile = CellSize(a)
	}
	var errs []error
	for _, s := range a.SubTextures {
		if s.Frame.X > tile.X || s.Frame.Y > tile.Y {
			errs = append(errs, fmt.Errorf("SubTexture %q: frame %dx%d is larger than the %dx%d tile",
				s.Name, s.Frame.X, s.Frame.Y, tile.X, tile.Y))
		}
	}
	if columns < 1 || margin < 0 || spacing < 0 || tile.X < 1 || tile.Y < 1 {
		errs = append(errs, errors.New("tile size and columns must be positive, margin and spacing not negative"))
	}
	if len(errs) > 0 {
		return Layout{}, errors.Join(errs...)
	}
	n := len(a.SubTextures)
	return Layout{Tile: tile, Columns: min(columns, n), Count: n, Margin: margin, Spacing: spacing}, nil
}

// Rows is the number of tile rows.
func (l Layout) Rows() int { return (l.Count + l.Columns - 1) / l.Columns }

// Size is the size of the output image.
func (l Layout) Size() image.Point {
	return image.Pt(
		2*l.Margin+l.Columns*l.Tile.X+(l.Columns-1)*l.Spacing,
		2*l.Margin+l.Rows()*l.Tile.Y+(l.Rows()-1)*l.Spacing,
	)
}

// Origin is the top-left pixel of cell i.
func (l Layout) Origin(i int) image.Point {
	col, row := i%l.Columns, i/l.Columns
	return image.Pt(
		l.Margin+col*(l.Tile.X+l.Spacing),
		l.Margin+row*(l.Tile.Y+l.Spacing),
	)
}

// Offset is where s's pixels start inside its cell: the frame is aligned in
// the cell, and the trimmed pixels sit at s.Trim inside the frame.
func (l Layout) Offset(s SubTexture, align Align) image.Point {
	var frame image.Point
	switch align {
	case Center:
		frame = image.Pt((l.Tile.X-s.Frame.X)/2, (l.Tile.Y-s.Frame.Y)/2)
	case BottomLeft:
		frame = image.Pt(0, l.Tile.Y-s.Frame.Y)
	}
	return frame.Add(s.Trim)
}
