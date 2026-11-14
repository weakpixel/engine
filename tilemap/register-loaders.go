package tilemap

import (
	"github.com/weakpixel/asset"
	"github.com/weakpixel/engine/render"
)

// RegisterLoaders registers [Tileset] (*.tsj), [Map] (*.tmj) and [World]
// (*.world) and their loaders on srv. Run render.RegisterLoaders first: it
// panics like [asset.StoreOf] ("asset: type not registered: render.Atlas")
// if the render loaders are missing, and like [asset.Register] if a type is
// already registered.
func RegisterLoaders(srv *asset.Server) {
	atlases := render.AtlasStoreOf(srv) // panics if the render loaders are missing
	sprites := render.SpriteStoreOf(srv)
	tilesets := asset.Register[Tileset, tilesetSpec](srv, tilesetLoader{atlases: atlases, sprites: sprites}, Tileset{})
	asset.Register[Map, mapSpec](srv, mapLoader{tilesets: TilesetStore{tilesets}, sprites: sprites}, Map{})
	asset.Register[World, worldSpec](srv, worldLoader{}, World{})
}
