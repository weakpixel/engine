package render

import "github.com/weakpixel/asset"

// Stores are the asset stores the Renderer resolves item handles through.
// Their placeholders (see [RegisterLoaders]) are what a zero, stale, loading
// or failed handle draws, so Draw never sees a nil image or face.
type Stores struct {
	Sprites SpriteStore
	Fonts   FontStore
	Shaders ShaderStore
}

// StoresOf returns the render stores of srv. It panics like [asset.StoreOf]
// if [RegisterLoaders] has not run on srv.
func StoresOf(srv *asset.Server) Stores {
	return Stores{
		Sprites: SpriteStoreOf(srv),
		Fonts:   FontStoreOf(srv),
		Shaders: ShaderStoreOf(srv),
	}
}
