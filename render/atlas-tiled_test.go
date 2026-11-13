package render

import (
	"image"
	"os"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/weakpixel/engine/gfx"
)

func TestTiledTilesetAtlas(t *testing.T) {
	temple, err := os.ReadFile("../tiled/testdata/temple.tsj")
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name     string
		tsj      []byte
		texW     int
		texH     int
		wantN    int
		wantRect map[string]image.Rectangle
	}{
		{
			name: "temple", tsj: temple, texW: 320, texH: 256, wantN: 320,
			wantRect: map[string]image.Rectangle{
				"0": image.Rect(0, 0, 16, 16), "19": image.Rect(304, 0, 320, 16),
				"92": image.Rect(192, 64, 208, 80), "319": image.Rect(304, 240, 320, 256),
			},
		},
		{
			name: "margin and spacing", texW: 30, texH: 20, wantN: 5,
			tsj: []byte(`{"image":"t.png","tilewidth":8,"tileheight":8,"columns":3,"tilecount":5,"margin":1,"spacing":2}`),
			wantRect: map[string]image.Rectangle{
				"0": image.Rect(1, 1, 9, 9), "2": image.Rect(21, 1, 29, 9), "4": image.Rect(11, 11, 19, 19),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newAssetEnv(t, fstest.MapFS{
				"tiles/x.tsj": &fstest.MapFile{Data: tt.tsj},
				// The temple tileset names temple.png; the fixture names t.png.
				"tiles/temple.png": pngFile(t, tt.texW, tt.texH, opaque),
				"tiles/t.png":      pngFile(t, tt.texW, tt.texH, opaque),
			}, 0)
			sc := e.srv.NewScope("s")
			ah := LoadAtlas(sc, "tiles/x.tsj")
			e.settle(sc)
			atlases, sprites := AtlasStoreOf(e.srv), SpriteStoreOf(e.srv)
			if !atlases.Ready(ah) {
				t.Fatalf("atlas not ready: %v", atlases.Err(ah))
			}
			a := atlases.Get(ah)
			if len(a.Order) != tt.wantN || len(a.Sprites) != tt.wantN {
				t.Fatalf("frames = %d / %d, want %d", len(a.Order), len(a.Sprites), tt.wantN)
			}
			for name, rect := range tt.wantRect {
				sp := sprites.Get(a.Sprites[name])
				size := gfx.Vec2{X: float32(rect.Dx()), Y: float32(rect.Dy())}
				if sp.Rect != rect || sp.Size != size {
					t.Errorf("tile %s: Rect %v size %v, want %v", name, sp.Rect, sp.Size, rect)
				}
			}
			if sprites.Key(a.Order[4]).Sub != "4" {
				t.Errorf("Order[4] is %v", sprites.Key(a.Order[4]))
			}
		})
	}

	// A tile is loadable as a sprite on its own, which loads the atlas.
	e := newAssetEnv(t, fstest.MapFS{
		"tiles/temple.tsj": &fstest.MapFile{Data: temple},
		"tiles/temple.png": pngFile(t, 320, 256, opaque),
	}, 0)
	sc := e.srv.NewScope("s")
	sp := LoadSprite(sc, "tiles/temple.tsj#92")
	e.settle(sc)
	if got := SpriteStoreOf(e.srv).Get(sp).Rect; got != image.Rect(192, 64, 208, 80) {
		t.Fatalf("#92 rect = %v (err %v)", got, SpriteStoreOf(e.srv).Err(sp))
	}
}

func TestTiledTilesetAtlasFailures(t *testing.T) {
	for name, tt := range map[string]struct {
		tsj  string
		texW int
		want string
	}{
		"collection": {`{"tilewidth":8,"tileheight":8,"tilecount":1,"tiles":[{"id":0,"image":"a.png","imagewidth":8,"imageheight":8}]}`, 8,
			"collection tileset has no single image; load it as tilemap.Tileset"},
		"invalid":            {`{"image":"t.png","tilewidth":0,"tileheight":8,"columns":1,"tilecount":1}`, 8, "tilewidth and tileheight must be positive"},
		"grid outside image": {`{"image":"t.png","tilewidth":8,"tileheight":8,"columns":4,"tilecount":4}`, 16, "outside"},
	} {
		t.Run(name, func(t *testing.T) {
			e := newAssetEnv(t, fstest.MapFS{"x.tsj": textFile(tt.tsj), "t.png": pngFile(t, tt.texW, 8, opaque)}, 0)
			sc := e.srv.NewScope("s")
			ah := LoadAtlas(sc, "x.tsj")
			e.settle(sc)
			if err := AtlasStoreOf(e.srv).Err(ah); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err = %v, want %q", err, tt.want)
			}
		})
	}
}
