package library

import (
	"errors"
	"fmt"
	"io/fs"
	"strings"

	"github.com/weakpixel/asset"
)

// LoadFunc loads key into sc, as asset.Load does for one asset type. It
// must not block.
type LoadFunc func(sc *asset.Scope, key string)

// Library is an asset library opened over a root FS. Create it with [Open],
// hand [Library.FS] to the asset server, then [Library.Attach] the server.
type Library struct {
	fs      *mountFS
	cat     *Catalog
	loaders map[Kind]LoadFunc
	srv     *asset.Server
	packs   *asset.Store[Pack]
	packOf  map[string]string // pack key -> package name
	pending []*Load
}

// Open opens the library at the root of base. If base has an
// assets.catalog.json it is read; otherwise base is listed with [Scan]
// (its findings are ignored here: run wpix assets check). A file system
// that cannot list directories (HTTP) needs a catalog.
//
// It returns an error for an invalid catalog or library.json, or a base
// without a catalog that cannot be listed.
func Open(base fs.FS) (*Library, error) {
	cat, err := loadCatalog(base)
	if err != nil {
		return nil, err
	}
	l := &Library{cat: cat, loaders: map[Kind]LoadFunc{}, packOf: map[string]string{}}
	packed := map[string]bool{}
	for _, p := range cat.Packages {
		if p.Pack != "" {
			packed[p.Name] = true
			l.packOf[p.Pack] = p.Name
		}
	}
	l.fs = newMountFS(base, packed)
	return l, nil
}

// loadCatalog reads the catalog of base, or scans base when it has none.
func loadCatalog(base fs.FS) (*Catalog, error) {
	data, err := fs.ReadFile(base, CatalogFile)
	if err == nil {
		return ReadCatalog(data)
	}
	if !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("library: %s: %w", CatalogFile, err)
	}
	if _, err := fs.ReadDir(base, "."); err != nil {
		return nil, fmt.Errorf("library: the file system cannot list directories and has no %s (write one with wpix assets catalog): %w", CatalogFile, err)
	}
	cat, _, err := Scan(base)
	return cat, err
}

// FS returns the file system to read assets from: give it to
// asset.Options.FS (game.Config.Library does). It serves the base FS plus
// the mounted packs and overlays, and is safe for concurrent reads.
func (l *Library) FS() fs.FS { return l.fs }

// Catalog returns the library's catalog: every package with its files,
// entries, uses and pack, natively and over HTTP, before anything is
// downloaded. Don't modify it.
func (l *Library) Catalog() *Catalog { return l.cat }

// Package returns the package named name ("global", "forest").
func (l *Library) Package(name string) (*Package, bool) { return l.cat.Package(name) }

// Mounted returns the names of the packages whose packs are mounted now,
// sorted. With loose files it is always empty: nothing needs mounting.
// Safe to call from any goroutine.
func (l *Library) Mounted() []string { return l.fs.packages() }

// Attach registers the pack asset type on srv, whose Options.FS must be
// [Library.FS]. Call it once, after the server's other loaders are
// registered, on the game goroutine.
//
// It panics with "library: Attach called twice", and like asset.Register
// if srv already has the Pack type.
func (l *Library) Attach(srv *asset.Server) {
	if l.srv != nil {
		panic("library: Attach called twice")
	}
	l.srv = srv
	l.packs = asset.Register[Pack, packData](srv, packLoader{lib: l}, Pack{})
}

// SetLoader makes [Library.LoadPackage] load entries of kind k with fn.
// Entries of a kind without a loader are skipped. game.New registers the
// engine's kinds; a game can replace them or add its own.
func (l *Library) SetLoader(k Kind, fn LoadFunc) { l.loaders[k] = fn }

// Mount serves fsys at prefix on top of the library: keys under prefix
// ("" for the whole root) are read from fsys first and fall through to
// what was mounted before, then to the packs and the base FS. Later mounts
// win, and every Mount stays above the packs, even those mounted after it,
// which makes patches (and later mods) an overlay. Call it on the game
// goroutine; assets already loaded keep their values until they reload.
func (l *Library) Mount(prefix string, fsys fs.FS) {
	l.fs.add(layerOverlay, strings.TrimSuffix(prefix, "/"), fsys)
}

// Unmount removes the newest [Library.Mount] of fsys at prefix and reports
// whether there was one.
func (l *Library) Unmount(prefix string, fsys fs.FS) bool {
	return l.fs.remove(layerOverlay, strings.TrimSuffix(prefix, "/"), fsys)
}

// MountPackage loads the packs of the package name and of the packages it
// uses into sc and blocks until they are mounted, so the package's files
// can be read right away, by LoadSync calls for example. With loose files
// it does nothing. Use it at boot (game.New mounts boot/ or global/ this
// way), never in a running level: it drives the server like
// asset.LoadSync.
//
// It returns the joined errors of the packs that failed, and panics like
// [Library.LoadPackage].
func (l *Library) MountPackage(sc *asset.Scope, name string) error {
	p := l.mustPackage("MountPackage", name)
	var errs []error
	for _, pn := range append([]string{p.Name}, p.Uses...) {
		if up, ok := l.cat.Package(pn); ok && up.Pack != "" {
			if _, err := asset.LoadSync[Pack](sc, up.Pack); err != nil {
				errs = append(errs, err)
			}
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("library: package %s: %w", name, errors.Join(errs...))
	}
	return nil
}

// mustPackage returns the package name or panics.
func (l *Library) mustPackage(op, name string) *Package {
	if l.srv == nil {
		panic("library: " + op + " before Attach")
	}
	p, ok := l.cat.Package(name)
	if !ok {
		panic(fmt.Sprintf("library: unknown package %q", name))
	}
	return p
}

// LoadPackage loads the package name into sc and returns at once: it
// mounts the packs of the package and of the packages it uses (in a
// packed build; each pack stays mounted while sc holds it), then loads
// every entry through the loader of its kind. It is the explicit preload of
// a package, for assets code spawns or plays by key. Call
// [Library.Update] after each Server.Update (the engine does) so the
// entries start once the packs are mounted; with loose files they start at
// once.
//
// It panics with "library: unknown package" and with "library:
// LoadPackage before Attach".
func (l *Library) LoadPackage(sc *asset.Scope, name string) *Load {
	return l.start("LoadPackage", sc, name, true)
}

// HoldPackage mounts the packs of the package name and of the packages it
// uses into sc, like [Library.LoadPackage], but loads no entries: the
// package's files become readable for loads that name them. The engine
// calls it before a level of the package spawns. It panics like
// LoadPackage.
func (l *Library) HoldPackage(sc *asset.Scope, name string) *Load {
	return l.start("HoldPackage", sc, name, false)
}

func (l *Library) start(op string, sc *asset.Scope, name string, entries bool) *Load {
	p := l.mustPackage(op, name)
	ld := &Load{lib: l, sc: sc, pkg: p, entries: entries}
	for _, pn := range append([]string{p.Name}, p.Uses...) {
		if up, ok := l.cat.Package(pn); ok && up.Pack != "" {
			ld.packs = append(ld.packs, asset.Load[Pack](sc, up.Pack))
		}
	}
	if !ld.advance() {
		l.pending = append(l.pending, ld)
	}
	return ld
}

// Update starts the entries of package loads whose packs are mounted and
// settles those whose packs failed. Call it on the game goroutine after
// Server.Update. With nothing pending it is a length check.
func (l *Library) Update() {
	if len(l.pending) == 0 {
		return
	}
	keep := l.pending[:0]
	for _, ld := range l.pending {
		if !ld.advance() {
			keep = append(keep, ld)
		}
	}
	clear(l.pending[len(keep):])
	l.pending = keep
}

// Load is a package being loaded by [Library.LoadPackage] or held by
// [Library.HoldPackage].
type Load struct {
	lib     *Library
	sc      *asset.Scope
	pkg     *Package
	entries bool // load the package's entries once mounted
	packs   []asset.Handle[Pack]
	started bool  // the packs are mounted (and the entries, if any, started)
	err     error // a pack failed; the entries never start
}

// Package returns the package being loaded.
func (ld *Load) Package() *Package { return ld.pkg }

// Scope returns the scope the package loads into.
func (ld *Load) Scope() *asset.Scope { return ld.sc }

// Mounted reports whether the package's files are readable: its packs (if
// any) are mounted and its entries (for LoadPackage) have started loading.
func (ld *Load) Mounted() bool { return ld.started }

// Progress returns how much is ready and how much there is: the packs
// until they are mounted, then everything the scope needs (sc.Progress).
func (ld *Load) Progress() (ready, total int) {
	if ld.started {
		return ld.sc.Progress()
	}
	for _, h := range ld.packs {
		if ld.lib.packs.Ready(h) {
			ready++
		}
	}
	total = len(ld.packs)
	if ld.entries {
		total += len(ld.pkg.Entries)
	}
	return ready, total
}

// Done reports whether loading has settled: every asset of the scope is
// Ready or Failed, or a pack failed.
func (ld *Load) Done() bool {
	return ld.err != nil || ld.started && ld.sc.Done()
}

// Err returns the failed packs or the failed assets of the scope, joined,
// or nil.
func (ld *Load) Err() error {
	if ld.err != nil {
		return ld.err
	}
	return ld.sc.Err()
}

// advance starts the entries once every pack is Ready and reports whether
// the load needs no more Updates.
func (ld *Load) advance() bool {
	var errs []error
	for _, h := range ld.packs {
		_, st := ld.lib.packs.Lookup(h)
		switch st {
		case asset.Ready:
		case asset.Failed:
			errs = append(errs, ld.lib.packs.Err(h))
		default:
			return false
		}
	}
	if len(errs) > 0 {
		ld.err = fmt.Errorf("library: package %s: %w", ld.pkg.Name, errors.Join(errs...))
		return true
	}
	if ld.entries {
		for _, key := range ld.pkg.Entries {
			p, _, _ := strings.Cut(key, "#")
			if fn := ld.lib.loaders[KindOf(p)]; fn != nil {
				fn(ld.sc, key)
			}
		}
	}
	ld.started = true
	return true
}
