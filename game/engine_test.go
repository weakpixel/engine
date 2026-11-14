package game

import (
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/weakpixel/asset"
	"github.com/weakpixel/ecs"
	"github.com/weakpixel/ecs/system"
	"github.com/weakpixel/engine/extract"
	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/render"
)

func mustPanic(t *testing.T, want string, f func()) {
	t.Helper()
	defer func() {
		t.Helper()
		r := recover()
		if r == nil {
			t.Fatalf("no panic, want %q", want)
		}
		if msg, _ := r.(string); !strings.Contains(msg, want) {
			t.Fatalf("panic %v, want %q", r, want)
		}
	}()
	f()
}

func TestEngineResourcesItself(t *testing.T) {
	e := New(Config{})
	defer e.Close()
	w := e.World()
	setEngine := EngineOf(&system.Context{World: w})
	if setEngine != e {
		t.Fatal("Engine resource is not the same as the one returned by New")
	}
}

func TestNewWiresResources(t *testing.T) {
	e := New(Config{})
	defer e.Close()
	w := e.World()
	if !ecs.HasResource[render.Queue](w) || !ecs.HasResource[render.Camera](w) ||
		!ecs.HasResource[extract.Screen](w) || !ecs.HasResource[asset.Server](w) ||
		!ecs.HasResource[render.LayerConfig](w) || !ecs.HasResource[render.GroupConfig](w) || !ecs.HasResource[render.PostChain](w) {
		t.Fatal("resources missing")
	}
	if ecs.GetResource[render.GroupConfig](w) != e.Groups() {
		t.Fatal("GroupConfig resource is not e.Groups()")
	}
	if ecs.GetResource[render.PostChain](w) != e.Post() {
		t.Fatal("PostChain resource is not e.Post()")
	}
	if e.Camera().Zoom != 1 {
		t.Fatalf("zoom = %v, want default 1", e.Camera().Zoom)
	}
}

func TestLayoutUpdatesScreen(t *testing.T) {
	e := New(Config{Width: 320, Height: 180})
	defer e.Close()
	w, h := e.Layout(1280, 720)
	if w != 320 || h != 180 {
		t.Fatalf("layout = %dx%d", w, h)
	}
	if e.screen.Size.X != 320 {
		t.Fatalf("screen = %v", e.screen.Size)
	}
}

func TestStageOrder(t *testing.T) {
	e := New(Config{})
	defer e.Close()
	var got []string
	rec := func(name string) system.System {
		return func(*system.Context) { got = append(got, name) }
	}
	// Added out of order: slots, not call order, decide.
	e.AddSystem(StagePost, rec("post"))
	e.AddSystem(StageInput, rec("input"))
	e.AddSystem(StageCamera, rec("camera"))
	e.AddSystem(StageSimulate, rec("sim1"), rec("sim2"))
	e.Start(func(*Engine) { got = append(got, "start") })

	if err := e.Update(); err != nil {
		t.Fatal(err)
	}
	want := "start input sim1 sim2 camera post"
	if s := strings.Join(got, " "); s != want {
		t.Fatalf("order = %q, want %q", s, want)
	}
}

func TestCameraSnapshotBeforeCameraStage(t *testing.T) {
	e := New(Config{})
	defer e.Close()
	e.AddSystem(StageCamera, func(*system.Context) { e.Camera().Pos.X += 5 })
	for range 2 {
		if err := e.Update(); err != nil {
			t.Fatal(err)
		}
	}
	if c := e.Camera(); c.PrevPos.X != 5 || c.Pos.X != 10 {
		t.Fatalf("prev=%v pos=%v, want 5 and 10", c.PrevPos.X, c.Pos.X)
	}
}

type initRunner struct{ inited, ran int }

func (r *initRunner) Init(*system.Context) { r.inited++ }
func (r *initRunner) Run(*system.Context)  { r.ran++ }

func TestAddRunnerGetsInit(t *testing.T) {
	e := New(Config{})
	defer e.Close()
	r := &initRunner{}
	e.AddRunner(StageSimulate, r)
	e.Update()
	e.Update()
	if r.inited != 1 || r.ran != 2 {
		t.Fatalf("inited=%d ran=%d", r.inited, r.ran)
	}
}

func TestAddAfterUpdatePanics(t *testing.T) {
	e := New(Config{})
	defer e.Close()
	e.Update()
	mustPanic(t, "game: AddSystem after the first Update", func() { e.AddSystem(StageInput, func(*system.Context) {}) })
}

func TestAddInvalid(t *testing.T) {
	e := New(Config{})
	defer e.Close()
	mustPanic(t, "game: invalid stage", func() { e.AddSystem(Stage(99), func(*system.Context) {}) })
	mustPanic(t, "game: nil system", func() { e.AddSystem(StageInput, nil) })
}

func TestQuit(t *testing.T) {
	e := New(Config{})
	defer e.Close()
	e.Start(func(e *Engine) { e.Quit() })
	if err := e.Update(); err != ebiten.Termination {
		t.Fatalf("err = %v, want Termination", err)
	}
	if err := e.Update(); err != ebiten.Termination {
		t.Fatalf("second err = %v", err)
	}
}

func TestCloseIdempotent(t *testing.T) {
	e := New(Config{})
	e.Close()
	e.Close()
}

func TestSpawnedEntityExtracted(t *testing.T) {
	e := New(Config{})
	defer e.Close()
	e.Start(func(e *Engine) {
		ecs.NewEntity2(e.World(), gfx.Transform{}, gfx.DebugShape{Kind: gfx.ShapeRect, Size: gfx.Vec2{X: 4, Y: 4}})
	})
	e.SetDebug(true)
	if err := e.Update(); err != nil {
		t.Fatal(err)
	}
	if e.queue.Len() != 1 {
		t.Fatalf("queue len = %d, want 1", e.queue.Len())
	}
}

func count(v ecs.View1[gfx.Transform]) (n int) {
	for range v.Each() {
		n++
	}
	return n
}

func TestLevel(t *testing.T) {
	fsys := fstest.MapFS{
		"lvl.json": {Data: []byte(`{"objects":[{"group":"gfx","class":"Visual","components":{"gfx:Transform":{"Pos":{"X":1,"Y":2},"PrevPos":{"X":1,"Y":2},"Rot":0,"Scale":{"X":1,"Y":1}},"gfx:Sprite":{"ID":"","Layer":0,"Origin":{"X":0,"Y":0},"FlipX":false,"FlipY":false,"Tint":0,"Blend":0,"Hidden":false}}}]}`)},
	}
	e := New(Config{FS: fsys})
	defer e.Close()
	view := ecs.NewView1[gfx.Transform](e.World())

	l, err := e.LoadLevel("lvl.json")
	if err != nil {
		t.Fatal(err)
	}
	if len(l.Objects()) != 1 || count(view) != 1 {
		t.Fatalf("objects=%d entities=%d", len(l.Objects()), count(view))
	}
	e.UnloadLevel(l)
	e.UnloadLevel(l)
	if count(view) != 0 {
		t.Fatalf("entities after unload = %d", count(view))
	}
	if _, err := e.LoadLevel("missing.json"); err == nil {
		t.Fatal("want error for missing file")
	}
	if _, err := New(Config{}).LoadLevel("x"); err == nil {
		t.Fatal("want error without FS")
	}
}

func TestStatsOverlayToggle(t *testing.T) {
	e := New(Config{NoAudio: true, StatsOverlay: true})
	defer e.Close()
	if !e.StatsOverlay() {
		t.Fatal("Config.StatsOverlay not applied")
	}
	screen := ebiten.NewImage(64, 64)
	e.Draw(screen) // draws the overlay after the queue
	e.SetStatsOverlay(false)
	if e.StatsOverlay() {
		t.Fatal("SetStatsOverlay(false) ignored")
	}
	e.Draw(screen)
}

func TestShaderTime(t *testing.T) {
	const tick = time.Second / 60
	tests := []struct {
		name    string
		elapsed time.Duration
		alpha   float64
		want    time.Duration
	}{
		{"before the first Update", 0, 1, 0},
		{"alpha 1 is the last tick", 10 * tick, 1, 10 * tick},
		{"alpha 0 is the previous tick", 10 * tick, 0, 9 * tick},
		{"alpha 0.5 is between", 10 * tick, 0.5, 10*tick - tick/2},
		{"never negative", 0, 0, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := shaderTime(tc.elapsed, tick, tc.alpha); got != tc.want {
				t.Fatalf("shaderTime = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestUpdateAdvancesShaderTime(t *testing.T) {
	e := New(Config{NoAudio: true})
	defer e.Close()
	for range 3 {
		if err := e.Update(); err != nil {
			t.Fatal(err)
		}
	}
	if want := 3 * (time.Second / time.Duration(e.tps())); e.elapsed != want {
		t.Fatalf("elapsed after 3 Updates = %v, want %v", e.elapsed, want)
	}
}
