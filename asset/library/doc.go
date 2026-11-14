// Package library is the opt-in standard asset library: every top folder of
// the asset root is a package (a collection of assets that loads, mounts
// and packs together), loaded into an [asset.Scope] the caller owns, with a
// catalog for file systems that cannot list directories and zip packs that
// are mounted while a scope that loaded them holds them.
//
// The layout (doc/guide/engine/asset-library.md):
//
//	boot/       optional, tiny: mounted first, blocking (a loading screen)
//	global/     required: loaded at boot into the global scope, never released
//	forest/     a package: any name; levels, maps, sprites, prefabs ...
//	temple/     a package other packages use
//
// A package is named by its folder ("forest"). Packages use each other by
// referencing their files; loading one mounts the packs of the packages it
// uses. Keys never change: "forest/tiles/trees.tsj" is the same key whether
// the file is loose, embedded or inside the package's pack.
//
// It is stdlib only plus [asset]. It knows asset kinds by file name
// ([KindOf]) but not their Go types: the engine (or a game) maps kinds to
// load functions with [Library.SetLoader].
//
// # Goroutines
//
// Everything runs on the game goroutine except reads through [Library.FS]
// and [Library.Mounted], which the asset workers may call concurrently;
// mounting publishes a new mount table atomically, so reads never lock.
//
// # Panics and errors
//
// Programmer errors panic with "library: " (unknown package, loading
// before [Library.Attach]). Bad files and a failing pack go through the
// asset pipeline: the asset or the [Load] fails.
package library
