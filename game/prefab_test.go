package game

import (
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/weakpixel/asset"
	"github.com/weakpixel/ecs"
	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/god"
	"github.com/weakpixel/engine/prefab"
	"github.com/weakpixel/engine/render"
)

const boxPrefab = `{"objects":[{"group":"gfx","class":"Visual","components":{` +
	`"gfx:Transform":{"Pos":{"X":0,"Y":0},"PrevPos":{"X":0,"Y":0},"Rot":0,"Scale":{"X":1,"Y":1}},` +
	`"gfx:Sprite":{"ID":"t.png","Layer":2,"Origin":{"X":0,"Y":0},"FlipX":false,"FlipY":false,"Tint":0,"Blend":0,"Hidden":false}}}]}`

func prefabFiles(t *testing.T) fstest.MapFS {
	fsys := mapFiles(t)
	fsys["prefabs/box.prefab.json"] = &fstest.MapFile{Data: []byte(boxPrefab)}
	fsys["prefabs/bad.prefab.json"] = &fstest.MapFile{Data: []byte(`{"objects":[{"group":"gfx","class":"Nope"}]}`)}
	fsys["lvl.json"] = &fstest.MapFile{Data: []byte(`{"objects":[` +
		`{"id":"a","prefab":"prefabs/box.prefab.json","components":{"gfx:Transform":{"Pos":{"X":5}}}},` +
		`{"id":"b","prefab":"prefabs/box.prefab.json","components":{"gfx:Sprite":{"Layer":4}}}]}`)}
	fsys["bad-prefab.json"] = &fstest.MapFile{Data: []byte(`{"objects":[{"prefab":"prefabs/bad.prefab.json"}]}`)}
	fsys["bad-override.json"] = &fstest.MapFile{Data: []byte(`{"objects":[{"prefab":"prefabs/box.prefab.json","components":{"gfx:Sprite":{"Layer":"x"}}}]}`)}
	return fsys
}

// TestLevelWithPrefabs loads a level of prefab instances: it spawns once the
// prefab is ready, holds the prefab, and releases it when unloaded.
func TestLevelWithPrefabs(t *testing.T) {
	e := New(Config{NoAudio: true, FS: prefabFiles(t), OnAssetError: func(error) {}})
	defer e.Close()

	l, err := e.LoadLevel("lvl.json")
	if err != nil {
		t.Fatal(err)
	}
	if l.Spawned() || len(l.Objects()) != 0 {
		t.Fatal("a level with prefabs spawned before they loaded")
	}
	tickUntil(t, e, l.Spawned, func() {})

	objs := l.Objects()
	if len(objs) != 2 || objs[0].Prefab != "prefabs/box.prefab.json" || objs[1].ID != "b" {
		t.Fatalf("objects = %+v", objs)
	}
	w := e.World()
	if tr := ecs.GetComp[gfx.Transform](w, objs[0].Entity); tr.Pos != (gfx.Vec2{X: 5}) {
		t.Errorf("a: Pos = %v", tr.Pos)
	}
	if s := ecs.GetComp[gfx.Sprite](w, objs[1].Entity); s.Layer != 4 || s.ID.IsZero() {
		t.Errorf("b: Sprite = %+v", *s)
	}
	if s := ecs.GetComp[gfx.Sprite](w, objs[0].Entity); s.Layer != 2 {
		t.Errorf("a: Layer = %d", s.Layer)
	}

	// Game code spawns from the same prefab with no JSON.
	prefabs := e.Stores().Prefabs()
	h := prefab.Load(l.Scope(), "prefabs/box.prefab.json")
	at := gfx.Vec2{X: 7, Y: 8}
	spawned := prefabs.Spawn(h, gfx.Transform{Pos: at, PrevPos: at, Scale: gfx.Vec2{X: 1, Y: 1}})
	if key, _ := god.PrefabOf(w, spawned); key != "prefabs/box.prefab.json" {
		t.Errorf("PrefabOf(spawned) = %q", key)
	}
	tickUntil(t, e, func() bool { return e.Stores().Sprites().Ready(ecs.GetComp[gfx.Sprite](w, spawned).ID) }, func() {})

	e.UnloadLevel(l)
	for range 3 {
		if err := e.Update(); err != nil {
			t.Fatal(err)
		}
	}
	if w.IsAlive(objs[0].Entity) || w.IsAlive(objs[1].Entity) {
		t.Error("UnloadLevel left instances alive")
	}
	if n := prefabs.Generic().Len(); n != 0 {
		t.Errorf("%d prefabs loaded after UnloadLevel, want 0", n)
	}
	if n := asset.StoreOf[render.Texture](e.Assets()).Len(); n != 0 {
		t.Errorf("%d textures loaded after UnloadLevel, want 0", n)
	}
}

func TestLevelWithPrefabsErrors(t *testing.T) {
	var reported []error
	e := New(Config{NoAudio: true, FS: prefabFiles(t), OnAssetError: func(err error) { reported = append(reported, err) }})
	defer e.Close()

	for path, want := range map[string]string{
		"bad-prefab.json":   `game: LoadLevel "bad-prefab.json": objects not spawned: prefab prefabs/bad.prefab.json failed to load`,
		"bad-override.json": `game: LoadLevel "bad-override.json": objects not spawned: objects[0]: component gfx:Sprite: $.Layer: expected integer, got string`,
	} {
		l, err := e.LoadLevel(path)
		if err != nil {
			t.Fatal(err)
		}
		tickUntil(t, e, func() bool { return l.Err() != nil && !slices.Contains(e.docPending, l) }, func() {})
		if !strings.Contains(l.Err().Error(), want) {
			t.Errorf("%s: Err = %v, want %q", path, l.Err(), want)
		}
		if l.Spawned() || len(l.Objects()) != 0 {
			t.Errorf("%s: spawned anyway", path)
		}
		e.UnloadLevel(l)
	}
	if len(reported) == 0 {
		t.Error("nothing reported to OnAssetError")
	}

	if _, err := e.LoadLevelData("x", []byte(`{"objects":[{"prefab":"bad key#"}]}`)); err == nil || !strings.Contains(err.Error(), `game: LoadLevel "x": prefab "bad key#"`) {
		t.Errorf("bad key: %v", err)
	}
}

// TestLevelWithMultiObjectPrefab places a prefab of a root and a part: the
// part is placed in the spawn tick, children override it, a level object
// attached to the part follows it, and deleting the root removes the group.
func TestLevelWithMultiObjectPrefab(t *testing.T) {
	fsys := mapFiles(t)
	fsys["prefabs/knight.prefab.json"] = &fstest.MapFile{Data: []byte(hierDoc(visual("knight", "0", ""), visual("sword", "0", "knight")))}
	fsys["lvl.json"] = &fstest.MapFile{Data: []byte(`{"objects":[` +
		`{"id":"k1","prefab":"prefabs/knight.prefab.json","components":{"gfx:Transform":{"Pos":{"X":100,"Y":0}}},` +
		`"children":{"sword":{"hierarchy:Parent":{"Local":{"Pos":{"X":7}}}}}},` +
		visual("torch", "0", "k1/sword") + `]}`)}
	e := New(Config{NoAudio: true, FS: fsys, OnAssetError: func(error) {}})
	defer e.Close()
	l, err := e.LoadLevel("lvl.json")
	if err != nil {
		t.Fatal(err)
	}
	tickUntil(t, e, l.Spawned, func() {})

	w := e.World()
	byID := map[string]ecs.Entity{}
	for _, o := range l.Objects() {
		byID[o.ID] = o.Entity
	}
	if len(l.Objects()) != 3 || !w.IsAlive(byID["k1/sword"]) {
		t.Fatalf("objects = %+v", l.Objects())
	}
	for id, want := range map[string]float32{"k1": 100, "k1/sword": 107, "torch": 112} {
		if tr := ecs.GetComp[gfx.Transform](w, byID[id]); tr.Pos.X != want || tr.PrevPos.X != want {
			t.Errorf("%s: Pos/PrevPos = %v/%v, want %v", id, tr.Pos.X, tr.PrevPos.X, want)
		}
	}

	// Deleting the root deletes the part and, through it, the torch.
	w.DeleteEntity(byID["k1"])
	update(t, e, 2)
	if w.IsAlive(byID["k1/sword"]) || w.IsAlive(byID["torch"]) {
		t.Error("the part or the torch survived its root")
	}
	e.UnloadLevel(l) // the entities are gone already: must not panic
}
