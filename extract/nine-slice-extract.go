package extract

import (
	"math"

	"github.com/weakpixel/ecs"
	"github.com/weakpixel/ecs/system"
	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/hierarchy"
	"github.com/weakpixel/engine/render"
)

// NineSliceExtract queues a world-space item for every visible entity with a
// Transform and a NineSlice. Hidden panels are skipped. A panel is culled
// when it is outside the camera bounds (plus Margin) by more than its own
// size: the test is conservative, so a panel whose pivot is off-screen but
// whose body is visible is kept, whatever its Origin and rotation. A panel
// with a zero Size has no size to add, so grow Margin for those. Culled
// panels count in render.Queue.Culled. A panel with a [hierarchy.Parent]
// sorts by its root's Y, as for [SpriteExtract].
type NineSliceExtract struct {
	// Margin grows the culling rectangle on every side, in world units.
	Margin float32

	view     ecs.View2[gfx.Transform, gfx.NineSlice] // without Parent
	children ecs.View3[gfx.Transform, gfx.NineSlice, hierarchy.Parent]
	res      resources
}

// NewNineSliceExtract returns the system for w.
func NewNineSliceExtract(w *ecs.World) *NineSliceExtract {
	return &NineSliceExtract{
		view:     ecs.NewView2[gfx.Transform, gfx.NineSlice](w, ecs.Without[hierarchy.Parent]()),
		children: ecs.NewView3[gfx.Transform, gfx.NineSlice, hierarchy.Parent](w),
	}
}

// Init implements system.Initializer.
func (s *NineSliceExtract) Init(ctx *system.Context) { s.res.init(ctx.World) }

// Run implements system.Runner.
func (s *NineSliceExtract) Run(ctx *system.Context) {
	bounds := s.res.bounds(s.Margin)
	culled := 0
	for _, c := range s.view.Each() {
		t, ns := c.Unpack()
		if ns.Hidden {
			continue
		}
		if !bounds.Grow(reach(t, ns)).Contains(t.Pos) {
			culled++
			continue
		}
		pushNineSlice(s.res.queue, render.WorldSpace, t, ns, t.Pos.Y)
	}
	for _, c := range s.children.Each() {
		t, ns, p := c.Unpack()
		if ns.Hidden {
			continue
		}
		if !bounds.Grow(reach(t, ns)).Contains(t.Pos) {
			culled++
			continue
		}
		pushNineSlice(s.res.queue, render.WorldSpace, t, ns, childSortY(p))
	}
	s.res.queue.AddCulled(culled)
}

// ScreenNineSliceExtract queues a screen-space item for every entity with a
// Transform and a ScreenNineSlice. There is no culling: HUD elements are few.
type ScreenNineSliceExtract struct {
	view ecs.View2[gfx.Transform, gfx.ScreenNineSlice]
	res  resources
}

// NewScreenNineSliceExtract returns the system for w.
func NewScreenNineSliceExtract(w *ecs.World) *ScreenNineSliceExtract {
	return &ScreenNineSliceExtract{view: ecs.NewView2[gfx.Transform, gfx.ScreenNineSlice](w)}
}

// Init implements system.Initializer.
func (s *ScreenNineSliceExtract) Init(ctx *system.Context) { s.res.init(ctx.World) }

// Run implements system.Runner.
func (s *ScreenNineSliceExtract) Run(ctx *system.Context) {
	for _, c := range s.view.Each() {
		t, sp := c.Unpack()
		if sp.NineSlice.Hidden {
			continue
		}
		pushNineSlice(s.res.queue, render.ScreenSpace, t, &sp.NineSlice, t.Pos.Y)
	}
}

// reach is the largest distance from the panel's pivot to any of its
// corners, in world units: the panel's scaled diagonal. Any origin and
// rotation keep the panel within this distance of Transform.Pos.
func reach(t *gfx.Transform, ns *gfx.NineSlice) float32 {
	s := scaleOrOne(t.Scale)
	return float32(math.Hypot(float64(ns.Size.X*s.X), float64(ns.Size.Y*s.Y)))
}

func pushNineSlice(q *render.Queue, space render.Space, t *gfx.Transform, ns *gfx.NineSlice, sortY float32) {
	q.Push(render.Item{
		Kind: render.KindNineSlice, Space: space, Layer: ns.Layer, SortY: sortY,
		Sprite: ns.ID, Pos: t.Pos, PrevPos: t.PrevPos,
		Rot: t.Rot, Scale: scaleOrOne(t.Scale), Origin: ns.Origin,
		Size: ns.Size, Insets: ns.Insets, Tile: ns.Tile,
		Color: ns.Tint, Blend: render.Blend(ns.Blend),
	})
}
