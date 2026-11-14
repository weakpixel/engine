package viewer

import (
	"encoding/json"
	"fmt"
	"image/color"
	"path"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/weakpixel/asset"
	"github.com/weakpixel/engine/editor/toolshell"
	"github.com/weakpixel/engine/render"
)

// Colors of the frame overlay.
var (
	colFrame    = color.RGBA{R: 60, G: 100, B: 170, A: 170}
	colHover    = color.RGBA{R: 255, G: 255, B: 255, A: 255}
	colSelected = color.RGBA{R: 255, G: 230, B: 60, A: 255}
)

// minLabelPx is the least width, in device-independent pixels, of a frame
// that gets its label without being hovered or selected; smaller frames would
// only be covered by their labels.
const minLabelPx = 24

// atlasView shows an atlas: the texture it is cut from, with every frame
// outlined and labelled, and the frame under the mouse described in the
// panel. It reads the atlas with the engine's loader, so what it shows is
// what the game gets.
type atlasView struct {
	textureView
	atlas   render.AtlasHandle
	version uint32
	frames  []Frame
	// hover and sel index frames, -1 for none.
	hover, sel int
	labels     labelMode
	prevDown   bool
	cache      labelCache
}

// Open implements [toolshell.Tool].
func (v *atlasView) Open(s *toolshell.Shell, key string) error {
	v.shell, v.key = s, key
	v.hover, v.sel = -1, -1
	srv := s.Engine().Assets()
	h, err := render.LoadAtlasSync(v.scope, key)
	if err != nil {
		return fmt.Errorf("view: %w", err)
	}
	v.atlas = h
	atlas := s.Engine().Stores().Atlases().Get(h)
	image := asset.StoreOf[render.Texture](srv).Key(atlas.Texture).Path
	if err := v.openImage(s, image); err != nil {
		return err
	}
	v.key = key // openImage names the panel after the texture
	v.reload()
	return nil
}

// reload reads the frames again and notes the atlas's version.
func (v *atlasView) reload() {
	e := v.shell.Engine()
	if a := e.Stores().Atlases().Get(v.atlas); a != nil {
		v.frames = framesOf(a, e.Stores().Sprites())
	}
	v.version = e.Stores().Atlases().Generic().Version(asset.HandleOf[render.Atlas](v.atlas.Ref))
	v.hover = min(v.hover, len(v.frames)-1)
	if v.sel >= len(v.frames) {
		v.sel = -1
	}
}

// Update implements [toolshell.Tool]: a reloaded atlas, the frame under the
// mouse and a click that selects it.
func (v *atlasView) Update(s *toolshell.Shell) {
	if !v.ready {
		return
	}
	v.textureView.Update(s)
	if ver := s.Engine().Stores().Atlases().Generic().Version(asset.HandleOf[render.Atlas](v.atlas.Ref)); ver != v.version {
		v.reload()
	}
	v.hover = frameAt(v.frames, s.MouseWorld())
	down := s.Mouse().Buttons&1 != 0
	panning := s.PanHeld()
	if down && !v.prevDown && !panning {
		if v.hover == v.sel {
			v.sel = -1
		} else {
			v.sel = v.hover
		}
	}
	v.prevDown = down
}

// Draw implements [toolshell.Tool]: the texture's border, then the frames.
func (v *atlasView) Draw(s *toolshell.Shell, screen ebiten.FinalScreen) {
	if !v.ready {
		return
	}
	v.textureView.Draw(s, screen)
	b := screen.Bounds()
	w, h := float32(b.Dx()), float32(b.Dy())
	outline := func(f Frame, c color.RGBA) {
		r := s.WindowRect(f.Rect)
		fw, fh := r.Max.X-r.Min.X, r.Max.Y-r.Min.Y
		s.FillRect(screen, r.Min.X, r.Min.Y, fw, 1, c)
		s.FillRect(screen, r.Min.X, r.Max.Y-1, fw, 1, c)
		s.FillRect(screen, r.Min.X, r.Min.Y, 1, fh, c)
		s.FillRect(screen, r.Max.X-1, r.Min.Y, 1, fh, c)
	}
	label := func(f Frame, text string) {
		if text == "" {
			return
		}
		r := s.WindowRect(f.Rect)
		v.cache.draw(screen, text, r.Min.X+1, r.Min.Y+1)
	}
	for i, f := range v.frames {
		r := s.WindowRect(f.Rect)
		if r.Max.X < 0 || r.Max.Y < 0 || r.Min.X > w || r.Min.Y > h {
			continue // off screen
		}
		if r.Max.X-r.Min.X < 3 && i != v.hover && i != v.sel {
			continue // a frame this small is only a smudge of outlines
		}
		outline(f, colFrame)
		if text := v.labels.text(f); v.cache.fits(text, r.Max.X-r.Min.X) {
			label(f, text)
		}
	}
	for _, i := range []int{v.hover, v.sel} {
		if i < 0 || i >= len(v.frames) {
			continue
		}
		c := colHover
		if i == v.sel {
			c = colSelected
		}
		outline(v.frames[i], c)
		outline(Frame{Rect: v.frames[i].Rect.Grow(1)}, c)
		label(v.frames[i], labelBoth.text(v.frames[i]))
	}
}

// shown is the frame the panel describes: the selected one, else the one
// under the mouse.
func (v *atlasView) shown() (Frame, bool) {
	for _, i := range []int{v.sel, v.hover} {
		if i >= 0 && i < len(v.frames) {
			return v.frames[i], true
		}
	}
	return Frame{}, false
}

// Panel implements [toolshell.Tool].
func (v *atlasView) Panel() toolshell.Panel {
	which := "none"
	fields := []toolshell.Field{
		toolshell.Label("atlas.count", "Frames", fmt.Sprint(len(v.frames))),
		toolshell.Dropdown("atlas.labels", "Labels", int(v.labels), labelModeNames...),
	}
	f, ok := v.shown()
	detail := func(id, label, text string) toolshell.Field { return toolshell.Label("frame."+id, label, text) }
	if ok {
		which = "hover"
		if v.sel >= 0 {
			which = "selected"
		}
		name := f.Name
		if name == "" {
			name = "-"
		}
		fields = append(fields,
			detail("which", "Frame", fmt.Sprintf("%d (%s)", f.Index, which)),
			detail("name", "Name", name),
			detail("rect", "Rect", rectText(f.Rect)),
			detail("size", "Untrimmed", pointText(f.Size)),
			detail("offset", "Trim offset", pointText(f.Offset)),
			detail("pivot", "Pivot", pointText(f.Pivot)),
		)
	} else {
		fields = append(fields, detail("which", "Frame", "click a frame to select it"))
	}
	return toolshell.Panel{Title: path.Base(v.key), Sections: []toolshell.Section{
		{ID: "atlas", Label: "Atlas", Fields: fields},
		v.imageSection(),
		v.shell.ViewSection(),
	}}
}

// Set implements [toolshell.Tool].
func (v *atlasView) Set(id string, val json.RawMessage) error {
	if id != "atlas.labels" {
		return fmt.Errorf("view: unknown widget %q", id)
	}
	var i int
	if err := json.Unmarshal(val, &i); err != nil || i < 0 || i >= len(labelModeNames) {
		return fmt.Errorf("view: bad label mode %s", val)
	}
	v.labels = labelMode(i)
	return nil
}
