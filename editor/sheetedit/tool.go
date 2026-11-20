package sheetedit

import (
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"math"
	"path"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/weakpixel/engine/editor/toolshell"
	"github.com/weakpixel/engine/editor/viewkit"
	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/render"
	"github.com/weakpixel/engine/sheetfile"
)

// Colors of the overlay.
var (
	colGrid     = color.RGBA{R: 255, G: 255, B: 255, A: 150}
	colUsed     = color.RGBA{R: 90, G: 150, B: 255, A: 255}
	colHover    = color.RGBA{R: 255, G: 230, B: 60, A: 255}
	colHoverFil = color.RGBA{R: 255, G: 230, B: 60, A: 60}
	colLeftover = color.RGBA{R: 0, G: 0, B: 0, A: 110}
)

// minCellPx is the smallest cell, in window pixels, whose outline is drawn:
// below it the grid is a blur, so only the border of the grid is.
const minCellPx = 4

// Tool is the sheet tool. It implements [toolshell.DocumentTool],
// [toolshell.Syncer] and the shell's optional Check.
type Tool struct {
	shell *toolshell.Shell
	key   string
	st    *State

	tex      viewkit.Texture
	imageKey string
	size     gfx.Vec2 // the image size the camera was last framed on
}

// New returns the sheet tool.
func New() toolshell.Tool { return &Tool{} }

// Check reports whether the tool can open the file at asset key.
func (*Tool) Check(key string) error {
	if !strings.HasSuffix(key, sheetfile.Suffix) {
		return fmt.Errorf("sheet: %s is not a %s file", key, sheetfile.Suffix)
	}
	return nil
}

// Open implements [toolshell.Tool].
func (t *Tool) Open(s *toolshell.Shell, key string) error {
	if err := t.Check(key); err != nil {
		return err
	}
	t.shell, t.key = s, key
	t.st = NewState(s.Edit)
	t.tex.Spawn(s, gfx.SpriteHandle{}) // the image comes with the first document
	return nil
}

// Load implements [toolshell.DocumentTool]: a new version of the document.
// One that does not parse leaves the shown sheet as it is and marks it stale.
func (t *Tool) Load(text []byte) error {
	f, err := sheetfile.Decode(text)
	if err != nil {
		t.st.MarkStale(err)
		return fmt.Errorf("sheet: %w", err)
	}
	if key := path.Join(path.Dir(t.key), f.Image); key != t.imageKey {
		t.loadImage(key)
	}
	t.st.SetFile(f)
	return nil
}

// loadImage loads the sheet's texture. A missing image keeps its (failed)
// handle, which draws the placeholder and recovers when the file appears.
func (t *Tool) loadImage(key string) {
	t.imageKey = key
	h, err := render.LoadSpriteSync(t.shell.Engine().Assets().Global(), key)
	t.size = gfx.Vec2{}
	t.st.SetImage(image.Point{}, err)
	t.tex.SetSprite(t.shell, h)
}

// Sync implements [toolshell.Syncer]: it follows a reloaded image and keeps
// it on screen (see [viewkit.Texture.Sync]).
func (t *Tool) Sync(s *toolshell.Shell) {
	st := t.st
	if st == nil || st.File == nil {
		return
	}
	size := t.tex.Size(s)
	if size != t.size {
		t.size = size
		if size.X > 0 && size.Y > 0 {
			st.SetImage(image.Pt(int(size.X), int(size.Y)), nil)
			s.SetContent(gfx.Rect{Max: size})
			s.Frame(gfx.Rect{Max: size})
		}
	}
	t.tex.Sync(s)
}

// Update implements [toolshell.Tool]: the frame under the mouse, and a
// click (not a pan) that selects it or clears the selection.
func (t *Tool) Update(s *toolshell.Shell) {
	if t.st == nil {
		return
	}
	w := s.MouseWorld()
	p := image.Pt(int(math.Floor(float64(w.X))), int(math.Floor(float64(w.Y))))
	t.st.SetHover(p, p.In(image.Rectangle{Max: t.st.Size}))
	t.st.Frames.Update(s)
}

// Panel implements [toolshell.Tool].
func (t *Tool) Panel() toolshell.Panel {
	return t.st.Panel(path.Base(t.key), t.shell.ViewSection())
}

// Set implements [toolshell.Tool].
func (t *Tool) Set(id string, v json.RawMessage) error { return t.st.Set(id, v) }

// Draw implements [toolshell.Tool]: the unused rest of the image dimmed,
// the border of the grid, then the frames (see [viewkit.Frames.Draw]), in
// window pixels.
func (t *Tool) Draw(s *toolshell.Shell, screen ebiten.FinalScreen) {
	st := t.st
	if st == nil || st.File == nil || st.Size.X <= 0 {
		return
	}
	fade := func(c color.RGBA) color.RGBA {
		if st.Stale == "" {
			return c
		}
		return color.RGBA{R: c.R / 3, G: c.G / 3, B: c.B / 3, A: c.A / 3}
	}
	rect := func(r image.Rectangle, c color.RGBA, w float32) {
		win := s.WindowRect(gfx.Rect{Min: gfx.Vec2{X: float32(r.Min.X), Y: float32(r.Min.Y)}, Max: gfx.Vec2{X: float32(r.Max.X), Y: float32(r.Max.Y)}})
		x, y, ww, hh := win.Min.X, win.Min.Y, win.Max.X-win.Min.X, win.Max.Y-win.Min.Y
		c = fade(c)
		s.FillRect(screen, x, y, ww, w, c)
		s.FillRect(screen, x, y+hh-w, ww, w, c)
		s.FillRect(screen, x, y, w, hh, c)
		s.FillRect(screen, x+ww-w, y, w, hh, c)
	}
	g := st.Grid()
	if g.Count() == 0 {
		return
	}
	used := g.Used()
	t.leftovers(s, screen, used, fade(colLeftover))
	rect(used, colUsed, 1)
	if len(st.Frames.List) > 0 {
		// Outlines, index labels, hover and selection, as the atlas
		// previewer draws them; cells under minCellPx are not outlined.
		st.Frames.Draw(s, screen)
		return
	}
	// Too many frames to list: the visible cells only, without labels.
	cell := s.WindowRect(gfx.Rect{Max: gfx.Vec2{X: float32(st.File.FrameW), Y: float32(st.File.FrameH)}})
	if cell.Max.X-cell.Min.X >= minCellPx && cell.Max.Y-cell.Min.Y >= minCellPx {
		c0, c1, r0, r1 := t.visible(s, g)
		for r := r0; r <= r1; r++ {
			for c := c0; c <= c1; c++ {
				rect(g.Rect(r*g.Cols+c), colGrid, 1)
			}
		}
	}
	if st.Hover >= 0 && st.Hover < g.Count() {
		r := g.Rect(st.Hover)
		win := s.WindowRect(gfx.Rect{Min: gfx.Vec2{X: float32(r.Min.X), Y: float32(r.Min.Y)}, Max: gfx.Vec2{X: float32(r.Max.X), Y: float32(r.Max.Y)}})
		s.FillRect(screen, win.Min.X, win.Min.Y, win.Max.X-win.Min.X, win.Max.Y-win.Min.Y, fade(colHoverFil))
		rect(r, colHover, 2)
	}
}

// visible is the range of columns and rows that is on screen.
func (t *Tool) visible(s *toolshell.Shell, g sheetfile.Grid) (c0, c1, r0, r1 int) {
	f := g.File
	view := s.Engine().Camera().DrawBounds(s.ScreenSize())
	stepX, stepY := float64(f.FrameW+f.Spacing), float64(f.FrameH+f.Spacing)
	c0 = max(0, int(math.Floor((float64(view.Min.X)-float64(f.Margin))/stepX)))
	c1 = min(g.Cols-1, int(math.Floor((float64(view.Max.X)-float64(f.Margin))/stepX)))
	r0 = max(0, int(math.Floor((float64(view.Min.Y)-float64(f.Margin))/stepY)))
	r1 = min(g.Rows-1, int(math.Floor((float64(view.Max.Y)-float64(f.Margin))/stepY)))
	return
}

// leftovers dims the strips of the image to the right of and below the grid
// and its margin: pixels no frame covers.
func (t *Tool) leftovers(s *toolshell.Shell, screen ebiten.FinalScreen, used image.Rectangle, c color.RGBA) {
	m := t.st.File.Margin
	size := t.st.Size
	strip := func(x0, y0, x1, y1 int) {
		if x1 <= x0 || y1 <= y0 {
			return
		}
		w := s.WindowRect(gfx.Rect{Min: gfx.Vec2{X: float32(x0), Y: float32(y0)}, Max: gfx.Vec2{X: float32(x1), Y: float32(y1)}})
		s.FillRect(screen, w.Min.X, w.Min.Y, w.Max.X-w.Min.X, w.Max.Y-w.Min.Y, c)
	}
	strip(used.Max.X+m, 0, size.X, size.Y)
	strip(0, used.Max.Y+m, used.Max.X+m, size.Y)
}
