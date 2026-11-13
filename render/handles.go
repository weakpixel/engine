package render

import (
	"github.com/weakpixel/asset"
	"github.com/weakpixel/engine/gfx"
)

// This file is the game-facing API for the sprite, font, shader and atlas
// kinds: Load helpers that return concrete handle types (gfx.SpriteHandle,
// gfx.FontHandle and gfx.ShaderHandle, which components hold, and
// AtlasHandle), and stores that take those handles. They are thin wrappers
// over the generic asset API (asset.Load[Sprite],
// *asset.Store[Sprite]); this is the one place that converts between concrete
// handles and asset.Handle, so game code never does.
//
// Every wrapper is a field copy plus a call, small enough to inline, so the
// facade costs nothing over the generic store (see BenchmarkSpriteStoreGet).

// LoadSprite returns the handle for a sprite key ("ui/cursor.png" or
// "chars/hero.atlas.json#idle_0") and adds a hold for sc. It never blocks and
// panics like [asset.Load].
func LoadSprite(sc *asset.Scope, key string) gfx.SpriteHandle {
	return gfx.SpriteHandle{Ref: asset.Load[Sprite](sc, key).Ref()}
}

// LoadSpriteSync is LoadSprite followed by loading until the sprite is ready
// or failed, like [asset.LoadSync]. It blocks: use it for boot screens, tests
// and tools.
func LoadSpriteSync(sc *asset.Scope, key string) (gfx.SpriteHandle, error) {
	h, err := asset.LoadSync[Sprite](sc, key)
	return gfx.SpriteHandle{Ref: h.Ref()}, err
}

// HoldSprite adds sc as an owner of h, like [asset.Hold].
func HoldSprite(sc *asset.Scope, h gfx.SpriteHandle) {
	asset.Hold(sc, asset.HandleOf[Sprite](h.Ref))
}

// SpriteStore resolves [gfx.SpriteHandle] values to sprites. Get one from
// [StoresOf] or [SpriteStoreOf]. It is a small value; copy it freely. The zero
// SpriteStore is "no store": only [Queue.Sort] accepts it.
type SpriteStore struct{ s *asset.Store[Sprite] }

// SpriteStoreOf returns the sprite store of srv. It panics like
// [asset.StoreOf] if [RegisterLoaders] has not run on srv.
func SpriteStoreOf(srv *asset.Server) SpriteStore {
	return SpriteStore{asset.StoreOf[Sprite](srv)}
}

// Get returns the sprite, or the placeholder; see [asset.Store.Get] for the
// pointer validity rule. It does not allocate.
func (s SpriteStore) Get(h gfx.SpriteHandle) *Sprite { return s.s.Get(asset.HandleOf[Sprite](h.Ref)) }

// Lookup is Get plus the load state; see [asset.Store.Lookup].
func (s SpriteStore) Lookup(h gfx.SpriteHandle) (*Sprite, asset.State) {
	return s.s.Lookup(asset.HandleOf[Sprite](h.Ref))
}

// Ready reports whether h refers to a loaded sprite.
func (s SpriteStore) Ready(h gfx.SpriteHandle) bool { return s.s.Ready(asset.HandleOf[Sprite](h.Ref)) }

// Err returns why the sprite failed to load, or nil.
func (s SpriteStore) Err(h gfx.SpriteHandle) error { return s.s.Err(asset.HandleOf[Sprite](h.Ref)) }

// Key returns the key h was loaded from, or the zero Key for a zero or stale
// handle.
func (s SpriteStore) Key(h gfx.SpriteHandle) asset.Key { return s.s.Key(asset.HandleOf[Sprite](h.Ref)) }

// Generic returns the underlying generic store, for loaders and tools that
// work with asset.Handle[Sprite].
func (s SpriteStore) Generic() *asset.Store[Sprite] { return s.s }

// LoadFont returns the handle for a font key ("fonts/ui.ttf#16") and adds a
// hold for sc. It never blocks and panics like [asset.Load].
func LoadFont(sc *asset.Scope, key string) gfx.FontHandle {
	return gfx.FontHandle{Ref: asset.Load[Font](sc, key).Ref()}
}

// LoadFontSync is LoadFont followed by loading until the font is ready or
// failed, like [asset.LoadSync]. It blocks.
func LoadFontSync(sc *asset.Scope, key string) (gfx.FontHandle, error) {
	h, err := asset.LoadSync[Font](sc, key)
	return gfx.FontHandle{Ref: h.Ref()}, err
}

// HoldFont adds sc as an owner of h, like [asset.Hold].
func HoldFont(sc *asset.Scope, h gfx.FontHandle) {
	asset.Hold(sc, asset.HandleOf[Font](h.Ref))
}

// FontStore resolves [gfx.FontHandle] values to fonts. Get one from
// [StoresOf] or [FontStoreOf]. It is a small value; copy it freely.
type FontStore struct{ s *asset.Store[Font] }

// FontStoreOf returns the font store of srv. It panics like [asset.StoreOf]
// if [RegisterLoaders] has not run on srv.
func FontStoreOf(srv *asset.Server) FontStore {
	return FontStore{asset.StoreOf[Font](srv)}
}

// Get returns the font, or the placeholder; see [asset.Store.Get]. It does
// not allocate.
func (s FontStore) Get(h gfx.FontHandle) *Font { return s.s.Get(asset.HandleOf[Font](h.Ref)) }

// Lookup is Get plus the load state; see [asset.Store.Lookup].
func (s FontStore) Lookup(h gfx.FontHandle) (*Font, asset.State) {
	return s.s.Lookup(asset.HandleOf[Font](h.Ref))
}

// Ready reports whether h refers to a loaded font.
func (s FontStore) Ready(h gfx.FontHandle) bool { return s.s.Ready(asset.HandleOf[Font](h.Ref)) }

// Err returns why the font failed to load, or nil.
func (s FontStore) Err(h gfx.FontHandle) error { return s.s.Err(asset.HandleOf[Font](h.Ref)) }

// Key returns the key h was loaded from, or the zero Key for a zero or stale
// handle.
func (s FontStore) Key(h gfx.FontHandle) asset.Key { return s.s.Key(asset.HandleOf[Font](h.Ref)) }

// Generic returns the underlying generic store, for loaders and tools that
// work with asset.Handle[Font].
func (s FontStore) Generic() *asset.Store[Font] { return s.s }

// AtlasHandle refers to an [Atlas]. It is 8 bytes, comparable and
// pointer-free; the zero value means "none". No component holds one (sprites
// hold the frames' gfx.SpriteHandle), so it lives here rather than in gfx.
type AtlasHandle struct{ asset.Ref }

// LoadAtlas returns the handle for an atlas key ("chars/hero.sheet.json") and
// adds a hold for sc. It never blocks and panics like [asset.Load].
func LoadAtlas(sc *asset.Scope, key string) AtlasHandle {
	return AtlasHandle{Ref: asset.Load[Atlas](sc, key).Ref()}
}

// LoadAtlasSync is LoadAtlas followed by loading until the atlas is ready or
// failed, like [asset.LoadSync]. It blocks.
func LoadAtlasSync(sc *asset.Scope, key string) (AtlasHandle, error) {
	h, err := asset.LoadSync[Atlas](sc, key)
	return AtlasHandle{Ref: h.Ref()}, err
}

// HoldAtlas adds sc as an owner of h, like [asset.Hold].
func HoldAtlas(sc *asset.Scope, h AtlasHandle) {
	asset.Hold(sc, asset.HandleOf[Atlas](h.Ref))
}

// AtlasStore resolves [AtlasHandle] values to atlases. Get one with
// [AtlasStoreOf]. It is a small value; copy it freely.
type AtlasStore struct{ s *asset.Store[Atlas] }

// AtlasStoreOf returns the atlas store of srv. It panics like
// [asset.StoreOf] if [RegisterLoaders] has not run on srv.
func AtlasStoreOf(srv *asset.Server) AtlasStore {
	return AtlasStore{asset.StoreOf[Atlas](srv)}
}

// Get returns the atlas, or the placeholder (an empty atlas); see
// [asset.Store.Get]. It does not allocate.
func (s AtlasStore) Get(h AtlasHandle) *Atlas { return s.s.Get(asset.HandleOf[Atlas](h.Ref)) }

// Lookup is Get plus the load state; see [asset.Store.Lookup].
func (s AtlasStore) Lookup(h AtlasHandle) (*Atlas, asset.State) {
	return s.s.Lookup(asset.HandleOf[Atlas](h.Ref))
}

// Ready reports whether h refers to a loaded atlas.
func (s AtlasStore) Ready(h AtlasHandle) bool { return s.s.Ready(asset.HandleOf[Atlas](h.Ref)) }

// Err returns why the atlas failed to load, or nil.
func (s AtlasStore) Err(h AtlasHandle) error { return s.s.Err(asset.HandleOf[Atlas](h.Ref)) }

// Key returns the key h was loaded from, or the zero Key for a zero or stale
// handle.
func (s AtlasStore) Key(h AtlasHandle) asset.Key { return s.s.Key(asset.HandleOf[Atlas](h.Ref)) }

// Generic returns the underlying generic store, for loaders and tools that
// work with asset.Handle[Atlas].
func (s AtlasStore) Generic() *asset.Store[Atlas] { return s.s }

// LoadShader returns the handle for a shader key ("fx/flash.kage") and adds a
// hold for sc. It never blocks and panics like [asset.Load].
func LoadShader(sc *asset.Scope, key string) gfx.ShaderHandle {
	return gfx.ShaderHandle{Ref: asset.Load[Shader](sc, key).Ref()}
}

// LoadShaderSync is LoadShader followed by loading until the shader is ready
// or failed, like [asset.LoadSync]. It blocks.
func LoadShaderSync(sc *asset.Scope, key string) (gfx.ShaderHandle, error) {
	h, err := asset.LoadSync[Shader](sc, key)
	return gfx.ShaderHandle{Ref: h.Ref()}, err
}

// HoldShader adds sc as an owner of h, like [asset.Hold].
func HoldShader(sc *asset.Scope, h gfx.ShaderHandle) {
	asset.Hold(sc, asset.HandleOf[Shader](h.Ref))
}

// ShaderStore resolves [gfx.ShaderHandle] values to shaders. Get one from
// [StoresOf] or [ShaderStoreOf]. It is a small value; copy it freely.
type ShaderStore struct{ s *asset.Store[Shader] }

// ShaderStoreOf returns the shader store of srv. It panics like
// [asset.StoreOf] if [RegisterLoaders] has not run on srv.
func ShaderStoreOf(srv *asset.Server) ShaderStore {
	return ShaderStore{asset.StoreOf[Shader](srv)}
}

// Get returns the shader, or the placeholder, whose Shader field is nil ("no
// shader"); see [asset.Store.Get]. It does not allocate.
func (s ShaderStore) Get(h gfx.ShaderHandle) *Shader { return s.s.Get(asset.HandleOf[Shader](h.Ref)) }

// Lookup is Get plus the load state; see [asset.Store.Lookup].
func (s ShaderStore) Lookup(h gfx.ShaderHandle) (*Shader, asset.State) {
	return s.s.Lookup(asset.HandleOf[Shader](h.Ref))
}

// Ready reports whether h refers to a loaded shader.
func (s ShaderStore) Ready(h gfx.ShaderHandle) bool { return s.s.Ready(asset.HandleOf[Shader](h.Ref)) }

// Err returns why the shader failed to load (for example its compile error),
// or nil.
func (s ShaderStore) Err(h gfx.ShaderHandle) error { return s.s.Err(asset.HandleOf[Shader](h.Ref)) }

// Key returns the key h was loaded from, or the zero Key for a zero or stale
// handle.
func (s ShaderStore) Key(h gfx.ShaderHandle) asset.Key { return s.s.Key(asset.HandleOf[Shader](h.Ref)) }

// Generic returns the underlying generic store, for loaders and tools that
// work with asset.Handle[Shader].
func (s ShaderStore) Generic() *asset.Store[Shader] { return s.s }
