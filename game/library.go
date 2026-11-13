package game

import (
	"errors"
	"fmt"
	"io/fs"
	"log"

	"github.com/weakpixel/asset"
	"github.com/weakpixel/engine/anim"
	"github.com/weakpixel/engine/asset/library"
	"github.com/weakpixel/engine/input/inputsys"
	"github.com/weakpixel/engine/particles"
	"github.com/weakpixel/engine/prefab"
	"github.com/weakpixel/engine/render"
	"github.com/weakpixel/engine/sound"
	"github.com/weakpixel/engine/tiled"
	"github.com/weakpixel/engine/tilemap"
)

// mountLibrary attaches Config.Library to the asset server, maps the
// library's kinds to the engine's loaders and mounts the first package in
// the global scope, blocking: boot/ when the library has one, else global/.
// Everything New and the game's Start hooks load from that package (the
// bindings file, a font) can then be read at once. A pack that fails is
// reported; loads of its files then fail too.
func (e *Engine) mountLibrary() {
	lib := e.cfg.Library
	if lib == nil {
		return
	}
	lib.Attach(e.assets)
	setLoaders(lib)
	first := library.GlobalDir
	if _, ok := lib.Package(library.BootDir); ok {
		first = library.BootDir
	}
	if _, ok := lib.Package(first); ok {
		if err := lib.MountPackage(e.assets.Global(), first); err != nil {
			e.reportAssetError(err)
		}
	}
}

// bootLibrary starts loading the entries of boot/ (when there is one) and
// of global/ into the global scope. It runs after every loader is
// registered. With boot/, global/'s packs mount in the background.
func (e *Engine) bootLibrary() {
	lib := e.cfg.Library
	if lib == nil {
		return
	}
	if _, ok := lib.Package(library.BootDir); ok {
		lib.LoadPackage(e.assets.Global(), library.BootDir)
	}
	if _, ok := lib.Package(library.GlobalDir); ok {
		e.boot = lib.LoadPackage(e.assets.Global(), library.GlobalDir)
	}
}

// setLoaders maps every kind the convention knows to the engine's load
// function for it. Prefabs are assets; levels are GOD documents, which the
// engine spawns itself.
func setLoaders(lib *library.Library) {
	lib.SetLoader(library.KindTexture, func(sc *asset.Scope, key string) { asset.Load[render.Texture](sc, key) })
	lib.SetLoader(library.KindAtlas, func(sc *asset.Scope, key string) { render.LoadAtlas(sc, key) })
	lib.SetLoader(library.KindShader, func(sc *asset.Scope, key string) { render.LoadShader(sc, key) })
	lib.SetLoader(library.KindFont, func(sc *asset.Scope, key string) { render.LoadFont(sc, key) })
	lib.SetLoader(library.KindClipSet, func(sc *asset.Scope, key string) { anim.LoadClipSet(sc, key) })
	lib.SetLoader(library.KindTileset, func(sc *asset.Scope, key string) { tilemap.LoadTileset(sc, key) })
	lib.SetLoader(library.KindMap, func(sc *asset.Scope, key string) { tilemap.LoadMap(sc, key) })
	lib.SetLoader(library.KindWorld, func(sc *asset.Scope, key string) { tilemap.LoadWorld(sc, key) })
	lib.SetLoader(library.KindSound, func(sc *asset.Scope, key string) { sound.LoadSound(sc, key) })
	lib.SetLoader(library.KindMusic, func(sc *asset.Scope, key string) { sound.LoadMusic(sc, key) })
	lib.SetLoader(library.KindBindings, func(sc *asset.Scope, key string) { inputsys.LoadBindings(sc, key) })
	lib.SetLoader(library.KindPrefab, func(sc *asset.Scope, key string) { prefab.Load(sc, key) })
	lib.SetLoader(library.KindEffect, func(sc *asset.Scope, key string) { particles.LoadEffect(sc, key) })
}

// Library returns Config.Library, or nil.
func (e *Engine) Library() *library.Library { return e.cfg.Library }

// Boot returns the load of the library's global package, which New starts
// in the global scope: use its Progress and Done for a boot screen. It is
// nil without Config.Library or without a global/ folder. With a boot/
// package, New mounts boot/ first and global/ loads in the background, so a
// startup screen drawn with boot/'s assets can show this progress.
func (e *Engine) Boot() *library.Load { return e.boot }

// LoadPackage loads every entry of the library package name into sc and
// returns at once (see library.Library.LoadPackage): the explicit preload
// of a package, for prefabs, effects or sounds that code spawns or plays by
// key. Loading a level never preloads its package. Load into a level's
// scope (Level.Scope) to release the assets with the level.
//
// It panics with "game: LoadPackage: Config.Library is nil" and like the
// library for an unknown package.
func (e *Engine) LoadPackage(sc *asset.Scope, name string) *library.Load {
	if e.cfg.Library == nil {
		panicf("LoadPackage: Config.Library is nil")
	}
	return e.cfg.Library.LoadPackage(sc, name)
}

// LoadLevelBase loads the level named base, a key without suffix
// ("forest/cave"), into a new scope owned by the returned Level and returns
// at once. Once the packs of base's package and of the packages it uses are
// mounted (at once for loose files), it spawns the level's parts into the
// same Level:
//
//   - <base>.level.json, a GOD document, like LoadLevel;
//   - <base>.tmj or <base>.world, like LoadMap with opts, which also spawns
//     the documents paired with the map (<base>.level.json is the pair of
//     <base>.tmj and spawns once; a world's children bring their own
//     <map>.level.json).
//
// Every part is optional, but one must exist. opts.NoDocuments and
// opts.SkipDocuments leave documents out. Everything lives in the Level's
// one scope; the package's other files load only when something references
// them (preload with [Engine.LoadPackage]). With loose files the GOD objects
// exist when it returns; from a pack they spawn in the assets stage of the
// tick in which the pack is mounted. [Level.Spawned] reports true once
// every part has spawned. Unload it with [Engine.UnloadLevel], which also
// unmounts the packs it holds.
//
// It works without a library too (the parts are looked up in Config.FS).
// It returns an error for an unknown package, for a base with none of the
// three files (loose files only) and, loose files only, for a GOD document
// that does not spawn; from a pack those errors go to [Level.Err] and
// OnAssetError. A paired document that does not spawn goes to [Level.Err]
// and OnAssetError and never blocks the map. Call it from a Start hook or a
// system.
func (e *Engine) LoadLevelBase(base string, opts tilemap.SpawnOptions) (*Level, error) {
	if e.cfg.FS == nil {
		return nil, fmt.Errorf("game: LoadLevelBase %q: Config.FS is nil", base)
	}
	if lib := e.cfg.Library; lib != nil {
		if p := library.PackageOf(base + documentSuffix); p != "" {
			if _, ok := lib.Package(p); !ok {
				return nil, fmt.Errorf("game: LoadLevelBase %q: the library has no package %s", base, p)
			}
		}
	}
	scope := e.assets.NewScope(base)
	l := &Level{path: base, scope: scope, opts: opts}
	start := func() error {
		if err := e.startBaseParts(l, base); err != nil {
			return fmt.Errorf("game: LoadLevelBase %q: %w", base, err)
		}
		return nil
	}
	if err := e.inPackage(l, base+documentSuffix, start); err != nil {
		scope.Release()
		return nil, err
	}
	e.levels = append(e.levels, l)
	return l, nil
}

// inPackage runs start for l at once when the files of key's package can be
// read now (no library, no package, loose files, packs already mounted),
// returning its error (start's errors carry their "game: " prefix);
// otherwise it holds the package's packs in l's scope
// and updateLibrary runs start once they are mounted.
func (e *Engine) inPackage(l *Level, key string, start func() error) error {
	lib := e.cfg.Library
	if lib == nil {
		return start()
	}
	name := library.PackageOf(key)
	if _, ok := lib.Package(name); !ok {
		return start()
	}
	l.load = lib.HoldPackage(l.scope, name)
	if l.load.Mounted() {
		l.started = true
		return start()
	}
	l.start = start
	e.packagePending = append(e.packagePending, l)
	return nil
}

// startBaseParts spawns <base>.level.json and starts <base>.tmj or
// <base>.world into l. The package's files must be readable.
func (e *Engine) startBaseParts(l *Level, base string) error {
	doc := base + documentSuffix
	var maps []string
	for _, suffix := range []string{tiled.MapSuffix, tiled.WorldSuffix} {
		if l.hasFile(e, base+suffix) {
			maps = append(maps, base+suffix)
		}
	}
	hasDoc := l.hasFile(e, doc)
	if !hasDoc && len(maps) == 0 {
		return fmt.Errorf("no %s, %s or %s", doc, base+tiled.MapSuffix, base+tiled.WorldSuffix)
	}
	if hasDoc && l.wantDocument(doc) {
		data, err := fs.ReadFile(e.cfg.FS, doc)
		if err != nil {
			return err
		}
		if err := e.startDocument(l, doc, data); err != nil {
			return fmt.Errorf("%s: %w", doc, err)
		}
	}
	if len(maps) > 0 {
		return e.startMap(l, maps[0])
	}
	l.spawned = true
	return nil
}

// updateLibrary advances package loads and starts the levels whose packs
// were mounted this tick. It runs in the assets stage, right after the
// asset server's Update; without a library it is a nil check.
func (e *Engine) updateLibrary() {
	if e.cfg.Library == nil {
		return
	}
	e.cfg.Library.Update()
	if len(e.packagePending) == 0 {
		return
	}
	keep := e.packagePending[:0]
	for _, l := range e.packagePending {
		switch {
		case l.load.Mounted():
			l.started = true
			if err := l.start(); err != nil {
				l.partErr = errors.Join(l.partErr, err)
				e.reportAssetError(err)
			}
			l.start = nil
		case l.load.Done():
			// A pack failed: the level never starts; Err says why.
		default:
			keep = append(keep, l)
		}
	}
	clear(e.packagePending[len(keep):])
	e.packagePending = keep
}

// reportAssetError sends err to Config.OnAssetError, or logs it.
func (e *Engine) reportAssetError(err error) {
	if e.cfg.OnAssetError != nil {
		e.cfg.OnAssetError(err)
		return
	}
	log.Print(err)
}
