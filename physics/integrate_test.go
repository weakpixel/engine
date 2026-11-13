package physics

import (
	"testing"

	"github.com/weakpixel/ecs"
	"github.com/weakpixel/engine/gfx"
)

func TestFreeFall(t *testing.T) {
	tw := newWorld(t, platformer)
	e := tw.box(v(0, 0), 8, 8)
	const n = 120
	tw.step(n)
	secs := float32(n) / 60
	want := 0.5 * 980 * secs * secs
	if got := tw.pos(e).Y; !near(got, want, want/100) {
		t.Errorf("fell %v after %d ticks, want %v within 1%%", got, n, want)
	}
}

func TestGravityScale(t *testing.T) {
	tw := newWorld(t, platformer)
	still := tw.box(v(0, 0), 8, 8)
	half := tw.box(v(100, 0), 8, 8)
	full := tw.box(v(200, 0), 8, 8)
	ramp := tw.box(v(300, 0), 8, 8)
	tw.body(still).GravityScale = 0
	tw.body(half).GravityScale = 0.5
	for i := range 60 {
		tw.body(ramp).GravityScale = max(0, 1-float32(i)/30) // a tween down to 0
		tw.step(1)
	}
	if p := tw.pos(still); p != v(0, 0) {
		t.Errorf("scale 0 moved to %v", p)
	}
	if h, f := tw.pos(half).Y, tw.pos(full).Y; !near(h, f/2, f/100) {
		t.Errorf("scale 0.5 fell %v, full %v", h, f)
	}
	vy := tw.vel(ramp).Y
	tw.step(10)
	if got := tw.vel(ramp).Y; got != vy {
		t.Errorf("ramped to 0 still accelerates: %v -> %v", vy, got)
	}
}

func TestDampingAndMaxSpeed(t *testing.T) {
	tw := newWorld(t, platformer)
	d := tw.box(v(0, 0), 8, 8)
	*tw.body(d) = Body{Damping: v(0.5, 0)}
	*tw.vel(d) = v(100, 0)
	m := tw.box(v(100, 0), 8, 8)
	tw.body(m).MaxSpeed = v(0, 200)
	tw.step(60)
	if got := tw.vel(d).X; !near(got, 50, 1) {
		t.Errorf("damping 0.5 left %v of 100 after a second, want 50", got)
	}
	if got := tw.vel(m).Y; got != 200 {
		t.Errorf("max speed: vy = %v, want 200", got)
	}
	l := tw.box(v(200, 0), 8, 8)
	tw.body(l).LockY = true
	*tw.vel(l) = v(10, 50)
	tw.step(6)
	if p := tw.pos(l); p.Y != 0 || !near(p.X, 201, 0.01) {
		t.Errorf("LockY body at %v, want (201, 0)", p)
	}
}

func TestKinematicIgnoresGravity(t *testing.T) {
	tw := newWorld(t, platformer)
	k := tw.box(v(0, 0), 8, 8)
	*tw.body(k) = Body{Kind: Kinematic, GravityScale: 1}
	*tw.vel(k) = v(60, 0)
	tw.step(60)
	if p := tw.pos(k); !near(p.X, 60, 0.01) || p.Y != 0 {
		t.Errorf("kinematic at %v, want (60, 0)", p)
	}
}

func TestBodyGetsVelocity(t *testing.T) {
	tw := newWorld(t, platformer)
	e := tw.w.NewEntity(gfx.Transform{}, Body{GravityScale: 1})
	tw.step(1)
	if !ecs.HasComp[Velocity](tw.w, e) {
		t.Fatal("body has no Velocity after a tick")
	}
	tw.step(10)
	if tw.pos(e).Y <= 0 {
		t.Errorf("body did not fall: %v", tw.pos(e))
	}
}

func TestNegativeCountsPanic(t *testing.T) {
	mustPanic(t, "physics: negative Substeps", func() { NewSystem(ecs.NewWorld(), Config{Substeps: -1}) })
	mustPanic(t, "physics: negative Iterations", func() { NewSystem(ecs.NewWorld(), Config{Iterations: -1}) })
	w := ecs.NewWorld()
	NewSystem(w, Config{})
	mustPanic(t, "collision: Space already has a system", func() { NewSystem(w, Config{}) })
}

func TestSubstepsFall(t *testing.T) {
	tw := newWorld(t, Config{Gravity: v(0, 980), Substeps: 4})
	e := tw.box(v(0, 0), 8, 8)
	tw.step(60)
	if got := tw.pos(e).Y; !near(got, 490, 5) {
		t.Errorf("fell %v in a second with 4 substeps, want ~490", got)
	}
}

func TestMoveSystem(t *testing.T) {
	tw := newWorld(t, platformer)
	tw.sched.AddRunner(NewMoveSystem(tw.w))
	bullet := tw.w.NewEntity(gfx.Transform{}, Velocity{Vel: v(120, -60)})
	b := tw.box(v(0, 0), 8, 8)
	tw.step(30)
	if p := tw.pos(bullet); !near(p.X, 60, 0.01) || !near(p.Y, -30, 0.01) {
		t.Errorf("mover at %v, want (60, -30)", p)
	}
	// Moved by physics only: the fall of one free fall, not two.
	want := float32(0.5 * 980 * 0.25)
	if got := tw.pos(b).Y; !near(got, want, want/20) {
		t.Errorf("body fell %v, want ~%v (moved once per tick)", got, want)
	}
}
