package bridge

import (
	"encoding/json"
	"image"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/god"
	"github.com/weakpixel/engine/input"
)

// bridgeGOD names the component bridge:Tint and the class bridge:Mob, which
// no package registers: the prefab of a game's own component, opened
// without the game's code.
const bridgeGOD = `{"version":0,"components":[{"group":"bridge","name":"Tint","schema":{"type":"object",
"properties":{"Icon":{"type":"string","format":"asset","x-asset":"sprite"},"Amount":{"type":"integer"}}}}],
"objects":[{"group":"bridge","class":"Mob","components":[
{"component":"gfx:Transform"},{"component":"gfx:Sprite"},{"component":"bridge:Tint","optional":true}]}]}`

const mobPrefab = `{"objects":[{"id":"mob","group":"bridge","class":"Mob","components":{` +
	`"gfx:Transform":{"Pos":{"X":0,"Y":0},"PrevPos":{"X":0,"Y":0},"Rot":0,"Scale":{"X":1,"Y":1}},` +
	`"gfx:Sprite":{"ID":"box.png","Layer":1,"Origin":{"X":0,"Y":0},"FlipX":false,"FlipY":false,"Tint":0,"Blend":0,"Hidden":false},` +
	`"bridge:Tint":{"Icon":"box.png","Amount":3}}}]}`

// TestDataOnlyComponents: a prefab and an instance of it holding a
// data-only component apply without error, the Transform drag of the
// instance reports an override, and "assetField" is answered from the GOD
// schema of the data-only component.
func TestDataOnlyComponents(t *testing.T) {
	if err := god.RegisterDataOnly(strings.NewReader(bridgeGOD)); err != nil {
		t.Fatal(err)
	}
	files := fstest.MapFS{"mob.prefab.json": {Data: []byte(mobPrefab)}}
	f := newFixtureFiles(t, files)
	inst := `{"id":"m1","prefab":"mob.prefab.json","components":{"gfx:Transform":{"Pos":{"X":5}},"bridge:Tint":{"Amount":4}}}`
	f.recv(documentMsg(inst))
	var applied []map[string]any
	f.until(func() bool { applied = append(applied, f.host.take("applied")...); return len(applied) > 0 })
	if applied[0]["error"] != "" {
		t.Fatalf("applied = %v", applied)
	}

	f.until(func() bool { _, ok := f.b.Bounds("m1"); return ok })
	p := f.e.Camera().WorldToScreen(gfx.Vec2{X: 6, Y: 1}, image.Pt(320, 180))
	f.src.Frame.Mouse = input.MouseFrame{X: p.X, Y: p.Y}
	f.src.Frame.Mouse.SetDown(input.MouseLeft, true)
	f.recv(`{"type":"frame"}`)
	f.src.Frame.Mouse.X += 10
	f.recv(`{"type":"frame"}`)
	f.src.Frame.Mouse.SetDown(input.MouseLeft, false)
	f.recv(`{"type":"frame"}`)
	edits := f.host.take("edit")
	if len(edits) != 1 || edits[0]["override"] != true || edits[0]["component"] != "gfx:Transform" {
		t.Fatalf("edits = %v", edits)
	}
	if got, _ := json.Marshal(edits[0]["json"]); string(got) != `{"Pos":{"X":15},"PrevPos":{"X":15}}` {
		t.Fatalf("edit json = %s", got)
	}

	f.recv(`{"type":"assetField","id":"m1","field":["bridge:Tint","Icon"],"path":"box.png"}`)
	if got := f.host.take("field"); len(got) != 1 || got[0]["value"] != "box.png" {
		t.Fatalf("field = %v (errors %v)", got, f.host.take("error"))
	}
	for msg, want := range map[string]string{
		`{"type":"assetField","id":"m1","field":["bridge:Tint","Amount"],"path":"box.png"}`: "not an asset field",
		`{"type":"assetField","id":"m1","field":["bridge:Tint","Nope"],"path":"box.png"}`:   "bridge:Tint has no field Nope",
		`{"type":"assetField","id":"m1","field":["bridge:Typo","Icon"],"path":"box.png"}`:   "bridge:Typo is not a registered component",
	} {
		f.recv(msg)
		if errs := f.host.take("error"); len(errs) != 1 || !strings.Contains(errs[0]["message"].(string), want) {
			t.Errorf("%s: errors %v, want %q", msg, errs, want)
		}
	}
}
