package library

import (
	"errors"
	"fmt"
	"io/fs"
	"reflect"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
)

// layer orders the mount table: a key is looked up in the overlay layer
// first, then in the packs, then in the base FS. Keeping overlays above
// packs means a pack mounted later (a package loading) never covers a mod
// or patch mounted before it.
type layer uint8

const (
	// layerPack holds the packs mounted by loading a package.
	layerPack layer = iota
	// layerOverlay holds [Library.Mount]: patches and, later, mods.
	layerOverlay
)

// mount is one entry of the mount table: fsys serves the keys under prefix.
type mount struct {
	layer  layer
	prefix string // "" for the whole root, else a clean path without trailing slash
	fsys   fs.FS
}

// mountFS is the file system the asset server reads: a base FS with mounts
// on top. A key under a mount's prefix is read from the newest overlay that
// has it, else the newest pack, else the base. A key of a packed package
// whose pack is not mounted fails instead of falling through to the base,
// which does not have it in a packed build.
//
// Decode workers read it concurrently while the game goroutine mounts and
// unmounts, so the table is copy-on-write: readers load one atomic pointer
// and never lock; writers serialize on mu and publish a new slice.
type mountFS struct {
	base   fs.FS
	mounts atomic.Pointer[[]mount] // sorted by layer, then newest last
	mu     sync.Mutex              // serializes writers
	packed map[string]bool         // packages whose files live only in a pack; read-only after Open
}

func newMountFS(base fs.FS, packed map[string]bool) *mountFS {
	m := &mountFS{base: base, packed: packed}
	m.mounts.Store(&[]mount{})
	return m
}

// add mounts fsys at prefix in layer l, above every existing mount of l
// and below every mount of a higher layer.
func (m *mountFS) add(l layer, prefix string, fsys fs.FS) {
	m.mu.Lock()
	defer m.mu.Unlock()
	cur := *m.mounts.Load()
	at := len(cur)
	for at > 0 && cur[at-1].layer > l {
		at--
	}
	next := slices.Insert(slices.Clone(cur), at, mount{layer: l, prefix: prefix, fsys: fsys})
	m.mounts.Store(&next)
}

// packages returns the prefixes of the mounted packs, sorted and unique.
func (m *mountFS) packages() []string {
	var names []string
	for _, mt := range *m.mounts.Load() {
		if mt.layer == layerPack && !slices.Contains(names, mt.prefix) {
			names = append(names, mt.prefix)
		}
	}
	slices.Sort(names)
	return names
}

// remove unmounts the newest mount of fsys at prefix in layer l. It
// reports whether there was one. Comparing fsys too lets a pack reload
// mount its new version before the old one's Unload removes the old.
func (m *mountFS) remove(l layer, prefix string, fsys fs.FS) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	cur := *m.mounts.Load()
	for i := len(cur) - 1; i >= 0; i-- {
		if cur[i].layer == l && cur[i].prefix == prefix && sameFS(cur[i].fsys, fsys) {
			next := slices.Delete(slices.Clone(cur), i, i+1)
			m.mounts.Store(&next)
			return true
		}
	}
	return false
}

// sameFS reports whether a and b are the same file system value. Map-based
// file systems (fstest.MapFS) are not comparable with ==, which would
// panic, so those compare by identity.
func sameFS(a, b fs.FS) bool {
	ta, tb := reflect.TypeOf(a), reflect.TypeOf(b)
	if ta != tb {
		return false
	}
	if ta.Comparable() {
		return a == b
	}
	va, vb := reflect.ValueOf(a), reflect.ValueOf(b)
	switch va.Kind() {
	case reflect.Map, reflect.Slice, reflect.Func:
		return va.Pointer() == vb.Pointer()
	}
	return false
}

// resolve calls op with the file system and name that serve name: each
// mount covering name, newest first, until one has it, then the base.
func resolve[R any](m *mountFS, opName, name string, op func(fsys fs.FS, name string) (R, error)) (R, error) {
	var zero R
	if !fs.ValidPath(name) {
		return zero, &fs.PathError{Op: opName, Path: name, Err: fs.ErrInvalid}
	}
	mounts := *m.mounts.Load()
	packMounted := false
	for i := len(mounts) - 1; i >= 0; i-- {
		mt := mounts[i]
		rest, ok := under(name, mt.prefix)
		if !ok {
			continue
		}
		if mt.layer == layerPack {
			packMounted = true
		}
		r, err := op(mt.fsys, rest)
		if !errors.Is(err, fs.ErrNotExist) {
			return r, err
		}
	}
	if p := PackageOf(name); p != "" && m.packed[p] && !packMounted {
		return zero, &fs.PathError{Op: opName, Path: name, Err: fmt.Errorf("library: package %s is packed and not loaded (load it with LoadPackage, or a level of it): %w", p, fs.ErrNotExist)}
	}
	return op(m.base, name)
}

// under returns name relative to prefix, if name is inside it.
func under(name, prefix string) (string, bool) {
	if prefix == "" {
		return name, true
	}
	rest, ok := strings.CutPrefix(name, prefix+"/")
	return rest, ok && rest != ""
}

// Open implements fs.FS.
func (m *mountFS) Open(name string) (fs.File, error) {
	return resolve(m, "open", name, func(f fs.FS, n string) (fs.File, error) { return f.Open(n) })
}

// ReadFile implements fs.ReadFileFS.
func (m *mountFS) ReadFile(name string) ([]byte, error) {
	return resolve(m, "readfile", name, fs.ReadFile)
}

// Stat implements fs.StatFS.
func (m *mountFS) Stat(name string) (fs.FileInfo, error) {
	return resolve(m, "stat", name, fs.Stat)
}
