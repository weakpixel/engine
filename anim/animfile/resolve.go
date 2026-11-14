package animfile

import (
	"errors"
	"fmt"
	"math"
	"time"
)

// BodyBoxID is the ID of the hurt boxes that come from "body".
const BodyBoxID = "body"

// BoxKind says what a [Box] is for. It mirrors gfx.BoxKind (same order).
type BoxKind uint8

// Box kinds.
const (
	// BoxHurt is where the entity can be hit ("hurt", and the "body" boxes).
	BoxHurt BoxKind = iota
	// BoxHit is where the entity deals a hit ("hit").
	BoxHit
)

// Box is a resolved hit or hurt box, relative to the pivot. Package anim
// converts it to gfx.Box.
type Box struct {
	Kind BoxKind
	ID   string
	Rect Rect
}

// Tag is a clip of the sheet as [Resolve] needs it: name, loop mode and
// frame durations in play order. Get them from [Sheet.PlayOrder], or from
// loaded clips.
type Tag struct {
	Name      string
	Loop      LoopMode
	Durations []time.Duration
}

// Resolved is a definition applied to its sheet's tags.
type Resolved struct {
	// Default is "defaultClip"; empty if the file has none.
	Default string
	// Clips has one entry per tag, in tag order, including tags the file
	// does not list (they get the top-level pivot and body only).
	Clips []ResolvedClip
	// Body is the union of the "body" rects, relative to the pivot; zero
	// without any. It is the same for every clip: the fixed body that
	// per-frame hurt shapes may replace (a collider uses this one).
	Body Rect
	// Omitted lists the tags without an entry in "clips", in tag order. It is
	// information, not an error: those clips keep the sheet's loop mode and
	// timing.
	Omitted []string
}

// ResolvedClip is one clip with every override applied.
type ResolvedClip struct {
	Name string
	// Listed reports whether the file has an entry for the clip.
	Listed bool
	Loop   LoopMode
	// Durations are the final frame durations, in play order.
	Durations []time.Duration
	// Pivot is the clip's pivot, else the top-level one; nil if neither.
	Pivot *Point
	// Boxes are the boxes per frame (nil for a frame without), all sharing
	// one backing array with each frame's capacity capped at its length. Nil
	// if the clip has no boxes at all.
	Boxes [][]Box
}

// Resolve applies f to tags and checks everything that needs the sheet:
// unknown or duplicate clip names, an unknown "defaultClip", "body" and shape
// rects (w, h > 0), shape kinds and frame ranges, "durations" (length,
// positive), "fps" and "speed" (> 0). Every problem is reported, one per
// line of the joined error, each naming its clip and shape.
//
// Rules: "loop" false is Once, true keeps Loop/PingPong (else Loop). Timing
// starts from "durations", else "fps" (1000/fps ms, whole ms, at least 1),
// else the tag's durations; "speed" then divides. Each frame's boxes are the
// body boxes, unless a hurt shape covers the frame (then the clip's hurt
// shapes replace them there), followed by the shapes covering the frame in
// file order.
func Resolve(f *File, tags []Tag) (*Resolved, error) {
	var errs []error
	body, bodyErrs := bodyBoxes(f.Body)
	errs = append(errs, bodyErrs...)

	known := make(map[string]bool, len(tags))
	for _, t := range tags {
		known[t.Name] = true
	}
	defs := make(map[string]*Clip, len(f.Clips))
	for i := range f.Clips {
		c := &f.Clips[i]
		switch {
		case defs[c.Name] != nil:
			errs = append(errs, fmt.Errorf("clip %q: listed twice", c.Name))
		case !known[c.Name]:
			errs = append(errs, fmt.Errorf("clip %q: no such tag in sheet %q", c.Name, f.Sheet))
		default:
			defs[c.Name] = &c.Clip
		}
	}
	if f.DefaultClip != "" && !known[f.DefaultClip] {
		errs = append(errs, fmt.Errorf(`"defaultClip" %q: no such tag in sheet %q`, f.DefaultClip, f.Sheet))
	}

	res := &Resolved{Default: f.DefaultClip, Clips: make([]ResolvedClip, len(tags))}
	for _, b := range body {
		res.Body = res.Body.union(b.Rect)
	}
	for i, t := range tags {
		rc, clipErrs := resolveClip(t, defs[t.Name], f.Pivot, body)
		errs = append(errs, withContext(fmt.Sprintf("clip %q", t.Name), clipErrs)...)
		res.Clips[i] = rc
		if !rc.Listed {
			res.Omitted = append(res.Omitted, t.Name)
		}
	}
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}
	return res, nil
}

// withContext prefixes every error with ctx. The errors end up as separate
// lines of one errors.Join, so each line must name its clip, shape or frame on
// its own: wrapping a joined error would prefix only the first line.
func withContext(ctx string, errs []error) []error {
	for i, err := range errs {
		errs[i] = fmt.Errorf("%s: %w", ctx, err)
	}
	return errs
}

// bodyBoxes turns "body" into hurt boxes, collecting a problem per rect.
func bodyBoxes(rects []Rect) ([]Box, []error) {
	var errs []error
	boxes := make([]Box, 0, len(rects))
	for i, r := range rects {
		if err := r.check(); err != nil {
			errs = append(errs, fmt.Errorf("body %d: %w", i, err))
			continue
		}
		boxes = append(boxes, Box{Kind: BoxHurt, ID: BodyBoxID, Rect: r})
	}
	return boxes, errs
}

// union returns the smallest rect holding r and o; a zero r is empty.
func (r Rect) union(o Rect) Rect {
	if r == (Rect{}) {
		return o
	}
	x0, y0 := min(r.X, o.X), min(r.Y, o.Y)
	x1, y1 := max(r.X+r.W, o.X+o.W), max(r.Y+r.H, o.Y+o.H)
	return Rect{X: x0, Y: y0, W: x1 - x0, H: y1 - y0}
}

func (r Rect) check() error {
	if r.W <= 0 || r.H <= 0 {
		return fmt.Errorf("rect w and h must be positive, got w=%d h=%d", r.W, r.H)
	}
	return nil
}

// resolveClip applies d (nil: not listed), the pivot (the clip's, else the
// sheet's) and the boxes (body plus d's shapes) to t. It returns every
// problem found.
func resolveClip(t Tag, d *Clip, sheetPivot *Point, body []Box) (ResolvedClip, []error) {
	rc := ResolvedClip{Name: t.Name, Listed: d != nil, Loop: t.Loop, Pivot: sheetPivot}
	rc.Durations = append([]time.Duration(nil), t.Durations...)
	var errs []error
	var shapes []Shape
	if d != nil {
		rc.Loop = loopMode(t.Loop, d.Loop)
		errs = append(errs, retime(rc.Durations, d)...)
		shapes = d.Shapes
		if d.Pivot != nil {
			rc.Pivot = d.Pivot
		}
	}
	var boxErrs []error
	rc.Boxes, boxErrs = frameBoxes(len(t.Durations), body, shapes)
	return rc, append(errs, boxErrs...)
}

// loopMode applies a "loop" override: false is Once; true keeps a looping
// mode (Loop or PingPong from the Aseprite direction) and otherwise loops.
func loopMode(base LoopMode, loop *bool) LoopMode {
	switch {
	case loop == nil:
		return base
	case !*loop:
		return Once
	case base == PingPong:
		return PingPong
	default:
		return Loop
	}
}

// retime applies the timing overrides to durations: "durations", else "fps",
// sets the base durations; "speed" then divides them.
func retime(durations []time.Duration, d *Clip) []error {
	var errs []error
	switch {
	case d.Durations != nil:
		if len(d.Durations) != len(durations) {
			errs = append(errs, fmt.Errorf(`"durations" has %d values for %d frames`, len(d.Durations), len(durations)))
			break
		}
		for i, ms := range d.Durations {
			if ms <= 0 {
				errs = append(errs, fmt.Errorf(`"durations"[%d] is %d, want a positive number of ms`, i, ms))
				continue
			}
			durations[i] = time.Duration(ms) * time.Millisecond
		}
	case d.FPS != nil:
		if *d.FPS <= 0 {
			errs = append(errs, fmt.Errorf(`"fps" is %d, want > 0`, *d.FPS))
			break
		}
		// Whole milliseconds, like Aseprite's own durations, and never zero.
		per := max(time.Duration(math.Round(1000/float64(*d.FPS)))*time.Millisecond, time.Millisecond)
		for i := range durations {
			durations[i] = per
		}
	}
	if d.Speed != nil {
		s := *d.Speed
		if !(s > 0) {
			errs = append(errs, fmt.Errorf(`"speed" is %g, want > 0`, s))
			return errs
		}
		for i := range durations {
			durations[i] = max(time.Duration(float64(durations[i])/s), 1)
		}
	}
	return errs
}

// shapeSpan is a validated shape: its box and inclusive frame range.
type shapeSpan struct {
	box      Box
	from, to int
}

func (s shapeSpan) covers(i int) bool { return i >= s.from && i <= s.to }

// frameBoxes builds the boxes of n frames: per frame the body boxes, unless a
// hurt shape covers the frame (then the clip's hurt shapes replace the body
// there), then the shapes that cover the frame, in file order.
//
// All boxes of the clip share one backing array, and each frame gets a
// sub-slice capped at its own length, so a clip costs one allocation and an
// append to one frame's slice can never overwrite the next frame's boxes.
func frameBoxes(n int, body []Box, shapes []Shape) ([][]Box, []error) {
	var errs []error
	spans := make([]shapeSpan, 0, len(shapes))
	for i, s := range shapes {
		sp, spanErrs := s.span(n)
		if len(spanErrs) > 0 {
			errs = append(errs, withContext(fmt.Sprintf("shape %d", i), spanErrs)...)
			continue
		}
		spans = append(spans, sp)
	}
	if len(body) == 0 && len(spans) == 0 {
		return nil, errs
	}

	replaced := make([]bool, n)
	total := 0
	for i := range n {
		for _, sp := range spans {
			if sp.covers(i) {
				total++
				replaced[i] = replaced[i] || sp.box.Kind == BoxHurt
			}
		}
		if !replaced[i] {
			total += len(body)
		}
	}
	all := make([]Box, 0, total)
	boxes := make([][]Box, n)
	for i := range n {
		start := len(all)
		if !replaced[i] {
			all = append(all, body...)
		}
		for _, sp := range spans {
			if sp.covers(i) {
				all = append(all, sp.box)
			}
		}
		if len(all) > start {
			boxes[i] = all[start:len(all):len(all)]
		}
	}
	return boxes, errs
}

// span validates s against a clip of n frames, collecting every problem.
func (s Shape) span(n int) (shapeSpan, []error) {
	var errs []error
	sp := shapeSpan{box: Box{ID: s.ID, Rect: s.Rect}, from: 0, to: n - 1}
	switch s.Kind {
	case "hurt":
		sp.box.Kind = BoxHurt
	case "hit":
		sp.box.Kind = BoxHit
	default:
		errs = append(errs, fmt.Errorf(`"kind" is %q, want "hit" or "hurt"`, s.Kind))
	}
	if s.Frames != nil {
		sp.from, sp.to = s.Frames.From, s.Frames.From
		if s.Frames.To != nil {
			sp.to = *s.Frames.To
		}
		if sp.from < 0 || sp.to < sp.from || sp.to >= n {
			errs = append(errs, fmt.Errorf("frames %d..%d are outside the clip's %d frames", sp.from, sp.to, n))
		}
	}
	if err := s.Rect.check(); err != nil {
		errs = append(errs, err)
	}
	return sp, errs
}
