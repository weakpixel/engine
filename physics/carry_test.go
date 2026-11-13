package physics

import (
	"testing"

	"github.com/weakpixel/ecs"
	"github.com/weakpixel/engine/collision"
	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/xmath"
)

func TestOnGroundFlags(t *testing.T) {
	tw := newWorld(t, platformer)
	floor := tw.wall(v(-100, 100), 300, 20)
	tw.wall(v(30, 0), 20, 100)   // wall right of the box
	tw.wall(v(-100, 80), 130, 6) // ceiling over the box (y 80..86)
	b := tw.box(v(18, 88), 12, 12, Touching{})
	plain := tw.box(v(-60, 88), 12, 12)
	tw.step(2)
	*tw.vel(b) = v(100, -300) // into the wall and the ceiling
	tw.step(2)                // flags come from the contacts detected at the start of a substep
	got := *ecs.GetComp[Touching](tw.w, b)
	if !got.OnCeiling || got.OnWall != 1 || got.OnGround {
		t.Errorf("into the corner: %+v, want OnCeiling and OnWall=+1", got)
	}
	tw.step(30)
	got = *ecs.GetComp[Touching](tw.w, b)
	if !got.OnGround || got.Ground != floor || got.GroundNormal != v(0, -1) {
		t.Errorf("resting: %+v, want OnGround on %v with normal (0,-1)", got, floor)
	}
	if ecs.HasComp[Touching](tw.w, plain) {
		t.Error("physics added Touching to a body without one")
	}
}

// platformWorld has a kinematic platform at (0, 100), 64 x 8, and a
// rider resting on it.
func platformWorld(t *testing.T, touching bool) (tw *world, platform, rider ecs.Entity) {
	tw = newWorld(t, platformer)
	platform = tw.w.NewEntity(gfx.Transform{Pos: v(0, 100)},
		collision.Collider{Size: v(64, 8), Layer: lWall}, Body{Kind: Kinematic}, Velocity{})
	if touching {
		rider = tw.box(v(20, 88), 12, 12, Touching{})
	} else {
		rider = tw.box(v(20, 88), 12, 12)
	}
	tw.step(10)
	return tw, platform, rider
}

func TestRiderMovesWithPlatform(t *testing.T) {
	tw, platform, rider := platformWorld(t, true)
	off := tw.pos(rider).Sub(tw.pos(platform))
	*tw.vel(platform) = v(90, 0)
	tw.step(60)
	*tw.vel(platform) = v(0, -60) // up
	tw.step(30)
	*tw.vel(platform) = v(-50, 80) // down-left, slower than a fall
	tw.step(30)
	if got := tw.pos(rider).Sub(tw.pos(platform)); !near(got.X, off.X, 0.1) || !near(got.Y, off.Y, 0.5) {
		t.Errorf("rider offset %v, want %v", got, off)
	}
	if ts := ecs.GetComp[Touching](tw.w, rider); !ts.OnGround || ts.Ground != platform {
		t.Errorf("rider Touching %+v, want on the platform", *ts)
	}
}

func TestCarryWithoutTouching(t *testing.T) {
	tw, platform, rider := platformWorld(t, false)
	off := tw.pos(rider).Sub(tw.pos(platform))
	*tw.vel(platform) = v(120, 0)
	tw.step(60)
	if got := tw.pos(rider).Sub(tw.pos(platform)); !near(got.X, off.X, 0.1) {
		t.Errorf("rider offset %v, want %v", got, off)
	}
}

func TestLeavePlatformKeepsItsVelocity(t *testing.T) {
	tw, platform, rider := platformWorld(t, false)
	*tw.vel(platform) = v(120, 0)
	tw.step(10)
	if vx := tw.vel(rider).X; !near(vx, 0, 0.01) {
		t.Errorf("rider velocity on the platform %v, want 0 (relative)", vx)
	}
	tw.vel(rider).Y = -400 // jump
	tw.step(3)
	if vx := tw.vel(rider).X; !near(vx, 120, 0.01) {
		t.Errorf("rider velocity after the jump %v, want the platform's 120", vx)
	}
	_ = platform
}

// TestMoverRidesPlatform runs a MoveAndSlide mover with gravity on a
// kinematic platform that moves up, down and sideways.
func TestMoverRidesPlatform(t *testing.T) {
	tw := newWorld(t, platformer)
	platform := tw.w.NewEntity(gfx.Transform{Pos: v(0, 100)},
		collision.Collider{Size: v(64, 8), Layer: lWall}, Body{Kind: Kinematic}, Velocity{})
	tw.step(1)
	hero := tw.w.NewEntity(gfx.Transform{Pos: v(30, 86)}, Velocity{}, Touching{},
		collision.Collider{Size: v(10, 14), Layer: lBody, Mask: lWall})
	var worst float32
	for _, pv := range []gfx.Vec2{v(0, -60), v(0, 80), v(70, -40), v(-90, 0)} {
		*tw.vel(platform) = pv
		for range 40 {
			vel := tw.vel(hero)
			vel.Y += 980.0 / 60
			res := MoveAndSlide(tw.w, hero, vel.Mul(1.0/60), MoveOptions{})
			if res.HitY || (res.OnGround && vel.Y > 0) {
				vel.Y = 0
			}
			tw.step(1)
			worst = max(worst, xmath.Abs(tw.pos(platform).Y-(tw.pos(hero).Y+14)))
		}
	}
	if worst > 1 {
		t.Errorf("mover drifted %v from the platform top, want at most 1", worst)
	}
	if off := tw.pos(hero).X - tw.pos(platform).X; !near(off, 30, 0.01) {
		t.Errorf("mover offset on the platform %v, want 30 (carried sideways)", off)
	}
}

func TestStackRidesPlatform(t *testing.T) {
	tw, platform, lower := platformWorld(t, false)
	upper := tw.box(v(20, 76), 12, 12)
	tw.step(20)
	gap := tw.pos(lower).Y - tw.pos(upper).Y
	for _, pv := range []gfx.Vec2{v(0, -120), v(0, 150), v(0, -60), v(0, 0)} {
		*tw.vel(platform) = pv
		for range 20 {
			tw.step(1)
			if g := tw.pos(lower).Y - tw.pos(upper).Y; !near(g, gap, 0.1) {
				t.Fatalf("platform at %v: stack gap %v, want %v", pv, g, gap)
			}
		}
	}
}
