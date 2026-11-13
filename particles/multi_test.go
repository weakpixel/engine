package particles

import (
	"fmt"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/weakpixel/ecs"
	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/particles/fxfile"
)

// spawnMulti creates an entity that plays a new effect of ems.
func spawnMulti(w *ecs.World, ems []fxfile.Emitter, c ...gfx.Particles) ecs.Entity {
	env := ecs.GetResource[fxEnv](w)
	env.n++
	key := fmt.Sprintf("fx/m%d.fx.json", env.n)
	env.files[key] = &fstest.MapFile{Data: fxfile.Encode(fxfile.Spec{Emitters: ems})}
	h, err := LoadEffectSync(env.srv.Global(), key)
	if err != nil {
		panic(err)
	}
	var comp gfx.Particles
	if len(c) > 0 {
		comp = c[0]
	}
	comp.Effect = h
	return ecs.NewView2[gfx.Transform, gfx.Particles](w).NewEntity(gfx.Transform{}, comp)
}

func TestMultiEmitterFile(t *testing.T) {
	in := fxfile.Spec{Emitters: []fxfile.Emitter{
		{Name: "smoke", Layer: -1, Rate: 5},
		{Name: "sparks", Burst: 9, Layer: 2},
		{},
	}}
	got, err := fxfile.Decode(fxfile.Encode(in))
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Emitters) != 3 || got.Emitters[0] != in.Emitters[0] || got.Emitters[1] != in.Emitters[1] {
		t.Fatalf("round trip = %+v", got.Emitters)
	}
	if _, err := fxfile.Decode([]byte(`{"emitters":[{},{"colour":1}]}`)); err == nil {
		t.Fatal("unknown key in the second emitter accepted")
	} else if want := "emitters[1].colour: unknown key"; !strings.Contains(err.Error(), want) {
		t.Fatalf("error %q lacks %q", err, want)
	}
}

func TestMultiEmitterPlaysAll(t *testing.T) {
	w, s, ctx := newTestSystem()
	e := spawnMulti(w, []fxfile.Emitter{
		{Burst: 4, LifeMin: 0.05, LifeMax: 0.05},
		{Burst: 6, LifeMin: 10, LifeMax: 10},
	})
	s.Run(ctx)
	c := emitterOf(w, e)
	if c.Live != 10 || c.Done || s.Pools() != 1 {
		t.Fatalf("Live=%d Done=%v pools=%d, want 10 live from both emitters in one pool", c.Live, c.Done, s.Pools())
	}
	for range 10 {
		s.Run(ctx)
	}
	if c.Live != 6 || c.Done {
		t.Fatalf("Live=%d Done=%v after the short emitter ended", c.Live, c.Done)
	}
}

func TestMultiEmitterDoneAndStop(t *testing.T) {
	w, s, ctx := newTestSystem()
	e := spawnMulti(w, []fxfile.Emitter{
		{Rate: 60, LifeMin: 0.05, LifeMax: 0.05},
		{Rate: 60, LifeMin: 0.05, LifeMax: 0.05},
	})
	for range 5 {
		s.Run(ctx)
	}
	c := emitterOf(w, e)
	if c.Live == 0 {
		t.Fatal("nothing spawned")
	}
	c.Stop = true
	for range 10 {
		s.Run(ctx)
	}
	if c.Live != 0 || !c.Done {
		t.Fatalf("Live=%d Done=%v after Stop", c.Live, c.Done)
	}
}

func TestMultiEmitterSeedsDiffer(t *testing.T) {
	w, s, ctx := newTestSystem()
	em := fxfile.Emitter{Burst: 1, SpeedMin: 10, SpeedMax: 50, Spread: 6, LifeMin: 10, LifeMax: 10}
	e := spawnMulti(w, []fxfile.Emitter{em, em}, gfx.Particles{Seed: 7})
	s.Run(ctx)
	p := s.poolOf(e, emitterOf(w, e).Pool)
	if p.lanes[0].ps[0].Vel == p.lanes[1].ps[0].Vel {
		t.Fatal("equal emitters spawned identical particles")
	}
}

// reloadMulti rewrites the effect file behind ent and settles the server.
func reloadMulti(w *ecs.World, ent ecs.Entity, ems []fxfile.Emitter) {
	env := ecs.GetResource[fxEnv](w)
	key := EffectStoreOf(env.srv).Key(emitterOf(w, ent).Effect).String()
	env.files[key] = &fstest.MapFile{Data: fxfile.Encode(fxfile.Spec{Emitters: ems})}
	env.srv.Changed(key)
	settle(env.srv)
}

func TestMultiEmitterReload(t *testing.T) {
	w, s, ctx := newTestSystem()
	a := fxfile.Emitter{Rate: 60, LifeMin: 100, LifeMax: 100, Max: 1000}
	b := fxfile.Emitter{Rate: 60, LifeMin: 100, LifeMax: 100, Max: 1000}
	e := spawnMulti(w, []fxfile.Emitter{a, b})
	for range 30 {
		s.Run(ctx)
	}
	p := s.poolOf(e, emitterOf(w, e).Pool)
	la, lb := len(p.lanes[0].ps), len(p.lanes[1].ps)
	if la == 0 || lb == 0 {
		t.Fatal("nothing spawned")
	}

	// A change of a cycle key of the second emitter restarts only that lane.
	b.Max = 500
	reloadMulti(w, e, []fxfile.Emitter{a, b})
	s.Run(ctx)
	if got := len(p.lanes[0].ps); got < la {
		t.Fatalf("first lane restarted: %d particles, had %d", got, la)
	}
	if got := len(p.lanes[1].ps); got >= lb {
		t.Fatalf("second lane kept %d particles, want a restart", got)
	}

	// A new emitter count restarts the pool and reuses its slot.
	reloadMulti(w, e, []fxfile.Emitter{a})
	s.Run(ctx)
	if p2 := s.poolOf(e, emitterOf(w, e).Pool); p2 != p || len(p.lanes) != 1 || len(p.lanes[0].ps) > 2 {
		t.Fatalf("lanes = %d with %d particles after losing an emitter", len(p.lanes), len(p.lanes[0].ps))
	}
	reloadMulti(w, e, []fxfile.Emitter{a, b})
	s.Run(ctx)
	if len(p.lanes) != 2 || p.lanes[1] == nil {
		t.Fatalf("lanes = %d after regaining an emitter", len(p.lanes))
	}
}

func TestMultiEmitterExtractLayers(t *testing.T) {
	w, s, x, q := newTestExtract(t)
	ctx := ctxOf(w)
	spawnMulti(w, []fxfile.Emitter{
		{Burst: 1, Layer: -1, LifeMin: 10, LifeMax: 10},
		{Burst: 2, Layer: 3, LifeMin: 10, LifeMax: 10},
	}, gfx.Particles{Layer: 5})
	s.Run(ctx)
	x.Run(ctx)
	if q.Len() != 3 {
		t.Fatalf("queued %d items, want 3", q.Len())
	}
	layers := map[int16]int{}
	for i := range q.Len() {
		layers[q.At(i).Layer]++
	}
	if layers[4] != 1 || layers[8] != 2 {
		t.Fatalf("layers = %v, want one on 4 and two on 8", layers)
	}
}

func TestMultiEmitterNoAllocs(t *testing.T) {
	w, s, x, q := newTestExtract(t)
	ctx := ctxOf(w)
	em := fxfile.Emitter{Rate: 300, LifeMin: 0.1, LifeMax: 0.5, Spread: 6, SpeedMax: 30, Max: 100}
	for range 10 {
		spawnMulti(w, []fxfile.Emitter{em, em, em})
	}
	for range 120 {
		s.Run(ctx)
	}
	if n := testing.AllocsPerRun(100, func() { s.Run(ctx); q.Reset(); x.Run(ctx) }); n != 0 {
		t.Fatalf("tick allocates %v times", n)
	}
}
