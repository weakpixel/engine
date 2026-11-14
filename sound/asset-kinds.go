package sound

import (
	"github.com/weakpixel/asset"
	"github.com/weakpixel/engine/assetkind"
	"github.com/weakpixel/engine/gfx"
)

// Asset kinds of this package: the names a component field carries in its
// +game:asset:kind=... marker.
const (
	// AssetKindSound is a [Sound], a short effect decoded to memory.
	AssetKindSound = "sound"
	// AssetKindMusic is a [Music], a track streamed while it plays.
	AssetKindMusic = "music"
)

func init() {
	assetkind.Register(AssetKindSound, assetkind.Of(LoadSound,
		func(srv *asset.Server, h gfx.SoundHandle) asset.Key { return SoundStoreOf(srv).Key(h) }))
	assetkind.Register(AssetKindMusic, assetkind.Of(LoadMusic,
		func(srv *asset.Server, h gfx.MusicHandle) asset.Key { return MusicStoreOf(srv).Key(h) }))
}
