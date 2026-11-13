// Package prefab makes prefabs (pre-defined Game Objects, `*.prefab.json`)
// assets of the engine. A [Prefab] is parsed and validated on a decode
// worker, bound to the World once (its components decoded, its asset keys
// resolved into a scope the prefab owns), and spawned many times without
// reading JSON:
//
//	h := prefab.Load(level.Scope(), "global/prefabs/goblin.prefab.json")
//	...
//	if prefabs.Ready(h) {
//		prefabs.Spawn(h, gfx.Transform{Pos: at, PrevPos: at, Scale: gfx.Vec2{X: 1, Y: 1}})
//	}
//
// A prefab file may hold several objects: a root plus parts attached to it
// with hierarchy:Parent. [Prefab.Spawn] returns the root and
// [Prefab.SpawnAll] every entity. The base of a variant prefab and the
// prefabs a prefab nests are dependencies, so such a prefab is Ready only
// after them, reloads with them, and a cycle fails the prefab that closes
// it. The art a prefab references loads in the background (placeholders
// until ready) and is released with the prefab.
//
// [Source] lets god.Deserialize place prefab instances in GOD documents;
// game.Engine wires both. The package imports gfx (for the handle type) but neither Ebiten nor render. The
// guide is doc/guide/engine/game-objects.md#prefabs.
package prefab

import (
	"encoding/json"
	"fmt"
	"path"
	"strings"

	"github.com/weakpixel/asset"
	"github.com/weakpixel/ecs"
	"github.com/weakpixel/engine/god"
)

// Suffix is the file name suffix of prefab files.
const Suffix = ".prefab.json"

// Prefab is a loaded prefab: the parsed god.Prefab and its copy bound to
// the engine's World. The placeholder (a prefab that is not ready) is the
// zero Prefab, which cannot spawn.
type Prefab struct {
	prefab *god.Prefab
	bound  *god.BoundPrefab
	// scope holds the art the bound component values reference.
	scope *asset.Scope
}

// Types returns the parsed prefab (its key, object type, component JSON),
// or nil for the placeholder.
func (p *Prefab) Types() *god.Prefab { return p.prefab }

// Spawn creates one entity from the prefab, with overrides replacing (or
// adding) components by type, and returns it; see god.BoundPrefab.Spawn.
// It panics with "prefab: Spawn: not ready" on the placeholder. A prefab
// with several objects spawns all of them and returns the root; deleting
// the root deletes the rest (the hierarchy stage removes children of dead
// parents).
func (p *Prefab) Spawn(overrides ...any) ecs.Entity {
	if p.bound == nil {
		panic("prefab: Spawn: not ready")
	}
	return p.bound.Spawn(overrides...)
}

// SpawnAll is [Prefab.Spawn] that appends every spawned entity to dst in
// [god.Prefab.Parts] order, root first, and returns it; see
// god.BoundPrefab.SpawnAll. It panics like Spawn.
func (p *Prefab) SpawnAll(dst []ecs.Entity, overrides ...any) []ecs.Entity {
	if p.bound == nil {
		panic("prefab: Spawn: not ready")
	}
	return p.bound.SpawnAll(dst, overrides...)
}

// Options configure [RegisterLoader].
type Options struct {
	// World is the world prefabs are bound to and spawn into. Required.
	World *ecs.World
	// Assets returns the AssetResolver that loads a prefab's art into sc,
	// the prefab's own scope; the engine passes its resolver.Resolver set to
	// sc. Required when a prefab has asset fields with keys.
	Assets func(sc *asset.Scope) god.AssetResolver
}

// RegisterLoader registers [Prefab] and its loader on srv. It panics if
// opts.World is nil, and like [asset.Register] if Prefab is already
// registered.
func RegisterLoader(srv *asset.Server, opts Options) {
	if opts.World == nil {
		panic("prefab: RegisterLoader: Options.World is nil")
	}
	l := &loader{srv: srv, opts: opts}
	l.store = asset.Register[Prefab, *spec](srv, l, Prefab{})
}

// spec is the decoded prefab file.
type spec struct {
	data []byte
	// parsed is set for a prefab that names no other prefab: it is parsed
	// and validated on the worker.
	parsed *god.Prefab
	// keys are the prefabs the file names (its base and the prefabs it
	// nests), and deps the dependencies on them.
	keys []string
	deps []asset.DepRef[Prefab]
}

type loader struct {
	srv   *asset.Server
	opts  Options
	store *asset.Store[Prefab]
}

func (l *loader) Decode(ctx *asset.DecodeContext, data []byte) (*spec, error) {
	key := ctx.Key.Path
	keys, err := god.PrefabKeys(data)
	if err != nil {
		return nil, err
	}
	s := &spec{data: data}
	if len(keys) == 0 {
		s.parsed, err = god.ParsePrefab(key, data, nil)
		if err == nil {
			err = checkAttached(s.parsed)
		}
		return s, err
	}
	// Every prefab it names must be Ready before it can be parsed. Prefab
	// keys are root-absolute; asset.Dep paths are relative to key's folder.
	s.keys = keys
	for _, k := range keys {
		s.deps = append(s.deps, asset.Dep[Prefab](ctx, relativeTo(key, k)))
	}
	return s, nil
}

func (l *loader) Finish(ctx *asset.FinishContext, s *spec) (Prefab, error) {
	p := s.parsed
	if p == nil {
		src := make(god.PrefabMap, len(s.keys))
		for i, k := range s.keys {
			src[k] = l.store.Get(asset.Resolve(ctx, s.deps[i])).prefab
		}
		var err error
		if p, err = god.ParsePrefab(ctx.Key.Path, s.data, src); err != nil {
			return Prefab{}, err
		}
		if err = checkAttached(p); err != nil {
			return Prefab{}, err
		}
	}
	scope := l.srv.NewScope("prefab " + ctx.Key.Path)
	var assets god.AssetResolver
	if l.opts.Assets != nil {
		assets = l.opts.Assets(scope)
	}
	bound, err := p.Bind(l.opts.World, assets)
	if err != nil {
		scope.Release()
		return Prefab{}, err
	}
	return Prefab{prefab: p, bound: bound, scope: scope}, nil
}

// parentComponent is the component that attaches a part to its parent.
const parentComponent = "hierarchy:Parent"

// checkAttached enforces the engine's rule for prefabs with several
// objects: every object other than the root reaches the root through
// hierarchy:Parent (directly or through other objects), so deleting the
// root deletes the group. It reads the JSON of each object's Parent.
func checkAttached(p *god.Prefab) error {
	ids := p.Parts()
	if len(ids) < 2 {
		return nil
	}
	parent := make(map[string]string, len(ids))
	for _, id := range ids[1:] {
		raw, ok := p.PartComponent(id, parentComponent)
		if !ok {
			continue
		}
		var c struct{ Entity string }
		if err := json.Unmarshal(raw, &c); err != nil {
			return fmt.Errorf("prefab %q: object %q: %s: %w", p.Key(), id, parentComponent, err)
		}
		parent[id] = c.Entity
	}
	root := ids[0]
	for _, id := range ids[1:] {
		at := id
		for range len(ids) {
			if at == root {
				break
			}
			at = parent[at]
		}
		if at != root {
			return fmt.Errorf("prefab %q: object %q is not attached to the root (%s)", p.Key(), id, parentComponent)
		}
	}
	return nil
}

func (l *loader) Unload(v *Prefab) {
	if v.scope != nil {
		v.scope.Release()
	}
}

// relativeTo returns target, a key relative to the asset root, as a path
// relative to the folder of key (what asset.Dep expects).
func relativeTo(key, target string) string {
	dir := path.Dir(key)
	if dir == "." {
		return target
	}
	return strings.Repeat("../", strings.Count(dir, "/")+1) + target
}

// Source returns a god.PrefabSource over srv's prefabs for
// god.Deserialize and god.Serialize: Prefab(key) loads key into the
// scope that scope returns (a no-op when that scope already holds it) and
// returns it if it is Ready. A prefab that is still loading is an error, so
// load a document's prefabs first (see god.PrefabKeys); game.Engine does.
func Source(srv *asset.Server, scope func() *asset.Scope) god.PrefabSource {
	return source{store: StoreOf(srv), scope: scope}
}

type source struct {
	store Store
	scope func() *asset.Scope
}

func (s source) Prefab(key string) (*god.Prefab, error) {
	h, err := load(s.scope(), key)
	if err != nil {
		return nil, err
	}
	p, state := s.store.Lookup(h)
	switch state {
	case asset.Ready:
		return p.prefab, nil
	case asset.Failed:
		return nil, s.store.Err(h)
	}
	return nil, fmt.Errorf("not loaded yet (%s)", state)
}

// load is Load with a bad key returned as an error: keys in documents are
// data.
func load(sc *asset.Scope, key string) (h Handle, err error) {
	defer func() {
		if r := recover(); r != nil {
			msg, ok := r.(string)
			if !ok {
				panic(r)
			}
			err = fmt.Errorf("%s", msg)
		}
	}()
	return Load(sc, key), nil
}
