// Package resolver connects Game Object data to the asset system. Its
// Resolver implements god.AssetResolver on top of an asset.Server, so
// god.Deserialize can turn the asset keys of a prefab or level
// ("chars/hero.aseprite.json#run") into the handles components hold, and
// god.SerializeComponents can turn them back.
//
// Wiring:
//
//	res := resolver.New(srv)
//	res.SetScope(levelScope)        // loads of Deserialize are held by this scope
//	resolver.Install(world, res)    // god.SetAssetResolver
//
// The resolver knows no asset kind itself. Every kind ("sprite", "clip",
// "sound", "voice", ...) is registered in package assetkind by the package
// that owns the asset type (render, anim, sound, prefab, particles, speech),
// so a kind exists in every program that links its owner; a game built on
// package game links all of them except the opt-in speech. The kinds and the
// fields that use them are listed in doc/guide/engine/kinds.md.
package resolver

import (
	"fmt"

	"github.com/weakpixel/asset"
	"github.com/weakpixel/ecs"
	"github.com/weakpixel/engine/assetkind"
	"github.com/weakpixel/engine/god"
)

// Resolver implements [god.AssetResolver] for an asset.Server. Like the
// server it is single-goroutine.
type Resolver struct {
	srv   *asset.Server
	scope *asset.Scope
}

// New returns a Resolver that loads into srv's global scope until
// [Resolver.SetScope] is called. The loaders of the kinds in use
// (render.RegisterLoaders, anim.RegisterLoaders, sound.RegisterLoaders) must
// be registered on srv.
func New(srv *asset.Server) *Resolver {
	return &Resolver{srv: srv, scope: srv.Global()}
}

// Install makes r the world's asset resolver (god.SetAssetResolver).
func Install(w *ecs.World, r *Resolver) { god.SetAssetResolver(w, r) }

// SetScope sets the scope that ResolveAsset loads into. Every resolved asset
// is held by that scope, so releasing a level's scope frees the art its
// prefabs loaded. Deleting an entity releases nothing. Set the scope before
// Deserialize; a nil scope means the global one.
func (r *Resolver) SetScope(sc *asset.Scope) {
	if sc == nil {
		sc = r.srv.Global()
	}
	r.scope = sc
}

// Scope returns the scope ResolveAsset loads into.
func (r *Resolver) Scope() *asset.Scope { return r.scope }

// ResolveAsset loads path as an asset of the given kind into the current
// scope and returns its handle (gfx.SpriteHandle for "sprite",
// gfx.ClipHandle for "clip", and so on: what the kind's Load returns). It
// does not block: the asset may still be loading, and draws its placeholder
// until it is ready. A malformed path, a kind that is not registered in
// package assetkind or a kind whose loader is not registered on the server
// is an error, not a panic.
func (r *Resolver) ResolveAsset(kind, path string) (h any, err error) {
	defer recoverAssetPanic(&err)
	k, ok := assetkind.Lookup(kind)
	if !ok {
		return nil, fmt.Errorf("unknown asset kind %q", kind)
	}
	return k.Load(r.scope, path), nil
}

// AssetKey returns the key h was loaded from ("path" or "path#sub"). A stale
// handle (its asset was unloaded) is an error, so serialization never writes
// a key that no longer means anything.
func (r *Resolver) AssetKey(kind string, h any) (key string, err error) {
	defer recoverAssetPanic(&err)
	rk, ok := assetkind.Lookup(kind)
	if !ok {
		return "", fmt.Errorf("unknown asset kind %q", kind)
	}
	k, ok := rk.Key(r.srv, h)
	if !ok {
		return "", wrongHandle(kind, h)
	}
	if k == (asset.Key{}) {
		return "", fmt.Errorf("stale %s handle", kind)
	}
	return k.String(), nil
}

func wrongHandle(kind string, h any) error {
	return fmt.Errorf("a %s asset needs the matching gfx handle, got %T", kind, h)
}

// recoverAssetPanic turns the asset package's programmer-error panics (bad
// key, type not registered, server closed) into errors: keys come from data
// files, where a bad one is bad input.
func recoverAssetPanic(err *error) {
	if r := recover(); r != nil {
		if msg, ok := r.(string); ok {
			*err = fmt.Errorf("%s", msg)
			return
		}
		panic(r)
	}
}
