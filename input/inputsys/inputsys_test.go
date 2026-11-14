package inputsys

import (
	"errors"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/weakpixel/asset"
	"github.com/weakpixel/ecs"
	"github.com/weakpixel/ecs/system"
	"github.com/weakpixel/engine/input"
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

type acts struct {
	*input.Actions
	move, jump input.Action
}

func newActs() acts {
	a := acts{Actions: &input.Actions{}}
	a.move = a.Vec2("move", "Walk.")
	a.jump = a.Button("jump", "Jump.")
	a.Freeze()
	return a
}

const bindings = `{
  "version": 1,
  "mappings": {
    "gameplay": {
      "bindings": {
        "move": [{ "control": "touch:stick" }],
        "jump": [{ "control": "key:Space" }, { "control": "touch:jump" }]
      },
      "touch": {
        "stick": { "kind": "stick", "area": [0, 0.5, 0.5, 1], "radius": 0.1 },
        "jump": { "kind": "button", "circle": [0.9, 0.8, 0.05] }
      }
    }
  }
}`

// world is a World with the input system wired and the gameplay mapping on
// player 0.
type world struct {
	w     *ecs.World
	src   *input.ManualSource
	r     *input.Router
	sys   *InputSystem
	sched system.Schedule
}

func newWorld(t *testing.T, a acts, extra ...system.Runner) *world {
	t.Helper()
	src := &input.ManualSource{}
	src.Frame.Width, src.Frame.Height = 400, 200
	r := input.NewRouter(src, input.Options{Actions: a.Actions})
	r.Player(0).Push("gameplay")
	tw := &world{w: ecs.NewWorld(), src: src, r: r, sys: New(r)}
	ecs.SetResource(tw.w, &render.Queue{})
	tw.sched.AddStage("input")
	tw.sched.AddRunner(tw.sys)
	tw.sched.AddRunner(extra...)
	return tw
}

func (tw *world) tick() { tw.sched.Run(tw.w, time.Second/60) }

func TestNewNilRouter(t *testing.T) {
	mustPanic(t, "input: inputsys.New: nil Router", func() { New(nil) })
}

func TestRegisterLoadersPanics(t *testing.T) {
	srv := asset.NewServer(asset.Options{FS: fstest.MapFS{}, Workers: 1})
	defer srv.Close()
	mustPanic(t, "input: RegisterLoaders: nil Actions", func() { RegisterLoaders(srv, nil) })
	mustPanic(t, "input: RegisterLoaders: Actions not frozen", func() { RegisterLoaders(srv, new(input.Actions)) })
}

func TestInputSystemPublishesAndWatches(t *testing.T) {
	a := newActs()
	srv := asset.NewServer(asset.Options{FS: fstest.MapFS{
		"input/bindings.json": {Data: []byte(bindings)},
		"input/bad.json":      {Data: []byte(`{"version": 1, "mappings": {"gameplay": {"bindings": {"fly": []}}}}`)},
	}, Workers: 1})
	defer srv.Close()
	RegisterLoaders(srv, a.Actions)
	h, err := LoadBindingsSync(srv.Global(), "input/bindings.json")
	if err != nil {
		t.Fatal(err)
	}

	tw := newWorld(t, a)
	tw.sys.Watch(srv, h)
	tw.src.Frame.Keys.Set(input.KeySpace)
	tw.tick()
	if ecs.GetResource[input.Router](tw.w) != tw.r || ecs.GetResource[input.Events](tw.w) != tw.r.Events() {
		t.Fatal("resources not published")
	}
	if !tw.r.Bindings().HasMapping("gameplay") {
		t.Fatal("watched bindings were not applied")
	}
	tw.tick() // Space was down when the mapping was first reached: fresh-press rule
	tw.src.Frame.Keys.Clear(input.KeySpace)
	tw.tick()
	tw.src.Frame.Keys.Set(input.KeySpace)
	tw.tick()
	if !tw.r.Player(0).State().JustPressed(a.jump) {
		t.Fatal("jump not pressed")
	}

	var got []error
	tw.sys.OnError = func(err error) { got = append(got, err) }
	bad, _ := LoadBindingsSync(srv.Global(), "input/bad.json")
	tw.sys.Watch(srv, bad)
	tw.tick()
	tw.tick()
	if len(got) != 1 || !strings.Contains(errors.Join(got...).Error(), `unknown action "fly"`) {
		t.Fatalf("errors = %v", got)
	}
	if !tw.r.Bindings().HasMapping("gameplay") {
		t.Fatal("a failed file replaced the bindings")
	}
}

func TestTouchControlsExtract(t *testing.T) {
	a := newActs()
	b, err := input.ParseBindings([]byte(bindings), a.Actions)
	if err != nil {
		t.Fatal(err)
	}
	ex := NewTouchControlsExtract()
	tw := newWorld(t, a, ex)
	tw.r.SetBindings(b)
	q := ecs.GetResource[render.Queue](tw.w)

	tw.tick()
	if q.Len() != 0 {
		t.Fatalf("drew %d items before the first touch", q.Len())
	}

	f := &tw.src.Frame
	f.Touches[0] = input.TouchFrame{ID: 1, X: 50, Y: 150}
	f.NTouches = 1
	tw.tick()
	if q.Len() != 3 { // stick base and knob, jump button
		t.Fatalf("items = %d, want 3", q.Len())
	}
	for i := range q.Len() {
		it := q.At(i)
		if it.Kind != render.KindCircle || it.Space != render.ScreenSpace || it.Layer != TouchLayer {
			t.Fatalf("item %d = %+v", i, it)
		}
	}
	q.Reset()

	// A key press makes the keyboard the active device: controls hide.
	f.NTouches = 0
	f.Keys.Set(input.KeySpace)
	tw.tick()
	if q.Len() != 0 {
		t.Fatalf("drew %d items while the keyboard is active", q.Len())
	}
	ex.AlwaysShow = true
	tw.tick()
	if q.Len() != 3 {
		t.Fatalf("AlwaysShow: items = %d", q.Len())
	}
}
