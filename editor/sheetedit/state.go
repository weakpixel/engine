package sheetedit

import (
	"encoding/json"
	"errors"
	"fmt"
	"image"

	"github.com/weakpixel/engine/editor/toolshell"
	"github.com/weakpixel/engine/sheetfile"
)

// ErrStale is returned by every edit while the document cannot be parsed:
// the tool keeps showing the last good version and never writes over the
// user's broken text.
var ErrStale = errors.New("the document has errors: fix it in the text editor first")

// Ids of the panel's widgets.
const (
	IDFrameW  = "sheet.frameW"
	IDFrameH  = "sheet.frameH"
	IDMargin  = "sheet.margin"
	IDSpacing = "sheet.spacing"
)

// State is everything the tool knows: the sheet (the model), the size of its
// image and the frame under the mouse. It has no Ebiten in it, so tests drive
// it directly; the tool in tool.go feeds it the texture and the mouse.
type State struct {
	// File is the decoded sheet, nil until the first valid document.
	File *sheetfile.File
	// Size is the size of the image in pixels, zero while it is unknown;
	// ImageErr says why when it could not be loaded.
	Size     image.Point
	ImageErr string
	// Stale is the parse error of a document that could not be loaded, ""
	// if the shown file is the document.
	Stale string
	// Hover is the index of the frame under the mouse, -1 for none.
	Hover int

	emit func(text []byte)
}

// NewState returns a state that sends every edit to emit as the text of the
// new document version.
func NewState(emit func(text []byte)) *State { return &State{emit: emit, Hover: -1} }

// SetFile replaces the sheet (a new document version) and clears a stale
// mark.
func (s *State) SetFile(f *sheetfile.File) {
	s.File, s.Stale = f, ""
	s.Hover = -1
}

// MarkStale keeps the shown sheet and notes that the document does not parse.
func (s *State) MarkStale(err error) { s.Stale = err.Error() }

// SetImage records the image size, or why there is none.
func (s *State) SetImage(size image.Point, err error) {
	s.Size, s.ImageErr = size, ""
	if err != nil {
		s.Size, s.ImageErr = image.Point{}, err.Error()
	}
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
		return errors.New("nothing to edit: no valid document yet")
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
	if s.Stale != "" {
		return ErrStale
	}
	if f != float64(n) || n < least {
		return fmt.Errorf("sheet: %s must be a whole number of at least %d, got %v", id, least, f)
	}
	if *field == n {
		return nil
	}
	old := *field
	*field = n
	data, err := sheetfile.Encode(s.File)
	if err != nil {
		*field = old
		return err
	}
	if s.emit != nil {
		s.emit(data)
	}
	return nil
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
		p.Sections = append(p.Sections, s.sheetSection(), s.infoSection())
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
	if s.Hover >= 0 {
		r := g.Rect(s.Hover)
		hover = fmt.Sprintf("%d  (x %d, y %d, %d × %d)", s.Hover, r.Min.X, r.Min.Y, r.Dx(), r.Dy())
	}
	return toolshell.Section{ID: "info", Label: "Frames", Fields: []toolshell.Field{
		toolshell.Label("info.size", "Image", size),
		toolshell.Label("info.grid", "Columns × rows", grid),
		toolshell.Label("info.count", "Frames", frames),
		toolshell.Label("info.left", "Unused", left),
		toolshell.Label("info.hover", "Under mouse", hover),
	}}
}
