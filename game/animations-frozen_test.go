package game

import (
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/weakpixel/ecs"
	"github.com/weakpixel/ecs/system"
	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/input"
	"github.com/weakpixel/engine/particles/fxfile"
	"github.com/weakpixel/engine/render"
)

// deltaRunner records the delta of its last Run and whether Init ran.
type deltaRunner struct {
	delta time.Duration
	init  bool
}

func (d *deltaRunner) Init(*system.Context) { d.init = true }
func (d *deltaRunner) Run(ctx *system.Context) {
	d.delta = ctx.Time.Delta
}

func TestStillClock(t *testing.T) {
	for _, tc := range []struct {
		name   string
		frozen bool
		want   time.Duration
	}{
		{"running", false, time.Second / 60},
		{"frozen", true, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			inner := &deltaRunner{}
			on := tc.frozen
			s := &stillClock{inner: inner, on: &on}
			ctx := &system.Context{Time: system.Time{Delta: time.Second / 60}}
			s.Init(ctx)
			s.Run(ctx)
			if !inner.init {
				t.Error("Init was not passed on")
			}
			if inner.delta != tc.want {
				t.Errorf("delta = %v, want %v", inner.delta, tc.want)
			}
			if ctx.Time.Delta != time.Second/60 {
				t.Error("the stage's context lost its delta")
			}
		})
	}
}

// TestAnimationsFrozenParticles: frozen particles keep their count and
// place and emit nothing; they move again once unfrozen.
func TestAnimationsFrozenParticles(t *testing.T) {
	e := New(Config{NoAudio: true, Width: 64, Height: 32, Input: InputConfig{Source: &input.ManualSource{}},
		FS: fxFiles(fxfile.Emitter{Rate: 120, LifeMin: 10, LifeMax: 10, SpeedMin: 10, SpeedMax: 20})})
	defer e.Close()
	ent := spawnEmitter(t, e)
	update := func(n int) {
		t.Helper()
		for range n {
			if err := e.Update(); err != nil {
				t.Fatal(err)
			}
		}
	}
	update(5)
	e.SetAnimationsFrozen(true)
	if !e.AnimationsFrozen() {
		t.Fatal("AnimationsFrozen is false after SetAnimationsFrozen(true)")
	}
	update(1)
	live := ecs.GetComp[gfx.Particles](e.World(), ent).Live
	first := *e.queue.At(0)
	update(30)
	if got := ecs.GetComp[gfx.Particles](e.World(), ent).Live; got != live {
		t.Errorf("Live = %d after 30 frozen ticks, want %d", got, live)
	}
	if got := *e.queue.At(0); got != first {
		t.Errorf("a frozen particle moved: %+v, was %+v", got.Pos, first.Pos)
	}
	e.SetAnimationsFrozen(false)
	update(5)
	if got := ecs.GetComp[gfx.Particles](e.World(), ent).Live; got <= live {
		t.Errorf("Live = %d after unfreezing, want more than %d", got, live)
	}
}

// TestAnimationsFrozenShaderTime: the simulation time behind the shader
// Time uniform stands still while frozen.
func TestAnimationsFrozenShaderTime(t *testing.T) {
	e := New(Config{NoAudio: true})
	defer e.Close()
	e.Update()
	e.SetAnimationsFrozen(true)
	at := e.elapsed
	for range 10 {
		e.Update()
	}
	if e.elapsed != at {
		t.Errorf("elapsed = %v after frozen ticks, want %v", e.elapsed, at)
	}
	e.SetAnimationsFrozen(false)
	e.Update()
	if e.elapsed <= at {
		t.Error("elapsed did not move after unfreezing")
	}
}

// shaderExtract queues one item with a shader every tick.
type shaderExtract struct{ sh gfx.ShaderHandle }

func (s *shaderExtract) Run(ctx *system.Context) {
	ecs.GetResource[render.Queue](ctx.World).Push(render.Item{Kind: render.KindRect, Shader: s.sh})
}

// TestFrameChangedFrozen: with frozen animations, shader items and post
// passes change the frame only when they differ from the drawn one.
func TestFrameChangedFrozen(t *testing.T) {
	g := &gateTool{draw: true}
	e := New(Config{NoAudio: true, Tool: g})
	defer e.Close()
	ex := &shaderExtract{sh: render.LoadShader(e.Assets().Global(), "fx/missing.kage")}
	e.AddExtract(ex)
	e.Post().Passes = []render.Pass{{Params: [4]float32{1}}}
	screen := ebiten.NewImage(8, 8)
	tick := func() {
		t.Helper()
		if err := e.Update(); err != nil {
			t.Fatal(err)
		}
	}
	deadline := time.Now().Add(5 * time.Second)
	for !e.Assets().Global().Done() { // the shader load fails, with an asset event
		if time.Now().After(deadline) {
			t.Fatal("the shader load never ended")
		}
		tick()
		time.Sleep(time.Millisecond)
	}
	tick()
	e.Draw(screen)
	tick()
	if !e.FrameChanged() {
		t.Fatal("a running shader item and post pass report no change")
	}
	e.SetAnimationsFrozen(true)
	if e.FrameChanged() {
		t.Fatal("a frozen frame that stands still reports a change")
	}
	for _, tc := range []struct {
		name   string
		change func()
		undo   func()
	}{
		{"pass params", func() { e.Post().Passes[0].Params[1] = 2 }, func() {}},
		{"pass added", func() { e.Post().Passes = append(e.Post().Passes, render.Pass{}) }, func() {}},
		{"final pass", func() { e.Post().Final = &render.Pass{} }, func() {}},
		{"final params", func() { e.Post().Final.Params[0] = 3 }, func() {}},
		{"final removed", func() { e.Post().Final = nil }, func() {}},
		{"shader item", func() { ex.sh = gfx.ShaderHandle{} }, func() {}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tc.change()
			tick()
			if !e.FrameChanged() {
				t.Errorf("a changed %s is not reported", tc.name)
			}
			e.Draw(screen)
			tick()
			if e.FrameChanged() {
				t.Errorf("still changed after drawing the new %s", tc.name)
			}
		})
	}
}
