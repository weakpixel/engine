package animedit

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/weakpixel/engine/anim"
	"github.com/weakpixel/engine/anim/animfile"
	"github.com/weakpixel/engine/editor/toolshell"
)

// Panel is the panel of the tool as it is now, without the view section the
// shell adds: the tool puts it in at the place it wants (before the
// problems).
func (s *State) Panel(title string, view toolshell.Section) toolshell.Panel {
	p := toolshell.Panel{Title: title}
	if s.Stale != "" {
		p.Status = "document has errors: " + s.Stale
	}
	if s.File != nil && s.Sheet != nil && len(s.Sheet.Tags) > 0 {
		if s.ReadOnly {
			p.Sections = append(p.Sections, s.playbackSection(), s.exportSection())
		} else {
			p.Sections = append(p.Sections, s.playbackSection(), s.fileSection(), s.clipSection(), s.boxListSection())
			if sec, ok := s.boxSection(); ok {
				p.Sections = append(p.Sections, sec)
			}
		}
	}
	p.Sections = append(p.Sections, view)
	if sec, ok := s.problemsSection(); ok {
		p.Sections = append(p.Sections, sec)
	}
	return p
}

func (s *State) tagNames() []string {
	names := make([]string, len(s.Sheet.Tags))
	for i, t := range s.Sheet.Tags {
		names[i] = t.Name
	}
	return names
}

// The rows the clip list and the box list show before they scroll.
const (
	clipRows = 6
	boxRows  = 8
)

func fnum(f toolshell.Field) toolshell.Field { return f.WithDigits(0) }

func (s *State) playbackSection() toolshell.Section {
	n := s.NFrames()
	play := "Play"
	if s.Play.Playing {
		play = "Pause"
	}
	frame := fnum(toolshell.Number("frame", "Frame", float64(s.Play.Frame))).Range(0, float64(max(n-1, 0)))
	fields := []toolshell.Field{
		toolshell.Dropdown("clip.sel", "Clip", s.Clip, s.tagNames()...).AsList(clipRows),
		toolshell.Button("play", play),
		frame,
		toolshell.Label("time", "Time", fmt.Sprintf("%d ms of %d ms", s.Play.Time.Milliseconds(), s.timeline().Total.Milliseconds())),
		toolshell.Checkbox("flip", "Flip H", s.Flip),
	}
	if !s.ReadOnly {
		fields = append(fields, toolshell.Dropdown("tool", "Tool", int(s.Tool), toolNames...))
	}
	return toolshell.Section{ID: "playback", Label: "Playback", Side: toolshell.SideLeft, Fields: fields}
}

// exportSection describes the Aseprite export of a read-only view.
func (s *State) exportSection() toolshell.Section {
	return toolshell.Section{ID: "export", Label: "Aseprite export", Fields: []toolshell.Field{
		toolshell.Label("export.mode", "Mode", "read-only"),
		toolshell.Label("export.frames", "Frames", fmt.Sprint(len(s.Sheet.Frames))),
		toolshell.Label("export.clips", "Clips", fmt.Sprint(len(s.Sheet.Tags))),
	}}
}

func (s *State) fileSection() toolshell.Section {
	def := 0
	opts := append([]string{"(none)"}, s.tagNames()...)
	for i, t := range s.Sheet.Tags {
		if t.Name == s.File.DefaultClip {
			def = i + 1
		}
	}
	px, py := 0.0, 0.0
	if p := s.File.Pivot; p != nil {
		px, py = float64(p.X), float64(p.Y)
	}
	return toolshell.Section{ID: "file", Label: "File", Fields: []toolshell.Field{
		toolshell.Dropdown("file.default", "Default clip", def, opts...),
		toolshell.Checkbox("file.haspivot", "Has pivot", s.File.Pivot != nil),
		fnum(toolshell.Number("file.px", "Pivot X", px)),
		fnum(toolshell.Number("file.py", "Pivot Y", py)),
		toolshell.Label("file.body", "Body", fmt.Sprintf("%d rects", len(s.File.Body))),
		toolshell.Button("file.clearbody", "Clear body"),
	}}
}

func (s *State) clipSection() toolshell.Section {
	c := clipEntry(s.File, s.ClipName())
	var clip animfile.Clip
	if c != nil {
		clip = *c
	}
	loop := LoopSheet
	if clip.Loop != nil {
		loop = LoopOff
		if *clip.Loop {
			loop = LoopOn
		}
	}
	fps, speed := 0.0, 0.0
	if clip.FPS != nil {
		fps = float64(*clip.FPS)
	}
	if clip.Speed != nil {
		speed = *clip.Speed
	}
	px, py := 0.0, 0.0
	if clip.Pivot != nil {
		px, py = float64(clip.Pivot.X), float64(clip.Pivot.Y)
	}
	var dur float64
	if d := s.FrameDurationsMS(); s.Play.Frame < len(d) {
		dur = float64(d[s.Play.Frame])
	}
	listed := "yes"
	if c == nil {
		listed = "no (keeps the sheet's loop and timing)"
	}
	fields := []toolshell.Field{toolshell.Label("clip.listed", "Listed in file", listed)}
	if c == nil {
		fields = append(fields, toolshell.Button("clip.add", "Add to file"))
	}
	fields = append(fields,
		toolshell.Dropdown("clip.loop", "Loop", loop, "Sheet", "Loop", "Once"),
		fnum(toolshell.Number("clip.fps", "FPS (0 = unset)", fps)),
		toolshell.Number("clip.speed", "Speed (0 = unset)", speed).WithDigits(2).WithStep(0.1),
		toolshell.Checkbox("clip.ownpivot", "Own pivot", clip.Pivot != nil),
		fnum(toolshell.Number("clip.px", "Pivot X", px)),
		fnum(toolshell.Number("clip.py", "Pivot Y", py)),
		fnum(toolshell.Number("clip.dur", "This frame (ms)", dur)),
		toolshell.Button("clip.cleardur", "Clear durations"),
	)
	return toolshell.Section{ID: "clip", Label: "Clip " + s.ClipName(), Fields: fields}
}

// boxLabel is the dropdown entry of a box.
func boxLabel(ref BoxRef, sh *animfile.Shape) string {
	if ref.Body {
		return fmt.Sprintf("body %d", ref.Index)
	}
	id := sh.ID
	if id == "" {
		id = "-"
	}
	frames := "all"
	if fr := sh.Frames; fr != nil {
		frames = fmt.Sprint(fr.From)
		if fr.To != nil {
			frames += ".." + fmt.Sprint(*fr.To)
		}
	}
	return fmt.Sprintf("%s %s %s", sh.Kind, id, frames)
}

// boxRefs lists the boxes of the shown clip in the dropdown's order: the body
// rects, then the shapes.
func (s *State) boxRefs() []BoxRef {
	var refs []BoxRef
	for i := range s.File.Body {
		refs = append(refs, BoxRef{Body: true, Index: i})
	}
	if c := clipEntry(s.File, s.ClipName()); c != nil {
		for i := range c.Shapes {
			refs = append(refs, BoxRef{Index: i})
		}
	}
	return refs
}

// boxListSection is the left section with the list of boxes of the shown
// clip, "(none)" first.
func (s *State) boxListSection() toolshell.Section {
	refs := s.boxRefs()
	opts := []string{"(none)"}
	sel := 0
	c := clipEntry(s.File, s.ClipName())
	for i, r := range refs {
		var sh *animfile.Shape
		if !r.Body {
			sh = &c.Shapes[r.Index]
		}
		opts = append(opts, boxLabel(r, sh))
		if s.Sel != nil && *s.Sel == r {
			sel = i + 1
		}
	}
	return toolshell.Section{ID: "boxlist", Label: "Boxes", Side: toolshell.SideLeft, Fields: []toolshell.Field{
		toolshell.Dropdown("box.sel", "Box", sel, opts...).AsList(boxRows),
	}}
}

// boxSection is the right section with the fields of the selected box; ok is
// false without a selection.
func (s *State) boxSection() (toolshell.Section, bool) {
	if s.Sel == nil {
		return toolshell.Section{}, false
	}
	fields := s.boxFields(clipEntry(s.File, s.ClipName()))
	if len(fields) == 0 {
		return toolshell.Section{}, false
	}
	return toolshell.Section{ID: "box", Label: "Box", Fields: fields}, true
}

func (s *State) boxFields(c *animfile.Clip) []toolshell.Field {
	r, err := rectOf(s.File, s.ClipName(), *s.Sel)
	if err != nil {
		return nil
	}
	var fields []toolshell.Field
	if !s.Sel.Body {
		sh := c.Shapes[s.Sel.Index]
		kind := 0
		if sh.Kind == KindHurt {
			kind = 1
		}
		all := sh.Frames == nil
		from, to, ranged := 0, 0, false
		if sh.Frames != nil {
			from = sh.Frames.From
			if sh.Frames.To != nil {
				to, ranged = *sh.Frames.To, true
			}
		}
		fields = append(fields,
			toolshell.Dropdown("box.kind", "Kind", kind, KindHit, KindHurt),
			toolshell.Text("box.id", "ID", sh.ID),
			toolshell.Checkbox("box.all", "All frames", all),
			fnum(toolshell.Number("box.from", "From", float64(from))),
			toolshell.Checkbox("box.range", "Range (to)", ranged),
			fnum(toolshell.Number("box.to", "To", float64(to))),
		)
	}
	fields = append(fields,
		fnum(toolshell.Number("box.x", "X", float64(r.X))),
		fnum(toolshell.Number("box.y", "Y", float64(r.Y))),
		fnum(toolshell.Number("box.w", "W", float64(r.W))),
		fnum(toolshell.Number("box.h", "H", float64(r.H))),
	)
	if !s.Sel.Body {
		fields = append(fields,
			toolshell.Button("box.copy", "Copy to this frame"),
			toolshell.Button("box.extend", "Extend to this frame"),
		)
	}
	return append(fields, toolshell.Button("box.delete", "Delete box"))
}

func (s *State) problemsSection() (toolshell.Section, bool) {
	if len(s.Findings) == 0 {
		return toolshell.Section{}, false
	}
	sec := toolshell.Section{ID: "problems", Label: "Problems"}
	for i, f := range s.Findings {
		sec.Fields = append(sec.Fields, toolshell.Label(fmt.Sprintf("problem.%d", i), strings.ToUpper(string(f.Level)), f.Msg))
	}
	return sec, true
}

// Set applies a changed widget. It returns an error for a value the format
// refuses; the file is not written then.
func (s *State) Set(id string, v json.RawMessage) error {
	if !s.Ready() {
		return fmt.Errorf("nothing to edit: %s", s.noDocMessage())
	}
	num := func() (int, error) {
		var f float64
		if err := json.Unmarshal(v, &f); err != nil {
			return 0, fmt.Errorf("%s: %w", id, err)
		}
		return int(f), nil
	}
	flt := func() (float64, error) {
		var f float64
		err := json.Unmarshal(v, &f)
		return f, err
	}
	on := func() (bool, error) {
		var b bool
		err := json.Unmarshal(v, &b)
		return b, err
	}
	clip := s.ClipName()
	switch id {
	case "clip.sel":
		i, err := num()
		if err != nil {
			return err
		}
		s.SelectClip(i)
		return nil
	case "play":
		if s.Play.Playing {
			s.Play.Playing = false
		} else {
			s.Play.Playing = true
			if c := s.timeline(); c.Loop == anim.Once && s.Play.Time >= c.Total {
				s.Play.Seek(c, 0) // a finished once clip restarts
			}
		}
		return nil
	case "frame":
		n, err := num()
		if err != nil {
			return err
		}
		s.Play.Playing = false
		s.Play.Seek(s.timeline(), n)
		return nil
	case "flip":
		b, err := on()
		s.Flip = b
		return err
	case "tool":
		n, err := num()
		if err != nil {
			return err
		}
		s.Tool = ToolKind(max(0, min(n, len(toolNames)-1)))
		return nil
	case "box.sel":
		n, err := num()
		if err != nil {
			return err
		}
		refs := s.boxRefs()
		if n <= 0 || n > len(refs) {
			s.Sel = nil
		} else {
			r := refs[n-1]
			s.Sel = &r
		}
		return nil
	}

	// Everything below writes the file.
	if strings.HasPrefix(id, "box.") && id != "box.sel" && s.Sel == nil {
		return nil
	}
	switch id {
	case "file.default":
		n, err := num()
		if err != nil {
			return err
		}
		name := ""
		if n > 0 && n <= len(s.Sheet.Tags) {
			name = s.Sheet.Tags[n-1].Name
		}
		return s.do(func(f *animfile.File) error { return SetDefaultClip(f, name) })
	case "file.haspivot":
		b, err := on()
		if err != nil {
			return err
		}
		var p *animfile.Point
		if b {
			p = &animfile.Point{}
		}
		return s.do(func(f *animfile.File) error { return SetFilePivot(f, p) })
	case "file.px", "file.py":
		n, err := num()
		if err != nil {
			return err
		}
		return s.do(func(f *animfile.File) error {
			if f.Pivot == nil {
				return fmt.Errorf("the file has no pivot: tick Has pivot first")
			}
			p := *f.Pivot
			setCoord(&p, id == "file.px", n)
			return SetFilePivot(f, &p)
		})
	case "file.clearbody":
		return s.do(ClearBody)
	case "clip.add":
		return s.do(func(f *animfile.File) error { return AddClipEntry(f, clip) })
	case "clip.loop":
		n, err := num()
		if err != nil {
			return err
		}
		return s.do(func(f *animfile.File) error { return SetLoop(f, clip, n) })
	case "clip.fps":
		n, err := num()
		if err != nil {
			return err
		}
		return s.do(func(f *animfile.File) error { return SetFPS(f, clip, n) })
	case "clip.speed":
		x, err := flt()
		if err != nil {
			return err
		}
		return s.do(func(f *animfile.File) error { return SetSpeed(f, clip, x) })
	case "clip.ownpivot":
		b, err := on()
		if err != nil {
			return err
		}
		var p *animfile.Point
		if b {
			cur := animfile.Point{}
			if fp := s.File.Pivot; fp != nil {
				cur = *fp
			}
			p = &cur
		}
		return s.do(func(f *animfile.File) error { return SetClipPivot(f, clip, p) })
	case "clip.px", "clip.py":
		n, err := num()
		if err != nil {
			return err
		}
		return s.do(func(f *animfile.File) error {
			c := clipEntry(f, clip)
			if c == nil || c.Pivot == nil {
				return fmt.Errorf("the clip has no pivot of its own: tick Own pivot first")
			}
			p := *c.Pivot
			setCoord(&p, id == "clip.px", n)
			return SetClipPivot(f, clip, &p)
		})
	case "clip.dur":
		n, err := num()
		if err != nil {
			return err
		}
		cur, frame := s.FrameDurationsMS(), s.Play.Frame
		return s.do(func(f *animfile.File) error { return SetFrameDuration(f, clip, cur, frame, n) })
	case "clip.cleardur":
		return s.do(func(f *animfile.File) error { return ClearDurations(f, clip) })
	}
	return s.setBox(id, num, on, v)
}

func setCoord(p *animfile.Point, x bool, n int) {
	if x {
		p.X = int32(n)
	} else {
		p.Y = int32(n)
	}
}

func (s *State) noDocMessage() string {
	switch {
	case s.File == nil:
		return "no document"
	case s.Sheet == nil:
		return "no sheet"
	}
	return "the sheet has no tags"
}

// setBox handles the widgets of the selected box.
func (s *State) setBox(id string, num func() (int, error), on func() (bool, error), v json.RawMessage) error {
	if !strings.HasPrefix(id, "box.") {
		return fmt.Errorf("unknown widget %q", id)
	}
	ref, clip, frame := *s.Sel, s.ClipName(), s.Play.Frame
	rect := func(set func(r *animfile.Rect, n int32)) error {
		n, err := num()
		if err != nil {
			return err
		}
		p, err := rectOf(s.File, clip, ref)
		if err != nil {
			return err
		}
		r := *p
		set(&r, int32(n))
		return s.do(func(f *animfile.File) error { return SetBoxRect(f, clip, ref, r) })
	}
	frames := func(mod func(all *bool, from *int, to **int)) error {
		sh, err := shapeOf(s.File, clip, ref)
		if err != nil {
			return err
		}
		all, from, to := sh.Frames == nil, 0, (*int)(nil)
		if sh.Frames != nil {
			from = sh.Frames.From
			if sh.Frames.To != nil {
				t := *sh.Frames.To
				to = &t
			}
		}
		mod(&all, &from, &to)
		return s.do(func(f *animfile.File) error { return SetShapeFrames(f, clip, ref, all, from, to) })
	}
	switch id {
	case "box.x":
		return rect(func(r *animfile.Rect, n int32) { r.X = n })
	case "box.y":
		return rect(func(r *animfile.Rect, n int32) { r.Y = n })
	case "box.w":
		return rect(func(r *animfile.Rect, n int32) { r.W = n })
	case "box.h":
		return rect(func(r *animfile.Rect, n int32) { r.H = n })
	case "box.delete":
		return s.DeleteSelected()
	case "box.kind":
		n, err := num()
		if err != nil {
			return err
		}
		kind := KindHit
		if n == 1 {
			kind = KindHurt
		}
		return s.do(func(f *animfile.File) error { return SetShapeKind(f, clip, ref, kind) })
	case "box.id":
		var text string
		if err := json.Unmarshal(v, &text); err != nil {
			return err
		}
		return s.do(func(f *animfile.File) error { return SetShapeID(f, clip, ref, text) })
	case "box.all":
		b, err := on()
		if err != nil {
			return err
		}
		return frames(func(all *bool, from *int, _ **int) {
			*all = b
			if !b && *from == 0 {
				*from = frame
			}
		})
	case "box.from":
		n, err := num()
		if err != nil {
			return err
		}
		return frames(func(all *bool, from *int, _ **int) { *all, *from = false, n })
	case "box.range":
		b, err := on()
		if err != nil {
			return err
		}
		return frames(func(all *bool, from *int, to **int) {
			*all = false
			if !b {
				*to = nil
			} else if *to == nil {
				t := *from
				*to = &t
			}
		})
	case "box.to":
		n, err := num()
		if err != nil {
			return err
		}
		return frames(func(all *bool, _ *int, to **int) { *all = false; *to = &n })
	case "box.copy":
		var made BoxRef
		err := s.do(func(f *animfile.File) (err error) {
			made, err = CopyToFrame(f, clip, ref, frame)
			return err
		})
		if err == nil {
			s.Sel = &made
		}
		return err
	case "box.extend":
		return s.do(func(f *animfile.File) error { return ExtendToFrame(f, clip, ref, frame) })
	}
	return fmt.Errorf("unknown widget %q", id)
}
