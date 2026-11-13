package particles

import (
	"testing"

	"github.com/weakpixel/ecs"
	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/particles/fxfile"
)

var clockEmitter = fxfile.Emitter{Rate: 30, LifeMin: 2, LifeMax: 2, SpeedMin: 10, SpeedMax: 10, Spread: 3}

var clockSeed = gfx.Particles{Seed: 7}

func TestClockModes(t *testing.T) {
	t.Run("paused", func(t *testing.T) {
		w, s, ctx := newTestSystem()
		ecs.SetResource(w, &Clock{Paused: true})
		e := spawnEmitter(w, clockEmitter, clockSeed)
		for range 10 {
			s.Run(ctx)
		}
		if em := emitterOf(w, e); em.Live != 0 || em.Time != 0 || s.Pools() != 1 {
			t.Fatalf("Live=%d Time=%v pools=%d while paused", em.Live, em.Time, s.Pools())
		}
	})
	t.Run("step", func(t *testing.T) {
		w, s, ctx := newTestSystem()
		c := &Clock{Paused: true, Step: 3}
		ecs.SetResource(w, c)
		e := spawnEmitter(w, clockEmitter, clockSeed)
		for range 10 {
			s.Run(ctx)
		}
		em := emitterOf(w, e)
		if c.Step != 0 || em.Time < 3.0/60-1e-4 || em.Time > 3.0/60+1e-4 {
			t.Fatalf("Step=%d Time=%v after 3 steps", c.Step, em.Time)
		}
	})
	t.Run("scale", func(t *testing.T) {
		w, s, ctx := newTestSystem()
		ecs.SetResource(w, &Clock{Scale: 0.5})
		e := spawnEmitter(w, clockEmitter, clockSeed)
		for range 60 {
			s.Run(ctx)
		}
		if tm := emitterOf(w, e).Time; tm < 0.49 || tm > 0.51 {
			t.Fatalf("Time = %v after 1 s at half speed, want 0.5", tm)
		}
	})
}

// TestClockSeekMatchesTicks proves a seek to t equals running t/dt ticks.
func TestClockSeekMatchesTicks(t *testing.T) {
	w1, s1, ctx1 := newTestSystem()
	e1 := spawnEmitter(w1, clockEmitter, clockSeed)
	for range 90 {
		s1.Run(ctx1)
	}
	w2, s2, ctx2 := newTestSystem()
	c := &Clock{}
	ecs.SetResource(w2, c)
	e2 := spawnEmitter(w2, clockEmitter, clockSeed)
	for range 5 {
		s2.Run(ctx2)
	}
	c.SeekTo(1.5)
	s2.Run(ctx2)
	a, b := emitterOf(w1, e1), emitterOf(w2, e2)
	if a.Live != b.Live || a.Time != b.Time {
		t.Fatalf("ticks: Live=%d Time=%v; seek: Live=%d Time=%v", a.Live, a.Time, b.Live, b.Time)
	}
	pa, pb := s1.poolOf(e1, a.Pool), s2.poolOf(e2, b.Pool)
	la, lb := pa.lanes[0], pb.lanes[0]
	if len(la.ps) == 0 || len(la.ps) != len(lb.ps) {
		t.Fatalf("particles: %d vs %d", len(la.ps), len(lb.ps))
	}
	for i := range la.ps {
		if la.ps[i].Pos != lb.ps[i].Pos {
			t.Fatalf("particle %d at %v vs %v", i, la.ps[i].Pos, lb.ps[i].Pos)
		}
	}
	seekTime := b.Time
	if c.seek {
		t.Error("seek request not consumed")
	}
	s2.Run(ctx2)
	if emitterOf(w2, e2).Time <= seekTime {
		t.Error("simulation did not continue after the seek")
	}
}

// TestDefaultClockChangesNothing runs the same emitter with no clock and a
// zero clock and compares, and checks the default clock stays at 0 allocs.
func TestDefaultClockChangesNothing(t *testing.T) {
	run := func(withClock bool) (uint16, float32) {
		w, s, ctx := newTestSystem()
		if withClock {
			ecs.SetResource(w, &Clock{})
		}
		e := spawnEmitter(w, clockEmitter, clockSeed)
		for range 100 {
			s.Run(ctx)
		}
		em := emitterOf(w, e)
		return em.Live, em.Time
	}
	l1, t1 := run(false)
	l2, t2 := run(true)
	if l1 != l2 || t1 != t2 {
		t.Fatalf("no clock: %d %v; zero clock: %d %v", l1, t1, l2, t2)
	}
	w, s, ctx := newTestSystem()
	ecs.SetResource(w, &Clock{})
	spawnEmitter(w, clockEmitter, clockSeed)
	s.Run(ctx)
	if n := testing.AllocsPerRun(100, func() { s.Run(ctx) }); n != 0 {
		t.Errorf("Run allocates %v with the default clock", n)
	}
}
