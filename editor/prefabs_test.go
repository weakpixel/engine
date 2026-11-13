package editor

import (
	"fmt"
	"image"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/weakpixel/ecs"
	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/god"
	"github.com/weakpixel/engine/hierarchy"
	"github.com/weakpixel/engine/input"
)

// prefabFixture is a fixture whose files have box.prefab.json (a gfx:Visual
// at 0,0 on layer 1).
func prefabFixture(t *testing.T) *fixture {
	t.Helper()
	files := testFiles(t)
	files["box.prefab.json"] = &fstest.MapFile{Data: []byte(doc(visual("box", 0, 0, 1)))}
	return newFixtureFS(t, Config{}, files)
}

// waitApplied ticks until the session reports an Applied event and returns
// its error.
func (f *fixture) waitApplied() error {
	f.t.Helper()
	// Prefabs decode on worker goroutines: wait by wall-clock time.
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(time.Millisecond) {
		for _, ev := range f.events {
			if a, ok := ev.(Applied); ok {
				f.events = nil
				return a.Err
			}
		}
		f.update()
	}
	f.t.Fatal("no Applied event")
	return nil
}

func TestPrefabInstances(t *testing.T) {
	f := prefabFixture(t)
	f.open(doc(`{"id":"a","prefab":"box.prefab.json","components":{"gfx:Transform":{"Pos":{"X":20}}}}`, visual("plain", 0, 0, 0)))
	if !f.s.Pending() || len(f.s.IDs()) != 0 {
		t.Fatalf("Open with a loading prefab: pending %v, ids %v", f.s.Pending(), f.s.IDs())
	}
	if err := f.waitApplied(); err != nil {
		t.Fatal(err)
	}
	w := f.e.World()
	a := f.entity("a")
	if f.pos("a") != (gfx.Vec2{X: 20}) || ecs.GetComp[gfx.Sprite](w, a).Layer != 1 {
		t.Fatalf("a: pos %v layer %d", f.pos("a"), ecs.GetComp[gfx.Sprite](w, a).Layer)
	}
	if key, _ := god.PrefabOf(w, a); key != "box.prefab.json" {
		t.Fatalf("PrefabOf(a) = %q", key)
	}
	if objs := f.s.Level().Objects(); objs[0].Prefab != "box.prefab.json" || objs[1].Prefab != "" {
		t.Fatalf("level objects %+v", objs)
	}

	// The prefab is loaded now: Apply works at once, by merged component.
	f.apply(doc(`{"id":"a","prefab":"box.prefab.json","components":{"gfx:Transform":{"Pos":{"X":20}},"gfx:Sprite":{"Layer":3}}}`, visual("plain", 0, 0, 0)))
	if f.s.Pending() || f.entity("a") != a || ecs.GetComp[gfx.Sprite](w, a).Layer != 3 {
		t.Fatalf("apply: pending %v, same entity %v", f.s.Pending(), f.entity("a") == a)
	}

	// An object that becomes an instance (or another prefab's) is created
	// again, so its PrefabInstance tag is right.
	plain := f.entity("plain")
	f.apply(doc(`{"id":"a","prefab":"box.prefab.json"}`, `{"id":"plain","prefab":"box.prefab.json"}`))
	if f.entity("plain") == plain || w.IsAlive(plain) {
		t.Fatal("plain was not recreated as an instance")
	}
	if key, _ := god.PrefabOf(w, f.entity("plain")); key != "box.prefab.json" {
		t.Fatalf("PrefabOf(plain) = %q", key)
	}

	// Problems of instances are reported like others.
	for text, want := range map[string]string{
		doc(`{"id":"a","prefab":"box.prefab.json","group":"gfx","class":"AnimatedVisual"}`): `objects[0] (a): object gfx:AnimatedVisual does not match prefab "box.prefab.json", which is gfx:Visual`,
		doc(`{"id":"a","prefab":"box.prefab.json","components":{"Nope":{}}}`):               `objects[0] (a): object gfx:Visual: component Nope is not registered`,
		doc(`{"id":"a","prefab":"box.prefab.json","components":{"gfx:Transform":null}}`):    `objects[0] (a): object gfx:Visual: missing required component gfx:Transform`,
		doc(`{"id":"a","prefab":"bad key#"}`):                                               `objects[0] (a): prefab "bad key#": `,
	} {
		if err := f.s.Apply([]byte(text)); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: got %v, want %q", text, err, want)
		}
	}
}

// TestPrefabPlayStop: Stop restores instances from the document.
func TestPrefabPlayStop(t *testing.T) {
	f := prefabFixture(t)
	f.open(doc(`{"id":"a","prefab":"box.prefab.json","components":{"gfx:Transform":{"Pos":{"X":20}}}}`))
	if err := f.waitApplied(); err != nil {
		t.Fatal(err)
	}
	f.s.Play()
	w := f.e.World()
	ecs.GetComp[gfx.Transform](w, f.entity("a")).Pos.X = 99
	w.DeleteEntity(f.entity("a"))
	f.s.Stop()
	if f.pos("a") != (gfx.Vec2{X: 20}) {
		t.Fatalf("after Stop: pos %v", f.pos("a"))
	}
	if key, _ := god.PrefabOf(w, f.entity("a")); key != "box.prefab.json" {
		t.Fatalf("PrefabOf after Stop = %q", key)
	}
}

// kid is a visual attached to parent at the local offset (x, 0).
func kid(id string, x float32, parent string, layer int) string {
	return strings.Replace(visual(id, 0, 0, layer), `"Hidden":false}}}`,
		fmt.Sprintf(`"Hidden":false},"hierarchy:Parent":{"Entity":%q,"Local":{"Pos":{"X":%g,"Y":0},"Rot":0,"Scale":{"X":1,"Y":1},"LayerOffset":0,"LayerMode":1,"YSortSide":0}}}}`, parent, x), 1)
}

// knightFixture has knight.prefab.json: a root and a sword 50 units to its
// right, attached to it.
func knightFixture(t *testing.T) *fixture {
	t.Helper()
	files := testFiles(t)
	files["knight.prefab.json"] = &fstest.MapFile{Data: []byte(doc(visual("knight", 0, 0, 1), kid("sword", 50, "knight", 2)))}
	return newFixtureFS(t, Config{}, files)
}

const knightLevel = `{"id":"k1","prefab":"knight.prefab.json","components":{"gfx:Transform":{"Pos":{"X":100}}},"children":{"sword":{"gfx:Sprite":{"Layer":4}}}}`

func TestMultiObjectInstance(t *testing.T) {
	f := knightFixture(t)
	f.open(doc(knightLevel, `{"id":"k2","prefab":"knight.prefab.json","children":{"sword":null}}`))
	if err := f.waitApplied(); err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(f.s.IDs(), ","); got != "k1,k1/sword,k2" {
		t.Fatalf("ids = %s", got)
	}
	w := f.e.World()
	objs := f.s.Level().Objects()
	if objs[0].Part != "" || objs[1].Part != "sword" || objs[1].Prefab != "knight.prefab.json" {
		t.Fatalf("objects %+v", objs)
	}
	if key, part, ok := god.PrefabPartOf(w, f.entity("k1/sword")); !ok || key != "knight.prefab.json" || part != "sword" {
		t.Fatalf("PrefabPartOf = %q %q %v", key, part, ok)
	}
	if ecs.GetComp[gfx.Sprite](w, f.entity("k1/sword")).Layer != 4 {
		t.Error("the children override was not applied")
	}
	if ecs.GetComp[hierarchy.Parent](w, f.entity("k1/sword")).Entity != f.entity("k1") {
		t.Error("the part is not attached to its instance's root")
	}

	// The same document again changes nothing; a removed part goes away.
	sword := f.entity("k1/sword")
	f.apply(doc(knightLevel, `{"id":"k2","prefab":"knight.prefab.json","children":{"sword":null}}`))
	if f.entity("k1/sword") != sword {
		t.Error("an unchanged part was recreated")
	}
	f.apply(doc(`{"id":"k1","prefab":"knight.prefab.json","children":{"sword":null}}`))
	if w.IsAlive(sword) || len(f.s.IDs()) != 1 {
		t.Errorf("the removed part is alive or listed: %v", f.s.IDs())
	}

	// Problems are reported with the entry.
	for text, want := range map[string]string{
		doc(`{"id":"k1","prefab":"knight.prefab.json","children":{"axe":{}}}`): `objects[0] (k1): children: prefab "knight.prefab.json" has no object "axe"`,
		doc(`{"id":"a/b","group":"gfx","class":"Visual"}`):                     `objects[0]: id "a/b" contains "/"`,
		doc(`{"id":"k1","group":"gfx","class":"Visual","children":{}}`):        `objects[0]: "children" needs a "prefab"`,
	} {
		if err := f.s.Apply([]byte(text)); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: got %v, want %q", text, err, want)
		}
	}
}

// TestFrameCountsAttachedObjectsPlaced: an opened level is framed on its
// attached objects where the hierarchy stage places them, also when the
// frame comes before that stage ran for them (a document applied this
// tick, or a level opened between ticks, as a shell's open message does).
func TestFrameCountsAttachedObjectsPlaced(t *testing.T) {
	tests := []struct {
		name string
		f    func(t *testing.T) *fixture
		doc  string
	}{
		{"prefab parts", knightFixture, doc(knightLevel)},
		{"attached object", func(t *testing.T) *fixture { return newFixture(t, Config{}) }, doc(visual("p", 100, 0, 0), kid("c", 50, "p", 1))},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := tt.f(t)
			f.open(tt.doc)
			if f.s.Pending() {
				if err := f.waitApplied(); err != nil {
					t.Fatal(err)
				}
			}
			f.update()
			got := f.e.Camera().Pos
			f.s.Frame()
			if want := f.e.Camera().Pos; got != want {
				t.Errorf("the level was framed at %v, its placed objects at %v", got, want)
			}
		})
	}
}

// TestMultiObjectClickSelectsPart: a click on a part selects the part, Alt
// selects its instance root, and dragging the root moves the group.
func TestMultiObjectClickSelectsPart(t *testing.T) {
	f := knightFixture(t)
	f.open(doc(knightLevel))
	if err := f.waitApplied(); err != nil {
		t.Fatal(err)
	}
	f.settle()
	f.update()
	if f.pos("k1/sword") != (gfx.Vec2{X: 150}) {
		t.Fatalf("sword at %v, want 150,0", f.pos("k1/sword"))
	}
	at := f.e.Camera().WorldToScreen(gfx.Vec2{X: 150}, image.Pt(screenW, screenH))
	f.gesture(at)
	if got := f.s.Selected(); len(got) != 1 || got[0] != "k1/sword" {
		t.Fatalf("selection after a click on the sword = %v, want [k1/sword]", got)
	}

	f.s.Select()
	f.keys(input.KeyAltLeft)
	f.gesture(at)
	if got := f.s.Selected(); len(got) != 1 || got[0] != "k1" {
		t.Fatalf("selection after an Alt+click on the sword = %v, want [k1]", got)
	}

	// Dragging the root moves the group; the part keeps its Local.
	f.keys()
	f.s.Select()
	f.keys(input.KeyAltLeft)
	f.gesture(at, gfx.Vec2{X: at.X + 30, Y: at.Y})
	f.keys()
	f.update()
	if f.pos("k1").X <= 100 || f.pos("k1/sword").X-f.pos("k1").X != 50 {
		t.Errorf("after a drag: root %v, sword %v", f.pos("k1"), f.pos("k1/sword"))
	}
}

// TestMultiObjectEdit: an edit of a part is reported with its root's id and
// the part, as the override for "children.<part>".
func TestMultiObjectEdit(t *testing.T) {
	f := knightFixture(t)
	f.open(doc(knightLevel))
	if err := f.waitApplied(); err != nil {
		t.Fatal(err)
	}
	f.settle()
	f.update()
	f.events = nil
	f.s.emitTransform("k1/sword")
	f.s.emitTransform("k1")
	var edits []Edit
	for _, ev := range f.events {
		if e, ok := ev.(Edit); ok {
			edits = append(edits, e)
		}
	}
	if len(edits) != 2 {
		t.Fatalf("%d edits", len(edits))
	}
	// The sword's Transform is derived (150,0 against the prefab's 0,0):
	// the override holds what differs from the prefab part.
	if e := edits[0]; e.ID != "k1" || e.Part != "sword" || !e.Override || !strings.Contains(string(e.JSON), `"X":150`) {
		t.Errorf("part edit %+v (%s)", e, e.JSON)
	}
	if e := edits[1]; e.ID != "k1" || e.Part != "" || !e.Override || !strings.Contains(string(e.JSON), `"X":100`) {
		t.Errorf("root edit %+v (%s)", e, e.JSON)
	}
}
