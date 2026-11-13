package physics

import (
	"testing"

	"github.com/weakpixel/ecs"
	"github.com/weakpixel/engine/collision"
	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/tween"
)

// followWorld has a kinematic Follow platform at (0, 100), 64 x 8, and a
// tween system that runs after physics, as in the engine.
func followWorld(t *testing.T) (tw *world, tws *tween.Tweens, platform ecs.Entity) {
	tw = newWorld(t, platformer)
	tws = tween.New(tw.w)
	tw.sched.AddRunner(tween.NewSystem(tws))
	platform = tw.w.NewEntity(gfx.Transform{Pos: v(0, 100)},
		collision.Collider{Size: v(64, 8), Layer: lWall}, Body{Kind: Kinematic}, Velocity{}, Follow{})
	return tw, tws, platform
}

func TestFollowInitTarget(t *testing.T) {
	tw, _, platform := followWorld(t)
	tw.step(10)
	if p := tw.pos(platform); p != v(0, 100) {
		t.Errorf("fresh Follow moved the platform to %v", p)
	}
}

func TestFollowReachesTarget(t *testing.T) {
	tw, _, platform := followWorld(t)
	tw.step(1)
	ecs.GetComp[Follow](tw.w, platform).Target = v(30, 70)
	tw.step(1)
	if p := tw.pos(platform); !near(p.X, 30, 1e-3) || !near(p.Y, 70, 1e-3) {
		t.Errorf("after a tick at %v, want the target (30, 70)", p)
	}
	tw.step(5)
	if p, vel := tw.pos(platform), *tw.vel(platform); !near(p.X, 30, 1e-3) || vel != (gfx.Vec2{}) {
		t.Errorf("at the target: pos %v vel %v, want still", p, vel)
	}
}

func TestFollowCarriesRider(t *testing.T) {
	tw, tws, platform := followWorld(t)
	rider := tw.box(v(20, 88), 12, 12)
	tw.step(10)
	off := tw.pos(rider).Sub(tw.pos(platform))
	tween.To(tws, platform, FollowTarget, v(200, 40), 1).Ease(tween.InOutSine).Yoyo().Repeat(-1)
	for range 60 * 2 { // a full yoyo cycle
		tw.step(1)
		if got := tw.pos(rider).Sub(tw.pos(platform)); !near(got.X, off.X, 0.1) || !near(got.Y, off.Y, 0.5) {
			t.Fatalf("rider offset %v, want %v", got, off)
		}
	}
	if p := tw.pos(platform); !near(p.X, 0, 0.5) || !near(p.Y, 100, 0.5) {
		t.Errorf("platform back at %v, want (0, 100)", p)
	}
}

func TestFollowPushesCrateWithMomentum(t *testing.T) {
	// A tweened pusher on an ice floor: the crate gets the pusher's speed
	// from the contact, and keeps sliding when the pusher stops. With a
	// Transform.Pos tween it would only be pushed out, without speed.
	tw, tws, pusher := followWorld(t)
	tw.wall(v(-100, 108), 600, 20, Material{})
	crate := tw.box(v(70, 96), 12, 12, Material{})
	tw.step(10)
	tween.To(tws, pusher, FollowTarget, v(90, 100), 0.5)
	tw.step(32)
	if vx := tw.vel(crate).X; vx < 100 {
		t.Errorf("crate speed %v after the push, want about the pusher's 180", vx)
	}
	x := tw.pos(crate).X
	tw.step(30)
	if tw.pos(crate).X <= x+40 {
		t.Errorf("crate stopped with the pusher: x %v -> %v", x, tw.pos(crate).X)
	}
}
