package speech

import (
	"github.com/weakpixel/asset"
	"github.com/weakpixel/engine/assetkind"
	"github.com/weakpixel/engine/gfx"
)

// Asset kinds of this package: the names a component field carries in its
// +game:asset:kind=... marker.
const (
	// AssetKindVoice is a [Voice]: a *.voice.json file or a starter voice
	// by name ("chirpy").
	AssetKindVoice = "voice"
	// AssetKindDialogue is a [Dialogue] (a *.dialogue.json file).
	AssetKindDialogue = "dialogue"
)

func init() {
	assetkind.Register(AssetKindVoice, assetkind.Of(LoadVoice,
		func(srv *asset.Server, h gfx.VoiceHandle) asset.Key { return VoiceStoreOf(srv).Key(h) }))
	assetkind.Register(AssetKindDialogue, assetkind.Of(LoadDialogue,
		func(srv *asset.Server, h gfx.DialogueHandle) asset.Key { return DialogueStoreOf(srv).Key(h) }))
}
