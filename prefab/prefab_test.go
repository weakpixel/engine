package prefab_test

import (
	"fmt"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/weakpixel/asset"
	"github.com/weakpixel/ecs"
	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/god"
	"github.com/weakpixel/engine/hierarchy"
	"github.com/weakpixel/engine/prefab"
)

// blob stands in for a sprite: any file, loaded as its text.
type blob struct{ text string }

type blobLoader struct{}

func (blobLoader) Decode(_ *asset.DecodeContext, data []byte) (string, error) {
	return string(data), nil
}
func (blobLoader) Finish(_ *asset.FinishContext, s string) (blob, error) { return blob{s}, nil }
func (blobLoader) Unload(*blob)                                          {}

// blobResolver resolves "sprite" keys to blob handles loaded into sc.
type blobResolver struct {
	srv *asset.Server
	sc  *asset.Scope
}

func (r blobResolver) ResolveAsset(kind, path string) (any, error) {
	if kind != "sprite" {
		return nil, fmt.Errorf("unknown kind %q", kind)
	}
	return gfx.SpriteHandle{Ref: asset.Load[blob](r.sc, path).Ref()}, nil
}

func (r blobResolver) AssetKey(kind string, h any) (string, error) {
	return asset.StoreOf[blob](r.srv).Key(asset.HandleOf[blob](h.(gfx.SpriteHandle).Ref)).String(), nil
}

const (
	goblin = `{"objects":[{"group":"gfx","class":"Visual","components":{
		"gfx:Transform":{"Pos":{"X":0,"Y":0},"PrevPos":{"X":0,"Y":0},"Rot":0,"Scale":{"X":1,"Y":1}},
		"gfx:Sprite":{"ID":"art/goblin.png","Layer":3,"Origin":{"X":0,"Y":0},"FlipX":false,"FlipY":false,"Tint":4294967295,"Blend":0,"Hidden":false}}}]}`
	archer = `{"objects":[{"prefab":"prefabs/goblin.prefab.json","components":{"gfx:Sprite":{"ID":"art/archer.png"}}}]}`
)

type fixture struct {
	files fstest.MapFS
	srv   *asset.Server
	world *ecs.World
	store prefab.Store
}

func newFixture(t *testing.T) *fixture { return newFixtureWatch(t, time.Hour) }

func newFixtureWatch(t *testing.T, watch time.Duration) *fixture {
	t.Helper()
	f := &fixture{
		files: fstest.MapFS{
			"prefabs/goblin.prefab.json": {Data: []byte(goblin)},
			"prefabs/archer.prefab.json": {Data: []byte(archer)},
			"prefabs/bad.prefab.json":    {Data: []byte(`{"objects":[{"group":"gfx","class":"Visual","components":{}}]}`)},
			"prefabs/loop-a.prefab.json": {Data: []byte(`{"objects":[{"prefab":"prefabs/loop-b.prefab.json"}]}`)},
			"prefabs/loop-b.prefab.json": {Data: []byte(`{"objects":[{"prefab":"prefabs/loop-a.prefab.json"}]}`)},
			"deep/er/archer.prefab.json": {Data: []byte(archer)},
			"prefabs/knight.prefab.json": {Data: []byte(knight)},
			"prefabs/horse.prefab.json":  {Data: []byte(horse(3))},
			"prefabs/rider.prefab.json":  {Data: []byte(rider)},
			"prefabs/loose.prefab.json":  {Data: []byte(loose)},
			"art/goblin.png":             {Data: []byte("goblin")},
			"art/archer.png":             {Data: []byte("archer")},
		},
		world: ecs.NewWorld(),
	}
	// WatchInterval turns on re-finishing dependents (a variant whose base
	// changed); the test reports changes with Changed, so polling never runs.
	f.srv = asset.NewServer(asset.Options{FS: f.files, OnError: func(error) {}, WatchInterval: watch})
	t.Cleanup(f.srv.Close)
	asset.Register[blob, string](f.srv, blobLoader{}, blob{})
	prefab.RegisterLoader(f.srv, prefab.Options{
		World:  f.world,
		Assets: func(sc *asset.Scope) god.AssetResolver { return blobResolver{f.srv, sc} },
	})
	f.store = prefab.StoreOf(f.srv)
	return f
}

func (f *fixture) load(t *testing.T, sc *asset.Scope, key string) prefab.Handle {
	t.Helper()
	h, err := prefab.LoadSync(sc, key)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

// settle runs Update until the art has loaded.
func (f *fixture) settle() {
	for range 20 {
		f.srv.Update()
	}
}

func (f *fixture) spriteText(e ecs.Entity) string {
	s := ecs.GetComp[gfx.Sprite](f.world, e)
	return asset.StoreOf[blob](f.srv).Get(asset.HandleOf[blob](s.ID.Ref)).text
}

func TestSpawn(t *testing.T) {
	f := newFixture(t)
	h := f.load(t, f.srv.Global(), "prefabs/goblin.prefab.json")
	f.settle()

	at := gfx.Vec2{X: 10, Y: 20}
	e := f.store.Spawn(h, gfx.Transform{Pos: at, PrevPos: at, Scale: gfx.Vec2{X: 1, Y: 1}})
	if tr := ecs.GetComp[gfx.Transform](f.world, e); tr.Pos != at {
		t.Errorf("Pos = %v", tr.Pos)
	}
	if s := ecs.GetComp[gfx.Sprite](f.world, e); s.Layer != 3 || f.spriteText(e) != "goblin" {
		t.Errorf("Sprite = %+v (%q)", *s, f.spriteText(e))
	}
	if key, _ := god.PrefabOf(f.world, e); key != "prefabs/goblin.prefab.json" {
		t.Errorf("PrefabOf = %q", key)
	}
	if meta, _ := god.ClassOf(f.world, e); meta.Key() != "gfx:Visual" {
		t.Errorf("ClassOf = %v", meta)
	}
	if p := f.store.Get(h).Types(); p.Key() != "prefabs/goblin.prefab.json" || p.Meta().Key() != "gfx:Visual" {
		t.Errorf("Types() = %v %v", p.Key(), p.Meta())
	}
}

func TestVariant(t *testing.T) {
	f := newFixture(t)
	for _, key := range []string{"prefabs/archer.prefab.json", "deep/er/archer.prefab.json"} {
		h := f.load(t, f.srv.Global(), key)
		f.settle()
		e := f.store.Spawn(h)
		if s := ecs.GetComp[gfx.Sprite](f.world, e); s.Layer != 3 || f.spriteText(e) != "archer" {
			t.Errorf("%s: Sprite = %+v (%q)", key, *s, f.spriteText(e))
		}
		if base := f.store.Get(h).Types().Base(); base != "prefabs/goblin.prefab.json" {
			t.Errorf("%s: Base = %q", key, base)
		}
	}
}

func TestLoadErrors(t *testing.T) {
	f := newFixture(t)
	for key, want := range map[string]string{
		"prefabs/bad.prefab.json":    "missing required component",
		"prefabs/loop-a.prefab.json": "cycle",
		"prefabs/nope.prefab.json":   "file does not exist",
	} {
		h, err := prefab.LoadSync(f.srv.Global(), key)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%s: got %v, want %q", key, err, want)
		}
		func() {
			defer func() {
				r := recover()
				if msg, _ := r.(string); !strings.Contains(msg, "prefab: Spawn: "+key+" is not ready (failed)") {
					t.Errorf("%s: Spawn panic = %v", key, r)
				}
			}()
			f.store.Spawn(h)
		}()
	}
}

// TestLifetime checks that the prefab holds its art and releases it with
// itself.
func TestLifetime(t *testing.T) {
	f := newFixture(t)
	sc := f.srv.NewScope("level")
	f.load(t, sc, "prefabs/archer.prefab.json")
	f.settle()
	blobs := asset.StoreOf[blob](f.srv)
	if n := blobs.Len(); n != 2 { // the archer's art and the goblin's (its base)
		t.Fatalf("%d blobs loaded, want 2", n)
	}
	sc.Release()
	f.settle()
	if n := blobs.Len(); n != 0 {
		t.Fatalf("%d blobs left after releasing the prefab, want 0", n)
	}
	if n := f.store.Generic().Len(); n != 0 {
		t.Fatalf("%d prefabs left, want 0", n)
	}
}

// TestReload checks that a changed base prefab changes the next spawn of
// its variant.
func TestReload(t *testing.T) {
	f := newFixture(t)
	h := f.load(t, f.srv.Global(), "prefabs/archer.prefab.json")
	f.settle()
	f.files["prefabs/goblin.prefab.json"] = &fstest.MapFile{Data: []byte(strings.Replace(goblin, `"Layer":3`, `"Layer":7`, 1))}
	f.srv.Changed("prefabs/goblin.prefab.json")
	f.settle()
	if s := ecs.GetComp[gfx.Sprite](f.world, f.store.Spawn(h)); s.Layer != 7 {
		t.Fatalf("Layer after reload = %d, want 7", s.Layer)
	}
}

// TestChangedBaseWithoutWatch checks that a variant follows its base when
// the change is reported with Changed and the server does not watch files
// (an editor build): it is reloaded from its file.
func TestChangedBaseWithoutWatch(t *testing.T) {
	f := newFixtureWatch(t, 0)
	h := f.load(t, f.srv.Global(), "prefabs/archer.prefab.json")
	f.settle()
	f.files["prefabs/goblin.prefab.json"] = &fstest.MapFile{Data: []byte(strings.Replace(goblin, `"Layer":3`, `"Layer":7`, 1))}
	f.srv.Changed("prefabs/goblin.prefab.json")
	f.settle()
	if s := ecs.GetComp[gfx.Sprite](f.world, f.store.Spawn(h)); s.Layer != 7 {
		t.Fatalf("Layer after Changed = %d, want 7", s.Layer)
	}
}

func TestSource(t *testing.T) {
	f := newFixture(t)
	sc := f.srv.NewScope("level")
	god.SetPrefabSource(f.world, prefab.Source(f.srv, func() *asset.Scope { return sc }))
	god.SetAssetResolver(f.world, blobResolver{f.srv, sc})
	doc := `{"objects":[{"id":"a","prefab":"prefabs/archer.prefab.json","components":{"gfx:Sprite":{"Layer":5}}}]}`

	_, err := god.Deserialize(f.world, strings.NewReader(doc))
	if err == nil || !strings.Contains(err.Error(), `prefab "prefabs/archer.prefab.json": not loaded yet`) {
		t.Fatalf("before loading: %v", err)
	}
	f.settle() // the source loaded it into sc
	objs, err := god.Deserialize(f.world, strings.NewReader(doc))
	if err != nil {
		t.Fatal(err)
	}
	f.settle()
	if s := ecs.GetComp[gfx.Sprite](f.world, objs[0].Entity); s.Layer != 5 || f.spriteText(objs[0].Entity) != "archer" {
		t.Fatalf("instance Sprite = %+v (%q)", *s, f.spriteText(objs[0].Entity))
	}
	var out strings.Builder
	if err := god.Serialize(f.world, &out, objs...); err != nil {
		t.Fatal(err)
	}
	if out.String() != `{"objects":[{"id":"a","prefab":"prefabs/archer.prefab.json","components":{"gfx:Sprite":{"Layer":5}}}]}` {
		t.Fatalf("Serialize = %s", out.String())
	}

	_, err = god.Deserialize(f.world, strings.NewReader(`{"objects":[{"prefab":"bad key#"}]}`))
	if err == nil || !strings.Contains(err.Error(), `prefab "bad key#"`) {
		t.Fatalf("bad key: %v", err)
	}
}

// visual is a gfx:Visual object with a parent, unless parent is "".
func visual(id, art, parent string, layer int) string {
	parentComp := ""
	if parent != "" {
		parentComp = `,"hierarchy:Parent":{"Entity":"` + parent + `","Local":{"Pos":{"X":8,"Y":-4},"Rot":0,"Scale":{"X":1,"Y":1},"LayerOffset":1,"LayerMode":0,"YSortSide":0}}`
	}
	return fmt.Sprintf(`{"id":%q,"group":"gfx","class":"Visual","components":{
		"gfx:Transform":{"Pos":{"X":0,"Y":0},"PrevPos":{"X":0,"Y":0},"Rot":0,"Scale":{"X":1,"Y":1}},
		"gfx:Sprite":{"ID":%q,"Layer":%d,"Origin":{"X":0,"Y":0},"FlipX":false,"FlipY":false,"Tint":4294967295,"Blend":0,"Hidden":false}%s}}`, id, art, layer, parentComp)
}

var (
	knight = `{"objects":[` + visual("knight", "art/goblin.png", "", 3) + `,` + visual("sword", "art/archer.png", "knight", 3) + `,` + visual("glint", "art/goblin.png", "sword", 3) + `]}`
	rider  = `{"objects":[` + visual("rider", "art/goblin.png", "", 3) + `,{"id":"horse","prefab":"prefabs/horse.prefab.json","components":{"hierarchy:Parent":{"Entity":"rider","Local":{"Pos":{"X":0,"Y":4},"Rot":0,"Scale":{"X":1,"Y":1},"LayerOffset":0,"LayerMode":0,"YSortSide":0}}}}]}`
	loose  = `{"objects":[` + visual("knight", "art/goblin.png", "", 3) + `,` + visual("sword", "art/archer.png", "knight", 3) + `,` + visual("torch", "art/goblin.png", "", 3) + `]}`
)

// horse is a two-object prefab (a horse and its saddle) whose root has the
// given draw layer.
func horse(layer int) string {
	return `{"objects":[` + visual("horse", "art/goblin.png", "", layer) + `,` + visual("saddle", "art/archer.png", "horse", 3) + `]}`
}

var _ = hierarchy.Parent{} // registers hierarchy:Parent

func TestSpawnAll(t *testing.T) {
	f := newFixture(t)
	h := f.load(t, f.srv.Global(), "prefabs/knight.prefab.json")
	f.settle()
	ents := f.store.SpawnAll(h, nil)
	if len(ents) != 3 {
		t.Fatalf("SpawnAll returned %d entities, want 3", len(ents))
	}
	if root := f.store.Spawn(h); ecs.GetCompOrNil[hierarchy.Parent](f.world, root) != nil {
		t.Error("Spawn did not return the root")
	}
	if ecs.GetCompOrNil[hierarchy.Parent](f.world, ents[0]) != nil {
		t.Error("ents[0] is not the root")
	}
	sword, glint := ecs.GetComp[hierarchy.Parent](f.world, ents[1]), ecs.GetComp[hierarchy.Parent](f.world, ents[2])
	if sword.Entity != ents[0] || glint.Entity != ents[1] {
		t.Errorf("parents: sword %v glint %v, ents %v", sword.Entity, glint.Entity, ents)
	}
	if f.spriteText(ents[1]) != "archer" {
		t.Errorf("sword art = %q", f.spriteText(ents[1]))
	}
	parts := f.store.Get(h).Types().Parts()
	if key, part, ok := god.PrefabPartOf(f.world, ents[2]); !ok || key != "prefabs/knight.prefab.json" || part != parts[2] {
		t.Errorf("PrefabPartOf = %q %q %v", key, part, ok)
	}
}

func TestNestedPrefab(t *testing.T) {
	f := newFixture(t)
	h := f.load(t, f.srv.Global(), "prefabs/rider.prefab.json")
	f.settle()
	p := f.store.Get(h).Types()
	if got := strings.Join(p.Parts(), ","); got != "rider,horse,horse/saddle" {
		t.Fatalf("Parts = %s", got)
	}
	ents := f.store.SpawnAll(h, nil)
	if len(ents) != 3 || ecs.GetComp[hierarchy.Parent](f.world, ents[2]).Entity != ents[1] || ecs.GetComp[hierarchy.Parent](f.world, ents[1]).Entity != ents[0] {
		t.Fatalf("nested group not linked: %v", ents)
	}
	// The nested prefab is a dependency: a change to it reloads the rider.
	f.files["prefabs/horse.prefab.json"] = &fstest.MapFile{Data: []byte(horse(9))}
	f.srv.Changed("prefabs/horse.prefab.json")
	f.settle()
	ents = f.store.SpawnAll(h, nil)
	if s := ecs.GetComp[gfx.Sprite](f.world, ents[1]); s.Layer != 9 {
		t.Fatalf("horse Layer after reload = %d, want 9", s.Layer)
	}
}

func TestUnattachedPart(t *testing.T) {
	f := newFixture(t)
	_, err := prefab.LoadSync(f.srv.Global(), "prefabs/loose.prefab.json")
	if err == nil || !strings.Contains(err.Error(), `prefab "prefabs/loose.prefab.json": object "torch" is not attached to the root (hierarchy:Parent)`) {
		t.Fatalf("got %v", err)
	}
}
