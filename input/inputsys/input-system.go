// Package inputsys connects an input.Router to an ecs World: [InputSystem]
// updates the router once per tick and publishes it as a resource, and
// [TouchControlsExtract] draws the virtual touch controls. game.Engine adds
// both; wire them by hand when using the low-level packages.
package inputsys

import (
	"fmt"
	"log"

	"github.com/weakpixel/asset"
	"github.com/weakpixel/ecs"
	"github.com/weakpixel/ecs/system"
	"github.com/weakpixel/engine/input"
)

// InputSystem runs [input.Router.Update] once per tick. Add it first in the
// input stage, so every later system in the tick reads the same action
// state.
//
// Init publishes the World resources *input.Router and *input.Events (the
// Router's events, reused every tick).
//
// With [InputSystem.Watch] it also follows a bindings asset: whenever the
// asset is committed (first load, and reloads), the router gets the new
// default bindings before the tick's Update. A file that fails to load is
// reported once through OnError and the router keeps its bindings.
type InputSystem struct {
	// OnError receives the load error of the watched bindings. nil logs it.
	OnError func(error)

	router   *input.Router
	store    BindingsStore
	handle   BindingsHandle
	watching bool
	version  uint32
	reported bool
}

// New returns the system for r. Panics with "input: " if r is nil.
func New(r *input.Router) *InputSystem {
	if r == nil {
		panic("input: inputsys.New: nil Router")
	}
	return &InputSystem{router: r}
}

// Router returns the router the system updates.
func (s *InputSystem) Router() *input.Router { return s.router }

// Watch makes the system apply the bindings asset h whenever it becomes
// ready or is committed again. srv must have the input loaders
// ([RegisterLoaders]); Watch panics like [BindingsStoreOf]
// otherwise. A second call replaces the watched handle.
func (s *InputSystem) Watch(srv *asset.Server, h BindingsHandle) {
	s.store = BindingsStoreOf(srv)
	s.handle = h
	s.watching = true
	s.version = 0
	s.reported = false
}

// Init implements system.Initializer.
func (s *InputSystem) Init(ctx *system.Context) {
	ecs.SetResource(ctx.World, s.router)
	ecs.SetResource(ctx.World, s.router.Events())
}

// Run implements system.Runner.
func (s *InputSystem) Run(*system.Context) {
	if s.watching {
		s.follow()
	}
	s.router.Update()
}

// follow applies the watched bindings when their version changed.
func (s *InputSystem) follow() {
	if err := s.store.Err(s.handle); err != nil {
		if !s.reported {
			s.reported = true
			s.onError(fmt.Errorf("input: bindings: %w", err))
		}
		return
	}
	v := s.store.Version(s.handle)
	if v == s.version || !s.store.Ready(s.handle) {
		return
	}
	s.version = v
	s.reported = false
	// Copy the value: the store's pointer is only valid until the asset is
	// committed again, while the router keeps its bindings indefinitely.
	// The compiled tables inside are immutable and shared.
	b := new(input.Bindings)
	*b = *s.store.Get(s.handle)
	s.router.SetBindings(b)
}

func (s *InputSystem) onError(err error) {
	if s.OnError != nil {
		s.OnError(err)
		return
	}
	log.Print(err)
}
