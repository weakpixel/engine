package game

import (
	"testing"
	"testing/fstest"

	"github.com/weakpixel/ecs"
	"github.com/weakpixel/ecs/system"
	"github.com/weakpixel/engine/input"
)

const testBindings = `{"version": 1, "mappings": {"gameplay": {"bindings": {"jump": [{"control": "key:Space"}]}}}}`

func TestEngineInput(t *testing.T) {
	acts := &input.Actions{}
	jump := acts.Button("jump", "Jump.")
	src := &input.ManualSource{}
	e := New(Config{
		NoAudio: true,
		FS:      fstest.MapFS{"input/bindings.json": {Data: []byte(testBindings)}},
		Input:   InputConfig{Actions: acts, Bindings: "input/bindings.json", Source: src},
	})
	defer e.Close()
	w := e.World()
	if ecs.GetResource[input.Router](w) != e.Input() || ecs.GetResource[input.Events](w) != e.Input().Events() {
		t.Fatal("input resources missing")
	}
	e.Input().Player(0).Push("gameplay")

	// A game input system sees the state the engine computed this tick.
	var pressedTicks int
	e.AddSystem(StageInput, func(ctx *system.Context) {
		if ecs.GetResource[input.Router](ctx.World).Player(0).State().JustPressed(jump) {
			pressedTicks++
		}
	})
	tickUntil(t, e, func() bool { return e.Input().Bindings().HasMapping("gameplay") }, func() {})
	e.Update() // the mapping is reached: arms its bindings
	src.Frame.Keys.Set(input.KeySpace)
	e.Update()
	e.Update()
	if pressedTicks != 1 {
		t.Fatalf("jump pressed on %d ticks, want 1", pressedTicks)
	}
}

func TestEngineInputDefaults(t *testing.T) {
	e := New(Config{NoAudio: true, Input: InputConfig{Source: &input.ManualSource{}, HideTouchControls: true}})
	defer e.Close()
	if e.Input().Actions().Len() != 0 || e.Input().Players() != 1 {
		t.Fatal("default router")
	}
	if err := e.Update(); err != nil {
		t.Fatal(err)
	}
}
