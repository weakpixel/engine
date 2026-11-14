// Package tiled reads the Tiled JSON formats (https://doc.mapeditor.org/en/stable/reference/json-map-format/):
// maps (*.tmj, [DecodeMap]), external tilesets (*.tsj, [DecodeTileset]) and
// worlds (*.world, [DecodeWorld]). It decodes one file at a time into a plain
// Go model and checks what can be checked from that file alone; it loads no
// images, resolves no paths and imports the standard library only.
//
// Package render (the *.tsj atlas format) and package tilemap (the Tileset,
// Map and World assets) are thin adapters over this package, and command-line
// tools can use it without Ebiten.
//
// Decoding is lenient about fields it does not model: Tiled adds fields
// between versions and its files are written by a tool, so unknown fields are
// ignored. Features the engine does not support (non-orthogonal maps, zstd,
// object templates, world patterns, embedded single-image tilesets, image
// sub-rectangles, transparent color keys, a format version other than 1.x)
// are errors, never silently dropped. Every problem of a file is reported at
// once, joined with [errors.Join].
//
// The guide is doc/guide/engine/tilemaps.md.
package tiled
