package game

import (
	"image"
	"math"
	"testing"
	"testing/fstest"
	"time"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/weakpixel/ecs/system"
	"github.com/weakpixel/engine/input"
)

// countingSource wraps a Source and counts Polls, to see whether the
// engine's input system ran.
type countingSource struct {
	input.Source
	polls int
}

func (c *countingSource) Poll(f *input.Frame, ch *input.DeviceChanges) {
	c.polls++
	c.Source.Poll(f, ch)
}

// testTool returns the modes in script, one per Tick (TickRun once they run
// out), and records the calls the engine makes.
type testTool struct {
	script []TickMode
	inits  int
	inputs int
	ticks  int
	src    *countingSource
}

func (t *testTool) Init(e *Engine) { t.inits++ }

func (t *testTool) Input(src input.Source) input.Source {
	t.inputs++
	t.src = &countingSource{Source: src}
	return t.src
}

func (t *testTool) Tick(e *Engine) TickMode {
	t.ticks++
	if len(t.script) == 0 {
		return TickRun
	}
	m := t.script[0]
	t.script = t.script[1:]
	return m
}

func (t *testTool) DrawFinal(e *Engine, screen ebiten.FinalScreen) {}

// toolRunner counts Init and Run calls of a stateful system.
type toolRunner struct{ inits, runs int }

func (r *toolRunner) Init(*system.Context) { r.inits++ }
func (r *toolRunner) Run(*system.Context)  { r.runs++ }

func TestToolPauseAndStep(t *testing.T) {
	tool := &testTool{script: []TickMode{TickRun, TickPause, TickPause, TickStep, TickPause}}
	e := New(Config{NoAudio: true, Tool: tool, Input: InputConfig{Source: &input.ManualSource{}}})
	defer e.Close()
	if tool.inits != 1 || tool.inputs != 1 {
		t.Fatalf("Init %d, Input %d times; want once each", tool.inits, tool.inputs)
	}

	counts := map[Stage]int{}
	for _, st := range []Stage{StageInput, StageSimulate, StageCamera, StagePost} {
		e.AddSystem(st, func(*system.Context) { counts[st]++ })
	}
	stateful := &toolRunner{}
	e.AddRunner(StageSimulate, stateful)

	wantPaused := []bool{false, true, true, false, true}
	wantRuns := []int{1, 1, 1, 2, 2}
	for i := range wantRuns {
		if err := e.Update(); err != nil {
			t.Fatal(err)
		}
		if e.Paused() != wantPaused[i] {
			t.Fatalf("tick %d: Paused = %v", i, e.Paused())
		}
		for st, n := range counts {
			if n != wantRuns[i] {
				t.Fatalf("tick %d: stage %v ran %d times, want %d", i, st, n, wantRuns[i])
			}
		}
		if tool.src.polls != wantRuns[i] {
			t.Fatalf("tick %d: input system polled %d times, want %d", i, tool.src.polls, wantRuns[i])
		}
	}
	if stateful.inits != 1 || stateful.runs != 2 {
		t.Fatalf("wrapped runner: Init %d, Run %d; want 1 and 2", stateful.inits, stateful.runs)
	}
	if tool.ticks != 5 {
		t.Fatalf("Tick called %d times", tool.ticks)
	}
	if e.elapsed != 5*(time.Second/time.Duration(e.tps())) {
		t.Fatalf("simulation time stopped while paused: %v", e.elapsed)
	}
}

// TestToolPausedEngineStagesRun: assets keep loading while paused.
func TestToolPausedEngineStagesRun(t *testing.T) {
	acts := &input.Actions{}
	acts.Button("jump", "Jump.")
	tool := &testTool{}
	for range 1000 {
		tool.script = append(tool.script, TickPause)
	}
	e := New(Config{
		NoAudio: true, Tool: tool,
		FS:    fstest.MapFS{"input/bindings.json": {Data: []byte(testBindings)}},
		Input: InputConfig{Actions: acts, Bindings: "input/bindings.json", Source: &input.ManualSource{}},
	})
	defer e.Close()
	sim := 0
	e.AddSystem(StageSimulate, func(*system.Context) { sim++ })
	// The bindings file loads in the assets stage, which runs while paused.
	if _, total := e.Assets().Global().Progress(); total == 0 {
		t.Fatal("bindings not requested")
	}
	tickUntil(t, e, func() bool { return e.Assets().Global().Done() }, func() {})
	if sim != 0 {
		t.Fatalf("a game system ran %d times while paused", sim)
	}
}

func TestNoToolUnchanged(t *testing.T) {
	e := New(Config{NoAudio: true, Input: InputConfig{Source: &input.ManualSource{}}})
	defer e.Close()
	ran := 0
	e.AddSystem(StageSimulate, func(*system.Context) { ran++ })
	e.Update()
	e.Update()
	if ran != 2 || e.Paused() {
		t.Fatalf("ran %d, paused %v", ran, e.Paused())
	}
}

func TestTickModeString(t *testing.T) {
	for m, want := range map[TickMode]string{TickRun: "run", TickPause: "pause", TickStep: "step", 9: "invalid"} {
		if m.String() != want {
			t.Fatalf("%d.String() = %q", m, m.String())
		}
	}
}

func TestWindowToLogical(t *testing.T) {
	e := New(Config{NoAudio: true})
	defer e.Close()
	near := func(got, want float32) bool { return math.Abs(float64(got-want)) < 1e-4 }

	if got := e.WindowToLogical(image.Pt(7, 9)); got.X != 7 || got.Y != 9 {
		t.Fatalf("identity before the first draw: %v", got)
	}
	for _, tc := range []struct {
		name          string
		scale, dx, dy float64
		in            image.Point
		wantX, wantY  float32
	}{
		{"integer scale", 3, 0, 0, image.Pt(30, 60), 10, 20},
		{"integer scale with letterbox", 2, 40, 10, image.Pt(140, 110), 50, 50},
		{"fractional scale", 2.5, 12.5, 0, image.Pt(37, 25), 9.8, 10},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var g ebiten.GeoM
			g.Scale(tc.scale, tc.scale)
			g.Translate(tc.dx, tc.dy)
			e.finalGeoM = g
			got := e.WindowToLogical(tc.in)
			if !near(got.X, tc.wantX) || !near(got.Y, tc.wantY) {
				t.Fatalf("WindowToLogical(%v) = %v, want (%v, %v)", tc.in, got, tc.wantX, tc.wantY)
			}
			back := e.LogicalToWindow(got)
			if !near(back.X, float32(tc.in.X)) || !near(back.Y, float32(tc.in.Y)) {
				t.Fatalf("LogicalToWindow(%v) = %v, want %v", got, back, tc.in)
			}
		})
	}
}

// editRunner is a toolRunner that opts in to running while paused.
type editRunner struct {
	toolRunner
	edit bool
}

func (r *editRunner) RunsWhilePaused() bool { return r.edit }

// TestToolEditRunnerRunsWhilePaused: only runners that opt in keep running
// while a Tool pauses, in their original order among the other runners.
func TestToolEditRunnerRunsWhilePaused(t *testing.T) {
	tool := &testTool{script: []TickMode{TickRun, TickPause, TickPause}}
	e := New(Config{NoAudio: true, Tool: tool, Input: InputConfig{Source: &input.ManualSource{}}})
	defer e.Close()

	var order []string
	e.AddSystem(StageSimulate, func(*system.Context) { order = append(order, "plain") })
	optIn := &editRunner{edit: true}
	optOut := &editRunner{edit: false}
	e.AddRunner(StageSimulate, optIn)
	e.AddRunner(StageSimulate, optOut)

	for range 3 {
		if err := e.Update(); err != nil {
			t.Fatal(err)
		}
	}
	if optIn.runs != 3 || optIn.inits != 1 {
		t.Fatalf("opted-in runner: Init %d, Run %d; want 1 and 3", optIn.inits, optIn.runs)
	}
	if optOut.runs != 1 || optOut.inits != 1 {
		t.Fatalf("opted-out runner: Init %d, Run %d; want 1 and 1", optOut.inits, optOut.runs)
	}
	if len(order) != 1 {
		t.Fatalf("plain system ran %d times, want 1", len(order))
	}
}
