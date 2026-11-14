package hierarchy

import (
	"slices"
	"strings"
	"testing"

	"github.com/weakpixel/ecs"
	"github.com/weakpixel/engine/gfx"
)

func TestAttachKeepWorld(t *testing.T) {
	tw := newTestWorld(t)
	p := tw.w.NewEntity(gfx.Transform{Pos: v(10, 10), Rot: 0.5, Scale: v(2, 2)})
	c := tw.w.NewEntity(gfx.Transform{Pos: v(30, 5), Rot: 1, Scale: v(3, 3)})
	before := tw.transform(c)
	Attach(tw.w, c, p, true)
	tw.run(1)
	if got := tw.transform(c); !nearT(got, before) {
		t.Fatalf("after Attach(keepWorld) %+v, want %+v", got, before)
	}
}

func TestAttachWithoutKeepWorldJumpsToParent(t *testing.T) {
	tw := newTestWorld(t)
	p := tw.w.NewEntity(gfx.Transform{Pos: v(10, 10)})
	c := tw.w.NewEntity(gfx.Transform{Pos: v(99, 99)})
	Attach(tw.w, c, p, false)
	tw.run(1)
	if got := tw.transform(c); got.Pos != v(10, 10) || got.PrevPos != v(10, 10) {
		t.Fatalf("child %+v, want at the parent with PrevPos set", got)
	}
}

func TestAttachAddsTransformAndRejectsCycle(t *testing.T) {
	tw := newTestWorld(t)
	p := tw.w.NewEntity(gfx.Transform{})
	c := tw.w.NewEntity()
	Attach(tw.w, c, p, false)
	if !ecs.HasComp[gfx.Transform](tw.w, c) {
		t.Fatal("Attach did not add a Transform")
	}
	mustPanic(t, "hierarchy: attach would create a cycle", func() { Attach(tw.w, p, c, false) })
	mustPanic(t, "hierarchy: attach would create a cycle", func() { Attach(tw.w, p, p, false) })
}

func TestDetachKeepsWorldTransform(t *testing.T) {
	tw := newTestWorld(t)
	p := tw.w.NewEntity(gfx.Transform{Pos: v(10, 0)})
	c := tw.child(p, v(5, 0))
	tw.run(1)
	Detach(tw.w, c)
	Detach(tw.w, c) // second call does nothing
	ecs.GetComp[gfx.Transform](tw.w, p).Pos = v(0, 0)
	tw.run(1)
	if got := tw.transform(c).Pos; got != v(15, 0) {
		t.Fatalf("detached child at %v, want (15, 0)", got)
	}
	if ecs.HasComp[Parent](tw.w, c) {
		t.Fatal("Parent still there")
	}
}

func TestChildrenAndDelete(t *testing.T) {
	tw := newTestWorld(t)
	p := tw.w.NewEntity(gfx.Transform{})
	a := tw.child(p, v(1, 0))
	b := tw.child(p, v(2, 0))
	g := tw.child(a, v(1, 0))
	other := tw.w.NewEntity(gfx.Transform{})
	got := Children(tw.w, p)
	if len(got) != 2 || !slices.Contains(got, a) || !slices.Contains(got, b) {
		t.Fatalf("Children = %v, want %v and %v", got, a, b)
	}
	Delete(tw.w, p)
	for _, e := range []ecs.Entity{p, a, b, g} {
		if tw.w.IsAlive(e) {
			t.Errorf("%v alive after Delete", e)
		}
	}
	if !tw.w.IsAlive(other) {
		t.Error("unrelated entity deleted")
	}
}

func TestCheckCycles(t *testing.T) {
	tw := newTestWorld(t)
	ok := tw.w.NewEntity(gfx.Transform{})
	okChild := tw.child(ok, v(0, 0))
	a := tw.w.NewEntity(gfx.Transform{}, Parent{})
	b := tw.child(a, v(0, 0))
	ecs.GetComp[Parent](tw.w, a).Entity = b
	x := tw.w.NewEntity(gfx.Transform{}, Parent{})
	ecs.GetComp[Parent](tw.w, x).Entity = x
	feeder := tw.child(a, v(0, 0))

	names := map[ecs.Entity]string{a: "a", b: "b", x: "x", feeder: "feeder", ok: "ok", okChild: "okChild"}
	name := func(e ecs.Entity) string { return names[e] }

	if err := CheckCycles(tw.w, []ecs.Entity{ok, okChild}, name); err != nil {
		t.Fatalf("healthy tree: %v", err)
	}
	err := CheckCycles(tw.w, []ecs.Entity{feeder, a, b, x, okChild}, name)
	if err == nil {
		t.Fatal("no error for cycles")
	}
	msg := err.Error()
	for _, want := range []string{"hierarchy: parent cycle a -> b", "hierarchy: parent cycle x"} {
		if !strings.Contains(msg, want) {
			t.Errorf("error %q lacks %q", msg, want)
		}
	}
	if strings.Count(msg, "parent cycle") != 2 {
		t.Errorf("want 2 cycles reported once each, got %q", msg)
	}
}

// TestWorldOf: WorldOf matches what the System writes, before the System ran.
func TestWorldOf(t *testing.T) {
	tw := newTestWorld(t)
	root := tw.w.NewEntity(gfx.Transform{Pos: v(10, 10), Rot: 0.5, Scale: v(2, 2)})
	mid := tw.w.NewEntity(gfx.Transform{})
	leaf := tw.w.NewEntity(gfx.Transform{})
	ecs.SetComp(tw.w, mid, Parent{Entity: root, Local: Local{Pos: v(5, 0), Rot: 0.25}})
	ecs.SetComp(tw.w, leaf, Parent{Entity: mid, Local: Local{Pos: v(0, 3), Scale: v(1, 2)}})
	before, ok := WorldOf(tw.w, leaf)
	if !ok {
		t.Fatal("WorldOf of an attached entity is not ok")
	}
	tw.run(1)
	if got := tw.transform(leaf); !nearT(got, before) {
		t.Errorf("WorldOf before the run %+v, System wrote %+v", before, got)
	}
	if got, ok := WorldOf(tw.w, root); !ok || got != tw.transform(root) {
		t.Errorf("WorldOf of a root = %+v %v, want its Transform", got, ok)
	}
	tw.w.DeleteEntity(root)
	if _, ok := WorldOf(tw.w, leaf); ok {
		t.Error("WorldOf through a dead parent is ok")
	}
	if _, ok := WorldOf(tw.w, tw.w.NewEntity()); ok {
		t.Error("WorldOf of an entity without Transform or Parent is ok")
	}
}
