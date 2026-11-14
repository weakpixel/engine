package anim

import (
	"github.com/weakpixel/asset"
	"github.com/weakpixel/engine/assetkind"
	"github.com/weakpixel/engine/gfx"
)

// AssetKindClip is the asset kind of a [Clip]
// ("chars/hero.aseprite.json#run"): the name a component field carries in
// its +game:asset:kind=... marker.
const AssetKindClip = "clip"

func init() {
	assetkind.Register(AssetKindClip, assetkind.Of(LoadClip,
		func(srv *asset.Server, h gfx.ClipHandle) asset.Key { return ClipStoreOf(srv).Key(h) }))
}
