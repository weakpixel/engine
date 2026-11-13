package viewer

import (
	"encoding/json"
	"fmt"
	"image/color"
	"math"
	"path"

	"github.com/hajimehoshi/ebiten/v2"
	"github.com/hajimehoshi/ebiten/v2/text/v2"

	"github.com/weakpixel/asset"
	"github.com/weakpixel/engine/editor/toolshell"
	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/render"
)

const (
	defaultSample = "The quick brown fox jumps over the lazy dog 0123456789"
	// captionH is the height reserved for the "16 px" line above a sample,
	// rowGap the space between two rows, both in world units.
	captionH = 16
	rowGap   = 14
)

// fontRow is one size of the font.
type fontRow struct {
	size   float64
	handle gfx.FontHandle
	// ver is the font's version when the row was measured, to notice a
	// reload.
	ver uint32
	// w and lh are the width of the sample and the font's line height.
	w, lh float64
	img   *ebiten.Image
	// imgText and imgColor are what img shows.
	imgText  string
	imgColor color.RGBA
}

// fontView shows sample text in a font at the sizes the project uses, drawn
// with the engine's own face of the font (the one gfx.Label uses), so it
// looks as it does in the game. The samples are drawn in Draw, not as Label
// entities: a label lives in screen space and could not be panned or zoomed.
type fontView struct {
	shell  *toolshell.Shell
	scope  *asset.Scope // holds the font at every size
	key    string       // the font file, "fonts/ui.ttf"
	used   []float64
	extra  float64
	sample string
	rows   []fontRow
	tops   []float32
	cache  labelCache
}

// Open implements [toolshell.Tool]: it finds the sizes the project uses and
// loads the font at each.
func (v *fontView) Open(s *toolshell.Shell, key string) error {
	v.shell, v.key, v.sample = s, key, defaultSample
	files := projectFiles(s.FS(), s.Files())
	v.used = fontSizes(s.FS(), files, key)
	if err := v.rebuild(); err != nil {
		return err
	}
	s.Frame(v.content())
	return nil
}

// sizes are the sizes shown: the ones the project uses (the defaults when
// there are none) and the extra one.
func (v *fontView) sizes() []float64 {
	sizes := append([]float64(nil), v.used...)
	if len(sizes) == 0 {
		sizes = append(sizes, defaultSizes...)
	}
	if v.extra > 0 {
		sizes = append(sizes, v.extra)
	}
	return sortedSizes(sizes)
}

// rebuild loads the font at every size and measures the sample. The first
// size that fails is returned; the rows that loaded stay.
func (v *fontView) rebuild() error {
	e := v.shell.Engine()
	var rows []fontRow
	var first error
	for _, size := range v.sizes() {
		h, err := render.LoadFontSync(v.scope, fmt.Sprintf("%s#%g", v.key, size))
		if err != nil {
			if first == nil {
				first = fmt.Errorf("view: %w", err)
			}
			continue
		}
		f := e.Stores().Fonts().Get(h)
		w, _ := text.Measure(v.sample, f.Face, f.LineHeight)
		rows = append(rows, fontRow{
			size: size, handle: h, w: w, lh: f.LineHeight,
			ver: e.Stores().Fonts().Generic().Version(assetFont(h)),
		})
	}
	v.rows = rows
	heights := make([]float32, len(rows))
	for i, r := range rows {
		heights[i] = captionH + float32(math.Ceil(r.lh))
	}
	v.tops, _ = layoutRows(heights, rowGap)
	return first
}

// content is the rectangle Fit frames.
func (v *fontView) content() gfx.Rect {
	var w, h float32
	for i, r := range v.rows {
		w = max(w, float32(math.Ceil(r.w)))
		h = v.tops[i] + captionH + float32(math.Ceil(r.lh))
	}
	return gfx.Rect{Max: gfx.Vec2{X: max(w, 64), Y: max(h, 16)}}
}

// Update implements [toolshell.Tool]: a reloaded font is measured again.
func (v *fontView) Update(s *toolshell.Shell) {
	st := s.Engine().Stores().Fonts()
	for _, r := range v.rows {
		if st.Generic().Version(assetFont(r.handle)) != r.ver {
			if err := v.rebuild(); err != nil {
				s.ReportError(err)
			}
			s.SetContent(v.content())
			return
		}
	}
}

// textColor is readable on the shell's background.
func (v *fontView) textColor() color.RGBA {
	bg := v.shell.Background()
	if lum := 0.299*float64(bg.R) + 0.587*float64(bg.G) + 0.114*float64(bg.B); lum > 128 {
		return color.RGBA{A: 255}
	}
	return color.RGBA{R: 240, G: 240, B: 240, A: 255}
}

// image renders row r's sample in c, once per text and color.
func (v *fontView) image(r *fontRow, c color.RGBA) *ebiten.Image {
	if r.img != nil && r.imgText == v.sample && r.imgColor == c {
		return r.img
	}
	f := v.shell.Engine().Stores().Fonts().Get(r.handle)
	if r.img != nil {
		r.img.Deallocate()
	}
	r.img = ebiten.NewImage(int(math.Ceil(r.w))+2, int(math.Ceil(r.lh))+2)
	op := &text.DrawOptions{}
	op.LineSpacing = f.LineHeight
	op.GeoM.Translate(1, 1)
	op.ColorScale.ScaleWithColor(c)
	text.Draw(r.img, v.sample, f.Face, op)
	r.imgText, r.imgColor = v.sample, c
	return r.img
}

// Draw implements [toolshell.Tool]: a caption and the sample of each size.
func (v *fontView) Draw(s *toolshell.Shell, screen ebiten.FinalScreen) {
	c := v.textColor()
	for i := range v.rows {
		r := &v.rows[i]
		top := s.WindowRect(gfx.Rect{Min: gfx.Vec2{Y: v.tops[i]}})
		v.cache.draw(screen, fmt.Sprintf("%g px", r.size), top.Min.X, top.Min.Y)
		at := s.WindowRect(gfx.Rect{Min: gfx.Vec2{Y: v.tops[i] + captionH}, Max: gfx.Vec2{X: 1, Y: v.tops[i] + captionH + 1}})
		var op ebiten.DrawImageOptions
		op.Filter = ebiten.FilterNearest
		op.GeoM.Scale(float64(at.Max.X-at.Min.X), float64(at.Max.Y-at.Min.Y)) // one world unit in window pixels
		op.GeoM.Translate(float64(at.Min.X), float64(at.Min.Y))
		screen.DrawImage(v.image(r, c), &op)
	}
}

// Panel implements [toolshell.Tool].
func (v *fontView) Panel() toolshell.Panel {
	sizes := []toolshell.Field{}
	for _, r := range v.rows {
		sizes = append(sizes, toolshell.Label(fmt.Sprintf("font.size.%g", r.size), fmt.Sprintf("%g px", r.size),
			fmt.Sprintf("line height %.1f, sample %d px wide", r.lh, int(math.Ceil(r.w)))))
	}
	source := "sizes used in levels and prefabs"
	if len(v.used) == 0 {
		source = "default sizes, the font is used nowhere in the project"
	}
	return toolshell.Panel{Title: path.Base(v.key), Sections: []toolshell.Section{
		{ID: "font", Label: "Font", Fields: []toolshell.Field{
			toolshell.Text("font.sample", "Sample", v.sample),
			toolshell.Number("font.extra", "Extra size (px)", v.extra).Range(0, 256).WithDigits(0).WithHint("Add a size to compare; 0 adds none"),
			toolshell.Label("font.source", "Shown", source),
		}},
		{ID: "sizes", Label: "Sizes", Fields: sizes},
		v.shell.ViewSection(),
	}}
}

// Set implements [toolshell.Tool].
func (v *fontView) Set(id string, val json.RawMessage) error {
	switch id {
	case "font.sample":
		var s string
		if err := json.Unmarshal(val, &s); err != nil {
			return fmt.Errorf("view: sample must be text: %w", err)
		}
		if s == "" {
			s = defaultSample
		}
		v.sample = s
	case "font.extra":
		var f float64
		if err := json.Unmarshal(val, &f); err != nil || f < 0 || math.IsNaN(f) || math.IsInf(f, 0) {
			return fmt.Errorf("view: extra size must be a number of pixels, got %s", val)
		}
		v.extra = math.Round(f)
	default:
		return fmt.Errorf("view: unknown widget %q", id)
	}
	err := v.rebuild()
	v.shell.SetContent(v.content())
	return err
}

// assetFont is the generic handle of a font handle, for the store's version.
func assetFont(h gfx.FontHandle) asset.Handle[render.Font] { return asset.HandleOf[render.Font](h.Ref) }
