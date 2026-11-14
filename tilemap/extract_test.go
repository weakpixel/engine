package tilemap

import (
	"image"
	"math"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/weakpixel/asset"
	"github.com/weakpixel/ecs"
	"github.com/weakpixel/ecs/system"
	"github.com/weakpixel/engine/extract"
	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/render"
	"github.com/weakpixel/engine/tiled"
)

// xenv is a World with the resources TilemapExtract reads and a server with
// fixture maps. The screen is 16x16, the camera zoom 1.
type xenv struct {
	t     testing.TB
	srv   *asset.Server
	w     *ecs.World
	queue *render.Queue
	cam   *render.Camera
	ex    *TilemapExtract
	// canvases holds the chunk images of Chunked layers.
	canvases *render.Canvases
}

// tilesetFiles are "t.tsj", four 8x8 tiles in a 2x2 grid (local 0 animates
// between local 0 and 1, 100 ms each), and "tall.tsj", one 8x24 tile.
func tilesetFiles(t testing.TB) fstest.MapFS {
	return fstest.MapFS{
		"t.tsj": textFile(`{"name":"t","image":"t.png","tilewidth":8,"tileheight":8,"columns":2,"tilecount":4,
			"tiles":[{"id":0,"animation":[{"tileid":0,"duration":100},{"tileid":1,"duration":100}]}]}`),
		"t.png":    pngFile(t, 16, 16),
		"tall.tsj": textFile(`{"name":"tall","image":"tall.png","tilewidth":8,"tileheight":24,"columns":1,"tilecount":1}`),
		"tall.png": pngFile(t, 8, 24),
		"bg.png":   pngFile(t, 20, 10),
	}
}

// plainMap is a 4x3 map of t.tsj with flipped cells; extra fields go into
// the map, layer fields into its one tile layer.
//
//	row 0: 2  3|H  0    4
//	row 1: 3  2    2|D  0
//	row 2: 4  0    1    4
func plainMap(mapFields, layerFields string) string {
	return `{"orientation":"orthogonal","width":4,"height":3,"tilewidth":8,"tileheight":8` + mapFields + `,
		"tilesets":[{"firstgid":1,"source":"t.tsj"}],
		"layers":[` + plainLayer(layerFields) + `]}`
}

// plainLayer is plainMap's tile layer, with extra fields.
func plainLayer(fields string) string {
	h, d := tiled.JoinGID(3, tiled.FlipH), tiled.JoinGID(2, tiled.FlipD)
	data := []uint32{2, h, 0, 4, 3, 2, d, 0, 4, 0, 1, 4}
	var sb strings.Builder
	for i, g := range data {
		if i > 0 {
			sb.WriteString(",")
		}
		sb.WriteString(itoa(int(g)))
	}
	return `{"type":"tilelayer","name":"l","width":4,"height":3,"data":[` + sb.String() + `]` + fields + `}`
}

func itoa(n int) string {
	if n >= 10 {
		return itoa(n/10) + string(rune('0'+n%10))
	}
	return string(rune('0' + n))
}

func newXEnv(t testing.TB, maps map[string]string) *xenv {
	t.Helper()
	files := tilesetFiles(t)
	for k, v := range maps {
		files[k] = textFile(v)
	}
	return newXEnvFS(t, files, image.Pt(16, 16), 1)
}

// newXEnvFS is newXEnv with any files, screen size and zoom.
func newXEnvFS(t testing.TB, files fstest.MapFS, screen image.Point, zoom float32) *xenv {
	t.Helper()
	e := &xenv{t: t, srv: newServer(t, files), w: ecs.NewWorld(), queue: &render.Queue{}, cam: &render.Camera{Zoom: zoom}}
	ecs.SetResource(e.w, e.srv)
	ecs.SetResource(e.w, e.queue)
	ecs.SetResource(e.w, e.cam)
	ecs.SetResource(e.w, &extract.Screen{Size: screen})
	e.canvases = render.NewCanvases(render.StoresOf(e.srv))
	ecs.SetResource(e.w, e.canvases)
	e.ex = NewTilemapExtract(e.w)
	e.ex.Init(&system.Context{World: e.w})
	return e
}

// spawn loads key synchronously and spawns it.
func (e *xenv) spawn(key string, opts SpawnOptions) []ecs.Entity {
	e.t.Helper()
	h, err := LoadMapSync(e.srv.Global(), key)
	if err != nil {
		e.t.Fatal(err)
	}
	es, err := Spawn(e.w, MapStoreOf(e.srv), h, opts)
	if err != nil {
		e.t.Fatal(err)
	}
	return es
}

// run resets the queue and runs the extract for a tick of length delta.
func (e *xenv) run(delta time.Duration) {
	e.queue.Reset()
	e.ex.Run(&system.Context{World: e.w, Time: system.Time{Delta: delta}})
}

// look puts the camera at pos for this tick and the last.
func (e *xenv) look(pos gfx.Vec2) { e.cam.Pos, e.cam.PrevPos = pos, pos }

func (e *xenv) items() []render.Item {
	out := make([]render.Item, e.queue.Len())
	for i := range out {
		out[i] = *e.queue.At(i)
	}
	return out
}

// sprite returns the sprite of local tile id of t.tsj.
func (e *xenv) sprite(id int) gfx.SpriteHandle {
	h, err := render.LoadSpriteSync(e.srv.Global(), "t.tsj#"+itoa(id))
	if err != nil {
		e.t.Fatal(err)
	}
	return h
}

func TestExtractCullsAndPlaces(t *testing.T) {
	e := newXEnv(t, map[string]string{"m.tmj": plainMap("", "")})
	e.spawn("m.tmj", SpawnOptions{BaseLayer: 2})
	e.look(gfx.Vec2{X: 8, Y: 8}) // bounds [0,16) x [0,16): cells (0..1, 0..1)
	e.run(0)
	items := e.items()
	if len(items) != 4 {
		t.Fatalf("%d items, want 4", len(items))
	}
	wantPos := []gfx.Vec2{{X: 4, Y: 4}, {X: 12, Y: 4}, {X: 4, Y: 12}, {X: 12, Y: 12}}
	wantSprite := []int{1, 2, 2, 1}
	for i, it := range items {
		if it.Pos != wantPos[i] || it.PrevPos != it.Pos || it.Sprite != e.sprite(wantSprite[i]) {
			t.Errorf("item %d: pos %v prev %v sprite %v", i, it.Pos, it.PrevPos, it.Sprite)
		}
		if it.Kind != render.KindSprite || it.Space != render.WorldSpace || it.Layer != 2 ||
			it.Origin != (gfx.Vec2{X: 0.5, Y: 0.5}) || it.Scale != (gfx.Vec2{X: 1, Y: 1}) || it.Color != 0 {
			t.Errorf("item %d = %+v", i, it)
		}
	}
	if items[0].SortY != 8 || items[2].SortY != 16 {
		t.Errorf("SortY = %v, %v", items[0].SortY, items[2].SortY)
	}
	if !items[1].FlipX || items[1].FlipY || items[1].Rot != 0 {
		t.Errorf("H flip: %+v", items[1])
	}

	e.look(gfx.Vec2{X: 20, Y: 12}) // bounds [12,28) x [4,20): cells (1..3, 0..2)
	e.run(0)
	items = e.items()
	if len(items) != 6 {
		t.Fatalf("%d items, want 6", len(items))
	}
	d := items[3] // row 1: (1,1), (2,1)
	if d.Pos != (gfx.Vec2{X: 20, Y: 12}) || math.Abs(float64(d.Rot)-math.Pi/2) > 1e-6 || d.FlipX || !d.FlipY {
		t.Errorf("D flip: %+v", d)
	}
}

// TestTilemapExtractCullsDrawnView checks that the tiles of the whole tick's
// view are pushed: the Renderer blends the camera from PrevPos to Pos, so the
// column visible only at PrevPos must not be culled.
func TestTilemapExtractCullsDrawnView(t *testing.T) {
	e := newXEnv(t, map[string]string{"m.tmj": plainMap("", "")})
	e.spawn("m.tmj", SpawnOptions{})
	// At Pos the view is [12,28) x [0,16): cells x 1..3. At PrevPos it is
	// [0,16): cells x 0..1. Row 0 holds 2, h, 0, 4 and row 1 holds 3, 2, d, 0.
	e.cam.PrevPos, e.cam.Pos = gfx.Vec2{X: 8, Y: 8}, gfx.Vec2{X: 20, Y: 8}
	e.run(0)
	var col0 int
	for _, it := range e.items() {
		if it.Pos.X == 4 {
			col0++
		}
	}
	if col0 != 2 || e.queue.Len() != 6 {
		t.Fatalf("%d items, %d in column 0; want 6 items, 2 in column 0", e.queue.Len(), col0)
	}
}

func TestExtractRenderOrder(t *testing.T) {
	for _, tc := range []struct {
		order string
		want  []gfx.Vec2
	}{
		{"right-down", []gfx.Vec2{{X: 4, Y: 4}, {X: 12, Y: 4}, {X: 4, Y: 12}, {X: 12, Y: 12}}},
		{"right-up", []gfx.Vec2{{X: 4, Y: 12}, {X: 12, Y: 12}, {X: 4, Y: 4}, {X: 12, Y: 4}}},
		{"left-down", []gfx.Vec2{{X: 12, Y: 4}, {X: 4, Y: 4}, {X: 12, Y: 12}, {X: 4, Y: 12}}},
		{"left-up", []gfx.Vec2{{X: 12, Y: 12}, {X: 4, Y: 12}, {X: 12, Y: 4}, {X: 4, Y: 4}}},
	} {
		t.Run(tc.order, func(t *testing.T) {
			e := newXEnv(t, map[string]string{"m.tmj": plainMap(`,"renderorder":"`+tc.order+`"`, "")})
			e.spawn("m.tmj", SpawnOptions{})
			e.look(gfx.Vec2{X: 8, Y: 8})
			e.run(0)
			items := e.items()
			for i, it := range items {
				if it.Pos != tc.want[i] {
					t.Errorf("item %d at %v, want %v", i, it.Pos, tc.want[i])
				}
			}
		})
	}
}

func TestExtractMapPositionAndOffset(t *testing.T) {
	e := newXEnv(t, map[string]string{"m.tmj": plainMap("", `,"offsetx":3,"offsety":-2`)})
	e.spawn("m.tmj", SpawnOptions{Pos: gfx.Vec2{X: 100, Y: 50}})
	e.look(gfx.Vec2{X: 111, Y: 56}) // the same view of the map as (8,8) at the origin
	e.run(0)
	items := e.items()
	if len(items) != 4 || items[0].Pos != (gfx.Vec2{X: 107, Y: 52}) {
		t.Fatalf("%d items, first at %v", len(items), items[0].Pos)
	}
}

func TestExtractOversizedTileAtEdge(t *testing.T) {
	e := newXEnv(t, map[string]string{"m.tmj": `{"orientation":"orthogonal","width":4,"height":3,"tilewidth":8,"tileheight":8,
		"tilesets":[{"firstgid":1,"source":"t.tsj"},{"firstgid":5,"source":"tall.tsj"}],
		"layers":[{"type":"tilelayer","name":"l","width":4,"height":3,"data":[0,0,0,0, 0,0,0,0, 5,0,0,0]}]}`})
	e.spawn("m.tmj", SpawnOptions{})
	e.look(gfx.Vec2{X: 8, Y: 8}) // row 2 (y 16..24) is outside, but the 24 px tile reaches up to y 0
	e.run(0)
	items := e.items()
	if len(items) != 1 || items[0].Pos != (gfx.Vec2{X: 4, Y: 12}) || items[0].SortY != 24 {
		t.Fatalf("items = %+v", items)
	}
	e.look(gfx.Vec2{X: 8, Y: -40})
	e.run(0)
	if e.queue.Len() != 0 {
		t.Fatal("a tile far outside the view was queued")
	}
}

func TestExtractParallax(t *testing.T) {
	e := newXEnv(t, map[string]string{"m.tmj": plainMap("", `,"parallaxx":0.5`)})
	e.spawn("m.tmj", SpawnOptions{})
	e.look(gfx.Vec2{X: 8, Y: 8})
	e.run(0)
	first := e.items()[0] // cell (0,0): offset (8-0)*0.5 = 4
	if first.Pos != (gfx.Vec2{X: 8, Y: 4}) {
		t.Fatalf("pos = %v", first.Pos)
	}
	e.cam.PrevPos, e.cam.Pos = gfx.Vec2{X: 8, Y: 8}, gfx.Vec2{X: 18, Y: 8}
	e.run(0)
	var moved *render.Item
	for _, it := range e.items() {
		if it.Pos.Y == 4 && it.Sprite == first.Sprite && it.Pos.X == 13 {
			moved = &it
		}
	}
	if moved == nil {
		t.Fatalf("cell (0,0) not at x 13 after the camera moved 10: %+v", e.items())
	}
	if moved.PrevPos != (gfx.Vec2{X: 8, Y: 4}) {
		t.Errorf("PrevPos = %v, want the previous tick's position", moved.PrevPos)
	}
}

func TestExtractAnimatedTile(t *testing.T) {
	e := newXEnv(t, map[string]string{"m.tmj": plainMap("", "")})
	e.spawn("m.tmj", SpawnOptions{})
	e.look(gfx.Vec2{X: 20, Y: 20}) // sees cell (2,2), GID 1
	frame := func() gfx.SpriteHandle {
		for _, it := range e.items() {
			if it.Pos == (gfx.Vec2{X: 20, Y: 20}) {
				return it.Sprite
			}
		}
		t.Fatal("animated cell not queued")
		return gfx.SpriteHandle{}
	}
	f0, f1 := e.sprite(0), e.sprite(1)
	for i, tc := range []struct {
		delta time.Duration
		want  gfx.SpriteHandle
	}{
		{60 * time.Millisecond, f0}, {39 * time.Millisecond, f0}, {1 * time.Millisecond, f1},
		{99 * time.Millisecond, f1}, {1 * time.Millisecond, f0}, {150 * time.Millisecond, f1},
	} {
		e.run(tc.delta)
		if got := frame(); got != tc.want {
			t.Errorf("step %d: frame %v, want %v", i, got, tc.want)
		}
	}
}

func TestExtractSkips(t *testing.T) {
	e := newXEnv(t, map[string]string{"m.tmj": plainMap("", ""), "slow.tmj": plainMap("", "")})
	es := e.spawn("m.tmj", SpawnOptions{})
	e.look(gfx.Vec2{X: 8, Y: 8})
	ecs.GetComp[TileLayer](e.w, es[0]).Hidden = true
	e.run(0)
	if e.queue.Len() != 0 {
		t.Fatal("hidden layer queued")
	}
	ecs.GetComp[TileLayer](e.w, es[0]).Index = 7
	ecs.GetComp[TileLayer](e.w, es[0]).Hidden = false
	e.run(0)
	if e.queue.Len() != 0 {
		t.Fatal("out-of-range layer index queued")
	}
	e.w.DeleteEntity(es[0])
	ecs.NewEntity2(e.w, gfx.Transform{}, TileLayer{Map: LoadMap(e.srv.Global(), "slow.tmj")})
	ecs.NewEntity2(e.w, gfx.Transform{}, TileLayer{})
	e.run(0)
	if e.queue.Len() != 0 {
		t.Fatal("a map that is not ready queued items")
	}
}

func TestExtractTintAndImageLayer(t *testing.T) {
	e := newXEnv(t, map[string]string{"m.tmj": `{"orientation":"orthogonal","width":1,"height":1,"tilewidth":8,"tileheight":8,
		"layers":[{"type":"imagelayer","name":"bg","image":"bg.png","offsetx":2,"offsety":3,"opacity":0.5},
		          {"type":"imagelayer","name":"sky","image":"bg.png","repeatx":true,"offsetx":6}]}`})
	es := e.spawn("m.tmj", SpawnOptions{})
	e.look(gfx.Vec2{X: 8, Y: 8})
	e.run(0)
	items := e.items()
	if len(items) != 3 {
		t.Fatalf("%d items, want 3: %+v", len(items), items)
	}
	bg := items[0]
	if bg.Pos != (gfx.Vec2{X: 12, Y: 8}) || bg.SortY != 13 || bg.Color != gfx.RGBA(255, 255, 255, 128) || bg.Layer != 0 {
		t.Errorf("bg = %+v", bg)
	}
	// sky repeats along x: copies at x -14..6 and 6..26 cover [0,16).
	if items[1].Pos != (gfx.Vec2{X: -4, Y: 5}) || items[2].Pos != (gfx.Vec2{X: 16, Y: 5}) {
		t.Errorf("sky = %v, %v", items[1].Pos, items[2].Pos)
	}
	ecs.GetComp[TileLayer](e.w, es[0]).Tint = gfx.RGBA(255, 0, 0, 255)
	e.run(0)
	if e.items()[0].Color != gfx.RGBA(255, 0, 0, 255) {
		t.Error("TileLayer.Tint not used")
	}
}

func TestExtractNoAllocs(t *testing.T) {
	if testing.CoverMode() != "" || raceEnabled {
		t.Skip("instrumentation allocates")
	}
	e := newXEnv(t, map[string]string{"m.tmj": plainMap("", "")})
	for range 3 {
		e.spawn("m.tmj", SpawnOptions{})
	}
	e.look(gfx.Vec2{X: 16, Y: 12})
	e.run(time.Millisecond) // warm up the queue
	if n := testing.AllocsPerRun(50, func() { e.run(16 * time.Millisecond) }); n != 0 {
		t.Fatalf("allocs per run = %v, want 0", n)
	}
	if e.queue.Len() == 0 {
		t.Fatal("nothing queued")
	}
}
