package game

import (
	"slices"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/weakpixel/engine/god"

	"github.com/weakpixel/asset"
	"github.com/weakpixel/ecs"
	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/tilemap"
)

const visualDoc = `{"objects":[{"group":"gfx","class":"Visual","components":{"gfx:Transform":{"Pos":{"X":1,"Y":2},"PrevPos":{"X":1,"Y":2},"Rot":0,"Scale":{"X":1,"Y":1}},"gfx:Sprite":{"ID":"","Layer":0,"Origin":{"X":0,"Y":0},"FlipX":false,"FlipY":false,"Tint":0,"Blend":0,"Hidden":false}}}]}`

func TestLoadLevelData(t *testing.T) {
	e := New(Config{NoAudio: true}) // no FS: the document comes from memory
	defer e.Close()
	view := ecs.NewView1[gfx.Transform](e.World())

	l, err := e.LoadLevelData("unsaved.json", []byte(visualDoc))
	if err != nil {
		t.Fatal(err)
	}
	if l.Path() != "unsaved.json" || len(l.Objects()) != 1 || count(view) != 1 {
		t.Fatalf("path=%q objects=%d entities=%d", l.Path(), len(l.Objects()), count(view))
	}

	if _, err := e.LoadLevelData("bad.json", []byte(`{"objects":[{"group":"gfx","class":"Nope","components":{}}]}`)); err == nil ||
		!strings.Contains(err.Error(), `game: LoadLevel "bad.json": objects[0]: unknown object gfx:Nope`) {
		t.Fatalf("invalid document: %v", err)
	}
	if count(view) != 1 {
		t.Fatal("an invalid document spawned entities")
	}
	if got := e.Levels(); len(got) != 1 || got[0] != l {
		t.Fatalf("Levels = %v, want only the valid level", got)
	}
}

func TestLevels(t *testing.T) {
	fsys := mapFiles(t)
	fsys["lvl.json"] = &fstest.MapFile{Data: []byte(visualDoc)}
	e := New(Config{NoAudio: true, FS: fsys})
	defer e.Close()

	god, err := e.LoadLevel("lvl.json")
	if err != nil {
		t.Fatal(err)
	}
	m, err := e.LoadMap("m.tmj", tilemap.SpawnOptions{})
	if err != nil {
		t.Fatal(err)
	}
	mem, err := e.LoadLevelData("mem.json", []byte(visualDoc))
	if err != nil {
		t.Fatal(err)
	}
	if got := e.Levels(); !slices.Equal(got, []*Level{god, m, mem}) {
		t.Fatalf("Levels = %v", got)
	}

	// The result is a copy.
	got := e.Levels()
	got[0] = nil
	if e.Levels()[0] != god {
		t.Fatal("Levels returned the engine's own slice")
	}

	e.UnloadLevel(m)
	e.UnloadLevel(m)
	if got := e.Levels(); !slices.Equal(got, []*Level{god, mem}) {
		t.Fatalf("Levels after unloading the map = %v", got)
	}
	e.UnloadLevel(god)
	e.UnloadLevel(mem)
	if len(e.Levels()) != 0 {
		t.Fatalf("Levels after unloading all = %v", e.Levels())
	}
}

func TestLevelSetObjectsAndScope(t *testing.T) {
	e := New(Config{NoAudio: true, FS: mapFiles(t)})
	defer e.Close()
	l, err := e.LoadLevelData("mem.json", []byte(visualDoc))
	if err != nil {
		t.Fatal(err)
	}
	if l.Scope() == nil {
		t.Fatal("no scope")
	}
	w := e.World()
	old := l.Objects()
	added := god.Object{Entity: w.NewEntity()}
	l.SetObjects(append(slices.Clone(old[1:]), added))

	e.UnloadLevel(l)
	if !w.IsAlive(old[0].Entity) {
		t.Error("an object no longer listed was deleted")
	}
	if w.IsAlive(added.Entity) {
		t.Error("a listed object survived UnloadLevel")
	}
	if l.Scope() != nil {
		t.Error("Scope after UnloadLevel is not nil")
	}
}

func TestLevelName(t *testing.T) {
	e := New(Config{NoAudio: true, FS: prefabFiles(t), OnAssetError: func(error) {}})
	defer e.Close()
	const inst = `{"id":"a","prefab":"prefabs/box.prefab.json"}`
	for _, tc := range []struct{ name, doc, want string }{
		{"named", `{"name":"Forest","objects":[]}`, "Forest"},
		{"unnamed", `{"objects":[]}`, ""},
		{"named with prefabs", `{"name":"Cave","objects":[` + inst + `]}`, "Cave"},
		{"unnamed with prefabs", `{"objects":[` + inst + `]}`, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			l, err := e.LoadLevelData(tc.name+".json", []byte(tc.doc))
			if err != nil {
				t.Fatal(err)
			}
			if l.Name() != tc.want {
				t.Errorf("Name() = %q before spawn, want %q", l.Name(), tc.want)
			}
			tickUntil(t, e, l.Spawned, func() {})
			if l.Name() != tc.want {
				t.Errorf("Name() = %q after spawn, want %q", l.Name(), tc.want)
			}
			e.UnloadLevel(l)
			if l.Name() != "" {
				t.Errorf("Name() = %q after UnloadLevel", l.Name())
			}
		})
	}
}

// With Config.KeepAlive a level that is unloaded and loaded again reuses its
// art: the second load fires no EventLoaded.
func TestKeepAliveReloadsNothing(t *testing.T) {
	for _, tt := range []struct {
		name      string
		keepAlive int64
		wantLoads bool
	}{{"off", 0, true}, {"on", 1 << 20, false}} {
		t.Run(tt.name, func(t *testing.T) {
			e := New(Config{NoAudio: true, FS: mapFiles(t), KeepAlive: tt.keepAlive})
			defer e.Close()
			// Update until the map and its tileset are loaded; reports whether
			// any asset finished loading meanwhile.
			update := func() (loaded bool) {
				for range 100 {
					time.Sleep(time.Millisecond) // the decode workers run in parallel
					for ev := range e.Assets().Update().All() {
						loaded = loaded || ev.Kind == asset.EventLoaded
					}
				}
				return loaded
			}
			l, err := e.LoadMap("m.tmj", tilemap.SpawnOptions{})
			if err != nil {
				t.Fatal(err)
			}
			if !update() {
				t.Fatal("first load loaded nothing")
			}
			e.UnloadLevel(l)
			update()
			if _, err := e.LoadMap("m.tmj", tilemap.SpawnOptions{}); err != nil {
				t.Fatal(err)
			}
			loaded := update()
			if loaded != tt.wantLoads {
				t.Fatalf("EventLoaded on the second load = %v, want %v", loaded, tt.wantLoads)
			}
		})
	}
}
