package anim

import (
	"time"

	"github.com/weakpixel/asset"
	"github.com/weakpixel/ecs"
	"github.com/weakpixel/ecs/system"
	"github.com/weakpixel/engine/gfx"
)

// AnimationSystem advances every entity that has a gfx.Animator and a
// gfx.Sprite: it adds the tick's delta (scaled by Animator.Speed) to
// Animator.Time, looks up the current frame and writes its sprite into
// Sprite.ID. If the frame has a pivot (Frame.Origin, from an *.anim.json
// file), it writes Sprite.Origin too, so a pivoted clip owns the Origin. Run it in the simulate stage, after the AssetSystem so that
// freshly loaded clips are Ready.
//
// It needs the *asset.Server resource. An entity whose clip handle is zero,
// or whose clip is not loaded (yet), keeps its Sprite as it is, so the
// sprite set by the game, or the placeholder, shows until the clip arrives.
// An entity whose Animator.Done is set is skipped. Run does not allocate.
type AnimationSystem struct {
	view  ecs.View2[gfx.Animator, gfx.Sprite]
	clips ClipStore
}

// NewAnimationSystem returns the system for w.
func NewAnimationSystem(w *ecs.World) *AnimationSystem {
	return &AnimationSystem{
		view: ecs.NewView2[gfx.Animator, gfx.Sprite](w),
	}
}

// Init implements system.Initializer. It caches the Clip store, so register
// the loaders (RegisterLoaders) and set the *asset.Server resource first.
func (s *AnimationSystem) Init(ctx *system.Context) {
	s.clips = ClipStoreOf(ecs.GetResource[asset.Server](ctx.World))
}

// Run implements system.Runner.
func (s *AnimationSystem) Run(ctx *system.Context) {
	for _, c := range s.view.Each() {
		an, sp := c.Unpack()
		if an.Done {
			continue
		}
		// Lookup, unlike Get, tells "not loaded" from "loaded": the
		// placeholder clip must not touch the sprite.
		clip, state := s.clips.Lookup(an.Clip)
		if state != asset.Ready || len(clip.Frames) == 0 {
			continue
		}
		speed := an.Speed
		if speed <= 0 {
			speed = 1
		}
		an.Time += time.Duration(float64(ctx.Time.Delta) * float64(speed))
		frame, done := clip.FrameAt(an.Time)
		an.Frame, an.Done = int32(frame), done
		f := &clip.Frames[frame]
		sp.ID = f.Sprite
		// A clip with a pivot (*.anim.json) owns Sprite.Origin; one without
		// leaves the game's Origin alone.
		if f.Origin != (gfx.Vec2{}) {
			sp.Origin = f.Origin
		}
	}
}
