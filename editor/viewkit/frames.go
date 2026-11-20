package viewkit

import (
	"fmt"

	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/render"
)

// Frame is one frame of an atlas or a sheet as a tool shows it.
type Frame struct {
	// Index is the position in the atlas's file order.
	Index int
	// Name is the key of the frame ("idle_0", "3").
	Name string
	// Rect is the frame's pixels in the texture.
	Rect gfx.Rect
	// Size is the untrimmed size, Offset where the trimmed image starts in
	// it, and Pivot the default pivot in untrimmed pixels.
	Size, Offset, Pivot gfx.Vec2
}

// FramesOf lists the frames of a loaded atlas in file order. A frame that is
// not ready (it cannot happen after a successful load) is left out.
func FramesOf(a *render.Atlas, sprites render.SpriteStore) []Frame {
	names := make(map[gfx.SpriteHandle]string, len(a.Sprites))
	for name, h := range a.Sprites {
		names[h] = name
	}
	frames := make([]Frame, 0, len(a.Order))
	for i, h := range a.Order {
		sp := sprites.Get(h)
		if sp == nil {
			continue
		}
		r := sp.Rect
		frames = append(frames, Frame{
			Index:  i,
			Name:   names[h],
			Rect:   gfx.Rect{Min: gfx.Vec2{X: float32(r.Min.X), Y: float32(r.Min.Y)}, Max: gfx.Vec2{X: float32(r.Max.X), Y: float32(r.Max.Y)}},
			Size:   sp.Size,
			Offset: sp.Offset,
			Pivot:  gfx.Vec2{X: sp.Origin.X * sp.Size.X, Y: sp.Origin.Y * sp.Size.Y},
		})
	}
	return frames
}

// FrameAt returns the index in frames of the frame under texture point p, or
// -1. Where frames overlap (a packer may share pixels) the smallest wins, so
// a small frame inside a big one can still be picked.
func FrameAt(frames []Frame, p gfx.Vec2) int {
	best, bestArea := -1, float32(0)
	for i, f := range frames {
		if !f.Rect.Contains(p) {
			continue
		}
		area := (f.Rect.Max.X - f.Rect.Min.X) * (f.Rect.Max.Y - f.Rect.Min.Y)
		if best < 0 || area < bestArea {
			best, bestArea = i, area
		}
	}
	return best
}

// LabelMode says what a tool writes on each frame.
type LabelMode int

// The label modes, in the order of the panel's dropdown.
const (
	LabelBoth LabelMode = iota
	LabelIndex
	LabelName
	LabelNone
)

// LabelModeNames are the entries of the Labels dropdown.
var LabelModeNames = []string{"Index and name", "Index", "Name", "None"}

// Text is the label of f: its index, its name, or both (the name only once
// when it is the index, as in a grid sheet).
func (m LabelMode) Text(f Frame) string {
	idx := fmt.Sprint(f.Index)
	switch m {
	case LabelIndex:
		return idx
	case LabelName:
		if f.Name == "" {
			return idx
		}
		return f.Name
	case LabelBoth:
		if f.Name == "" || f.Name == idx {
			return idx
		}
		return idx + " " + f.Name
	}
	return ""
}

// RectText is "x, y, w × h" of r.
func RectText(r gfx.Rect) string {
	return fmt.Sprintf("%d, %d, %d × %d", int(r.Min.X), int(r.Min.Y), int(r.Max.X-r.Min.X), int(r.Max.Y-r.Min.Y))
}

// PointText is "x, y" of p.
func PointText(p gfx.Vec2) string { return fmt.Sprintf("%g, %g", p.X, p.Y) }
