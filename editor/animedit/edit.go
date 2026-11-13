package animedit

import (
	"errors"
	"fmt"
	"slices"

	"github.com/weakpixel/engine/anim/animfile"
)

// Shape kinds of a definition, and the kind name of body boxes.
const (
	KindHit  = "hit"
	KindHurt = "hurt"
	KindBody = "body"
)

// BoxRef names a box of a clip: a rect of the file's "body" (Body true, Index
// into it) or a shape of the clip (Index into its "shapes").
type BoxRef struct {
	Body  bool
	Index int
}

// clipEntry returns the file's entry for the clip, nil when it has none.
func clipEntry(f *animfile.File, name string) *animfile.Clip {
	for i := range f.Clips {
		if f.Clips[i].Name == name {
			return &f.Clips[i].Clip
		}
	}
	return nil
}

// ensureClip returns the clip's entry and adds an empty one when the file has
// none. An empty entry changes nothing the loader does (an unlisted tag keeps
// the sheet's timing and still gets the pivot and body).
func ensureClip(f *animfile.File, name string) *animfile.Clip {
	if c := clipEntry(f, name); c != nil {
		return c
	}
	f.Clips = append(f.Clips, animfile.NamedClip{Name: name})
	return &f.Clips[len(f.Clips)-1].Clip
}

// AddClipEntry lists the clip in the file ({}), if it is not.
func AddClipEntry(f *animfile.File, name string) error {
	ensureClip(f, name)
	return nil
}

// SetDefaultClip sets "defaultClip"; an empty name removes it.
func SetDefaultClip(f *animfile.File, name string) error {
	f.DefaultClip = name
	return nil
}

// SetFilePivot sets the top-level pivot; nil removes it.
func SetFilePivot(f *animfile.File, p *animfile.Point) error {
	f.Pivot = p
	return nil
}

// SetClipPivot sets the clip's own pivot; nil removes it.
func SetClipPivot(f *animfile.File, clip string, p *animfile.Point) error {
	if p == nil {
		if c := clipEntry(f, clip); c != nil {
			c.Pivot = nil
		}
		return nil
	}
	ensureClip(f, clip).Pivot = p
	return nil
}

// Loop modes of the panel's dropdown, in order: the sheet's, "loop": true,
// "loop": false.
const (
	LoopSheet = iota
	LoopOn
	LoopOff
)

// SetLoop sets the clip's "loop" from a LoopSheet, LoopOn or LoopOff.
func SetLoop(f *animfile.File, clip string, mode int) error {
	if mode < LoopSheet || mode > LoopOff {
		return fmt.Errorf("loop mode %d is out of range", mode)
	}
	if mode == LoopSheet && clipEntry(f, clip) == nil {
		return nil
	}
	c := ensureClip(f, clip)
	switch mode {
	case LoopSheet:
		c.Loop = nil
	default:
		v := mode == LoopOn
		c.Loop = &v
	}
	return nil
}

// SetFPS sets the clip's "fps"; 0 removes it.
func SetFPS(f *animfile.File, clip string, fps int) error {
	if fps < 0 {
		return fmt.Errorf("fps is %d, want > 0 (0 removes it)", fps)
	}
	if fps == 0 && clipEntry(f, clip) == nil {
		return nil
	}
	c := ensureClip(f, clip)
	if fps == 0 {
		c.FPS = nil
	} else {
		c.FPS = &fps
	}
	return nil
}

// SetSpeed sets the clip's "speed"; 0 removes it.
func SetSpeed(f *animfile.File, clip string, speed float64) error {
	if speed < 0 {
		return fmt.Errorf("speed is %v, want > 0 (0 removes it)", speed)
	}
	if speed == 0 && clipEntry(f, clip) == nil {
		return nil
	}
	c := ensureClip(f, clip)
	if speed == 0 {
		c.Speed = nil
	} else {
		c.Speed = &speed
	}
	return nil
}

// SetFrameDuration sets the duration in ms of one frame (play order) of the
// clip. The first change writes the whole "durations" array from current,
// the durations the clip has now in ms, since the format wants one value per
// frame.
func SetFrameDuration(f *animfile.File, clip string, current []int, frame, ms int) error {
	if frame < 0 || frame >= len(current) {
		return fmt.Errorf("frame %d is outside the clip's %d frames", frame, len(current))
	}
	if ms <= 0 {
		return fmt.Errorf("duration is %d ms, want > 0", ms)
	}
	c := ensureClip(f, clip)
	if len(c.Durations) != len(current) {
		c.Durations = slices.Clone(current)
	}
	c.Durations[frame] = ms
	return nil
}

// ClearDurations removes the clip's "durations".
func ClearDurations(f *animfile.File, clip string) error {
	if c := clipEntry(f, clip); c != nil {
		c.Durations = nil
	}
	return nil
}

func checkRect(r animfile.Rect) error {
	if r.W <= 0 || r.H <= 0 {
		return fmt.Errorf("rect is %dx%d, want w and h > 0", r.W, r.H)
	}
	return nil
}

// AddBox adds a box and returns its reference. kind is KindBody (appended to
// "body"), KindHit or KindHurt (a shape of clip over frame only).
func AddBox(f *animfile.File, clip, kind string, frame int, r animfile.Rect) (BoxRef, error) {
	if err := checkRect(r); err != nil {
		return BoxRef{}, err
	}
	switch kind {
	case KindBody:
		f.Body = append(f.Body, r)
		return BoxRef{Body: true, Index: len(f.Body) - 1}, nil
	case KindHit, KindHurt:
		if frame < 0 {
			return BoxRef{}, fmt.Errorf("frame %d is negative", frame)
		}
		c := ensureClip(f, clip)
		c.Shapes = append(c.Shapes, animfile.Shape{Kind: kind, Frames: &animfile.FrameRange{From: frame}, Rect: r})
		return BoxRef{Index: len(c.Shapes) - 1}, nil
	}
	return BoxRef{}, fmt.Errorf("unknown box kind %q", kind)
}

// rectOf returns a pointer to the rect of ref.
func rectOf(f *animfile.File, clip string, ref BoxRef) (*animfile.Rect, error) {
	if ref.Body {
		if ref.Index < 0 || ref.Index >= len(f.Body) {
			return nil, fmt.Errorf("body %d does not exist", ref.Index)
		}
		return &f.Body[ref.Index], nil
	}
	s, err := shapeOf(f, clip, ref)
	if err != nil {
		return nil, err
	}
	return &s.Rect, nil
}

func shapeOf(f *animfile.File, clip string, ref BoxRef) (*animfile.Shape, error) {
	c := clipEntry(f, clip)
	if ref.Body || c == nil || ref.Index < 0 || ref.Index >= len(c.Shapes) {
		return nil, fmt.Errorf("clip %q has no shape %d", clip, ref.Index)
	}
	return &c.Shapes[ref.Index], nil
}

// SetBoxRect replaces the rect of a box.
func SetBoxRect(f *animfile.File, clip string, ref BoxRef, r animfile.Rect) error {
	if err := checkRect(r); err != nil {
		return err
	}
	p, err := rectOf(f, clip, ref)
	if err != nil {
		return err
	}
	*p = r
	return nil
}

// DeleteBox removes a box.
func DeleteBox(f *animfile.File, clip string, ref BoxRef) error {
	if _, err := rectOf(f, clip, ref); err != nil {
		return err
	}
	if ref.Body {
		f.Body = slices.Delete(f.Body, ref.Index, ref.Index+1)
		return nil
	}
	c := clipEntry(f, clip)
	c.Shapes = slices.Delete(c.Shapes, ref.Index, ref.Index+1)
	if len(c.Shapes) == 0 {
		c.Shapes = nil
	}
	return nil
}

// ClearBody removes every "body" rect.
func ClearBody(f *animfile.File) error {
	f.Body = nil
	return nil
}

// SetShapeKind sets a shape's kind to KindHit or KindHurt.
func SetShapeKind(f *animfile.File, clip string, ref BoxRef, kind string) error {
	if kind != KindHit && kind != KindHurt {
		return fmt.Errorf("unknown shape kind %q", kind)
	}
	s, err := shapeOf(f, clip, ref)
	if err != nil {
		return err
	}
	s.Kind = kind
	return nil
}

// SetShapeID sets a shape's id.
func SetShapeID(f *animfile.File, clip string, ref BoxRef, id string) error {
	s, err := shapeOf(f, clip, ref)
	if err != nil {
		return err
	}
	s.ID = id
	return nil
}

// SetShapeFrames sets a shape's frame range: all is every frame; otherwise
// from, and to unless it is nil (the single frame from).
func SetShapeFrames(f *animfile.File, clip string, ref BoxRef, all bool, from int, to *int) error {
	s, err := shapeOf(f, clip, ref)
	if err != nil {
		return err
	}
	if all {
		s.Frames = nil
		return nil
	}
	var errs []error
	if from < 0 {
		errs = append(errs, fmt.Errorf("frames from is %d, want >= 0", from))
	}
	if to != nil && *to < from {
		errs = append(errs, fmt.Errorf("frames to %d is before from %d", *to, from))
	}
	if err := errors.Join(errs...); err != nil {
		return err
	}
	fr := &animfile.FrameRange{From: from}
	if to != nil {
		v := *to
		fr.To = &v
	}
	s.Frames = fr
	return nil
}

// CopyToFrame adds a shape like the one at ref, on frame only, and returns
// it: the way to key a box per frame.
func CopyToFrame(f *animfile.File, clip string, ref BoxRef, frame int) (BoxRef, error) {
	s, err := shapeOf(f, clip, ref)
	if err != nil {
		return BoxRef{}, err
	}
	if frame < 0 {
		return BoxRef{}, fmt.Errorf("frame %d is negative", frame)
	}
	c := clipEntry(f, clip)
	cp := *s
	cp.Frames = &animfile.FrameRange{From: frame}
	c.Shapes = append(c.Shapes, cp)
	return BoxRef{Index: len(c.Shapes) - 1}, nil
}

// ExtendToFrame grows a shape's frame range to include frame. A shape over
// all frames already does.
func ExtendToFrame(f *animfile.File, clip string, ref BoxRef, frame int) error {
	s, err := shapeOf(f, clip, ref)
	if err != nil {
		return err
	}
	if frame < 0 {
		return fmt.Errorf("frame %d is negative", frame)
	}
	if s.Frames == nil {
		return nil
	}
	lo, hi := s.Frames.From, s.Frames.From
	if s.Frames.To != nil {
		hi = *s.Frames.To
	}
	lo, hi = min(lo, frame), max(hi, frame)
	fr := &animfile.FrameRange{From: lo}
	if hi != lo {
		fr.To = &hi
	}
	s.Frames = fr
	return nil
}
