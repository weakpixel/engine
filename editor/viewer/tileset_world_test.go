package viewer

import (
	"slices"
	"testing"
	"testing/fstest"
	"time"

	"github.com/weakpixel/ecs"
	"github.com/weakpixel/engine/editor/toolshell"
	"github.com/weakpixel/engine/game"
	"github.com/weakpixel/engine/gfx"
	testassets "github.com/weakpixel/engine/testdata/assets"
)

// animTileset is a 4 x 2 single-image tileset whose tile 0 plays tiles 0 and
// 1, 100 ms each, and has one collision rectangle.
const animTileset = `{"columns":4,"image":"t.png","imageheight":32,"imagewidth":64,"margin":0,"spacing":0,
 "name":"anim","tilecount":8,"tilewidth":16,"tileheight":16,
 "tiles":[{"id":0,"animation":[{"tileid":0,"duration":100},{"tileid":1,"duration":100}],
   "objectgroup":{"draworder":"index","name":"","objects":[{"id":1,"x":2,"y":2,"width":8,"height":8,"visible":true}],"opacity":1,"type":"objectgroup","visible":true,"x":0,"y":0}}]}`

// collection is an image collection with tiles 0 and 2 (id 1 is missing).
const collection = `{"name":"things","tilewidth":16,"tileheight":16,"tilecount":3,"columns":0,
 "tiles":[{"id":0,"image":"a.png","imagewidth":16,"imageheight":16},
          {"id":2,"image":"b.png","imagewidth":8,"imageheight":8,"class":"rock"}]}`

func tileSprites(e *game.Engine) []gfx.Vec2 {
	var out []gfx.Vec2
	for _, c := range ecs.NewView2[gfx.Transform, gfx.Sprite](e.World()).Each() {
		tr, _ := c.Unpack()
		out = append(out, tr.Pos)
	}
	return out
}

func TestTilesetGrid(t *testing.T) {
	e, v, src, _ := openFiles(t, testassets.Lib(), "temple/temple.tsj")
	tv := v.inner.(*tilesetView)
	if len(tv.tiles) != 8 || tv.cols != 4 || tv.rows != 2 {
		t.Fatalf("tiles = %d, grid %d x %d, want 8 tiles on 4 x 2", len(tv.tiles), tv.cols, tv.rows)
	}
	if got := label(t, v, "tileset.kind"); got != "single image" {
		t.Errorf("kind = %q", got)
	}
	if got := label(t, v, "tileset.columns"); got != "4" {
		t.Errorf("columns = %q", got)
	}
	// Tile 5 is column 1, row 1; its sprite is centered in the cell.
	if !slices.Contains(tileSprites(e), gfx.Vec2{X: 24, Y: 24}) {
		t.Error("no sprite centered at (24, 24) for tile 5")
	}
	moveTo(e, src, tv.shell, gfx.Vec2{X: 24, Y: 24})
	e.Update()
	if got := label(t, v, "tile.which"); got != "5 (hover)" {
		t.Errorf("hover = %q", got)
	}
}

func TestTilesetCollisionShapes(t *testing.T) {
	e, v, src, _ := openFiles(t, testassets.Lib(), "temple/colliders.tsj")
	tv := v.inner.(*tilesetView)
	moveTo(e, src, tv.shell, gfx.Vec2{X: 8, Y: 8}) // tile 0
	e.Update()
	if got := label(t, v, "tile.shapes"); got != "1" {
		t.Errorf("shapes of tile 0 = %q, want 1", got)
	}
	if got := label(t, v, "tile.class"); got != "wall" {
		t.Errorf("class = %q, want wall", got)
	}
}

func TestTilesetCollection(t *testing.T) {
	files := fstest.MapFS{
		"things.tsj": {Data: []byte(collection)},
		"a.png":      {Data: pngBytes(t, 16, 16)},
		"b.png":      {Data: pngBytes(t, 8, 8)},
	}
	e, v, src, _ := openFiles(t, files, "things.tsj")
	tv := v.inner.(*tilesetView)
	// Three ids (one skipped) lay out on 2 columns; tile 2 is at row 1,
	// column 0, its 8 px image at the cell's top-left corner.
	if len(tv.tiles) != 2 || tv.cols != 2 || tv.rows != 2 {
		t.Fatalf("tiles = %d on %d x %d, want 2 on 2 x 2", len(tv.tiles), tv.cols, tv.rows)
	}
	if !slices.Contains(tileSprites(e), gfx.Vec2{X: 4, Y: 20}) {
		t.Errorf("sprites at %v, want tile 2 centered at (4, 20)", tileSprites(e))
	}
	if got := label(t, v, "tileset.kind"); got != "image collection" {
		t.Errorf("kind = %q", got)
	}
	moveTo(e, src, tv.shell, gfx.Vec2{X: 20, Y: 4}) // the skipped id 1
	e.Update()
	if got := label(t, v, "tile.which"); got != "click a tile to select it" {
		t.Errorf("a cell without a tile: %q", got)
	}
}

func TestTilesetAnimation(t *testing.T) {
	files := fstest.MapFS{
		"anim.tsj": {Data: []byte(animTileset)},
		"t.png":    {Data: pngBytes(t, 64, 32)},
	}
	e, v, src, _ := openFiles(t, files, "anim.tsj")
	tv := v.inner.(*tilesetView)
	ts := tv.tileset()
	frame := func() int {
		for _, c := range ecs.NewView2[gfx.Transform, gfx.Sprite](e.World()).Each() {
			tr, sp := c.Unpack()
			if tr.Pos == (gfx.Vec2{X: 8, Y: 8}) {
				switch sp.ID {
				case ts.Tiles[0].Anim[0].Sprite:
					return 0
				case ts.Tiles[0].Anim[1].Sprite:
					return 1
				}
			}
		}
		return -1
	}
	if frame() != 0 {
		t.Fatalf("frame at start = %d, want 0", frame())
	}
	for range 7 { // 7 ticks are 116 ms: the second frame
		e.Update()
	}
	if frame() != 1 {
		t.Errorf("frame after 116 ms = %d, want 1", frame())
	}
	toggle(t, v, "show.animate", false)
	for range 20 {
		e.Update()
	}
	if frame() != 0 {
		t.Errorf("with Animate off the tile shows frame %d, want 0", frame())
	}
	moveTo(e, src, tv.shell, gfx.Vec2{X: 8, Y: 8})
	e.Update()
	if got := label(t, v, "tile.anim"); got != "2 frames, 200 ms: 0, 1" {
		t.Errorf("animation = %q", got)
	}
	if got := label(t, v, "tile.shapes"); got != "1" {
		t.Errorf("shapes = %q, want 1", got)
	}
}

func TestWorldPreview(t *testing.T) {
	e, v, src, rec := openFiles(t, testassets.Lib(), "world-1/world-1.world")
	wv := v.inner.(*worldView)
	if len(wv.maps) != 2 {
		t.Fatalf("maps = %d, want 2", len(wv.maps))
	}
	// Map 1 spans (-96, 0) to (0, 64), map 2 (0, 0) to (64, 64).
	if got := label(t, v, "world.bounds"); got != "-96, 0, 160 × 64" {
		t.Errorf("bounds = %q", got)
	}
	if cam := e.Camera(); cam.Pos.X != -16 || cam.Pos.Y != 32 {
		t.Errorf("camera = %+v, want centered on the union", *cam)
	}
	if got := label(t, v, "map.which"); got == "" || got[0] == '0' {
		t.Logf("nothing hovered yet: %q", got)
	}
	moveTo(e, src, wv.shell, gfx.Vec2{X: -50, Y: 30}) // inside map 1
	e.Update()
	if got := label(t, v, "map.which"); got != "0 (hover)" {
		t.Fatalf("hover = %q", got)
	}
	if got := label(t, v, "map.file"); got != "world-1/level-1.tmj" {
		t.Errorf("file = %q", got)
	}
	// One click selects and sends nothing; the second click of a double
	// click opens the map's first level.
	click := func() {
		src.Frame.Mouse.Buttons = 1
		e.Update()
		src.Frame.Mouse.Buttons = 0
		e.Update()
	}
	click()
	if got := opens(rec); len(got) != 0 {
		t.Fatalf("a single click opened %v", got)
	}
	if got := label(t, v, "map.which"); got != "0 (selected)" {
		t.Errorf("after one click = %q", got)
	}
	click()
	if got := opens(rec); !slices.Equal(got, []string{"world-1/level-1.level.json"}) {
		t.Errorf("double click opened %v", got)
	}
	// Clicks far apart in time are two single clicks.
	for range 2 * doubleClickTicks {
		e.Update()
	}
	click()
	for range 2 * doubleClickTicks {
		e.Update()
	}
	click()
	if got := opens(rec); len(got) != 1 {
		t.Errorf("slow clicks opened %v", got)
	}
	// The buttons do the same as a double click.
	press(t, v, "level.0")
	press(t, v, "map.preview")
	if got := opens(rec); !slices.Equal(got[1:], []string{"world-1/level-1.level.json", "world-1/level-1.tmj"}) {
		t.Errorf("buttons opened %v", got[1:])
	}
	// Map 2 has no level: a double click does nothing.
	moveTo(e, src, wv.shell, gfx.Vec2{X: 30, Y: 30})
	e.Update()
	n := len(opens(rec))
	click()
	click()
	if len(opens(rec)) != n {
		t.Error("a map without a level opened something")
	}
	if got := label(t, v, "levels.none"); got != "none" {
		t.Errorf("levels = %q", got)
	}
}

func TestWorldGroupLevelAndReload(t *testing.T) {
	files := testassets.Lib()
	files["world-1/world-1.level.json"] = &fstest.MapFile{Data: []byte("{}")}
	e, v, src, _ := openFiles(t, files, "world-1/world-1.world")
	wv := v.inner.(*worldView)
	if !slices.Equal(wv.maps[1].levels, []string{"world-1/world-1.level.json"}) {
		t.Errorf("map 2 levels = %v, want the group level (its backdrop is the world)", wv.maps[1].levels)
	}
	if len(wv.maps[0].levels) != 2 {
		t.Errorf("map 1 levels = %v, want its own and the group's", wv.maps[0].levels)
	}
	moveTo(e, src, wv.shell, gfx.Vec2{X: 30, Y: 30})
	e.Update()
	src.Frame.Mouse.Buttons = 1
	e.Update()
	src.Frame.Mouse.Buttons = 0
	before := wv.versions
	e.Assets().Changed("world-1/level-2.tmj")
	for range 5 {
		time.Sleep(10 * time.Millisecond)
		e.Update()
		if !slices.Equal(wv.versions, before) {
			break
		}
	}
	if slices.Equal(wv.versions, before) {
		t.Fatal("the world did not reload after one of its maps changed")
	}
	if wv.sel != 1 {
		t.Errorf("selection = %d after the reload, want it kept at 1", wv.sel)
	}
}

// Switching from a Tiled file to another removes the layers and tiles.
func TestSwitchingAwayFromTiledFiles(t *testing.T) {
	files := testassets.Lib()
	files["box.png"] = &fstest.MapFile{Data: pngBytes(t, 16, 16)}
	for _, key := range []string{"world-1/level-1.tmj", "world-1/world-1.world", "temple/temple.tsj"} {
		v := New().(*Viewer)
		sh := toolshell.New(toolshell.Config{Name: "view", Tool: v, Key: key, FS: files})
		e := game.New(game.Config{NoAudio: true, Width: 320, Height: 180, FS: files, Tool: sh})
		count := func() int {
			n := 0
			for range ecs.NewView1[gfx.Transform](e.World()).Each() {
				n++
			}
			return n
		}
		e.Update()
		if count() == 0 {
			t.Fatalf("%s: no entities", key)
		}
		sh.Receive([]byte(`{"type":"open","path":"box.png"}`))
		e.Update()
		if n := count(); n != 1 {
			t.Errorf("%s: %d entities after switching to a texture, want its sprite only", key, n)
		}
		e.Close()
	}
}
