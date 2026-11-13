// Package assetkind is the registry of asset kinds: the names a component
// field carries in its +game:asset:kind=... marker ("sprite", "clip",
// "voice"), each with how to load a key of that kind and how to find the key
// of a handle again.
//
// The package that owns an asset type registers its kinds from an init
// function, so a kind exists in every program that links its owner:
//
//	func init() {
//		assetkind.Register(AssetKindSprite, assetkind.Of(LoadSprite,
//			func(srv *asset.Server, h gfx.SpriteHandle) asset.Key { return SpriteStoreOf(srv).Key(h) }))
//	}
//
// Package resolver looks kinds up here when it turns the asset keys of a
// level or prefab into handles and back. The package imports only the
// standard library and the asset module, so every owner can import it.
package assetkind
