package tilemap

import (
	"image"
	"testing"
	"time"

	"github.com/weakpixel/ecs"
	"github.com/weakpixel/ecs/system"
	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/render"
)

// gridEnv is an env with "grid.tmj", an n x n map of 16 px tiles with the
// given number of full layers (see largeMap), so a chunk is 256 px.
func gridEnv(t testing.TB, n, layers int, screen image.Point, zoom float32) *xenv {
	t.Helper()
	files := tilesetFiles(t)
	files["t16.tsj"] = textFile(`{"name":"t16","image":"t16.png","tilewidth":16,"tileheight":16,"columns":4,"tilecount":16}`)
	files["t16.png"] = pngFile(t, 64, 64)
	files["grid.tmj"] = textFile(largeMap(n, layers))
	return newXEnvFS(t, files, screen, zoom)
}

// count returns how many queued items are canvases and how many sprites.
func (e *xenv) count() (canvases, sprites int) {
	for _, it := range e.items() {
		switch it.Kind {
		case render.KindCanvas:
			canvases++
		case render.KindSprite:
			sprites++
		}
	}
	return canvases, sprites
}

func TestChunkedQueuesCanvases(t *testing.T) {
	// 40x40 cells: chunks of 16 cells, the last ones 8 cells wide/high.
	e := gridEnv(t, 40, 2, image.Pt(64, 64), 1)
	es := e.spawn("grid.tmj", SpawnOptions{Chunked: true, BaseLayer: 3})
	ecs.GetComp[TileLayer](e.w, es[1]).Tint = gfx.RGBA(255, 0, 0, 128)
	// The view [224, 288) holds cells 14..17 of both axes: chunks 0 and 1.
	e.look(gfx.Vec2{X: 256, Y: 256})
	e.cam.PrevPos = gfx.Vec2{X: 250, Y: 256} // the layers move left by 6
	e.run(time.Millisecond)

	items := e.items()
	if len(items) != 8 {
		t.Fatalf("queued %d items, want 2 layers x 4 chunks", len(items))
	}
	// Render order right-down: per layer, chunk rows top to bottom.
	tops := []gfx.Vec2{{X: 0, Y: 0}, {X: 256, Y: 0}, {X: 0, Y: 256}, {X: 256, Y: 256}}
	for i, it := range items {
		top := tops[i%4]
		if it.Kind != render.KindCanvas || it.Canvas.IsZero() {
			t.Fatalf("item %d is not a canvas: %+v", i, it)
		}
		if it.Pos != top || it.PrevPos != top {
			t.Errorf("item %d at %v (prev %v), want %v", i, it.Pos, it.PrevPos, top)
		}
		if want := int16(3 + i/4); it.Layer != want {
			t.Errorf("item %d layer %d, want %d", i, it.Layer, want)
		}
		if want := top.Y + 256; it.SortY != want {
			t.Errorf("item %d SortY %v, want %v", i, it.SortY, want)
		}
		size := e.canvases.Size(it.Canvas)
		if size != image.Pt(256, 256) {
			t.Errorf("item %d canvas is %v, want 256x256", i, size)
		}
	}
	if items[4].Color != gfx.RGBA(255, 0, 0, 128) || items[0].Color != 0 {
		t.Errorf("tints %v, %v; want the layer's", items[0].Color, items[4].Color)
	}
	if n := e.canvases.Len(); n != 8 {
		t.Errorf("canvases = %d, want 8", n)
	}
	// The next tick reuses the chunks.
	first := items[0].Canvas
	e.run(time.Millisecond)
	if e.items()[0].Canvas != first || e.canvases.Len() != 8 {
		t.Error("chunks were baked again")
	}
	// The edge chunks are 8 cells.
	e.look(gfx.Vec2{X: 600, Y: 600})
	e.run(time.Millisecond)
	if size := e.canvases.Size(e.items()[0].Canvas); size != image.Pt(128, 128) {
		t.Errorf("edge chunk canvas is %v, want 128x128", size)
	}
}

func TestChunkedAnimatedTilesStayItems(t *testing.T) {
	// plainMap's cell (2, 2) holds the animated tile.
	e := newXEnv(t, map[string]string{"m.tmj": plainMap("", "")})
	e.spawn("m.tmj", SpawnOptions{})
	e.look(gfx.Vec2{X: 16, Y: 12})
	e.run(150 * time.Millisecond)
	var want render.Item
	for _, it := range e.items() {
		if it.Sprite == e.sprite(1) { // the animation's second frame
			want = it
		}
	}

	e = newXEnv(t, map[string]string{"m.tmj": plainMap("", "")})
	e.spawn("m.tmj", SpawnOptions{Chunked: true})
	e.look(gfx.Vec2{X: 16, Y: 12})
	e.run(150 * time.Millisecond)
	items := e.items()
	if len(items) != 2 || items[0].Kind != render.KindCanvas {
		t.Fatalf("got %+v, want the chunk and then the animated tile", items)
	}
	if items[1] != want {
		t.Errorf("animated tile %+v, want %+v as without chunks", items[1], want)
	}
	if size := e.canvases.Size(items[0].Canvas); size != image.Pt(32, 24) {
		t.Errorf("canvas %v, want the 4x3 cells of 8 px", size)
	}
}

func TestChunkedFallsBackToTiles(t *testing.T) {
	tests := []struct {
		name  string
		setup func(e *xenv)
	}{
		{"y-sorted render layer", func(e *xenv) {
			cfg := &render.LayerConfig{}
			cfg.Set(0, render.LayerOptions{YSort: true})
			ecs.SetResource(e.w, cfg)
		}},
		{"no canvases", func(e *xenv) { ecs.RemoveResource[render.Canvases](e.w) }},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			e := gridEnv(t, 40, 1, image.Pt(64, 64), 1)
			tc.setup(e)
			e.ex.Init(&system.Context{World: e.w})
			e.spawn("grid.tmj", SpawnOptions{Chunked: true})
			e.look(gfx.Vec2{X: 256, Y: 256})
			e.run(time.Millisecond)
			if c, s := e.count(); c != 0 || s != 16 {
				t.Errorf("queued %d canvases and %d tiles, want 0 and 4x4", c, s)
			}
		})
	}
}

func TestChunkedBakeBudget(t *testing.T) {
	// The whole 40x40 map is visible: 3x3 chunks on each of 4 layers.
	e := gridEnv(t, 40, 4, image.Pt(160, 160), 0.25)
	e.spawn("grid.tmj", SpawnOptions{Chunked: true})
	e.look(gfx.Vec2{X: 320, Y: 320})
	for tick, want := range []int{8, 16, 24, 32, 36, 36} {
		e.run(time.Millisecond)
		canvases, sprites := e.count()
		if canvases != want || e.canvases.Len() != want {
			t.Fatalf("tick %d: %d canvas items, %d canvases; want %d", tick, canvases, e.canvases.Len(), want)
		}
		// Chunks that are not baked yet draw tile by tile: 256 tiles for a
		// full chunk, 128 for an edge one, 64 for the corner.
		if canvases == 36 && sprites != 0 {
			t.Fatalf("tick %d: %d tiles next to all chunks", tick, sprites)
		}
		if canvases < 36 && sprites == 0 {
			t.Fatalf("tick %d: the chunks over budget drew nothing", tick)
		}
	}
	if c, s := e.count(); c+s == 0 || s != 0 {
		t.Fatal("not all chunks baked")
	}
}

func TestChunksFreedWhenUnused(t *testing.T) {
	e := gridEnv(t, 40, 1, image.Pt(64, 64), 1)
	e.spawn("grid.tmj", SpawnOptions{Chunked: true})
	e.look(gfx.Vec2{X: 256, Y: 256})
	e.run(time.Millisecond)
	if n := e.canvases.Len(); n != 4 {
		t.Fatalf("canvases = %d, want 4", n)
	}
	// Away from the map, the chunks stay for chunkKeep ticks.
	e.look(gfx.Vec2{X: -1000, Y: -1000})
	for range chunkKeep {
		e.run(time.Millisecond)
	}
	if n := e.canvases.Len(); n != 4 {
		t.Fatalf("canvases = %d after %d ticks, want 4", n, chunkKeep)
	}
	e.run(time.Millisecond)
	if n := e.canvases.Len(); n != 0 {
		t.Fatalf("canvases = %d after %d ticks, want 0", n, chunkKeep+1)
	}
	for range chunkKeep {
		e.run(time.Millisecond)
	}
	if n := len(e.ex.chunks.layers); n != 0 {
		t.Errorf("%d layers cached, want 0", n)
	}
	// Coming back bakes them again.
	e.look(gfx.Vec2{X: 256, Y: 256})
	e.run(time.Millisecond)
	if c, _ := e.count(); c != 4 || e.canvases.Len() != 4 {
		t.Errorf("%d canvas items, %d canvases after coming back, want 4", c, e.canvases.Len())
	}
}

func TestChunksFollowMapReload(t *testing.T) {
	files := tilesetFiles(t)
	files["m.tmj"] = textFile(plainMap("", ""))
	e := newXEnvFS(t, files, image.Pt(16, 16), 1)
	e.spawn("m.tmj", SpawnOptions{Chunked: true})
	e.look(gfx.Vec2{X: 16, Y: 12})
	e.run(time.Millisecond)
	before := e.items()[0].Canvas

	// The reloaded map is 2 cells wider.
	files["m.tmj"] = textFile(`{"orientation":"orthogonal","width":6,"height":1,"tilewidth":8,"tileheight":8,
		"tilesets":[{"firstgid":1,"source":"t.tsj"}],
		"layers":[{"type":"tilelayer","name":"l","width":6,"height":1,"data":[2,2,2,2,2,2]}]}`)
	e.srv.Changed("m.tmj")
	for range 10 {
		e.srv.Update()
	}
	e.run(time.Millisecond)
	items := e.items()
	if len(items) != 1 || items[0].Canvas == before {
		t.Fatalf("got %+v, want one new canvas", items)
	}
	if size := e.canvases.Size(items[0].Canvas); size != image.Pt(48, 8) {
		t.Errorf("canvas %v, want 48x8", size)
	}
	if e.canvases.Size(before) != (image.Point{}) || e.canvases.Len() != 1 {
		t.Errorf("the old chunk was not freed (%d canvases)", e.canvases.Len())
	}
}

func TestChunkedNoAllocs(t *testing.T) {
	if testing.CoverMode() != "" || raceEnabled {
		t.Skip("instrumentation allocates")
	}
	e := gridEnv(t, 40, 2, image.Pt(64, 64), 1)
	e.spawn("grid.tmj", SpawnOptions{Chunked: true})
	e.look(gfx.Vec2{X: 256, Y: 256})
	e.warmUp()
	if n := testing.AllocsPerRun(50, func() { e.run(16 * time.Millisecond) }); n != 0 {
		t.Fatalf("allocs per run = %v, want 0", n)
	}
	if c, _ := e.count(); c == 0 {
		t.Fatal("no chunk queued")
	}
}
