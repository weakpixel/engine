package tween

import "github.com/weakpixel/ecs/system"

// System advances a [Tweens] by the tick's ctx.Time.Delta. The game.Engine
// runs it first in its camera stage: tweens started by game systems run in
// the same tick, the follow camera sees the tweened position, and the stage
// is paused with the rest of the game while a Tool pauses it.
type System struct {
	tw *Tweens
}

// NewSystem returns the system that advances tw.
func NewSystem(tw *Tweens) *System {
	if tw == nil {
		panic("tween: nil Tweens")
	}
	return &System{tw: tw}
}

// Run implements system.Runner.
func (s *System) Run(ctx *system.Context) {
	s.tw.Update(float32(ctx.Time.Delta.Seconds()))
}
