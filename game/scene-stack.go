package game

import (
	"math"
	"slices"
	"time"

	"github.com/weakpixel/ecs/system"
)

// sceneOp is a queued stack change.
type sceneOp struct {
	kind  sceneOpKind
	scene *Scene
}

type sceneOpKind uint8

const (
	opPush sceneOpKind = iota
	opPop
	opReplace
)

// Root returns the root scene: the systems added with [Engine.AddSystem] and
// [Engine.AddRunner]. It is always the bottom of the stack and cannot be
// popped or replaced. A game that never pushes a scene only has the root.
func (e *Engine) Root() *Scene { return e.root }

// Top returns the topmost scene of the stack (the root when nothing was
// pushed).
func (e *Engine) Top() *Scene { return e.stack[len(e.stack)-1] }

// Scenes returns the scene stack, bottom to top, root first. The slice is a
// copy.
func (e *Engine) Scenes() []*Scene { return slices.Clone(e.stack) }

// Push puts s on top of the stack. Push, Pop and Replace only queue the
// change: [Engine.Update] applies the queue in call order at its start,
// before any system runs, so systems never see a half-switched stack. Called
// before the first Update or from a Start hook, they apply before the first
// tick.
//
// Applying a push sends OnCover to the old top, initializes the runners of
// s the first time it is pushed (before the root's own Init on the very first
// tick), then sends OnEnter to s.
//
// Panics with "game: nil scene" for a nil s. Applying it panics with "game:
// scene already on the stack" if s is on the stack.
func (e *Engine) Push(s *Scene) {
	e.queueScene(opPush, s)
}

// Pop removes the topmost scene: OnExit, then its levels are unloaded, its
// owned entities deleted and its scope released, then OnUncover goes to the
// scene below. The scene may be pushed again; its levels and scope start
// empty and its Init does not run again.
//
// Applying it panics with "game: Pop with only the root scene" when only the
// root is left.
func (e *Engine) Pop() { e.queueScene(opPop, nil) }

// Replace pops the topmost scene and pushes s in its place, without OnUncover
// and OnCover in between. Replacing with the scene itself reenters it.
//
// Panics like [Engine.Push]; applying it panics like [Engine.Pop] when only
// the root is left.
func (e *Engine) Replace(s *Scene) { e.queueScene(opReplace, s) }

func (e *Engine) queueScene(kind sceneOpKind, s *Scene) {
	if kind != opPop && s == nil {
		panicf("nil scene")
	}
	e.sceneOps = append(e.sceneOps, sceneOp{kind, s})
}

// applySceneOps applies the queued operations in call order and rebuilds the
// dispatch lists if there were any. Hooks may queue more; they apply in the
// same pass.
func (e *Engine) applySceneOps(dt time.Duration) {
	if len(e.sceneOps) == 0 {
		return
	}
	for i := 0; i < len(e.sceneOps); i++ {
		op := e.sceneOps[i]
		switch op.kind {
		case opPush:
			e.pushScene(op.scene, dt, true)
		case opPop:
			e.popScene(true)
		case opReplace:
			e.popScene(false)
			e.pushScene(op.scene, dt, false)
		}
	}
	clear(e.sceneOps)
	e.sceneOps = e.sceneOps[:0]
	e.rebuildActive()
}

func (e *Engine) pushScene(s *Scene, dt time.Duration, cover bool) {
	if slices.Contains(e.stack, s) {
		panicf("scene already on the stack")
	}
	if old := e.Top(); cover && old.cover != nil {
		old.cover(e, old)
	}
	s.engine = e
	if !s.pushed {
		s.pushed = true
		e.initScene(s, dt)
	}
	e.stack = append(e.stack, s)
	if s.enter != nil {
		s.enter(e, s)
	}
}

func (e *Engine) popScene(uncover bool) {
	if len(e.stack) == 1 {
		panicf("Pop with only the root scene")
	}
	s := e.Top()
	if s.exit != nil {
		s.exit(e, s)
	}
	s.cleanup()
	e.stack[len(e.stack)-1] = nil
	e.stack = e.stack[:len(e.stack)-1]
	if top := e.Top(); uncover && top.uncover != nil {
		top.uncover(e, top)
	}
}

// initScene runs Init on the scene's runners, in stage order, outside the
// schedule (which only initializes what it was built with), then applies the
// commands they queued like the schedule does.
func (e *Engine) initScene(s *Scene, dt time.Duration) {
	cmds := system.CommandsOf(e.world)
	ctx := system.Context{World: e.world, Time: system.Time{Delta: dt, Tick: e.ticks}, Commands: cmds}
	for st := range numStages {
		for _, r := range s.runners[st] {
			if in, ok := r.(system.Initializer); ok {
				in.Init(&ctx)
			}
		}
	}
	cmds.Apply(e.world)
}

// rebuildActive recomputes what each stage dispatcher runs and the layer
// floor from the stack. A scene is active when every scene above it has
// UpdateBelow, and runs its input stage when also every scene above has
// InputBelow. It allocates only when the stack changed.
func (e *Engine) rebuildActive() {
	for st := range numStages {
		e.active[st] = e.active[st][:0]
		e.editing[st] = e.editing[st][:0]
	}
	n := len(e.stack)
	first, input := n-1, n-1 // lowest active scene, lowest scene reading input
	for first > 0 && e.stack[first].opts.UpdateBelow {
		first--
	}
	for input > first && e.stack[input].opts.InputBelow {
		input--
	}
	for i := first; i < n; i++ {
		for st := range numStages {
			if st == StageInput && i < input {
				continue
			}
			for _, r := range e.stack[i].runners[st] {
				e.active[st] = append(e.active[st], r)
				if er, ok := r.(EditRunner); ok && er.RunsWhilePaused() {
					e.editing[st] = append(e.editing[st], r)
				}
			}
		}
	}
	e.minLayer = math.MinInt16
	for i := n - 1; i > 0; i-- {
		if s := e.stack[i]; !s.opts.DrawBelow {
			e.minLayer = s.opts.Layer
			break
		}
	}
}

// dispatcher runs the active scenes' runners of one stage, bottom to top,
// where the root's own runners were before. The engine's runners of the slot
// (input system, tweens, camera) stay outside it. It is an EditRunner so a
// Tool's pause reaches the scene runners that are EditRunners themselves.
type dispatcher struct {
	e  *Engine
	st Stage
}

// RunsWhilePaused implements EditRunner.
func (d *dispatcher) RunsWhilePaused() bool { return true }

// Init initializes the root's runners; the schedule only knows the
// dispatcher. Scenes initialize their runners when pushed.
func (d *dispatcher) Init(ctx *system.Context) {
	for _, r := range d.e.root.runners[d.st] {
		if in, ok := r.(system.Initializer); ok {
			in.Init(ctx)
		}
	}
}

func (d *dispatcher) Run(ctx *system.Context) {
	rs := d.e.active[d.st]
	if d.e.paused {
		rs = d.e.editing[d.st]
	}
	for _, r := range rs {
		r.Run(ctx)
	}
}
