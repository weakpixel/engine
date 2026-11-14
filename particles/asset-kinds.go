package particles

import (
	"github.com/weakpixel/asset"
	"github.com/weakpixel/engine/assetkind"
	"github.com/weakpixel/engine/gfx"
)

// AssetKindEffect is the asset kind of an [Effect] (a *.fx.json file): the
// name a component field carries in its +game:asset:kind=... marker.
const AssetKindEffect = "effect"

func init() {
	assetkind.Register(AssetKindEffect, assetkind.Of(LoadEffect,
		func(srv *asset.Server, h gfx.EffectHandle) asset.Key { return EffectStoreOf(srv).Key(h) }))
}
