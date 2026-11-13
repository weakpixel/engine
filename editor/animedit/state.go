package animedit

import (
	"errors"
	"fmt"
	"time"

	"github.com/weakpixel/engine/anim"
	"github.com/weakpixel/engine/anim/animfile"
	"github.com/weakpixel/engine/gfx"
)

// ToolKind is the tool the mouse uses in the viewport.
type ToolKind int

// The tools, in the order of the panel's dropdown.
const (
	ToolSelect ToolKind = iota
	ToolPivot
	ToolHit
	ToolHurt
	ToolBody
)

// toolNames are the dropdown entries of the tools.
var toolNames = []string{"Select (S)", "Pivot (P)", "Hit box (H)", "Hurt box (U)", "Body (B)"}

// ErrStale is returned by every edit while the document cannot be parsed:
// the tool keeps showing the last good version and never writes over the
// user's broken text.
var ErrStale = errors.New("the document has errors: fix it in the text editor first")

// ErrReadOnly is returned by every edit of a read-only view, an Aseprite
// export opened in the anim tool: it is somebody else's file, shown to play
// its clips.
var ErrReadOnly = errors.New("this is an Aseprite export, shown read-only: edit a .anim.json definition instead")

// State is everything the tool knows: the definition (the model), its sheet,
// what is selected and played. It has no Ebiten in it, so tests drive it
// directly; the tool in tool.go feeds it the mouse and the keys.
type State struct {
	// File is the decoded definition; Sheet is what it refers to (nil when
	// the sheet cannot be read, then SheetErr says why).
	File     *animfile.File
	Sheet    *animfile.Sheet
	SheetErr string
	// Res is File resolved against Sheet, nil when Resolve found errors;
	// Findings are the errors, warnings and notes of animfile.Diagnose.
	Res      *animfile.Resolved
	Findings []animfile.Finding

	// Clip indexes Sheet.Tags.
	Clip int
	Play Playback
	Flip bool
	Tool ToolKind
	// ReadOnly is set when the file is an Aseprite export (File is then an
	// empty definition of it): clips play, nothing can be edited.
	ReadOnly bool
	// Sel is the selected box of the clip, if any.
	Sel *BoxRef
	// Drag is the mouse gesture in progress.
	Drag *Drag
	// Geom is the sprite of the shown frame, set by the tool each tick; the
	// pivot tool needs it to turn a point into trimmed-image pixels.
	Geom Geom
	// Stale is the parse error of a document that could not be loaded, "" if
	// the shown file is the document.
	Stale string

	emit func(text []byte)
}

// Geom is what the tool needs to know of the shown sprite: its untrimmed
// size, where the trimmed image starts in it and how big that image is, and
// the default pivot the sprite is drawn around when the file has none (a
// fraction of Size).
type Geom struct {
	Size, Offset, Trim, Origin gfx.Vec2
}

// NewState returns a state that sends every edit to emit as the text of the
// new document version.
func NewState(emit func(text []byte)) *State { return &State{emit: emit} }

// SetFile replaces the definition (a new document version) and clears a stale
// mark. The selection and the clip survive when they are still valid.
func (s *State) SetFile(f *animfile.File) {
	s.File, s.Stale = f, ""
	s.Drag = nil
	s.refresh()
}

// SetSheet replaces the sheet, or records why there is none.
func (s *State) SetSheet(sh *animfile.Sheet, err error) {
	s.Sheet, s.SheetErr = sh, ""
	if err != nil {
		s.Sheet, s.SheetErr = nil, err.Error()
	}
	s.refresh()
}

// MarkStale keeps the shown file and notes that the document does not parse.
func (s *State) MarkStale(err error) {
	s.Stale = err.Error()
	s.Drag = nil
}

// Ready reports whether there is a definition and a sheet with tags to show.
func (s *State) Ready() bool { return s.File != nil && s.Sheet != nil && len(s.Sheet.Tags) > 0 }

// refresh resolves the file again and keeps Clip, Sel and the frame valid.
func (s *State) refresh() {
	s.Res, s.Findings = nil, nil
	if s.File == nil {
		return
	}
	if s.Sheet == nil {
		s.Findings = []animfile.Finding{{Level: animfile.LevelError, Msg: "sheet " + fmt.Sprintf("%q", s.File.Sheet) + ": " + s.SheetErr}}
		return
	}
	s.Res, s.Findings = animfile.Diagnose(s.File, s.Sheet)
	s.Clip = max(0, min(s.Clip, len(s.Sheet.Tags)-1))
	if s.Sel != nil && !s.validRef(*s.Sel) {
		s.Sel = nil
	}
	s.Play.Seek(s.timeline(), s.Play.Frame)
}

func (s *State) validRef(r BoxRef) bool {
	if r.Body {
		return r.Index >= 0 && r.Index < len(s.File.Body)
	}
	c := clipEntry(s.File, s.ClipName())
	return c != nil && r.Index >= 0 && r.Index < len(c.Shapes)
}

// ClipName is the name of the tag being shown, "" without a sheet.
func (s *State) ClipName() string {
	if s.Sheet == nil || s.Clip >= len(s.Sheet.Tags) {
		return ""
	}
	return s.Sheet.Tags[s.Clip].Name
}

// resolvedClip is the clip as it plays: the resolved one, or without a
// valid definition the sheet's own timing.
func (s *State) resolvedClip() animfile.ResolvedClip {
	if s.Res != nil && s.Clip < len(s.Res.Clips) {
		return s.Res.Clips[s.Clip]
	}
	tag := s.Sheet.PlayOrder()[s.Clip]
	return animfile.ResolvedClip{Name: tag.Name, Loop: tag.Loop, Durations: tag.Durations}
}

// timeline is the clip to play.
func (s *State) timeline() *anim.Clip {
	if !s.Ready() {
		return &anim.Clip{}
	}
	rc := s.resolvedClip()
	return Timeline(&rc)
}

// NFrames is the number of frames of the shown clip.
func (s *State) NFrames() int {
	if !s.Ready() {
		return 0
	}
	return len(s.resolvedClip().Durations)
}

// Pivot is the pivot of the shown clip: its own, else the file's, nil if
// neither.
func (s *State) Pivot() *animfile.Point {
	if c := clipEntry(s.File, s.ClipName()); c != nil && c.Pivot != nil {
		return c.Pivot
	}
	return s.File.Pivot
}

// pivotBase is the pivot in trimmed-image pixels that world (0, 0) stands
// for: the shown pivot, else the sprite's default one.
func (s *State) pivotBase() animfile.Point {
	if p := s.Pivot(); p != nil {
		return *p
	}
	g := s.Geom
	return pixel(gfx.Vec2{X: g.Origin.X*g.Size.X - g.Offset.X, Y: g.Origin.Y*g.Size.Y - g.Offset.Y})
}

// FrameDurationsMS are the shown clip's durations in whole ms.
func (s *State) FrameDurationsMS() []int {
	rc := s.resolvedClip()
	out := make([]int, len(rc.Durations))
	for i, d := range rc.Durations {
		out[i] = int(d / time.Millisecond)
	}
	return out
}

// SelectClip shows tag i and clears the selection.
func (s *State) SelectClip(i int) {
	if s.Sheet == nil || i < 0 || i >= len(s.Sheet.Tags) {
		return
	}
	s.Clip, s.Sel, s.Drag = i, nil, nil
	s.Play = Playback{}
}

// Boxes are the boxes on the current frame.
func (s *State) Boxes() []Box {
	return ActiveBoxes(s.File, s.ClipName(), s.Play.Frame, s.NFrames())
}

// do runs an edit of the file, and when it succeeds sends the new text and
// resolves again. A refused edit leaves the file as it was.
func (s *State) do(edit func(f *animfile.File) error) error {
	if s.ReadOnly {
		return ErrReadOnly
	}
	if s.Stale != "" {
		return ErrStale
	}
	if !s.Ready() {
		return errors.New("there is no sheet to edit against")
	}
	s.Play.Playing = false
	if err := edit(s.File); err != nil {
		return err
	}
	data, err := animfile.Encode(s.File)
	if err != nil {
		return err
	}
	s.refresh()
	if s.emit != nil {
		s.emit(data)
	}
	return nil
}
