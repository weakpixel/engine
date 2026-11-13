package prefab

import (
	"github.com/weakpixel/asset"
	"github.com/weakpixel/ecs"
	"github.com/weakpixel/engine/gfx"
)

// This file is the game-facing API for prefab assets, following
// tilemap/handles.go: a concrete handle type, Load helpers that return it,
// and a store that takes it.

// Handle refers to a [Prefab]. It is 8 bytes, comparable and pointer-free;
// the zero value means "none". It is an alias of [gfx.PrefabHandle] so
// components (a spawner's prefab field) can hold it without gfx importing
// prefab.
type Handle = gfx.PrefabHandle

// Load returns the handle for a prefab key
// ("global/prefabs/goblin.prefab.json") and adds a hold for sc. It never
// blocks and panics like [asset.Load].
func Load(sc *asset.Scope, key string) Handle {
	return gfx.PrefabHandle{Ref: asset.Load[Prefab](sc, key).Ref()}
}

// LoadSync is Load followed by loading until the prefab (and its base) is
// ready or failed, like [asset.LoadSync]. It blocks: use it for boot
// screens, tests and tools.
func LoadSync(sc *asset.Scope, key string) (Handle, error) {
	h, err := asset.LoadSync[Prefab](sc, key)
	return gfx.PrefabHandle{Ref: h.Ref()}, err
}

// Hold adds sc as an owner of h, like [asset.Hold].
func Hold(sc *asset.Scope, h Handle) {
	asset.Hold(sc, asset.HandleOf[Prefab](h.Ref))
}

// Store resolves [Handle] values. Get one with [StoreOf]. It is a small
// value; copy it freely.
type Store struct{ s *asset.Store[Prefab] }

// StoreOf returns the prefab store of srv. It panics like [asset.StoreOf]
// if [RegisterLoader] has not run on srv.
func StoreOf(srv *asset.Server) Store {
	return Store{asset.StoreOf[Prefab](srv)}
}

// Get returns the prefab, or the placeholder (which cannot spawn); see
// [asset.Store.Get] for the pointer validity rule. It does not allocate.
func (s Store) Get(h Handle) *Prefab { return s.s.Get(asset.HandleOf[Prefab](h.Ref)) }

// Lookup is Get plus the load state; see [asset.Store.Lookup].
func (s Store) Lookup(h Handle) (*Prefab, asset.State) {
	return s.s.Lookup(asset.HandleOf[Prefab](h.Ref))
}

// Ready reports whether h refers to a loaded prefab.
func (s Store) Ready(h Handle) bool { return s.s.Ready(asset.HandleOf[Prefab](h.Ref)) }

// Err returns why the prefab failed to load, or nil.
func (s Store) Err(h Handle) error { return s.s.Err(asset.HandleOf[Prefab](h.Ref)) }

// Key returns the key h was loaded from, or the zero Key for a zero or stale
// handle.
func (s Store) Key(h Handle) asset.Key { return s.s.Key(asset.HandleOf[Prefab](h.Ref)) }

// Version is 0 until h's prefab is loaded, 1 after, and goes up by one with
// every reload that replaced it (see [asset.Store.Version]).
func (s Store) Version(h Handle) uint32 { return s.s.Version(asset.HandleOf[Prefab](h.Ref)) }

// Generic returns the underlying generic store.
func (s Store) Generic() *asset.Store[Prefab] { return s.s }

// Spawn spawns the prefab h refers to with overrides (see [Prefab.Spawn]).
// It panics with "prefab: Spawn: <key> is not ready" if h is not Ready:
// check [Store.Ready], or load the prefab with the level and wait for it.
func (s Store) Spawn(h Handle, overrides ...any) ecs.Entity {
	p, state := s.Lookup(h)
	if state != asset.Ready {
		panic("prefab: Spawn: " + s.Key(h).String() + " is not ready (" + state.String() + ")")
	}
	return p.Spawn(overrides...)
}

// SpawnAll spawns the prefab h refers to and appends every entity to dst,
// root first (see [Prefab.SpawnAll]). It panics like [Store.Spawn].
func (s Store) SpawnAll(h Handle, dst []ecs.Entity, overrides ...any) []ecs.Entity {
	p, state := s.Lookup(h)
	if state != asset.Ready {
		panic("prefab: Spawn: " + s.Key(h).String() + " is not ready (" + state.String() + ")")
	}
	return p.SpawnAll(dst, overrides...)
}
