package room

import (
	"bytes"
	"errors"
	"fmt"
	"image"
	"image/png"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/weakpixel/ecs"
	"github.com/weakpixel/ecs/system"
	"github.com/weakpixel/engine/asset/library"
	"github.com/weakpixel/engine/extract"
	"github.com/weakpixel/engine/game"
	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/input"
)

// transform is a gfx:Transform document at (x, y).
func transform(x, y float32) string {
	return fmt.Sprintf(`"gfx:Transform":{"Pos":{"X":%v,"Y":%v},"PrevPos":{"X":%v,"Y":%v},"Rot":0,"Scale":{"X":1,"Y":1}}`, x, y, x, y)
}

// exitDoc is a room:ExitArea at (x, y), 10 by 10, to target/spawn.
func exitDoc(x, y float32, target, spawn string, trigger ExitTrigger, action string) string {
	return fmt.Sprintf(`{"group":"room","class":"ExitArea","components":{%s,"room:Exit":{"Target":%q,"Spawn":%q,"Size":{"X":10,"Y":10},"Trigger":%d,"Action":%q,"Transition":""}}}`,
		transform(x, y), target, spawn, trigger, action)
}

// spawnDoc is a room:SpawnPoint named name at (x, y).
func spawnDoc(x, y float32, name string) string {
	return fmt.Sprintf(`{"group":"room","class":"SpawnPoint","components":{%s,"room:Spawn":{"Name":%q}}}`, transform(x, y), name)
}

func level(objects ...string) *fstest.MapFile {
	return &fstest.MapFile{Data: []byte(`{"objects":[` + strings.Join(objects, ",") + `]}`)}
}

// tileset is a one-tile tileset image.
func tileset(t *testing.T) *fstest.MapFile {
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 8, 8))); err != nil {
		t.Fatal(err)
	}
	return &fstest.MapFile{Data: buf.Bytes()}
}

// roomFiles is a library of three rooms:
//
//   - a: an exit at (100, 0) to b's "from-a", an action exit at (200, 0) to
//     b's "door", and spawns "start" (0, 0) and "back" (50, 50);
//   - b: spawn "from-a" at (5, 5) inside an exit back to a's "back", and a
//     map whose Tiled point "door" is at (4, 4) (the map is at the origin);
//   - broken: a GOD document that does not decode.
func roomFiles(t *testing.T) fstest.MapFS {
	m := `{"orientation":"orthogonal","width":2,"height":2,"tilewidth":8,"tileheight":8,
		"tilesets":[{"firstgid":1,"source":"t.tsj"}],
		"layers":[{"type":"tilelayer","name":"ground","width":2,"height":2,"data":[1,1,1,1]},
		          {"type":"objectgroup","name":"o","objects":[{"id":1,"name":"door","point":true,"x":4,"y":4}]}]}`
	return fstest.MapFS{
		"global/bindings.json": {Data: []byte(`{"version": 1, "mappings": {"gameplay": {"bindings": {"interact": [{"control": "key:Space"}]}}}}`)},
		"a/a.level.json": level(
			exitDoc(100, 0, "b/b", "from-a", OnEnter, ""),
			exitDoc(200, 0, "b/b", "door", OnAction, "interact"),
			spawnDoc(0, 0, "start"), spawnDoc(50, 50, "back"),
		),
		"b/b.level.json":           level(exitDoc(0, 0, "a/a", "back", OnEnter, ""), spawnDoc(5, 5, "from-a")),
		"b/extra.level.json":       level(spawnDoc(7, 9, "extra")),
		"b/b.tmj":                  {Data: []byte(m)},
		"b/t.tsj":                  {Data: []byte(`{"name":"t","image":"t.png","tilewidth":8,"tileheight":8,"columns":1,"tilecount":1}`)},
		"b/t.png":                  tileset(t),
		"global/fx/t.kage":         {Data: []byte("//kage:unit pixels\n\npackage main\n\nvar Params vec4\n\nfunc Fragment(dst vec4, src vec2, color vec4) vec4 {\n\treturn imageSrc0At(src) * Params.x\n}\n")},
		"broken/broken.level.json": {Data: []byte(`{"objects":[{"group":"room","class":"Nope","components":{}}]}`)},
	}
}

// env is an engine with the rooms of roomFiles, a hero and its input.
type env struct {
	e       *game.Engine
	rooms   *Rooms
	hero    ecs.Entity
	src     *input.ManualSource
	walkTo  *gfx.Vec2 // where the walk system moves the hero next
	errs    []error
	entered []Entered
}

func newEnv(t *testing.T, cfg Config) *env {
	t.Helper()
	lib, err := library.Open(roomFiles(t))
	if err != nil {
		t.Fatal(err)
	}
	acts := &input.Actions{}
	acts.Button("interact", "Use a door.")
	en := &env{src: &input.ManualSource{}}
	en.e = game.New(game.Config{NoAudio: true, Width: 64, Height: 64, Library: lib,
		Input: game.InputConfig{Actions: acts, Bindings: "global/bindings.json", Source: en.src, HideTouchControls: true}})
	t.Cleanup(en.e.Close)
	en.e.Input().Player(0).Push("gameplay")
	if cfg.OnError == nil {
		cfg.OnError = func(err error) { en.errs = append(en.errs, err) }
	}
	if cfg.OnEnter == nil {
		cfg.OnEnter = func(ev Entered) { en.entered = append(en.entered, ev) }
	}
	en.rooms = Install(en.e, cfg)
	en.hero = ecs.NewEntity2(en.e.World(), gfx.Transform{}, Traveler{})
	en.e.AddSystem(game.StageSimulate, func(*system.Context) {
		if en.walkTo != nil {
			ecs.GetComp[gfx.Transform](en.e.World(), en.hero).Pos = *en.walkTo
			en.walkTo = nil
		}
	})
	return en
}

// tickUntil updates the engine until done is true. Assets decode on worker
// goroutines, so it waits in real time.
func (en *env) tickUntil(t *testing.T, what string, done func() bool) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for !done() {
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for %s", what)
		}
		if err := en.e.Update(); err != nil {
			t.Fatal(err)
		}
		time.Sleep(100 * time.Microsecond)
	}
}

func (en *env) update(t *testing.T, n int) {
	t.Helper()
	for range n {
		if err := en.e.Update(); err != nil {
			t.Fatal(err)
		}
	}
}

func (en *env) idle(t *testing.T) {
	t.Helper()
	en.tickUntil(t, "the transition to end", func() bool { return !en.rooms.Busy() })
}

func (en *env) pos() gfx.Vec2 { return ecs.GetComp[gfx.Transform](en.e.World(), en.hero).Pos }

// walk makes the walk system move the hero to p in the next tick, as a
// game system would: PrevPos keeps where it was before.
func (en *env) walk(p gfx.Vec2) { en.walkTo = &p }

// start enters room a at "start".
func (en *env) start(t *testing.T) {
	t.Helper()
	if !en.rooms.Start("a/a", "start") {
		t.Fatal("Start returned false")
	}
	en.idle(t)
	if en.rooms.Room() != "a/a" || len(en.rooms.Current()) != 1 {
		t.Fatalf("room %q, %d levels after Start", en.rooms.Room(), len(en.rooms.Current()))
	}
}

func count[A, B any](v ecs.View2[A, B]) int {
	n := 0
	for range v.Each() {
		n++
	}
	return n
}

func TestStart(t *testing.T) {
	en := newEnv(t, Config{})
	en.start(t)
	if p := en.pos(); p != (gfx.Vec2{}) {
		t.Fatalf("hero at %v, want the start spawn (0, 0)", p)
	}
	if len(en.entered) != 1 || en.entered[0].From != "" || en.entered[0].To != "a/a" || en.entered[0].Spawn != "start" || len(en.entered[0].Levels) != 1 {
		t.Fatalf("entered %+v", en.entered)
	}
	if len(en.errs) != 0 {
		t.Fatal(en.errs)
	}
}

// TestWalkIntoExit: entering the exit swaps a for b, keeps the hero, lands
// it on the spawn and unloads a; arriving inside b's exit back does not
// fire it.
func TestWalkIntoExit(t *testing.T) {
	en := newEnv(t, Config{})
	en.start(t)
	old := en.rooms.Current()[0]
	en.walk(gfx.Vec2{X: 105, Y: 5})
	en.update(t, 1)
	if !en.rooms.Busy() || !en.e.Held() {
		t.Fatalf("busy %v held %v after walking into the exit", en.rooms.Busy(), en.e.Held())
	}
	en.idle(t)
	if en.rooms.Room() != "b/b" || en.rooms.Current()[0] == old || en.e.Held() {
		t.Fatalf("room %q, held %v", en.rooms.Room(), en.e.Held())
	}
	if !en.e.World().IsAlive(en.hero) {
		t.Fatal("the hero did not survive the swap")
	}
	if p := en.pos(); p != (gfx.Vec2{X: 5, Y: 5}) {
		t.Fatalf("hero at %v, want from-a (5, 5)", p)
	}
	exits := ecs.NewView2[gfx.Transform, Exit](en.e.World())
	if n := count(exits); n != 1 {
		t.Fatalf("%d exits alive, want b's one (a was unloaded)", n)
	}
	if len(en.entered) != 2 || en.entered[1].From != "a/a" || en.entered[1].To != "b/b" {
		t.Fatalf("entered %+v", en.entered)
	}
	if b := en.e.CameraRig().Bounds; b != (gfx.Rect{Max: gfx.Vec2{X: 16, Y: 16}}) {
		t.Fatalf("camera bounds %v, want b's map", b)
	}
	// The hero stands in b's exit back to a: it must not fire.
	en.update(t, 5)
	if en.rooms.Busy() || en.rooms.Room() != "b/b" {
		t.Fatal("arriving inside an exit fired it")
	}
	// Leaving and coming back in does.
	en.walk(gfx.Vec2{X: 30, Y: 30})
	en.update(t, 1)
	en.walk(gfx.Vec2{X: 2, Y: 2})
	en.update(t, 1)
	en.idle(t)
	if en.rooms.Room() != "a/a" || en.pos() != (gfx.Vec2{X: 50, Y: 50}) {
		t.Fatalf("room %q, hero %v, want a at back (50, 50)", en.rooms.Room(), en.pos())
	}
	if len(en.errs) != 0 {
		t.Fatal(en.errs)
	}
}

// TestActionExit: an OnAction exit fires on the press only, and a Tiled
// object names the spawn.
func TestActionExit(t *testing.T) {
	en := newEnv(t, Config{})
	en.tickUntil(t, "the bindings", func() bool { return en.e.Input().Bindings().HasMapping("gameplay") })
	en.start(t)
	en.walk(gfx.Vec2{X: 205, Y: 5})
	en.update(t, 3)
	if en.rooms.Busy() {
		t.Fatal("an action exit fired without the press")
	}
	en.src.Frame.Keys.Set(input.KeySpace)
	en.update(t, 1)
	if !en.rooms.Busy() {
		t.Fatal("the press did not fire the action exit")
	}
	en.idle(t)
	if en.rooms.Room() != "b/b" || en.pos() != (gfx.Vec2{X: 4, Y: 4}) {
		t.Fatalf("room %q, hero %v, want b at the Tiled point door (4, 4)", en.rooms.Room(), en.pos())
	}
	if len(en.errs) != 0 {
		t.Fatal(en.errs)
	}
}

func TestGoToErrors(t *testing.T) {
	tests := []struct {
		name, room, spawn string
		want              string
		wantRoom          string
	}{
		{"unknown room", "nope/nope", "x", `room: GoTo "nope/nope"`, "a/a"},
		{"room that does not spawn", "broken/broken", "x", `room: GoTo "broken/broken"`, "a/a"},
		{"unknown spawn", "b/b", "nowhere", `room "b/b" has no spawn "nowhere"`, "b/b"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			en := newEnv(t, Config{})
			en.start(t)
			en.walk(gfx.Vec2{X: 1, Y: 2})
			en.update(t, 1)
			if !en.rooms.GoTo(tt.room, tt.spawn) {
				t.Fatal("GoTo returned false")
			}
			en.idle(t)
			if err := errors.Join(en.errs...); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("errors %v, want %q", err, tt.want)
			}
			if en.rooms.Room() != tt.wantRoom || en.pos() != (gfx.Vec2{X: 1, Y: 2}) || en.e.Held() {
				t.Fatalf("room %q, hero %v, held %v", en.rooms.Room(), en.pos(), en.e.Held())
			}
		})
	}
}

// TestGoToTeleport: an empty room id moves the travelers inside the current
// room, and a second GoTo while busy is refused.
func TestGoToTeleport(t *testing.T) {
	en := newEnv(t, Config{})
	en.start(t)
	old := en.rooms.Current()[0]
	if !en.rooms.GoTo("", "back", Seconds(0.05, 0.05)) || en.rooms.GoTo("b/b", "from-a") {
		t.Fatal("GoTo: want the first call accepted and the second refused")
	}
	en.idle(t)
	if en.rooms.Room() != "a/a" || en.rooms.Current()[0] != old || en.pos() != (gfx.Vec2{X: 50, Y: 50}) {
		t.Fatalf("room %q, hero %v", en.rooms.Room(), en.pos())
	}
	if ev := en.entered[len(en.entered)-1]; ev.From != "a/a" || ev.To != "a/a" {
		t.Fatalf("entered %+v, want a to a", ev)
	}
}

// TestOnEnterAddsLevels: a level OnEnter adds with SetCurrent holds the
// spawn, and leaving the room unloads it too.
func TestOnEnterAddsLevels(t *testing.T) {
	var en *env
	en = newEnv(t, Config{OnEnter: func(ev Entered) {
		if ev.To != "b/b" {
			return
		}
		l, err := en.e.LoadLevel("b/extra.level.json")
		if err != nil {
			t.Error(err)
			return
		}
		en.rooms.SetCurrent("b/b", append(ev.Levels, l)...)
	}})
	en.start(t)
	en.rooms.GoTo("b/b", "extra")
	en.idle(t)
	if len(en.rooms.Current()) != 2 || en.pos() != (gfx.Vec2{X: 7, Y: 9}) {
		t.Fatalf("%d levels, hero %v, want 2 levels and the extra spawn (7, 9)", len(en.rooms.Current()), en.pos())
	}
	en.rooms.GoTo("a/a", "start")
	en.idle(t)
	if n := count(ecs.NewView2[gfx.Transform, Spawn](en.e.World())); n != 2 {
		t.Fatalf("%d spawns alive, want a's two (b and its extra level unloaded)", n)
	}
	if len(en.errs) != 0 {
		t.Fatal(en.errs)
	}
}

func TestKeepSimulating(t *testing.T) {
	en := newEnv(t, Config{KeepSimulating: true})
	en.start(t)
	en.rooms.GoTo("", "back")
	en.update(t, 1)
	if !en.rooms.Busy() || en.e.Held() {
		t.Fatalf("busy %v held %v, want busy and not held", en.rooms.Busy(), en.e.Held())
	}
}

// TestDebugShapes: exits and spawns get a debug shape of their size.
func TestDebugShapes(t *testing.T) {
	en := newEnv(t, Config{})
	en.start(t)
	w := en.e.World()
	for en, c := range ecs.NewView2[gfx.Transform, Exit](w).Each() {
		_, ex := c.Unpack()
		if ds := ecs.GetCompOrNil[gfx.DebugShape](w, en); ds == nil || ds.Kind != gfx.ShapeRect || ds.Size != ex.Size {
			t.Fatalf("exit shape %+v", ds)
		}
	}
	for en := range ecs.NewView2[gfx.Transform, Spawn](w).Each() {
		if ds := ecs.GetCompOrNil[gfx.DebugShape](w, en); ds == nil || ds.Kind != gfx.ShapeCircle {
			t.Fatalf("spawn shape %+v", ds)
		}
	}
}

func TestIdleTickNoAllocs(t *testing.T) {
	if testing.CoverMode() != "" {
		t.Skip("instrumentation allocates")
	}
	en := newEnv(t, Config{})
	en.start(t)
	en.update(t, 2)
	allocs := testing.AllocsPerRun(50, func() { en.rooms.markShapes(); en.rooms.checkExits() })
	if allocs != 0 {
		t.Fatalf("idle director tick allocates %v times", allocs)
	}
}

func ecsScreen(en *env) image.Point {
	return ecs.GetResource[extract.Screen](en.e.World()).Size
}

func TestNoExits(t *testing.T) {
	en := newEnv(t, Config{NoExits: true})
	en.start(t)
	en.walk(gfx.Vec2{X: 105, Y: 5})
	en.update(t, 2)
	if en.rooms.Busy() {
		t.Fatal("an exit fired with NoExits")
	}
}

// TestRoomsInScene: with Config.Scene the director runs as a system of the
// scene and loads rooms through it, so popping the scene unloads the room;
// the hero (a root entity) survives.
func TestRoomsInScene(t *testing.T) {
	scene := game.NewScene("play", game.SceneOptions{UpdateBelow: true})
	en := newEnv(t, Config{Scene: scene})
	en.e.Push(scene)
	en.update(t, 1)
	en.start(t)
	en.walk(gfx.Vec2{X: 105, Y: 5})
	en.update(t, 1)
	en.idle(t)
	if en.rooms.Room() != "b/b" {
		t.Fatalf("room %q, want b after walking into the exit", en.rooms.Room())
	}
	spawns := ecs.NewView2[gfx.Transform, Spawn](en.e.World())
	if count(spawns) == 0 {
		t.Fatal("room b has no spawn")
	}

	en.e.Pop()
	en.update(t, 1)
	if n := count(spawns); n != 0 || len(en.e.Levels()) != 0 {
		t.Fatalf("after the pop: %d spawns, %d levels, want none", n, len(en.e.Levels()))
	}
	if !en.e.World().IsAlive(en.hero) {
		t.Fatal("the hero was deleted with the scene")
	}
}
