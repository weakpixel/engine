package game

import (
	"github.com/weakpixel/asset"
	"github.com/weakpixel/engine/anim"
	"github.com/weakpixel/engine/input/inputsys"
	"github.com/weakpixel/engine/prefab"
	"github.com/weakpixel/engine/render"
	"github.com/weakpixel/engine/sound"
	"github.com/weakpixel/engine/tilemap"
)

// Stores gives typed access to the asset stores of an [Engine]:
// e.Stores().ClipSets().Get(h) instead of anim.ClipSetStoreOf(e.Assets()).
// It is a small value; copy it freely.
//
// Every method resolves its store when called (one map lookup, like the
// XStoreOf function it wraps) and panics like [asset.StoreOf] if that kind's
// loaders are not registered, so an unregistered kind fails only when asked
// for. Hot paths cache the store in a local or a system field. A kind the
// engine does not know uses [asset.StoreOf] with e.Assets().
type Stores struct{ srv *asset.Server }

// Stores returns the stores of the engine's asset server.
func (e *Engine) Stores() Stores { return Stores{e.assets} }

// StoresOf returns the stores of srv, for code that has a server but no
// Engine. It panics only when a store is asked for, not here.
func StoresOf(srv *asset.Server) Stores { return Stores{srv} }

// Sprites returns the sprite store; see [render.SpriteStoreOf].
func (s Stores) Sprites() render.SpriteStore { return render.SpriteStoreOf(s.srv) }

// Fonts returns the font store; see [render.FontStoreOf].
func (s Stores) Fonts() render.FontStore { return render.FontStoreOf(s.srv) }

// Atlases returns the atlas store; see [render.AtlasStoreOf].
func (s Stores) Atlases() render.AtlasStore { return render.AtlasStoreOf(s.srv) }

// Shaders returns the shader store; see [render.ShaderStoreOf].
func (s Stores) Shaders() render.ShaderStore { return render.ShaderStoreOf(s.srv) }

// Clips returns the clip store; see [anim.ClipStoreOf].
func (s Stores) Clips() anim.ClipStore { return anim.ClipStoreOf(s.srv) }

// ClipSets returns the clip set store; see [anim.ClipSetStoreOf].
func (s Stores) ClipSets() anim.ClipSetStore { return anim.ClipSetStoreOf(s.srv) }

// Tilesets returns the tileset store; see [tilemap.TilesetStoreOf].
func (s Stores) Tilesets() tilemap.TilesetStore { return tilemap.TilesetStoreOf(s.srv) }

// Maps returns the map store; see [tilemap.MapStoreOf].
func (s Stores) Maps() tilemap.MapStore { return tilemap.MapStoreOf(s.srv) }

// Worlds returns the world store; see [tilemap.WorldStoreOf].
func (s Stores) Worlds() tilemap.WorldStore { return tilemap.WorldStoreOf(s.srv) }

// Sounds returns the sound store; see [sound.SoundStoreOf].
func (s Stores) Sounds() sound.SoundStore { return sound.SoundStoreOf(s.srv) }

// Music returns the music store; see [sound.MusicStoreOf].
func (s Stores) Music() sound.MusicStore { return sound.MusicStoreOf(s.srv) }

// Bindings returns the input bindings store; see [inputsys.BindingsStoreOf].
func (s Stores) Bindings() inputsys.BindingsStore { return inputsys.BindingsStoreOf(s.srv) }

// Prefabs returns the prefab store; see [prefab.StoreOf].
func (s Stores) Prefabs() prefab.Store { return prefab.StoreOf(s.srv) }
