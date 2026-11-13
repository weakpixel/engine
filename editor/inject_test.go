package editor

import (
	"testing"

	"github.com/weakpixel/ecs/system"
	"github.com/weakpixel/engine/game"
	"github.com/weakpixel/engine/input"
)

// injectFixture is a session on an engine whose router binds "jump" to
// Space and the pad's south button; jumps counts the ticks it was down.
func injectFixture(t *testing.T) (f *fixture, jumps *int) {
	t.Helper()
	acts := &input.Actions{}
	jump := acts.Button("jump", "Jump.")
	acts.Freeze()
	f = &fixture{t: t, src: &input.ManualSource{}}
	f.s = New(Config{Path: "test.level.json", Events: func(ev Event) { f.events = append(f.events, ev) }})
	f.e = game.New(game.Config{
		NoAudio: true, Width: screenW, Height: screenH, FS: testFiles(t), Tool: f.s,
		Input: game.InputConfig{Source: f.src, Actions: acts},
	})
	t.Cleanup(f.e.Close)
	b, err := input.ParseBindings([]byte(`{"version":1,"mappings":{"play":{"bindings":{"jump":[{"control":"key:Space"},{"control":"pad:south"}]}}}}`), acts)
	if err != nil {
		t.Fatal(err)
	}
	r := f.e.Input()
	r.SetBindings(b)
	r.Player(0).SetMappings("play")
	jumps = new(int)
	f.e.AddSystem(game.StageSimulate, func(*system.Context) {
		if r.Player(0).State().Down(jump) {
			*jumps++
		}
	})
	f.open(doc(visual("a", 0, 0, 0)))
	return f, jumps
}

func TestInject(t *testing.T) {
	f, jumps := injectFixture(t)

	// Edit mode ignores it.
	f.s.Inject(Injection{Keys: []input.Key{input.KeySpace}, Ticks: 5})
	f.s.Step()
	f.update()
	f.update()
	if *jumps != 0 {
		t.Fatalf("an injection in edit mode reached the game (%d ticks)", *jumps)
	}

	f.s.Play()
	f.s.Inject(Injection{Keys: []input.Key{input.KeySpace}, Ticks: 3})
	for range 6 {
		f.update()
	}
	if *jumps != 3 {
		t.Errorf("Space held for 3 ticks: jump down %d ticks", *jumps)
	}

	// The virtual pad connects, is given to player 0 and presses south.
	*jumps = 0
	pad := &input.PadFrame{}
	pad.SetDown(input.PadSouth, true)
	f.s.Inject(Injection{Pad: pad, Ticks: 2})
	for range 4 {
		f.update()
	}
	if *jumps != 2 {
		t.Errorf("pad south held for 2 ticks: jump down %d ticks", *jumps)
	}
	if _, ok := f.e.Input().Device(InjectedPad); !ok {
		t.Error("the injected pad is not a device of the router")
	}

	// The mouse moves to the given position while the injection lasts.
	f.s.Inject(Injection{Mouse: &input.MouseFrame{X: 12, Y: 34}, Ticks: 1})
	f.update()
	if m := f.e.Input().Frame().Mouse; m.X != 12 || m.Y != 34 {
		t.Errorf("mouse at %v,%v, want 12,34", m.X, m.Y)
	}

	// Paused Play reads no input: the ticks wait for a step.
	*jumps = 0
	f.s.Pause(true)
	f.s.Inject(Injection{Keys: []input.Key{input.KeySpace}, Ticks: 2})
	f.update()
	f.update()
	if *jumps != 0 {
		t.Fatalf("a paused game read input (%d ticks)", *jumps)
	}
	f.s.Step()
	f.update()
	f.update()
	if *jumps != 1 {
		t.Errorf("one step: jump down %d ticks, want 1", *jumps)
	}

	// Stop drops what is left.
	f.s.Stop()
	f.s.Play()
	*jumps = 0
	f.update()
	if *jumps != 0 {
		t.Errorf("an injection survived Stop (%d ticks)", *jumps)
	}
}

func TestPause(t *testing.T) {
	f := newFixture(t, Config{})
	f.open(doc(visual("a", 0, 0, 0)))
	runs := 0
	f.e.AddSystem(game.StageSimulate, func(*system.Context) { runs++ })

	// Pause does nothing in edit mode.
	f.s.Pause(true)
	if f.s.Paused() {
		t.Fatal("paused in edit mode")
	}
	f.s.Play()
	f.update()
	f.events = nil
	f.s.Pause(true)
	f.s.Pause(true) // no second event
	if len(f.events) != 1 || f.events[0] != (Mode{Play: true, Paused: true}) {
		t.Errorf("Pause events %v", f.events)
	}
	before := runs
	f.update()
	f.update()
	if runs != before {
		t.Fatalf("a paused game ran %d ticks", runs-before)
	}
	f.s.Step()
	f.update()
	f.update()
	if runs != before+1 {
		t.Errorf("Step while paused ran %d ticks, want 1", runs-before)
	}
	f.s.Pause(false)
	f.update()
	if runs != before+2 {
		t.Errorf("resumed: %d ticks, want 2", runs-before)
	}
	f.s.Pause(true)
	f.s.Stop()
	if f.s.Paused() || f.s.Playing() {
		t.Error("Stop left the session paused or playing")
	}
	f.s.Play()
	if f.s.Paused() {
		t.Error("Play started paused")
	}
}
