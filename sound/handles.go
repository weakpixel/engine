package sound

import (
	"github.com/weakpixel/asset"
	"github.com/weakpixel/engine/gfx"
)

// This file is the game-facing API for the sound and music kinds: Load
// helpers that return gfx.SoundHandle and gfx.MusicHandle (the handles
// components and the Queue hold), and stores that take them. Like
// render.SpriteStore, the wrappers inline to the generic asset.Store calls;
// this is the one place that converts between the concrete handles and
// asset.Handle.

// LoadSound returns the handle for a sound key ("sfx/hit.wav") and adds a hold for
// sc. It never blocks and panics like [asset.Load].
func LoadSound(sc *asset.Scope, key string) gfx.SoundHandle {
	return gfx.SoundHandle{Ref: asset.Load[Sound](sc, key).Ref()}
}

// LoadSoundSync is LoadSound followed by loading until the sound is ready or
// failed, like [asset.LoadSync]. It blocks: use it for boot screens, tests
// and tools.
func LoadSoundSync(sc *asset.Scope, key string) (gfx.SoundHandle, error) {
	h, err := asset.LoadSync[Sound](sc, key)
	return gfx.SoundHandle{Ref: h.Ref()}, err
}

// HoldSound adds sc as an owner of h, like [asset.Hold].
func HoldSound(sc *asset.Scope, h gfx.SoundHandle) {
	asset.Hold(sc, asset.HandleOf[Sound](h.Ref))
}

// SoundStore resolves [gfx.SoundHandle] values. Get one with [SoundStoreOf]. It is
// a small value; copy it freely.
type SoundStore struct{ s *asset.Store[Sound] }

// SoundStoreOf returns the sound store of srv. It panics like
// [asset.StoreOf] if [RegisterLoaders] has not run on srv.
func SoundStoreOf(srv *asset.Server) SoundStore {
	return SoundStore{asset.StoreOf[Sound](srv)}
}

// Get returns the sound, or the placeholder (silence); see
// [asset.Store.Get] for the pointer validity rule. It does not allocate.
func (s SoundStore) Get(h gfx.SoundHandle) *Sound { return s.s.Get(asset.HandleOf[Sound](h.Ref)) }

// Lookup is Get plus the load state; see [asset.Store.Lookup].
func (s SoundStore) Lookup(h gfx.SoundHandle) (*Sound, asset.State) {
	return s.s.Lookup(asset.HandleOf[Sound](h.Ref))
}

// Ready reports whether h refers to a loaded sound.
func (s SoundStore) Ready(h gfx.SoundHandle) bool { return s.s.Ready(asset.HandleOf[Sound](h.Ref)) }

// Err returns why the sound failed to load, or nil.
func (s SoundStore) Err(h gfx.SoundHandle) error { return s.s.Err(asset.HandleOf[Sound](h.Ref)) }

// Key returns the key h was loaded from, or the zero Key for a zero or stale
// handle.
func (s SoundStore) Key(h gfx.SoundHandle) asset.Key { return s.s.Key(asset.HandleOf[Sound](h.Ref)) }

// Generic returns the underlying generic store, for loaders and tools that
// work with asset.Handle[Sound].
func (s SoundStore) Generic() *asset.Store[Sound] { return s.s }

// LoadMusic returns the handle for a track key ("music/level1.ogg") and adds a hold for
// sc. It never blocks and panics like [asset.Load].
func LoadMusic(sc *asset.Scope, key string) gfx.MusicHandle {
	return gfx.MusicHandle{Ref: asset.Load[Music](sc, key).Ref()}
}

// LoadMusicSync is LoadMusic followed by loading until the track is ready or
// failed, like [asset.LoadSync]. It blocks: use it for boot screens, tests
// and tools.
func LoadMusicSync(sc *asset.Scope, key string) (gfx.MusicHandle, error) {
	h, err := asset.LoadSync[Music](sc, key)
	return gfx.MusicHandle{Ref: h.Ref()}, err
}

// HoldMusic adds sc as an owner of h, like [asset.Hold].
func HoldMusic(sc *asset.Scope, h gfx.MusicHandle) {
	asset.Hold(sc, asset.HandleOf[Music](h.Ref))
}

// MusicStore resolves [gfx.MusicHandle] values. Get one with [MusicStoreOf]. It is
// a small value; copy it freely.
type MusicStore struct{ s *asset.Store[Music] }

// MusicStoreOf returns the track store of srv. It panics like
// [asset.StoreOf] if [RegisterLoaders] has not run on srv.
func MusicStoreOf(srv *asset.Server) MusicStore {
	return MusicStore{asset.StoreOf[Music](srv)}
}

// Get returns the track, or the placeholder (silence); see
// [asset.Store.Get] for the pointer validity rule. It does not allocate.
func (s MusicStore) Get(h gfx.MusicHandle) *Music { return s.s.Get(asset.HandleOf[Music](h.Ref)) }

// Lookup is Get plus the load state; see [asset.Store.Lookup].
func (s MusicStore) Lookup(h gfx.MusicHandle) (*Music, asset.State) {
	return s.s.Lookup(asset.HandleOf[Music](h.Ref))
}

// Ready reports whether h refers to a loaded track.
func (s MusicStore) Ready(h gfx.MusicHandle) bool { return s.s.Ready(asset.HandleOf[Music](h.Ref)) }

// Err returns why the track failed to load, or nil.
func (s MusicStore) Err(h gfx.MusicHandle) error { return s.s.Err(asset.HandleOf[Music](h.Ref)) }

// Key returns the key h was loaded from, or the zero Key for a zero or stale
// handle.
func (s MusicStore) Key(h gfx.MusicHandle) asset.Key { return s.s.Key(asset.HandleOf[Music](h.Ref)) }

// Generic returns the underlying generic store, for loaders and tools that
// work with asset.Handle[Music].
func (s MusicStore) Generic() *asset.Store[Music] { return s.s }
