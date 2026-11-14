// Package tilemap loads Tiled maps as assets and draws their tile and image
// layers through the render queue.
//
// It has a data half and a runtime half. The data half is the asset types
// [Tileset] (*.tsj), [Map] (*.tmj) and [World] (*.world), which
// [RegisterLoaders] makes loadable; a Map is a self-contained, read-only
// value with its layers flattened in draw order, a GID table for O(1) tile
// lookups and the map's objects and custom properties as data. The runtime
// half is the [TileLayer] component, [Spawn] and [SpawnWorld], which create
// one entity per drawable layer, and [TilemapExtract], which queues one
// render.Item per visible tile. [NewCollisionGrid] builds a solid-cell
// lookup from the tiles' collision shapes.
//
// File parsing and per-file checks are package tiled's job; the sprites of a
// single-image tileset are cut by the render atlas loader (the same *.tsj key
// loaded as a render.Atlas). The package makes no Ebiten calls.
//
// The engine gives no meaning to objects, classes or custom properties: game
// code reads them through the Map's query API. The guide is
// doc/guide/engine/tilemaps.md.
package tilemap
