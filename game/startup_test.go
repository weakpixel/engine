package game

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/png"
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/weakpixel/ecs/system"
	"github.com/weakpixel/engine/input"
	"github.com/weakpixel/engine/tilemap"
)

const msec = time.Millisecond

// runSeq ticks s with dt until it is done (at most limit ticks) and returns
// the elapsed total; loadedAt is the total time at which loading finishes,
// pressAt the totals at which a press happens.
func runSeq(t *testing.T, s *startupSeq, dt, loadedAt time.Duration, pressAt ...time.Duration) time.Duration {
	t.Helper()
	var total time.Duration
	for range 10000 {
		if s.done {
			return total
		}
		total += dt
		pressed := false
		for _, p := range pressAt {
			pressed = pressed || p == total
		}
		s.tick(dt, total >= loadedAt, pressed)
	}
	t.Fatal("sequence never ended")
	return 0
}

func TestStartupSeqTiming(t *testing.T) {
	tests := []struct {
		name     string
		durs     []time.Duration
		fades    []time.Duration
		noSkip   bool
		loadedAt time.Duration
		pressAt  []time.Duration
		want     time.Duration
	}{
		{"fast load ends at Duration", []time.Duration{5000 * msec}, []time.Duration{250 * msec}, false, 0, nil, 5000 * msec},
		{"slow load holds, then fades out", []time.Duration{5000 * msec}, []time.Duration{250 * msec}, false, 8000 * msec, nil, 8250 * msec},
		{"three screens advance on time", []time.Duration{1000 * msec, 2000 * msec, 500 * msec}, []time.Duration{100 * msec, 100 * msec, 100 * msec}, false, 0, nil, 3500 * msec},
		{"later load holds only the last screen", []time.Duration{1000 * msec, 1000 * msec}, []time.Duration{0, 0}, false, 3000 * msec, nil, 3000 * msec},
		{"no fade", []time.Duration{1000 * msec}, []time.Duration{0}, false, 0, nil, 1000 * msec},
		{"skip ends a screen early", []time.Duration{5000 * msec}, []time.Duration{250 * msec}, false, 0, []time.Duration{1000 * msec}, 1250 * msec},
		{"skip on an earlier screen ignores loading", []time.Duration{5000 * msec, 1000 * msec}, []time.Duration{0, 0}, false, 9000 * msec, []time.Duration{500 * msec}, 9000 * msec},
		{"skip on the last screen waits for the load", []time.Duration{5000 * msec}, []time.Duration{0}, false, 3000 * msec, []time.Duration{1000 * msec}, 3000 * msec},
		{"NoSkip ignores presses", []time.Duration{2000 * msec}, []time.Duration{0}, true, 0, []time.Duration{500 * msec}, 2000 * msec},
		{"no screens is done at once", nil, nil, false, 0, nil, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			s := newStartupSeq(tt.durs, tt.fades, tt.noSkip)
			if got := runSeq(t, s, 50*msec, tt.loadedAt, tt.pressAt...); got != tt.want {
				t.Fatalf("ended after %v, want %v", got, tt.want)
			}
		})
	}
}

func TestStartupSeqAlpha(t *testing.T) {
	s := newStartupSeq([]time.Duration{1000 * msec}, []time.Duration{200 * msec}, false)
	want := map[int]float32{1: 0.5, 2: 1, 10: 1, 17: 0.5}
	for i := 1; !s.done; i++ {
		s.tick(100*msec, true, false)
		if w, ok := want[i]; ok && !s.done && s.alpha() != w {
			t.Fatalf("tick %d: alpha %v, want %v", i, s.alpha(), w)
		}
	}
	if s.alpha() != 0 {
		t.Fatalf("alpha after the end = %v, want 0", s.alpha())
	}

	// A skip during the fade in fades out from where it was.
	s = newStartupSeq([]time.Duration{1000 * msec}, []time.Duration{200 * msec}, false)
	s.tick(100*msec, true, true)
	if a := s.alpha(); a != 0.5 {
		t.Fatalf("alpha right after the skip = %v, want 0.5", a)
	}
	s.tick(100*msec, true, false)
	if a := s.alpha(); a != 0.5 {
		t.Fatalf("alpha while fading out = %v, want 0.5", a)
	}
}

func pngBytes(t *testing.T) []byte {
	t.Helper()
	var b bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, 4, 2))
	img.Set(0, 0, color.White)
	if err := png.Encode(&b, img); err != nil {
		t.Fatal(err)
	}
	return b.Bytes()
}

func TestStartupScreens(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	draw := func(*ebiten.Image, StartupState) {}
	for _, sc := range []StartupScreen{{}, {Image: img, Path: "a.png"}, {Image: img, Draw: draw}} {
		mustPanic(t, "game: startup screen 0: set one of Image, Path or Draw", func() {
			startupScreens(StartupConfig{Screens: []StartupScreen{sc}}, nil, nil)
		})
	}

	def := startupScreens(StartupConfig{}, nil, nil)
	if len(def) != 1 || def[0].Image == nil || def[0].Duration != 5*time.Second || def[0].Fade != 250*msec {
		t.Fatalf("default screens = %+v", def)
	}
	if b := def[0].Image.Bounds(); b.Dx() != 720 || b.Dy() != 720 {
		t.Fatalf("default image is %v", b)
	}

	var errs []error
	fsys := fstest.MapFS{"global/logo.png": {Data: pngBytes(t)}, "bad.png": {Data: []byte("nope")}}
	got := startupScreens(StartupConfig{MinDuration: 2 * time.Second, Screens: []StartupScreen{
		{Path: "global/logo.png", Fade: -1},
		{Path: "missing.png"},
		{Path: "bad.png"},
		{Draw: draw, Duration: time.Second, Background: color.White},
	}}, fsys, func(err error) { errs = append(errs, err) })
	if len(got) != 2 {
		t.Fatalf("got %d screens, want the 2 that load", len(got))
	}
	if got[0].Image == nil || got[0].Image.Bounds().Dx() != 4 || got[0].Duration != 2*time.Second || got[0].Fade != 0 || got[0].Background != color.Black {
		t.Fatalf("path screen = %+v", got[0])
	}
	if got[1].Duration != time.Second || got[1].Background != color.White {
		t.Fatalf("draw screen = %+v", got[1])
	}
	if len(errs) != 2 || !strings.Contains(errs[0].Error(), "game: startup screen 1: ") ||
		!errors.Is(errs[0], fs.ErrNotExist) || !strings.Contains(errs[1].Error(), "game: startup screen 2: bad.png") {
		t.Fatalf("errors = %v", errs)
	}
}

// newStartupEngine is an engine with the startup screen on (cfg.Startup,
// or the default) and a game
// system that counts its runs.
func newStartupEngine(t *testing.T, cfg Config) (e *Engine, src *input.ManualSource, runs *int) {
	t.Helper()
	src = &input.ManualSource{}
	cfg.NoAudio = true
	cfg.Input.Source = src
	if cfg.Startup == nil {
		cfg.Startup = &StartupConfig{}
	}
	e = New(cfg)
	t.Cleanup(e.Close)
	runs = new(int)
	e.AddSystem(StageSimulate, func(*system.Context) { *runs++ })
	return e, src, runs
}

func TestStartupHoldsTheGame(t *testing.T) {
	e, _, runs := newStartupEngine(t, Config{})
	if !e.StartupActive() {
		t.Fatal("&StartupConfig{} does not turn the startup screen on")
	}
	ticks := 0
	for e.StartupActive() {
		if err := e.Update(); err != nil {
			t.Fatal(err)
		}
		ticks++
		if ticks > 1000 {
			t.Fatal("the startup screen never ended")
		}
	}
	// Five seconds (to a tick or two of rounding, 1s/60 is not exact in
	// nanoseconds); the game runs in the tick the screen ends.
	dt := time.Second / time.Duration(e.tps())
	if got := time.Duration(ticks) * dt; got < 5*time.Second || got > 5*time.Second+2*dt || *runs != 1 {
		t.Fatalf("screen ended after %v with %d game runs, want 5s and 1", got, *runs)
	}
	if e.held {
		t.Fatal("still held after the end")
	}
}

func TestStartupNilOrTool(t *testing.T) {
	for name, cfg := range map[string]Config{
		"nil":  {},
		"tool": {Tool: &testTool{}, Startup: &StartupConfig{}},
	} {
		t.Run(name, func(t *testing.T) {
			cfg.NoAudio = true
			cfg.Input.Source = &input.ManualSource{}
			e := New(cfg)
			defer e.Close()
			runs := 0
			e.AddSystem(StageSimulate, func(*system.Context) { runs++ })
			if e.StartupActive() {
				t.Fatal("startup screen active")
			}
			e.Update()
			if runs != 1 {
				t.Fatalf("game ran %d times, want 1", runs)
			}
		})
	}
}

func TestStartupSkip(t *testing.T) {
	e, src, runs := newStartupEngine(t, Config{Startup: &StartupConfig{MinDuration: time.Hour, Screens: []StartupScreen{
		{Image: image.NewRGBA(image.Rect(0, 0, 1, 1)), Fade: -1},
		{Image: image.NewRGBA(image.Rect(0, 0, 1, 1)), Fade: -1},
	}}})
	// A key held from launch does not skip until it is released.
	src.Frame.Keys.Set(input.KeySpace)
	for range 3 {
		e.Update()
	}
	if e.startup.seq.i != 0 {
		t.Fatal("a key held from launch skipped")
	}
	press := func(set func(f *input.Frame)) {
		src.Frame = input.Frame{}
		e.Update()
		set(&src.Frame)
		e.Update() // the router sees the press
		e.Update() // the startup screen sees it
	}
	press(func(f *input.Frame) { f.Mouse.SetDown(input.MouseLeft, true) })
	if !e.StartupActive() || e.startup.seq.i != 1 {
		t.Fatal("a click did not skip to the second screen")
	}
	press(func(f *input.Frame) { f.Keys.Set(input.KeyEnter) })
	if e.StartupActive() {
		t.Fatal("a key did not skip the last screen")
	}
	if *runs != 1 {
		t.Fatalf("game ran %d times, want 1 (the tick the screen ended)", *runs)
	}
}

func TestStartupNoSkip(t *testing.T) {
	e, src, _ := newStartupEngine(t, Config{Startup: &StartupConfig{NoSkip: true, MinDuration: time.Hour}})
	e.Update()
	src.Frame.Keys.Set(input.KeyEnter)
	for range 5 {
		e.Update()
	}
	if !e.StartupActive() {
		t.Fatal("NoSkip screen was skipped")
	}
}

// slowFS delays every Open, so loads are still running when the minimum
// time is over.
type slowFS struct {
	fs.FS
	delay time.Duration
}

func (s slowFS) Open(name string) (fs.File, error) {
	time.Sleep(s.delay)
	return s.FS.Open(name)
}

func TestStartupWaitsForLevels(t *testing.T) {
	var l *Level
	e, _, runs := newStartupEngine(t, Config{
		FS:      slowFS{FS: mapFiles(t), delay: 20 * msec},
		Startup: &StartupConfig{MinDuration: time.Nanosecond, Screens: []StartupScreen{{Image: image.NewRGBA(image.Rect(0, 0, 1, 1)), Fade: -1}}},
	})
	e.Start(func(e *Engine) {
		var err error
		if l, err = e.LoadMap("m.tmj", tilemap.SpawnOptions{}); err != nil {
			t.Fatal(err)
		}
	})
	e.Update()
	if !e.StartupActive() || *runs != 0 {
		t.Fatal("the screen ended before the level loaded")
	}
	tickUntil(t, e, func() bool { return !e.StartupActive() }, func() {
		if e.StartupActive() && l.Spawned() && *runs > 0 {
			t.Fatal("a game system ran while the screen was up")
		}
	})
	if !l.Spawned() {
		t.Fatal("the screen ended before the level spawned")
	}
}

func TestStartupFailedAssetEnds(t *testing.T) {
	var errs []error
	e, _, _ := newStartupEngine(t, Config{
		FS:           fstest.MapFS{},
		OnAssetError: func(err error) { errs = append(errs, err) },
		Startup:      &StartupConfig{MinDuration: time.Nanosecond, Screens: []StartupScreen{{Image: image.NewRGBA(image.Rect(0, 0, 1, 1)), Fade: -1}}},
	})
	e.Start(func(e *Engine) {
		if _, err := e.LoadMap("missing.tmj", tilemap.SpawnOptions{}); err != nil {
			t.Fatal(err)
		}
	})
	tickUntil(t, e, func() bool { return !e.StartupActive() }, func() {})
	if len(errs) == 0 {
		t.Fatal("the failed map was not reported")
	}
}

func TestStartupReady(t *testing.T) {
	ready := false
	e, _, _ := newStartupEngine(t, Config{Startup: &StartupConfig{
		MinDuration: time.Nanosecond,
		Ready:       func(*Engine) bool { return ready },
		Screens:     []StartupScreen{{Image: image.NewRGBA(image.Rect(0, 0, 1, 1)), Fade: -1}},
	}})
	for range 10 {
		e.Update()
	}
	if !e.StartupActive() {
		t.Fatal("ended while Ready returned false")
	}
	ready = true
	e.Update()
	if e.StartupActive() {
		t.Fatal("did not end once Ready returned true")
	}
}

func TestTickNoAllocsAfterStartup(t *testing.T) {
	if testing.CoverMode() != "" || raceEnabled {
		t.Skip("instrumentation allocates")
	}
	e, _, _ := newStartupEngine(t, Config{Width: 64, Height: 32, Startup: &StartupConfig{MinDuration: time.Nanosecond, Screens: []StartupScreen{{Image: image.NewRGBA(image.Rect(0, 0, 1, 1)), Fade: -1}}}})
	for range 3 {
		e.Update()
	}
	if e.StartupActive() {
		t.Fatal("startup screen still up")
	}
	if n := testing.AllocsPerRun(20, func() { e.Update() }); n != 0 {
		t.Fatalf("allocs per tick = %v, want 0", n)
	}
}

func TestFitOptions(t *testing.T) {
	op := fitOptions(200, 100, 400, 400)
	x, y := op.GeoM.Apply(0, 0)
	x2, y2 := op.GeoM.Apply(200, 100)
	if x != 0 || y != 100 || x2 != 400 || y2 != 300 {
		t.Fatalf("fit maps the image to (%v,%v)-(%v,%v)", x, y, x2, y2)
	}
}
