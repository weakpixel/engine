package particles

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/weakpixel/asset"
	"github.com/weakpixel/ecs"
	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/particles/fxfile"
)

func settle(srv *asset.Server) {
	for range 20 {
		srv.Update()
	}
}

func TestLoadEffect(t *testing.T) {
	w, _, _ := newTestSystem()
	env := ecs.GetResource[fxEnv](w)
	sparks := fxfile.Encode(fxfile.Spec{Emitters: []fxfile.Emitter{{Sprite: "fx/spark.png", Burst: 12}}})
	env.files["fx/a.fx.json"] = &fstest.MapFile{Data: sparks}
	h, err := LoadEffectSync(env.srv.Global(), "fx/a.fx.json")
	if err != nil {
		t.Fatal(err)
	}
	store := EffectStoreOf(env.srv)
	eff, state := store.Lookup(h)
	if state != asset.Ready || eff.Emitters[0].Burst != 12 || eff.Emitters[0].Sprite != "fx/spark.png" {
		t.Fatalf("state %v effect %+v", state, eff.Emitters)
	}
	if store.Key(h).String() != "fx/a.fx.json" || store.Version(h) != 1 {
		t.Fatalf("key %v version %d", store.Key(h), store.Version(h))
	}
	// The image does not exist: the handle is there and draws the placeholder.
	if eff.Emitters[0].SpriteRef.IsZero() || !eff.Emitters[0].ShaderRef.IsZero() {
		t.Fatalf("sprite %v shader %v", eff.Emitters[0].SpriteRef, eff.Emitters[0].ShaderRef)
	}
}

func TestLoadEffectFails(t *testing.T) {
	w, _, _ := newTestSystem()
	env := ecs.GetResource[fxEnv](w)
	tests := []struct {
		name, text, want string
	}{
		{"bad json", `{`, "effect json"},
		{"bad value", `{"emitters":[{"blend":"x"}]}`, "blend"},
		{"bad sprite key", `{"emitters":[{"sprite":"../up.png"}]}`, "sprite: asset: bad key"},
		{"bad shader key", `{"emitters":[{"shader":"a\\b.kage"}]}`, "shader: asset: bad key"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			key := "fx/" + strings.ReplaceAll(tc.name, " ", "-") + ".fx.json"
			env.files[key] = &fstest.MapFile{Data: []byte(tc.text)}
			h, err := LoadEffectSync(env.srv.Global(), key)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("error = %v, want %q", err, tc.want)
			}
			if EffectStoreOf(env.srv).Ready(h) {
				t.Fatal("a failed effect is Ready")
			}
		})
	}
}

// TestNotReadyPlaysNothing: a zero handle, a loading effect and a failed one
// have no pool, and the component stays untouched.
func TestNotReadyPlaysNothing(t *testing.T) {
	w, s, ctx := newTestSystem()
	env := ecs.GetResource[fxEnv](w)
	env.files["fx/bad.fx.json"] = &fstest.MapFile{Data: []byte(`{}`)}
	bad, _ := LoadEffectSync(env.srv.Global(), "fx/bad.fx.json")
	loading := LoadEffect(env.srv.Global(), "fx/missing.fx.json") // not settled yet
	view := ecs.NewView2[gfx.Transform, gfx.Particles](w)
	var ents []ecs.Entity
	for _, h := range []gfx.EffectHandle{{}, bad, loading} {
		ents = append(ents, view.NewEntity(gfx.Transform{}, gfx.Particles{Effect: h}))
	}
	for range 5 {
		s.Run(ctx)
	}
	for i, e := range ents {
		if c := emitterOf(w, e); c.Pool != 0 || c.Live != 0 || c.Time != 0 {
			t.Errorf("entity %d: %+v", i, *c)
		}
	}
	if s.Pools() != 0 {
		t.Fatalf("pools = %d, want 0", s.Pools())
	}
}

// TestEffectReadyLater: an entity placed while its effect loads starts
// playing once it is ready.
func TestEffectReadyLater(t *testing.T) {
	w, s, ctx := newTestSystem()
	env := ecs.GetResource[fxEnv](w)
	env.files["fx/late.fx.json"] = &fstest.MapFile{Data: fxfile.Encode(fxfile.Spec{Emitters: []fxfile.Emitter{{Rate: 60, LifeMin: 10, LifeMax: 10}}})}
	h := LoadEffect(env.srv.Global(), "fx/late.fx.json")
	e := ecs.NewView2[gfx.Transform, gfx.Particles](w).NewEntity(gfx.Transform{}, gfx.Particles{Effect: h})
	s.Run(ctx)
	if emitterOf(w, e).Pool != 0 {
		t.Fatal("played before the effect was ready")
	}
	settle(env.srv)
	for range 10 {
		s.Run(ctx)
	}
	if c := emitterOf(w, e); c.Pool == 0 || c.Live == 0 {
		t.Fatalf("%+v: not playing after the effect was ready", *c)
	}
}

// TestHotReloadRestarts: a changed effect file restarts the pools that play
// it, with the new values, and leaves other effects alone.
func TestHotReloadRestarts(t *testing.T) {
	w, s, ctx := newTestSystem()
	env := ecs.GetResource[fxEnv](w)
	em := fxfile.Emitter{Rate: 60, LifeMin: 100, LifeMax: 100, Max: 10}
	a := spawnEmitter(w, em)
	b := spawnEmitter(w, em)
	for range 30 {
		s.Run(ctx)
	}
	if c := emitterOf(w, a); c.Live != 10 {
		t.Fatalf("Live = %d, want the 10 the pool holds", c.Live)
	}
	poolB := emitterOf(w, b).Pool

	key := EffectStoreOf(env.srv).Key(emitterOf(w, a).Effect).String()
	em.Max, em.Rate = 25, 120
	env.files[key] = &fstest.MapFile{Data: fxfile.Encode(fxfile.Spec{Emitters: []fxfile.Emitter{em}})}
	env.srv.Changed(key)
	settle(env.srv)
	s.Run(ctx)

	ca := emitterOf(w, a)
	if ca.Time > 2*float32(ctx.Time.Delta.Seconds()) || ca.Live > 3 {
		t.Fatalf("after the reload Time=%v Live=%d: the pool did not start over", ca.Time, ca.Live)
	}
	for range 30 {
		s.Run(ctx)
	}
	if ca.Live != 25 {
		t.Fatalf("Live = %d, want the new Max of 25", ca.Live)
	}
	if cb := emitterOf(w, b); cb.Live != 10 || cb.Pool != poolB {
		t.Fatalf("the other effect changed: %+v", *cb)
	}
}

// TestHotReloadKeepsLiveParticles: a change live particles can follow (color,
// rate, gravity) applies at once and does not restart the pool.
func TestHotReloadKeepsLiveParticles(t *testing.T) {
	w, s, ctx := newTestSystem()
	env := ecs.GetResource[fxEnv](w)
	em := fxfile.Emitter{Rate: 60, LifeMin: 100, LifeMax: 100, Max: 100}
	a := spawnEmitter(w, em)
	for range 30 {
		s.Run(ctx)
	}
	before := emitterOf(w, a).Live
	key := EffectStoreOf(env.srv).Key(emitterOf(w, a).Effect).String()
	em.Rate, em.Gravity = 0, gfx.Vec2{Y: 10}
	env.files[key] = &fstest.MapFile{Data: fxfile.Encode(fxfile.Spec{Emitters: []fxfile.Emitter{em}})}
	env.srv.Changed(key)
	settle(env.srv)
	for range 30 {
		s.Run(ctx)
	}
	if c := emitterOf(w, a); c.Live != before || c.Time < 0.99 {
		t.Fatalf("Live=%d (was %d) Time=%v: the pool restarted", c.Live, before, c.Time)
	}
}
