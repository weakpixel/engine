package editor

import (
	"image"
	"math"
	"slices"
	"strings"
	"testing"

	"github.com/weakpixel/ecs"
	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/input"
)

// gizmoFixture opens the levelDoc objects and returns the fixture plus a
// function mapping world positions to logical pixels.
func gizmoFixture(t *testing.T, objs ...string) (*fixture, func(x, y float32) gfx.Vec2) {
	t.Helper()
	f := newFixture(t, Config{})
	f.open(doc(objs...))
	f.settle()
	f.update()
	return f, func(x, y float32) gfx.Vec2 {
		return f.e.Camera().WorldToScreen(gfx.Vec2{X: x, Y: y}, image.Pt(screenW, screenH))
	}
}

func (f *fixture) keys(ks ...input.Key) {
	f.src.Frame.Keys = input.KeySet{}
	for _, k := range ks {
		f.src.Frame.Keys.Set(k)
	}
}

// gesture presses at from, moves to each of the points and releases there.
func (f *fixture) gesture(from gfx.Vec2, via ...gfx.Vec2) {
	f.t.Helper()
	f.mouse(from.X, from.Y, input.MouseLeft)
	f.update()
	last := from
	for _, p := range via {
		f.mouse(p.X, p.Y, input.MouseLeft)
		f.update()
		last = p
	}
	f.mouse(last.X, last.Y)
	f.update()
}

func (f *fixture) edits() (n int) {
	for _, ev := range f.events {
		if _, ok := ev.(Edit); ok {
			n++
		}
	}
	return n
}

func (f *fixture) transform(id string) gfx.Transform {
	return *ecs.GetComp[gfx.Transform](f.e.World(), f.entity(id))
}

func TestRotateGizmo(t *testing.T) {
	f, px := gizmoFixture(t, visual("a", 0, 0, 0))
	f.s.Select("a")
	o := px(0, 0)
	// The knob stands at angle Rot=0 on the ring; drag it to straight down.
	f.gesture(gfx.Vec2{X: o.X + rotateRadius, Y: o.Y}, gfx.Vec2{X: o.X, Y: o.Y + 30})
	if got := f.transform("a").Rot; math.Abs(float64(got)-math.Pi/2) > 1e-4 {
		t.Errorf("Rot = %v, want pi/2", got)
	}
	if f.edits() != 1 {
		t.Fatalf("%d edits, want 1", f.edits())
	}
	ed := f.events[len(f.events)-1].(Edit)
	if ed.ID != "a" || !strings.Contains(string(ed.JSON), `"Rot":1.5707`) {
		t.Errorf("edit %s", ed.JSON)
	}
	if f.transform("a").Pos != (gfx.Vec2{}) {
		t.Errorf("rotate moved the object to %v", f.transform("a").Pos)
	}

	// Shift snaps to 15 degrees.
	f.keys(input.KeyShiftLeft)
	k := rotateKnob(o, f.transform("a").Rot)
	f.gesture(k, gfx.Vec2{X: o.X + 40, Y: o.Y + 41})
	if got := f.transform("a").Rot; math.Abs(float64(got)-math.Pi/4) > 1e-4 {
		t.Errorf("snapped Rot = %v, want pi/4", got)
	}
}

func TestScaleGizmo(t *testing.T) {
	f, px := gizmoFixture(t, visual("a", 0, 0, 0))
	f.s.Select("a")
	o := px(0, 0)
	box := scaleBox(o)
	// Twice as far from the object: twice the scale, on both axes.
	f.gesture(box, gfx.Vec2{X: o.X + 2*scaleOffset, Y: o.Y + 2*scaleOffset})
	if got := f.transform("a").Scale; got != (gfx.Vec2{X: 2, Y: 2}) {
		t.Errorf("Scale = %v, want (2, 2)", got)
	}
	if f.edits() != 1 {
		t.Errorf("%d edits, want 1", f.edits())
	}
	f.keys(input.KeyShiftLeft)
	f.gesture(scaleBox(o), gfx.Vec2{X: o.X + 1.3*scaleOffset, Y: o.Y + 1.3*scaleOffset})
	if got := f.transform("a").Scale; got != (gfx.Vec2{X: 2.5, Y: 2.5}) {
		t.Errorf("snapped Scale = %v, want (2.5, 2.5)", got)
	}
}

func TestGizmosHiddenForSeveral(t *testing.T) {
	f, px := gizmoFixture(t, visual("a", 0, 0, 0), visual("b", 50, 0, 0))
	f.s.Select("a", "b")
	if _, ok := f.s.pickGizmo(scaleBox(px(0, 0))); ok {
		t.Error("gizmo for two objects")
	}
}

func TestShiftClickToggles(t *testing.T) {
	f, px := gizmoFixture(t, visual("a", 0, 0, 0), visual("b", 70, 40, 0), visual("c", 100, 0, 0))
	f.gesture(px(0, 0))
	f.keys(input.KeyShiftLeft)
	f.gesture(px(70, 40))
	f.gesture(px(100, 0))
	if got := f.s.Selected(); !slices.Equal(got, []string{"a", "b", "c"}) {
		t.Fatalf("selected %v", got)
	}
	f.gesture(px(70, 40))
	if got := f.s.Selected(); !slices.Equal(got, []string{"a", "c"}) {
		t.Errorf("after toggling b off: %v", got)
	}
	if f.edits() != 0 {
		t.Errorf("clicking emitted %d edits", f.edits())
	}
	// Without a modifier a click on an object replaces the selection.
	f.keys()
	f.gesture(px(100, 0))
	if got := f.s.Selected(); !slices.Equal(got, []string{"c"}) {
		t.Errorf("plain click: %v", got)
	}
}

func TestBoxSelect(t *testing.T) {
	f, px := gizmoFixture(t, visual("a", 0, 0, 0), visual("b", 50, 0, 0), visual("c", 100, 80, 0))
	// A box over a and b (not c), started on empty space.
	from, to := px(-30, -30), px(60, 20)
	f.gesture(from, to)
	if got := f.s.Selected(); !slices.Equal(got, []string{"a", "b"}) {
		t.Fatalf("box selected %v", got)
	}
	if f.edits() != 0 {
		t.Errorf("box emitted %d edits", f.edits())
	}
	// A box with Shift adds.
	f.keys(input.KeyShiftLeft)
	f.gesture(px(90, 60), px(120, 100))
	if got := f.s.Selected(); !slices.Equal(got, []string{"a", "b", "c"}) {
		t.Errorf("shift box: %v", got)
	}
	// A plain box replaces.
	f.keys()
	f.gesture(px(90, 60), px(120, 100))
	if got := f.s.Selected(); !slices.Equal(got, []string{"c"}) {
		t.Errorf("plain box: %v", got)
	}
}

func TestGroupMove(t *testing.T) {
	f, px := gizmoFixture(t, visual("a", 0, 0, 0), visual("b", 50, 0, 0), visual("c", 100, 80, 0))
	f.s.Select("a", "b")
	a := px(0, 0)
	f.gesture(a, gfx.Vec2{X: a.X + 10, Y: a.Y + 5})
	if f.pos("a") != (gfx.Vec2{X: 10, Y: 5}) || f.pos("b") != (gfx.Vec2{X: 60, Y: 5}) || f.pos("c") != (gfx.Vec2{X: 100, Y: 80}) {
		t.Errorf("positions a=%v b=%v c=%v", f.pos("a"), f.pos("b"), f.pos("c"))
	}
	var ids []string
	for _, ev := range f.events {
		if ed, ok := ev.(Edit); ok {
			ids = append(ids, ed.ID)
		}
	}
	if !slices.Equal(ids, []string{"a", "b"}) {
		t.Errorf("edits for %v, want a and b", ids)
	}
	if got := f.s.Selected(); !slices.Equal(got, []string{"a", "b"}) {
		t.Errorf("selection %v", got)
	}
}

// TestGroupClickNarrows: a press on a selected object of several that does
// not move selects only it.
func TestGroupClickNarrows(t *testing.T) {
	f, px := gizmoFixture(t, visual("a", 0, 0, 0), visual("b", 50, 0, 0))
	f.s.Select("a", "b")
	f.gesture(px(50, 0))
	if got := f.s.Selected(); !slices.Equal(got, []string{"b"}) {
		t.Errorf("selection %v", got)
	}
}

func TestSetColliders(t *testing.T) {
	f := newFixture(t, Config{})
	if f.s.hideColliders {
		t.Error("colliders hidden by default")
	}
	f.s.SetColliders(false)
	if !f.s.hideColliders {
		t.Error("SetColliders(false) did not hide them")
	}
}
