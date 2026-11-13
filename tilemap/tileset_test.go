package tilemap

import (
	"image"
	"testing"
	"testing/fstest"
	"time"

	"github.com/weakpixel/asset"
	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/render"
	testassets "github.com/weakpixel/engine/testdata/assets"
)

func TestRegisterLoadersNeedsRender(t *testing.T) {
	srv := asset.NewServer(asset.Options{})
	defer srv.Close()
	mustPanic(t, "asset: type not registered: render.Atlas", func() { RegisterLoaders(srv) })
}

func TestTilesets(t *testing.T) {
	srv := newServer(t, testassets.Lib())
	store, sprites := TilesetStoreOf(srv), render.SpriteStoreOf(srv)

	th, err := LoadTilesetSync(srv.Global(), "temple/temple.tsj")
	if err != nil {
		t.Fatal(err)
	}
	temple := store.Get(th)
	if temple.Name != "temple" || len(temple.Tiles) != 8 || temple.Atlas.IsZero() || temple.GridScale ||
		temple.TileSize != (gfx.Vec2{X: 16, Y: 16}) {
		t.Fatalf("temple = %+v", temple)
	}
	// The tile's sprite is the atlas frame of the same key.
	sp6, err := render.LoadSpriteSync(srv.Global(), "temple/temple.tsj#6")
	if err != nil {
		t.Fatal(err)
	}
	if tile := temple.Tiles[6]; tile.Sprite != sp6 || tile.Size != (gfx.Vec2{X: 16, Y: 16}) || tile.Scale != (gfx.Vec2{X: 1, Y: 1}) {
		t.Errorf("tile 6 = %+v, sprite #6 = %v", tile, sp6)
	}
	// Tile 6 is column 2, row 1 of the 4-column image.
	if r := sprites.Get(temple.Tiles[6].Sprite).Rect; r.Min != image.Pt(32, 16) {
		t.Errorf("tile 6 rect = %v", r)
	}

	ch, err := LoadTilesetSync(srv.Global(), "temple/colliders.tsj")
	if err != nil {
		t.Fatal(err)
	}
	coll := store.Get(ch)
	wall := coll.Tiles[0]
	if wall.Class != "wall" || len(wall.Shapes) != 1 || wall.Shapes[0].Width != 16 {
		t.Errorf("wall = %+v", wall)
	}
	if st, ok := wall.Props.Bool("static"); !ok || !st {
		t.Errorf("static = %v %v", st, ok)
	}
}

func TestCollectionTilesetAndAnimation(t *testing.T) {
	srv := newServer(t, fstest.MapFS{
		"props/props.tsj": textFile(`{"name":"props","tilewidth":32,"tileheight":32,"tilecount":3,"tileoffset":{"x":1,"y":2},
			"tiles":[
				{"id":0,"image":"img/rock.png","imagewidth":16,"imageheight":16,"type":"rock",
				 "animation":[{"tileid":0,"duration":100},{"tileid":4,"duration":300}]},
				{"id":4,"image":"img/tree.png","imagewidth":32,"imageheight":48}]}`),
		"props/img/rock.png": pngFile(t, 16, 16),
		"props/img/tree.png": pngFile(t, 32, 48),
	})
	h, err := LoadTilesetSync(srv.Global(), "props/props.tsj")
	if err != nil {
		t.Fatal(err)
	}
	ts := TilesetStoreOf(srv).Get(h)
	if !ts.Atlas.IsZero() || len(ts.Tiles) != 5 {
		t.Fatalf("tileset = %+v", ts)
	}
	rock, tree := ts.Tiles[0], ts.Tiles[4]
	if !ts.Tiles[1].Sprite.IsZero() {
		t.Error("missing ID 1 has a sprite")
	}
	if tree.Size != (gfx.Vec2{X: 32, Y: 48}) || rock.Size != (gfx.Vec2{X: 16, Y: 16}) || rock.Offset != (gfx.Vec2{X: 1, Y: 2}) {
		t.Errorf("sizes %v %v, offset %v", rock.Size, tree.Size, rock.Offset)
	}
	if want, _ := render.LoadSpriteSync(srv.Global(), "props/img/tree.png"); tree.Sprite != want {
		t.Error("tree sprite is not the whole-image sprite")
	}
	if rock.AnimTotal != 400*time.Millisecond || len(rock.Anim) != 2 || rock.Anim[1].Sprite != tree.Sprite {
		t.Fatalf("anim = %+v", rock.Anim)
	}
	for _, tc := range []struct {
		at   time.Duration
		want gfx.SpriteHandle
	}{
		{0, rock.Sprite}, {99 * time.Millisecond, rock.Sprite}, {100 * time.Millisecond, tree.Sprite},
		{399 * time.Millisecond, tree.Sprite}, {400 * time.Millisecond, rock.Sprite}, {1150 * time.Millisecond, tree.Sprite}, {1250 * time.Millisecond, rock.Sprite},
	} {
		if got := rock.FrameAt(tc.at); got != tc.want {
			t.Errorf("FrameAt(%v) = %v, want %v", tc.at, got, tc.want)
		}
	}
	if tree.FrameAt(time.Hour) != tree.Sprite {
		t.Error("static tile FrameAt")
	}
}

func TestTilesetFailures(t *testing.T) {
	for name, tc := range map[string]struct {
		files fstest.MapFS
		want  string
	}{
		"bad animation tile": {fstest.MapFS{
			"t.tsj": textFile(`{"image":"t.png","tilewidth":8,"tileheight":8,"columns":1,"tilecount":1,
				"tiles":[{"id":0,"animation":[{"tileid":3,"duration":10}]}]}`),
			"t.png": pngFile(t, 8, 8),
		}, "animation frame 0: no tile 3"},
		"missing image": {fstest.MapFS{
			"t.tsj": textFile(`{"image":"nope.png","tilewidth":8,"tileheight":8,"columns":1,"tilecount":1}`),
		}, "dependency"},
		"missing collection image": {fstest.MapFS{
			"t.tsj": textFile(`{"tilewidth":8,"tileheight":8,"tilecount":1,"tiles":[{"id":0,"image":"nope.png"}]}`),
		}, "nope.png"},
	} {
		t.Run(name, func(t *testing.T) {
			srv := newServer(t, tc.files)
			_, err := LoadTilesetSync(srv.Global(), "t.tsj")
			mustContain(t, err, tc.want)
		})
	}
}
