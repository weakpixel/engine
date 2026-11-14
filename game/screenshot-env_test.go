package game

import (
	"slices"
	"testing"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/weakpixel/engine/input"
)

func envOf(m map[string]string) func(string) string { return func(k string) string { return m[k] } }

func TestParseShotEnv(t *testing.T) {
	cfg, env, errs := parseShotEnv(ScreenshotConfig{Dir: "shots"}, envOf(map[string]string{
		shotKeyEnv: "F12", shotDirEnv: "out", shotAtEnv: "60, 1,60", shotQuitEnv: "1",
	}))
	if len(errs) != 0 || cfg.Key != input.KeyF12 || cfg.Dir != "out" || !env.quit || !slices.Equal(env.at, []int{1, 60}) {
		t.Fatalf("got %+v %+v %v", cfg, env, errs)
	}

	// A key set in code wins over the variable; bad values are reported
	// and skipped, good ones still apply.
	cfg, env, errs = parseShotEnv(ScreenshotConfig{Key: input.KeyF11}, envOf(map[string]string{
		shotKeyEnv: "Nope", shotAtEnv: "x,0,5",
	}))
	if cfg.Key != input.KeyF11 || !slices.Equal(env.at, []int{5}) || len(errs) != 2 {
		t.Fatalf("got %+v %+v %v", cfg, env, errs)
	}
	cfg, _, errs = parseShotEnv(ScreenshotConfig{}, envOf(map[string]string{shotKeyEnv: "Nope"}))
	if cfg.Key != 0 || len(errs) != 1 {
		t.Fatalf("bad key: %+v %v", cfg, errs)
	}
}

func TestShotHotkey(t *testing.T) {
	src := &input.ManualSource{}
	e := New(Config{NoAudio: true, Screenshots: ScreenshotConfig{Key: input.KeyF12, KeyTarget: ShotWindow},
		Input: InputConfig{Source: src, HideTouchControls: true}})
	defer e.Close()
	queued := func() int { return len(e.shots.queue) }
	e.Update()
	if queued() != 0 {
		t.Fatal("request without a key press")
	}
	src.Frame.Keys.Set(input.KeyF12)
	e.Update()
	e.Update() // held: no repeat
	if queued() != 1 || e.shots.queue[0].Target != ShotWindow {
		t.Fatalf("queued %d after one press", queued())
	}
	src.Frame.Keys.Clear(input.KeyF12)
	e.Update()
	src.Frame.Keys.Set(input.KeyF12)
	e.Update()
	if queued() != 2 {
		t.Fatalf("queued %d after two presses", queued())
	}
}

// pausedTool pauses the game every tick.
type pausedTool struct{}

func (pausedTool) Init(*Engine)                          {}
func (pausedTool) Input(src input.Source) input.Source   { return src }
func (pausedTool) DrawFinal(*Engine, ebiten.FinalScreen) {}

func (pausedTool) Tick(*Engine) TickMode { return TickPause }

func TestShotHotkeyWhilePaused(t *testing.T) {
	src := &input.ManualSource{}
	e := New(Config{NoAudio: true, Tool: pausedTool{}, Screenshots: ScreenshotConfig{Key: input.KeyF12},
		Input: InputConfig{Source: src, HideTouchControls: true}})
	defer e.Close()
	e.Update()
	src.Frame.Keys.Set(input.KeyF12)
	e.Update()
	if len(e.shots.queue) != 1 {
		t.Fatalf("queued %d while paused", len(e.shots.queue))
	}
}

func TestShotAtSchedule(t *testing.T) {
	e := New(Config{NoAudio: true, Input: InputConfig{Source: &input.ManualSource{}, HideTouchControls: true}})
	defer e.Close()
	e.shots.env = shotEnv{at: []int{2, 4}, quit: true}
	var got []int
	for i := 1; i <= 5; i++ {
		e.Update()
		got = append(got, len(e.shots.queue))
		clear(e.shots.queue)
		e.shots.queue = e.shots.queue[:0]
	}
	if !slices.Equal(got, []int{0, 1, 0, 1, 0}) {
		t.Fatalf("queued per tick = %v", got)
	}
}
