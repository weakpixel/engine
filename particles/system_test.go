package particles

import (
	"fmt"
	"testing"
	"testing/fstest"
	"time"

	"github.com/weakpixel/asset"
	"github.com/weakpixel/ecs"
	"github.com/weakpixel/ecs/system"
	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/particles/fxfile"
	"github.com/weakpixel/engine/render"
)

// fxEnv is the resource of the test world that holds the asset server the
// test effects load from.
type fxEnv struct {
	srv   *asset.Server
	files fstest.MapFS
	n     int
}

func newTestSystem() (*ecs.World, *System, *system.Context) {
	w := ecs.NewWorld()
	env := &fxEnv{files: fstest.MapFS{}}
	env.srv = asset.NewServer(asset.Options{FS: env.files})
	render.RegisterLoaders(env.srv, render.LoaderOptions{})
	RegisterLoaders(env.srv)
	ecs.SetResource(w, env)
	ctx := &system.Context{World: w, Time: system.Time{Delta: time.Second / 60}}
	return w, NewSystem(w, EffectStoreOf(env.srv)), ctx
}

// putEffect writes em as an effect file and loads it.
func putEffect(w *ecs.World, em fxfile.Emitter) gfx.EffectHandle {
	env := ecs.GetResource[fxEnv](w)
	env.n++
	key := fmt.Sprintf("fx/e%d.fx.json", env.n)
	env.files[key] = &fstest.MapFile{Data: fxfile.Encode(fxfile.Spec{Emitters: []fxfile.Emitter{em}})}
	h, err := LoadEffectSync(env.srv.Global(), key)
	if err != nil {
		panic(err)
	}
	return h
}

// spawnEmitter creates an entity that plays a new effect of em. c, if given,
// is the component to start from (Layer, Seed, Stop).
func spawnEmitter(w *ecs.World, em fxfile.Emitter, c ...gfx.Particles) ecs.Entity {
	var comp gfx.Particles
	if len(c) > 0 {
		comp = c[0]
	}
	comp.Effect = putEffect(w, em)
	return ecs.NewView2[gfx.Transform, gfx.Particles](w).NewEntity(gfx.Transform{}, comp)
}

func emitterOf(w *ecs.World, e ecs.Entity) *gfx.Particles {
	return ecs.GetComp[gfx.Particles](w, e)
}

func TestSystemRunsEmitter(t *testing.T) {
	w, s, ctx := newTestSystem()
	e := spawnEmitter(w, fxfile.Emitter{Rate: 60, LifeMin: 10, LifeMax: 10})
	for range 10 {
		s.Run(ctx)
	}
	em := emitterOf(w, e)
	if em.Pool == 0 || em.Live < 9 || em.Time <= 0 {
		t.Fatalf("Pool=%d Live=%d Time=%v after 10 ticks", em.Pool, em.Live, em.Time)
	}
}

func TestDeletedEmitterFreesPool(t *testing.T) {
	w, s, ctx := newTestSystem()
	e := spawnEmitter(w, fxfile.Emitter{Rate: 60})
	s.Run(ctx)
	if s.Pools() != 1 {
		t.Fatalf("pools = %d, want 1", s.Pools())
	}
	w.DeleteEntity(e)
	s.Run(ctx)
	if s.Pools() != 0 {
		t.Fatalf("pools = %d after deleting the emitter, want 0", s.Pools())
	}
}

func TestRemovedComponentFreesPool(t *testing.T) {
	w, s, ctx := newTestSystem()
	e := spawnEmitter(w, fxfile.Emitter{Rate: 60})
	s.Run(ctx)
	ecs.RemoveComp[gfx.Particles](w, e)
	s.Run(ctx)
	if s.Pools() != 0 {
		t.Fatalf("pools = %d after removing the component, want 0", s.Pools())
	}
}

func TestClonedPoolIDGetsOwnPool(t *testing.T) {
	w, s, ctx := newTestSystem()
	a := spawnEmitter(w, fxfile.Emitter{Rate: 60, LifeMin: 10, LifeMax: 10})
	s.Run(ctx)
	copied := *emitterOf(w, a) // a clone carries the runtime fields too
	b := ecs.NewView2[gfx.Transform, gfx.Particles](w).NewEntity(gfx.Transform{}, copied)
	s.Run(ctx)
	ea, eb := emitterOf(w, a), emitterOf(w, b)
	if ea.Pool == eb.Pool || s.Pools() != 2 {
		t.Fatalf("pools a=%d b=%d, count %d: the clone shares a pool", ea.Pool, eb.Pool, s.Pools())
	}
	if s.poolOf(a, ea.Pool).owner != a || s.poolOf(b, eb.Pool).owner != b {
		t.Fatal("owners are mixed up")
	}
}

func TestRecycledPoolReusesBuffers(t *testing.T) {
	w, s, ctx := newTestSystem()
	a := spawnEmitter(w, fxfile.Emitter{Burst: 10, LifeMin: 10, LifeMax: 10})
	s.Run(ctx)
	buf := &s.poolOf(a, emitterOf(w, a).Pool).lanes[0].ps[:1][0]
	w.DeleteEntity(a)
	s.Run(ctx)
	b := spawnEmitter(w, fxfile.Emitter{Burst: 5, LifeMin: 10, LifeMax: 10})
	s.Run(ctx)
	p := s.poolOf(b, emitterOf(w, b).Pool)
	if p == nil || len(p.lanes[0].ps) != 5 {
		t.Fatalf("recycled pool holds %v", p)
	}
	if &p.lanes[0].ps[:1][0] != buf || len(s.pools) != 1 {
		t.Fatal("the released pool was not reused")
	}
}

func TestSystemNoAllocs(t *testing.T) {
	w, s, ctx := newTestSystem()
	for i := range 20 {
		spawnEmitter(w, fxfile.Emitter{
			Rate: 300, LifeMin: 0.1, LifeMax: 0.5, Spread: 6, SpeedMax: 30,
			Shape: gfx.EmitCircle, ShapeSize: gfx.Vec2{X: 5}, Max: 100, Loop: i%2 == 0, Duration: 1,
		})
	}
	for range 120 { // reach the steady state: pools created and full
		s.Run(ctx)
	}
	if n := testing.AllocsPerRun(100, func() { s.Run(ctx) }); n != 0 {
		t.Fatalf("Run allocates %v times per tick", n)
	}
}

func ctxOf(w *ecs.World) *system.Context {
	return &system.Context{World: w, Time: system.Time{Delta: time.Second / 60}}
}
