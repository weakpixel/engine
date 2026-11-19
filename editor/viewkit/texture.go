package viewkit

import (
	"image/color"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/weakpixel/ecs"
	"github.com/weakpixel/engine/editor/toolshell"
	"github.com/weakpixel/engine/gfx"
)

// Texture is one texture shown as a sprite entity with its top-left corner at
// world (0, 0), so a world point is a texture pixel. The zero value shows
// nothing until [Texture.Spawn].
type Texture struct {
	// Sprite is the texture shown, the zero handle for none.
	Sprite gfx.SpriteHandle
	entity ecs.Entity
	alive  bool
}

// cornerOrigin is the Sprite.Origin that puts the top-left corner at the
// entity's position: the zero Origin means the sprite's default pivot (the
// center), so it is kept just above zero.
var cornerOrigin = gfx.Vec2{X: 0.0001, Y: 0.0001}

// Spawn creates the sprite entity for h. Call it once; [Texture.SetSprite]
// changes the texture later.
func (t *Texture) Spawn(s *toolshell.Shell, h gfx.SpriteHandle) {
	t.Sprite = h
	t.entity = ecs.NewEntity2(s.Engine().World(), gfx.Transform{}, gfx.Sprite{ID: h, Origin: cornerOrigin})
	t.alive = true
}

// SetSprite shows h instead of the current texture.
func (t *Texture) SetSprite(s *toolshell.Shell, h gfx.SpriteHandle) {
	t.Sprite = h
	if sp := t.sprite(s); sp != nil {
		sp.ID = h
	}
}

// Entity is the sprite entity, for tools that set more of its Sprite (a
// shader and its Params).
func (t *Texture) Entity() ecs.Entity { return t.entity }

// Close deletes the sprite entity.
func (t *Texture) Close(s *toolshell.Shell) {
	if w := s.Engine().World(); t.alive && w.IsAlive(t.entity) {
		w.DeleteEntity(t.entity)
	}
	t.alive = false
}

// Size reads the texture's size from the asset store, which a hot reload can
// change; zero without a texture.
func (t *Texture) Size(s *toolshell.Shell) gfx.Vec2 {
	if t.Sprite == (gfx.SpriteHandle{}) {
		return gfx.Vec2{}
	}
	if sp := s.Engine().Stores().Sprites().Get(t.Sprite); sp != nil {
		return sp.Size
	}
	return gfx.Vec2{}
}

func (t *Texture) sprite(s *toolshell.Shell) *gfx.Sprite {
	if !t.alive {
		return nil
	}
	return ecs.GetCompOrNil[gfx.Sprite](s.Engine().World(), t.entity)
}

// Sync keeps the texture visible; call it from the tool's [toolshell.Syncer].
// The engine culls a sprite by its position alone, so a texture whose
// top-left corner left the screen would vanish while most of it is still
// visible. The sprite is instead pinned at the center of the part of the
// texture that is on screen, with its pivot set so the texture stays where
// it is.
func (t *Texture) Sync(s *toolshell.Shell) {
	if !t.alive {
		return
	}
	e := s.Engine()
	tr := ecs.GetCompOrNil[gfx.Transform](e.World(), t.entity)
	sp := t.sprite(s)
	size := t.Size(s)
	if tr == nil || sp == nil || size.X <= 0 || size.Y <= 0 {
		return
	}
	view := e.Camera().DrawBounds(s.ScreenSize())
	lo := gfx.Vec2{X: max(view.Min.X, 0), Y: max(view.Min.Y, 0)}
	hi := gfx.Vec2{X: min(view.Max.X, size.X), Y: min(view.Max.Y, size.Y)}
	if lo.X > hi.X || lo.Y > hi.Y {
		return // nothing of the texture is on screen
	}
	p := lo.Add(hi).Mul(0.5)
	tr.Pos, tr.PrevPos = p, p
	// A zero Origin means the default pivot, so keep it just above zero.
	sp.Origin = gfx.Vec2{X: max(p.X/size.X, cornerOrigin.X), Y: max(p.Y/size.Y, cornerOrigin.Y)}
}

// DrawBorder draws a one pixel border in c just outside the world rectangle
// r, in window pixels.
func DrawBorder(s *toolshell.Shell, screen ebiten.FinalScreen, r gfx.Rect, c color.RGBA) {
	DrawOutline(s, screen, s.WindowRect(r).Grow(1), c)
}

// DrawOutline draws a one pixel outline in c along the inside of the window
// rectangle r.
func DrawOutline(s *toolshell.Shell, screen ebiten.FinalScreen, r gfx.Rect, c color.RGBA) {
	w, h := r.Max.X-r.Min.X, r.Max.Y-r.Min.Y
	s.FillRect(screen, r.Min.X, r.Min.Y, w, 1, c)
	s.FillRect(screen, r.Min.X, r.Max.Y-1, w, 1, c)
	s.FillRect(screen, r.Min.X, r.Min.Y, 1, h, c)
	s.FillRect(screen, r.Max.X-1, r.Min.Y, 1, h, c)
}
