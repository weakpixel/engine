package prefab

import (
	"github.com/weakpixel/asset"
	"github.com/weakpixel/engine/assetkind"
)

// AssetKind is the asset kind of a [Prefab] (a *.prefab.json file): the name
// a component field carries in its +game:asset:kind=... marker.
const AssetKind = "prefab"

func init() {
	assetkind.Register(AssetKind, assetkind.Of(Load,
		func(srv *asset.Server, h Handle) asset.Key { return StoreOf(srv).Key(h) }))
}
