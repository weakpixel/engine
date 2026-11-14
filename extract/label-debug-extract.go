package extract

import (
	"iter"

	"github.com/weakpixel/ecs"
	"github.com/weakpixel/ecs/system"
	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/render"
)

// LabelExtract queues a screen-space text item for every entity with a
// Transform and a Label. Transform.Pos is in logical screen pixels.
type LabelExtract struct {
	view ecs.View2[gfx.Transform, gfx.Label]
	res  resources
}

// NewLabelExtract returns the system for w.
func NewLabelExtract(w *ecs.World) *LabelExtract {
	return &LabelExtract{view: ecs.NewView2[gfx.Transform, gfx.Label](w)}
}

// Init implements system.Initializer.
func (s *LabelExtract) Init(ctx *system.Context) { s.res.init(ctx.World) }

// Run implements system.Runner.
func (s *LabelExtract) Run(ctx *system.Context) {
	for _, c := range s.view.Each() {
		t, l := c.Unpack()
		s.res.queue.Push(render.Item{
			Kind: render.KindText, Space: render.ScreenSpace, Layer: l.Layer,
			Pos: t.Pos, PrevPos: t.PrevPos, Rot: t.Rot, Scale: scaleOrOne(t.Scale),
			Color: l.Color, Text: l.Text, Font: l.Font,
		})
	}
}

// DebugExtract queues a world-space shape item for every entity with a
// Transform and a DebugShape, culled like sprites (and counted in
// render.Queue.Culled). Shapes draw on layer
// DebugLayer, above ordinary content.
//
// It also draws the outlines a shape package publishes as the World's
// gfx.DebugOutlines resource: collision publishes its colliders as the last
// run placed them (nothing without a collision system), colored by the
// collider's lowest layer (collision.ColliderColor). Custom shapes draw as
// their bounds.
type DebugExtract struct {
	// Margin grows the culling rectangle, in world units.
	Margin float32

	view ecs.View2[gfx.Transform, gfx.DebugShape]
	res  resources
}

// DebugLayer is the layer debug shapes are drawn on.
const DebugLayer int16 = 32767

// NewDebugExtract returns the system for w.
func NewDebugExtract(w *ecs.World) *DebugExtract {
	return &DebugExtract{view: ecs.NewView2[gfx.Transform, gfx.DebugShape](w)}
}

// Init implements system.Initializer.
func (s *DebugExtract) Init(ctx *system.Context) { s.res.init(ctx.World) }

// Run implements system.Runner.
func (s *DebugExtract) Run(ctx *system.Context) {
	bounds := s.res.bounds(s.Margin)
	culled := 0
	for _, c := range s.view.Each() {
		t, d := c.Unpack()
		if !bounds.Contains(t.Pos) {
			culled++
			continue
		}
		s.res.queue.Push(render.Item{
			Kind: shapeKind(d.Kind), Layer: DebugLayer,
			Pos: t.Pos, PrevPos: t.PrevPos, Rot: t.Rot, Scale: scaleOrOne(t.Scale),
			Size: d.Size, Color: d.Color,
		})
	}
	if o := ecs.GetResourceOrNil[gfx.DebugOutlines](ctx.World); o != nil && o.Outlines != nil {
		culled += s.outlines(o.Outlines, bounds)
	}
	s.res.queue.AddCulled(culled)
}

// outlines queues the outlines a shape package published as
// gfx.DebugOutlines and returns how many were culled.
func (s *DebugExtract) outlines(seq iter.Seq[gfx.DebugOutline], bounds gfx.Rect) (culled int) {
	one := gfx.Vec2{X: 1, Y: 1}
	for o := range seq {
		b := o.Bounds
		if o.Circle {
			b = gfx.Rect{Min: gfx.Vec2{X: o.Center.X - o.Radius, Y: o.Center.Y - o.Radius}, Max: gfx.Vec2{X: o.Center.X + o.Radius, Y: o.Center.Y + o.Radius}}
		}
		if b.Max.X < bounds.Min.X || b.Min.X > bounds.Max.X || b.Max.Y < bounds.Min.Y || b.Min.Y > bounds.Max.Y {
			culled++
			continue
		}
		it := render.Item{Kind: render.KindRect, Layer: DebugLayer, Scale: one, Color: o.Color,
			Pos: b.Min, PrevPos: b.Min, Size: b.Max.Sub(b.Min)}
		if o.Circle {
			it.Kind, it.Pos, it.PrevPos, it.Size = render.KindCircle, o.Center, o.Center, gfx.Vec2{X: o.Radius}
		}
		s.res.queue.Push(it)
	}
	return culled
}

func shapeKind(k gfx.ShapeKind) render.Kind {
	switch k {
	case gfx.ShapeCircle:
		return render.KindCircle
	case gfx.ShapeLine:
		return render.KindLine
	default:
		return render.KindRect
	}
}
