package physics

import (
	"testing"

	"github.com/weakpixel/ecs"
	"github.com/weakpixel/ecs/system"
	"github.com/weakpixel/engine/collision"
)

func TestPreSolveIgnore(t *testing.T) {
	var ghost ecs.Entity
	tw := newWorld(t, Config{Gravity: v(0, 980), PreSolve: func(c *Contact) bool {
		return c.Self != ghost && c.Other != ghost
	}})
	tw.wall(v(-50, 100), 200, 20)
	ghost = tw.box(v(0, 0), 8, 8)
	solid := tw.box(v(40, 0), 8, 8)
	tw.step(60)
	if y := tw.pos(ghost).Y; y < 120 {
		t.Errorf("ignored body stopped at y=%v", y)
	}
	if y := tw.pos(solid).Y; !near(y, 92, 0.5) {
		t.Errorf("other body at y=%v, want resting at 92", y)
	}
}

func TestPreSolveTrampoline(t *testing.T) {
	tw := newWorld(t, Config{Gravity: v(0, 980), Substeps: 4, PreSolve: func(c *Contact) bool {
		if c.Normal.Y < -0.7 { // hit from above
			c.Bounce = 1
		}
		return true
	}})
	tw.wall(v(-50, 200), 200, 20)
	b := tw.box(v(0, 92), 8, 8)
	top := float32(1e9)
	for range 90 {
		tw.step(1)
		if tw.vel(b).Y < 0 {
			top = min(top, tw.pos(b).Y)
		}
	}
	if !near(top, 92, 10) {
		t.Errorf("bounce 1 from PreSolve: apex y=%v, want ~92", top)
	}
}

func TestEventsOncePerTick(t *testing.T) {
	tw := newWorld(t, Config{Gravity: v(0, 980), Substeps: 4})
	enter := system.NewEventReader[collision.Enter](tw.w)
	impact := system.NewEventReader[Impact](tw.w)
	tw.wall(v(-50, 100), 200, 20)
	b := tw.box(v(0, 0), 8, 8)
	var enters, impacts int
	for range 120 {
		tw.step(1)
		for range enter.Read() {
			enters++
		}
		for ev := range impact.Read() {
			if ev.Self != b {
				t.Errorf("Impact.Self = %v, want %v", ev.Self, b)
			}
			impacts++
		}
	}
	if enters != 1 || impacts != 1 {
		t.Errorf("landing sent %d Enter and %d Impact, want 1 each", enters, impacts)
	}
}

func TestImpactSpeed(t *testing.T) {
	tw := newWorld(t, platformer)
	impact := system.NewEventReader[Impact](tw.w)
	g := newGrid().row(4, -2, 4)
	tw.sys.Space().AddTiles(g, collision.TileOptions{Layer: lWall})
	tw.box(v(0, 0), 8, 8)
	for range 60 {
		tw.step(1)
		for ev := range impact.Read() {
			// Bottom falls 56 units: about sqrt(2 * 980 * 56) = 331.
			if ev.Tiles != g || ev.Normal != v(0, -1) || !near(ev.Speed, 331, 20) {
				t.Errorf("Impact %+v, want tiles, normal (0,-1), speed ~331", ev)
			}
			return
		}
	}
	t.Error("no Impact")
}

func TestMutualPairSolvedOnce(t *testing.T) {
	// Two equal bodies in each other's masks, overlapping by 4: each is
	// pushed 2 by one constraint (two would push each 4).
	tw := newWorld(t, Config{})
	a := tw.box(v(0, 0), 10, 10)
	b := tw.box(v(6, 0), 10, 10)
	var n int
	tw.sys.pre = func(c *Contact) bool { n++; return true }
	tw.step(1)
	if n != 1 {
		t.Errorf("PreSolve saw %d contacts, want 1 merged pair", n)
	}
	dx := tw.pos(b).X - tw.pos(a).X
	if want := float32(6 + 0.8*(4-slop)); !near(dx, want, 0.01) {
		t.Errorf("distance after one substep %v, want %v", dx, want)
	}
}
