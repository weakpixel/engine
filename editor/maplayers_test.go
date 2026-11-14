package editor

import (
	"bytes"
	"image"
	"image/png"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/tilemap"
)

// mapJSON has an image layer, a ground tile layer, an object layer and a
// trees tile layer (hidden in Tiled), in this order.
const mapJSON = `{"orientation":"orthogonal","width":4,"height":2,"tilewidth":8,"tileheight":8,
	"tilesets":[{"firstgid":1,"source":"t.tsj"}],
	"layers":[{"type":"imagelayer","name":"sky","image":"box.png"},
	          {"type":"tilelayer","name":"ground","width":4,"height":2,"data":[1,2,3,4,1,2,3,4]},
	          {"type":"objectgroup","name":"objs","objects":[]},
	          {"type":"tilelayer","name":"trees","visible":false,"width":4,"height":2,"data":[0,0,1,0,0,0,0,0]}]}`

// mapFixture is a session over a level with the map m.tmj (mapJSON) loaded
// as its backdrop with opts.
func mapFixture(t *testing.T, opts tilemap.SpawnOptions) *fixture {
	t.Helper()
	return mapFixtureJSON(t, mapJSON, opts)
}

// mapFixtureJSON is mapFixture with the map file m.tmj given.
func mapFixtureJSON(t *testing.T, mapText string, opts tilemap.SpawnOptions) *fixture {
	t.Helper()
	files := testFiles(t)
	img := image.NewRGBA(image.Rect(0, 0, 16, 16))
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	files["m.tmj"] = &fstest.MapFile{Data: []byte(mapText)}
	files["t.tsj"] = &fstest.MapFile{Data: []byte(`{"name":"t","image":"t.png","tilewidth":8,"tileheight":8,"columns":2,"tilecount":4}`)}
	files["t.png"] = &fstest.MapFile{Data: buf.Bytes()}
	f := newFixtureFS(t, Config{}, files)
	f.update() // attaches the session
	f.open(doc(visual("a", 0, 0, 0)))
	l, err := f.e.LoadMap("m.tmj", opts)
	if err != nil {
		t.Fatal(err)
	}
	for deadline := time.Now().Add(10 * time.Second); !l.Spawned(); time.Sleep(time.Millisecond) {
		if time.Now().After(deadline) {
			t.Fatal("map never spawned")
		}
		f.update()
	}
	f.update()
	return f
}

func layerNums(ls []MapLayer) (out []int16) {
	for _, l := range ls {
		out = append(out, l.Layer)
	}
	return out
}

func TestMapLayers(t *testing.T) {
	f := mapFixture(t, tilemap.SpawnOptions{})
	got := f.s.MapLayers()
	want := []MapLayer{
		{Map: "m.tmj", Path: "sky", Name: "sky", Kind: "image", Layer: 0},
		{Map: "m.tmj", Path: "ground", Name: "ground", Kind: "tiles", Layer: 1},
		{Map: "m.tmj", Path: "trees", Name: "trees", Kind: "tiles", Layer: 2, Hidden: true},
	}
	if !slices.Equal(got, want) {
		t.Fatalf("MapLayers = %+v, want %+v", got, want)
	}

	// Reported once, not every tick.
	var sent []LayersChanged
	for _, ev := range f.events {
		if lc, ok := ev.(LayersChanged); ok {
			sent = append(sent, lc)
		}
	}
	if len(sent) != 1 || !slices.Equal(sent[0].Layers, want) {
		t.Fatalf("LayersChanged events = %+v", sent)
	}
	f.update()
	f.update()
	if n := countLayerEvents(f.events); n != 1 {
		t.Fatalf("%d LayersChanged events after idle ticks, want 1", n)
	}
}

func countLayerEvents(evs []Event) (n int) {
	for _, ev := range evs {
		if _, ok := ev.(LayersChanged); ok {
			n++
		}
	}
	return n
}

func TestMapLayersLayerFor(t *testing.T) {
	f := mapFixture(t, tilemap.SpawnOptions{LayerFor: func(m *tilemap.Map, l *tilemap.Layer, i int) (int16, bool) {
		return int16(10 * (i + 1)), l.Name != "sky"
	}})
	got := f.s.MapLayers()
	if nums := layerNums(got); !slices.Equal(nums, []int16{20, 30}) || got[0].Name != "ground" {
		t.Fatalf("MapLayers = %+v", got)
	}
	if layer, ok := f.s.DefaultLayer(gfx.Vec2{}); !ok || layer != 20 {
		t.Fatalf("DefaultLayer = %d, %v, want 20 (the ground's own layer)", layer, ok)
	}
}

func TestSetMapLayerHidden(t *testing.T) {
	f := mapFixture(t, tilemap.SpawnOptions{})
	if err := f.s.SetMapLayerHidden("m.tmj", "ground", true); err != nil {
		t.Fatal(err)
	}
	f.update()
	if l := f.s.MapLayers()[1]; l.Name != "ground" || !l.Hidden {
		t.Fatalf("ground = %+v, want hidden", l)
	}
	var last LayersChanged
	for _, ev := range f.events {
		if lc, ok := ev.(LayersChanged); ok {
			last = lc
		}
	}
	if !last.Layers[1].Hidden || countLayerEvents(f.events) != 2 {
		t.Fatalf("hiding sent %d events, last %+v", countLayerEvents(f.events), last)
	}
	if err := f.s.SetMapLayerHidden("m.tmj", "ground", false); err != nil {
		t.Fatal(err)
	}
	if f.s.MapLayers()[1].Hidden {
		t.Fatal("ground still hidden")
	}
	for _, c := range [][2]string{{"m.tmj", "nope"}, {"other.tmj", "ground"}} {
		if err := f.s.SetMapLayerHidden(c[0], c[1], true); err == nil {
			t.Errorf("SetMapLayerHidden(%q, %q) = nil, want an error", c[0], c[1])
		}
	}
	f.s.Play()
	if err := f.s.SetMapLayerHidden("m.tmj", "ground", true); err == nil {
		t.Error("hiding in Play = nil, want an error")
	}
}

func TestDefaultLayer(t *testing.T) {
	f := mapFixture(t, tilemap.SpawnOptions{})
	// The object layer sits between ground (1) and trees (2).
	if layer, ok := f.s.DefaultLayer(gfx.Vec2{X: 4, Y: 4}); !ok || layer != 1 {
		t.Fatalf("DefaultLayer = %d, %v, want 1", layer, ok)
	}
}

// tmj builds a map file from layer kinds: "t" a tile layer, "o" an object
// layer.
func tmj(kinds string) string {
	var layers []string
	for i, k := range kinds {
		name := string(k) + string(rune('0'+i))
		if k == 't' {
			layers = append(layers, `{"type":"tilelayer","name":"`+name+`","width":4,"height":2,"data":[1,2,3,4,1,2,3,4]}`)
		} else {
			layers = append(layers, `{"type":"objectgroup","name":"`+name+`","objects":[]}`)
		}
	}
	return `{"orientation":"orthogonal","width":4,"height":2,"tilewidth":8,"tileheight":8,` +
		`"tilesets":[{"firstgid":1,"source":"t.tsj"}],"layers":[` + strings.Join(layers, ",") + `]}`
}

func TestDefaultLayerVariants(t *testing.T) {
	tests := []struct {
		name   string
		layers string
		opts   tilemap.SpawnOptions
		want   int16
		ok     bool
	}{
		{"topmost object layer wins", "tototo", tilemap.SpawnOptions{}, 2, true},
		{"nearest drawable below", "ttot", tilemap.SpawnOptions{}, 1, true},
		{"base layer", "tot", tilemap.SpawnOptions{BaseLayer: 5}, 5, true},
		{"none below gives the lowest", "ott", tilemap.SpawnOptions{BaseLayer: 5}, 5, true},
		{"no object layer", "tt", tilemap.SpawnOptions{}, 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := mapFixtureJSON(t, tmj(tt.layers), tt.opts)
			layer, ok := f.s.DefaultLayer(gfx.Vec2{X: 4, Y: 4})
			if layer != tt.want || ok != tt.ok {
				t.Fatalf("DefaultLayer = %d, %v, want %d, %v", layer, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestDefaultLayerWithoutMap(t *testing.T) {
	f := newFixture(t, Config{})
	f.update()
	f.open(doc(visual("a", 0, 0, 0)))
	if layer, ok := f.s.DefaultLayer(gfx.Vec2{}); ok {
		t.Fatalf("DefaultLayer = %d, true, want false without a backdrop", layer)
	}
}

func TestReveal(t *testing.T) {
	f := newFixture(t, Config{})
	f.update()
	f.open(doc(visual("near", 0, 0, 0), visual("far", 1000, 0, 0)))
	f.update()
	cam := f.e.Camera()

	f.s.Select("near")
	f.s.Reveal()
	if cam.Pos != (gfx.Vec2{}) {
		t.Fatalf("a visible object moved the camera to %v", cam.Pos)
	}
	f.s.Select()
	f.s.Reveal()
	if cam.Pos != (gfx.Vec2{}) {
		t.Fatalf("an empty selection moved the camera to %v", cam.Pos)
	}
	cam.Zoom = 2
	f.s.Select("far")
	f.s.Reveal()
	if cam.Pos.X != 1000 || cam.Zoom != 2 || cam.PrevPos != cam.Pos {
		t.Fatalf("camera = %+v, want centered on far with zoom 2", *cam)
	}
	// Now in view: a second reveal leaves it alone.
	cam.Pos = gfx.Vec2{X: 1010}
	f.s.Reveal()
	if cam.Pos.X != 1010 {
		t.Fatalf("a visible object moved the camera to %v", cam.Pos)
	}

	f.s.Play()
	cam.Pos = gfx.Vec2{}
	f.s.Reveal()
	if cam.Pos != (gfx.Vec2{}) {
		t.Fatal("Reveal moved the camera in Play")
	}
}
