package bridge

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/color"
	"image/png"
	"os"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/weakpixel/ecs"
	"github.com/weakpixel/engine/editor"
	"github.com/weakpixel/engine/game"
	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/input"
)

// fakeHost is the Transport of the tests: it records what the game sent.
type fakeHost struct{ sent []map[string]any }

func (h *fakeHost) Send(msg []byte) {
	var m map[string]any
	if err := json.Unmarshal(msg, &m); err != nil {
		panic(err)
	}
	h.sent = append(h.sent, m)
}

// take returns and forgets the messages of type typ.
func (h *fakeHost) take(typ string) []map[string]any {
	var out, rest []map[string]any
	for _, m := range h.sent {
		if m["type"] == typ {
			out = append(out, m)
		} else {
			rest = append(rest, m)
		}
	}
	h.sent = rest
	return out
}

type fixture struct {
	t    *testing.T
	e    *game.Engine
	b    *Bridge
	host *fakeHost
	src  *input.ManualSource
}

func newFixture(t *testing.T) *fixture { return newFixtureFiles(t, fstest.MapFS{}) }

// newFixtureFiles is newFixture with files added to the game's FS (which
// has box.png).
func newFixtureFiles(t *testing.T, files fstest.MapFS) *fixture {
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 16, 8))); err != nil {
		t.Fatal(err)
	}
	files["box.png"] = &fstest.MapFile{Data: buf.Bytes()}
	f := &fixture{t: t, host: &fakeHost{}, src: &input.ManualSource{}}
	f.b = New(editor.Config{Path: "test.level.json"}, f.host)
	f.e = game.New(game.Config{
		NoAudio: true, Width: 320, Height: 180, Tool: f.b,
		FS:    files,
		Input: game.InputConfig{Source: f.src},
	})
	t.Cleanup(f.e.Close)
	return f
}

// recv queues a message from the host and runs one tick to handle it.
func (f *fixture) recv(msg string) {
	f.t.Helper()
	f.b.Receive([]byte(msg))
	if err := f.e.Update(); err != nil {
		f.t.Fatal(err)
	}
}

func visual(id string, x, y float32) string {
	return fmt.Sprintf(`{"id":%q,"group":"gfx","class":"Visual","components":{`+
		`"gfx:Transform":{"Pos":{"X":%g,"Y":%g},"PrevPos":{"X":%g,"Y":%g},"Rot":0,"Scale":{"X":1,"Y":1}},`+
		`"gfx:Sprite":{"ID":"box.png","Layer":0,"Origin":{"X":0,"Y":0},"FlipX":false,"FlipY":false,"Tint":0,"Blend":0,"Hidden":false}}}`,
		id, x, y, x, y)
}

// documentMsg is a "document" message holding objs.
func documentMsg(objs ...string) string {
	text := `{"objects":[` + strings.Join(objs, ",") + `]}`
	msg, _ := json.Marshal(map[string]string{"type": "document", "text": text})
	return string(msg)
}

func TestReadyAndDocument(t *testing.T) {
	f := newFixture(t)
	f.recv(documentMsg(visual("a", 1, 2)))

	ready := f.host.take("ready")
	if len(ready) != 1 || ready[0]["version"] != float64(Version) {
		t.Fatalf("ready = %v", ready)
	}
	var keys []string
	for _, s := range ready[0]["schemas"].([]any) {
		s := s.(map[string]any)
		keys = append(keys, fmt.Sprint(s["group"], ":", s["class"]))
	}
	if !slices.Contains(keys, "gfx:Visual") {
		t.Errorf("schemas %v lack gfx:Visual", keys)
	}
	var comps []string
	for _, c := range ready[0]["components"].([]any) {
		c := c.(map[string]any)
		comps = append(comps, fmt.Sprint(c["group"], ":", c["name"]))
	}
	if !slices.Contains(comps, "gfx:Transform") || !slices.IsSorted(comps) {
		t.Errorf("components %v lack gfx:Transform or are not sorted", comps)
	}
	if ready[0]["assetKinds"].(map[string]any)[".png"] != "sprite" {
		t.Errorf("assetKinds %v", ready[0]["assetKinds"])
	}

	if applied := f.host.take("applied"); len(applied) != 1 || applied[0]["error"] != "" {
		t.Fatalf("applied = %v", applied)
	}
	objs := f.host.take("objects")
	if len(objs) == 0 {
		t.Fatal("no objects message")
	}
	last := objs[len(objs)-1]["objects"].([]any)
	if len(last) != 1 || last[0].(map[string]any)["id"] != "a" {
		t.Fatalf("objects %v", last)
	}

	// An invalid version is reported and changes nothing; the next valid
	// one applies.
	f.recv(documentMsg(`{"id":"a","group":"gfx","class":"Nope","components":{}}`))
	if applied := f.host.take("applied"); len(applied) != 1 || !strings.Contains(applied[0]["error"].(string), "unknown object gfx:Nope") {
		t.Fatalf("applied = %v", applied)
	}
	f.recv(documentMsg(visual("a", 5, 6)))
	en, _ := f.b.Entity("a")
	if pos := ecs.GetComp[gfx.Transform](f.e.World(), en).Pos; pos != (gfx.Vec2{X: 5, Y: 6}) {
		t.Errorf("a at %v", pos)
	}
}

// TestOpenSwitchesFile: "open" after "document" replaces the level (new
// path, new objects, Play stopped, selection cleared) and is answered like
// a document; later documents apply to the new level.
func TestOpenSwitchesFile(t *testing.T) {
	f := newFixture(t)
	f.recv(documentMsg(visual("a", 0, 0)))
	f.recv(`{"type":"select","ids":["a"]}`)
	f.recv(`{"type":"play"}`)
	f.host.sent = nil

	text := `{"objects":[` + visual("b", 10, 0) + `]}`
	msg, _ := json.Marshal(map[string]string{"type": "open", "path": "b.level.json", "text": text})
	f.recv(string(msg))
	if applied := f.host.take("applied"); len(applied) != 1 || applied[0]["error"] != "" {
		t.Fatalf("applied = %v", applied)
	}
	objs := f.host.take("objects")
	if len(objs) != 1 {
		t.Fatalf("objects messages %v", objs)
	}
	if list := objs[0]["objects"].([]any); len(list) != 1 || list[0].(map[string]any)["id"] != "b" {
		t.Errorf("objects %v", list)
	}
	if modes := f.host.take("mode"); len(modes) != 1 || modes[0]["play"] != false {
		t.Errorf("modes %v", modes)
	}
	if sel := f.host.take("selection"); len(sel) != 1 || len(sel[0]["ids"].([]any)) != 0 {
		t.Errorf("selection %v", sel)
	}
	if f.b.Playing() || f.b.Level().Path() != "b.level.json" {
		t.Errorf("playing %v, path %q", f.b.Playing(), f.b.Level().Path())
	}

	f.recv(documentMsg(visual("b", 20, 0)))
	en, _ := f.b.Entity("b")
	if pos := ecs.GetComp[gfx.Transform](f.e.World(), en).Pos; pos != (gfx.Vec2{X: 20}) {
		t.Errorf("b at %v after a document", pos)
	}
	f.host.sent = nil

	f.recv(`{"type":"open","path":"c.level.json","text":"{"}`)
	if applied := f.host.take("applied"); len(applied) != 1 || applied[0]["error"] == "" {
		t.Errorf("a bad open was not reported: %v", applied)
	}
}

func TestModesSelectionAndErrors(t *testing.T) {
	f := newFixture(t)
	f.recv(documentMsg(visual("a", 0, 0), visual("b", 40, 0)))
	f.recv(`{"type":"select","ids":["b"]}`)
	if got := f.b.Selected(); !slices.Equal(got, []string{"b"}) {
		t.Errorf("selected %v", got)
	}
	f.recv(`{"type":"play"}`)
	if !f.b.Playing() {
		t.Fatal("not playing after play")
	}
	f.recv(`{"type":"stop"}`)
	modes := f.host.take("mode")
	if len(modes) != 2 || modes[0]["play"] != true || modes[1]["play"] != false {
		t.Errorf("modes %v", modes)
	}

	// Removing b from the document drops it from the selection.
	f.host.take("selection")
	f.recv(documentMsg(visual("a", 0, 0)))
	sel := f.host.take("selection")
	if len(sel) != 1 || len(sel[0]["ids"].([]any)) != 0 {
		t.Errorf("selection %v", sel)
	}

	f.recv(`{"type":"teleport"}`)
	f.recv(`not json`)
	errs := f.host.take("error")
	if len(errs) != 2 || !strings.Contains(errs[0]["message"].(string), `unknown message type "teleport"`) {
		t.Errorf("errors %v", errs)
	}
}

func TestDragSendsEdit(t *testing.T) {
	f := newFixture(t)
	f.recv(documentMsg(visual("a", 0, 0)))
	for range 5 { // load box.png
		f.recv(`{"type":"frame"}`)
	}
	p := f.e.Camera().WorldToScreen(gfx.Vec2{}, image.Pt(320, 180))
	f.src.Frame.Mouse = input.MouseFrame{X: p.X, Y: p.Y}
	f.src.Frame.Mouse.SetDown(input.MouseLeft, true)
	f.recv(`{"type":"frame"}`)
	f.src.Frame.Mouse.X += 10
	f.recv(`{"type":"frame"}`)
	f.src.Frame.Mouse.SetDown(input.MouseLeft, false)
	f.recv(`{"type":"frame"}`)

	edits := f.host.take("edit")
	if len(edits) != 1 || edits[0]["id"] != "a" || edits[0]["component"] != "gfx:Transform" {
		t.Fatalf("edits %v", edits)
	}
	pos := edits[0]["json"].(map[string]any)["Pos"].(map[string]any)
	if pos["X"] != float64(10) {
		t.Errorf("edit Pos %v", pos)
	}
}

func TestDrop(t *testing.T) {
	f := newFixture(t)
	f.recv(documentMsg(visual("visual-1", 0, 0)))

	// A .png fits every object type with a required sprite field: the host
	// is asked to choose.
	f.recv(`{"type":"drop","path":"box.png","x":160,"y":90}`)
	choose := f.host.take("choose")
	if len(choose) != 1 {
		t.Fatalf("choose = %v (errors %v)", choose, f.host.take("error"))
	}
	var opts []string
	for _, o := range choose[0]["options"].([]any) {
		opts = append(opts, o.(string))
	}
	if !slices.Contains(opts, "gfx:Visual") || !slices.Contains(opts, "gfx:AnimatedVisual") {
		t.Fatalf("options %v", opts)
	}

	f.recv(`{"type":"drop","path":"box.png","x":160,"y":90,"class":"gfx:Visual"}`)
	created := f.host.take("create")
	if len(created) != 1 {
		t.Fatalf("create = %v (errors %v)", created, f.host.take("error"))
	}
	obj := created[0]["object"].(map[string]any)
	comps := obj["components"].(map[string]any)
	if obj["id"] != "visual-2" || obj["class"] != "Visual" || comps["gfx:Sprite"].(map[string]any)["ID"] != "box.png" {
		t.Fatalf("created %v", obj)
	}
	// The drop point (window = logical before the first draw) is the world
	// point the camera shows there.
	want := f.e.Camera().ScreenToWorld(gfx.Vec2{X: 160, Y: 90}, image.Pt(320, 180))
	pos := comps["gfx:Transform"].(map[string]any)["Pos"].(map[string]any)
	if pos["X"] != float64(want.X) || pos["Y"] != float64(want.Y) {
		t.Errorf("created at %v, want %v", pos, want)
	}

	// The host appends it to the document; it then exists.
	raw, _ := json.Marshal(obj)
	f.recv(documentMsg(visual("visual-1", 0, 0), string(raw)))
	if applied := f.host.take("applied"); applied[len(applied)-1]["error"] != "" {
		t.Fatalf("applying the created object: %v", applied)
	}
	if _, ok := f.b.Entity("visual-2"); !ok {
		t.Fatal("the created object does not exist")
	}

	f.recv(`{"type":"drop","path":"notes.txt","x":0,"y":0}`)
	if errs := f.host.take("error"); len(errs) != 1 || !strings.Contains(errs[0]["message"].(string), "no object type takes this kind of file") {
		t.Errorf("errors %v", errs)
	}
}

func TestKindOf(t *testing.T) {
	for p, want := range map[string]string{
		"hero/hero.anim.json":  "clip",
		"hero/sheet.json":      "",
		"ui/Cursor.PNG":        "sprite",
		"fx/flash.kage":        "shader",
		"sfx/hit.wav":          "sound",
		"talk/kip.voice.json":  "voice",
		"talk/v.dialogue.json": "dialogue",
		"talk/elf.lang.json":   "",
		"readme.md":            "",
	} {
		if got := kindOf(p); got != want {
			t.Errorf("kindOf(%q) = %q, want %q", p, got, want)
		}
	}
}

func TestCreateAndAssetField(t *testing.T) {
	f := newFixture(t)
	f.recv(documentMsg(visual("visual-1", 0, 0)))
	f.e.Camera().Pos = gfx.Vec2{X: 30, Y: 40}

	f.recv(`{"type":"create","class":"gfx:Visual"}`)
	created := f.host.take("create")
	if len(created) != 1 {
		t.Fatalf("create = %v (errors %v)", created, f.host.take("error"))
	}
	obj := created[0]["object"].(map[string]any)
	pos := obj["components"].(map[string]any)["gfx:Transform"].(map[string]any)["Pos"].(map[string]any)
	if obj["id"] != "visual-2" || pos["X"] != 30.0 || pos["Y"] != 40.0 {
		t.Fatalf("created %v", obj)
	}
	// Required components start at their declared defaults (+game:default).
	scale := obj["components"].(map[string]any)["gfx:Transform"].(map[string]any)["Scale"].(map[string]any)
	if scale["X"] != 1.0 || scale["Y"] != 1.0 {
		t.Errorf("created Scale %v, want the default (1, 1)", scale)
	}
	f.recv(`{"type":"create","class":"gfx:AnimatedVisual"}`)
	created = f.host.take("create")
	if len(created) != 1 {
		t.Fatalf("create = %v (errors %v)", created, f.host.take("error"))
	}
	anim := created[0]["object"].(map[string]any)["components"].(map[string]any)["gfx:Animator"].(map[string]any)
	if anim["Speed"] != 1.0 {
		t.Errorf("created Animator %v, want Speed 1", anim)
	}
	f.recv(`{"type":"create","class":"gfx:Nope"}`)
	if errs := f.host.take("error"); len(errs) != 1 || !strings.Contains(errs[0]["message"].(string), `unknown object type "gfx:Nope"`) {
		t.Errorf("errors %v", errs)
	}

	f.recv(`{"type":"assetField","id":"visual-1","field":["gfx:Sprite","ID"],"path":"box.png"}`)
	if got := f.host.take("field"); len(got) != 1 || got[0]["value"] != "box.png" || got[0]["id"] != "visual-1" {
		t.Fatalf("field = %v (errors %v)", got, f.host.take("error"))
	}
	for msg, want := range map[string]string{
		`{"type":"assetField","id":"visual-1","field":["gfx:Sprite","Shader"],"path":"box.png"}`: "the field takes a shader, the file is a sprite",
		`{"type":"assetField","id":"visual-1","field":["gfx:Sprite","Layer"],"path":"box.png"}`:  "not an asset field",
		`{"type":"assetField","id":"visual-1","field":["gfx:Sprite","Nope"],"path":"box.png"}`:   "gfx:Sprite has no field Nope",
		`{"type":"assetField","id":"nope","field":["gfx:Sprite","ID"],"path":"box.png"}`:         `no object with id "nope"`,
		// A component the class lacks is looked up in the registry.
		`{"type":"assetField","id":"visual-1","field":["gfx:Animator","Clip"],"path":"box.png"}`: "the field takes a clip, the file is a sprite",
		`{"type":"assetField","id":"visual-1","field":["x:Nope","ID"],"path":"box.png"}`:         "x:Nope is not a registered component",
	} {
		f.recv(msg)
		if errs := f.host.take("error"); len(errs) != 1 || !strings.Contains(errs[0]["message"].(string), want) {
			t.Errorf("%s: errors %v, want %q", msg, errs, want)
		}
	}
}

// TestCameraMessages: a "camera" message restores the editor camera, also
// before the document arrives, and the camera is reported once it settles.
func TestCameraMessages(t *testing.T) {
	f := newFixture(t)
	f.recv(`{"type":"camera","x":40,"y":-20,"zoom":2}`)
	f.recv(documentMsg(visual("a", 500, 500)))
	tick := func(n int) {
		for range n {
			if err := f.e.Update(); err != nil {
				t.Fatal(err)
			}
		}
	}
	tick(40)
	if cam := f.e.Camera(); cam.Pos != (gfx.Vec2{X: 40, Y: -20}) || cam.Zoom != 2 {
		t.Fatalf("camera %v zoom %v; the opened level was framed over the restored view", cam.Pos, cam.Zoom)
	}
	if got := f.host.take("camera"); len(got) != 0 {
		t.Fatalf("the restored camera was sent back: %v", got)
	}

	f.src.Frame.Mouse = input.MouseFrame{X: 100, Y: 100}
	f.src.Frame.Mouse.SetDown(input.MouseMiddle, true)
	tick(1)
	f.src.Frame.Mouse.X, f.src.Frame.Mouse.Y = 120, 100 // 20 px at zoom 2: 10 world units
	tick(1)
	f.src.Frame.Mouse.SetDown(input.MouseMiddle, false)
	tick(40)
	got := f.host.take("camera")
	if len(got) != 1 || got[0]["x"] != float64(30) || got[0]["y"] != float64(-20) || got[0]["zoom"] != float64(2) {
		t.Fatalf("camera messages %v; want one at 30,-20 zoom 2", got)
	}
}

func TestSettingsSnap(t *testing.T) {
	f := newFixture(t)
	f.recv(`{"type":"settings","snap":8}`)
	if got := f.b.Snap(); got != 8 {
		t.Errorf("snap %v after settings, want 8", got)
	}
	f.recv(`{"type":"settings","snap":0}`)
	if got := f.b.Snap(); got != 0 {
		t.Errorf("snap %v after settings 0, want off", got)
	}
}

// TestSettingsColliders: the colliders part turns the outlines of edit mode
// off and on, and a message without it keeps them.
func TestSettingsColliders(t *testing.T) {
	f := newFixture(t)
	f.recv(`{"type":"settings","colliders":false}`)
	f.recv(`{"type":"settings","snap":8}`)
	if f.b.Colliders() {
		t.Error("colliders on after settings colliders:false")
	}
	f.recv(`{"type":"settings","colliders":true}`)
	if !f.b.Colliders() {
		t.Error("colliders off after settings colliders:true")
	}
}

// TestSettingsKeepsParts: a settings message changes only the parts it has,
// and the stage part is ignored by a level session.
func TestSettingsKeepsParts(t *testing.T) {
	f := newFixture(t)
	f.recv(`{"type":"settings","snap":8}`)
	f.recv(`{"type":"settings","stage":{"background":"#102030","grid":4}}`)
	if got := f.b.Snap(); got != 8 {
		t.Errorf("snap %v after a settings message without snap, want 8", got)
	}
	if got := f.host.take("error"); len(got) != 0 {
		t.Errorf("a level session reported %v for stage settings", got)
	}
}

func TestParseColor(t *testing.T) {
	if c, err := parseColor("#102030"); err != nil || c != (color.RGBA{R: 0x10, G: 0x20, B: 0x30, A: 0xff}) {
		t.Errorf("parseColor = %v, %v", c, err)
	}
	for _, bad := range []string{"", "102030", "#fff", "#12345g"} {
		if _, err := parseColor(bad); err == nil {
			t.Errorf("parseColor(%q) accepted", bad)
		}
	}
}

// TestCreateGolden pins what "create" writes for a class. The MCP server
// (tools/internal/mcpserver, place_object) must write the same objects, so
// it compares against the same file. Run with UPDATE_GOLDEN=1 to rewrite it.
func TestCreateGolden(t *testing.T) {
	f := newFixture(t)
	f.recv(documentMsg(visual("visual-1", 0, 0)))
	f.e.Camera().Pos = gfx.Vec2{X: 30, Y: 40}
	var objs []any
	for _, class := range []string{"gfx:Visual", "gfx:AnimatedVisual"} {
		f.recv(`{"type":"create","class":"` + class + `"}`)
		created := f.host.take("create")
		if len(created) != 1 {
			t.Fatalf("create %s = %v (errors %v)", class, created, f.host.take("error"))
		}
		objs = append(objs, created[0]["object"])
	}
	got, err := json.MarshalIndent(objs, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	got = append(got, '\n')
	const golden = "testdata/create-golden.json"
	if os.Getenv("UPDATE_GOLDEN") != "" {
		if err := os.WriteFile(golden, got, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("create wrote\n%s\nwant (%s)\n%s", got, golden, want)
	}
}

// TestObjectsMessageHasParts: the parts of a multi-object prefab instance
// are listed with their part id, after their root.
func TestObjectsMessageHasParts(t *testing.T) {
	kid := strings.Replace(visual("sword", 0, 0), `"Hidden":false}}}`,
		`"Hidden":false},"hierarchy:Parent":{"Entity":"knight","Local":{"Pos":{"X":5,"Y":0},"Rot":0,"Scale":{"X":1,"Y":1},"LayerOffset":0,"LayerMode":0,"YSortSide":0}}}}`, 1)
	f := newFixtureFiles(t, fstest.MapFS{"knight.prefab.json": {Data: []byte(`{"objects":[` + visual("knight", 0, 0) + `,` + kid + `]}`)}})
	f.recv(documentMsg(`{"id":"k1","prefab":"knight.prefab.json"}`))
	for range 400 {
		if len(f.host.take("applied")) > 0 && len(f.b.Level().Objects()) > 0 {
			break
		}
		f.recv(`{"type":"pause","paused":false}`)
		time.Sleep(time.Millisecond)
	}
	f.b.sendObjects()
	objs := f.host.take("objects")
	last := objs[len(objs)-1]["objects"].([]any)
	if len(last) != 2 || last[0].(map[string]any)["part"] != nil || last[1].(map[string]any)["id"] != "k1/sword" || last[1].(map[string]any)["part"] != "sword" {
		t.Fatalf("objects %v", last)
	}
}
