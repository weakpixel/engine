package hierarchy

import (
	"testing"

	"github.com/weakpixel/ecs"
	"github.com/weakpixel/engine/gfx"
)

func TestSystemChildFollowsParent(t *testing.T) {
	tw := newTestWorld(t)
	p := tw.w.NewEntity(gfx.Transform{Pos: v(10, 20)})
	c := tw.child(p, v(1, 2))
	tw.run(1)
	if got := tw.transform(c).Pos; got != v(11, 22) {
		t.Fatalf("child at %v, want (11, 22)", got)
	}
	ecs.GetComp[gfx.Transform](tw.w, p).Pos = v(0, 0)
	tw.run(1)
	if got := tw.transform(c).Pos; got != v(1, 2) {
		t.Fatalf("child at %v, want (1, 2)", got)
	}
}

func TestSystemGrandchildAnyOrder(t *testing.T) {
	// Spawn the grandchild first and the root last: the walk does not depend
	// on the order of the archetype rows.
	tw := newTestWorld(t)
	root := tw.w.NewEntity(gfx.Transform{Pos: v(100, 0), Rot: 0, Scale: v(2, 2)})
	mid := tw.w.NewEntity(gfx.Transform{}, Parent{Entity: root, Local: Local{Pos: v(10, 0)}})
	leaf := tw.w.NewEntity(gfx.Transform{}, Parent{Entity: mid, Local: Local{Pos: v(1, 0)}})
	tw.run(1)
	if got := tw.transform(mid).Pos; got != v(120, 0) {
		t.Fatalf("mid at %v, want (120, 0)", got)
	}
	// leaf: root + 2*(10) + (2*1) = 122
	if got := tw.transform(leaf); got.Pos != v(122, 0) || got.Scale != v(2, 2) {
		t.Fatalf("leaf = %+v, want pos (122, 0) scale (2, 2)", got)
	}
}

func TestSystemFirstPlacementSetsPrevPos(t *testing.T) {
	tw := newTestWorld(t)
	p := tw.w.NewEntity(gfx.Transform{Pos: v(50, 50)})
	c := tw.child(p, v(5, 0))
	tw.run(1)
	if got := tw.transform(c); got.Pos != v(55, 50) || got.PrevPos != got.Pos {
		t.Fatalf("after first run %+v, want PrevPos = Pos = (55, 50)", got)
	}
	// Later runs leave PrevPos to the snapshot system.
	ecs.GetComp[gfx.Transform](tw.w, p).Pos = v(60, 50)
	tw.run(1)
	if got := tw.transform(c); got.Pos != v(65, 50) || got.PrevPos != v(55, 50) {
		t.Fatalf("after second run %+v, want Pos (65, 50) PrevPos (55, 50)", got)
	}
}

func TestSystemDeletesDescendantsOfDeadParent(t *testing.T) {
	tw := newTestWorld(t)
	p := tw.w.NewEntity(gfx.Transform{})
	c := tw.child(p, v(1, 0))
	g := tw.child(c, v(1, 0))
	other := tw.w.NewEntity(gfx.Transform{})
	tw.run(1)
	tw.w.DeleteEntity(p)
	tw.run(1)
	if tw.w.IsAlive(c) || tw.w.IsAlive(g) {
		t.Fatalf("child alive %v, grandchild alive %v; want both deleted", tw.w.IsAlive(c), tw.w.IsAlive(g))
	}
	if !tw.w.IsAlive(other) {
		t.Fatal("unrelated entity was deleted")
	}
}

func TestSystemCyclePanics(t *testing.T) {
	tw := newTestWorld(t)
	a := tw.w.NewEntity(gfx.Transform{}, Parent{})
	b := tw.child(a, v(0, 0))
	ecs.GetComp[Parent](tw.w, a).Entity = b
	mustPanic(t, "hierarchy: cycle or chain deeper than 32", func() { tw.run(1) })
}

func TestSystemChainTooDeepPanics(t *testing.T) {
	tw := newTestWorld(t)
	e := tw.w.NewEntity(gfx.Transform{})
	for range MaxDepth + 1 {
		e = tw.child(e, v(1, 0))
	}
	mustPanic(t, "hierarchy: cycle", func() { tw.run(1) })
}

func TestSystemRootY(t *testing.T) {
	tw := newTestWorld(t)
	p := tw.w.NewEntity(gfx.Transform{Pos: v(0, 77)})
	c := tw.child(p, v(0, 5))
	g := tw.child(c, v(0, 5))
	tw.run(1)
	for _, e := range []ecs.Entity{c, g} {
		if y := ecs.GetComp[Parent](tw.w, e).RootY(); y != 77 {
			t.Fatalf("RootY = %v, want 77", y)
		}
	}
}
