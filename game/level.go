package game

import (
	"bytes"
	"errors"
	"fmt"
	"io/fs"
	"slices"
	"strconv"
	"strings"

	"github.com/weakpixel/asset"
	"github.com/weakpixel/ecs"
	"github.com/weakpixel/engine/asset/library"
	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/god"
	"github.com/weakpixel/engine/hierarchy"
	"github.com/weakpixel/engine/prefab"
	"github.com/weakpixel/engine/tiled"
	"github.com/weakpixel/engine/tilemap"
)

// Level is a loaded level: a GOD file ([Engine.LoadLevel]), a Tiled map or
// world ([Engine.LoadMap]) with its optional <map>.level.json documents, or
// the parts of a base key ([Engine.LoadLevelBase]). It owns the entities it spawned and the one asset scope holding the
// assets of the map and of all its documents.
type Level struct {
	path    string
	name    string
	scope   *asset.Scope
	objects []god.Object

	// Map levels: exactly one handle is set. Their layers are spawned when
	// the map (or every map of the world) is Ready.
	mapH    tilemap.MapHandle
	worldH  tilemap.WorldHandle
	opts    tilemap.SpawnOptions
	maps    []LoadedMap
	bounds  gfx.Rect
	spawned bool

	// Levels inside a library package: the hold of the package's packs,
	// whether the level's parts were started, what starts them once the
	// packs are mounted, and why a part failed.
	load    *library.Load
	started bool
	start   func() error
	partErr error

	// docKeys are the documents started or tried so far (a document spawns
	// once per level); named is set once name holds the first one's name.
	docKeys []string
	named   bool

	// GOD documents that name prefabs spawn once the prefabs are Ready
	// (spawnPendingDocs). They stay listed if the prefabs never are.
	docs []*pendingDoc
}

// pendingDoc is a GOD document waiting for its prefabs.
type pendingDoc struct {
	name    string
	data    []byte
	prefabs []prefab.Handle
}

// LoadedMap is one spawned map of a map level.
type LoadedMap struct {
	// Map is the map's handle; read it with tilemap.MapStoreOf.
	Map tilemap.MapHandle
	// Pos is the world position of the map's top-left corner.
	Pos gfx.Vec2
	// Layers are the map's layer entities, in draw order.
	Layers []ecs.Entity
}

// Path returns the file the level was loaded from, or the base key
// ("forest/cave") for a level loaded with LoadLevelBase.
func (l *Level) Path() string { return l.path }

// Name returns the optional top-level "name" of the level's first document
// (the base's document, else the first paired one), a label for editors, or ""
// when it has none, there is no document and after UnloadLevel. Names are not
// unique and nothing looks levels up by them.
func (l *Level) Name() string { return l.name }

// Objects returns the spawned Game Objects of all the level's documents, in
// spawn order. A map level without a document has none: the map's objects are
// data in its Maps.
func (l *Level) Objects() []god.Object { return l.objects }

// SetObjects replaces the level's object list, for tools that create and
// delete a level's objects in place (an editor applying a changed document):
// [Engine.UnloadLevel] deletes the entities listed here. The level keeps
// objs; the caller must not change it afterwards.
func (l *Level) SetObjects(objs []god.Object) { l.objects = objs }

// Scope returns the asset scope that holds the level's assets; nil after
// [Engine.UnloadLevel]. Tools that decode more components for the level
// resolve their asset keys into it (resolver.Resolver.SetScope).
func (l *Level) Scope() *asset.Scope { return l.scope }

// Maps returns the maps of a map level with their layer entities: one map
// for a .tmj, every map of a .world in file order. It is empty until
// [Level.Spawned], and for a GOD level.
func (l *Level) Maps() []LoadedMap { return l.maps }

// Bounds returns the world rectangle covered by the maps of a map level:
// the union of each map's tilemap.Map.Bounds placed at its Pos. It is the
// zero Rect until [Level.Spawned], and for a GOD level. Use it for the camera
// bounds, after the level spawned:
//
//	e.CameraRig().Bounds = level.Bounds()
func (l *Level) Bounds() gfx.Rect { return l.bounds }

// Spawned reports whether the level's entities exist. A GOD level spawns in
// LoadLevel, or, when its document places prefab instances, in the tick in
// which those prefabs are Ready; a map level in the tick in which its maps
// become Ready. It is true when every map and every document has spawned or
// failed. A level that failed never spawns: check [Level.Err].
func (l *Level) Spawned() bool { return l.spawned && len(l.docs) == 0 }

// Progress returns how many of the level's assets are ready and how many it
// references; use it for a loading bar. A level whose package is packed
// counts the packs first (see library.Load.Progress).
func (l *Level) Progress() (ready, total int) {
	if l.load != nil && !l.started {
		return l.load.Progress()
	}
	return l.scope.Progress()
}

// Done reports whether every asset of the level is ready (or failed). A
// level is also done when the packs of its package failed.
func (l *Level) Done() bool {
	if l.load != nil && !l.started {
		return l.load.Done() && l.load.Err() != nil
	}
	return l.scope.Done()
}

// Err returns the asset errors of the level joined, or nil. It includes each
// GOD document that did not spawn (unreadable, invalid, a failed prefab) and
// failed packs. A missing paired document is never an error.
func (l *Level) Err() error {
	if l.load != nil && !l.started {
		return errors.Join(l.partErr, l.load.Err())
	}
	return errors.Join(l.partErr, l.scope.Err())
}

// LoadLevel reads the GOD file at path from Config.FS and spawns its
// objects into the World. The entities exist immediately; assets load in
// the background and sprites draw the placeholder until [Level.Done].
//
// Asset references resolve in a scope owned by the level, so
// [Engine.UnloadLevel] can free them. To switch levels without a gap, load
// the new level, wait for Done, then unload the old one.
//
// A document whose objects are prefab instances ("prefab": "<key>", see
// doc/guide/engine/game-objects.md#prefabs) loads those prefabs into the
// level's scope first and spawns in the assets stage of the tick in which
// they are all Ready, before any game system runs; until then
// [Level.Spawned] is false. Errors found then (a prefab that failed, an
// invalid override) go to [Level.Err] and Config.OnAssetError, and nothing
// spawns.
//
// It returns an error for a missing file, invalid JSON or an invalid
// object; nothing is spawned then. Call it from a Start hook or a system.
func (e *Engine) LoadLevel(path string) (*Level, error) {
	if e.cfg.FS == nil {
		return nil, fmt.Errorf("game: LoadLevel %q: Config.FS is nil", path)
	}
	scope := e.assets.NewScope(path)
	l := &Level{path: path, scope: scope}
	start := func() error {
		data, err := fs.ReadFile(e.cfg.FS, path)
		if err != nil {
			return fmt.Errorf("game: LoadLevel: %w", err)
		}
		if err := e.startDocument(l, path, data); err != nil {
			return fmt.Errorf("game: LoadLevel %q: %w", path, err)
		}
		l.spawned = true
		return nil
	}
	if err := e.inPackage(l, path, start); err != nil {
		scope.Release()
		return nil, err
	}
	e.levels = append(e.levels, l)
	return l, nil
}

// LoadLevelData is [Engine.LoadLevel] for a document held in memory, for
// example the unsaved text of an editor: it spawns the objects of the GOD
// document data. path only names the level (its asset scope and
// [Level.Path]); nothing is read from it. Assets still load from Config.FS.
// A document with prefab instances spawns later, like LoadLevel's.
//
// It returns an error for invalid JSON or an invalid object; nothing is
// spawned then. Call it from a Start hook or a system.
func (e *Engine) LoadLevelData(path string, data []byte) (*Level, error) {
	scope := e.assets.NewScope(path)
	l := &Level{path: path, scope: scope, spawned: true}
	if err := e.startDocument(l, path, data); err != nil {
		scope.Release()
		return nil, fmt.Errorf("game: LoadLevel %q: %w", path, err)
	}
	e.levels = append(e.levels, l)
	return l, nil
}

// startDocument spawns the GOD document data (named name) into l. A
// document without prefab instances spawns at once; otherwise its prefabs
// are loaded into l's scope and spawnPendingDocs spawns it once they are
// Ready.
func (e *Engine) startDocument(l *Level, name string, data []byte) error {
	if !l.named {
		l.name, l.named = god.DocumentName(data), true
	}
	keys, err := god.PrefabKeys(data)
	if err != nil || len(keys) == 0 {
		// Invalid JSON is reported by Deserialize, with its usual message.
		objs, err := e.spawnObjects(l.scope, data)
		if err != nil {
			return err
		}
		l.objects = append(l.objects, objs...)
		return nil
	}
	d := &pendingDoc{name: name, data: data}
	for _, k := range keys {
		h, err := loadPrefab(l.scope, k)
		if err != nil {
			return fmt.Errorf("prefab %q: %w", k, err)
		}
		d.prefabs = append(d.prefabs, h)
	}
	if len(l.docs) == 0 {
		e.docPending = append(e.docPending, l)
	}
	l.docs = append(l.docs, d)
	return nil
}

// loadPrefab is prefab.Load with a bad key as an error: keys in documents
// are data.
func loadPrefab(sc *asset.Scope, key string) (h prefab.Handle, err error) {
	defer func() {
		if r := recover(); r != nil {
			msg, ok := r.(string)
			if !ok {
				panic(r)
			}
			err = errors.New(msg)
		}
	}()
	return prefab.Load(sc, key), nil
}

// spawnPendingDocs spawns the GOD documents whose prefabs became Ready and
// drops those whose prefabs failed. It runs in the assets stage, right
// after the asset server's Update; with nothing pending it is a length
// check.
func (e *Engine) spawnPendingDocs() {
	if len(e.docPending) == 0 {
		return
	}
	store := e.Stores().Prefabs()
	keep := e.docPending[:0]
	for _, l := range e.docPending {
		l.docs = slices.DeleteFunc(l.docs, func(d *pendingDoc) bool {
			state := asset.Ready
			var failed prefab.Handle
			for _, h := range d.prefabs {
				switch _, st := store.Lookup(h); st {
				case asset.Ready:
				case asset.Failed:
					state, failed = asset.Failed, h
				default:
					if state != asset.Failed {
						state = st
					}
				}
			}
			switch state {
			case asset.Ready:
				objs, err := e.spawnObjects(l.scope, d.data)
				if err != nil {
					e.failDocument(l, d.name, err)
					return true
				}
				l.objects = append(l.objects, objs...)
				return true
			case asset.Failed:
				e.failDocument(l, d.name, fmt.Errorf("prefab %s failed to load", store.Key(failed)))
				return true
			}
			return false
		})
		if len(l.docs) > 0 {
			keep = append(keep, l)
		}
	}
	clear(e.docPending[len(keep):])
	e.docPending = keep
}

// failDocument records why the document name of l did not spawn: it is joined
// into l's Err and sent to OnAssetError; the rest of the level is unaffected.
func (e *Engine) failDocument(l *Level, name string, err error) {
	err = fmt.Errorf("game: LoadLevel %q: objects not spawned: %w", name, err)
	l.partErr = errors.Join(l.partErr, err)
	if l.mapH.IsZero() && l.worldH.IsZero() {
		// Without a map the document was the level: it never spawned.
		l.spawned = false
	}
	e.reportAssetError(err)
}

// spawnObjects deserializes the GOD document data into the World, with
// its asset references loaded into scope.
func (e *Engine) spawnObjects(scope *asset.Scope, data []byte) ([]god.Object, error) {
	prev := e.resolver.Scope()
	e.resolver.SetScope(scope)
	defer e.resolver.SetScope(prev)
	objs, err := god.Deserialize(e.world, bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	ents := make([]ecs.Entity, len(objs))
	ids := make(map[ecs.Entity]string, len(objs))
	for i, o := range objs {
		ents[i] = o.Entity
		ids[o.Entity] = o.ID
	}
	err = hierarchy.CheckCycles(e.world, ents, func(en ecs.Entity) string {
		if id := ids[en]; id != "" {
			return strconv.Quote(id)
		}
		return fmt.Sprint(en)
	})
	if err != nil {
		for _, en := range ents {
			e.world.DeleteEntity(en)
		}
		return nil, err
	}
	e.hierarchyDirty = true
	return objs, nil
}

// Levels returns the levels loaded with [Engine.LoadLevel],
// [Engine.LoadLevelData] or [Engine.LoadMap] and not unloaded yet, in load
// order. The slice is a copy.
func (e *Engine) Levels() []*Level { return slices.Clone(e.levels) }

// LoadMap starts loading a Tiled map (*.tmj) or world (*.world) into a scope
// owned by the level and returns immediately. The engine spawns the layers
// (see tilemap.Spawn and tilemap.SpawnWorld; opts applies to each map, and a
// world's maps are placed at opts.Pos plus their world position) in the
// assets stage of the tick in which the map, or every map of the world,
// becomes Ready, before any game system runs. Until then [Level.Spawned] is
// false and [Level.Maps] is empty. A map that fails to load spawns nothing
// and sets [Level.Err].
//
// A map inside a library package that is packed and not mounted loads once
// the packs of its package (and of the packages it uses) are mounted, which
// the level's scope then holds.
//
// It returns an error for a path with another suffix or when Config.FS is
// nil. Call it from a Start hook or a system.
func (e *Engine) LoadMap(path string, opts tilemap.SpawnOptions) (*Level, error) {
	if e.cfg.FS == nil {
		return nil, fmt.Errorf("game: LoadMap %q: Config.FS is nil", path)
	}
	if !strings.HasSuffix(path, tiled.MapSuffix) && !strings.HasSuffix(path, tiled.WorldSuffix) {
		return nil, fmt.Errorf("game: LoadMap %q: want a %s map or a %s world", path, tiled.MapSuffix, tiled.WorldSuffix)
	}
	l := &Level{path: path, opts: opts, scope: e.assets.NewScope(path)}
	if err := e.inPackage(l, path, func() error { return e.startMap(l, path) }); err != nil {
		l.scope.Release()
		return nil, err
	}
	e.levels = append(e.levels, l)
	return l, nil
}

// startMap loads the map or world key into l's scope (created here unless l
// has one), queues l for spawning and starts the document paired with it.
func (e *Engine) startMap(l *Level, key string) error {
	switch {
	case strings.HasSuffix(key, tiled.MapSuffix):
		if l.scope == nil {
			l.scope = e.assets.NewScope(key)
		}
		l.mapH = tilemap.LoadMap(l.scope, key)
	case strings.HasSuffix(key, tiled.WorldSuffix):
		if l.scope == nil {
			l.scope = e.assets.NewScope(key)
		}
		l.worldH = tilemap.LoadWorld(l.scope, key)
	default:
		return fmt.Errorf("game: LoadMap %q: want a %s map or a %s world", key, tiled.MapSuffix, tiled.WorldSuffix)
	}
	e.pending = append(e.pending, l)
	e.startPairedDocument(l, key)
	return nil
}

// spawnPendingMaps spawns the map levels whose assets became Ready and drops
// those that failed. It runs in the assets stage, right after the asset
// server's Update; with nothing pending it is a length check.
func (e *Engine) spawnPendingMaps() {
	if len(e.pending) == 0 {
		return
	}
	keep := e.pending[:0]
	for _, l := range e.pending {
		switch l.spawn(e) {
		case asset.Ready, asset.Failed:
			// Settled: spawned, or failed with Err set.
		default:
			keep = append(keep, l)
		}
	}
	clear(e.pending[len(keep):])
	e.pending = keep
}

// addBounds grows l.bounds by a map's bounds r placed at pos.
func (l *Level) addBounds(r gfx.Rect, pos gfx.Vec2) {
	r = gfx.Rect{Min: r.Min.Add(pos), Max: r.Max.Add(pos)}
	if len(l.maps) == 1 {
		l.bounds = r
		return
	}
	l.bounds = gfx.Rect{
		Min: gfx.Vec2{X: min(l.bounds.Min.X, r.Min.X), Y: min(l.bounds.Min.Y, r.Min.Y)},
		Max: gfx.Vec2{X: max(l.bounds.Max.X, r.Max.X), Y: max(l.bounds.Max.Y, r.Max.Y)},
	}
}

// spawn spawns l if its map or world is Ready and returns the load state.
func (l *Level) spawn(e *Engine) asset.State {
	maps := e.Stores().Maps()
	var state asset.State
	var list []tilemap.WorldMap
	if !l.mapH.IsZero() {
		_, state = maps.Lookup(l.mapH)
		list = []tilemap.WorldMap{{Map: l.mapH}}
	} else {
		var w *tilemap.World
		w, state = e.Stores().Worlds().Lookup(l.worldH)
		list = w.Maps
	}
	if state != asset.Ready {
		return state
	}
	for _, wm := range list {
		opts := l.opts
		opts.Pos = l.opts.Pos.Add(wm.Pos)
		// Every map is Ready (a World is Ready only when its maps are), so
		// Spawn cannot fail here.
		layers, err := tilemap.Spawn(e.world, maps, wm.Map, opts)
		if err != nil {
			panicf("LoadMap: " + err.Error())
		}
		l.maps = append(l.maps, LoadedMap{Map: wm.Map, Pos: opts.Pos, Layers: layers})
		l.addBounds(maps.Get(wm.Map).Bounds(), opts.Pos)
		// The map's own document follows its layers (a no-op for the map or
		// world LoadMap started it for).
		e.startPairedDocument(l, maps.Key(wm.Map).Path)
	}
	l.spawned = true
	return state
}

// UnloadLevel deletes the level's entities that are still alive (Game
// Objects or map layers) and releases its assets (and the packs it holds,
// which are unmounted unless another scope holds them). A map level that has not
// spawned yet never will. The level must not be used afterwards, except
// Path. Unloading twice is a no-op.
func (e *Engine) UnloadLevel(l *Level) {
	if l == nil || l.scope == nil {
		return
	}
	for _, o := range l.objects {
		if e.world.IsAlive(o.Entity) {
			e.world.DeleteEntity(o.Entity)
		}
	}
	for _, m := range l.maps {
		for _, en := range m.Layers {
			if e.world.IsAlive(en) {
				e.world.DeleteEntity(en)
			}
		}
	}
	if i := slices.Index(e.pending, l); i >= 0 {
		e.pending = append(e.pending[:i], e.pending[i+1:]...)
	}
	if i := slices.Index(e.packagePending, l); i >= 0 {
		e.packagePending = slices.Delete(e.packagePending, i, i+1)
	}
	if i := slices.Index(e.docPending, l); i >= 0 {
		e.docPending = slices.Delete(e.docPending, i, i+1)
	}
	if i := slices.Index(e.levels, l); i >= 0 {
		e.levels = slices.Delete(e.levels, i, i+1)
	}
	l.scope.Release()
	l.scope, l.name, l.objects, l.maps, l.docs = nil, "", nil, nil, nil
}
