package library

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"testing/fstest"

	"github.com/weakpixel/asset"
)

// open returns a library over fsys attached to a server (Workers 0) where
// every kind loads as text.
func open(t *testing.T, fsys fs.FS) (*Library, *asset.Server) {
	t.Helper()
	lib, err := Open(fsys)
	if err != nil {
		t.Fatal(err)
	}
	srv := asset.NewServer(asset.Options{FS: lib.FS(), OnError: func(error) {}})
	t.Cleanup(srv.Close)
	asset.Register[text, string](srv, textLoader{}, text{S: "placeholder"})
	lib.Attach(srv)
	for _, k := range []Kind{KindTexture, KindAtlas, KindClipSet, KindTileset, KindMap, KindWorld, KindFont, KindShader, KindSound, KindMusic} {
		lib.SetLoader(k, func(sc *asset.Scope, key string) { asset.Load[text](sc, key) })
	}
	return lib, srv
}

// settle runs Update until ld is done.
func settle(t *testing.T, lib *Library, srv *asset.Server, ld *Load) {
	t.Helper()
	for range 100 {
		srv.Update()
		lib.Update()
		if ld.Done() {
			return
		}
	}
	t.Fatal("package load did not settle")
}

// textOf loads key as text into sc and returns it, or "" if it fails.
func textOf(srv *asset.Server, sc *asset.Scope, key string) string {
	h, err := asset.LoadSync[text](sc, key)
	if err != nil {
		return ""
	}
	return asset.StoreOf[text](srv).Get(h).S
}

func TestLoadPackageLoose(t *testing.T) {
	lib, srv := open(t, fixture())
	sc := srv.NewScope("forest")
	ld := lib.LoadPackage(sc, "forest")
	if !ld.Mounted() {
		t.Fatal("loose files must start at once")
	}
	settle(t, lib, srv, ld)
	if err := ld.Err(); err != nil {
		t.Fatal(err)
	}
	if ready, total := ld.Progress(); ready != 2 || total != 2 {
		t.Fatalf("progress %d/%d, want the two entries", ready, total)
	}
	if got := textOf(srv, sc, "forest/music/theme.ogg"); got != "ogg" {
		t.Fatalf("theme = %q", got)
	}
}

func TestOpenNeedsCatalogWithoutListing(t *testing.T) {
	_, err := Open(noList{fixture()})
	if err == nil || !strings.Contains(err.Error(), "cannot list directories and has no assets.catalog.json") {
		t.Fatalf("err %v", err)
	}
	cat, _, _ := Scan(fixture())
	fsys := fixture()
	fsys[CatalogFile] = &fstest.MapFile{Data: cat.Encode()}
	lib, err := Open(noList{fsys})
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := lib.Package("trees"); !ok {
		t.Fatal("catalog packages missing")
	}
}

func TestLoadPackagePanics(t *testing.T) {
	lib, err := Open(fixture())
	if err != nil {
		t.Fatal(err)
	}
	srv := asset.NewServer(asset.Options{FS: lib.FS()})
	defer srv.Close()
	mustPanic(t, "library: LoadPackage before Attach", func() { lib.LoadPackage(srv.Global(), "global") })
	lib.Attach(srv)
	mustPanic(t, "library: Attach called twice", func() { lib.Attach(srv) })
	mustPanic(t, `library: unknown package "levels/none"`, func() { lib.LoadPackage(srv.Global(), "levels/none") })
}

// packed writes the fixture as packs and opens it like a web build would:
// from a file system that cannot list directories.
func packed(t *testing.T) (*Library, *asset.Server, string) {
	t.Helper()
	out := t.TempDir()
	if _, _, err := WritePacks(fixture(), out); err != nil {
		t.Fatal(err)
	}
	lib, srv := open(t, noList{os.DirFS(out)})
	return lib, srv, out
}

func TestWritePacksLayout(t *testing.T) {
	_, _, out := packed(t)
	data, err := os.ReadFile(filepath.Join(out, CatalogFile))
	if err != nil {
		t.Fatal(err)
	}
	cat, err := ReadCatalog(data)
	if err != nil {
		t.Fatal(err)
	}
	g, _ := cat.Package("forest")
	if !strings.HasPrefix(g.Pack, "packs/forest.") || !strings.HasSuffix(g.Pack, ".zip") || g.PackSize == 0 || len(g.PackSHA256) != 64 {
		t.Fatalf("pack %+v", g)
	}
	if _, err := os.Stat(filepath.Join(out, "forest")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal("packaged files must not be copied loose")
	}
	if _, err := os.Stat(filepath.Join(out, "library.json")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatal("library.json is read into the catalog, not copied")
	}
	out2 := t.TempDir()
	cat2, _, err := WritePacks(fixture(), out2)
	if err != nil {
		t.Fatal(err)
	}
	if g2, _ := cat2.Package("forest"); g2.Pack != g.Pack {
		t.Fatalf("packs are not deterministic: %s vs %s", g2.Pack, g.Pack)
	}
}

func TestLoadPackagePacked(t *testing.T) {
	lib, srv, _ := packed(t)

	if _, err := fs.ReadFile(lib.FS(), "forest/bg.png"); err == nil || !strings.Contains(err.Error(), "library: package forest is packed and not loaded") {
		t.Fatalf("read before mount: %v", err)
	}
	if !errors.Is(func() error { _, err := fs.ReadFile(lib.FS(), "forest/bg.png"); return err }(), fs.ErrNotExist) {
		t.Fatal("the not-loaded error must wrap fs.ErrNotExist")
	}

	sc := srv.NewScope("forest")
	ld := lib.LoadPackage(sc, "forest")
	if ld.Mounted() {
		t.Fatal("packed package mounted before its packs loaded")
	}
	if ready, total := ld.Progress(); ready != 0 || total != 3+2 {
		t.Fatalf("progress %d/%d, want 0 of 3 packs + 2 entries", ready, total)
	}
	settle(t, lib, srv, ld)
	if err := ld.Err(); err != nil {
		t.Fatal(err)
	}
	if got := lib.Mounted(); !slices.Equal(got, []string{"forest", "global", "trees"}) {
		t.Fatalf("mounted = %v", got)
	}
	if got := textOf(srv, sc, "forest/forest.tmj"); !strings.Contains(got, "trees.tsj") {
		t.Fatalf("map read from pack = %q", got)
	}
	// The used packages' packs are mounted too.
	if got := textOf(srv, sc, "trees/trees.png"); got != "png" {
		t.Fatalf("used package file = %q", got)
	}

	sc.Release()
	srv.Update()
	if _, err := fs.ReadFile(lib.FS(), "forest/bg.png"); err == nil {
		t.Fatal("pack still mounted after its scope was released")
	}
	if got := lib.Mounted(); len(got) != 0 {
		t.Fatalf("mounted after release = %v", got)
	}
}

func TestLoadPackagePackFails(t *testing.T) {
	_, _, out := packed(t)
	data, _ := os.ReadFile(filepath.Join(out, CatalogFile))
	cat, _ := ReadCatalog(data)
	g, _ := cat.Package("title")
	if err := os.WriteFile(filepath.Join(out, filepath.FromSlash(g.Pack)), []byte("not a zip"), 0o644); err != nil {
		t.Fatal(err)
	}
	lib, srv := open(t, noList{os.DirFS(out)})
	ld := lib.LoadPackage(srv.NewScope("title"), "title")
	settle(t, lib, srv, ld)
	if err := ld.Err(); err == nil || !strings.Contains(err.Error(), "library: package title:") || ld.Mounted() {
		t.Fatalf("err %v, mounted %v", ld.Err(), ld.Mounted())
	}
}

func TestMountOverlay(t *testing.T) {
	lib, err := Open(fixture())
	if err != nil {
		t.Fatal(err)
	}
	mod := fstest.MapFS{"hero/hero.png": {Data: []byte("modded")}}
	patch := fstest.MapFS{"global/hero/hero.png": {Data: []byte("patched")}}
	read := func(p string) string {
		data, err := fs.ReadFile(lib.FS(), p)
		if err != nil {
			return "error: " + err.Error()
		}
		return string(data)
	}
	lib.Mount("global/", mod)
	if got := read("global/hero/hero.png"); got != "modded" {
		t.Fatalf("mount = %q", got)
	}
	if got := read("global/sfx/hit.wav"); got != "wav" {
		t.Fatalf("fall through to base = %q", got)
	}
	lib.Mount("", patch)
	if got := read("global/hero/hero.png"); got != "patched" {
		t.Fatalf("newest mount must win: %q", got)
	}
	if !lib.Unmount("", patch) || lib.Unmount("", patch) {
		t.Fatal("Unmount must remove exactly once")
	}
	lib.Unmount("global", mod)
	if got := read("global/hero/hero.png"); got != "png" {
		t.Fatalf("after unmount = %q", got)
	}
	if _, err := lib.FS().Open("../x"); !errors.Is(err, fs.ErrInvalid) {
		t.Fatalf("invalid path: %v", err)
	}
	if fi, err := fs.Stat(lib.FS(), "global/sfx/hit.wav"); err != nil || fi.Size() != 3 {
		t.Fatalf("stat: %v %v", fi, err)
	}
}

// TestMountConcurrentReads runs under -race (make all does not, run
// `go test -race ./asset/library`): workers read while the game goroutine
// mounts and unmounts.
func TestMountConcurrentReads(t *testing.T) {
	lib, err := Open(fixture())
	if err != nil {
		t.Fatal(err)
	}
	var wg sync.WaitGroup
	stop := make(chan struct{})
	for range 4 {
		wg.Go(func() {
			for {
				select {
				case <-stop:
					return
				default:
				}
				if _, err := fs.ReadFile(lib.FS(), "global/sfx/hit.wav"); err != nil {
					t.Error(err)
					return
				}
			}
		})
	}
	for i := range 200 {
		m := fstest.MapFS{"x.png": {Data: []byte{byte(i)}}}
		lib.Mount("global", m)
		lib.Unmount("global", m)
	}
	close(stop)
	wg.Wait()
}

func TestMountPackage(t *testing.T) {
	lib, srv, _ := packed(t)
	if err := lib.MountPackage(srv.Global(), "forest"); err != nil {
		t.Fatal(err)
	}
	// Mounted before any Update: a LoadSync reads from the packs at once,
	// including those of the packages it uses.
	for _, key := range []string{"forest/bg.png", "trees/trees.png", "global/hero/hero.png"} {
		if got := textOf(srv, srv.Global(), key); got != "png" {
			t.Errorf("%s = %q", key, got)
		}
	}
	// LoadPackage afterwards starts the entries at once.
	if ld := lib.LoadPackage(srv.Global(), "forest"); !ld.Mounted() {
		t.Fatal("LoadPackage after MountPackage must start at once")
	}
	loose, lsrv := open(t, fixture())
	if err := loose.MountPackage(lsrv.Global(), "global"); err != nil {
		t.Fatal(err)
	}
	mustPanic(t, `library: unknown package "nope"`, func() { _ = loose.MountPackage(lsrv.Global(), "nope") })
}

// TestHoldPackage checks that holding a package mounts its packs and those
// of the packages it uses without loading its entries.
func TestHoldPackage(t *testing.T) {
	lib, srv, _ := packed(t)
	sc := srv.NewScope("cave")
	ld := lib.HoldPackage(sc, "cave")
	if ready, total := ld.Progress(); ready != 0 || total != 2 {
		t.Fatalf("progress %d/%d, want 0 of 2 packs (cave, forest) and no entries", ready, total)
	}
	settle(t, lib, srv, ld)
	if err := ld.Err(); err != nil || !ld.Mounted() {
		t.Fatalf("err %v, mounted %v", err, ld.Mounted())
	}
	if got := lib.Mounted(); !slices.Equal(got, []string{"cave", "forest"}) {
		t.Fatalf("mounted = %v", got)
	}
	if _, total := sc.Progress(); total != 2 {
		t.Fatalf("scope holds %d assets, want only the 2 packs", total)
	}
	if got := textOf(srv, sc, "forest/bg.png"); got != "png" {
		t.Fatalf("used package file = %q", got)
	}
}

// TestMountStaysAbovePacks checks the layer order: an overlay mounted
// before a package's pack still wins over it.
func TestMountStaysAbovePacks(t *testing.T) {
	lib, srv, _ := packed(t)
	lib.Mount("forest", fstest.MapFS{"bg.png": {Data: []byte("mod")}})
	ld := lib.HoldPackage(srv.NewScope("forest"), "forest")
	settle(t, lib, srv, ld)
	data, err := fs.ReadFile(lib.FS(), "forest/bg.png")
	if err != nil || string(data) != "mod" {
		t.Fatalf("forest/bg.png = %q, %v; want the overlay", data, err)
	}
	if data, err := fs.ReadFile(lib.FS(), "forest/forest.tmj"); err != nil || !strings.Contains(string(data), "trees.tsj") {
		t.Fatalf("pack file under the overlay = %q, %v", data, err)
	}
}
