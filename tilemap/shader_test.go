package tilemap

import (
	"fmt"
	"image"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/weakpixel/asset"
	"github.com/weakpixel/ecs"
	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/render"
)

// shaderEnv is an env whose plainMap layer "l" carries a shader and params
// (as properties, so Spawn copies them), next to an image layer without.
func shaderEnv(t *testing.T) *xenv {
	t.Helper()
	props := `,"properties":[{"name":"shader","type":"file","value":"fx/w.kage"},{"name":"shaderParams","type":"string","value":"2,3"}]`
	e := newXEnv(t, map[string]string{"m.tmj": plainMap("", props), "fx/w.kage": testKage})
	return e
}

func TestSpawnCopiesLayerShader(t *testing.T) {
	e := shaderEnv(t)
	es := e.spawn("m.tmj", SpawnOptions{})
	h, _ := LoadMapSync(e.srv.Global(), "m.tmj")
	m := MapStoreOf(e.srv).Get(h)
	tls, _ := spawned(t, e.w, es)
	if tls[0].Shader.IsZero() || tls[0].Shader != m.Layers[0].Shader || tls[0].Params != ([4]float32{2, 3}) {
		t.Fatalf("TileLayer = %+v, want the layer's shader and params", tls[0])
	}
}

func TestUnchunkedTileItemsCarryShader(t *testing.T) {
	e := shaderEnv(t)
	e.spawn("m.tmj", SpawnOptions{})
	e.look(gfx.Vec2{X: 8, Y: 8})
	e.run(0)
	items := e.items()
	if len(items) == 0 {
		t.Fatal("no items")
	}
	for i, it := range items {
		if it.Shader.IsZero() || it.Params != ([4]float32{2, 3}) {
			t.Errorf("item %d shader %v params %v", i, it.Shader, it.Params)
		}
	}
}

func TestChunkedItemsCarryShader(t *testing.T) {
	e := shaderEnv(t)
	e.spawn("m.tmj", SpawnOptions{Chunked: true})
	e.look(gfx.Vec2{X: 16, Y: 12})
	e.run(150 * time.Millisecond)
	items := e.items()
	if len(items) != 2 || items[0].Kind != render.KindCanvas {
		t.Fatalf("got %+v, want the chunk and then the animated tile", items)
	}
	for i, it := range items {
		if it.Shader.IsZero() || it.Params != ([4]float32{2, 3}) {
			t.Errorf("item %d (%v) shader %v params %v", i, it.Kind, it.Shader, it.Params)
		}
	}
}

func TestRuntimeShaderChange(t *testing.T) {
	e := newXEnv(t, map[string]string{"m.tmj": plainMap("", ""), "fx/w.kage": testKage})
	es := e.spawn("m.tmj", SpawnOptions{})
	e.look(gfx.Vec2{X: 8, Y: 8})
	e.run(0)
	for _, it := range e.items() {
		if !it.Shader.IsZero() || it.Params != ([4]float32{}) {
			t.Fatalf("layer without shader queued %+v", it)
		}
	}
	tl := tileLayerOf(e, es[0])
	tl.Shader, tl.Params = shaderHandle(t, e), [4]float32{1}
	e.run(0)
	for _, it := range e.items() {
		if it.Shader.IsZero() || it.Params[0] != 1 {
			t.Fatalf("after the field write the item is %+v", it)
		}
	}
}

func tileLayerOf(e *xenv, en ecs.Entity) *TileLayer { return ecs.GetComp[TileLayer](e.w, en) }

func shaderHandle(t *testing.T, e *xenv) gfx.ShaderHandle {
	t.Helper()
	h, err := render.LoadShaderSync(e.srv.Global(), "fx/w.kage")
	if err != nil {
		t.Fatal(err)
	}
	return h
}

// reloadMapShader replaces m.tmj and reloads it.
func reloadMapShader(e *xenv, files fstest.MapFS, props string) {
	files["m.tmj"] = textFile(plainMap("", props))
	e.srv.Changed("m.tmj")
	for range 10 {
		e.srv.Update()
	}
}

func TestLayerShaderFollowsMapReload(t *testing.T) {
	const props = `,"properties":[{"name":"shader","type":"file","value":"fx/w.kage"},{"name":"shaderParams","type":"string","value":"%s"}]`
	files := tilesetFiles(t)
	files["fx/w.kage"] = textFile(testKage)
	files["m.tmj"] = textFile(plainMap("", fmt.Sprintf(props, "2,3")))
	e := newXEnvFS(t, files, image.Pt(16, 16), 1)
	es := e.spawn("m.tmj", SpawnOptions{})
	e.look(gfx.Vec2{X: 8, Y: 8})
	e.run(0)
	tl := tileLayerOf(e, es[0])
	shader := tl.Shader

	// A new shaderParams reaches the spawned layer.
	reloadMapShader(e, files, fmt.Sprintf(props, "5,6"))
	e.run(0)
	if tl.Params != ([4]float32{5, 6}) || tl.Shader != shader {
		t.Fatalf("after reload: shader %v params %v, want %v and 5,6", tl.Shader, tl.Params, shader)
	}
	for _, it := range e.items() {
		if it.Params != ([4]float32{5, 6}) {
			t.Fatalf("item params %v, want 5,6", it.Params)
		}
	}

	// Params that game code wrote are kept; the shader (not written) follows.
	tl.Params = [4]float32{9}
	reloadMapShader(e, files, fmt.Sprintf(props, "7,8"))
	e.run(0)
	if tl.Params != ([4]float32{9}) {
		t.Fatalf("params = %v, want the game's 9", tl.Params)
	}

	// Removing the property from the map clears the shader of a layer that
	// still holds the map's.
	reloadMapShader(e, files, "")
	e.run(0)
	if !tl.Shader.IsZero() {
		t.Fatalf("shader = %v, want none", tl.Shader)
	}
}

func TestLayerShaderFollowsShaderFileReload(t *testing.T) {
	files := tilesetFiles(t)
	files["fx/w.kage"] = textFile(testKage)
	files["m.tmj"] = textFile(plainMap("", `,"properties":[{"name":"shader","type":"file","value":"fx/w.kage"}]`))
	e := newXEnvFS(t, files, image.Pt(16, 16), 1)
	es := e.spawn("m.tmj", SpawnOptions{})
	e.look(gfx.Vec2{X: 8, Y: 8})
	e.run(0)
	tl := tileLayerOf(e, es[0])
	shader := tl.Shader
	h, _ := LoadMapSync(e.srv.Global(), "m.tmj")
	maps := MapStoreOf(e.srv).Generic()
	before := maps.Version(asset.HandleOf[Map](h.Ref))
	shaders := render.ShaderStoreOf(e.srv)
	old := shaders.Get(shader).Shader

	// Editing the .kage swaps the compiled shader under the same handle.
	files["fx/w.kage"] = textFile(strings.Replace(testKage, "* color", "* color * 1.0", 1))
	e.srv.Changed("fx/w.kage")
	for range 10 {
		e.srv.Update()
	}
	e.run(0)
	if tl.Shader != shader || shaders.Get(shader).Shader == old || shaders.Get(shader).Shader == nil {
		t.Fatal("the layer's shader was not swapped in place")
	}
	if after := maps.Version(asset.HandleOf[Map](h.Ref)); after == before {
		t.Errorf("map version %d not bumped by its shader's reload", after)
	}
}
