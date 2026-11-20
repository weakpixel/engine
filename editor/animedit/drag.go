package animedit

import (
	"errors"
	"math"

	"github.com/weakpixel/engine/anim/animfile"
	"github.com/weakpixel/engine/editor/toolshell"
	"github.com/weakpixel/engine/gfx"
)

// DragKind is what a mouse gesture does.
type DragKind int

// Gestures.
const (
	DragMove DragKind = iota
	DragResize
	DragDraw
	DragPivot
)

// Drag is a mouse gesture in progress. A drag only previews: the file is
// written once, on release, so one gesture is one undo step.
type Drag struct {
	Kind   DragKind
	Ref    BoxRef
	Handle Handle
	// Rect is the box as the gesture has it now (for DragDraw the box
	// being drawn; Box is its tool kind). Pivot is the pivot being placed.
	Rect animfile.Rect
	Box  string
	// At is the pivot being placed as a world point (pixels from the
	// current pivot); Pivot is the same in trimmed-image pixels.
	At, Pivot animfile.Point

	start gfx.Vec2
	orig  animfile.Rect
}

func pixel(v gfx.Vec2) animfile.Point {
	return animfile.Point{X: int32(math.Round(float64(v.X))), Y: int32(math.Round(float64(v.Y)))}
}

// errNoPivot is the notice of a box drawn before the pivot exists: rects are
// relative to the pivot, so they mean nothing without one.
var errNoPivot = errors.New("set a pivot first (P)")

// PointerDown starts a gesture at world point p (in file space, so already
// mirrored when the view is flipped). tol is the handle size in world units.
func (s *State) PointerDown(p gfx.Vec2, tol float32) error {
	if s.Stale != "" {
		return toolshell.ErrStale
	}
	if !s.Ready() {
		return nil
	}
	switch s.Tool {
	case ToolPivot:
		s.Drag = &Drag{Kind: DragPivot}
		s.movePivot(p)
	case ToolHit, ToolHurt, ToolBody:
		if s.Pivot() == nil {
			return errNoPivot
		}
		kind := map[ToolKind]string{ToolHit: KindHit, ToolHurt: KindHurt, ToolBody: KindBody}[s.Tool]
		s.Drag = &Drag{Kind: DragDraw, Box: kind, start: p, Rect: RectBetween(p, p)}
	default:
		boxes := s.Boxes()
		if s.Sel != nil {
			if b, ok := findBox(boxes, *s.Sel); ok && !b.Replaced {
				if h := HandleAt(b.Rect, p, tol); h != HandleNone {
					s.Drag = &Drag{Kind: DragResize, Ref: b.Ref, Handle: h, start: p, orig: b.Rect, Rect: b.Rect}
					return nil
				}
			}
		}
		ref, ok := Pick(boxes, p)
		if !ok {
			s.Sel = nil
			return nil
		}
		b, _ := findBox(boxes, ref)
		s.Sel = &ref
		s.Drag = &Drag{Kind: DragMove, Ref: ref, start: p, orig: b.Rect, Rect: b.Rect}
	}
	return nil
}

func findBox(boxes []Box, ref BoxRef) (Box, bool) {
	for _, b := range boxes {
		if b.Ref == ref {
			return b, true
		}
	}
	return Box{}, false
}

// PointerMove updates the gesture's preview.
func (s *State) PointerMove(p gfx.Vec2) {
	d := s.Drag
	if d == nil {
		return
	}
	dx, dy := pixel(gfx.Vec2{X: p.X - d.start.X, Y: p.Y - d.start.Y}).X, pixel(gfx.Vec2{X: p.X - d.start.X, Y: p.Y - d.start.Y}).Y
	switch d.Kind {
	case DragMove:
		d.Rect = MoveRect(d.orig, dx, dy)
	case DragResize:
		d.Rect = ResizeRect(d.orig, d.Handle, dx, dy)
	case DragDraw:
		d.Rect = RectBetween(d.start, p)
	case DragPivot:
		s.movePivot(p)
	}
}

// movePivot puts the pivot being placed under the world point p.
func (s *State) movePivot(p gfx.Vec2) {
	base := s.pivotBase()
	d := s.Drag
	d.At = pixel(p)
	d.Pivot = animfile.Point{X: base.X + d.At.X, Y: base.Y + d.At.Y}
}

// PointerUp ends the gesture and writes it as one edit. A box that is not
// 1×1 yet, or a move that moved nothing, writes nothing.
func (s *State) PointerUp(p gfx.Vec2) error {
	s.PointerMove(p)
	d := s.Drag
	s.Drag = nil
	if d == nil {
		return nil
	}
	clip := s.ClipName()
	switch d.Kind {
	case DragMove, DragResize:
		if d.Rect == d.orig {
			return nil
		}
		return s.do(func(f *animfile.File) error { return SetBoxRect(f, clip, d.Ref, d.Rect) })
	case DragDraw:
		if d.Rect.W < 1 || d.Rect.H < 1 {
			return nil
		}
		frame := s.Play.Frame
		var ref BoxRef
		err := s.do(func(f *animfile.File) (err error) {
			ref, err = AddBox(f, clip, d.Box, frame, d.Rect)
			return err
		})
		if err == nil {
			s.Sel = &ref
		}
		return err
	default:
		pt := d.Pivot
		if c := clipEntry(s.File, clip); c != nil && c.Pivot != nil {
			return s.do(func(f *animfile.File) error { return SetClipPivot(f, clip, &pt) })
		}
		return s.do(func(f *animfile.File) error { return SetFilePivot(f, &pt) })
	}
}

// Cancel drops a gesture without writing it.
func (s *State) Cancel() { s.Drag = nil }

// DeleteSelected removes the selected box.
func (s *State) DeleteSelected() error {
	if s.Sel == nil {
		return nil
	}
	ref, clip := *s.Sel, s.ClipName()
	if err := s.do(func(f *animfile.File) error { return DeleteBox(f, clip, ref) }); err != nil {
		return err
	}
	s.Sel = nil
	return nil
}

// Nudge moves the selected box by dx, dy pixels.
func (s *State) Nudge(dx, dy int32) error {
	if s.Sel == nil {
		return nil
	}
	ref, clip := *s.Sel, s.ClipName()
	p, err := rectOf(s.File, clip, ref)
	if err != nil {
		return err
	}
	r := MoveRect(*p, dx, dy)
	return s.do(func(f *animfile.File) error { return SetBoxRect(f, clip, ref, r) })
}
