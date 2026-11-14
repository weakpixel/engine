package game

import (
	"bytes"
	"image"
	"image/png"
	"testing"
	"testing/fstest"
	"time"

	"github.com/weakpixel/ecs"
	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/render"
	"github.com/weakpixel/engine/tilemap"
)

// mapFiles is a 4x2 map of 8x8 tiles with two tile layers, a world placing
// it twice, and a map whose tileset is missing.
func mapFiles(t *testing.T) fstest.MapFS {
	img := image.NewRGBA(image.Rect(0, 0, 16, 16))
	for i := range img.Pix {
		img.Pix[i] = 255
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	m := `{"orientation":"orthogonal","width":4,"height":2,"tilewidth":8,"tileheight":8,
		"tilesets":[{"firstgid":1,"source":"t.tsj"}],
		"layers":[{"type":"tilelayer","name":"a","width":4,"height":2,"data":[1,2,3,4,1,2,3,4]},
		          {"type":"objectgroup","name":"o","objects":[{"id":1,"name":"spawn","point":true,"x":4,"y":4}]},
		          {"type":"tilelayer","name":"b","width":4,"height":2,"data":[0,0,1,0,0,0,0,0]}]}`
	return fstest.MapFS{
		"m.tmj":       {Data: []byte(m)},
		"m2.tmj":      {Data: []byte(m)},
		"w.world":     {Data: []byte(`{"maps":[{"fileName":"m.tmj","x":0,"y":0},{"fileName":"m2.tmj","x":32,"y":0}]}`)},
		"t.tsj":       {Data: []byte(`{"name":"t","image":"t.png","tilewidth":8,"tileheight":8,"columns":2,"tilecount":4}`)},
		"t.png":       {Data: buf.Bytes()},
		"broken.tmj":  {Data: []byte(`{"orientation":"orthogonal","width":1,"height":1,"tilewidth":8,"tileheight":8,"tilesets":[{"firstgid":1,"source":"nope.tsj"}],"layers":[]}`)},
		"bad.world":   {Data: []byte(`{"maps":[{"fileName":"m.tmj"},{"fileName":"broken.tmj"}]}`)},
		"unknown.txt": {Data: []byte(`x`)},
	}
}

// tickUntil updates e until done reports true and checks inv after every
// tick. Assets decode on worker goroutines, so it waits in real time.
func tickUntil(t *testing.T, e *Engine, done func() bool, inv func()) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if err := e.Update(); err != nil {
			t.Fatal(err)
		}
		inv()
		if done() {
			return
		}
		time.Sleep(100 * time.Microsecond)
	}
	t.Fatal("condition never met")
}

func layerCount(w *ecs.World) (n int) {
	for range ecs.NewView1[tilemap.TileLayer](w).Each() {
		n++
	}
	return n
}

func TestLoadMap(t *testing.T) {
	e := New(Config{FS: mapFiles(t), Width: 64, Height: 32, Camera: render.Camera{Pos: gfx.Vec2{X: 16, Y: 8}}})
	defer e.Close()
	var l *Level
	e.Start(func(e *Engine) {
		var err error
		if l, err = e.LoadMap("m.tmj", tilemap.SpawnOptions{Pos: gfx.Vec2{X: 2, Y: 3}, BaseLayer: 5}); err != nil {
			t.Fatal(err)
		}
	})
	if err := e.Update(); err != nil {
		t.Fatal(err)
	}
	maps := e.Stores().Maps()
	// The layers appear in the tick in which the map becomes Ready, not
	// before and not later.
	tickUntil(t, e, l.Spawned, func() {
		if ready := maps.Ready(l.mapH); ready != l.Spawned() {
			t.Fatalf("ready = %v, spawned = %v", ready, l.Spawned())
		}
		if !l.Spawned() && l.Bounds() != (gfx.Rect{}) {
			t.Fatalf("Bounds before spawn = %+v, want zero", l.Bounds())
		}
	})
	if got, want := l.Bounds(), (gfx.Rect{Min: gfx.Vec2{X: 2, Y: 3}, Max: gfx.Vec2{X: 34, Y: 19}}); got != want {
		t.Errorf("Bounds = %+v, want %+v", got, want)
	}
	if len(l.Maps()) != 1 || len(l.Maps()[0].Layers) != 2 || l.Maps()[0].Pos != (gfx.Vec2{X: 2, Y: 3}) || layerCount(e.World()) != 2 {
		t.Fatalf("maps = %+v", l.Maps())
	}
	tl := ecs.GetComp[tilemap.TileLayer](e.World(), l.Maps()[0].Layers[1])
	if tl.Layer != 6 {
		t.Errorf("second layer on render layer %d, want 6", tl.Layer)
	}
	tr := ecs.GetComp[gfx.Transform](e.World(), l.Maps()[0].Layers[0])
	if tr.Pos != (gfx.Vec2{X: 2, Y: 3}) || tr.PrevPos != tr.Pos {
		t.Errorf("transform = %+v", tr)
	}
	// Objects are data in the map.
	m := maps.Get(l.Maps()[0].Map)
	if _, o, ok := m.FindObject("spawn"); !ok || o.X != 4 {
		t.Error("object not found")
	}

	// The next tick queues the tiles: 8 + 1 cells, all inside the camera.
	if err := e.Update(); err != nil {
		t.Fatal(err)
	}
	tiles := 0
	for i := range e.queue.Len() {
		if it := e.queue.At(i); it.Layer == 5 || it.Layer == 6 {
			tiles++
		}
	}
	if tiles != 9 {
		t.Errorf("%d tile items queued, want 9", tiles)
	}

	e.UnloadLevel(l)
	if layerCount(e.World()) != 0 {
		t.Error("layers survive UnloadLevel")
	}
	e.UnloadLevel(l)
}

func TestLoadMapWorldAndFailures(t *testing.T) {
	e := New(Config{FS: mapFiles(t), OnAssetError: func(error) {}})
	defer e.Close()
	if err := e.Update(); err != nil {
		t.Fatal(err)
	}
	if _, err := e.LoadMap("unknown.txt", tilemap.SpawnOptions{}); err == nil {
		t.Error("LoadMap of a .txt succeeded")
	}
	world, err := e.LoadMap("w.world", tilemap.SpawnOptions{Pos: gfx.Vec2{X: 100}})
	if err != nil {
		t.Fatal(err)
	}
	broken, _ := e.LoadMap("broken.tmj", tilemap.SpawnOptions{})
	badWorld, _ := e.LoadMap("bad.world", tilemap.SpawnOptions{})
	tickUntil(t, e, func() bool { return world.Spawned() && broken.Done() && badWorld.Done() }, func() {})
	if got := world.Maps(); len(got) != 2 || got[0].Pos != (gfx.Vec2{X: 100}) || got[1].Pos != (gfx.Vec2{X: 132}) {
		t.Fatalf("world maps = %+v", got)
	}
	// Two 32x16 maps side by side at x 100 and 132.
	if got, want := world.Bounds(), (gfx.Rect{Min: gfx.Vec2{X: 100}, Max: gfx.Vec2{X: 164, Y: 16}}); got != want {
		t.Errorf("world Bounds = %+v, want %+v", got, want)
	}
	if broken.Bounds() != (gfx.Rect{}) {
		t.Errorf("failed level Bounds = %+v, want zero", broken.Bounds())
	}
	if broken.Spawned() || broken.Err() == nil || badWorld.Spawned() || badWorld.Err() == nil {
		t.Errorf("failed levels: spawned %v %v, err %v / %v", broken.Spawned(), badWorld.Spawned(), broken.Err(), badWorld.Err())
	}
	if n := layerCount(e.World()); n != 4 {
		t.Errorf("%d layers, want 4 (two per world map, none for the failures)", n)
	}
	if len(e.pending) != 0 {
		t.Errorf("%d levels still pending", len(e.pending))
	}

	// Unloading a level that has not spawned yet cancels it.
	late, _ := e.LoadMap("m2.tmj", tilemap.SpawnOptions{})
	e.UnloadLevel(late)
	for range 20 {
		e.Update()
	}
	if late.Spawned() || layerCount(e.World()) != 4 {
		t.Error("an unloaded level spawned")
	}
}

func TestTickNoAllocsWithMap(t *testing.T) {
	if testing.CoverMode() != "" || raceEnabled {
		t.Skip("instrumentation allocates")
	}
	if pixelsEnabled {
		t.Skip("a real graphics driver renders the chunk canvases, which allocates")
	}
	e := New(Config{FS: mapFiles(t), Width: 64, Height: 32, Camera: render.Camera{Pos: gfx.Vec2{X: 16, Y: 8}}})
	defer e.Close()
	if err := e.Update(); err != nil {
		t.Fatal(err)
	}
	l, err := e.LoadMap("m.tmj", tilemap.SpawnOptions{})
	if err != nil {
		t.Fatal(err)
	}
	tickUntil(t, e, l.Spawned, func() {})
	e.Update() // warm up the queue with the tiles
	if n := testing.AllocsPerRun(20, func() { e.Update() }); n != 0 {
		t.Fatalf("allocs per tick = %v, want 0", n)
	}
	if e.queue.Len() == 0 {
		t.Fatal("nothing queued")
	}
}

// pairEngine is an engine over mapFiles plus the given extra files, counting
// reports to OnAssetError.
func pairEngine(t *testing.T, extra map[string]string, reported *[]error) *Engine {
	fsys := prefabFiles(t)
	for k, v := range extra {
		fsys[k] = &fstest.MapFile{Data: []byte(v)}
	}
	e := New(Config{NoAudio: true, FS: fsys, OnAssetError: func(err error) { *reported = append(*reported, err) }})
	t.Cleanup(e.Close)
	return e
}

func loadPair(t *testing.T, e *Engine, key string, opts tilemap.SpawnOptions) *Level {
	t.Helper()
	l, err := e.LoadMap(key, opts)
	if err != nil {
		t.Fatal(err)
	}
	tickUntil(t, e, func() bool { return l.Spawned() }, func() {})
	return l
}

func TestLoadMapPairedDocument(t *testing.T) {
	var reported []error
	e := pairEngine(t, map[string]string{"m.level.json": visualDoc}, &reported)

	// A map with its pair: one level, one scope, objects and layers.
	l := loadPair(t, e, "m.tmj", tilemap.SpawnOptions{})
	if len(l.Objects()) != 1 || len(l.Maps()) != 1 || layerCount(e.World()) != 2 || l.Scope() == nil {
		t.Fatalf("objects %d maps %d layers %d", len(l.Objects()), len(l.Maps()), layerCount(e.World()))
	}
	e.UnloadLevel(l)
	if layerCount(e.World()) != 0 || count(ecs.NewView1[gfx.Transform](e.World())) != 0 {
		t.Fatal("UnloadLevel left entities")
	}

	// Without the pair, and with a missing pair that is not an error.
	l = loadPair(t, e, "m2.tmj", tilemap.SpawnOptions{})
	if len(l.Objects()) != 0 || l.Err() != nil || len(reported) != 0 {
		t.Fatalf("objects %d err %v reported %v", len(l.Objects()), l.Err(), reported)
	}

	// Opt-outs.
	l = loadPair(t, e, "m.tmj", tilemap.SpawnOptions{NoDocuments: true})
	if len(l.Objects()) != 0 || len(l.Maps()) != 1 {
		t.Fatalf("NoDocuments: objects %d", len(l.Objects()))
	}
	l = loadPair(t, e, "m.tmj", tilemap.SpawnOptions{SkipDocuments: []string{"m.level.json"}})
	if len(l.Objects()) != 0 || len(l.Maps()) != 1 {
		t.Fatalf("SkipDocuments: objects %d", len(l.Objects()))
	}
}

func TestLoadWorldPairedDocuments(t *testing.T) {
	var reported []error
	e := pairEngine(t, map[string]string{"w.level.json": visualDoc, "m2.level.json": visualDoc}, &reported)
	l := loadPair(t, e, "w.world", tilemap.SpawnOptions{})
	// The world's pair and the pair of the second child; m.tmj has none.
	if len(l.Objects()) != 2 || len(l.Maps()) != 2 || l.Err() != nil || len(reported) != 0 {
		t.Fatalf("objects %d maps %d err %v", len(l.Objects()), len(l.Maps()), l.Err())
	}
	e.UnloadLevel(l)
	if count(ecs.NewView1[gfx.Transform](e.World())) != 0 {
		t.Fatal("UnloadLevel left entities")
	}
}

func TestLoadMapPairedDocumentErrors(t *testing.T) {
	var reported []error
	e := pairEngine(t, map[string]string{
		"m.level.json":  `{"objects":[{"group":"gfx","class":"Nope","components":{}}]}`,
		"m2.level.json": `{"objects":[{"prefab":"prefabs/box.prefab.json"}]}`,
	}, &reported)
	// An invalid pair is reported; the map still spawns.
	l := loadPair(t, e, "m.tmj", tilemap.SpawnOptions{})
	if l.Err() == nil || len(reported) != 1 || len(l.Maps()) != 1 || layerCount(e.World()) != 2 {
		t.Fatalf("err %v reported %d maps %d", l.Err(), len(reported), len(l.Maps()))
	}
	// A pair with prefab instances spawns once its prefab is Ready.
	l = loadPair(t, e, "m2.tmj", tilemap.SpawnOptions{})
	if len(l.Objects()) != 1 || len(l.Maps()) != 1 || l.Err() != nil {
		t.Fatalf("prefab pair: objects %d err %v", len(l.Objects()), l.Err())
	}
}
