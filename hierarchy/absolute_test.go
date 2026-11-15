package hierarchy

import (
	"fmt"
	"testing"

	"github.com/weakpixel/ecs"
	"github.com/weakpixel/ecs/system"
	"github.com/weakpixel/engine/gfx"
)

// absChild spawns an entity attached to parent at offset pos with
// PlaceAbsolute.
func (tw *testWorld) absChild(parent ecs.Entity, pos gfx.Vec2, values ...any) ecs.Entity {
	vals := []any{gfx.Transform{}, Parent{Entity: parent, Local: Local{Pos: pos, PlaceMode: PlaceAbsolute}}}
	return tw.w.NewEntity(append(vals, values...)...)
}

func TestAbsoluteChild(t *testing.T) {
	tests := []struct {
		name  string
		move  func(*gfx.Transform)
		first gfx.Vec2 // after the first run
	}{
		{"move", func(p *gfx.Transform) { p.Pos = v(100, 100) }, v(15, 20)},
		{"rotate", func(p *gfx.Transform) { p.Rot = 1 }, v(15, 20)},
		{"scale", func(p *gfx.Transform) { p.Scale = v(3, -2) }, v(15, 20)},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tw := newTestWorld(t)
			p := tw.w.NewEntity(gfx.Transform{Pos: v(10, 20)})
			c := tw.absChild(p, v(5, 0))
			tw.run(1)
			got := tw.transform(c)
			if got.Pos != tt.first || got.PrevPos != got.Pos {
				t.Fatalf("first placement %+v, want Pos = PrevPos = %v (from Local)", got, tt.first)
			}
			tt.move(ecs.GetComp[gfx.Transform](tw.w, p))
			tw.run(2)
			if after := tw.transform(c); after != got {
				t.Fatalf("after the parent changed %+v, want it unchanged %+v", after, got)
			}
			// The child's own Transform is its to change.
			ecs.GetComp[gfx.Transform](tw.w, c).Pos = v(-3, -4)
			tw.run(1)
			if after := tw.transform(c).Pos; after != v(-3, -4) {
				t.Fatalf("own move overwritten: at %v, want (-3, -4)", after)
			}
		})
	}
}

// TestAbsoluteAncestor: a relative grandchild follows its absolute parent,
// whatever order the entities were spawned in, and sorts by its Y.
func TestAbsoluteAncestor(t *testing.T) {
	for _, order := range []string{"root first", "leaf first"} {
		t.Run(order, func(t *testing.T) {
			tw := newTestWorld(t)
			var root, mid, leaf ecs.Entity
			spawn := []func(){
				func() { root = tw.w.NewEntity(gfx.Transform{Pos: v(100, 0), Scale: v(2, 2)}) },
				func() { mid = tw.w.NewEntity(gfx.Transform{}) },
				func() { leaf = tw.w.NewEntity(gfx.Transform{}) },
			}
			if order == "leaf first" {
				spawn[0], spawn[2] = spawn[2], spawn[0]
			}
			for _, f := range spawn {
				f()
			}
			// The order of SetComp is the order of the rows the System visits.
			attach := []func(){
				func() {
					ecs.SetComp(tw.w, mid, Parent{Entity: root, Local: Local{Pos: v(10, 5), PlaceMode: PlaceAbsolute}})
				},
				func() { ecs.SetComp(tw.w, leaf, Parent{Entity: mid, Local: Local{Pos: v(1, 0)}}) },
			}
			if order == "leaf first" {
				attach[0], attach[1] = attach[1], attach[0]
			}
			for _, f := range attach {
				f()
			}
			tw.run(1)
			// mid: 100 + 2*10, 0 + 2*5; leaf: mid + 2*1 (mid's world scale)
			if got := tw.transform(mid).Pos; got != v(120, 10) {
				t.Fatalf("mid at %v, want (120, 10)", got)
			}
			if got := tw.transform(leaf).Pos; got != v(122, 10) {
				t.Fatalf("leaf at %v, want (122, 10)", got)
			}
			for _, e := range []ecs.Entity{mid, leaf} {
				if y := ecs.GetComp[Parent](tw.w, e).RootY(); y != 10 {
					t.Fatalf("RootY = %v, want 10 (the absolute link's Y)", y)
				}
			}
			ecs.GetComp[gfx.Transform](tw.w, root).Pos = v(0, 0)
			ecs.GetComp[gfx.Transform](tw.w, mid).Pos = v(50, 50)
			tw.run(1)
			if got := tw.transform(leaf).Pos; got != v(52, 50) {
				t.Fatalf("leaf at %v, want (52, 50): it follows mid, not the root", got)
			}
		})
	}
}

func TestAbsoluteLifetimeAndLayer(t *testing.T) {
	tw := newTestWorld(t)
	p := tw.w.NewEntity(gfx.Transform{}, gfx.Sprite{Layer: 4})
	c := tw.absChild(p, v(1, 0), gfx.Sprite{})
	g := tw.child(c, v(1, 0))
	tw.run(1)
	ecs.GetComp[gfx.Sprite](tw.w, p).Layer = 7
	tw.run(1)
	if got := ecs.GetComp[gfx.Sprite](tw.w, c).Layer; got != 7 {
		t.Fatalf("absolute child's layer %d, want the parent's 7 (LayerRelative)", got)
	}
	tw.w.DeleteEntity(p)
	tw.run(1)
	if tw.w.IsAlive(c) || tw.w.IsAlive(g) {
		t.Fatalf("absolute child alive %v, its child alive %v; want both deleted", tw.w.IsAlive(c), tw.w.IsAlive(g))
	}
}

// TestWorldOfAbsolute: WorldOf matches the System for chains with absolute
// links, before the first run and after the parent moved.
func TestWorldOfAbsolute(t *testing.T) {
	tw := newTestWorld(t)
	root := tw.w.NewEntity(gfx.Transform{Pos: v(10, 10), Rot: 0.5, Scale: v(2, 2)})
	mid := tw.absChild(root, v(5, 0))
	leaf := tw.child(mid, v(0, 3))
	check := func(when string) {
		t.Helper()
		var want [2]gfx.Transform
		for i, e := range []ecs.Entity{mid, leaf} {
			got, ok := WorldOf(tw.w, e)
			if !ok {
				t.Fatalf("%s: WorldOf not ok", when)
			}
			want[i] = got
		}
		tw.run(1)
		for i, e := range []ecs.Entity{mid, leaf} {
			if got := tw.transform(e); !nearT(got, want[i]) {
				t.Errorf("%s: WorldOf %+v, System wrote %+v", when, want[i], got)
			}
		}
	}
	check("before the first run")
	ecs.GetComp[gfx.Transform](tw.w, root).Pos = v(-40, 0)
	check("after the root moved")
}

func TestWorldOfAllocs(t *testing.T) {
	tw := newTestWorld(t)
	root := tw.w.NewEntity(gfx.Transform{Pos: v(10, 10)})
	leaf := tw.child(tw.absChild(tw.child(root, v(1, 0)), v(1, 0)), v(1, 0))
	tw.run(1)
	if got := testing.AllocsPerRun(50, func() { WorldOf(tw.w, leaf) }); got != 0 {
		t.Fatalf("WorldOf allocates %v per call, want 0", got)
	}
}

func TestSystemAllocsAbsolute(t *testing.T) {
	for _, depth := range []int{1, 3} {
		t.Run(fmt.Sprintf("depth%d", depth), func(t *testing.T) {
			w := ecs.NewWorld()
			for range 333 {
				e := w.NewEntity(gfx.Transform{}, gfx.Sprite{Layer: 1})
				for i := range depth {
					l := Local{Pos: v(1, 1)}
					if i == 0 {
						l.PlaceMode = PlaceAbsolute
					}
					e = w.NewEntity(gfx.Transform{}, gfx.Sprite{}, Parent{Entity: e, Local: l})
				}
			}
			sys := NewSystem(w)
			ctx := &system.Context{World: w}
			sys.Run(ctx)
			if got := testing.AllocsPerRun(20, func() { sys.Run(ctx) }); got != 0 {
				t.Fatalf("System.Run allocates %v per run, want 0", got)
			}
		})
	}
}

func TestSetPlace(t *testing.T) {
	setup := func(t *testing.T) (*testWorld, ecs.Entity, ecs.Entity) {
		tw := newTestWorld(t)
		p := tw.w.NewEntity(gfx.Transform{Pos: v(10, 0), Rot: 0.5, Scale: v(2, 3)})
		c := tw.absChild(p, v(4, 0))
		tw.run(1)
		ecs.GetComp[gfx.Transform](tw.w, p).Pos = v(-20, 7)
		ecs.GetComp[gfx.Transform](tw.w, c).Rot = 1.25
		tw.run(1)
		return tw, p, c
	}
	t.Run("relative keepWorld stays", func(t *testing.T) {
		tw, _, c := setup(t)
		want := tw.transform(c)
		SetPlace(tw.w, c, PlaceRelative, true)
		tw.run(1)
		if got := tw.transform(c); !nearT(got, want) {
			t.Fatalf("after SetPlace keepWorld %+v, want %+v", got, want)
		}
	})
	t.Run("relative returns to Local", func(t *testing.T) {
		tw, p, c := setup(t)
		SetPlace(tw.w, c, PlaceRelative, false)
		tw.run(1)
		want := Compose(tw.transform(p), Local{Pos: v(4, 0)})
		if got := tw.transform(c); !nearT(got, want) {
			t.Fatalf("after SetPlace %+v, want %+v (its Local)", got, want)
		}
	})
	t.Run("absolute stays", func(t *testing.T) {
		tw := newTestWorld(t)
		p := tw.w.NewEntity(gfx.Transform{Pos: v(10, 0)})
		c := tw.child(p, v(4, 0))
		tw.run(1)
		SetPlace(tw.w, c, PlaceAbsolute, false)
		ecs.GetComp[gfx.Transform](tw.w, p).Pos = v(0, 0)
		tw.run(1)
		if got := tw.transform(c).Pos; got != v(14, 0) {
			t.Fatalf("absolute child at %v, want (14, 0)", got)
		}
	})
	t.Run("panics", func(t *testing.T) {
		tw := newTestWorld(t)
		e := tw.w.NewEntity(gfx.Transform{})
		mustPanic(t, "hierarchy: SetPlace: ", func() { SetPlace(tw.w, e, PlaceAbsolute, false) })
		mustPanic(t, "has no Parent", func() { SetPlace(tw.w, e, PlaceAbsolute, false) })
	})
}

func TestReposition(t *testing.T) {
	tw := newTestWorld(t)
	p := tw.w.NewEntity(gfx.Transform{Pos: v(10, 0)})
	c := tw.absChild(p, v(4, 0))
	tw.run(1)
	ecs.GetComp[gfx.Transform](tw.w, p).Pos = v(50, 50)
	tw.run(1)
	Reposition(tw.w, c)
	tw.run(1)
	if got := tw.transform(c); got.Pos != v(54, 50) || got.PrevPos != got.Pos {
		t.Fatalf("after Reposition %+v, want Pos = PrevPos = (54, 50)", got)
	}
	ecs.GetComp[gfx.Transform](tw.w, p).Pos = v(0, 0)
	tw.run(1)
	if got := tw.transform(c).Pos; got != v(54, 50) {
		t.Fatalf("absolute after Reposition moved to %v, want it to stay at (54, 50)", got)
	}
	e := tw.w.NewEntity(gfx.Transform{})
	mustPanic(t, "hierarchy: Reposition: ", func() { Reposition(tw.w, e) })
}
