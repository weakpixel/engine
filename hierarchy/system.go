package hierarchy

import (
	"fmt"
	"math"

	"github.com/weakpixel/ecs"
	"github.com/weakpixel/ecs/system"
	"github.com/weakpixel/engine/gfx"
)

// MaxDepth is the longest parent chain the [System] walks. A longer chain,
// or a cycle, panics.
const MaxDepth = 32

// System writes the world gfx.Transform and the draw layers of every entity
// with a [Parent], and deletes the children of dead parents.
//
// For each child it walks up to the root (the first ancestor without a
// Parent) and composes the Locals of the chain onto the root's Transform. It
// never reads an intermediate ancestor's Transform or layer, so the order in
// which the entities are visited does not matter and no child lists or depth
// tags are needed. The cost is O(children x depth), with 0 allocations.
//
// A root without a Transform counts as a zero Transform.
type System struct {
	w    *ecs.World
	view ecs.View2[gfx.Transform, Parent]
	// Optional components looked up per entity and tick: views resolve the
	// component ids once, where ecs.GetCompOrNil looks the type up per call.
	transforms ecs.View1[gfx.Transform]
	parents    ecs.View1[Parent]
	sprites    ecs.View1[gfx.Sprite]
	nineSlices ecs.View1[gfx.NineSlice]
	labels     ecs.View1[gfx.Label]
	particles  ecs.View1[gfx.Particles]
	dead       []ecs.Entity // reused; entities to delete after the loop
	chain      [MaxDepth]*Parent
}

// NewSystem returns the System of w.
func NewSystem(w *ecs.World) *System {
	return &System{
		w:          w,
		view:       ecs.NewView2[gfx.Transform, Parent](w),
		transforms: ecs.NewView1[gfx.Transform](w),
		parents:    ecs.NewView1[Parent](w),
		sprites:    ecs.NewView1[gfx.Sprite](w),
		nineSlices: ecs.NewView1[gfx.NineSlice](w),
		labels:     ecs.NewView1[gfx.Label](w),
		particles:  ecs.NewView1[gfx.Particles](w),
	}
}

// Run implements system.Runner.
func (s *System) Run(*system.Context) {
	s.dead = s.dead[:0]
	for e, c := range s.view.Each() {
		t, p := c.Unpack()
		n, root, ok := s.walk(e, p)
		if !ok {
			s.dead = append(s.dead, e)
			continue
		}
		world := gfx.Transform{}
		if rt := s.transforms.GetOrNil(root); rt != nil {
			world = *rt
		}
		rootY := world.Pos.Y
		for i := n - 1; i >= 0; i-- {
			world = Compose(world, s.chain[i].Local)
		}
		t.Pos, t.Rot, t.Scale = world.Pos, world.Rot, world.Scale
		if !p.placed {
			t.PrevPos = t.Pos
			p.placed = true
		}
		p.rootY = rootY
		if p.Local.LayerMode == LayerRelative {
			s.writeLayers(e, n)
		}
	}
	// Delete after the loop: structural changes are not allowed in Each. All
	// descendants of a dead entity found a dead link in their own walk, so
	// they are in the list too.
	for _, e := range s.dead {
		if s.w.IsAlive(e) {
			s.w.DeleteEntity(e)
		}
	}
}

// walk fills s.chain with the Parent components from e (index 0) up to, but
// not including, the root, and returns the chain length and the root. ok is
// false when a link of the chain is dead.
func (s *System) walk(e ecs.Entity, p *Parent) (n int, root ecs.Entity, ok bool) {
	for {
		if n == MaxDepth {
			panic(fmt.Sprintf("hierarchy: cycle or chain deeper than %d at %v", MaxDepth, e))
		}
		s.chain[n] = p
		n++
		if !s.w.IsAlive(p.Entity) {
			return n, p.Entity, false
		}
		pp := s.parents.GetOrNil(p.Entity)
		if pp == nil {
			return n, p.Entity, true
		}
		p = pp
	}
}

// writeLayers sets the layer of every drawn component of e to the base of
// its chain plus e's LayerOffset. The base comes from the nearest ancestor
// with a drawn component, plus the offsets of the relative ancestors
// between them. A relative ancestor's own layer is derived, so the walk
// goes through it instead of reading it (the visiting order must not
// matter); an absolute or root ancestor's layer is authored and read.
func (s *System) writeLayers(e ecs.Entity, n int) {
	sp := s.sprites.GetOrNil(e)
	ns := s.nineSlices.GetOrNil(e)
	lb := s.labels.GetOrNil(e)
	pt := s.particles.GetOrNil(e)
	if sp == nil && ns == nil && lb == nil && pt == nil {
		return
	}
	acc := int32(s.chain[0].Local.LayerOffset)
	for i := 1; i <= n; i++ {
		// Ancestor i is chain[i-1].Entity; it has the Parent chain[i] unless
		// i == n, when it is the root.
		anc := s.chain[i-1].Entity
		if i < n && s.chain[i].Local.LayerMode == LayerRelative {
			acc += int32(s.chain[i].Local.LayerOffset)
			continue
		}
		if l, ok := s.drawnLayer(anc); ok {
			acc += int32(l)
			break
		}
	}
	layer := int16(max(math.MinInt16, min(math.MaxInt16, acc)))
	if sp != nil {
		sp.Layer = layer
	}
	if ns != nil {
		ns.Layer = layer
	}
	if lb != nil {
		lb.Layer = layer
	}
	if pt != nil {
		pt.Layer = layer
	}
}

// drawnLayer returns the layer of e's first drawn component: Sprite, then
// NineSlice, Label, Particles.
func (s *System) drawnLayer(e ecs.Entity) (int16, bool) {
	if sp := s.sprites.GetOrNil(e); sp != nil {
		return sp.Layer, true
	}
	if ns := s.nineSlices.GetOrNil(e); ns != nil {
		return ns.Layer, true
	}
	if lb := s.labels.GetOrNil(e); lb != nil {
		return lb.Layer, true
	}
	if pt := s.particles.GetOrNil(e); pt != nil {
		return pt.Layer, true
	}
	return 0, false
}
