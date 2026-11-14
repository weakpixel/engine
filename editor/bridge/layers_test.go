package bridge

import (
	"bytes"
	"image"
	"image/png"
	"testing"
	"testing/fstest"
	"time"

	"github.com/weakpixel/engine/tilemap"
)

// layerMap is a map with ground (render layer base), an object layer and
// trees (base+1).
const layerMap = `{"orientation":"orthogonal","width":4,"height":2,"tilewidth":8,"tileheight":8,
	"tilesets":[{"firstgid":1,"source":"t.tsj"}],
	"layers":[{"type":"tilelayer","name":"ground","width":4,"height":2,"data":[1,2,3,4,1,2,3,4]},
	          {"type":"objectgroup","name":"objs","objects":[]},
	          {"type":"tilelayer","name":"trees","width":4,"height":2,"data":[0,0,1,0,0,0,0,0]}]}`

// mapFixture is a bridge over a level with m.tmj loaded as its backdrop
// with opts, and the prefab box.prefab.json (sprite layer 1) available.
func mapFixture(t *testing.T, opts tilemap.SpawnOptions) *fixture {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 16, 16))); err != nil {
		t.Fatal(err)
	}
	f := newFixtureFiles(t, fstest.MapFS{
		"m.tmj":           {Data: []byte(layerMap)},
		"t.tsj":           {Data: []byte(`{"name":"t","image":"t.png","tilewidth":8,"tileheight":8,"columns":2,"tilecount":4}`)},
		"t.png":           {Data: buf.Bytes()},
		"box.prefab.json": {Data: []byte(boxPrefab)},
	})
	f.recv(documentMsg(visual("a", 0, 0)))
	l, err := f.e.LoadMap("m.tmj", opts)
	if err != nil {
		t.Fatal(err)
	}
	for deadline := time.Now().Add(10 * time.Second); !l.Spawned(); time.Sleep(time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatal("map never spawned")
		}
		f.recv(`{"type":"frame"}`)
	}
	f.recv(`{"type":"frame"}`)
	return f
}

func TestSelectReveal(t *testing.T) {
	f := newFixture(t)
	f.recv(documentMsg(visual("near", 0, 0), visual("far", 1000, 0)))
	f.recv(`{"type":"camera","x":0,"y":0,"zoom":1}`)
	f.recv(`{"type":"select","ids":["far"]}`)
	if f.e.Camera().Pos.X != 0 {
		t.Fatalf("select moved the camera to %v", f.e.Camera().Pos)
	}
	f.recv(`{"type":"select","ids":["far"],"reveal":true}`)
	if f.e.Camera().Pos.X != 1000 {
		t.Fatalf("select with reveal left the camera at %v", f.e.Camera().Pos)
	}
	f.recv(`{"type":"select","ids":["near"],"reveal":true}`)
	if f.e.Camera().Pos.X != 0 {
		t.Fatalf("reveal of an off-screen object left the camera at %v", f.e.Camera().Pos)
	}
}

func TestMapLayersMessage(t *testing.T) {
	f := mapFixture(t, tilemap.SpawnOptions{})
	msgs := f.host.take("mapLayers")
	if len(msgs) != 1 {
		t.Fatalf("%d mapLayers messages, want 1", len(msgs))
	}
	ls := msgs[0]["layers"].([]any)
	if len(ls) != 2 {
		t.Fatalf("layers = %v", ls)
	}
	l := ls[1].(map[string]any)
	if l["map"] != "m.tmj" || l["path"] != "trees" || l["name"] != "trees" || l["kind"] != "tiles" || l["layer"] != float64(1) || l["hidden"] != false {
		t.Fatalf("layers[1] = %v", l)
	}
	f.recv(`{"type":"frame"}`)
	if n := len(f.host.take("mapLayers")); n != 0 {
		t.Fatalf("%d more mapLayers messages while nothing changed", n)
	}

	f.recv(`{"type":"mapLayerHidden","map":"m.tmj","path":"trees","hidden":true}`)
	f.recv(`{"type":"frame"}`)
	msgs = f.host.take("mapLayers")
	if len(msgs) != 1 || msgs[0]["layers"].([]any)[1].(map[string]any)["hidden"] != true {
		t.Fatalf("after hiding: %v", msgs)
	}
	f.recv(`{"type":"mapLayerHidden","map":"m.tmj","path":"nope","hidden":true}`)
	if errs := f.host.take("error"); len(errs) != 1 {
		t.Fatalf("errors = %v, want one for the unknown layer", errs)
	}
}

// layerOf returns the Layer of component key in the created object.
func layerOf(t *testing.T, obj map[string]any, key string) (float64, bool) {
	t.Helper()
	c, ok := obj["components"].(map[string]any)[key].(map[string]any)
	if !ok {
		return 0, false
	}
	v, ok := c["Layer"].(float64)
	return v, ok
}

func TestDropDefaultLayer(t *testing.T) {
	f := mapFixture(t, tilemap.SpawnOptions{BaseLayer: 5})
	f.recv(`{"type":"drop","path":"box.png","x":160,"y":90,"class":"gfx:Visual"}`)
	created := f.host.take("create")
	if len(created) != 1 {
		t.Fatalf("create = %v (errors %v)", created, f.host.take("error"))
	}
	if layer, ok := layerOf(t, created[0]["object"].(map[string]any), "gfx:Sprite"); !ok || layer != 5 {
		t.Fatalf("dropped sprite Layer = %v, %v, want the ground's 5", layer, ok)
	}

	// LayerFor decides the render layers, so it decides the default.
	f = mapFixture(t, tilemap.SpawnOptions{LayerFor: func(m *tilemap.Map, l *tilemap.Layer, i int) (int16, bool) {
		return int16(40 - 10*i), true
	}})
	f.recv(`{"type":"drop","path":"box.png","x":160,"y":90,"class":"gfx:Visual"}`)
	created = f.host.take("create")
	if layer, _ := layerOf(t, created[0]["object"].(map[string]any), "gfx:Sprite"); layer != 40 {
		t.Fatalf("dropped sprite Layer = %v, want 40 (LayerFor's ground)", layer)
	}
}

func TestDropWithoutMapKeepsLayer(t *testing.T) {
	f := newFixture(t)
	f.recv(documentMsg(visual("a", 0, 0)))
	f.recv(`{"type":"drop","path":"box.png","x":160,"y":90,"class":"gfx:Visual"}`)
	created := f.host.take("create")
	if layer, ok := layerOf(t, created[0]["object"].(map[string]any), "gfx:Sprite"); !ok || layer != 0 {
		t.Fatalf("Layer = %v, %v, want the initial 0", layer, ok)
	}
}

func TestInstanceDefaultLayer(t *testing.T) {
	// The prefab's sprite is on layer 1: ground at 1 needs no override,
	// ground at 5 does.
	for _, tt := range []struct {
		base int16
		want string
	}{{1, ""}, {5, `{"Layer":5}`}} {
		f := mapFixture(t, tilemap.SpawnOptions{BaseLayer: tt.base})
		f.recv(`{"type":"drop","path":"box.prefab.json","x":200,"y":90}`)
		created := f.host.take("create")
		if len(created) != 1 {
			t.Fatalf("base %d: create = %v (errors %v)", tt.base, created, f.host.take("error"))
		}
		comps := created[0]["object"].(map[string]any)["components"].(map[string]any)
		sprite, has := comps["gfx:Sprite"].(map[string]any)
		if (tt.want == "") == has || (has && (len(sprite) != 1 || sprite["Layer"] != float64(tt.base))) {
			t.Fatalf("base %d: components = %v, want sprite override %q", tt.base, comps, tt.want)
		}
		if _, ok := comps["gfx:Transform"]; !ok {
			t.Fatalf("base %d: no Transform override in %v", tt.base, comps)
		}
	}
}
