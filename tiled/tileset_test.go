package tiled

import (
	"testing"
	"time"
)

func TestDecodeRealTilesets(t *testing.T) {
	temple, err := DecodeTileset(readTestdata(t, "temple.tsj"))
	if err != nil {
		t.Fatal(err)
	}
	if temple.Name != "temple" || temple.IsCollection() || temple.Image != "temple.png" ||
		temple.TileCount != 320 || temple.Columns != 20 || temple.TileWidth != 16 || temple.TileHeight != 16 ||
		temple.ImageWidth != 320 || temple.ImageHeight != 256 || temple.TileRenderSize != "tile" || temple.FillMode != "stretch" {
		t.Errorf("temple = %+v", temple)
	}
	if len(temple.Tiles) != 12 {
		t.Errorf("temple has %d tiles with data, want 12", len(temple.Tiles))
	}
	if tile, ok := temple.Tile(92); !ok || tile.Probability != 8 {
		t.Errorf("Tile(92) = %+v, %v", tile, ok)
	}
	if _, ok := temple.Tile(93); ok {
		t.Error("Tile(93) has data")
	}

	coll, err := DecodeTileset(readTestdata(t, "colliders.tsj"))
	if err != nil {
		t.Fatal(err)
	}
	if coll.TileCount != 100 || len(coll.Tiles) != 18 {
		t.Errorf("colliders: %d tiles, %d with data", coll.TileCount, len(coll.Tiles))
	}
	wall, ok := coll.Tile(0)
	if !ok || wall.Class != "wall" || len(wall.Collision) != 1 {
		t.Fatalf("Tile(0) = %+v", wall)
	}
	if s := wall.Collision[0]; s.Shape != ShapeRect || s.Class != "wall" || s.Width != 16 || s.Height != 16 {
		t.Errorf("wall shape = %+v", s)
	}
	if st, ok := wall.Props.Bool("static"); !ok || !st {
		t.Errorf("static = %v, %v", st, ok)
	}
	// Tile 4 has the property on its shape, not on itself.
	t4, _ := coll.Tile(4)
	if _, ok := t4.Props.Bool("static"); ok {
		t.Error("tile 4 has its own static property")
	}
	if st, ok := t4.Collision[0].Props.Bool("static"); !ok || !st || t4.Collision[0].Y != 8 {
		t.Errorf("tile 4 shape = %+v", t4.Collision[0])
	}
}

func TestDecodeCollectionTileset(t *testing.T) {
	ts, err := DecodeTileset([]byte(`{
		"name":"props","tilewidth":32,"tileheight":48,"tilecount":3,"columns":0,
		"tileoffset":{"x":2,"y":-4},"tilerendersize":"grid","version":"1.10",
		"tiles":[
			{"id":5,"image":"tree.png","imagewidth":32,"imageheight":48,"x":0,"y":0,"width":32,"height":48},
			{"id":1,"image":"rock.png","imagewidth":16,"imageheight":16,"class":"rock",
			 "animation":[{"tileid":1,"duration":100},{"tileid":5,"duration":250}]},
			{"id":2,"image":"bush.png","imagewidth":16,"imageheight":16}
		]}`))
	if err != nil {
		t.Fatal(err)
	}
	if !ts.IsCollection() || ts.TileOffset != (Point{2, -4}) || ts.TileRenderSize != "grid" {
		t.Errorf("tileset = %+v", ts)
	}
	if ids := []int{ts.Tiles[0].ID, ts.Tiles[1].ID, ts.Tiles[2].ID}; ids[0] != 1 || ids[1] != 2 || ids[2] != 5 {
		t.Errorf("tiles not sorted: %v", ids)
	}
	rock, _ := ts.Tile(1)
	if rock.Image != "rock.png" || rock.Class != "rock" || len(rock.Animation) != 2 ||
		rock.Animation[1] != (Frame{TileID: 5, Duration: 250 * time.Millisecond}) {
		t.Errorf("rock = %+v", rock)
	}
}

func TestDecodeTilesetErrors(t *testing.T) {
	for _, tc := range []struct {
		name string
		json string
		want []string
	}{
		{"json", `{`, []string{"tileset: invalid JSON"}},
		{"version", `{"version":"2.0","tilewidth":16,"tileheight":16,"image":"a.png","columns":1,"tilecount":1}`,
			[]string{"unsupported Tiled feature: format version", `"2.0"`}},
		{"numeric version", `{"version":1.2,"tilewidth":16,"tileheight":16,"image":"a.png","columns":1,"tilecount":1}`, nil},
		{"fillmode", `{"fillmode":"preserve-aspect-fit","tilewidth":16,"tileheight":16,"image":"a.png","columns":1,"tilecount":1}`,
			[]string{"unsupported Tiled feature: fillmode", "preserve-aspect-fit"}},
		{"transparentcolor", `{"transparentcolor":"#ff00ff","tilewidth":16,"tileheight":16,"image":"a.png","columns":1,"tilecount":1}`,
			[]string{"unsupported Tiled feature: transparentcolor"}},
		{"sub-rect", `{"tilewidth":16,"tileheight":16,"tiles":[{"id":0,"image":"a.png","imagewidth":32,"imageheight":32,"x":16,"y":0,"width":16,"height":16}]}`,
			[]string{"tile 0", "unsupported Tiled feature: image sub-rectangles"}},
		{"all at once", `{"tilewidth":0,"tileheight":16,"image":"a.png","columns":0,"tilecount":4,"margin":-1,
			"tiles":[{"id":9},{"id":1,"animation":[{"tileid":7,"duration":0}]}]}`,
			[]string{"tilewidth and tileheight must be positive", "columns and tilecount must be positive",
				"margin and spacing must not be negative", "tile 9: id out of range", "animation frame 0: duration must be positive",
				"tile 1: animation frame 0: no tile 7"}},
		{"collection without image", `{"tilewidth":16,"tileheight":16,"tiles":[{"id":0}]}`, []string{"collection tile has no image"}},
		{"bad property", `{"tilewidth":16,"tileheight":16,"image":"a.png","columns":1,"tilecount":1,"properties":[{"name":"x","type":"int","value":"a"}]}`,
			[]string{`property "x"`}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := DecodeTileset([]byte(tc.json))
			if tc.want == nil {
				if err != nil {
					t.Fatal(err)
				}
				return
			}
			mustContain(t, err, tc.want...)
		})
	}
}
