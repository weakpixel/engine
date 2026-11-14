package collision

import (
	"github.com/weakpixel/ecs"
	"github.com/weakpixel/ecs/system"
)

// Config configures the [System] of a World.
type Config struct {
	// CellSize is the cell size of the default grid broad phases, in world
	// units; zero is DefaultCellSize. Raycast also queries the broad phases
	// in pieces of this length.
	CellSize float32
	// Broadphase replaces the grid of the dynamic colliders, refilled every
	// run; StaticBroadphase the grid of the static ones, refilled when one
	// of them changes. Nil uses a Grid of CellSize.
	Broadphase, StaticBroadphase Broadphase
	// Filter, when set, decides per detecting side whether self reacts to
	// other, after the mask test passed: a bullet ignores its shooter. It is
	// not called for tile grids.
	Filter func(self, other ecs.Entity) bool
	// Stay, when set, also sends a Stay event every run for each pair that
	// is still in contact. Off by default: a pair that stays in contact (a
	// hero on a floor, a crowd) then costs an event per tick; read
	// Space.Contacts instead where that is enough.
	Stay bool
}

// System runs collision detection on its World's [Space] every tick: it
// places the colliders, finds the contacts and sends [Enter] and [Exit]
// events (and [Stay] events with Config.Stay). Add it where the moves of the tick are done, usually at the end of
// the simulate stage; readers of the events and of [Space.Contacts] run
// after it in the same tick or before it in the next.
type System struct{ space *Space }

// NewSystem configures w's Space with cfg and returns the system that
// updates it.
//
// Panics with "collision: Space already has a system" when called twice for
// one World.
func NewSystem(w *ecs.World, cfg Config) *System {
	s := SpaceOf(w)
	if s.hasSystem {
		panic("collision: Space already has a system")
	}
	s.hasSystem = true
	s.filter = cfg.Filter
	if cfg.Stay {
		s.stay = system.EventsOf[Stay](w)
	}
	if cfg.CellSize > 0 {
		s.cellSize = cfg.CellSize
	}
	s.dyn, s.stat = cfg.Broadphase, cfg.StaticBroadphase
	if s.dyn == nil {
		s.dyn = NewGrid(cfg.CellSize)
	}
	if s.stat == nil {
		s.stat = NewGrid(cfg.CellSize)
	}
	return &System{space: s}
}

// Space returns the Space the system updates.
func (sys *System) Space() *Space { return sys.space }

// Run implements system.Runner: [System.Detect], then [System.Publish].
func (sys *System) Run(*system.Context) {
	sys.Detect()
	sys.Publish()
}

// Detect places the colliders and finds the contacts, without sending
// events: queries and [Space.Contacts] see the new state right after it.
// Package physics calls it once per substep and [System.Publish] once per
// tick, so events stay once per tick.
func (sys *System) Detect() { sys.space.detect() }

// Publish sends the [Enter], [Exit] (and [Stay]) events for the contacts of
// the last Detect, compared with the contacts of the Detect before the
// previous Publish. Detecting several times between two Publish calls
// reports only the state of the last Detect.
func (sys *System) Publish() { sys.space.diff() }
