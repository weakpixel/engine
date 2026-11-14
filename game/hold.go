package game

import "github.com/weakpixel/ecs/system"

// Hold holds the game's [StageSimulate] until the returned release is
// called: its systems are skipped while at least one hold is active. The
// input, camera and post stages keep running (tweens advance, the follow
// camera moves, input is read), and so do the engine's own stages, so a
// screen transition can animate while gameplay stands still. Holds nest:
// each Hold needs its own release, and calling a release twice is a no-op.
// Simulation time keeps advancing. Hold may be called before the first
// Update and from any system.
func (e *Engine) Hold() (release func()) {
	e.holds++
	done := false
	return func() {
		if !done {
			done = true
			e.holds--
		}
	}
}

// Held reports whether a [Engine.Hold] is active, so StageSimulate is
// skipped. The startup screen holds the game on its own and does not count.
func (e *Engine) Held() bool { return e.holds > 0 }

// holdGate runs a stage slot's runners unless the engine is held (see
// [Engine.Hold]). It wraps the simulate slot around the gate of the Tool
// and the startup screen (when there is one), so the check costs one
// branch per tick.
type holdGate struct {
	inner []system.Runner
	holds *int
}

// Init forwards to every runner that has an Init, in order.
func (g *holdGate) Init(ctx *system.Context) {
	for _, r := range g.inner {
		if in, ok := r.(system.Initializer); ok {
			in.Init(ctx)
		}
	}
}

func (g *holdGate) Run(ctx *system.Context) {
	if *g.holds > 0 {
		return
	}
	for _, r := range g.inner {
		r.Run(ctx)
	}
}
