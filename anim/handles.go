package anim

import (
	"github.com/weakpixel/asset"
	"github.com/weakpixel/engine/gfx"
)

// This file is the game-facing API for the clip and clip set kinds: Load
// helpers that return gfx.ClipHandle (the handle gfx.Animator holds) and
// ClipSetHandle, and stores that take them. Like render.SpriteStore, the wrappers inline to the generic
// asset.Store[Clip] calls; this is the one place that converts between
// gfx.ClipHandle and asset.Handle[Clip].

// LoadClip returns the handle for a clip key
// ("chars/hero.aseprite.json#run") and adds a hold for sc. It never blocks
// and panics like [asset.Load].
func LoadClip(sc *asset.Scope, key string) gfx.ClipHandle {
	return gfx.ClipHandle{Ref: asset.Load[Clip](sc, key).Ref()}
}

// LoadClipSync is LoadClip followed by loading until the clip is ready or
// failed, like [asset.LoadSync]. It blocks: use it for boot screens, tests
// and tools.
func LoadClipSync(sc *asset.Scope, key string) (gfx.ClipHandle, error) {
	h, err := asset.LoadSync[Clip](sc, key)
	return gfx.ClipHandle{Ref: h.Ref()}, err
}

// HoldClip adds sc as an owner of h, like [asset.Hold].
func HoldClip(sc *asset.Scope, h gfx.ClipHandle) {
	asset.Hold(sc, asset.HandleOf[Clip](h.Ref))
}

// ClipStore resolves [gfx.ClipHandle] values to clips. Get one with
// [ClipStoreOf]. It is a small value; copy it freely.
type ClipStore struct{ s *asset.Store[Clip] }

// ClipStoreOf returns the clip store of srv. It panics like [asset.StoreOf]
// if [RegisterLoaders] has not run on srv.
func ClipStoreOf(srv *asset.Server) ClipStore {
	return ClipStore{asset.StoreOf[Clip](srv)}
}

// Get returns the clip, or the placeholder (an empty clip); see
// [asset.Store.Get] for the pointer validity rule. It does not allocate.
func (s ClipStore) Get(h gfx.ClipHandle) *Clip { return s.s.Get(asset.HandleOf[Clip](h.Ref)) }

// Lookup is Get plus the load state; see [asset.Store.Lookup]. Use it to
// tell a loaded clip from the placeholder.
func (s ClipStore) Lookup(h gfx.ClipHandle) (*Clip, asset.State) {
	return s.s.Lookup(asset.HandleOf[Clip](h.Ref))
}

// Ready reports whether h refers to a loaded clip.
func (s ClipStore) Ready(h gfx.ClipHandle) bool { return s.s.Ready(asset.HandleOf[Clip](h.Ref)) }

// Err returns why the clip failed to load, or nil.
func (s ClipStore) Err(h gfx.ClipHandle) error { return s.s.Err(asset.HandleOf[Clip](h.Ref)) }

// Key returns the key h was loaded from, or the zero Key for a zero or stale
// handle.
func (s ClipStore) Key(h gfx.ClipHandle) asset.Key { return s.s.Key(asset.HandleOf[Clip](h.Ref)) }

// Generic returns the underlying generic store, for loaders and tools that
// work with asset.Handle[Clip].
func (s ClipStore) Generic() *asset.Store[Clip] { return s.s }

// ClipSetHandle refers to a [ClipSet]. It is 8 bytes, comparable and
// pointer-free; the zero value means "none". No component holds one (the
// Animator holds a clip), so it lives here rather than in gfx.
type ClipSetHandle struct{ asset.Ref }

// LoadClipSet returns the handle for a clip set key
// ("chars/hero.aseprite.json" or "chars/hero.anim.json") and adds a hold for
// sc. It never blocks and panics like [asset.Load].
func LoadClipSet(sc *asset.Scope, key string) ClipSetHandle {
	return ClipSetHandle{Ref: asset.Load[ClipSet](sc, key).Ref()}
}

// LoadClipSetSync is LoadClipSet followed by loading until the set is ready
// or failed, like [asset.LoadSync]. It blocks.
func LoadClipSetSync(sc *asset.Scope, key string) (ClipSetHandle, error) {
	h, err := asset.LoadSync[ClipSet](sc, key)
	return ClipSetHandle{Ref: h.Ref()}, err
}

// HoldClipSet adds sc as an owner of h, like [asset.Hold].
func HoldClipSet(sc *asset.Scope, h ClipSetHandle) {
	asset.Hold(sc, asset.HandleOf[ClipSet](h.Ref))
}

// ClipSetStore resolves [ClipSetHandle] values to clip sets. Get one with
// [ClipSetStoreOf]. It is a small value; copy it freely.
type ClipSetStore struct{ s *asset.Store[ClipSet] }

// ClipSetStoreOf returns the clip set store of srv. It panics like
// [asset.StoreOf] if [RegisterLoaders] has not run on srv.
func ClipSetStoreOf(srv *asset.Server) ClipSetStore {
	return ClipSetStore{asset.StoreOf[ClipSet](srv)}
}

// Get returns the clip set, or the placeholder (an empty set); see
// [asset.Store.Get]. It does not allocate.
func (s ClipSetStore) Get(h ClipSetHandle) *ClipSet { return s.s.Get(asset.HandleOf[ClipSet](h.Ref)) }

// Lookup is Get plus the load state; see [asset.Store.Lookup].
func (s ClipSetStore) Lookup(h ClipSetHandle) (*ClipSet, asset.State) {
	return s.s.Lookup(asset.HandleOf[ClipSet](h.Ref))
}

// Ready reports whether h refers to a loaded clip set.
func (s ClipSetStore) Ready(h ClipSetHandle) bool { return s.s.Ready(asset.HandleOf[ClipSet](h.Ref)) }

// Err returns why the clip set failed to load, or nil.
func (s ClipSetStore) Err(h ClipSetHandle) error { return s.s.Err(asset.HandleOf[ClipSet](h.Ref)) }

// Key returns the key h was loaded from, or the zero Key for a zero or stale
// handle.
func (s ClipSetStore) Key(h ClipSetHandle) asset.Key { return s.s.Key(asset.HandleOf[ClipSet](h.Ref)) }

// Generic returns the underlying generic store, for loaders and tools that
// work with asset.Handle[ClipSet].
func (s ClipSetStore) Generic() *asset.Store[ClipSet] { return s.s }
