package render

import (
	"github.com/weakpixel/asset"
	"github.com/weakpixel/engine/assetkind"
	"github.com/weakpixel/engine/gfx"
)

// Asset kinds of this package: the names a component field carries in its
// +game:asset:kind=... marker.
const (
	// AssetKindSprite is a [Sprite]: a whole image or an atlas frame.
	AssetKindSprite = "sprite"
	// AssetKindFont is a [Font] ("fonts/ui.ttf#16").
	AssetKindFont = "font"
	// AssetKindShader is a [Shader] (a *.kage file).
	AssetKindShader = "shader"
)

func init() {
	assetkind.Register(AssetKindSprite, assetkind.Of(LoadSprite,
		func(srv *asset.Server, h gfx.SpriteHandle) asset.Key { return SpriteStoreOf(srv).Key(h) }))
	assetkind.Register(AssetKindFont, assetkind.Of(LoadFont,
		func(srv *asset.Server, h gfx.FontHandle) asset.Key { return FontStoreOf(srv).Key(h) }))
	assetkind.Register(AssetKindShader, assetkind.Of(LoadShader,
		func(srv *asset.Server, h gfx.ShaderHandle) asset.Key { return ShaderStoreOf(srv).Key(h) }))
}
