package bridge

import (
	"encoding/json"
	"image"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/weakpixel/ecs"
	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/input"
)

const boxPrefab = `{"objects":[{"id":"box","group":"gfx","class":"Visual","components":{` +
	`"gfx:Transform":{"Pos":{"X":0,"Y":0},"PrevPos":{"X":0,"Y":0},"Rot":0,"Scale":{"X":1,"Y":1}},` +
	`"gfx:Sprite":{"ID":"box.png","Layer":1,"Origin":{"X":0,"Y":0},"FlipX":false,"FlipY":false,"Tint":0,"Blend":0,"Hidden":false}}}]}`

// until runs ticks until cond holds. Prefabs decode on worker goroutines,
// so it waits by wall-clock time, not a number of ticks.
func (f *fixture) until(cond func() bool) {
	f.t.Helper()
	for deadline := time.Now().Add(5 * time.Second); time.Now().Before(deadline); time.Sleep(time.Millisecond) {
		if cond() {
			return
		}
		f.recv(`{"type":"frame"}`)
	}
	f.t.Fatal("condition not reached")
}

// TestPrefabInstances: a document with instances is answered once the
// prefab loaded; drags send overrides; drops and "create" make instances;
// a saved prefab re-applies the level.
func TestPrefabInstances(t *testing.T) {
	files := fstest.MapFS{"box.prefab.json": {Data: []byte(boxPrefab)}}
	f := newFixtureFiles(t, files)
	inst := `{"id":"p1","prefab":"box.prefab.json","components":{"gfx:Transform":{"Pos":{"X":5}}}}`
	f.recv(documentMsg(inst))
	var applied []map[string]any
	f.until(func() bool { applied = append(applied, f.host.take("applied")...); return len(applied) > 0 })
	if len(applied) != 1 || applied[0]["error"] != "" {
		t.Fatalf("applied = %v", applied)
	}
	objs := f.host.take("objects")
	last := objs[len(objs)-1]["objects"].([]any)
	if o := last[0].(map[string]any); len(last) != 1 || o["prefab"] != "box.prefab.json" || o["class"] != "Visual" {
		t.Fatalf("objects = %v", last)
	}
	e, _ := f.b.Entity("p1")
	w := f.e.World()
	if tr := ecs.GetComp[gfx.Transform](w, e); tr.Pos != (gfx.Vec2{X: 5}) {
		t.Fatalf("p1 Pos = %v", tr.Pos)
	}

	// A drag reports the Transform override, not the whole component.
	f.until(func() bool { _, ok := f.b.Bounds("p1"); return ok })
	p := f.e.Camera().WorldToScreen(gfx.Vec2{X: 6, Y: 1}, image.Pt(320, 180))
	f.src.Frame.Mouse = input.MouseFrame{X: p.X, Y: p.Y}
	f.src.Frame.Mouse.SetDown(input.MouseLeft, true)
	f.recv(`{"type":"frame"}`)
	f.src.Frame.Mouse.X += 10
	f.recv(`{"type":"frame"}`)
	f.src.Frame.Mouse.SetDown(input.MouseLeft, false)
	f.recv(`{"type":"frame"}`)
	edits := f.host.take("edit")
	if len(edits) != 1 || edits[0]["override"] != true {
		t.Fatalf("edits = %v", edits)
	}
	if got, _ := json.Marshal(edits[0]["json"]); string(got) != `{"Pos":{"X":15},"PrevPos":{"X":15}}` {
		t.Fatalf("edit json = %s", got)
	}

	// Dropping the prefab file makes an instance at the drop point.
	f.recv(`{"type":"drop","path":"box.prefab.json","x":160,"y":90}`)
	created := f.host.take("create")
	if len(created) != 1 {
		t.Fatalf("create = %v (errors %v)", created, f.host.take("error"))
	}
	at := f.e.Camera().ScreenToWorld(gfx.Vec2{X: 160, Y: 90}, image.Pt(320, 180))
	obj := created[0]["object"].(map[string]any)
	if obj["id"] != "box-1" || obj["prefab"] != "box.prefab.json" || obj["group"] != nil {
		t.Fatalf("created %v", obj)
	}
	// Only the coordinates that differ from the prefab's (0) are written.
	tr, _ := obj["components"].(map[string]any)["gfx:Transform"].(map[string]any)
	for _, field := range []string{"Pos", "PrevPos"} {
		v, _ := tr[field].(map[string]any)
		x, _ := v["X"].(float64)
		y, _ := v["Y"].(float64)
		if (gfx.Vec2{X: float32(x), Y: float32(y)}) != at {
			t.Fatalf("created %s %v, want %v", field, v, at)
		}
	}
	f.recv(`{"type":"create","prefab":"box.prefab.json"}`)
	if created := f.host.take("create"); len(created) != 1 || created[0]["object"].(map[string]any)["id"] != "box-1" {
		t.Fatalf("create by prefab = %v (errors %v)", created, f.host.take("error"))
	}

	// Saving the prefab re-applies the level: p1 follows, unasked.
	files["box.prefab.json"] = &fstest.MapFile{Data: []byte(strings.Replace(boxPrefab, `"Layer":1`, `"Layer":6`, 1))}
	f.recv(`{"type":"assetsChanged","paths":["box.prefab.json"]}`)
	f.until(func() bool { return ecs.GetComp[gfx.Sprite](w, e).Layer == 6 })
	if applied := f.host.take("applied"); len(applied) != 1 || applied[0]["error"] != "" {
		t.Fatalf("applied after the prefab changed = %v", applied)
	}
	// The document is the truth: this host never wrote the drag back.
	if tr := ecs.GetComp[gfx.Transform](w, e); tr.Pos != (gfx.Vec2{X: 5}) {
		t.Fatalf("p1 Pos after the prefab changed = %v, want the document's", tr.Pos)
	}

	// A broken instance is reported once the prefab is known.
	f.recv(documentMsg(`{"id":"p1","prefab":"box.prefab.json","components":{"gfx:Sprite":{"Layer":"x"}}}`))
	if applied := f.host.take("applied"); len(applied) != 1 || !strings.Contains(applied[0]["error"].(string), "objects[0] (p1): gfx:Sprite: $.Layer: expected integer") {
		t.Fatalf("applied = %v", applied)
	}
	f.recv(documentMsg(`{"id":"p1","prefab":"missing.prefab.json"}`))
	f.until(func() bool { applied = f.host.take("applied"); return len(applied) > 0 })
	if !strings.Contains(applied[0]["error"].(string), `objects[0] (p1): prefab "missing.prefab.json": `) {
		t.Fatalf("applied = %v", applied)
	}
}
