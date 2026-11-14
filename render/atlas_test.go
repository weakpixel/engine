package render

import (
	"image"
	"image/color"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/weakpixel/asset"
	"github.com/weakpixel/engine/gfx"
)

var opaque = color.RGBA{G: 255, A: 255}

func TestGridSheet(t *testing.T) {
	tests := []struct {
		name       string
		texW, texH int
		json       string
		wantN      int
		wantRect   map[string]image.Rectangle // spot checks
	}{
		{
			name: "plain", texW: 32, texH: 16,
			json:  `{"image":"t.png","frameW":8,"frameH":8}`,
			wantN: 8,
			wantRect: map[string]image.Rectangle{
				"0": image.Rect(0, 0, 8, 8), "3": image.Rect(24, 0, 32, 8), "4": image.Rect(0, 8, 8, 16), "7": image.Rect(24, 8, 32, 16),
			},
		},
		{
			name: "margin and spacing", texW: 30, texH: 20,
			json:  `{"image":"t.png","frameW":8,"frameH":8,"margin":1,"spacing":2}`,
			wantN: 6,
			wantRect: map[string]image.Rectangle{
				"0": image.Rect(1, 1, 9, 9), "1": image.Rect(11, 1, 19, 9), "3": image.Rect(1, 11, 9, 19),
			},
		},
		{
			name: "leftover pixels are ignored", texW: 20, texH: 10,
			json:  `{"image":"t.png","frameW":8,"frameH":8}`,
			wantN: 2,
			wantRect: map[string]image.Rectangle{
				"1": image.Rect(8, 0, 16, 8),
			},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := newAssetEnv(t, fstest.MapFS{
				"fx/s.sheet.json": textFile(tt.json),
				"fx/t.png":        pngFile(t, tt.texW, tt.texH, opaque),
			}, 0)
			sc := e.srv.NewScope("s")
			ah := LoadAtlas(sc, "fx/s.sheet.json")
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
				if sp.Rect != rect || sp.Image.Bounds() != rect {
					t.Errorf("frame %s: Rect %v, image bounds %v, want %v", name, sp.Rect, sp.Image.Bounds(), rect)
				}
				if sp.Texture != a.Texture {
					t.Errorf("frame %s: texture handle differs from the atlas's", name)
				}
				if sp.Origin != (gfx.Vec2{X: 0.5, Y: 0.5}) || sp.Size != (gfx.Vec2{X: 8, Y: 8}) || sp.Offset != (gfx.Vec2{}) {
					t.Errorf("frame %s: origin/size/offset = %v %v %v", name, sp.Origin, sp.Size, sp.Offset)
				}
			}
			// Order follows the names "0", "1", ... .
			if sprites.Key(a.Order[1]).Sub != "1" {
				t.Fatalf("Order[1] is %v", sprites.Key(a.Order[1]))
			}
		})
	}
}

func TestGridSheetPivot(t *testing.T) {
	e := newAssetEnv(t, fstest.MapFS{
		"s.sheet.json": textFile(`{"image":"t.png","frameW":4,"frameH":4,"pivot":{"x":0.5,"y":1}}`),
		"t.png":        pngFile(t, 4, 4, opaque),
	}, 0)
	sc := e.srv.NewScope("s")
	sp := LoadSprite(sc, "s.sheet.json#0")
	e.settle(sc)
	if got := SpriteStoreOf(e.srv).Get(sp).Origin; got != (gfx.Vec2{X: 0.5, Y: 1}) {
		t.Fatalf("origin = %v", got)
	}
}

func TestAtlasFrameLoadedBeforeAndAfterParent(t *testing.T) {
	for _, workers := range []int{0, 3} {
		e := newAssetEnv(t, fstest.MapFS{
			"s.sheet.json": textFile(`{"image":"t.png","frameW":8,"frameH":8}`),
			"t.png":        pngFile(t, 16, 8, opaque),
		}, workers)
		sc := e.srv.NewScope("s")
		sprites := SpriteStoreOf(e.srv)

		early := LoadSprite(sc, "s.sheet.json#1") // parent not loaded yet
		if _, st := sprites.Lookup(early); st != asset.Waiting {
			t.Fatalf("workers=%d: reserved frame state = %v", workers, st)
		}
		e.settle(sc)
		if !sprites.Ready(early) {
			t.Fatalf("workers=%d: frame not ready: %v", workers, sprites.Err(early))
		}
		ah := LoadAtlas(sc, "s.sheet.json")
		a := AtlasStoreOf(e.srv).Get(ah)
		if a.Sprites["1"] != early {
			t.Fatalf("workers=%d: atlas handle for frame 1 differs from the one Load returned", workers)
		}
		late := LoadSprite(sc, "s.sheet.json#0")
		if late != a.Sprites["0"] || !sprites.Ready(late) {
			t.Fatalf("workers=%d: frame loaded after the parent is not the atlas's", workers)
		}
	}
}

func TestAtlasUnknownFrame(t *testing.T) {
	e := newAssetEnv(t, fstest.MapFS{
		"s.sheet.json": textFile(`{"image":"t.png","frameW":8,"frameH":8}`),
		"t.png":        pngFile(t, 8, 8, opaque),
	}, 0)
	sc := e.srv.NewScope("s")
	h := LoadSprite(sc, "s.sheet.json#99")
	e.settle(sc)
	if err := SpriteStoreOf(e.srv).Err(h); err == nil || !strings.Contains(err.Error(), `has no Sprite "99"`) {
		t.Fatalf("err = %v", err)
	}
}

func TestAtlasFailures(t *testing.T) {
	tests := map[string]struct {
		files fstest.MapFS
		want  string
	}{
		"bad json": {
			fstest.MapFS{"s.sheet.json": textFile("{")},
			"grid sheet",
		},
		"missing image field": {
			fstest.MapFS{"s.sheet.json": textFile(`{"frameW":8,"frameH":8}`)},
			`"image" is required`,
		},
		"bad frame size": {
			fstest.MapFS{"s.sheet.json": textFile(`{"image":"t.png","frameW":0,"frameH":-1}`)},
			"frameW and frameH must be positive",
		},
		"negative margin": {
			fstest.MapFS{"s.sheet.json": textFile(`{"image":"t.png","frameW":4,"frameH":4,"margin":-1}`)},
			"must not be negative",
		},
		"missing texture": {
			fstest.MapFS{"s.sheet.json": textFile(`{"image":"nope.png","frameW":4,"frameH":4}`)},
			"dependency",
		},
		"corrupt texture": {
			fstest.MapFS{"s.sheet.json": textFile(`{"image":"t.png","frameW":4,"frameH":4}`), "t.png": textFile("x")},
			"dependency",
		},
		"frame larger than texture": {
			fstest.MapFS{"s.sheet.json": textFile(`{"image":"t.png","frameW":64,"frameH":64}`), "t.png": pngFile(t, 8, 8, opaque)},
			"does not fit",
		},
		"image path escapes the root": {
			fstest.MapFS{"s.sheet.json": textFile(`{"image":"../t.png","frameW":4,"frameH":4}`)},
			"asset: bad key",
		},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			e := newAssetEnv(t, tt.files, 0)
			sc := e.srv.NewScope("s")
			ah := LoadAtlas(sc, "s.sheet.json")
			child := LoadSprite(sc, "s.sheet.json#0")
			e.settle(sc)

			atlases := AtlasStoreOf(e.srv)
			if err := atlases.Err(ah); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("atlas err = %v, want it to contain %q", err, tt.want)
			}
			if SpriteStoreOf(e.srv).Err(child) == nil {
				t.Fatal("reserved frame of a failed atlas did not fail")
			}
			// Placeholders, not nil or zero images.
			if atlases.Get(ah).Sprites != nil || SpriteStoreOf(e.srv).Get(child).Image == nil {
				t.Fatal("failed atlas or frame does not resolve to its placeholder")
			}
		})
	}
}

func TestWholeImageSprite(t *testing.T) {
	for _, workers := range []int{0, 3} {
		e := newAssetEnv(t, fstest.MapFS{"ui/cursor.png": pngFile(t, 12, 20, opaque)}, workers)
		sc := e.srv.NewScope("s")
		sh := LoadSprite(sc, "ui/cursor.png")
		th := asset.Load[Texture](sc, "ui/cursor.png") // shares the texture the sprite depends on
		e.settle(sc)

		sprites, textures := SpriteStoreOf(e.srv), asset.StoreOf[Texture](e.srv)
		if !sprites.Ready(sh) {
			t.Fatalf("workers=%d: sprite not ready: %v", workers, sprites.Err(sh))
		}
		sp := sprites.Get(sh)
		if sp.Texture != th || sp.Image != textures.Get(th).Image {
			t.Fatalf("workers=%d: sprite does not use the texture handle/image", workers)
		}
		if sp.Rect != image.Rect(0, 0, 12, 20) || sp.Size != (gfx.Vec2{X: 12, Y: 20}) || sp.Origin != defaultOrigin {
			t.Fatalf("workers=%d: sprite = %+v", workers, sp)
		}
		if textures.Len() != 1 {
			t.Fatalf("workers=%d: texture loaded %d times", workers, textures.Len())
		}
	}
}

func TestWholeImageSpriteFailures(t *testing.T) {
	e := newAssetEnv(t, fstest.MapFS{"junk.png": textFile("x")}, 0)
	sc := e.srv.NewScope("s")
	sprites := SpriteStoreOf(e.srv)
	for _, key := range []string{"junk.png", "missing.png"} {
		h := LoadSprite(sc, key)
		e.settle(sc)
		if sprites.Err(h) == nil {
			t.Fatalf("%s did not fail", key)
		}
	}
}

func TestAtlasUnloadsWithScope(t *testing.T) {
	e := newAssetEnv(t, fstest.MapFS{
		"s.sheet.json": textFile(`{"image":"t.png","frameW":4,"frameH":4}`),
		"t.png":        pngFile(t, 8, 4, opaque),
	}, 0)
	sc := e.srv.NewScope("s")
	LoadSprite(sc, "s.sheet.json#0")
	e.settle(sc)
	sc.Release()
	e.srv.Update()
	for name, n := range map[string]int{
		"textures": asset.StoreOf[Texture](e.srv).Len(),
		"atlases":  AtlasStoreOf(e.srv).Generic().Len(),
		"sprites":  SpriteStoreOf(e.srv).Generic().Len(),
	} {
		if n != 0 {
			t.Errorf("%d %s left after release", n, name)
		}
	}
}
