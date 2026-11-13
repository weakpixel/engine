package particles

import (
	"testing"
	"testing/fstest"

	"github.com/weakpixel/ecs"
	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/particles/fxfile"
)

func TestResolveOverrides(t *testing.T) {
	eff := &EffectEmitter{Emitter: fxfile.Emitter{Rate: 16, Color: 0xff0000ff, Shape: gfx.EmitCircle, Loop: true}}
	got := Resolve(eff, &gfx.Particles{
		Rate: gfx.Override[float32](30), Loop: gfx.Override(false), Gravity: gfx.Override(gfx.Vec2{Y: 9}),
	})
	if got.Rate != 30 || got.Loop || got.Gravity.Y != 9 {
		t.Errorf("overrides not applied: %+v", got)
	}
	if got.Color != 0xff0000ff || got.Shape != gfx.EmitCircle {
		t.Errorf("non-overridden values changed: %+v", got)
	}
	if want := eff.Emitter; want.Rate != 16 || !want.Loop {
		t.Errorf("the effect was written through: %+v", want)
	}
}

// reload rewrites the effect file behind ent and settles the server.
func reload(w *ecs.World, ent ecs.Entity, em fxfile.Emitter) {
	env := ecs.GetResource[fxEnv](w)
	key := EffectStoreOf(env.srv).Key(emitterOf(w, ent).Effect).String()
	env.files[key] = &fstest.MapFile{Data: fxfile.Encode(fxfile.Spec{Emitters: []fxfile.Emitter{em}})}
	env.srv.Changed(key)
	settle(env.srv)
}

func TestOverrideFollowsFileForOtherValues(t *testing.T) {
	w, s, ctx := newTestSystem()
	em := fxfile.Emitter{Rate: 60, LifeMin: 100, LifeMax: 100, Max: 1000}
	a := spawnEmitter(w, em, gfx.Particles{Rate: gfx.Override[float32](30)})
	for range 60 {
		s.Run(ctx)
	}
	if live := emitterOf(w, a).Live; live < 29 || live > 31 {
		t.Fatalf("Live = %d after 1s, want about 30 (the override, not the file's 60)", live)
	}
	// The file's rate is ignored; its gravity (not overridden) applies.
	em.Rate, em.Gravity = 600, gfx.Vec2{Y: 10}
	reload(w, a, em)
	before := emitterOf(w, a).Live
	for range 60 {
		s.Run(ctx)
	}
	c := emitterOf(w, a)
	if c.Live < before+29 || c.Live > before+31 {
		t.Fatalf("Live %d -> %d, want +30: the file's rate leaked through the override", before, c.Live)
	}
	if c.Time < 1.99 {
		t.Fatalf("Time = %v: the pool restarted for a change particles can follow", c.Time)
	}
}

func TestOverrideOfCycleKeyRestarts(t *testing.T) {
	w, s, ctx := newTestSystem()
	a := spawnEmitter(w, fxfile.Emitter{Rate: 60, LifeMin: 100, LifeMax: 100, Max: 10})
	for range 30 {
		s.Run(ctx)
	}
	if c := emitterOf(w, a); c.Live != 10 {
		t.Fatalf("Live = %d, want the 10 the pool holds", c.Live)
	}
	emitterOf(w, a).Max = gfx.Override[uint16](25)
	s.Run(ctx)
	if c := emitterOf(w, a); c.Time > 2*float32(ctx.Time.Delta.Seconds()) || c.Live > 3 {
		t.Fatalf("Time=%v Live=%d: the override of Max did not restart the pool", c.Time, c.Live)
	}
	for range 30 {
		s.Run(ctx)
	}
	if c := emitterOf(w, a); c.Live != 25 {
		t.Fatalf("Live = %d, want the overridden Max of 25", c.Live)
	}
	// Removing the override returns to the file at once.
	emitterOf(w, a).Max = nil
	s.Run(ctx)
	for range 30 {
		s.Run(ctx)
	}
	if c := emitterOf(w, a); c.Live != 10 {
		t.Fatalf("Live = %d after Reset, want the file's 10", c.Live)
	}
}

func TestSpriteAndShaderOverride(t *testing.T) {
	w, s, x, q := newTestExtract(t)
	ctx := ctxOf(w)
	store := s.effects
	other, _ := store.Lookup(putEffect(w, fxfile.Emitter{Sprite: "fx/other.png"}))
	base := fxfile.Emitter{Burst: 1, LifeMin: 10, LifeMax: 10, Sprite: "fx/base.png"}
	spawnEmitter(w, base)
	spawnEmitter(w, base, gfx.Particles{Sprite: other.Emitters[0].SpriteRef})
	s.Run(ctx)
	x.Run(ctx)
	if q.Len() != 2 {
		t.Fatalf("queued %d, want 2", q.Len())
	}
	a, b := q.At(0).Sprite, q.At(1).Sprite
	if a == b {
		t.Fatal("the sprite override was not drawn")
	}
	if a != other.Emitters[0].SpriteRef && b != other.Emitters[0].SpriteRef {
		t.Fatal("no item uses the overriding sprite")
	}
}

func TestOverrideNoAllocs(t *testing.T) {
	w, s, x, q := newTestExtract(t)
	ctx := ctxOf(w)
	for range 10 {
		spawnEmitter(w, fxfile.Emitter{Rate: 600, LifeMin: 0.2, LifeMax: 0.5, Max: 200, SpeedMax: 10},
			gfx.Particles{Rate: gfx.Override[float32](300), Color: gfx.Override(gfx.Color(0xff0000ff))})
	}
	for range 60 {
		s.Run(ctx)
	}
	n := testing.AllocsPerRun(50, func() {
		q.Reset()
		s.Run(ctx)
		x.Run(ctx)
	})
	if n != 0 {
		t.Fatalf("tick allocates %v times", n)
	}
}
