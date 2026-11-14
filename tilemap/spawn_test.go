package tilemap

import (
	"testing"
	"testing/fstest"

	"github.com/weakpixel/ecs"
	"github.com/weakpixel/engine/gfx"
	testassets "github.com/weakpixel/engine/testdata/assets"
)

// spawned returns the TileLayer and Transform of each entity.
func spawned(t *testing.T, w *ecs.World, es []ecs.Entity) ([]TileLayer, []gfx.Transform) {
	t.Helper()
	var tls []TileLayer
	var trs []gfx.Transform
	for _, e := range es {
		tls = append(tls, *ecs.GetComp[TileLayer](w, e))
		trs = append(trs, *ecs.GetComp[gfx.Transform](w, e))
	}
	return tls, trs
}

func TestSpawn(t *testing.T) {
	srv := newServer(t, testassets.Lib())
	h, err := LoadMapSync(srv.Global(), "world-1/level-1.tmj")
	if err != nil {
		t.Fatal(err)
	}
	maps := MapStoreOf(srv)
	m := maps.Get(h)
	w := ecs.NewWorld()
	pos := gfx.Vec2{X: 10, Y: 20}
	es, err := Spawn(w, maps, h, SpawnOptions{Pos: pos, BaseLayer: 3})
	if err != nil {
		t.Fatal(err)
	}
	tls, trs := spawned(t, w, es)
	names := []string{"bg", "main", "entities", "front", "collider-tiles"}
	if len(tls) != len(names) {
		t.Fatalf("%d layers spawned, want %d", len(tls), len(names))
	}
	for i, tl := range tls {
		l := &m.Layers[tl.Index]
		if l.Name != names[i] || tl.Layer != 3+int16(i) || tl.Map != h {
			t.Errorf("entity %d: layer %q render layer %d", i, l.Name, tl.Layer)
		}
		if trs[i].Pos != pos || trs[i].PrevPos != pos {
			t.Errorf("entity %d: transform %+v", i, trs[i])
		}
	}
	if !tls[0].Hidden || !tls[3].Hidden || tls[1].Hidden {
		t.Errorf("hidden flags: %v %v %v", tls[0].Hidden, tls[3].Hidden, tls[1].Hidden)
	}
	// collider-tiles has opacity 0.2: white with alpha 51.
	if tls[4].Tint != gfx.RGBA(255, 255, 255, 51) || tls[1].Tint != 0 {
		t.Errorf("tints %#x %#x", uint32(tls[4].Tint), uint32(tls[1].Tint))
	}

	// LayerFor skips and renumbers.
	w = ecs.NewWorld()
	es, err = Spawn(w, maps, h, SpawnOptions{LayerFor: func(_ *Map, l *Layer, i int) (int16, bool) {
		if p, _ := l.Props.String("renderPass"); p == "debug" {
			return 0, false
		}
		return int16(100 - i), true
	}})
	if err != nil {
		t.Fatal(err)
	}
	tls, _ = spawned(t, w, es)
	if len(tls) != 4 || tls[0].Layer != 100 || tls[3].Layer != 97 {
		t.Errorf("LayerFor: %+v", tls)
	}
}

func TestSpawnNotReady(t *testing.T) {
	srv := newServer(t, fstest.MapFS{"bad.tmj": textFile(`{`)})
	maps := MapStoreOf(srv)
	w := ecs.NewWorld()
	if _, err := Spawn(w, maps, MapHandle{}, SpawnOptions{}); err == nil {
		t.Error("zero handle spawned")
	}
	h := LoadMap(srv.Global(), "bad.tmj")
	_, err := Spawn(w, maps, h, SpawnOptions{})
	mustContain(t, err, `tilemap: map "bad.tmj" is not ready (queued)`)
	LoadMapSync(srv.Global(), "bad.tmj")
	_, err = Spawn(w, maps, h, SpawnOptions{})
	mustContain(t, err, "tilemap: map bad.tmj failed", "invalid JSON")
	n := 0
	for range ecs.NewView1[TileLayer](w).Each() {
		n++
	}
	if n != 0 {
		t.Errorf("%d layers created", n)
	}
}

func TestSpawnWorld(t *testing.T) {
	srv := newServer(t, testassets.Lib())
	h, err := LoadWorldSync(srv.Global(), "world-1/world-1.world")
	if err != nil {
		t.Fatal(err)
	}
	w := ecs.NewWorld()
	es, err := SpawnWorld(w, WorldStoreOf(srv), MapStoreOf(srv), h, SpawnOptions{Pos: gfx.Vec2{X: 1, Y: 2}})
	if err != nil {
		t.Fatal(err)
	}
	tls, trs := spawned(t, w, es)
	if len(es) != 7 {
		t.Fatalf("%d entities, want 7 (5 of level-1, 2 of level-2)", len(es))
	}
	world := WorldStoreOf(srv).Get(h)
	if tls[0].Map != world.Maps[0].Map || tls[5].Map != world.Maps[1].Map {
		t.Error("entities are not grouped by map in world order")
	}
	if trs[0].Pos != (gfx.Vec2{X: -95, Y: 2}) || trs[5].Pos != (gfx.Vec2{X: 1, Y: 2}) {
		t.Errorf("positions %v %v", trs[0].Pos, trs[5].Pos)
	}
	if tls[5].Layer != 0 || tls[6].Layer != 1 {
		t.Error("each map's layers start at BaseLayer")
	}
}
