package game

import (
	"errors"
	"io/fs"
	"os"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/weakpixel/asset"
	"github.com/weakpixel/ecs"
	"github.com/weakpixel/engine/asset/library"
	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/render"
	"github.com/weakpixel/engine/tilemap"
)

// libraryFiles is a library of packages: a texture in global/, a package
// with a GOD document and a map at the same base, a package with a broken
// document and one with a document only.
func libraryFiles(t *testing.T) fstest.MapFS {
	m := mapFiles(t)
	return fstest.MapFS{
		"global/ui.png":          m["t.png"],
		"one/one.level.json":     {Data: []byte(visualDoc)},
		"one/one.tmj":            m["m.tmj"],
		"one/t.tsj":              m["t.tsj"],
		"one/t.png":              m["t.png"],
		"one/extra.level.json":   {Data: []byte(visualDoc)},
		"two/two.level.json":     {Data: []byte(`{"objects":[{"group":"gfx","class":"Nope","components":{}}]}`)},
		"title/title.level.json": {Data: []byte(visualDoc)},
	}
}

func newLibraryEngine(t *testing.T, fsys fs.FS, onErr func(error)) *Engine {
	t.Helper()
	lib, err := library.Open(fsys)
	if err != nil {
		t.Fatal(err)
	}
	e := New(Config{NoAudio: true, Library: lib, OnAssetError: onErr})
	t.Cleanup(e.Close)
	return e
}

// packedLibrary writes fsys as a packed build and returns it.
func packedLibrary(t *testing.T, fsys fs.FS) fs.FS {
	t.Helper()
	out := t.TempDir()
	if _, _, err := library.WritePacks(fsys, out); err != nil {
		t.Fatal(err)
	}
	return os.DirFS(out)
}

func TestLibraryBootAndLevelBase(t *testing.T) {
	e := newLibraryEngine(t, libraryFiles(t), nil)
	objects := ecs.NewView1[gfx.Transform](e.World())
	boot := e.Boot()
	if boot == nil || boot.Package().Name != "global" {
		t.Fatalf("boot %v", boot)
	}
	tickUntil(t, e, boot.Done, func() {})
	if err := boot.Err(); err != nil {
		t.Fatal(err)
	}
	ui := asset.Load[render.Texture](e.Assets().Global(), "global/ui.png")
	if !asset.StoreOf[render.Texture](e.Assets()).Ready(ui) {
		t.Fatal("boot did not load the global texture")
	}

	l, err := e.LoadLevelBase("one/one", tilemap.SpawnOptions{})
	if err != nil {
		t.Fatal(err)
	}
	// Loose files: the GOD objects exist at once, the map spawns later.
	if l.Path() != "one/one" || len(l.Objects()) != 1 || count(objects) != 1 || l.Spawned() {
		t.Fatalf("path %q objects %d entities %d spawned %v", l.Path(), len(l.Objects()), count(objects), l.Spawned())
	}
	tickUntil(t, e, func() bool { return l.Spawned() && l.Done() }, func() {})
	if err := l.Err(); err != nil {
		t.Fatal(err)
	}
	if layerCount(e.World()) != 2 || len(l.Maps()) != 1 {
		t.Fatalf("layers %d maps %d", layerCount(e.World()), len(l.Maps()))
	}
	e.UnloadLevel(l)
	if layerCount(e.World()) != 0 || count(objects) != 0 {
		t.Fatal("UnloadLevel left entities")
	}

	// A base with a document only, in another package.
	s, err := e.LoadLevelBase("title/title", tilemap.SpawnOptions{})
	if err != nil || !s.Spawned() || len(s.Objects()) != 1 {
		t.Fatalf("title %v %v", s, err)
	}
}

func TestLibraryPackedLevelBase(t *testing.T) {
	var reported []error
	e := newLibraryEngine(t, packedLibrary(t, libraryFiles(t)), func(err error) { reported = append(reported, err) })
	objects := ecs.NewView1[gfx.Transform](e.World())

	// New mounted the global pack: global/ is readable before any Update.
	if !e.Boot().Mounted() {
		t.Fatal("global pack not mounted by New")
	}
	if _, err := asset.LoadSync[render.Texture](e.Assets().Global(), "global/ui.png"); err != nil {
		t.Fatal(err)
	}

	l, err := e.LoadLevelBase("one/one", tilemap.SpawnOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if len(l.Objects()) != 0 || l.Spawned() || l.Done() {
		t.Fatal("a packed level spawned before its pack was mounted")
	}
	if got := e.Library().Mounted(); !slices.Equal(got, []string{"global"}) {
		t.Fatalf("mounted before the pack loaded = %v", got)
	}
	tickUntil(t, e, func() bool { return l.Spawned() && l.Done() }, func() {})
	if err := l.Err(); err != nil {
		t.Fatal(err)
	}
	// Layer entities have a Transform too: 1 object + 2 layers. The package's
	// other document (extra.level.json) is not preloaded.
	if len(l.Objects()) != 1 || count(objects) != 3 || layerCount(e.World()) != 2 {
		t.Fatalf("objects %d entities %d layers %d", len(l.Objects()), count(objects), layerCount(e.World()))
	}
	if got := e.Library().Mounted(); !slices.Equal(got, []string{"global", "one"}) {
		t.Fatalf("mounted = %v", got)
	}

	e.UnloadLevel(l)
	if err := e.Update(); err != nil {
		t.Fatal(err)
	}
	if _, err := fs.ReadFile(e.Library().FS(), "one/one.tmj"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("pack still mounted after UnloadLevel: %v", err)
	}

	// A document that does not spawn is reported once the pack is mounted.
	bad, err := e.LoadLevelBase("two/two", tilemap.SpawnOptions{})
	if err != nil {
		t.Fatal(err)
	}
	tickUntil(t, e, bad.Done, func() {})
	if err := bad.Err(); err == nil || !strings.Contains(err.Error(), `game: LoadLevelBase "two/two": two/two.level.json: objects[0]: unknown object gfx:Nope`) {
		t.Fatalf("err %v", err)
	}
	if len(reported) != 1 {
		t.Fatalf("reported %v", reported)
	}
}

// TestLibraryPackedLoadLevel checks that LoadLevel and LoadMap of a key in
// a packed package that is not mounted mount it first.
func TestLibraryPackedLoadLevel(t *testing.T) {
	e := newLibraryEngine(t, packedLibrary(t, libraryFiles(t)), nil)
	doc, err := e.LoadLevel("one/extra.level.json")
	if err != nil {
		t.Fatal(err)
	}
	if doc.Spawned() || len(doc.Objects()) != 0 {
		t.Fatal("spawned before the pack was mounted")
	}
	m, err := e.LoadMap("one/one.tmj", tilemap.SpawnOptions{NoDocuments: true})
	if err != nil {
		t.Fatal(err)
	}
	tickUntil(t, e, func() bool { return doc.Spawned() && m.Spawned() && doc.Done() && m.Done() }, func() {})
	if doc.Err() != nil || m.Err() != nil || len(doc.Objects()) != 1 || len(m.Maps()) != 1 {
		t.Fatalf("doc %d objects, %v; map %d maps, %v", len(doc.Objects()), doc.Err(), len(m.Maps()), m.Err())
	}
}

// TestLibraryBootPackage checks that New mounts boot/ before global/, and
// that global/ then loads in the background.
func TestLibraryBootPackage(t *testing.T) {
	files := libraryFiles(t)
	files["boot/logo.png"] = files["global/ui.png"]
	e := newLibraryEngine(t, packedLibrary(t, files), nil)
	if _, err := asset.LoadSync[render.Texture](e.Assets().Global(), "boot/logo.png"); err != nil {
		t.Fatalf("boot/ not mounted by New: %v", err)
	}
	boot := e.Boot()
	if boot.Package().Name != "global" || boot.Mounted() {
		t.Fatalf("boot load %s, mounted %v: global/ must load in the background", boot.Package().Name, boot.Mounted())
	}
	if _, total := boot.Progress(); total == 0 {
		t.Fatal("no progress to show")
	}
	tickUntil(t, e, boot.Done, func() {})
	if err := boot.Err(); err != nil {
		t.Fatal(err)
	}
	if got := e.Library().Mounted(); !slices.Equal(got, []string{"boot", "global"}) {
		t.Fatalf("mounted = %v", got)
	}
}

func TestLibraryLevelBaseErrors(t *testing.T) {
	e := newLibraryEngine(t, libraryFiles(t), nil)
	if _, err := e.LoadLevelBase("nope/nope", tilemap.SpawnOptions{}); err == nil || !strings.Contains(err.Error(), "the library has no package nope") {
		t.Fatalf("err %v", err)
	}
	if _, err := e.LoadLevelBase("one/missing", tilemap.SpawnOptions{}); err == nil || !strings.Contains(err.Error(), `game: LoadLevelBase "one/missing": no one/missing.level.json, one/missing.tmj or one/missing.world`) {
		t.Fatalf("err %v", err)
	}
	if _, err := e.LoadLevelBase("two/two", tilemap.SpawnOptions{}); err == nil || !strings.Contains(err.Error(), "unknown object gfx:Nope") {
		t.Fatalf("err %v", err)
	}
	if len(e.Levels()) != 0 {
		t.Fatal("a failed level was kept")
	}
	mustPanic(t, "game: LoadPackage: Config.Library is nil", func() {
		plain := New(Config{NoAudio: true})
		defer plain.Close()
		plain.LoadPackage(plain.Assets().Global(), "global")
	})

	lib, _ := library.Open(libraryFiles(t))
	mustPanic(t, "game: set Config.FS or Config.Library, not both", func() {
		New(Config{NoAudio: true, Library: lib, FS: libraryFiles(t)})
	})
}

// TestLibraryLoadPackage checks the explicit preload.
func TestLibraryLoadPackage(t *testing.T) {
	e := newLibraryEngine(t, libraryFiles(t), nil)
	sc := e.Assets().NewScope("one")
	ld := e.LoadPackage(sc, "one")
	tickUntil(t, e, ld.Done, func() {})
	if err := ld.Err(); err != nil {
		t.Fatal(err)
	}
	if _, total := sc.Progress(); total == 0 {
		t.Fatal("LoadPackage loaded nothing")
	}
}

func TestLibraryLevelBaseDocumentOptOut(t *testing.T) {
	e := newLibraryEngine(t, libraryFiles(t), nil)
	for name, opts := range map[string]tilemap.SpawnOptions{
		"NoDocuments":   {NoDocuments: true},
		"SkipDocuments": {SkipDocuments: []string{"one/one.level.json"}},
	} {
		l, err := e.LoadLevelBase("one/one", opts)
		if err != nil {
			t.Fatal(err)
		}
		tickUntil(t, e, func() bool { return l.Spawned() && l.Done() }, func() {})
		if len(l.Objects()) != 0 || len(l.Maps()) != 1 || l.Err() != nil {
			t.Errorf("%s: objects %d maps %d err %v", name, len(l.Objects()), len(l.Maps()), l.Err())
		}
		e.UnloadLevel(l)
	}
}
