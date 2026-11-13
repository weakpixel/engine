package collision

import (
	"iter"
	"slices"
	"sort"

	"github.com/weakpixel/ecs"
	"github.com/weakpixel/ecs/system"
	"github.com/weakpixel/engine/gfx"
)

// Space holds the collision state of a World: the colliders placed in the
// last [System] run, their contacts, the tile grids and the custom shapes.
// It is a World resource; get it with [SpaceOf]. Queries and [Space.Contacts]
// read the state of the last run, so a collider spawned or moved this tick
// counts from the next run on.
type Space struct {
	narrow    narrow
	filter    func(self, other ecs.Entity) bool
	cellSize  float32 // the length of the pieces Raycast queries
	hasSystem bool

	view        ecs.View2[gfx.Transform, Collider]
	enter       *system.Events[Enter]
	stay        *system.Events[Stay] // nil unless Config.Stay
	exit        *system.Events[Exit]
	dyn, stat   Broadphase
	dynamic     []Shape       // by dynamic Proxy, from the last run
	statics     []Shape       // by static Proxy
	staticIn    []staticInput // what the statics were built from
	newStaticIn []staticInput // statics collected this run, compared with staticIn

	tiles    []tileGrid
	nextGrid uint32

	// Scratch and results, reused every run so a warm run does not
	// allocate.
	pairs          []Pair
	hits           []Proxy
	contacts       []Contact // sorted by cmpContact
	sortKeys       []sortKey // sortContacts scratch
	sortedContacts []Contact // and the buffer it gathers into
	keys           []pairKey // detecting pairs of this run, sorted
	prevKeys       []pairKey // and of the run before
	first          []int     // index in contacts of each key's first contact
	queryBuf       [][]Proxy // a stack of query buffers, for nested queries
	cell           Shape     // the tile cell tileContacts tests
}

// SpaceOf returns w's Space, creating it on first use with the default grid
// broad phase. Game systems, physics and the debug overlay share it.
func SpaceOf(w *ecs.World) *Space {
	if s := ecs.GetResourceOrNil[Space](w); s != nil {
		return s
	}
	s := &Space{
		view:  ecs.NewView2[gfx.Transform, Collider](w),
		enter: system.EventsOf[Enter](w),
		exit:  system.EventsOf[Exit](w),
		dyn:   NewGrid(DefaultCellSize),
		stat:  NewGrid(DefaultCellSize),

		cellSize: DefaultCellSize,
	}
	ecs.SetResource(w, s)
	// The debug overlay (extract.DebugExtract) outlines the colliders of the
	// last run through this resource, so extract does not import collision.
	ecs.SetResource(w, &gfx.DebugOutlines{Outlines: s.debugOutlines})
	return s
}

// RegisterShape makes a custom kind (ShapeCustom or above) usable: bounds
// returns the world box of a collider of that kind. Register the narrow
// phase of each pair of kinds that can meet with [Space.RegisterPair].
// Queries and rays test custom shapes by their bounds.
//
// Panics with "collision: shape kind <k> is built in" for a kind below
// ShapeCustom and "collision: nil bounds" for a nil bounds.
func (s *Space) RegisterShape(kind ShapeKind, bounds BoundsFunc) {
	if isBuiltin(kind) {
		panic("collision: shape kind " + kind.String() + " is built in")
	}
	if bounds == nil {
		panic("collision: nil bounds")
	}
	if s.narrow.bounds == nil {
		s.narrow.bounds = map[ShapeKind]BoundsFunc{}
	}
	s.narrow.bounds[kind] = bounds
}

// RegisterPair sets the narrow phase for shapes of kinds a and b; the
// reverse pair (b, a) is derived from it by flipping the normal. One of the
// kinds must be custom; ShapeRect stands for ShapeAnimBody too (tile cells
// are ShapeRect). A pair that meets without a narrow phase panics with
// "collision: no narrow phase for shapes <a> and <b>".
//
// Panics with "collision: shapes <a> and <b> are built in" if neither kind
// is custom and "collision: nil narrow phase" for a nil fn.
func (s *Space) RegisterPair(a, b ShapeKind, fn NarrowFunc) {
	if isBuiltin(a) && isBuiltin(b) {
		panic("collision: shapes " + a.String() + " and " + b.String() + " are built in")
	}
	if fn == nil {
		panic("collision: nil narrow phase")
	}
	if s.narrow.pairs == nil {
		s.narrow.pairs = map[pairKind]NarrowFunc{}
	}
	s.narrow.pairs[pairKind{a, b}] = fn
	if a != b {
		s.narrow.pairs[pairKind{b, a}] = func(self, other *Shape) (gfx.Vec2, float32, bool) {
			n, d, ok := fn(other, self)
			return n.Mul(-1), d, ok
		}
	}
}

// Contacts returns the contacts in which e is Self in the last run, sorted
// by Other and then by tile cell. The slice belongs to the Space: it is
// valid until the next run and must not be changed.
func (s *Space) Contacts(e ecs.Entity) []Contact {
	cs := s.contacts
	i := sort.Search(len(cs), func(i int) bool { return cmpEntity(cs[i].Self, e) >= 0 })
	j := i
	for j < len(cs) && cs[j].Self == e {
		j++
	}
	return cs[i:j:j]
}

// AllContacts returns every contact of the last run, sorted by Self, Other
// and cell. Same ownership as [Space.Contacts].
func (s *Space) AllContacts() []Contact { return s.contacts[:len(s.contacts):len(s.contacts)] }

// Shapes yields the colliders placed in the last run, dynamic ones first:
// what the debug overlay draws. The shapes belong to the Space and are
// valid until the next run.
func (s *Space) Shapes() iter.Seq[*Shape] {
	return func(yield func(*Shape) bool) {
		for _, set := range [2][]Shape{s.dynamic, s.statics} {
			for i := range set {
				if !yield(&set[i]) {
					return
				}
			}
		}
	}
}

// detect places the colliders and finds the contacts, without events.
func (s *Space) detect() {
	s.collect()
	s.contacts = s.contacts[:0]
	s.pairs = s.dyn.Pairs(s.pairs[:0])
	for _, p := range s.pairs {
		s.testPair(&s.dynamic[p.A], &s.dynamic[p.B])
	}
	for i := range s.dynamic {
		d := &s.dynamic[i]
		s.hits = s.stat.Query(d.Bounds, s.hits[:0])
		for _, h := range s.hits {
			s.testPair(d, &s.statics[h])
		}
		for t := range s.tiles {
			if d.Mask.Any(s.tiles[t].layer) {
				s.tileContacts(d, &s.tiles[t])
			}
		}
	}
	s.sortContacts()
}

// sortContacts orders the contacts by cmpContact. It sorts small integer
// keys and then moves each contact once, which is several times faster than
// sorting the contacts themselves when a tick has thousands of them.
func (s *Space) sortContacts() {
	if len(s.contacts) < 2 {
		return
	}
	s.sortKeys = s.sortKeys[:0]
	for i := range s.contacts {
		s.sortKeys = append(s.sortKeys, newSortKey(&s.contacts[i], i))
	}
	slices.SortFunc(s.sortKeys, cmpSortKey)
	s.sortedContacts = s.sortedContacts[:0]
	for _, k := range s.sortKeys {
		s.sortedContacts = append(s.sortedContacts, s.contacts[k.i])
	}
	s.contacts, s.sortedContacts = s.sortedContacts, s.contacts
}

// staticInput is what the shape of a static collider is made from. The
// Space compares these every run and builds a Shape only when one changed.
type staticInput struct {
	entity    ecs.Entity
	transform gfx.Transform
	collider  Collider
}

// collect places every collider, refills the dynamic broad phase and, when
// a static collider was added, removed or changed, the static one. The
// static check is a compare of the statics' Transform and Collider with
// last run's, so a "static" door moved from code or the editor never leaves
// a stale grid, and a run without a change builds no static Shape.
func (s *Space) collect() {
	s.dynamic = s.dynamic[:0]
	s.newStaticIn = s.newStaticIn[:0]
	changed := false
	for e, c := range s.view.Each() {
		if c.Comp2.Static {
			in := staticInput{e, *c.Comp1, *c.Comp2}
			i := len(s.newStaticIn)
			changed = changed || i >= len(s.staticIn) || s.staticIn[i] != in
			s.newStaticIn = append(s.newStaticIn, in)
			continue
		}
		if sh, ok := makeShape(e, c.Comp1, c.Comp2, &s.narrow); ok {
			s.dynamic = append(s.dynamic, sh)
		}
	}
	s.dyn.Reset()
	for i := range s.dynamic {
		s.dyn.Insert(Proxy(i), s.dynamic[i].Bounds, s.dynamic[i].Layer, s.dynamic[i].Mask)
	}
	if changed || len(s.newStaticIn) != len(s.staticIn) {
		s.staticIn, s.newStaticIn = s.newStaticIn, s.staticIn
		s.statics = s.statics[:0]
		for i := range s.staticIn {
			in := &s.staticIn[i]
			if sh, ok := makeShape(in.entity, &in.transform, &in.collider, &s.narrow); ok {
				s.statics = append(s.statics, sh)
			}
		}
		s.stat.Reset()
		for i := range s.statics {
			s.stat.Insert(Proxy(i), s.statics[i].Bounds, s.statics[i].Layer, s.statics[i].Mask)
		}
	}
}

// testPair tests a candidate pair and adds a contact for each side that
// detects the other ([Collider.Detects] and the filter).
func (s *Space) testPair(a, b *Shape) {
	ad := a.Collider.Detects(&b.Collider) && (s.filter == nil || s.filter(a.Entity, b.Entity))
	bd := b.Collider.Detects(&a.Collider) && (s.filter == nil || s.filter(b.Entity, a.Entity))
	if !ad && !bd {
		return
	}
	n, d, ok := s.narrow.test(a, b)
	if !ok {
		return
	}
	if ad {
		s.contacts = append(s.contacts, Contact{Self: a.Entity, Other: b.Entity, Normal: n, Depth: d})
	}
	if bd {
		s.contacts = append(s.contacts, Contact{Self: b.Entity, Other: a.Entity, Normal: n.Mul(-1), Depth: d})
	}
}

// diff turns the sorted contacts into this run's pair keys and sends Enter
// for keys that are new, Stay for keys that carried over (when enabled) and
// Exit for keys that are gone: a merge of two sorted slices, no map.
func (s *Space) diff() {
	s.keys, s.first = s.keys[:0], s.first[:0]
	for i := range s.contacts {
		c := &s.contacts[i]
		if i > 0 && cmpPair(&s.contacts[i-1], c) == 0 {
			continue
		}
		s.keys = append(s.keys, pairKey{self: c.Self, other: c.Other, grid: c.grid, tiles: c.Tiles})
		s.first = append(s.first, i)
	}
	cur, prev := s.keys, s.prevKeys
	i, j := 0, 0
	for i < len(cur) || j < len(prev) {
		var c int
		switch {
		case i == len(cur):
			c = 1
		case j == len(prev):
			c = -1
		default:
			c = cmpKey(&cur[i], &prev[j])
		}
		switch {
		case c < 0:
			s.enter.Send(Enter{s.contacts[s.first[i]]})
			i++
		case c > 0:
			p := &prev[j]
			s.exit.Send(Exit{Self: p.self, Other: p.other, Tiles: p.tiles})
			j++
		default:
			if s.stay != nil {
				s.stay.Send(Stay{s.contacts[s.first[i]]})
			}
			i++
			j++
		}
	}
	clear(s.prevKeys) // drop the grid references of the old keys
	s.keys, s.prevKeys = s.prevKeys[:0], s.keys
}
