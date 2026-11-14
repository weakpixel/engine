package editor

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/weakpixel/ecs"
	"github.com/weakpixel/ecs/system"
	"github.com/weakpixel/engine/game"
	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/input"
	"github.com/weakpixel/engine/testdata/gametypes"
	"github.com/weakpixel/engine/tween"
)

// The logical screen of the test engine; the default camera shows world
// (0, 0) at its center.
const screenW, screenH = 320, 180

// fixture is an engine with a session attached, fed by a ManualSource.
type fixture struct {
	t      *testing.T
	e      *game.Engine
	s      *Session
	src    *input.ManualSource
	events []Event
}

func newFixture(t *testing.T, cfg Config) *fixture {
	t.Helper()
	return newFixtureFS(t, cfg, testFiles(t))
}

// newFixtureFS is newFixture over the game files files.
func newFixtureFS(t *testing.T, cfg Config, files fstest.MapFS) *fixture {
	t.Helper()
	f := &fixture{t: t, src: &input.ManualSource{}}
	if cfg.Path == "" {
		cfg.Path = "test.level.json"
	}
	cfg.Events = func(ev Event) { f.events = append(f.events, ev) }
	f.s = New(cfg)
	f.e = game.New(game.Config{
		NoAudio: true, Width: screenW, Height: screenH, FS: files, Tool: f.s,
		Input: game.InputConfig{Source: f.src},
	})
	t.Cleanup(f.e.Close)
	return f
}

// testFiles holds box.png, a 16x8 image: a whole-image sprite with a center
// pivot, so an object at P covers P-(8,4) .. P+(8,4).
func testFiles(t *testing.T) fstest.MapFS {
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 16, 8))); err != nil {
		t.Fatal(err)
	}
	return fstest.MapFS{"box.png": {Data: buf.Bytes()}}
}

func (f *fixture) open(doc string) {
	f.t.Helper()
	if err := f.s.Open([]byte(doc)); err != nil {
		f.t.Fatal(err)
	}
}

func (f *fixture) apply(doc string) {
	f.t.Helper()
	if err := f.s.Apply([]byte(doc)); err != nil {
		f.t.Fatal(err)
	}
}

// update runs one tick.
func (f *fixture) update() {
	f.t.Helper()
	if err := f.e.Update(); err != nil {
		f.t.Fatal(err)
	}
}

// settle ticks until every asset has loaded.
func (f *fixture) settle() {
	f.t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !f.e.Assets().Global().Done() || (f.s.Level() != nil && !f.s.Level().Scope().Done()) {
		if time.Now().After(deadline) {
			f.t.Fatal("assets never loaded")
		}
		f.update()
		time.Sleep(time.Millisecond)
	}
}

func (f *fixture) entity(id string) ecs.Entity {
	f.t.Helper()
	en, ok := f.s.Entity(id)
	if !ok {
		f.t.Fatalf("no object %q", id)
	}
	return en
}

func (f *fixture) pos(id string) gfx.Vec2 {
	f.t.Helper()
	return ecs.GetComp[gfx.Transform](f.e.World(), f.entity(id)).Pos
}

// mouse sets the cursor (logical pixels) and the buttons down.
func (f *fixture) mouse(x, y float32, buttons ...input.MouseButton) {
	f.src.Frame.Mouse = input.MouseFrame{X: x, Y: y}
	for _, b := range buttons {
		f.src.Frame.Mouse.SetDown(b, true)
	}
}

// visual is a gfx:Visual object with the box sprite.
func visual(id string, x, y float32, layer int) string {
	return fmt.Sprintf(`{"id":%q,"group":"gfx","class":"Visual","components":{`+
		`"gfx:Transform":{"Pos":{"X":%g,"Y":%g},"PrevPos":{"X":%g,"Y":%g},"Rot":0,"Scale":{"X":1,"Y":1}},`+
		`"gfx:Sprite":{"ID":"box.png","Layer":%d,"Origin":{"X":0,"Y":0},"FlipX":false,"FlipY":false,"Tint":0,"Blend":0,"Hidden":false}}}`,
		id, x, y, x, y, layer)
}

func doc(objs ...string) string { return `{"objects":[` + strings.Join(objs, ",") + `]}` }

func TestOpenNeedsIDs(t *testing.T) {
	f := newFixture(t, Config{})
	noID := strings.Replace(visual("a", 0, 0, 0), `"id":"a",`, "", 1)
	tests := []struct{ name, doc, want string }{
		{"missing id", doc(noID), `objects[0]: no "id"`},
		{"duplicate id", doc(visual("a", 0, 0, 0), visual("a", 1, 1, 0)), `objects[1]: duplicate id "a" (first used by objects[0])`},
		{"unknown object", doc(`{"id":"x","group":"gfx","class":"Nope","components":{}}`), "objects[0]: unknown object gfx:Nope"},
		{"no objects", `{}`, `editor: missing "objects"`},
		{"trailing data", `{"objects":[]} x`, "editor: unexpected data after the JSON document"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := f.s.Open([]byte(tt.doc))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Open = %v, want an error containing %q", err, tt.want)
			}
			if f.s.Level() != nil || len(f.e.Levels()) != 0 {
				t.Fatal("a failed Open left a level")
			}
		})
	}
}

func TestOpenAcceptsName(t *testing.T) {
	f := newFixture(t, Config{})
	f.open(`{"name":"Box","objects":[` + visual("a", 0, 0, 0) + `]}`)
}

func TestApplyByID(t *testing.T) {
	f := newFixture(t, Config{})
	f.open(doc(visual("a", 1, 2, 0), visual("b", 3, 4, 0)))
	w := f.e.World()
	a, b := f.entity("a"), f.entity("b")
	if l := f.s.Level(); l == nil || len(l.Objects()) != 2 || l.Objects()[0].ID != "a" {
		t.Fatalf("level objects %v", l.Objects())
	}

	// A change the document does not know about: only components whose JSON
	// changed are set, so it survives an Apply that moves a.
	ecs.GetComp[gfx.Sprite](w, a).Tint = gfx.Color(0x112233ff)

	f.apply(doc(visual("c", 5, 6, 0), visual("a", 10, 20, 0)))
	if got := f.entity("a"); got != a {
		t.Fatal("a got a new entity")
	}
	if f.pos("a") != (gfx.Vec2{X: 10, Y: 20}) {
		t.Errorf("a at %v", f.pos("a"))
	}
	if ecs.GetComp[gfx.Sprite](w, a).Tint != 0x112233ff {
		t.Error("an unchanged component was set again")
	}
	if w.IsAlive(b) {
		t.Error("b was not deleted")
	}
	if _, ok := f.s.Entity("b"); ok {
		t.Error("b is still an object")
	}
	if f.pos("c") != (gfx.Vec2{X: 5, Y: 6}) {
		t.Errorf("c at %v", f.pos("c"))
	}
	if got := f.s.IDs(); !slices.Equal(got, []string{"c", "a"}) {
		t.Errorf("IDs = %v", got)
	}
	if objs := f.s.Level().Objects(); len(objs) != 2 || objs[0].ID != "c" || objs[1].Entity != a {
		t.Errorf("level objects %v", objs)
	}

	// Reformatted but equal JSON is not a change.
	ecs.GetComp[gfx.Transform](w, a).Pos.X = 99
	pretty, _ := json.MarshalIndent(json.RawMessage(doc(visual("c", 5, 6, 0), visual("a", 10, 20, 0))), "", "  ")
	f.apply(string(pretty))
	if f.pos("a").X != 99 {
		t.Error("reformatting set the component again")
	}
}

func TestApplyInvalidChangesNothing(t *testing.T) {
	f := newFixture(t, Config{})
	f.open(doc(visual("a", 1, 2, 0)))
	n := countEntities(f.e.World())
	bad := strings.Replace(visual("a", 7, 7, 0), `"Rot":0`, `"Rot":"x"`, 1)
	err := f.s.Apply([]byte(doc(bad, visual("new", 0, 0, 0))))
	if err == nil || !strings.Contains(err.Error(), "objects[0] (a): gfx:Transform:") {
		t.Fatalf("Apply = %v", err)
	}
	if countEntities(f.e.World()) != n || f.pos("a") != (gfx.Vec2{X: 1, Y: 2}) {
		t.Fatal("an invalid document changed the World")
	}
	if _, ok := f.s.Entity("new"); ok {
		t.Fatal("an object of an invalid document exists")
	}
}

func countEntities(w *ecs.World) int {
	n := 0
	for range w.Query().Each() {
		n++
	}
	return n
}

func TestApplyClassChangeRecreates(t *testing.T) {
	f := newFixture(t, Config{})
	f.open(doc(visual("a", 1, 2, 0)))
	old := f.entity("a")
	animated := strings.Replace(strings.Replace(visual("a", 1, 2, 0), `"class":"Visual"`, `"class":"AnimatedVisual"`, 1),
		`}}}`, `},"gfx:Animator":{"Speed":1}}}`, 1)
	f.apply(doc(animated))
	if f.e.World().IsAlive(old) {
		t.Error("the old entity survived a class change")
	}
	if !ecs.HasComp[gfx.Animator](f.e.World(), f.entity("a")) {
		t.Error("the new entity has no Animator")
	}
}

// hero is a gametypes Hero; follow, if set, is the id its Follow targets.
func hero(id, follow string) string {
	f := ""
	if follow != "" {
		f = fmt.Sprintf(`,"game:Follow":{"Target":%q,"Distance":2}`, follow)
	}
	return fmt.Sprintf(`{"id":%q,"group":"game","class":"Hero","components":{"game:Transform":{"X":0,"Y":0},`+
		`"game:Acceleration":{"X":0,"Y":0},"game:State":{"Active":true,"Kind":"idle"},"game:Health":{"Current":1,"Max":1},`+
		`"game:Vel":{"X":0,"Y":0},"game:Hitbox":{"Width":1,"Height":1,"OffsetX":0,"OffsetY":0},"game:Companion":{"Name":"x"}%s}}`, id, f)
}

func TestApplyRefsAndOptionalComponents(t *testing.T) {
	f := newFixture(t, Config{})
	w := f.e.World()
	f.open(doc(hero("a", "b"), hero("b", "")))
	if got := ecs.GetComp[gametypes.Follow](w, f.entity("a")).Target; got != f.entity("b") {
		t.Fatalf("Follow.Target = %v, want b %v", got, f.entity("b"))
	}

	// b becomes a Boss (a new entity); a's text is unchanged, but its
	// reference must follow b.
	boss := strings.Replace(strings.Replace(hero("b", ""), `"class":"Hero"`, `"class":"Boss"`, 1), `}}`, `},"game:Armor":{"Value":3}}`, 1)
	f.apply(doc(hero("a", "b"), boss))
	if got := ecs.GetComp[gametypes.Follow](w, f.entity("a")).Target; got != f.entity("b") {
		t.Fatalf("after recreating b: Follow.Target = %v, want %v", got, f.entity("b"))
	}

	// Dropping the optional component removes it.
	f.apply(doc(hero("a", ""), boss))
	if ecs.HasComp[gametypes.Follow](w, f.entity("a")) {
		t.Error("Follow was not removed")
	}

	// A reference to an id that is not in the document is an error.
	if err := f.s.Apply([]byte(doc(hero("a", "nobody"), boss))); err == nil || !strings.Contains(err.Error(), `no object with id "nobody"`) {
		t.Errorf("dangling reference: %v", err)
	}
}

// An object may hold any registered component, not only its class's:
// adding one to the document sets it, removing it removes it.
func TestApplyExtraComponent(t *testing.T) {
	f := newFixture(t, Config{})
	w := f.e.World()
	f.open(doc(visual("a", 1, 2, 0)))
	armored := strings.Replace(visual("a", 1, 2, 0), `}}}`, `},"game:Armor":{"Value":4}}}`, 1)
	f.apply(doc(armored))
	if a := ecs.GetCompOrNil[gametypes.Armor](w, f.entity("a")); a == nil || a.Value != 4 {
		t.Fatalf("Armor = %v", a)
	}
	raw, err := f.s.encode("a", "game:Armor")
	if err != nil || string(raw) != `{"Value":4}` {
		t.Fatalf("encode = %s, %v", raw, err)
	}
	f.apply(doc(visual("a", 1, 2, 0)))
	if ecs.HasComp[gametypes.Armor](w, f.entity("a")) {
		t.Error("Armor was not removed")
	}
}

func TestPick(t *testing.T) {
	f := newFixture(t, Config{})
	f.open(doc(visual("back", 0, 0, 0), visual("front", 4, 0, 1), visual("far", 100, 100, 0)))
	f.settle()
	tests := []struct {
		p    gfx.Vec2
		want string
	}{
		{gfx.Vec2{X: 0, Y: 0}, "front"}, // both cover it; front has the higher layer
		{gfx.Vec2{X: -7, Y: 0}, "back"}, // only back (back spans -8..8, front -4..12)
		{gfx.Vec2{X: 100, Y: 103}, "far"},
		{gfx.Vec2{X: 50, Y: 50}, ""},
	}
	for _, tt := range tests {
		got, ok := f.s.Pick(tt.p)
		if got != tt.want || ok != (tt.want != "") {
			t.Errorf("Pick(%v) = %q, %v; want %q", tt.p, got, ok, tt.want)
		}
	}
	if r, _ := f.s.Bounds("back"); r != (gfx.Rect{Min: gfx.Vec2{X: -8, Y: -4}, Max: gfx.Vec2{X: 8, Y: 4}}) {
		t.Errorf("Bounds(back) = %v", r)
	}
}

func TestDragEmitsOneEdit(t *testing.T) {
	f := newFixture(t, Config{})
	f.open(doc(visual("a", 0, 0, 0), visual("b", 50, 0, 0)))
	f.settle()
	// The first edit tick framed the camera on the level; a is at world (0, 0).
	if c := f.e.Camera().Pos; c != (gfx.Vec2{X: 25, Y: 0}) {
		t.Fatalf("camera at %v, want the center of the level (25, 0)", c)
	}
	origin := f.e.Camera().WorldToScreen(gfx.Vec2{}, image.Pt(screenW, screenH))
	cx, cy := origin.X, origin.Y

	f.mouse(cx, cy, input.MouseLeft)
	f.update()
	f.mouse(cx+10, cy+5, input.MouseLeft)
	f.update()
	f.mouse(cx+20, cy+10, input.MouseLeft)
	f.update()
	if f.pos("a") != (gfx.Vec2{X: 20, Y: 10}) {
		t.Fatalf("a at %v while dragging", f.pos("a"))
	}
	f.mouse(cx+20, cy+10)
	f.update()

	var edits []Edit
	var sels []Selection
	for _, ev := range f.events {
		switch ev := ev.(type) {
		case Edit:
			edits = append(edits, ev)
		case Selection:
			sels = append(sels, ev)
		}
	}
	if len(sels) != 1 || !slices.Equal(sels[0].IDs, []string{"a"}) {
		t.Errorf("selections %v", sels)
	}
	if len(edits) != 1 {
		t.Fatalf("%d edits, want 1: %v", len(edits), edits)
	}
	ed := edits[0]
	if ed.ID != "a" || ed.Component != TransformKey || !strings.Contains(string(ed.JSON), `"Pos":{"X":20,"Y":10}`) {
		t.Errorf("edit %s %s %s", ed.ID, ed.Component, ed.JSON)
	}

	// The shell writes the edit into the document and sends it back: a
	// no-op. A click on empty space clears the selection.
	f.apply(doc(visual("a", 20, 10, 0), visual("b", 50, 0, 0)))
	f.mouse(cx-100, cy-60, input.MouseLeft)
	f.update()
	f.mouse(cx-100, cy-60)
	f.update()
	if len(f.s.Selected()) != 0 {
		t.Errorf("selection %v after clicking empty space", f.s.Selected())
	}
	if last, ok := f.events[len(f.events)-1].(Selection); !ok || len(last.IDs) != 0 {
		t.Errorf("last event %v, want an empty Selection", f.events[len(f.events)-1])
	}
}

func TestCameraPanAndZoom(t *testing.T) {
	f := newFixture(t, Config{})
	f.open(doc())
	cam := f.e.Camera()
	f.mouse(100, 100, input.MouseMiddle)
	f.update()
	f.mouse(110, 95, input.MouseMiddle)
	f.update()
	if cam.Pos != (gfx.Vec2{X: -10, Y: 5}) {
		t.Fatalf("camera at %v after panning", cam.Pos)
	}

	size := image.Pt(screenW, screenH)
	cursor := gfx.Vec2{X: 40, Y: 30}
	before := cam.ScreenToWorld(cursor, size)
	f.mouse(cursor.X, cursor.Y)
	f.src.Frame.Mouse.WheelY = 2
	f.update()
	if cam.Zoom <= 1 {
		t.Fatalf("zoom %v after wheel up", cam.Zoom)
	}
	after := cam.ScreenToWorld(cursor, size)
	if d := after.Sub(before); d.X*d.X+d.Y*d.Y > 1e-6 {
		t.Errorf("the point under the cursor moved from %v to %v", before, after)
	}
}

func TestPlayStop(t *testing.T) {
	resets := 0
	f := newFixture(t, Config{OnReset: func(*game.Engine) { resets++ }})
	f.open(doc(visual("a", 0, 0, 0), visual("b", 5, 5, 0)))
	f.s.Select("b")
	w := f.e.World()

	// A game system that moves a, deletes b and spawns an entity per tick.
	runs := 0
	var spawned []ecs.Entity
	f.e.AddSystem(game.StageSimulate, func(ctx *system.Context) {
		runs++
		if en, ok := f.s.Entity("a"); ok && w.IsAlive(en) {
			ecs.GetComp[gfx.Transform](w, en).Pos.X += 1
		}
		if en, ok := f.s.Entity("b"); ok && w.IsAlive(en) {
			w.DeleteEntity(en)
		}
		spawned = append(spawned, w.NewEntity(gfx.Transform{}))
	})
	f.update()
	f.update()
	if runs != 0 {
		t.Fatalf("game systems ran %d times in edit mode", runs)
	}
	f.s.Step()
	f.update()
	f.update()
	if runs != 1 {
		t.Fatalf("Step ran the game %d times, want 1", runs)
	}

	editCam := gfx.Vec2{X: 7, Y: 7}
	f.e.Camera().Pos = editCam
	before := countEntities(w)
	f.s.Play()
	f.update()
	f.update()
	if runs != 3 || f.pos("a").X != 3 {
		t.Fatalf("Play: runs %d, a at %v", runs, f.pos("a"))
	}
	if f.e.Camera().Pos == editCam {
		t.Error("Play kept the editor camera")
	}
	en, _ := f.s.Entity("a")
	spin := tween.To(f.e.Tweens(), en, tween.Rot, 1, 10).Repeat(-1).ID()
	f.e.CameraRig().AddTrauma(1)
	f.s.Stop()
	if f.e.Tweens().Running(spin) || f.e.Tweens().Len() != 0 {
		t.Error("a tween started during Play survived Stop")
	}
	if tr := f.e.CameraRig().Trauma(); tr != 0 {
		t.Errorf("a shake started during Play survived Stop (trauma %v)", tr)
	}

	for _, en := range spawned[1:] { // spawned[0] came from Step, before Play
		if w.IsAlive(en) {
			t.Fatal("an entity spawned during Play survived Stop")
		}
	}
	if f.pos("a") != (gfx.Vec2{}) || f.pos("b") != (gfx.Vec2{X: 5, Y: 5}) {
		t.Errorf("after Stop: a at %v, b at %v", f.pos("a"), f.pos("b"))
	}
	// a, b (the stepped tick deleted it; Stop restores it from the
	// document) and the entity the stepped tick spawned before Play.
	if n := countEntities(w); n != before+1 || n != 3 {
		t.Errorf("%d entities after Stop, %d before Play", n, before)
	}
	if !slices.Equal(f.s.Selected(), []string{"b"}) {
		t.Errorf("selection %v after Stop", f.s.Selected())
	}
	if resets != 1 || f.e.Camera().Pos != editCam {
		t.Errorf("OnReset called %d times, camera at %v", resets, f.e.Camera().Pos)
	}
	var modes []bool
	for _, ev := range f.events {
		if m, ok := ev.(Mode); ok {
			modes = append(modes, m.Play)
		}
	}
	if !slices.Equal(modes, []bool{true, false}) {
		t.Errorf("modes %v", modes)
	}
}

// TestCameraReportedWhenSettled: a pan is one Camera event, sent after the
// camera stopped moving; SetCamera restores it without reporting it again.
func TestCameraReportedWhenSettled(t *testing.T) {
	f := newFixture(t, Config{})
	f.open(doc())
	count := func() (n int, last Camera) {
		for _, ev := range f.events {
			if c, ok := ev.(Camera); ok {
				n, last = n+1, c
			}
		}
		return
	}
	// Let the opening frame settle, then forget it.
	for range camSettleTicks + 2 {
		f.update()
	}
	f.events = nil

	f.mouse(100, 100, input.MouseMiddle)
	f.update()
	f.mouse(110, 95, input.MouseMiddle)
	f.update()
	f.mouse(110, 95)
	if n, _ := count(); n != 0 {
		t.Fatalf("camera reported %d times while panning", n)
	}
	for range camSettleTicks + 2 {
		f.update()
	}
	n, last := count()
	if want := (Camera{X: -10, Y: 5, Zoom: 1}); n != 1 || last != want {
		t.Fatalf("camera events %d, last %v; want one %v", n, last, want)
	}
	for range 3 * camSettleTicks {
		f.update()
	}
	if n, _ := count(); n != 1 {
		t.Errorf("an idle camera was reported again (%d events)", n)
	}

	f.events = nil
	f.s.SetCamera(gfx.Vec2{X: 30, Y: 40}, 2)
	for range 2 * camSettleTicks {
		f.update()
	}
	if cam := f.e.Camera(); cam.Pos != (gfx.Vec2{X: 30, Y: 40}) || cam.Zoom != 2 {
		t.Errorf("camera %v zoom %v after SetCamera", cam.Pos, cam.Zoom)
	}
	if n, _ := count(); n != 0 {
		t.Errorf("SetCamera was reported back (%d events)", n)
	}
}

// TestSetCameraSurvivesOpen: a restored camera is not replaced by the
// framing a newly opened level gets.
func TestSetCameraSurvivesOpen(t *testing.T) {
	f := newFixture(t, Config{})
	f.s.SetCamera(gfx.Vec2{X: 500, Y: 500}, 3)
	f.open(doc(visual("far", 100, 100, 0)))
	f.settle()
	f.update()
	if cam := f.e.Camera(); cam.Pos != (gfx.Vec2{X: 500, Y: 500}) || cam.Zoom != 3 {
		t.Errorf("camera %v zoom %v; the level framed over the restored view", cam.Pos, cam.Zoom)
	}
}

// dragFixture opens one box at the origin and returns a function that drags
// from its center by (dx, dy) screen pixels (zoom 1: world units), with
// extra keys held.
func dragFixture(t *testing.T, snap float32) (*fixture, func(dx, dy float32, keys ...input.Key)) {
	f := newFixture(t, Config{})
	f.s.SetSnap(snap)
	f.open(doc(visual("a", 0, 0, 0)))
	f.settle()
	f.update()
	grab := f.e.Camera().WorldToScreen(gfx.Vec2{}, image.Pt(screenW, screenH)) // the box center
	return f, func(dx, dy float32, keys ...input.Key) {
		f.src.Frame.Keys = input.KeySet{}
		for _, k := range keys {
			f.src.Frame.Keys.Set(k)
		}
		f.mouse(grab.X, grab.Y, input.MouseLeft)
		f.update()
		f.mouse(grab.X+dx, grab.Y+dy, input.MouseLeft)
		f.update()
		f.mouse(grab.X+dx, grab.Y+dy)
		f.update()
	}
}

func TestDragSnapsToGrid(t *testing.T) {
	tests := []struct {
		name string
		snap float32
		keys []input.Key
		want gfx.Vec2
	}{
		{"off", 0, nil, gfx.Vec2{X: 13, Y: 6}},
		{"grid 8", 8, nil, gfx.Vec2{X: 16, Y: 8}},
		{"alt ignores the grid", 8, []input.Key{input.KeyAltLeft}, gfx.Vec2{X: 13, Y: 6}},
		{"right alt too", 8, []input.Key{input.KeyAltRight}, gfx.Vec2{X: 13, Y: 6}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, drag := dragFixture(t, tt.snap)
			drag(13, 6, tt.keys...)
			if got := f.pos("a"); got != tt.want {
				t.Errorf("object at %v, want %v", got, tt.want)
			}
		})
	}
}

// TestMoveHandle: with one object selected, pressing an arrow of its handle
// drags along that axis only, even when the cursor moves on both.
func TestMoveHandle(t *testing.T) {
	tests := []struct {
		name  string
		press gfx.Vec2 // from the object's window position
		want  gfx.Vec2
	}{
		{"x arrow", gfx.Vec2{X: 20}, gfx.Vec2{X: 13}},
		{"y arrow", gfx.Vec2{Y: 20}, gfx.Vec2{Y: 9}},
		{"x arrow, near its line", gfx.Vec2{X: 30, Y: 5}, gfx.Vec2{X: 13}},
		{"past the tip, a plain click", gfx.Vec2{X: 60}, gfx.Vec2{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newFixture(t, Config{})
			f.open(doc(visual("a", 0, 0, 0)))
			f.settle()
			f.update()
			o := f.e.Camera().WorldToScreen(gfx.Vec2{}, image.Pt(screenW, screenH))
			f.mouse(o.X, o.Y, input.MouseLeft) // select
			f.update()
			f.mouse(o.X, o.Y)
			f.update()
			if sel := f.s.Selected(); len(sel) != 1 || sel[0] != "a" {
				t.Fatalf("selection %v", sel)
			}
			p := gfx.Vec2{X: o.X + tt.press.X, Y: o.Y + tt.press.Y}
			f.mouse(p.X, p.Y, input.MouseLeft)
			f.update()
			f.mouse(p.X+13, p.Y+9, input.MouseLeft)
			f.update()
			f.mouse(p.X+13, p.Y+9)
			f.update()
			if got := f.pos("a"); got != tt.want {
				t.Errorf("object at %v, want %v", got, tt.want)
			}
		})
	}
}

// TestMoveHandleHidden: no handle for several objects or none.
func TestMoveHandleHidden(t *testing.T) {
	f := newFixture(t, Config{})
	f.open(doc(visual("a", 0, 0, 0), visual("b", 50, 0, 0)))
	f.settle()
	if _, ok := f.s.handleOrigin(); ok {
		t.Error("handle without a selection")
	}
	f.s.Select("a", "b")
	if _, ok := f.s.handleOrigin(); ok {
		t.Error("handle for two objects")
	}
	f.s.Select("a")
	if _, ok := f.s.handleOrigin(); !ok {
		t.Error("no handle for one object")
	}
}

func TestSessionName(t *testing.T) {
	f := newFixture(t, Config{})
	if f.s.Name() != "" {
		t.Fatalf("Name() = %q before Open", f.s.Name())
	}
	f.open(`{"name":"Box","objects":[` + visual("a", 0, 0, 0) + `]}`)
	if f.s.Name() != "Box" {
		t.Fatalf("Name() = %q, want Box", f.s.Name())
	}
	// A rejected document (no id) leaves the name alone.
	if err := f.s.Open([]byte(`{"name":"Bad","objects":[{"group":"gfx","class":"Visual","components":{}}]}`)); err == nil {
		t.Fatal("document without id accepted")
	}
	if f.s.Name() != "Box" {
		t.Errorf("Name() = %q after a rejected Open, want Box", f.s.Name())
	}
	f.open(`{"name":"Crate","objects":[]}`)
	if f.s.Name() != "Crate" {
		t.Errorf("Name() = %q after a second Open, want Crate", f.s.Name())
	}
	f.open(doc())
	if f.s.Name() != "" {
		t.Errorf("Name() = %q for an unnamed document, want empty", f.s.Name())
	}
}

// TestOpenAtSwitchesLevel: switching files stops Play, clears the
// selection, uses the new path and frames the new level.
func TestOpenAtSwitchesLevel(t *testing.T) {
	resets := 0
	f := newFixture(t, Config{OnReset: func(*game.Engine) { resets++ }})
	w := f.e.World()
	var spawned ecs.Entity
	f.e.AddSystem(game.StageSimulate, func(ctx *system.Context) {
		if spawned == (ecs.Entity{}) {
			spawned = w.NewEntity(gfx.Transform{})
		}
	})
	f.s.SetCamera(gfx.Vec2{X: 500, Y: 500}, 2)
	f.open(doc(visual("a", 0, 0, 0)))
	f.settle()
	f.update()
	a := f.entity("a")
	f.s.Select("a")
	f.s.Play()
	f.update()
	f.events = nil

	if err := f.s.OpenAt("b.level.json", []byte(doc(visual("b", 100, 60, 0)))); err != nil {
		t.Fatal(err)
	}
	f.settle()
	f.update()
	if f.s.Playing() || resets != 1 {
		t.Errorf("playing %v, OnReset called %d times", f.s.Playing(), resets)
	}
	if spawned == (ecs.Entity{}) || w.IsAlive(a) || w.IsAlive(spawned) {
		t.Error("entities of the old level survived the switch")
	}
	if !slices.Equal(f.s.IDs(), []string{"b"}) || len(f.s.Selected()) != 0 {
		t.Errorf("ids %v, selection %v", f.s.IDs(), f.s.Selected())
	}
	if p := f.s.Level().Path(); p != "b.level.json" {
		t.Errorf("level path %q", p)
	}
	if cam := f.e.Camera(); cam.Pos != (gfx.Vec2{X: 100, Y: 60}) {
		t.Errorf("camera at %v, want the new level framed", cam.Pos)
	}
	var sel, modes int
	for _, ev := range f.events {
		switch ev := ev.(type) {
		case Selection:
			if len(ev.IDs) == 0 {
				sel++
			}
		case Mode:
			modes++
		}
	}
	if sel != 1 || modes != 1 {
		t.Errorf("%d empty selection events, %d mode events; want 1 each", sel, modes)
	}

	// A camera restored after the switch wins over framing.
	if err := f.s.OpenAt("c.level.json", []byte(doc(visual("c", -40, 0, 0)))); err != nil {
		t.Fatal(err)
	}
	f.s.SetCamera(gfx.Vec2{X: 7, Y: 8}, 3)
	f.settle()
	f.update()
	if cam := f.e.Camera(); cam.Pos != (gfx.Vec2{X: 7, Y: 8}) || cam.Zoom != 3 {
		t.Errorf("camera %v zoom %v; the level framed over the restored view", cam.Pos, cam.Zoom)
	}

	// A document that does not parse changes nothing.
	if err := f.s.OpenAt("d.level.json", []byte(`{"objects":[{}]}`)); err == nil {
		t.Fatal("OpenAt accepted an object without id")
	}
	if !slices.Equal(f.s.IDs(), []string{"c"}) || f.s.Level().Path() != "c.level.json" {
		t.Errorf("a bad document changed the level: ids %v, path %q", f.s.IDs(), f.s.Level().Path())
	}
}

// TestPanGestures: the middle button, or the left button with Space held,
// pans; the right button does not, and a left press with Space held does
// not select the object under it.
func TestPanGestures(t *testing.T) {
	for _, tc := range []struct {
		name string
		keys []input.Key
		btn  input.MouseButton
		pan  bool
	}{
		{"middle", nil, input.MouseMiddle, true},
		{"space left", []input.Key{input.KeySpace}, input.MouseLeft, true},
		{"right", nil, input.MouseRight, false},
		{"left", nil, input.MouseLeft, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newFixture(t, Config{})
			f.open(doc(visual("a", 0, 0, 0)))
			f.settle()
			cam := f.e.Camera()
			start := cam.Pos
			o := cam.WorldToScreen(gfx.Vec2{}, image.Pt(screenW, screenH))
			f.keys(tc.keys...)
			f.mouse(o.X, o.Y, tc.btn)
			f.update()
			f.mouse(o.X+10, o.Y-5, tc.btn)
			f.update()
			if moved := cam.Pos != start; moved != tc.pan {
				t.Fatalf("camera %v -> %v, pan = %v", start, cam.Pos, tc.pan)
			}
			if tc.pan && len(f.s.Selected()) != 0 {
				t.Errorf("selected %v while panning", f.s.Selected())
			}
		})
	}
}
