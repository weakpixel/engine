package animedit

import (
	"math"

	"github.com/weakpixel/engine/anim/animfile"
	"github.com/weakpixel/engine/gfx"
)

// Box is a box of the current frame with where it comes from in the file.
type Box struct {
	Ref  BoxRef
	Kind string // KindBody, KindHit or KindHurt
	ID   string
	Rect animfile.Rect
	// Replaced is set on a body box that a hurt shape replaces on this
	// frame: it is drawn dimmed and cannot be picked.
	Replaced bool
}

// covers reports whether the shape's range includes frame of a clip with n
// frames, by the rule of animfile.Resolve: no range is every frame, no "to"
// is the single frame "from".
func covers(s *animfile.Shape, frame, n int) bool {
	if frame < 0 || frame >= n {
		return false
	}
	if s.Frames == nil {
		return true
	}
	to := s.Frames.From
	if s.Frames.To != nil {
		to = *s.Frames.To
	}
	return frame >= s.Frames.From && frame <= to
}

// ActiveBoxes returns the boxes on frame of a clip with n frames in the order
// of animfile.Resolve: the body boxes, then the shapes that cover the frame in
// file order. If a hurt shape covers the frame the body boxes are still
// listed but marked Replaced.
func ActiveBoxes(f *animfile.File, clip string, frame, n int) []Box {
	var shapes []animfile.Shape
	if c := clipEntry(f, clip); c != nil {
		shapes = c.Shapes
	}
	replaced := false
	for i := range shapes {
		if shapes[i].Kind == KindHurt && covers(&shapes[i], frame, n) {
			replaced = true
		}
	}
	var out []Box
	for i, r := range f.Body {
		out = append(out, Box{Ref: BoxRef{Body: true, Index: i}, Kind: KindBody, ID: animfile.BodyBoxID, Rect: r, Replaced: replaced})
	}
	for i := range shapes {
		if s := &shapes[i]; covers(s, frame, n) {
			out = append(out, Box{Ref: BoxRef{Index: i}, Kind: s.Kind, ID: s.ID, Rect: s.Rect})
		}
	}
	return out
}

// Contains reports whether the world point p is inside r.
func Contains(r animfile.Rect, p gfx.Vec2) bool {
	return p.X >= float32(r.X) && p.X < float32(r.X+r.W) && p.Y >= float32(r.Y) && p.Y < float32(r.Y+r.H)
}

// Pick returns the topmost box at p: later boxes over earlier ones, so shapes
// over the body. Replaced boxes cannot be picked.
func Pick(boxes []Box, p gfx.Vec2) (BoxRef, bool) {
	for i := len(boxes) - 1; i >= 0; i-- {
		if !boxes[i].Replaced && Contains(boxes[i].Rect, p) {
			return boxes[i].Ref, true
		}
	}
	return BoxRef{}, false
}

// Handle is one of the eight resize handles of a rect, or none.
type Handle uint8

// Handles, clockwise from the top-left corner.
const (
	HandleNone Handle = iota
	HandleNW
	HandleN
	HandleNE
	HandleE
	HandleSE
	HandleS
	HandleSW
	HandleW
)

// HandleAt returns the handle of r within tol world units of p, corners
// before edges.
func HandleAt(r animfile.Rect, p gfx.Vec2, tol float32) Handle {
	x0, y0 := float32(r.X), float32(r.Y)
	x1, y1 := float32(r.X+r.W), float32(r.Y+r.H)
	xm, ym := (x0+x1)/2, (y0+y1)/2
	near := func(a, b float32) bool { return math.Abs(float64(a-b)) <= float64(tol) }
	pts := []struct {
		h    Handle
		x, y float32
	}{
		{HandleNW, x0, y0}, {HandleNE, x1, y0}, {HandleSE, x1, y1}, {HandleSW, x0, y1},
		{HandleN, xm, y0}, {HandleE, x1, ym}, {HandleS, xm, y1}, {HandleW, x0, ym},
	}
	for _, h := range pts {
		if near(p.X, h.x) && near(p.Y, h.y) {
			return h.h
		}
	}
	return HandleNone
}

// ResizeRect moves the edges of r that handle h controls by dx, dy and keeps
// the size at least 1×1: the edge opposite to the handle stays where it is.
func ResizeRect(r animfile.Rect, h Handle, dx, dy int32) animfile.Rect {
	x0, y0, x1, y1 := r.X, r.Y, r.X+r.W, r.Y+r.H
	switch h {
	case HandleNW, HandleW, HandleSW:
		x0 = min(x0+dx, x1-1)
	case HandleNE, HandleE, HandleSE:
		x1 = max(x1+dx, x0+1)
	}
	switch h {
	case HandleNW, HandleN, HandleNE:
		y0 = min(y0+dy, y1-1)
	case HandleSW, HandleS, HandleSE:
		y1 = max(y1+dy, y0+1)
	}
	return animfile.Rect{X: x0, Y: y0, W: x1 - x0, H: y1 - y0}
}

// MoveRect moves r by dx, dy.
func MoveRect(r animfile.Rect, dx, dy int32) animfile.Rect {
	r.X += dx
	r.Y += dy
	return r
}

// RectBetween returns the pixel rect spanned by two world points, each
// rounded to the nearest pixel corner. It may be empty (a click without a
// drag): the caller drops anything under 1×1.
func RectBetween(a, b gfx.Vec2) animfile.Rect {
	r := func(v float32) int32 { return int32(math.Round(float64(v))) }
	x0, x1 := r(min(a.X, b.X)), r(max(a.X, b.X))
	y0, y1 := r(min(a.Y, b.Y)), r(max(a.Y, b.Y))
	return animfile.Rect{X: x0, Y: y0, W: x1 - x0, H: y1 - y0}
}

// Flip mirrors r around x = 0, as Box.World does for a flipped sprite. It is
// its own inverse, so it also maps a mouse position back.
func Flip(r animfile.Rect) animfile.Rect {
	r.X = -(r.X + r.W)
	return r
}

// ToRect converts a pixel rect to a world rect.
func ToRect(r animfile.Rect) gfx.Rect {
	return gfx.Rect{
		Min: gfx.Vec2{X: float32(r.X), Y: float32(r.Y)},
		Max: gfx.Vec2{X: float32(r.X + r.W), Y: float32(r.Y + r.H)},
	}
}
