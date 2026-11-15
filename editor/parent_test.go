package editor

import (
	"image"
	"math"
	"strings"
	"testing"

	"github.com/weakpixel/ecs"
	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/hierarchy"
)

// knightOpen opens knightLevel and returns the fixture and a world-to-pixel
// function.
func knightOpen(t *testing.T, level string) (*fixture, func(x, y float32) gfx.Vec2) {
	t.Helper()
	f := knightFixture(t)
	f.open(level)
	if err := f.waitApplied(); err != nil {
		t.Fatal(err)
	}
	f.settle()
	f.update()
	f.events = nil
	return f, func(x, y float32) gfx.Vec2 {
		return f.e.Camera().WorldToScreen(gfx.Vec2{X: x, Y: y}, image.Pt(screenW, screenH))
	}
}

func (f *fixture) parent(id string) hierarchy.Parent {
	return *ecs.GetComp[hierarchy.Parent](f.e.World(), f.entity(id))
}

func (f *fixture) lastEdit() Edit {
	f.t.Helper()
	for i := len(f.events) - 1; i >= 0; i-- {
		if e, ok := f.events[i].(Edit); ok {
			return e
		}
	}
	f.t.Fatal("no edit")
	return Edit{}
}

// A part is moved with its handle: its Local changes, the instance root
// does not, and the edit is the part's Parent override.
func TestPartMoveGizmoWritesLocal(t *testing.T) {
	f, px := knightOpen(t, doc(knightLevel))
	f.s.Select("k1/sword")
	o := px(150, 0)
	to := px(180, 0)
	f.gesture(gfx.Vec2{X: o.X + handleLen/2, Y: o.Y}, gfx.Vec2{X: to.X + handleLen/2, Y: o.Y})
	f.update()
	if got := f.parent("k1/sword").Local.Pos; math.Abs(float64(got.X)-80) > 0.01 || got.Y != 0 {
		t.Errorf("Local.Pos = %v, want (80, 0)", got)
	}
	if f.pos("k1").X != 100 || math.Abs(float64(f.pos("k1/sword").X)-180) > 0.01 {
		t.Errorf("root %v, sword %v", f.pos("k1"), f.pos("k1/sword"))
	}
	if f.edits() != 1 {
		t.Fatalf("%d edits, want 1", f.edits())
	}
	e := f.lastEdit()
	if e.ID != "k1" || e.Part != "sword" || e.Component != ParentKey || !e.Override || !strings.Contains(string(e.JSON), `"Pos":{"X":80`) {
		t.Errorf("edit %+v (%s)", e, e.JSON)
	}
	if strings.Contains(string(e.JSON), `"Rot"`) || strings.Contains(string(e.JSON), `"Entity"`) {
		t.Errorf("the override holds more than what changed: %s", e.JSON)
	}
}

// Rotating and scaling a part change only that field of Local.
func TestPartRotateScaleGizmo(t *testing.T) {
	f, px := knightOpen(t, doc(knightLevel))
	f.s.Select("k1/sword")
	o := px(150, 0)
	f.gesture(gfx.Vec2{X: o.X + rotateRadius, Y: o.Y}, gfx.Vec2{X: o.X, Y: o.Y + 30})
	if got := f.parent("k1/sword").Local; math.Abs(float64(got.Rot)-math.Pi/2) > 1e-4 || got.Pos.X != 50 {
		t.Errorf("Local after rotate = %+v", got)
	}
	e := f.lastEdit()
	if e.Part != "sword" || e.Component != ParentKey || !strings.Contains(string(e.JSON), `"Rot":1.57`) {
		t.Errorf("rotate edit %+v (%s)", e, e.JSON)
	}
	f.gesture(scaleBox(o), gfx.Vec2{X: o.X + 2*scaleOffset, Y: o.Y + 2*scaleOffset})
	if got := f.parent("k1/sword").Local.Scale; got != (gfx.Vec2{X: 2, Y: 2}) {
		t.Errorf("Local.Scale = %v, want (2, 2)", got)
	}
}

// Under a rotated and scaled parent the child still ends where the cursor
// left it.
func TestPartMoveUnderRotatedParent(t *testing.T) {
	level := doc(`{"id":"k1","prefab":"knight.prefab.json","components":{"gfx:Transform":{"Pos":{"X":100},"Rot":1.5707964,"Scale":{"X":2,"Y":2}}}}`)
	f, px := knightOpen(t, level)
	f.s.Select("k1/sword")
	start := f.pos("k1/sword")
	o := px(start.X, start.Y)
	to := px(start.X+20, start.Y-10)
	f.gesture(gfx.Vec2{X: o.X + handleLen/2, Y: o.Y}, gfx.Vec2{X: to.X + handleLen/2, Y: to.Y})
	f.update()
	want := start.Add(gfx.Vec2{X: 20}) // the X arrow keeps Y
	got := f.pos("k1/sword")
	if math.Abs(float64(got.X-want.X)) > 0.05 || math.Abs(float64(got.Y-want.Y)) > 0.05 {
		t.Errorf("sword at %v, want %v", got, want)
	}
	parent := f.transform("k1")
	if w := hierarchy.Compose(parent, f.parent("k1/sword").Local); math.Abs(float64(w.Pos.X-got.X)) > 0.01 {
		t.Errorf("Compose(parent, Local) = %v, but the sword is at %v", w.Pos, got)
	}
}

// A selected parent and its selected child move once.
func TestGroupMoveSkipsDescendants(t *testing.T) {
	f, px := knightOpen(t, doc(knightLevel))
	f.s.Select("k1", "k1/sword")
	at := px(100, 0)
	f.gesture(at, gfx.Vec2{X: at.X + 20, Y: at.Y})
	f.update()
	if f.parent("k1/sword").Local.Pos.X != 50 {
		t.Errorf("the child's Local changed: %v", f.parent("k1/sword").Local.Pos)
	}
	if f.edits() != 1 || f.lastEdit().ID != "k1" || f.lastEdit().Component != TransformKey {
		t.Errorf("edits %v", f.events)
	}
}

// A level object attached to another one reports its Parent, not a
// Transform, and no override.
func TestAttachedLevelObject(t *testing.T) {
	f, px := gizmoFixture(t, visual("p", 0, 0, 1), kid("c", 40, "p", 2))
	f.s.Select("c")
	o := px(40, 0)
	to := px(70, 0)
	f.gesture(gfx.Vec2{X: o.X + handleLen/2, Y: o.Y}, gfx.Vec2{X: to.X + handleLen/2, Y: o.Y})
	if got := f.parent("c").Local.Pos.X; math.Abs(float64(got)-70) > 0.01 {
		t.Errorf("Local.Pos.X = %v, want 70", got)
	}
	e := f.lastEdit()
	if e.ID != "c" || e.Component != ParentKey || e.Override || e.Part != "" || !strings.Contains(string(e.JSON), `"Entity":"p"`) {
		t.Errorf("edit %+v (%s)", e, e.JSON)
	}
	// Moving the parent leaves the child's Local alone.
	f.s.Select("p")
	f.gesture(px(0, 0), gfx.Vec2{X: px(0, 0).X + 10, Y: px(0, 0).Y})
	if got := f.parent("c").Local.Pos.X; math.Abs(float64(got)-70) > 0.01 {
		t.Errorf("Local.Pos.X after moving the parent = %v", got)
	}
}

// In the prefab stage a multi-object prefab file is a plain document: the
// attached object's handle edits its Local and the edit is not an override.
func TestPrefabStageEditsLocal(t *testing.T) {
	f := newFixture(t, Config{Stage: &Stage{Grid: 16}})
	f.open(doc(visual("knight", 0, 0, 1), kid("sword", 50, "knight", 2)))
	f.settle()
	f.update()
	px := func(x, y float32) gfx.Vec2 {
		return f.e.Camera().WorldToScreen(gfx.Vec2{X: x, Y: y}, image.Pt(screenW, screenH))
	}
	f.s.Select("sword")
	o, to := px(50, 0), px(70, 0)
	f.gesture(gfx.Vec2{X: o.X + handleLen/2, Y: o.Y}, gfx.Vec2{X: to.X + handleLen/2, Y: o.Y})
	e := f.lastEdit()
	if e.ID != "sword" || e.Component != ParentKey || e.Override || !strings.Contains(string(e.JSON), `"Pos":{"X":70`) {
		t.Errorf("edit %+v (%s)", e, e.JSON)
	}
}

// absKid is kid with PlaceAbsolute.
func absKid(id string, x float32, parent string, layer int) string {
	return strings.Replace(kid(id, x, parent, layer), `"PlaceMode":0`, `"PlaceMode":1`, 1)
}

// In edit mode an absolute child moves with a dragged parent and gets no
// edit; dragging it reports its Local; an edit of its Parent places it anew.
func TestAbsoluteChildInEditMode(t *testing.T) {
	f, px := gizmoFixture(t, visual("p", 0, 0, 1), absKid("c", 40, "p", 2))
	if got := f.pos("c").X; got != 40 {
		t.Fatalf("absolute child starts at %v, want 40 (from Local)", got)
	}
	f.s.Select("p")
	f.gesture(px(0, 0), px(10, 0))
	f.update()
	if got := f.pos("c").X; math.Abs(float64(got)-50) > 0.01 {
		t.Errorf("absolute child at %v after the parent moved by 10, want 50", got)
	}
	if f.edits() != 1 || f.lastEdit().ID != "p" {
		t.Errorf("edits %v, want one for p", f.events)
	}
	if got := f.parent("c").Local.Pos.X; got != 40 {
		t.Errorf("child's Local.Pos.X = %v, want 40", got)
	}

	f.events = nil
	f.s.Select("c")
	o, to := px(50, 0), px(80, 0)
	f.gesture(gfx.Vec2{X: o.X + handleLen/2, Y: o.Y}, gfx.Vec2{X: to.X + handleLen/2, Y: o.Y})
	f.update()
	e := f.lastEdit()
	if e.ID != "c" || e.Component != ParentKey || !strings.Contains(string(e.JSON), `"Pos":{"X":70`) {
		t.Errorf("edit %+v (%s), want c's Parent with Local.Pos.X 70", e, e.JSON)
	}
	if got := f.pos("c").X; math.Abs(float64(got)-80) > 0.01 {
		t.Errorf("dragged absolute child at %v, want 80", got)
	}

	// An inspector edit of its Parent re-decodes the component and places it
	// from the new Local.
	f.apply(doc(visual("p", 10, 0, 1), absKid("c", 5, "p", 2)))
	f.settle()
	f.update()
	if got := f.pos("c").X; math.Abs(float64(got)-15) > 0.01 {
		t.Errorf("after an edit of Local the child is at %v, want 15", got)
	}
}
