package viewer

import (
	"encoding/json"
	"fmt"
	"image/color"
	"math"
	"path"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/weakpixel/asset"
	"github.com/weakpixel/engine/editor/toolshell"
	"github.com/weakpixel/engine/editor/viewkit"
	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/render"
)

// textureView shows one texture on the grid. The atlas previewer builds on
// it.
type textureView struct {
	key   string
	scope *asset.Scope // holds what the previewer loads
	tex   viewkit.Texture
	size  gfx.Vec2
	shell *toolshell.Shell
	// ready is set once the texture is loaded; a viewer whose file failed to
	// load does nothing, and its panel shows the error.
	ready bool
}

// outline is the colour of the border drawn around the texture.
var outline = color.RGBA{R: 90, G: 150, B: 255, A: 255}

// openImage loads the texture at asset key, puts it at the origin with its
// top-left corner there, and frames it.
func (v *textureView) openImage(s *toolshell.Shell, key string) error {
	v.shell, v.key = s, key
	h, err := render.LoadSpriteSync(v.scope, key)
	if err != nil {
		return fmt.Errorf("view: %w", err)
	}
	v.tex.Spawn(s, h)
	v.size = v.tex.Size(s)
	v.ready = true
	s.Frame(gfx.Rect{Max: v.size})
	return nil
}

// close removes the sprite entity.
func (v *textureView) close(s *toolshell.Shell) {
	v.tex.Close(s)
	v.ready = false
}

// Open implements [toolshell.Tool].
func (v *textureView) Open(s *toolshell.Shell, key string) error { return v.openImage(s, key) }

// Sync implements [toolshell.Syncer]: it keeps the texture on screen (see
// [viewkit.Texture.Sync]).
func (v *textureView) Sync(s *toolshell.Shell) {
	if v.ready {
		v.tex.Sync(s)
	}
}

// Update implements [toolshell.Tool]: a reloaded texture of another size
// updates what Fit frames.
func (v *textureView) Update(s *toolshell.Shell) {
	if !v.ready {
		return
	}
	if size := v.tex.Size(s); size != v.size {
		v.size = size
		s.SetContent(gfx.Rect{Max: size})
	}
}

// Draw implements [toolshell.Tool]: a border around the texture.
func (v *textureView) Draw(s *toolshell.Shell, screen ebiten.FinalScreen) {
	if !v.ready {
		return
	}
	viewkit.DrawBorder(s, screen, gfx.Rect{Max: v.size}, outline)
}

// imageSection is the section with the texture's size and the pixel and
// grid cell under the mouse.
func (v *textureView) imageSection() toolshell.Section {
	return toolshell.Section{ID: "image", Label: "Image", Fields: []toolshell.Field{
		toolshell.Label("image.size", "Size", fmt.Sprintf("%d × %d px", int(v.size.X), int(v.size.Y))),
		toolshell.Label("image.pixel", "Pixel", v.pixelText()),
		toolshell.Label("image.cell", "Cell", v.cellText()),
	}}
}

// Panel implements [toolshell.Tool].
func (v *textureView) Panel() toolshell.Panel {
	return toolshell.Panel{Title: path.Base(v.key), Sections: []toolshell.Section{v.imageSection(), v.shell.ViewSection()}}
}

// pixelText is the texture pixel under the mouse, "x, y", or "outside".
func (v *textureView) pixelText() string {
	p := v.shell.MouseWorld()
	x, y := int(math.Floor(float64(p.X))), int(math.Floor(float64(p.Y)))
	if x < 0 || y < 0 || float32(x) >= v.size.X || float32(y) >= v.size.Y {
		return "outside"
	}
	return fmt.Sprintf("%d, %d", x, y)
}

// cellText is the grid cell under the mouse, "col, row".
func (v *textureView) cellText() string {
	g := v.shell.Grid()
	if g.Cell <= 0 {
		return "no grid"
	}
	p := v.shell.MouseWorld()
	col := int(math.Floor(float64((p.X - g.OffsetX) / g.Cell)))
	row := int(math.Floor(float64((p.Y - g.OffsetY) / g.Cell)))
	return fmt.Sprintf("%d, %d", col, row)
}

// Set implements [toolshell.Tool]: the viewer has no widgets of its own.
func (v *textureView) Set(id string, _ json.RawMessage) error {
	return fmt.Errorf("view: unknown widget %q", id)
}
