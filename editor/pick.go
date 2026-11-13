package editor

import (
	"cmp"
	"math"
	"slices"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/weakpixel/ecs"
	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/hierarchy"
	"github.com/weakpixel/engine/render"
)

// handleSize is the side of the square that stands for an object with a
// Transform and nothing drawable the editor can measure.
const handleSize = 8

// hit is a pickable object with its world bounds and draw order.
type hit struct {
	id     string
	bounds gfx.Rect
	layer  int16
	y      float32
	index  int // document order
}

// bounds returns the world-space rectangle of the object with entity e, and
// false when it has no Transform (it cannot be placed in the viewport).
// Sprites use the visible art of their loaded frame (alpha > 0; the whole
// frame while loading or unreadable), with the renderer's pivot rules and
// Transform.Scale; rotation and flips are ignored. A NineSlice uses its
// whole rectangle. A DebugShape uses its outline; anything else a handle around
// Transform.Pos.
func bounds(w *ecs.World, sprites render.SpriteStore, alpha *alphaCache, e ecs.Entity) (gfx.Rect, int16, bool) {
	if ecs.GetCompOrNil[gfx.Transform](w, e) == nil {
		return gfx.Rect{}, 0, false
	}
	// An attached object's Transform is written by the hierarchy stage, which
	// has not run yet for objects spawned since the last tick (a level framed
	// right after its document applied): compose its placement instead.
	world, ok := hierarchy.WorldOf(w, e)
	if !ok {
		return gfx.Rect{}, 0, false
	}
	t := &world
	scale := t.Scale
	if scale == (gfx.Vec2{}) {
		scale = gfx.Vec2{X: 1, Y: 1}
	}
	if sp := ecs.GetCompOrNil[gfx.Sprite](w, e); sp != nil {
		frame := sprites.Get(sp.ID)
		origin := sp.Origin
		if origin == (gfx.Vec2{}) {
			origin = frame.Origin
		}
		// Like render.spriteShift: the trimmed image starts at Offset in the
		// untrimmed frame, and the pivot is a fraction of the untrimmed size.
		shift := gfx.Vec2{X: frame.Offset.X - origin.X*frame.Size.X, Y: frame.Offset.Y - origin.Y*frame.Size.Y}
		art := alpha.of(sprites, sp.ID, frame)
		shift = shift.Add(gfx.Vec2{X: float32(art.Min.X), Y: float32(art.Min.Y)})
		size := gfx.Vec2{X: float32(art.Dx()), Y: float32(art.Dy())}
		a := t.Pos.Add(gfx.Vec2{X: shift.X * scale.X, Y: shift.Y * scale.Y})
		b := a.Add(gfx.Vec2{X: size.X * scale.X, Y: size.Y * scale.Y})
		return normRect(a, b), sp.Layer, true
	}
	if ns := ecs.GetCompOrNil[gfx.NineSlice](w, e); ns != nil {
		// The panel is its whole rectangle: Size (the frame's size when
		// zero) from the Origin pivot, scaled by Transform.Scale.
		size := ns.Size
		if frame := sprites.Get(ns.ID); size.X == 0 || size.Y == 0 {
			if size.X == 0 {
				size.X = frame.Size.X
			}
			if size.Y == 0 {
				size.Y = frame.Size.Y
			}
		}
		a := t.Pos.Sub(gfx.Vec2{X: ns.Origin.X * size.X * scale.X, Y: ns.Origin.Y * size.Y * scale.Y})
		return normRect(a, a.Add(gfx.Vec2{X: size.X * scale.X, Y: size.Y * scale.Y})), ns.Layer, true
	}
	if ds := ecs.GetCompOrNil[gfx.DebugShape](w, e); ds != nil {
		switch ds.Kind {
		case gfx.ShapeRect, gfx.ShapeLine:
			return normRect(t.Pos, t.Pos.Add(ds.Size)), 0, true
		case gfx.ShapeCircle:
			r := gfx.Vec2{X: ds.Size.X, Y: ds.Size.X}
			return gfx.Rect{Min: t.Pos.Sub(r), Max: t.Pos.Add(r)}, 0, true
		}
	}
	h := gfx.Vec2{X: handleSize / 2, Y: handleSize / 2}
	return gfx.Rect{Min: t.Pos.Sub(h), Max: t.Pos.Add(h)}, 0, true
}

// normRect is the rectangle spanned by a and b in any order (negative
// scales and lines flip them).
func normRect(a, b gfx.Vec2) gfx.Rect {
	return gfx.Rect{
		Min: gfx.Vec2{X: min(a.X, b.X), Y: min(a.Y, b.Y)},
		Max: gfx.Vec2{X: max(a.X, b.X), Y: max(a.Y, b.Y)},
	}
}

// hits returns the level's placeable objects, front-most first: higher
// render layer, then lower on screen (larger Y, as Y-sorted layers draw
// it), then later in the document.
func (s *Session) hits() []hit {
	w := s.e.World()
	sprites := s.e.Stores().Sprites()
	hs := make([]hit, 0, len(s.order))
	for i, id := range s.order {
		o := s.objs[id]
		if !w.IsAlive(o.entity) {
			continue
		}
		r, layer, ok := bounds(w, sprites, s.alpha, o.entity)
		if !ok {
			continue
		}
		y := ecs.GetComp[gfx.Transform](w, o.entity).Pos.Y
		hs = append(hs, hit{id: id, bounds: r, layer: layer, y: y, index: i})
	}
	slices.SortStableFunc(hs, func(a, b hit) int {
		return cmp.Or(cmp.Compare(b.layer, a.layer), cmp.Compare(b.y, a.y), cmp.Compare(b.index, a.index))
	})
	return hs
}

// minPickPx is the least size, in device-independent window pixels, of the
// area that picks an object. A tiny sprite or a zoomed-out view would
// otherwise leave a target of a pixel or two.
const minPickPx = 12

// deviceScale is the display's scale factor (devicePixelRatio in a browser):
// window pixels are device pixels. It is a variable so tests can set it.
var deviceScale = func() float64 {
	if m := ebiten.Monitor(); m != nil {
		return max(m.DeviceScaleFactor(), 1)
	}
	return 1
}

// Pick returns the id of the front-most object of the level whose bounds
// contain the world position p. When none does, an object smaller on screen
// than minPickPx is picked within that size around its center (the nearest
// center wins), so small objects stay clickable.
func (s *Session) Pick(p gfx.Vec2) (string, bool) {
	hs := s.hits()
	for _, h := range hs {
		if h.bounds.Contains(p) {
			return h.id, true
		}
	}
	unit := s.windowRect(gfx.Rect{Max: gfx.Vec2{X: 1, Y: 1}})
	pxPerUnit := unit.Max.X - unit.Min.X
	if pxPerUnit <= 0 {
		return "", false
	}
	least := minPickPx * float32(deviceScale()) / pxPerUnit // in world units
	best, bestD := "", float32(math.Inf(1))
	for _, h := range hs {
		if atLeast(h.bounds, least).Contains(p) {
			c := gfx.Vec2{X: (h.bounds.Min.X + h.bounds.Max.X) / 2, Y: (h.bounds.Min.Y + h.bounds.Max.Y) / 2}
			if d := p.Sub(c).Len(); d < bestD {
				best, bestD = h.id, d
			}
		}
	}
	return best, best != ""
}

// Bounds returns the world bounds of the object with id, as picking sees
// them.
func (s *Session) Bounds(id string) (gfx.Rect, bool) {
	o, ok := s.objs[id]
	if !ok || !s.e.World().IsAlive(o.entity) {
		return gfx.Rect{}, false
	}
	r, _, ok := bounds(s.e.World(), s.e.Stores().Sprites(), s.alpha, o.entity)
	return r, ok
}
