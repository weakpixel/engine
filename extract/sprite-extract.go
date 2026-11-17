package extract

import (
	"github.com/weakpixel/ecs"
	"github.com/weakpixel/ecs/system"
	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/hierarchy"
	"github.com/weakpixel/engine/render"
)

// SpriteExtract queues a world-space item for every visible entity with a
// Transform and a Sprite. Hidden sprites and sprites whose position is
// outside the camera bounds (plus Margin) are skipped; the latter count in
// render.Queue.Culled.
//
// A sprite with a [hierarchy.Parent] sorts by its root's Y, just in front of
// or behind it (see childSortY), so it stays glued to its root in a
// Y-sorted layer.
type SpriteExtract struct {
	// Margin grows the culling rectangle on every side, in world units.
	// Set it to about half of the largest sprite so that sprites at the
	// edge do not pop. The default zero culls exactly at the view edge.
	Margin float32

	view     ecs.View2[gfx.Transform, gfx.Sprite] // without Parent
	children ecs.View3[gfx.Transform, gfx.Sprite, hierarchy.Parent]
	res      resources
}

// NewSpriteExtract returns the system for w.
func NewSpriteExtract(w *ecs.World) *SpriteExtract {
	return &SpriteExtract{
		view:     ecs.NewView2[gfx.Transform, gfx.Sprite](w, ecs.Without[hierarchy.Parent]()),
		children: ecs.NewView3[gfx.Transform, gfx.Sprite, hierarchy.Parent](w),
	}
}

// Init implements system.Initializer.
func (s *SpriteExtract) Init(ctx *system.Context) { s.res.init(ctx.World) }

// Run implements system.Runner.
func (s *SpriteExtract) Run(ctx *system.Context) {
	bounds := s.res.bounds(s.Margin)
	culled := 0
	for _, c := range s.view.Each() {
		t, sp := c.Unpack()
		if sp.Hidden {
			continue
		}
		if !bounds.Contains(t.Pos) {
			culled++
			continue
		}
		pushSprite(s.res.queue, render.WorldSpace, t, sp, t.Pos.Y)
	}
	for _, c := range s.children.Each() {
		t, sp, p := c.Unpack()
		if sp.Hidden {
			continue
		}
		if !bounds.Contains(t.Pos) {
			culled++
			continue
		}
		pushSprite(s.res.queue, render.WorldSpace, t, sp, childSortY(p))
	}
	s.res.queue.AddCulled(culled)
}

// ScreenSpriteExtract queues a screen-space item for every entity with a
// Transform and a ScreenSprite. There is no culling: HUD elements are few.
type ScreenSpriteExtract struct {
	view ecs.View2[gfx.Transform, gfx.ScreenSprite]
	res  resources
}

// NewScreenSpriteExtract returns the system for w.
func NewScreenSpriteExtract(w *ecs.World) *ScreenSpriteExtract {
	return &ScreenSpriteExtract{view: ecs.NewView2[gfx.Transform, gfx.ScreenSprite](w)}
}

// Init implements system.Initializer.
func (s *ScreenSpriteExtract) Init(ctx *system.Context) { s.res.init(ctx.World) }

// Run implements system.Runner.
func (s *ScreenSpriteExtract) Run(ctx *system.Context) {
	for _, c := range s.view.Each() {
		t, sp := c.Unpack()
		if sp.Sprite.Hidden {
			continue
		}
		pushSprite(s.res.queue, render.ScreenSpace, t, &sp.Sprite, t.Pos.Y)
	}
}

func pushSprite(q *render.Queue, space render.Space, t *gfx.Transform, sp *gfx.Sprite, sortY float32) {
	q.Push(render.Item{
		Kind: render.KindSprite, Space: space, Layer: sp.Layer, SortY: sortY,
		Sprite: sp.ID, Pos: t.Pos, PrevPos: t.PrevPos,
		Rot: t.Rot, Scale: scaleOrOne(t.Scale), Origin: sp.Origin,
		FlipX: sp.FlipX, FlipY: sp.FlipY, Color: sp.Tint, Blend: render.Blend(sp.Blend),
		Shader: sp.Shader, Params: sp.Params, Aux: sp.Aux,
	})
}
