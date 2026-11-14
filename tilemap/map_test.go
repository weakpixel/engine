package tilemap

import (
	"image"
	"math"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/weakpixel/asset"
	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/render"
	testassets "github.com/weakpixel/engine/testdata/assets"
	"github.com/weakpixel/engine/tiled"
)

func near(a, b float32) bool { return math.Abs(float64(a-b)) < 1e-4 }

// TestMapLayersAndTiles loads a map with an image layer, groups, a tile
// and an object layer under the same path, and two tilesets.
func TestMapLayersAndTiles(t *testing.T) {
	srv := newServer(t, testassets.Lib())
	h, err := LoadMapSync(srv.Global(), "world-1/level-1.tmj")
	if err != nil {
		t.Fatal(err)
	}
	m := MapStoreOf(srv).Get(h)
	if m.Width != 6 || m.Height != 4 || m.TileW != 16 || m.TileH != 16 || m.RenderOrder != RightDown || m.Infinite {
		t.Fatalf("map = %+v", m)
	}
	var paths []string
	for _, l := range m.Layers {
		paths = append(paths, l.Path+":"+l.Kind.String())
	}
	if !slices.Equal(paths, testassets.Level1Layers) {
		t.Fatalf("layers = %v", paths)
	}

	img := m.Layers[0]
	if img.Visible || img.Image.IsZero() || img.ImageSize != (gfx.Vec2{X: 128, Y: 64}) || img.Offset != (gfx.Vec2{X: -16}) {
		t.Errorf("image layer = %+v", img)
	}
	if !render.SpriteStoreOf(srv).Ready(img.Image) {
		t.Error("the invisible image layer's image did not load")
	}
	ct, ok := m.LayerByName("collider-tiles")
	if !ok || !near(ct.Opacity, 0.2) || ct.Class != "colliders" || m.Layers[ct.Parent].Name != "colliders" {
		t.Errorf("collider-tiles = %+v", ct)
	}
	if p, _ := ct.Props.String("renderPass"); p != "debug" {
		t.Errorf("renderPass = %q", p)
	}
	if l, ok := m.LayerByPathKind("entity-group/entities", Objects); !ok || len(l.Objects) != 3 || m.Layers[l.Parent].Name != "entity-group" {
		t.Errorf("entities objects = %+v", l)
	}
	if l, _ := m.LayerByPath("entity-group/entities"); l.Kind != Tiles {
		t.Error("LayerByPath does not return the first layer in draw order")
	}

	// GIDs 1-8 are temple, 9-11 colliders.
	wall, ok := m.Tile(9)
	if !ok || wall.Class != "wall" || len(wall.Shapes) != 1 || wall.Shapes[0].Width != 16 || wall.Shapes[0].Height != 16 {
		t.Fatalf("Tile(9) = %+v", wall)
	}
	if st, ok := wall.Props.Bool("static"); !ok || !st {
		t.Error("static")
	}
	if _, ok := m.Tile(0); ok {
		t.Error("Tile(0)")
	}
	if _, ok := m.Tile(12); ok {
		t.Error("Tile(12) is past the last tileset")
	}
	if t3, _ := m.Tile(3); t3.Sprite != TilesetStoreOf(srv).Get(m.Tilesets[0].Tileset).Tiles[2].Sprite {
		t.Error("GID 3 is not local tile 2 of temple")
	}

	main, _ := m.LayerByName("main")
	nonEmpty := 0
	for cy := range m.Height {
		for cx := range m.Width {
			gid, _, ok := m.Cell(main, cx, cy)
			if !ok {
				t.Fatalf("Cell(%d,%d) not ok", cx, cy)
			}
			if gid != 0 {
				nonEmpty++
			}
		}
	}
	if nonEmpty != testassets.Level1MainTiles {
		t.Errorf("main has %d tiles, want %d", nonEmpty, testassets.Level1MainTiles)
	}
	if _, _, ok := m.Cell(main, m.Width, 0); ok {
		t.Error("Cell outside the grid is ok")
	}
	if _, _, ok := m.Cell(&m.Layers[2], 0, 0); ok {
		t.Error("Cell of a group is ok")
	}
}

func TestMapObjects(t *testing.T) {
	srv := newServer(t, testassets.Lib())
	h, err := LoadMapSync(srv.Global(), "world-1/level-1.tmj")
	if err != nil {
		t.Fatal(err)
	}
	m := MapStoreOf(srv).Get(h)
	l, player, ok := m.FindObject("player")
	if !ok || player.Class != "player" || l.Name != "entities" {
		t.Fatalf("player = %+v", player)
	}
	if p := ObjectPos(l, player, gfx.Vec2{X: -96, Y: 0}); !near(p.X, -96+24) || !near(p.Y, 40) {
		t.Errorf("ObjectPos = %v", p)
	}
	chicks := 0
	for _, o := range m.ObjectsOfClass("chick") {
		chicks++
		if mass, ok := o.Props.Float("mass"); !ok || mass != 1 {
			t.Errorf("chick %d mass = %v", o.ID, mass)
		}
	}
	all := 0
	for range m.Objects() {
		all++
	}
	if chicks != 2 || all != 3 {
		t.Errorf("%d chicks, %d objects", chicks, all)
	}
	if _, _, ok := m.FindObject("nobody"); ok {
		t.Error("FindObject(nobody)")
	}
}

func TestMapsShareTilesets(t *testing.T) {
	srv := newServer(t, testassets.Lib())
	sc := srv.NewScope("levels")
	h1 := LoadMap(sc, "world-1/level-1.tmj")
	h2 := LoadMap(sc, "world-1/level-2.tmj")
	for !sc.Done() {
		srv.Update()
	}
	if err := sc.Err(); err != nil {
		t.Fatal(err)
	}
	maps := MapStoreOf(srv)
	m1, m2 := maps.Get(h1), maps.Get(h2)
	for i := range 2 {
		if m1.Tilesets[i].Tileset != m2.Tilesets[i].Tileset || m1.Tilesets[i].Tileset.IsZero() {
			t.Errorf("tileset %d: %v vs %v", i, m1.Tilesets[i].Tileset, m2.Tilesets[i].Tileset)
		}
	}
	if n := TilesetStoreOf(srv).Generic().Len(); n != 2 {
		t.Errorf("%d tilesets loaded, want 2", n)
	}
	if n := render.AtlasStoreOf(srv).Generic().Len(); n != 2 {
		t.Errorf("%d atlases loaded, want 2", n)
	}
	if n := asset.StoreOf[render.Texture](srv).Len(); n != 3 {
		t.Errorf("%d textures loaded, want 3 (two tilesets, one background)", n)
	}
}

// fixtureFiles is a tileset "t.tsj" of four 8x8 tiles plus the given map.
func fixtureFiles(t *testing.T, mapJSON string) fstest.MapFS {
	return fstest.MapFS{
		"m.tmj": textFile(mapJSON),
		"t.tsj": textFile(`{"name":"t","image":"t.png","tilewidth":8,"tileheight":8,"columns":2,"tilecount":4}`),
		"t.png": pngFile(t, 16, 16),
	}
}

func TestInfiniteMapCells(t *testing.T) {
	srv := newServer(t, fixtureFiles(t, `{"orientation":"orthogonal","infinite":true,"width":2,"height":2,"tilewidth":8,"tileheight":8,
		"tilesets":[{"firstgid":1,"source":"t.tsj"}],
		"layers":[{"type":"tilelayer","name":"l","startx":-4,"starty":-2,"width":6,"height":4,
			"chunks":[{"x":-4,"y":-2,"width":2,"height":2,"data":[1,2,3,4]},
			          {"x":0,"y":0,"width":2,"height":2,"data":[0,2147483649,0,4]}]}]}`))
	h, err := LoadMapSync(srv.Global(), "m.tmj")
	if err != nil {
		t.Fatal(err)
	}
	m := MapStoreOf(srv).Get(h)
	l := &m.Layers[0]
	if l.Origin != image.Pt(-4, -2) || l.W != 6 || l.H != 4 || len(l.GIDs) != 24 {
		t.Fatalf("grid origin %v size %dx%d", l.Origin, l.W, l.H)
	}
	for _, tc := range []struct {
		cx, cy int
		gid    uint32
		flip   tiled.Flip
		ok     bool
	}{
		{-4, -2, 1, 0, true}, {-3, -1, 4, 0, true}, {1, 0, 1, tiled.FlipH, true}, {1, 1, 4, 0, true},
		{-1, -1, 0, 0, true}, {2, 0, 0, 0, false}, {-5, 0, 0, 0, false},
	} {
		gid, flip, ok := m.Cell(l, tc.cx, tc.cy)
		if gid != tc.gid || flip != tc.flip || ok != tc.ok {
			t.Errorf("Cell(%d,%d) = %d %b %v, want %d %b %v", tc.cx, tc.cy, gid, flip, ok, tc.gid, tc.flip, tc.ok)
		}
	}
	// Pixels floor to cells: (-25,-9) is in cell (-4,-2), (-17,-1) in (-3,-1).
	if gid, _, _ := m.CellAt(l, gfx.Vec2{X: -25, Y: -9}); gid != 1 {
		t.Errorf("CellAt(-25,-9) = %d, want 1", gid)
	}
	if gid, _, _ := m.CellAt(l, gfx.Vec2{X: -17, Y: -1}); gid != 4 {
		t.Errorf("CellAt(-17,-1) = %d, want 4", gid)
	}
}

func TestGroupComposition(t *testing.T) {
	srv := newServer(t, fixtureFiles(t, `{"orientation":"orthogonal","width":1,"height":1,"tilewidth":8,"tileheight":8,
		"tilesets":[{"firstgid":1,"source":"t.tsj"}],
		"layers":[{"type":"group","name":"g","offsetx":10,"offsety":5,"opacity":0.5,"parallaxx":0.5,"tintcolor":"#ff8080",
			"layers":[{"type":"group","name":"inner","offsetx":1,"visible":false,"parallaxy":2,
				"layers":[{"type":"tilelayer","name":"t","width":1,"height":1,"data":[1],"opacity":0.5,"tintcolor":"#80ffffff"}]}]}]}`))
	h, err := LoadMapSync(srv.Global(), "m.tmj")
	if err != nil {
		t.Fatal(err)
	}
	m := MapStoreOf(srv).Get(h)
	l, ok := m.LayerByPath("g/inner/t")
	if !ok {
		t.Fatal("no g/inner/t")
	}
	if l.Offset != (gfx.Vec2{X: 11, Y: 5}) || !near(l.Opacity, 0.25) || l.Parallax != (gfx.Vec2{X: 0.5, Y: 2}) || l.Visible || l.Parent != 1 {
		t.Errorf("layer = %+v", l)
	}
	// #ff8080 (opaque) times #80ffffff (alpha 0x80): RGB ff8080, alpha 80.
	if l.Tint != gfx.RGBA(0xff, 0x80, 0x80, 0x80) {
		t.Errorf("tint = %#x", uint32(l.Tint))
	}
}

func TestGridRenderSizeAndOverhang(t *testing.T) {
	srv := newServer(t, fstest.MapFS{
		"m.tmj": textFile(`{"orientation":"orthogonal","width":1,"height":1,"tilewidth":16,"tileheight":16,
			"tilesets":[{"firstgid":1,"source":"big.tsj"},{"firstgid":2,"source":"tall.tsj"}],
			"layers":[{"type":"tilelayer","name":"t","width":1,"height":1,"data":[2]}]}`),
		"big.tsj":  textFile(`{"image":"b.png","tilewidth":32,"tileheight":32,"columns":1,"tilecount":1,"tilerendersize":"grid"}`),
		"b.png":    pngFile(t, 32, 32),
		"tall.tsj": textFile(`{"image":"c.png","tilewidth":16,"tileheight":48,"columns":1,"tilecount":1,"tileoffset":{"x":0,"y":-4}}`),
		"c.png":    pngFile(t, 16, 48),
	})
	h, err := LoadMapSync(srv.Global(), "m.tmj")
	if err != nil {
		t.Fatal(err)
	}
	m := MapStoreOf(srv).Get(h)
	if big, _ := m.Tile(1); big.Scale != (gfx.Vec2{X: 0.5, Y: 0.5}) {
		t.Errorf("grid scale = %v", big.Scale)
	}
	if tall, _ := m.Tile(2); tall.Scale != (gfx.Vec2{X: 1, Y: 1}) || tall.Offset != (gfx.Vec2{Y: -4}) {
		t.Errorf("tall = %+v", tall)
	}
	if m.overhang != 48-16+4 {
		t.Errorf("overhang = %v, want 36", m.overhang)
	}
}

func TestMapFailures(t *testing.T) {
	bad := make([]string, 16)
	for i := range bad {
		bad[i] = "9"
	}
	for name, tc := range map[string]struct {
		files fstest.MapFS
		want  []string
	}{
		"gid out of range": {fixtureFiles(t, `{"orientation":"orthogonal","width":3,"height":1,"tilewidth":8,"tileheight":8,
			"tilesets":[{"firstgid":1,"source":"t.tsj"}],
			"layers":[{"type":"group","name":"g","layers":[{"type":"tilelayer","name":"l","width":3,"height":1,"data":[1,0,7]}]}]}`),
			[]string{`layer "g/l": cell (2,0): GID 7 is not a tile of any tileset`}},
		"many bad cells": {fixtureFiles(t, `{"orientation":"orthogonal","width":4,"height":4,"tilewidth":8,"tileheight":8,
			"tilesets":[{"firstgid":1,"source":"t.tsj"}],
			"layers":[{"type":"tilelayer","name":"l","width":4,"height":4,"data":[`+strings.Join(bad, ",")+`]}]}`),
			[]string{`layer "l": cell (1,2)`, `layer "l": 6 more cells with bad GIDs`}},
		"tileset fails": {fstest.MapFS{
			"m.tmj": textFile(`{"orientation":"orthogonal","width":1,"height":1,"tilewidth":8,"tileheight":8,
				"tilesets":[{"firstgid":1,"source":"tiles/t.tsj"}],"layers":[]}`),
			"tiles/t.tsj": textFile(`{"image":"t.png","tilewidth":8,"tileheight":8,"columns":1,"tilecount":1,"fillmode":"preserve-aspect-fit"}`),
			"tiles/t.png": pngFile(t, 8, 8),
		}, []string{"dependency", "tiles/t.tsj", "fillmode"}},
		"missing image": {fixtureFiles(t, `{"orientation":"orthogonal","width":1,"height":1,"tilewidth":8,"tileheight":8,
			"layers":[{"type":"imagelayer","name":"bg","image":"nope.png"}]}`), []string{"nope.png"}},
		"overlapping tilesets": {fixtureFiles(t, `{"orientation":"orthogonal","width":1,"height":1,"tilewidth":8,"tileheight":8,
			"tilesets":[{"firstgid":1,"source":"t.tsj"},{"firstgid":3,"source":"t.tsj"}],"layers":[]}`),
			[]string{`tileset "t" (firstgid 1) overlaps tileset "t" (firstgid 3)`}},
		"decode errors": {fixtureFiles(t, `{"orientation":"hexagonal","width":1,"height":1,"tilewidth":8,"tileheight":8}`),
			[]string{"unsupported Tiled feature: orientation"}},
	} {
		t.Run(name, func(t *testing.T) {
			srv := newServer(t, tc.files)
			_, err := LoadMapSync(srv.Global(), "m.tmj")
			mustContain(t, err, tc.want...)
			if name == "many bad cells" && strings.Count(err.Error(), "\n") != maxBadCells {
				t.Errorf("%d lines, want %d", strings.Count(err.Error(), "\n")+1, maxBadCells+1)
			}
		})
	}
}

func TestEmbeddedCollection(t *testing.T) {
	srv := newServer(t, fstest.MapFS{
		"maps/m.tmj": textFile(`{"orientation":"orthogonal","width":2,"height":1,"tilewidth":8,"tileheight":8,
			"tilesets":[{"firstgid":1,"name":"props","tilewidth":8,"tileheight":8,"tilecount":2,
				"tiles":[{"id":0,"image":"../img/a.png","imagewidth":8,"imageheight":8},{"id":1,"image":"../img/b.png","imagewidth":8,"imageheight":16}]}],
			"layers":[{"type":"tilelayer","name":"l","width":2,"height":1,"data":[1,2]}]}`),
		"img/a.png": pngFile(t, 8, 8),
		"img/b.png": pngFile(t, 8, 16),
	})
	h, err := LoadMapSync(srv.Global(), "maps/m.tmj")
	if err != nil {
		t.Fatal(err)
	}
	m := MapStoreOf(srv).Get(h)
	if !m.Tilesets[0].Tileset.IsZero() || m.Tilesets[0].Name != "props" {
		t.Errorf("tileset = %+v", m.Tilesets[0])
	}
	b, ok := m.Tile(2)
	if !ok || b.Size != (gfx.Vec2{X: 8, Y: 16}) {
		t.Fatalf("Tile(2) = %+v", b)
	}
	if want, _ := render.LoadSpriteSync(srv.Global(), "img/b.png"); b.Sprite != want {
		t.Error("embedded tile image is not relative to the map")
	}
}

func TestMapBounds(t *testing.T) {
	tests := []struct {
		name string
		json string
		want gfx.Rect
	}{
		{"finite", `{"orientation":"orthogonal","width":5,"height":3,"tilewidth":16,"tileheight":8,
			"tilesets":[{"firstgid":1,"source":"t.tsj"}],"layers":[]}`,
			gfx.Rect{Max: gfx.Vec2{X: 80, Y: 24}}},
		{"infinite uses the cells", `{"orientation":"orthogonal","infinite":true,"width":2,"height":2,"tilewidth":8,"tileheight":8,
			"tilesets":[{"firstgid":1,"source":"t.tsj"}],
			"layers":[{"type":"tilelayer","name":"a","startx":-4,"starty":-2,"width":6,"height":4,
				"chunks":[{"x":-4,"y":-2,"width":2,"height":2,"data":[1,2,3,4]},{"x":0,"y":0,"width":2,"height":2,"data":[1,0,0,4]}]},
			          {"type":"tilelayer","name":"b","startx":2,"starty":4,"width":2,"height":2,
				"chunks":[{"x":2,"y":4,"width":2,"height":2,"data":[1,0,0,0]}]}]}`,
			gfx.Rect{Min: gfx.Vec2{X: -32, Y: -16}, Max: gfx.Vec2{X: 32, Y: 48}}},
		{"infinite without cells", `{"orientation":"orthogonal","infinite":true,"width":2,"height":3,"tilewidth":8,"tileheight":8,
			"tilesets":[{"firstgid":1,"source":"t.tsj"}],"layers":[]}`,
			gfx.Rect{Max: gfx.Vec2{X: 16, Y: 24}}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := newServer(t, fixtureFiles(t, tt.json))
			h, err := LoadMapSync(srv.Global(), "m.tmj")
			if err != nil {
				t.Fatal(err)
			}
			if got := MapStoreOf(srv).Get(h).Bounds(); got != tt.want {
				t.Errorf("Bounds = %+v, want %+v", got, tt.want)
			}
		})
	}
}
