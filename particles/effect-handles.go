package particles

import (
	"github.com/weakpixel/asset"
	"github.com/weakpixel/engine/gfx"
)

// This file is the game-facing API for the effect kind: Load helpers that
// return gfx.EffectHandle (the handle gfx.Particles holds) and a store that
// takes it, like the clip helpers in package anim.

// LoadEffect returns the handle for an effect key ("fx/embers.fx.json") and
// adds a hold for sc. It never blocks and panics like [asset.Load].
func LoadEffect(sc *asset.Scope, key string) gfx.EffectHandle {
	return gfx.EffectHandle{Ref: asset.Load[Effect](sc, key).Ref()}
}

// LoadEffectSync is LoadEffect followed by loading until the effect is ready
// or failed, like [asset.LoadSync]. It blocks: use it for boot screens, tests
// and tools.
func LoadEffectSync(sc *asset.Scope, key string) (gfx.EffectHandle, error) {
	h, err := asset.LoadSync[Effect](sc, key)
	return gfx.EffectHandle{Ref: h.Ref()}, err
}

// HoldEffect adds sc as an owner of h, like [asset.Hold].
func HoldEffect(sc *asset.Scope, h gfx.EffectHandle) {
	asset.Hold(sc, asset.HandleOf[Effect](h.Ref))
}

// EffectStore resolves [gfx.EffectHandle] values to effects. Get one with
// [EffectStoreOf]. It is a small value; copy it freely.
type EffectStore struct{ s *asset.Store[Effect] }

// EffectStoreOf returns the effect store of srv. It panics like
// [asset.StoreOf] if [RegisterLoaders] has not run on srv.
func EffectStoreOf(srv *asset.Server) EffectStore {
	return EffectStore{asset.StoreOf[Effect](srv)}
}

// Lookup returns the effect and its load state. Only a Ready effect is
// real: otherwise the pointer is the placeholder, an effect without an
// emitter. The pointer is valid until the next [asset.Server.Update] and must
// not be stored. It does not allocate.
func (s EffectStore) Lookup(h gfx.EffectHandle) (*Effect, asset.State) {
	return s.s.Lookup(asset.HandleOf[Effect](h.Ref))
}

// Get returns the effect, or the placeholder; see [EffectStore.Lookup].
func (s EffectStore) Get(h gfx.EffectHandle) *Effect { return s.s.Get(asset.HandleOf[Effect](h.Ref)) }

// Ready reports whether h refers to a loaded effect.
func (s EffectStore) Ready(h gfx.EffectHandle) bool { return s.s.Ready(asset.HandleOf[Effect](h.Ref)) }

// Err returns why the effect failed to load, or nil.
func (s EffectStore) Err(h gfx.EffectHandle) error { return s.s.Err(asset.HandleOf[Effect](h.Ref)) }

// Key returns the key h was loaded from, or the zero Key for a zero or stale
// handle.
func (s EffectStore) Key(h gfx.EffectHandle) asset.Key { return s.s.Key(asset.HandleOf[Effect](h.Ref)) }

// Version is 0 until the effect is committed and goes up with every hot
// reload that replaced it; the [System] compares it to restart the pools of a
// changed effect.
func (s EffectStore) Version(h gfx.EffectHandle) uint32 {
	return s.s.Version(asset.HandleOf[Effect](h.Ref))
}

// Generic returns the underlying generic store, for loaders and tools that
// work with asset.Handle[Effect].
func (s EffectStore) Generic() *asset.Store[Effect] { return s.s }
