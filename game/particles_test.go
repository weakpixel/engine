package game

import (
	"testing"
	"testing/fstest"

	"github.com/weakpixel/ecs"
	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/input"
	"github.com/weakpixel/engine/particles"
	"github.com/weakpixel/engine/particles/fxfile"
)

// fxFiles is the asset filesystem of a game that plays one effect: sparks.
func fxFiles(em fxfile.Emitter) fstest.MapFS {
	return fstest.MapFS{"fx/sparks.fx.json": {Data: fxfile.Encode(fxfile.Spec{Emitters: []fxfile.Emitter{em}})}}
}

// spawnEmitter plays fx/sparks.fx.json at the origin.
func spawnEmitter(t *testing.T, e *Engine) ecs.Entity {
	t.Helper()
	h, err := particles.LoadEffectSync(e.Assets().Global(), "fx/sparks.fx.json")
	if err != nil {
		t.Fatal(err)
	}
	return ecs.NewView2[gfx.Transform, gfx.Particles](e.World()).NewEntity(gfx.Transform{}, gfx.Particles{Effect: h})
}

// TestParticlesDrawn: the engine runs the particle system and extract, and
// the particles keep simulating while a Tool pauses the game.
func TestParticlesDrawn(t *testing.T) {
	tool := &testTool{}
	e := New(Config{NoAudio: true, Tool: tool, Width: 64, Height: 32, Input: InputConfig{Source: &input.ManualSource{}},
		FS: fxFiles(fxfile.Emitter{Rate: 120, LifeMin: 10, LifeMax: 10})})
	defer e.Close()
	ent := spawnEmitter(t, e)
	if err := e.Update(); err != nil {
		t.Fatal(err)
	}
	if e.queue.Len() == 0 {
		t.Fatal("no particle was queued")
	}
	live := ecs.GetComp[gfx.Particles](e.World(), ent).Live

	for range 30 {
		tool.script = append(tool.script, TickPause)
	}
	for range 30 {
		if err := e.Update(); err != nil {
			t.Fatal(err)
		}
	}
	if !e.Paused() {
		t.Fatal("the engine did not pause")
	}
	if got := ecs.GetComp[gfx.Particles](e.World(), ent).Live; got <= live {
		t.Fatalf("Live = %d after 30 paused ticks (was %d): particles stopped under TickPause", got, live)
	}
}

func TestTickNoAllocsWithParticles(t *testing.T) {
	if testing.CoverMode() != "" || raceEnabled {
		t.Skip("instrumentation allocates")
	}
	e := New(Config{NoAudio: true, Width: 64, Height: 32, Input: InputConfig{Source: &input.ManualSource{}},
		FS: fxFiles(fxfile.Emitter{Rate: 600, LifeMin: 0.2, LifeMax: 0.4, SpeedMax: 20, Spread: 0.1, Max: 300})})
	defer e.Close()
	spawnEmitter(t, e)
	for range 120 { // fill the pool and the queue
		e.Update()
	}
	if n := testing.AllocsPerRun(20, func() { e.Update() }); n != 0 {
		t.Fatalf("allocs per tick = %v, want 0", n)
	}
	if e.queue.Len() == 0 {
		t.Fatal("nothing queued")
	}
}
