package viewer

import (
	"encoding/json"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/weakpixel/ecs"
	"github.com/weakpixel/engine/editor/toolshell"
	"github.com/weakpixel/engine/game"
	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/input"
	testassets "github.com/weakpixel/engine/testdata/assets"
	"github.com/weakpixel/engine/tiled"
	"github.com/weakpixel/engine/tilemap"
)

// openFiles is like open, with a recording transport so tests see the
// messages the tool sends.
func openFiles(t *testing.T, files fstest.MapFS, key string) (*game.Engine, *Viewer, *input.ManualSource, *recorder) {
	t.Helper()
	src, rec := &input.ManualSource{}, &recorder{}
	v := New().(*Viewer)
	sh := toolshell.New(toolshell.Config{Name: "view", Tool: v, Key: key, FS: files, Transport: rec})
	e := game.New(game.Config{NoAudio: true, Width: 320, Height: 180, FS: files, Tool: sh, Input: game.InputConfig{Source: src}})
	t.Cleanup(e.Close)
	if err := e.Update(); err != nil {
		t.Fatal(err)
	}
	if err := v.Panel().Check(); err != nil {
		t.Fatalf("panel: %v", err)
	}
	return e, v, src, rec
}

// opens returns the paths of the open messages sent so far.
func opens(r *recorder) []string {
	var out []string
	for _, m := range r.msgs {
		if m["type"] == "open" {
			out = append(out, m["path"].(string))
		}
	}
	return out
}

func press(t *testing.T, v *Viewer, id string) {
	t.Helper()
	if err := v.Set(id, json.RawMessage("null")); err != nil {
		t.Fatalf("press %s: %v", id, err)
	}
}

func toggle(t *testing.T, v *Viewer, id string, on bool) {
	t.Helper()
	b, _ := json.Marshal(on)
	if err := v.Set(id, b); err != nil {
		t.Fatalf("set %s: %v", id, err)
	}
}

// moveTo puts the mouse over world position p.
func moveTo(e *game.Engine, src *input.ManualSource, sh *toolshell.Shell, p gfx.Vec2) {
	s := e.Camera().WorldToScreen(p, sh.ScreenSize())
	src.Frame.Mouse.X, src.Frame.Mouse.Y = s.X, s.Y
}

func checkboxValue(t *testing.T, v toolshell.Tool, id string) bool {
	t.Helper()
	return label(t, v, id) == "true"
}

func TestHitTable(t *testing.T) {
	obj := func(shape tiled.Shape, w, h, rot float64, pts ...tiled.Point) *tiled.Object {
		return &tiled.Object{Shape: shape, Width: w, Height: h, Rotation: rot, Points: pts}
	}
	tri := []tiled.Point{{X: 0, Y: 0}, {X: 20, Y: 0}, {X: 0, Y: 20}}
	line := []tiled.Point{{X: 0, Y: 0}, {X: 40, Y: 0}}
	at := gfx.Vec2{X: 100, Y: 100}
	for _, c := range []struct {
		name string
		o    *tiled.Object
		p    gfx.Vec2
		want bool
	}{
		{"rect inside", obj(tiled.ShapeRect, 20, 10, 0), gfx.Vec2{X: 110, Y: 105}, true},
		{"rect outside", obj(tiled.ShapeRect, 20, 10, 0), gfx.Vec2{X: 110, Y: 120}, false},
		{"rect rotated 90 turns clockwise", obj(tiled.ShapeRect, 20, 10, 90), gfx.Vec2{X: 95, Y: 110}, true},
		{"rect rotated 90 leaves its old place", obj(tiled.ShapeRect, 20, 10, 90), gfx.Vec2{X: 115, Y: 105}, false},
		{"ellipse by its box", obj(tiled.ShapeEllipse, 20, 20, 0), gfx.Vec2{X: 101, Y: 101}, true},
		{"tile object sits above its anchor", obj(tiled.ShapeTile, 16, 16, 0), gfx.Vec2{X: 108, Y: 92}, true},
		{"tile object not below its anchor", obj(tiled.ShapeTile, 16, 16, 0), gfx.Vec2{X: 108, Y: 108}, false},
		{"polygon inside", obj(tiled.ShapePolygon, 0, 0, 0, tri...), gfx.Vec2{X: 105, Y: 105}, true},
		{"polygon in the box but outside", obj(tiled.ShapePolygon, 0, 0, 0, tri...), gfx.Vec2{X: 115, Y: 115}, false},
		{"polyline near", obj(tiled.ShapePolyline, 0, 0, 0, line...), gfx.Vec2{X: 120, Y: 102}, true},
		{"polyline far", obj(tiled.ShapePolyline, 0, 0, 0, line...), gfx.Vec2{X: 120, Y: 110}, false},
		{"polyline past its end", obj(tiled.ShapePolyline, 0, 0, 0, line...), gfx.Vec2{X: 150, Y: 100}, false},
		{"point near", obj(tiled.ShapePoint, 0, 0, 0), gfx.Vec2{X: 102, Y: 101}, true},
		{"point far", obj(tiled.ShapePoint, 0, 0, 0), gfx.Vec2{X: 110, Y: 100}, false},
		{"zero size rect needs the slack", obj(tiled.ShapeRect, 0, 0, 0), gfx.Vec2{X: 101, Y: 100}, true},
	} {
		if got := hit(c.o, at, c.p, 3); got != c.want {
			t.Errorf("%s: hit = %v, want %v", c.name, got, c.want)
		}
	}
}

func TestOutlines(t *testing.T) {
	at := gfx.Vec2{X: 10, Y: 20}
	r := outlines(&tiled.Object{Shape: tiled.ShapeRect, Width: 4, Height: 2}, at)
	if len(r) != 1 || !r[0].closed || !slices.Equal(r[0].pts, []gfx.Vec2{{X: 10, Y: 20}, {X: 14, Y: 20}, {X: 14, Y: 22}, {X: 10, Y: 22}}) {
		t.Errorf("rect = %+v", r)
	}
	if e := outlines(&tiled.Object{Shape: tiled.ShapeEllipse, Width: 4, Height: 2}, at); len(e[0].pts) != ellipseSegments {
		t.Errorf("ellipse has %d points, want %d", len(e[0].pts), ellipseSegments)
	}
	if pl := outlines(&tiled.Object{Shape: tiled.ShapePolyline, Points: []tiled.Point{{}, {X: 5}}}, at); pl[0].closed {
		t.Error("a polyline is open")
	}
	if pt := outlines(&tiled.Object{Shape: tiled.ShapePoint}, at); pt != nil {
		t.Errorf("a point has no outline, got %v", pt)
	}
}

func TestLevelsOfMap(t *testing.T) {
	files := map[string]bool{
		"a/one.tmj": true, "a/one.level.json": true, "a/two.tmj": true,
		"a/a.world": true, "a/a.level.json": true,
		"b/b.tmj": true, "b/b.level.json": true, "b/other.tmj": true,
		"loose.tmj": true, "loose.level.json": true,
		"c/c.tmj": true,
	}
	look := fileLookup{
		exists: func(k string) bool { return files[k] },
		worldMaps: func(w string) []string {
			if w == "a/a.world" {
				return []string{"a/one.tmj", "a/two.tmj"}
			}
			return nil
		},
	}
	for _, c := range []struct {
		key  string
		want []string
	}{
		{"a/one.tmj", []string{"a/one.level.json", "a/a.level.json"}},
		{"a/two.tmj", []string{"a/a.level.json"}},
		{"b/b.tmj", []string{"b/b.level.json"}},
		{"b/other.tmj", nil},
		{"loose.tmj", nil},
		{"c/c.tmj", nil},
	} {
		if got := levelsOfMap(look, c.key); !slices.Equal(got, c.want) {
			t.Errorf("levelsOfMap(%s) = %v, want %v", c.key, got, c.want)
		}
	}
	if got := levelsOfWorld(look, "a/a.world"); !slices.Equal(got, []string{"a/a.level.json"}) {
		t.Errorf("levelsOfWorld = %v", got)
	}
}

func layerEntities(e *game.Engine) []tilemap.TileLayer {
	var out []tilemap.TileLayer
	for _, c := range ecs.NewView1[tilemap.TileLayer](e.World()).Each() {
		out = append(out, *c)
	}
	return out
}

func hiddenCount(e *game.Engine) (hidden, total int) {
	for _, tl := range layerEntities(e) {
		total++
		if tl.Hidden {
			hidden++
		}
	}
	return
}

func TestMapPreview(t *testing.T) {
	e, v, src, _ := openFiles(t, testassets.Lib(), "world-1/level-1.tmj")
	mv, ok := v.inner.(*mapView)
	if !ok || !mv.ready {
		t.Fatalf("inner = %T, ready = %v", v.inner, ok)
	}
	m := mv.mapData()
	// The map is 6 x 4 cells of 16 px: framed and centered.
	if cam := e.Camera(); cam.Pos.X != 48 || cam.Pos.Y != 32 {
		t.Errorf("camera = %+v, want centered on the 96 x 64 map", *cam)
	}
	if got := label(t, v, "map.size"); got != "6 × 4 cells, 96 × 64 px" {
		t.Errorf("size = %q", got)
	}
	_, total := hiddenCount(e)
	drawable := 0
	for _, l := range m.Layers {
		if l.Kind == tilemap.Tiles || l.Kind == tilemap.Image {
			drawable++
		}
	}
	if total != drawable || drawable == 0 {
		t.Fatalf("TileLayer entities = %d, want one per tile and image layer (%d)", total, drawable)
	}
	// The background image layer is invisible in Tiled: its checkbox is off.
	if checkboxValue(t, v, "layer.0") {
		t.Error("the invisible image layer starts checked")
	}
	hidden0, _ := hiddenCount(e)
	toggle(t, v, "layer.0", true)
	if hidden, _ := hiddenCount(e); hidden != hidden0-1 {
		t.Errorf("checking the image layer: hidden %d -> %d, want one less", hidden0, hidden)
	}
	// A group's checkbox hides its children: every layer below entity-group.
	group, children := -1, 0
	for i, l := range m.Layers {
		if l.Kind == tilemap.Group && l.Name == "entity-group" {
			group = i
		}
		if l.Parent == group && group >= 0 && (l.Kind == tilemap.Tiles || l.Kind == tilemap.Image) {
			children++
		}
	}
	if group < 0 || children == 0 {
		t.Fatalf("no group with a drawable child in the map: group %d children %d", group, children)
	}
	before, _ := hiddenCount(e)
	toggle(t, v, "layer."+itoa(group), false)
	if after, _ := hiddenCount(e); after != before+children {
		t.Errorf("hiding the group: hidden %d -> %d, want %d more", before, after, children)
	}
	toggle(t, v, "layer."+itoa(group), true)
	if after, _ := hiddenCount(e); after != before {
		t.Errorf("showing the group: hidden %d, want %d", after, before)
	}

	// The three point objects of the entities layer: hover the last, click it.
	var ref objRef
	for i := range m.Layers {
		if m.Layers[i].Kind == tilemap.Objects && len(m.Layers[i].Objects) > 0 {
			ref = objRef{i, 2}
			break
		}
	}
	lay, obj := mv.object(ref)
	moveTo(e, src, mv.shell, anchor(lay, obj))
	e.Update()
	if got := label(t, v, "obj.which"); got != "3 (hover)" {
		t.Fatalf("hover = %q, want object 3", got)
	}
	if got := label(t, v, "obj.class"); got != "chick" {
		t.Errorf("class = %q", got)
	}
	if got := label(t, v, "obj.prop.0"); got != "1" {
		t.Errorf("property mass = %q", got)
	}
	src.Frame.Mouse.Buttons = 1
	e.Update()
	src.Frame.Mouse.Buttons = 0
	src.Frame.Mouse.X, src.Frame.Mouse.Y = 0, 0
	e.Update()
	if got := label(t, v, "obj.which"); got != "3 (selected)" {
		t.Errorf("after the click = %q", got)
	}
	// Hiding the object layer takes its objects out of hover.
	src.Frame.Mouse.Buttons = 1
	e.Update() // a click on nothing clears the selection...
	src.Frame.Mouse.Buttons = 0
	toggle(t, v, "layer."+itoa(ref.layer), false)
	moveTo(e, src, mv.shell, anchor(lay, obj))
	e.Update()
	if got := label(t, v, "obj.which"); got == "3 (hover)" {
		t.Errorf("a hidden layer's object is still hovered: %q", got)
	}

	// No package level around the map? It has one: level-1.level.json.
	if _, ok := v.Panel().Find("level.0"); !ok {
		t.Error("no Open button for level-1.level.json")
	}
}

func TestMapReloadKeepsToggles(t *testing.T) {
	files := testassets.Lib()
	e, v, _, _ := openFiles(t, files, "world-1/level-1.tmj")
	mv := v.inner.(*mapView)
	toggle(t, v, "layer.0", true)
	cam := *e.Camera()
	cam.Pos.X += 5 // not a Frame: the camera must stay where the user put it
	*e.Camera() = cam
	before := len(mv.ents)
	v1 := mv.version
	e.Assets().Changed("world-1/level-1.tmj")
	for range 5 {
		time.Sleep(10 * time.Millisecond)
		e.Update()
		if mv.version != v1 {
			break
		}
	}
	if mv.version == v1 {
		t.Fatal("the map did not reload after Changed")
	}
	if len(mv.ents) != before {
		t.Errorf("entities = %d, want %d", len(mv.ents), before)
	}
	if !checkboxValue(t, v, "layer.0") {
		t.Error("the toggle was lost by the reload")
	}
	if e.Camera().Pos != cam.Pos {
		t.Errorf("camera = %v, want it kept at %v", e.Camera().Pos, cam.Pos)
	}
}

func TestMapOpenButton(t *testing.T) {
	_, v, _, rec := openFiles(t, testassets.Lib(), "world-1/level-1.tmj")
	press(t, v, "level.0")
	if got := opens(rec); !slices.Equal(got, []string{"world-1/level-1.level.json"}) {
		t.Errorf("open messages = %v", got)
	}
	if err := v.Set("level.9", nil); err == nil {
		t.Error("an unknown level button was accepted")
	}
}

func TestMapWithoutLevels(t *testing.T) {
	files := testassets.Lib()
	delete(files, "world-1/level-1.level.json")
	_, v, _, _ := openFiles(t, files, "world-1/level-2.tmj")
	if got := label(t, v, "levels.none"); !strings.Contains(got, "none") {
		t.Errorf("levels = %q, want the section to say there are none", got)
	}
}

func TestMapFailsToLoad(t *testing.T) {
	src := &input.ManualSource{}
	v := New().(*Viewer)
	files := fstest.MapFS{"bad.tmj": {Data: []byte("{")}}
	sh := toolshell.New(toolshell.Config{Name: "view", Tool: v, Key: "bad.tmj", FS: files})
	e := game.New(game.Config{NoAudio: true, Width: 320, Height: 180, FS: files, Tool: sh, Input: game.InputConfig{Source: src}})
	defer e.Close()
	if err := e.Update(); err != nil {
		t.Fatal(err)
	}
	e.Update()
	if err := v.Panel().Check(); err != nil {
		t.Errorf("panel after a failed open: %v", err)
	}
}

func itoa(i int) string { b, _ := json.Marshal(i); return string(b) }

func TestObjectLabel(t *testing.T) {
	for _, c := range []struct {
		o    tiled.Object
		want string
	}{
		{tiled.Object{Name: "door", Class: "trigger"}, "door"},
		{tiled.Object{Class: "bug"}, "bug"},
		{tiled.Object{Shape: tiled.ShapeTile, GID: 42}, "gid 42"},
		{tiled.Object{}, ""},
	} {
		if got := objectLabel(&c.o); got != c.want {
			t.Errorf("objectLabel(%+v) = %q, want %q", c.o, got, c.want)
		}
	}
}
