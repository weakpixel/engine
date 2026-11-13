package viewer

import (
	"encoding/json"
	"fmt"
	"image/color"
	"math"
	"path"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/weakpixel/asset"
	"github.com/weakpixel/ecs"
	"github.com/weakpixel/engine/editor/toolshell"
	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/render"
)

// textureView shows one texture on the grid. The atlas previewer builds on
// it.
type textureView struct {
	key    string
	scope  *asset.Scope // holds what the previewer loads
	sprite gfx.SpriteHandle
	size   gfx.Vec2
	shell  *toolshell.Shell
	entity ecs.Entity
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
	e := s.Engine()
	h, err := render.LoadSpriteSync(v.scope, key)
	if err != nil {
		return fmt.Errorf("view: %w", err)
	}
	v.sprite = h
	// The zero Origin means the sprite's default pivot (the center); a tiny
	// value puts the top-left corner at the entity's position.
	v.entity = ecs.NewEntity2(e.World(), gfx.Transform{}, gfx.Sprite{ID: h, Origin: gfx.Vec2{X: 0.0001, Y: 0.0001}})
	v.size = v.textureSize()
	v.ready = true
	s.Frame(gfx.Rect{Max: v.size})
	return nil
}

// close removes the sprite entity.
func (v *textureView) close(s *toolshell.Shell) {
	if w := s.Engine().World(); v.ready && w.IsAlive(v.entity) {
		w.DeleteEntity(v.entity)
	}
	v.ready = false
}

// Open implements [toolshell.Tool].
func (v *textureView) Open(s *toolshell.Shell, key string) error { return v.openImage(s, key) }

// textureSize reads the size from the asset store, which a hot reload can
// change.
func (v *textureView) textureSize() gfx.Vec2 {
	return v.shell.Engine().Stores().Sprites().Get(v.sprite).Size
}

// Sync implements [toolshell.Syncer]. The engine culls a sprite by its
// position alone, so a texture whose top-left corner left the screen would
// vanish while most of it is still visible. The sprite is instead pinned at
// the center of the part of the texture that is on screen, with its pivot
// set so the texture stays where it is.
func (v *textureView) Sync(s *toolshell.Shell) {
	e := s.Engine()
	if !v.ready {
		return
	}
	t := ecs.GetCompOrNil[gfx.Transform](e.World(), v.entity)
	sp := ecs.GetCompOrNil[gfx.Sprite](e.World(), v.entity)
	if t == nil || sp == nil || v.size.X <= 0 || v.size.Y <= 0 {
		return
	}
	view := e.Camera().DrawBounds(s.ScreenSize())
	lo := gfx.Vec2{X: max(view.Min.X, 0), Y: max(view.Min.Y, 0)}
	hi := gfx.Vec2{X: min(view.Max.X, v.size.X), Y: min(view.Max.Y, v.size.Y)}
	if lo.X > hi.X || lo.Y > hi.Y {
		return // nothing of the texture is on screen
	}
	p := lo.Add(hi).Mul(0.5)
	t.Pos, t.PrevPos = p, p
	// A zero Origin means the default pivot, so keep it just above zero.
	sp.Origin = gfx.Vec2{X: max(p.X/v.size.X, 0.0001), Y: max(p.Y/v.size.Y, 0.0001)}
}

// Update implements [toolshell.Tool]: a reloaded texture of another size
// updates what Fit frames.
func (v *textureView) Update(s *toolshell.Shell) {
	if !v.ready {
		return
	}
	if size := v.textureSize(); size != v.size {
		v.size = size
		s.SetContent(gfx.Rect{Max: size})
	}
}

// Draw implements [toolshell.Tool]: a border around the texture.
func (v *textureView) Draw(s *toolshell.Shell, screen ebiten.FinalScreen) {
	if !v.ready {
		return
	}
	r := s.WindowRect(gfx.Rect{Max: v.size}).Grow(1)
	w, h := r.Max.X-r.Min.X, r.Max.Y-r.Min.Y
	s.FillRect(screen, r.Min.X, r.Min.Y, w, 1, outline)
	s.FillRect(screen, r.Min.X, r.Max.Y-1, w, 1, outline)
	s.FillRect(screen, r.Min.X, r.Min.Y, 1, h, outline)
	s.FillRect(screen, r.Max.X-1, r.Min.Y, 1, h, outline)
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
