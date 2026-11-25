package game

import (
	"github.com/weakpixel/ecs/system"
	"github.com/weakpixel/engine/particles"
)

// SetAnimationsFrozen stops (on) or restarts everything that moves on its
// own, without a game system: sprite clips (gfx.Animator) and animated
// tiles keep their frame, particles stay where they are and no emitter
// spawns, and the shader Time uniform stays at its value. A clip that is
// set or loaded while frozen still shows its frame at the clip's time.
// [Engine.FrameChanged] then compares items with a shader and the post
// chain's passes by value, so a frozen scene that stands still reports no
// change. An editor freezes the animations in edit mode so an idle view
// draws nothing. Game systems run as before, and Play-time code should not
// rely on this: the tick's delta of the game stages is unchanged.
func (e *Engine) SetAnimationsFrozen(on bool) { e.frozen = on }

// AnimationsFrozen reports what [Engine.SetAnimationsFrozen] set last.
func (e *Engine) AnimationsFrozen() bool { return e.frozen }

// stillClock runs inner with a zero tick delta while *on is set, so a
// system that adds the delta to its clock (the animation system, the tile
// extract) stops there but still does its other work. It keeps inner's
// Init, which the schedule calls.
type stillClock struct {
	inner system.Runner
	on    *bool
	// still is the zero-delta copy of the stage's context. It lives here,
	// not on the stack: passed to an interface method, a local copy would
	// escape and allocate on every frozen tick.
	still system.Context
}

func (s *stillClock) Init(ctx *system.Context) {
	if i, ok := s.inner.(system.Initializer); ok {
		i.Init(ctx)
	}
}

func (s *stillClock) Run(ctx *system.Context) {
	if !*s.on {
		s.inner.Run(ctx)
		return
	}
	// A copy: the next runners of the stage see the real delta.
	s.still = *ctx
	s.still.Time.Delta = 0
	s.inner.Run(&s.still)
}

// stillParticles runs the particle simulation, or only keeps its pools
// while *on is set (a zero delta would burst every tick: an emitter's time
// would stay at 0).
type stillParticles struct {
	inner *particles.System
	on    *bool
}

func (s *stillParticles) Run(ctx *system.Context) {
	if *s.on {
		s.inner.RunPaused(ctx)
		return
	}
	s.inner.Run(ctx)
}
