package speech

import (
	"github.com/weakpixel/asset"
	"github.com/weakpixel/engine/gfx"
)

// This file is the game-facing API of the speech assets: Load helpers that
// return gfx.VoiceHandle and gfx.DialogueHandle (the handles components
// hold) and LanguageHandle, and stores that take them, like sound's
// SoundStore. It is the one place that converts between the concrete
// handles and asset.Handle.

// LanguageHandle refers to a [Language]. Components do not hold languages
// (a voice names its own), so the handle lives here, not in gfx.
type LanguageHandle struct{ asset.Ref }

// LoadVoice returns the handle for a voice key ("chars/bard.voice.json", or
// a starter name such as "chirpy") and adds a hold for sc. It never blocks
// and panics like [asset.Load].
func LoadVoice(sc *asset.Scope, key string) gfx.VoiceHandle {
	return gfx.VoiceHandle{Ref: asset.Load[Voice](sc, key).Ref()}
}

// LoadVoiceSync is LoadVoice followed by loading until the voice is ready or
// failed, like [asset.LoadSync]. It blocks: use it for boot screens, tests
// and tools.
func LoadVoiceSync(sc *asset.Scope, key string) (gfx.VoiceHandle, error) {
	h, err := asset.LoadSync[Voice](sc, key)
	return gfx.VoiceHandle{Ref: h.Ref()}, err
}

// LoadDialogue returns the handle for a dialogue key
// ("talk/village.dialogue.json") and adds a hold for sc. It never blocks and
// panics like [asset.Load].
func LoadDialogue(sc *asset.Scope, key string) gfx.DialogueHandle {
	return gfx.DialogueHandle{Ref: asset.Load[Dialogue](sc, key).Ref()}
}

// LoadDialogueSync is LoadDialogue followed by loading until the dialogue is
// ready or failed, like [asset.LoadSync]. It blocks.
func LoadDialogueSync(sc *asset.Scope, key string) (gfx.DialogueHandle, error) {
	h, err := asset.LoadSync[Dialogue](sc, key)
	return gfx.DialogueHandle{Ref: h.Ref()}, err
}

// LoadLanguage returns the handle for a language key
// ("lang/elvish.lang.json") and adds a hold for sc. A voice loads its own
// language; load one directly to parse text with it. It never blocks and
// panics like [asset.Load].
func LoadLanguage(sc *asset.Scope, key string) LanguageHandle {
	return LanguageHandle{Ref: asset.Load[Language](sc, key).Ref()}
}

// VoiceStore resolves [gfx.VoiceHandle] values. Get one with [VoiceStoreOf].
// It is a small value; copy it freely.
type VoiceStore struct{ s *asset.Store[Voice] }

// VoiceStoreOf returns the voice store of srv. It panics like
// [asset.StoreOf] if [RegisterLoaders] has not run on srv.
func VoiceStoreOf(srv *asset.Server) VoiceStore { return VoiceStore{asset.StoreOf[Voice](srv)} }

// Get returns the voice, or the placeholder (a silent voice); see
// [asset.Store.Get] for the pointer validity rule. It does not allocate.
func (s VoiceStore) Get(h gfx.VoiceHandle) *Voice { return s.s.Get(asset.HandleOf[Voice](h.Ref)) }

// Lookup is Get plus the load state; see [asset.Store.Lookup].
func (s VoiceStore) Lookup(h gfx.VoiceHandle) (*Voice, asset.State) {
	return s.s.Lookup(asset.HandleOf[Voice](h.Ref))
}

// Ready reports whether h refers to a loaded voice.
func (s VoiceStore) Ready(h gfx.VoiceHandle) bool { return s.s.Ready(asset.HandleOf[Voice](h.Ref)) }

// Err returns why the voice failed to load, or nil.
func (s VoiceStore) Err(h gfx.VoiceHandle) error { return s.s.Err(asset.HandleOf[Voice](h.Ref)) }

// Version returns the voice's version, which a hot reload increments; see
// [asset.Store.Version].
func (s VoiceStore) Version(h gfx.VoiceHandle) uint32 {
	return s.s.Version(asset.HandleOf[Voice](h.Ref))
}

// Key returns the key h was loaded from, or the zero Key for a zero or stale
// handle.
func (s VoiceStore) Key(h gfx.VoiceHandle) asset.Key { return s.s.Key(asset.HandleOf[Voice](h.Ref)) }

// Generic returns the underlying generic store.
func (s VoiceStore) Generic() *asset.Store[Voice] { return s.s }

// DialogueStore resolves [gfx.DialogueHandle] values. Get one with
// [DialogueStoreOf]. It is a small value; copy it freely.
type DialogueStore struct{ s *asset.Store[Dialogue] }

// DialogueStoreOf returns the dialogue store of srv. It panics like
// [asset.StoreOf] if [RegisterLoaders] has not run on srv.
func DialogueStoreOf(srv *asset.Server) DialogueStore {
	return DialogueStore{asset.StoreOf[Dialogue](srv)}
}

// Get returns the dialogue, or the placeholder (no script); see
// [asset.Store.Get] for the pointer validity rule. It does not allocate.
func (s DialogueStore) Get(h gfx.DialogueHandle) *Dialogue {
	return s.s.Get(asset.HandleOf[Dialogue](h.Ref))
}

// Lookup is Get plus the load state; see [asset.Store.Lookup].
func (s DialogueStore) Lookup(h gfx.DialogueHandle) (*Dialogue, asset.State) {
	return s.s.Lookup(asset.HandleOf[Dialogue](h.Ref))
}

// Ready reports whether h refers to a loaded dialogue.
func (s DialogueStore) Ready(h gfx.DialogueHandle) bool {
	return s.s.Ready(asset.HandleOf[Dialogue](h.Ref))
}

// Err returns why the dialogue failed to load, or nil.
func (s DialogueStore) Err(h gfx.DialogueHandle) error {
	return s.s.Err(asset.HandleOf[Dialogue](h.Ref))
}

// Key returns the key h was loaded from, or the zero Key for a zero or stale
// handle.
func (s DialogueStore) Key(h gfx.DialogueHandle) asset.Key {
	return s.s.Key(asset.HandleOf[Dialogue](h.Ref))
}

// Generic returns the underlying generic store.
func (s DialogueStore) Generic() *asset.Store[Dialogue] { return s.s }

// LanguageStore resolves [LanguageHandle] values. Get one with
// [LanguageStoreOf]. It is a small value; copy it freely.
type LanguageStore struct{ s *asset.Store[Language] }

// LanguageStoreOf returns the language store of srv. It panics like
// [asset.StoreOf] if [RegisterLoaders] has not run on srv.
func LanguageStoreOf(srv *asset.Server) LanguageStore {
	return LanguageStore{asset.StoreOf[Language](srv)}
}

// Get returns the language, or the placeholder (the default language). It
// does not allocate.
func (s LanguageStore) Get(h LanguageHandle) *Language {
	return s.s.Get(asset.HandleOf[Language](h.Ref))
}

// Err returns why the language failed to load, or nil.
func (s LanguageStore) Err(h LanguageHandle) error { return s.s.Err(asset.HandleOf[Language](h.Ref)) }

// Generic returns the underlying generic store.
func (s LanguageStore) Generic() *asset.Store[Language] { return s.s }
