package game

import (
	"fmt"

	"github.com/weakpixel/asset"
	"github.com/weakpixel/ecs"
	"github.com/weakpixel/ecs/system"
	"github.com/weakpixel/engine/tilemap"
)

// SceneOptions say how a [Scene] affects the scenes below it on the stack.
// The zero value is the pause/menu default: scenes below are frozen, get no
// input and are not drawn.
type SceneOptions struct {
	// UpdateBelow keeps the scenes below updating (their systems run) while
	// this scene is on the stack.
	UpdateBelow bool
	// InputBelow lets the scenes below run their [StageInput] systems. When
	// false, only the scenes above them read input, even if UpdateBelow
	// keeps them simulating.
	InputBelow bool
	// DrawBelow keeps the scenes below drawing. When false, items on layers
	// below Layer are not drawn (see render.Queue.SetMinLayer), so give
	// stacked scenes increasing layer bands (game 0 to 999, pause 1000 up).
	DrawBelow bool
	// Layer is the lowest layer this scene draws on when DrawBelow is false.
	Layer int16
}

// Scene is one top-level screen of the game (title, gameplay, pause): its own
// systems, levels, entities and asset scope, run while it is on the engine's
// scene stack. Create it with [NewScene], add systems, then [Engine.Push] it.
// A Scene belongs to one Engine. Like the Engine, it is single-goroutine.
type Scene struct {
	name string
	opts SceneOptions

	runners [numStages][]system.Runner
	// pushed is set when the scene is first pushed (the root: at the first
	// Update); its systems are fixed from then on.
	pushed bool
	engine *Engine

	levels []*Level
	owned  []ecs.Entity
	scope  *asset.Scope

	enter, exit, cover, uncover func(*Engine, *Scene)
}

// NewScene returns a scene named name (for debugging and its asset scope)
// with the given options. Nothing runs until it is pushed.
func NewScene(name string, opts SceneOptions) *Scene {
	return &Scene{name: name, opts: opts}
}

// Name returns the name given to [NewScene] ("root" for [Engine.Root]).
func (s *Scene) Name() string { return s.name }

// Options returns the scene's options.
func (s *Scene) Options() SceneOptions { return s.opts }

// AddSystem appends systems to a stage slot of the scene; within a slot they
// run in the order added, after the systems of the scenes below. Like
// [Engine.AddSystem], plain functions get no Init call.
//
// Panics with "game: AddSystem after the scene was pushed" once the scene
// has been pushed, with "game: invalid stage" for an unknown stage and with
// "game: nil system".
func (s *Scene) AddSystem(st Stage, sys ...system.System) {
	runners := make([]system.Runner, len(sys))
	for i, fn := range sys {
		if fn == nil {
			panicf("nil system")
		}
		runners[i] = fn
	}
	s.AddRunner(st, runners...)
}

// AddRunner is like [Scene.AddSystem] for [system.Runner] values. A runner
// that implements system.Initializer is initialized when the scene is first
// pushed, once per Scene value.
//
// Panics like [Scene.AddSystem].
func (s *Scene) AddRunner(st Stage, runners ...system.Runner) {
	if s.pushed {
		panicf("AddSystem after the scene was pushed")
	}
	if st < 0 || st >= numStages {
		panicf("invalid stage")
	}
	for _, r := range runners {
		if r == nil {
			panicf("nil system")
		}
	}
	s.runners[st] = append(s.runners[st], runners...)
}

// OnEnter registers fn to run each time the scene is pushed, after its Init
// (the first time) and with the scene already on the stack. Spawn the
// scene's entities and load its levels here.
func (s *Scene) OnEnter(fn func(*Engine, *Scene)) { s.enter = fn }

// OnExit registers fn to run when the scene is popped or replaced, before
// its levels, owned entities and scope are cleaned up.
func (s *Scene) OnExit(fn func(*Engine, *Scene)) { s.exit = fn }

// OnCover registers fn to run when another scene is pushed on top of this
// one (not for a [Engine.Replace] of the scene above it).
func (s *Scene) OnCover(fn func(*Engine, *Scene)) { s.cover = fn }

// OnUncover registers fn to run when the scene above this one is popped (not
// for a [Engine.Replace]).
func (s *Scene) OnUncover(fn func(*Engine, *Scene)) { s.uncover = fn }

// LoadLevel is [Engine.LoadLevel] for a level the scene owns: popping the
// scene unloads it. It returns an error if the scene was never pushed.
func (s *Scene) LoadLevel(path string) (*Level, error) {
	if err := s.checkEngine("LoadLevel"); err != nil {
		return nil, err
	}
	return s.own(s.engine.LoadLevel(path))
}

// LoadMap is [Engine.LoadMap] for a level the scene owns.
func (s *Scene) LoadMap(path string, opts tilemap.SpawnOptions) (*Level, error) {
	if err := s.checkEngine("LoadMap"); err != nil {
		return nil, err
	}
	return s.own(s.engine.LoadMap(path, opts))
}

// LoadLevelBase is [Engine.LoadLevelBase] for a level the scene owns.
func (s *Scene) LoadLevelBase(base string, opts tilemap.SpawnOptions) (*Level, error) {
	if err := s.checkEngine("LoadLevelBase"); err != nil {
		return nil, err
	}
	return s.own(s.engine.LoadLevelBase(base, opts))
}

func (s *Scene) checkEngine(op string) error {
	if s.engine == nil {
		return fmt.Errorf("game: Scene.%s: scene %q was never pushed", op, s.name)
	}
	return nil
}

// own records a level the engine loaded for the scene.
func (s *Scene) own(l *Level, err error) (*Level, error) {
	if err == nil {
		s.levels = append(s.levels, l)
	}
	return l, err
}

// Own records entities spawned by the scene's systems; popping the scene
// deletes those still alive. Entities of the scene's levels need no Own.
func (s *Scene) Own(entities ...ecs.Entity) { s.owned = append(s.owned, entities...) }

// Scope returns the asset scope of the scene, named "scene/<name>" and
// created on first use. Popping the scene releases it; a scene pushed again
// gets a new one.
//
// Panics with "game: Scope before the scene was pushed" for a scene that was
// never pushed.
func (s *Scene) Scope() *asset.Scope {
	if s.engine == nil {
		panicf("Scope before the scene was pushed")
	}
	if s.scope == nil {
		s.scope = s.engine.assets.NewScope("scene/" + s.name)
	}
	return s.scope
}

// cleanup unloads the scene's levels, deletes its owned entities that are
// still alive and releases its scope, so a pushed again scene starts empty.
func (s *Scene) cleanup() {
	e := s.engine
	for i := len(s.levels) - 1; i >= 0; i-- {
		e.UnloadLevel(s.levels[i])
	}
	for _, en := range s.owned {
		if e.world.IsAlive(en) {
			e.world.DeleteEntity(en)
		}
	}
	if s.scope != nil {
		s.scope.Release()
	}
	s.levels, s.owned, s.scope = nil, nil, nil
}
