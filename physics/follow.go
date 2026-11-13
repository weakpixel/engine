package physics

import (
	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/tween"
)

// Follow moves a kinematic body to Target every tick by setting its
// Velocity to (Target - Pos) / dt, so a platform driven by a tween (see
// [FollowTarget]) is an ordinary kinematic body: it carries riders, drags
// crates by friction and pushes with momentum. Tweening Transform.Pos
// instead teleports the platform after physics has run.
//
// A fresh Follow takes the body's position as its target on the first tick
// physics sees it, so it never jumps to the origin. Removing Follow leaves
// the body with its last Velocity: zero it.
//
// +game:component
type Follow struct {
	// Target is the position the body reaches at the end of the tick.
	//
	// +game:runtime
	Target gfx.Vec2 `json:"-"`

	started bool // Target holds a real target
}

// FollowTarget is the tween lens of Follow.Target:
//
//	tween.To(tw, platform, physics.FollowTarget, gfx.Vec2{X: 300, Y: 80}, 2)
var FollowTarget = tween.NewLens(func(f *Follow) *gfx.Vec2 { return &f.Target }, tween.LerpVec2)
