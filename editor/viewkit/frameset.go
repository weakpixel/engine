package viewkit

import (
	"encoding/json"
	"fmt"
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/weakpixel/engine/editor/toolshell"
)

// Colors of the frame overlay.
var (
	colFrame    = color.RGBA{R: 60, G: 100, B: 170, A: 170}
	colHover    = color.RGBA{R: 255, G: 255, B: 255, A: 255}
	colSelected = color.RGBA{R: 255, G: 230, B: 60, A: 255}
)

// FrameRows is the rows the frame list shows before it scrolls.
const FrameRows = 8

// Frames is the frames over a texture as a tool shows them: every frame
// outlined and labelled, the one under the mouse and the selected one
// highlighted, and a left section with the label mode and a list to pick a
// frame from. Its widget ids are Prefix + ".labels" and Prefix + ".frame".
type Frames struct {
	// Prefix starts the ids of the widgets ("atlas", "sheet").
	Prefix string
	// List is the frames in file order.
	List []Frame
	// Hover and Sel index List, -1 for none.
	Hover, Sel int
	// Mode is what each frame is labelled with.
	Mode LabelMode

	prevDown bool
	labels   Labels
}

// NewFrames returns frames with the widget id prefix and nothing hovered or
// selected.
func NewFrames(prefix string) Frames { return Frames{Prefix: prefix, Hover: -1, Sel: -1} }

// SetList replaces the frames (a reload) and keeps the selection while it
// still exists.
func (f *Frames) SetList(list []Frame) {
	f.List = list
	f.Hover = min(f.Hover, len(list)-1)
	if f.Sel >= len(list) {
		f.Sel = -1
	}
}

// Update sets the frame under the mouse; a click (not a pan) selects it, or
// clears the selection when it is the selected one or none.
func (f *Frames) Update(s *toolshell.Shell) {
	f.Hover = FrameAt(f.List, s.MouseWorld())
	down := s.Mouse().Buttons&1 != 0
	if down && !f.prevDown && !s.PanHeld() {
		if f.Hover == f.Sel {
			f.Sel = -1
		} else {
			f.Sel = f.Hover
		}
	}
	f.prevDown = down
}

// Shown is the frame a panel describes: the selected one, else the one under
// the mouse.
func (f *Frames) Shown() (Frame, bool) {
	for _, i := range []int{f.Sel, f.Hover} {
		if i >= 0 && i < len(f.List) {
			return f.List[i], true
		}
	}
	return Frame{}, false
}

// Draw outlines the frames on screen and labels those wide enough, then
// highlights the hovered and the selected frame with their full label.
func (f *Frames) Draw(s *toolshell.Shell, screen ebiten.FinalScreen) {
	b := screen.Bounds()
	w, h := float32(b.Dx()), float32(b.Dy())
	for i, fr := range f.List {
		r := s.WindowRect(fr.Rect)
		if r.Max.X < 0 || r.Max.Y < 0 || r.Min.X > w || r.Min.Y > h {
			continue // off screen
		}
		if r.Max.X-r.Min.X < 3 && i != f.Hover && i != f.Sel {
			continue // a frame this small is only a smudge of outlines
		}
		DrawOutline(s, screen, r, colFrame)
		if text := f.Mode.Text(fr); text != "" && f.labels.Fits(text, r.Max.X-r.Min.X) {
			f.labels.Draw(screen, text, r.Min.X+1, r.Min.Y+1)
		}
	}
	for _, i := range []int{f.Hover, f.Sel} {
		if i < 0 || i >= len(f.List) {
			continue
		}
		c := colHover
		if i == f.Sel {
			c = colSelected
		}
		r := s.WindowRect(f.List[i].Rect)
		DrawOutline(s, screen, r, c)
		DrawOutline(s, screen, s.WindowRect(f.List[i].Rect.Grow(1)), c)
		f.labels.Draw(screen, LabelBoth.Text(f.List[i]), r.Min.X+1, r.Min.Y+1)
	}
}

// Section is the left section to pick frames from: the label mode and the
// list of frames, "(none)" first (index 0 clears the selection).
func (f *Frames) Section() toolshell.Section {
	options := make([]string, 0, len(f.List)+1)
	options = append(options, "(none)")
	for _, fr := range f.List {
		if fr.Name == "" {
			options = append(options, fmt.Sprint(fr.Index))
		} else {
			options = append(options, fmt.Sprintf("%d %s", fr.Index, fr.Name))
		}
	}
	return toolshell.Section{ID: "frames", Label: "Frames", Side: toolshell.SideLeft, Fields: []toolshell.Field{
		toolshell.Dropdown(f.Prefix+".labels", "Labels", int(f.Mode), LabelModeNames...),
		toolshell.Dropdown(f.Prefix+".frame", "Frame", f.Sel+1, options...).AsList(FrameRows),
	}}
}

// Set applies a widget of the section; ok is false for any other id.
func (f *Frames) Set(id string, val json.RawMessage) (ok bool, err error) {
	var i int
	switch id {
	case f.Prefix + ".labels":
		if err := json.Unmarshal(val, &i); err != nil || i < 0 || i >= len(LabelModeNames) {
			return true, fmt.Errorf("%s: bad label mode %s", f.Prefix, val)
		}
		f.Mode = LabelMode(i)
	case f.Prefix + ".frame":
		// The same selection as a click on the frame; 0 is "(none)". Unlike a
		// click, choosing the selected frame again does not clear it.
		if err := json.Unmarshal(val, &i); err != nil || i < 0 || i > len(f.List) {
			return true, fmt.Errorf("%s: bad frame %s", f.Prefix, val)
		}
		f.Sel = i - 1
	default:
		return false, nil
	}
	return true, nil
}
