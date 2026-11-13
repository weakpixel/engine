package game

import (
	"image"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/weakpixel/ecs/system"
	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/input"
)

// Tool is a development tool attached to an Engine with [Config.Tool], for
// example a level editor. It can pause and step the game, filter the input
// the game sees and draw over the finished frame at window resolution. The
// engine never implements one: a game that sets no Tool links no tool code
// and pays one nil check per Update and Draw.
type Tool interface {
	// Init is called once, at the end of [New], when every engine part
	// exists. The schedule is not built yet.
	Init(e *Engine)
	// Input is called once, in [New] before Init, with the device source
	// (Ebiten's, or InputConfig.Source). The router reads the source it returns, so a
	// tool can hide the clicks its own UI used from the game. Returning src
	// leaves input unchanged.
	Input(src input.Source) input.Source
	// Tick is called at the start of every Update, after the Start hooks of
	// the first one, and decides how this tick runs.
	Tick(e *Engine) TickMode
	// DrawFinal is called at the end of [Engine.DrawFinalScreen], after the
	// final pass, to draw in window pixels (use [Engine.WindowToLogical] to
	// map the mouse into the game).
	DrawFinal(e *Engine, screen ebiten.FinalScreen)
}

// TickMode is how one Update runs; a [Tool] returns it from Tick.
type TickMode int

const (
	// TickRun runs the whole schedule (the default).
	TickRun TickMode = iota
	// TickPause skips the game: the input stage (including the engine's
	// input system) and every system in StageSimulate, StageCamera and
	// StagePost. The engine's own stages still run: assets load and
	// hot-reload, transforms are snapshotted, animations play, the frame is
	// extracted and audio is updated, so the paused scene keeps drawing and
	// stays live. Simulation time (the shader Time uniform) keeps advancing.
	TickPause
	// TickStep runs one full tick, like TickRun. A tool returns it once
	// while paused to advance the game by one tick, then TickPause again.
	TickStep
)

// String returns "run", "pause" or "step".
func (m TickMode) String() string {
	switch m {
	case TickRun:
		return "run"
	case TickPause:
		return "pause"
	case TickStep:
		return "step"
	}
	return "invalid"
}

// Paused reports whether the current (or, outside Update, the last) tick ran
// in [TickPause] mode.
func (e *Engine) Paused() bool { return e.paused }

// WindowToLogical converts a position in window pixels (what
// ebiten.CursorPosition reports to a Tool, and what DrawFinal draws in) to
// logical screen pixels, the space render.Camera.ScreenToWorld takes. It
// inverts the scale and offset of the most recent [Engine.DrawFinalScreen];
// before the first one, it is the identity.
func (e *Engine) WindowToLogical(p image.Point) gfx.Vec2 {
	inv := e.finalGeoM
	if inv.IsInvertible() {
		inv.Invert()
	}
	x, y := inv.Apply(float64(p.X), float64(p.Y))
	return gfx.Vec2{X: float32(x), Y: float32(y)}
}

// LogicalToWindow is the inverse of [Engine.WindowToLogical]: it converts
// logical screen pixels (what render.Camera.WorldToScreen returns) to window
// pixels, where a Tool's DrawFinal draws.
func (e *Engine) LogicalToWindow(p gfx.Vec2) gfx.Vec2 {
	x, y := e.finalGeoM.Apply(float64(p.X), float64(p.Y))
	return gfx.Vec2{X: float32(x), Y: float32(y)}
}

// EditRunner is an optional interface for a system.Runner. A runner whose
// RunsWhilePaused returns true keeps running while a [Tool] pauses the game
// ([TickPause]), so an edited scene gets the setup and presentation work it
// needs (placing the hero once a world has spawned, merging layers). Use it
// for that, never for simulation. It is asked once, when the schedule is
// built, and has no effect without a Tool.
type EditRunner interface {
	RunsWhilePaused() bool
}

// gate runs a stage slot's runners, or only its [EditRunner]s while a Tool
// pauses the engine, or only the engine's own runners of the slot while the
// startup screen holds the game. It is installed only when a Tool is set or
// the startup screen is on, so other games keep calling their systems
// directly. One gate wraps a whole slot and
// the runner lists are split once at schedule build time, so the paused check
// costs one branch per stage, not per system.
type gate struct {
	runners []system.Runner // every runner, in order
	editing []system.Runner // the subset that runs while paused, in order
	engine  []system.Runner // the engine's runners that run while held
	paused  *bool
	held    *bool
}

// newGate splits runners into the full list and the paused subset.
func newGate(runners, engine []system.Runner, paused, held *bool) *gate {
	g := &gate{runners: runners, engine: engine, paused: paused, held: held}
	for _, r := range runners {
		if er, ok := r.(EditRunner); ok && er.RunsWhilePaused() {
			g.editing = append(g.editing, r)
		}
	}
	return g
}

// Init forwards to every runner that has an Init, in order, so wrapped
// systems are initialized exactly as unwrapped ones.
func (g *gate) Init(ctx *system.Context) {
	for _, r := range g.runners {
		if in, ok := r.(system.Initializer); ok {
			in.Init(ctx)
		}
	}
}

func (g *gate) Run(ctx *system.Context) {
	rs := g.runners
	switch {
	case *g.paused:
		rs = g.editing
	case *g.held:
		rs = g.engine
	}
	for _, r := range rs {
		r.Run(ctx)
	}
}
