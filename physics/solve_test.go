package physics

import (
	"testing"

	"github.com/weakpixel/ecs"
	"github.com/weakpixel/engine/gfx"
)

func TestBallBounce(t *testing.T) {
	tw := newWorld(t, Config{Gravity: v(0, 980), Substeps: 4})
	tw.wall(v(-50, 200), 100, 20)
	ball := tw.box(v(0, 92), 8, 8, Material{Bounce: 0.5, Friction: 0.4}) // bottom at 100: drop 100
	ballBottom := func() float32 { return tw.pos(ball).Y + 8 }
	// Fall, bounce, and track the apex after the first upward move.
	up, top := false, float32(1e9)
	for range 120 {
		tw.step(1)
		vy := tw.vel(ball).Y
		if vy < 0 {
			up = true
			top = min(top, ballBottom())
		}
		if up && vy > 0 {
			break
		}
	}
	rebound := 200 - top
	want := float32(0.5 * 0.5 * 100)
	if !near(rebound, want, want*0.2) {
		t.Errorf("rebound height %v, want ~%v (Bounce^2 x drop)", rebound, want)
	}
}

func TestRestingStackStable(t *testing.T) {
	tw := newWorld(t, platformer)
	tw.wall(v(-50, 100), 200, 20)
	var boxes [3]ecs.Entity
	for i := range boxes {
		boxes[i] = tw.box(v(0, 100-16*float32(i+1)), 16, 16)
	}
	tw.step(60) // settle
	var start [3]gfx.Vec2
	for i, b := range boxes {
		start[i] = tw.pos(b)
	}
	tw.step(600)
	for i, b := range boxes {
		p := tw.pos(b)
		if !near(p.X, start[i].X, 0.1) || !near(p.Y, start[i].Y, 0.1) {
			t.Errorf("box %d drifted from %v to %v", i, start[i], p)
		}
		if want := 100 - 16*float32(i+1); !near(p.Y, want, 0.5) {
			t.Errorf("box %d rests at y=%v, want ~%v", i, p.Y, want)
		}
	}
}

func TestFrictionStopsSlide(t *testing.T) {
	tw := newWorld(t, platformer)
	tw.wall(v(-1000, 100), 3000, 20)
	b := tw.box(v(0, 84), 16, 16)
	tw.step(5)
	*tw.vel(b) = v(200, 0)
	tw.step(120)
	if vx := tw.vel(b).X; vx != 0 {
		t.Errorf("box still slides at %v after 2 s on default friction", vx)
	}
	// mu*g = 392 u/s^2: stops after ~0.5 s and ~51 units.
	if x := tw.pos(b).X; !near(x, 51, 8) {
		t.Errorf("box slid to x=%v, want ~51", x)
	}
}

func TestIceKeepsSliding(t *testing.T) {
	tw := newWorld(t, platformer)
	tw.wall(v(-1000, 100), 3000, 20, Material{Friction: 0})
	b := tw.box(v(0, 84), 16, 16)
	tw.step(5)
	*tw.vel(b) = v(200, 0)
	tw.step(60)
	if vx := tw.vel(b).X; !near(vx, 200, 0.01) {
		t.Errorf("box on ice slows to %v, want 200", vx)
	}
}

func TestConveyorMovesBox(t *testing.T) {
	tw := newWorld(t, platformer)
	tw.wall(v(-1000, 100), 3000, 20, Material{Friction: 1, SurfaceVel: v(50, 0)})
	b := tw.box(v(0, 84), 16, 16)
	tw.step(60)
	if vx := tw.vel(b).X; !near(vx, 50, 0.5) {
		t.Errorf("box on a conveyor moves at %v, want 50", vx)
	}
}

func TestTrampoline(t *testing.T) {
	tw := newWorld(t, Config{Gravity: v(0, 980), Substeps: 4})
	tw.wall(v(-50, 200), 40, 20)
	tw.wall(v(50, 200), 40, 20, Material{Bounce: 1.2})
	plain := tw.box(v(-40, 92), 8, 8, Material{Bounce: 0.5})
	tramp := tw.box(v(60, 92), 8, 8)
	topPlain, topTramp := float32(1e9), float32(1e9)
	for range 90 {
		tw.step(1)
		if tw.vel(plain).Y < 0 {
			topPlain = min(topPlain, tw.pos(plain).Y)
		}
		if tw.vel(tramp).Y < 0 {
			topTramp = min(topTramp, tw.pos(tramp).Y)
		}
	}
	if topTramp >= 92 {
		t.Errorf("trampoline apex y=%v, want above the drop height 92", topTramp)
	}
	if topTramp >= topPlain {
		t.Errorf("trampoline apex %v not above plain bounce apex %v", topTramp, topPlain)
	}
}

func TestOneSidedMaskPush(t *testing.T) {
	tw := newWorld(t, Config{})
	// a reacts to b; b does not react to a: only a is pushed.
	a := tw.box(v(0, 0), 10, 10)
	b := tw.box(v(5, 0), 10, 10)
	ecsCollider(tw, b).Mask = 0
	*tw.vel(a) = v(0, 0)
	tw.step(10)
	if p := tw.pos(b); p != v(5, 0) {
		t.Errorf("b moved to %v, want (5, 0)", p)
	}
	if p := tw.pos(a); !near(p.X, -5, 0.1) {
		t.Errorf("a at %v, want pushed to x~-5", p)
	}
	// Both react: split by inverse mass (b is 3 times heavier).
	tw2 := newWorld(t, Config{})
	c := tw2.box(v(0, 0), 10, 10)
	d := tw2.box(v(8, 0), 10, 10)
	tw2.body(d).Mass = 3
	tw2.step(10)
	dc, dd := -tw2.pos(c).X, tw2.pos(d).X-8
	if !near(dc, 1.5, 0.1) || !near(dd, 0.5, 0.1) {
		t.Errorf("mass split: c moved %v, d moved %v, want 1.5 and 0.5", dc, dd)
	}
}

func TestSensorNotSolid(t *testing.T) {
	tw := newWorld(t, platformer)
	tw.wall(v(-50, 100), 200, 20)
	s := tw.wall(v(-50, 50), 200, 20)
	ecsCollider(tw, s).Sensor = true
	b := tw.box(v(0, 0), 8, 8)
	tw.step(30)
	if y := tw.pos(b).Y; y < 60 {
		t.Errorf("body stopped at y=%v by a sensor", y)
	}
}
