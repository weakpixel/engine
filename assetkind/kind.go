package assetkind

import (
	"slices"
	"sync"

	"github.com/weakpixel/asset"
)

// Kind is how one asset kind is loaded and written back.
type Kind struct {
	// Load loads key into sc and returns the handle components hold. It
	// may panic like asset.Load; the resolver turns that into an error.
	Load func(sc *asset.Scope, key string) any
	// Key returns the key h was loaded from on srv, and false when h is not
	// a handle of the kind. A stale handle returns the zero Key.
	Key func(srv *asset.Server, h any) (asset.Key, bool)
}

// Of builds a [Kind] from the two typed functions an owner already has: its
// Load helper and the Key method of its store. H is the handle type
// components hold (gfx.SpriteHandle); the Kind's Key reports false for any
// other type.
func Of[H any](load func(sc *asset.Scope, key string) H, key func(srv *asset.Server, h H) asset.Key) Kind {
	return Kind{
		Load: func(sc *asset.Scope, k string) any { return load(sc, k) },
		Key: func(srv *asset.Server, h any) (asset.Key, bool) {
			th, ok := h.(H)
			if !ok {
				return asset.Key{}, false
			}
			return key(srv, th), true
		},
	}
}

var (
	mu    sync.RWMutex
	kinds = map[string]Kind{}
)

// Register adds an asset kind under name. Call it from an init function of
// the package that owns the asset type. It is safe for concurrent use.
//
// Panics with "assetkind: kind ... needs Load and Key" when either function
// is nil, and with "assetkind: kind ... is already registered" when name is
// taken: a kind has one owner.
func Register(name string, k Kind) {
	if k.Load == nil || k.Key == nil {
		panic(`assetkind: kind "` + name + `" needs Load and Key`)
	}
	mu.Lock()
	defer mu.Unlock()
	if _, dup := kinds[name]; dup {
		panic(`assetkind: kind "` + name + `" is already registered`)
	}
	kinds[name] = k
}

// Lookup returns the kind registered under name.
func Lookup(name string) (Kind, bool) {
	mu.RLock()
	defer mu.RUnlock()
	k, ok := kinds[name]
	return k, ok
}

// Names returns the registered kind names, sorted. It allocates; use it for
// tools, tests and error texts.
func Names() []string {
	mu.RLock()
	defer mu.RUnlock()
	names := make([]string, 0, len(kinds))
	for n := range kinds {
		names = append(names, n)
	}
	slices.Sort(names)
	return names
}
