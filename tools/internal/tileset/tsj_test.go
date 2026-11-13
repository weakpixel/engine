package tileset

import (
	"image"
	"testing"

	"github.com/weakpixel/engine/tiled"
)

func TestTSJ(t *testing.T) {
	a := mustAtlas(t, twoSizes)
	l, err := NewLayout(a, image.Point{}, 2, 1, 2)
	if err != nil {
		t.Fatal(err)
	}
	data, err := EncodeTSJ(a, l, "set", "set.png")
	if err != nil {
		t.Fatal(err)
	}
	ts, err := tiled.DecodeTileset(data)
	if err != nil {
		t.Fatal(err)
	}
	if ts.Name != "set" || ts.Image != "set.png" || ts.TileWidth != 4 || ts.TileHeight != 4 ||
		ts.TileCount != 3 || ts.Columns != 2 || ts.Margin != 1 || ts.Spacing != 2 ||
		ts.ImageWidth != l.Size().X || ts.ImageHeight != l.Size().Y {
		t.Errorf("tileset = %+v", ts)
	}
	for i, want := range []string{"a", "b", "c"} {
		tile, ok := ts.Tile(i)
		if !ok {
			t.Fatalf("no tile %d", i)
		}
		if got, _ := tile.Props.String("name"); got != want {
			t.Errorf("tile %d name = %q, want %q", i, got, want)
		}
	}
}
