package tilemap

import "github.com/weakpixel/asset"

// This file is the game-facing API for the tileset, map and world kinds:
// concrete handle types, Load helpers that return them, and stores that take
// them, following anim/handles.go. It is the one place that converts between
// these handles and asset.Handle; every wrapper is a field copy plus a call.

// TilesetHandle refers to a [Tileset]. It is 8 bytes, comparable and pointer-free;
// the zero value means "none".
type TilesetHandle struct{ asset.Ref }

// LoadTileset returns the handle for a tileset key ("tiles/temple.tsj") and adds a hold
// for sc. It never blocks and panics like [asset.Load].
func LoadTileset(sc *asset.Scope, key string) TilesetHandle {
	return TilesetHandle{Ref: asset.Load[Tileset](sc, key).Ref()}
}

// LoadTilesetSync is LoadTileset followed by loading until the tileset (and
// everything it depends on) is ready or failed, like [asset.LoadSync]. It
// blocks: use it for boot screens, tests and tools.
func LoadTilesetSync(sc *asset.Scope, key string) (TilesetHandle, error) {
	h, err := asset.LoadSync[Tileset](sc, key)
	return TilesetHandle{Ref: h.Ref()}, err
}

// HoldTileset adds sc as an owner of h, like [asset.Hold].
func HoldTileset(sc *asset.Scope, h TilesetHandle) {
	asset.Hold(sc, asset.HandleOf[Tileset](h.Ref))
}

// TilesetStore resolves [TilesetHandle] values. Get one with [TilesetStoreOf]. It is a
// small value; copy it freely.
type TilesetStore struct{ s *asset.Store[Tileset] }

// TilesetStoreOf returns the tileset store of srv. It panics like
// [asset.StoreOf] if [RegisterLoaders] has not run on srv.
func TilesetStoreOf(srv *asset.Server) TilesetStore {
	return TilesetStore{asset.StoreOf[Tileset](srv)}
}

// Get returns the tileset, or the placeholder (an empty Tileset); see
// [asset.Store.Get] for the pointer validity rule. It does not allocate.
func (s TilesetStore) Get(h TilesetHandle) *Tileset { return s.s.Get(asset.HandleOf[Tileset](h.Ref)) }

// Lookup is Get plus the load state; see [asset.Store.Lookup]. Use it to
// tell a loaded tileset from the placeholder.
func (s TilesetStore) Lookup(h TilesetHandle) (*Tileset, asset.State) {
	return s.s.Lookup(asset.HandleOf[Tileset](h.Ref))
}

// Ready reports whether h refers to a loaded tileset.
func (s TilesetStore) Ready(h TilesetHandle) bool { return s.s.Ready(asset.HandleOf[Tileset](h.Ref)) }

// Err returns why the tileset failed to load, or nil.
func (s TilesetStore) Err(h TilesetHandle) error { return s.s.Err(asset.HandleOf[Tileset](h.Ref)) }

// Key returns the key h was loaded from, or the zero Key for a zero or stale
// handle.
func (s TilesetStore) Key(h TilesetHandle) asset.Key { return s.s.Key(asset.HandleOf[Tileset](h.Ref)) }

// Generic returns the underlying generic store, for loaders and tools that
// work with asset.Handle[Tileset].
func (s TilesetStore) Generic() *asset.Store[Tileset] { return s.s }

// MapHandle refers to a [Map]. It is 8 bytes, comparable and pointer-free;
// the zero value means "none".
type MapHandle struct{ asset.Ref }

// LoadMap returns the handle for a map key ("level-1.tmj") and adds a hold
// for sc. It never blocks and panics like [asset.Load].
func LoadMap(sc *asset.Scope, key string) MapHandle {
	return MapHandle{Ref: asset.Load[Map](sc, key).Ref()}
}

// LoadMapSync is LoadMap followed by loading until the map (and
// everything it depends on) is ready or failed, like [asset.LoadSync]. It
// blocks: use it for boot screens, tests and tools.
func LoadMapSync(sc *asset.Scope, key string) (MapHandle, error) {
	h, err := asset.LoadSync[Map](sc, key)
	return MapHandle{Ref: h.Ref()}, err
}

// HoldMap adds sc as an owner of h, like [asset.Hold].
func HoldMap(sc *asset.Scope, h MapHandle) {
	asset.Hold(sc, asset.HandleOf[Map](h.Ref))
}

// MapStore resolves [MapHandle] values. Get one with [MapStoreOf]. It is a
// small value; copy it freely.
type MapStore struct{ s *asset.Store[Map] }

// MapStoreOf returns the map store of srv. It panics like
// [asset.StoreOf] if [RegisterLoaders] has not run on srv.
func MapStoreOf(srv *asset.Server) MapStore {
	return MapStore{asset.StoreOf[Map](srv)}
}

// Get returns the map, or the placeholder (an empty Map); see
// [asset.Store.Get] for the pointer validity rule. It does not allocate.
func (s MapStore) Get(h MapHandle) *Map { return s.s.Get(asset.HandleOf[Map](h.Ref)) }

// Lookup is Get plus the load state; see [asset.Store.Lookup]. Use it to
// tell a loaded map from the placeholder.
func (s MapStore) Lookup(h MapHandle) (*Map, asset.State) {
	return s.s.Lookup(asset.HandleOf[Map](h.Ref))
}

// Ready reports whether h refers to a loaded map.
func (s MapStore) Ready(h MapHandle) bool { return s.s.Ready(asset.HandleOf[Map](h.Ref)) }

// Err returns why the map failed to load, or nil.
func (s MapStore) Err(h MapHandle) error { return s.s.Err(asset.HandleOf[Map](h.Ref)) }

// Key returns the key h was loaded from, or the zero Key for a zero or stale
// handle.
func (s MapStore) Key(h MapHandle) asset.Key { return s.s.Key(asset.HandleOf[Map](h.Ref)) }

// Generic returns the underlying generic store, for loaders and tools that
// work with asset.Handle[Map].
func (s MapStore) Generic() *asset.Store[Map] { return s.s }

// WorldHandle refers to a [World]. It is 8 bytes, comparable and pointer-free;
// the zero value means "none".
type WorldHandle struct{ asset.Ref }

// LoadWorld returns the handle for a world key ("world-1.world") and adds a hold
// for sc. It never blocks and panics like [asset.Load].
func LoadWorld(sc *asset.Scope, key string) WorldHandle {
	return WorldHandle{Ref: asset.Load[World](sc, key).Ref()}
}

// LoadWorldSync is LoadWorld followed by loading until the world (and
// everything it depends on) is ready or failed, like [asset.LoadSync]. It
// blocks: use it for boot screens, tests and tools.
func LoadWorldSync(sc *asset.Scope, key string) (WorldHandle, error) {
	h, err := asset.LoadSync[World](sc, key)
	return WorldHandle{Ref: h.Ref()}, err
}

// HoldWorld adds sc as an owner of h, like [asset.Hold].
func HoldWorld(sc *asset.Scope, h WorldHandle) {
	asset.Hold(sc, asset.HandleOf[World](h.Ref))
}

// WorldStore resolves [WorldHandle] values. Get one with [WorldStoreOf]. It is a
// small value; copy it freely.
type WorldStore struct{ s *asset.Store[World] }

// WorldStoreOf returns the world store of srv. It panics like
// [asset.StoreOf] if [RegisterLoaders] has not run on srv.
func WorldStoreOf(srv *asset.Server) WorldStore {
	return WorldStore{asset.StoreOf[World](srv)}
}

// Get returns the world, or the placeholder (an empty World); see
// [asset.Store.Get] for the pointer validity rule. It does not allocate.
func (s WorldStore) Get(h WorldHandle) *World { return s.s.Get(asset.HandleOf[World](h.Ref)) }

// Lookup is Get plus the load state; see [asset.Store.Lookup]. Use it to
// tell a loaded world from the placeholder.
func (s WorldStore) Lookup(h WorldHandle) (*World, asset.State) {
	return s.s.Lookup(asset.HandleOf[World](h.Ref))
}

// Ready reports whether h refers to a loaded world.
func (s WorldStore) Ready(h WorldHandle) bool { return s.s.Ready(asset.HandleOf[World](h.Ref)) }

// Err returns why the world failed to load, or nil.
func (s WorldStore) Err(h WorldHandle) error { return s.s.Err(asset.HandleOf[World](h.Ref)) }

// Key returns the key h was loaded from, or the zero Key for a zero or stale
// handle.
func (s WorldStore) Key(h WorldHandle) asset.Key { return s.s.Key(asset.HandleOf[World](h.Ref)) }

// Generic returns the underlying generic store, for loaders and tools that
// work with asset.Handle[World].
func (s WorldStore) Generic() *asset.Store[World] { return s.s }
