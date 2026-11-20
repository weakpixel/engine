package sheetedit

import (
	"encoding/json"
	"fmt"
	"image"

	"github.com/weakpixel/engine/editor/toolshell"
	"github.com/weakpixel/engine/editor/viewkit"
	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/sheetfile"
)

// Ids of the panel's widgets.
const (
	IDFrameW  = "sheet.frameW"
	IDFrameH  = "sheet.frameH"
	IDMargin  = "sheet.margin"
	IDSpacing = "sheet.spacing"
	// IDLabels and IDFrame are the label mode and the frame list of the
	// Frames section (viewkit.Frames with the prefix "sheet").
	IDLabels = "sheet.labels"
	IDFrame  = "sheet.frame"
)

// MaxListed is the most frames the Frames section lists and the overlay
// outlines one by one. A frame size of a few pixels on a big image (while
// typing a width, say) would make a list nobody can use; above it only the
// grid is drawn.
const MaxListed = 4096

// State is everything the tool knows: the sheet (the model), the size of its
// image, the frame under the mouse and the frames as listed and selected. It has no Ebiten in it, so tests drive
// it directly; the tool in tool.go feeds it the texture and the mouse.
type State struct {
	// Model holds the decoded sheet (File, nil until the first valid
	// document), its stale mark and the edit path.
	toolshell.Model[*sheetfile.File]
	// Size is the size of the image in pixels, zero while it is unknown;
	// ImageErr says why when it could not be loaded.
	Size     image.Point
	ImageErr string
	// Hover is the index of the frame under the mouse, -1 for none.
	Hover int
	// Frames is the grid's frames as the overlay and the Frames section
	// show them: index labels, the label mode, the selected frame. Its list
	// follows the grid and is empty above [MaxListed] frames.
	Frames viewkit.Frames
}

// NewState returns a state that sends every edit to emit as the text of the
// new document version.
func NewState(emit func(text []byte)) *State {
	s := &State{Hover: -1, Frames: viewkit.NewFrames("sheet")}
	s.Frames.MinPx = minCellPx
	s.Encode, s.Emit = sheetfile.Encode, emit
	return s
}

// syncFrames lists the frames of the grid as it is now. The selection stays
// while its index still exists.
func (s *State) syncFrames() {
	g := s.Grid()
	n := g.Count()
	if n > MaxListed {
		n = 0
	}
	list := make([]viewkit.Frame, n)
	for i := range n {
		r := g.Rect(i)
		list[i] = viewkit.Frame{
			Index: i,
			Rect:  gfx.Rect{Min: gfx.Vec2{X: float32(r.Min.X), Y: float32(r.Min.Y)}, Max: gfx.Vec2{X: float32(r.Max.X), Y: float32(r.Max.Y)}},
		}
	}
	s.Frames.SetList(list)
}

// SetFile replaces the sheet (a new document version) and clears a stale
// mark.
func (s *State) SetFile(f *sheetfile.File) {
	s.Model.SetFile(f)
	s.Hover = -1
	s.syncFrames()
}

// SetImage records the image size, or why there is none.
func (s *State) SetImage(size image.Point, err error) {
	s.Size, s.ImageErr = size, ""
	if err != nil {
		s.Size, s.ImageErr = image.Point{}, err.Error()
	}
	s.syncFrames()
}

// Grid is the layout of the sheet over the image; it has no frames while the
// size is unknown.
func (s *State) Grid() sheetfile.Grid {
	if s.File == nil || s.Size.X <= 0 || s.Size.Y <= 0 {
		return sheetfile.Grid{File: s.File}
	}
	return s.File.Layout(s.Size)
}

// SetHover sets the frame under the image pixel p; ok is false when the
// pointer is not over the image.
func (s *State) SetHover(p image.Point, ok bool) {
	s.Hover = -1
	if !ok || s.File == nil {
		return
	}
	if i, hit := s.Grid().At(p); hit {
		s.Hover = i
	}
}

// Findings are the problems of the sheet against its image, for the
// Problems section.
func (s *State) Findings() []string {
	var out []string
	if s.ImageErr != "" {
		out = append(out, fmt.Sprintf("image %q: %s", s.File.Image, s.ImageErr))
	} else if s.Size.X > 0 && s.Grid().Count() == 0 {
		out = append(out, fmt.Sprintf("a %dx%d frame does not fit in the %dx%d image with margin %d", s.File.FrameW, s.File.FrameH, s.Size.X, s.Size.Y, s.File.Margin))
	}
	return out
}

// Set applies a changed widget. It returns an error for a value the format
// refuses; the file is not written then.
func (s *State) Set(id string, v json.RawMessage) error {
	if s.File == nil {
		return toolshell.ErrNoDocument
	}
	// The label mode and the selection are views, not edits: they work on
	// a stale or read-only document too.
	if ok, err := s.Frames.Set(id, v); ok {
		return err
	}
	var f float64
	if err := json.Unmarshal(v, &f); err != nil {
		return fmt.Errorf("%s: %w", id, err)
	}
	n := int(f)
	var field *int
	least := 0
	switch id {
	case IDFrameW:
		field, least = &s.File.FrameW, 1
	case IDFrameH:
		field, least = &s.File.FrameH, 1
	case IDMargin:
		field = &s.File.Margin
	case IDSpacing:
		field = &s.File.Spacing
	default:
		return fmt.Errorf("sheet: no widget %q", id)
	}
	if err := s.Editable(); err != nil {
		return err
	}
	if f != float64(n) || n < least {
		return fmt.Errorf("sheet: %s must be a whole number of at least %d, got %v", id, least, f)
	}
	if *field == n {
		return nil
	}
	old := *field
	err := s.Do(func(*sheetfile.File) error { *field = n; return nil })
	if err != nil {
		*field = old // the encoder refused it: nothing was written
	}
	s.syncFrames()
	return err
}

// Panel is the panel of the tool as it is now, without the view section the
// shell adds: the tool puts it in at the place it wants (before the
// problems).
func (s *State) Panel(title string, view toolshell.Section) toolshell.Panel {
	p := toolshell.Panel{Title: title}
	if s.Stale != "" {
		p.Status = "document has errors: " + s.Stale
	}
	if s.File != nil {
		p.Sections = append(p.Sections, s.framesSection(), s.sheetSection(), s.infoSection())
	}
	p.Sections = append(p.Sections, view)
	if s.File != nil {
		if found := s.Findings(); len(found) > 0 {
			sec := toolshell.Section{ID: "problems", Label: "Problems"}
			for i, msg := range found {
				sec.Fields = append(sec.Fields, toolshell.Label(fmt.Sprintf("problem.%d", i), "ERROR", msg))
			}
			p.Sections = append(p.Sections, sec)
		}
	}
	return p
}

// framesSection is the left Frames section: label mode and frame list, and
// a note instead of the list when the grid has too many frames for one.
func (s *State) framesSection() toolshell.Section {
	sec := s.Frames.Section()
	if n := s.Grid().Count(); n > MaxListed {
		sec.Fields = append(sec.Fields, toolshell.Label("sheet.unlisted", "Note", fmt.Sprintf("%d frames are too many to list (at most %d)", n, MaxListed)))
	}
	return sec
}

// shown is the frame the Grid section describes: the selected one, else the
// one under the mouse, with which of the two it is; -1 for none.
func (s *State) shown() (int, string) {
	if s.Frames.Sel >= 0 {
		return s.Frames.Sel, "selected"
	}
	if s.Hover >= 0 {
		return s.Hover, "hover"
	}
	return -1, ""
}

func (s *State) sheetSection() toolshell.Section {
	f := s.File
	px := func(id, label string, v int, least float64) toolshell.Field {
		return toolshell.Number(id, label, float64(v)).WithDigits(0).AtLeast(least)
	}
	return toolshell.Section{ID: "sheet", Label: "Sheet", Fields: []toolshell.Field{
		toolshell.Label("sheet.image", "Image", f.Image),
		px(IDFrameW, "Frame width", f.FrameW, 1),
		px(IDFrameH, "Frame height", f.FrameH, 1),
		px(IDMargin, "Margin", f.Margin, 0).WithHint("Border around the grid in pixels"),
		px(IDSpacing, "Spacing", f.Spacing, 0).WithHint("Gap between frames in pixels"),
	}}
}

func (s *State) infoSection() toolshell.Section {
	g := s.Grid()
	size, grid, frames, left, hover := "unknown", "–", "–", "–", "–"
	if s.Size.X > 0 {
		size = fmt.Sprintf("%d × %d px", s.Size.X, s.Size.Y)
		grid = fmt.Sprintf("%d × %d", g.Cols, g.Rows)
		frames = fmt.Sprint(g.Count())
		if g.Count() > 0 {
			u := g.Used()
			left = fmt.Sprintf("%d px right, %d px bottom", s.Size.X-u.Max.X-s.File.Margin, s.Size.Y-u.Max.Y-s.File.Margin)
		}
	}
	if i, which := s.shown(); i >= 0 && i < g.Count() {
		r := g.Rect(i)
		hover = fmt.Sprintf("%d (%s)  x %d, y %d, %d × %d", i, which, r.Min.X, r.Min.Y, r.Dx(), r.Dy())
	} else if s.Size.X > 0 {
		hover = "click a frame to select it"
	}
	return toolshell.Section{ID: "info", Label: "Grid", Side: toolshell.SideLeft, Fields: []toolshell.Field{
		toolshell.Label("info.size", "Image", size),
		toolshell.Label("info.grid", "Columns × rows", grid),
		toolshell.Label("info.count", "Frames", frames),
		toolshell.Label("info.left", "Unused", left),
		toolshell.Label("info.hover", "Frame", hover),
	}}
}
