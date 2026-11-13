package render

import (
	"image"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/weakpixel/engine/gfx"
)

const tpHash = `{
  "frames": {
    "b_second": {"frame": {"x":16,"y":0,"w":16,"h":16}},
    "a_first":  {"frame": {"x":0,"y":0,"w":12,"h":14}, "trimmed": true,
                 "spriteSourceSize": {"x":2,"y":1,"w":12,"h":14},
                 "sourceSize": {"w":16,"h":16},
                 "pivot": {"x":0.25,"y":1}}
  },
  "meta": {"image": "hero.png"}
}`

const tpArray = `{
  "frames": [
    {"filename": "b_second", "frame": {"x":16,"y":0,"w":16,"h":16}},
    {"filename": "a_first", "frame": {"x":0,"y":0,"w":12,"h":14},
     "spriteSourceSize": {"x":2,"y":1,"w":12,"h":14},
     "sourceSize": {"w":16,"h":16},
     "pivot": {"x":0.25,"y":1}}
  ],
  "meta": {"image": "hero.png"}
}`

func TestTexturePacker(t *testing.T) {
	for name, js := range map[string]string{"hash": tpHash, "array": tpArray} {
		t.Run(name, func(t *testing.T) {
			e := newAssetEnv(t, fstest.MapFS{
				"chars/hero.json": textFile(js),
				"chars/hero.png":  pngFile(t, 32, 16, opaque), // meta.image is relative to the JSON
			}, 0)
			sc := e.srv.NewScope("s")
			ah := LoadAtlas(sc, "chars/hero.json")
			e.settle(sc)

			atlases, sprites := AtlasStoreOf(e.srv), SpriteStoreOf(e.srv)
			if !atlases.Ready(ah) {
				t.Fatalf("not ready: %v", atlases.Err(ah))
			}
			a := atlases.Get(ah)
			// File order is kept, even for the hash form (not alphabetical).
			if len(a.Order) != 2 || sprites.Key(a.Order[0]).Sub != "b_second" || sprites.Key(a.Order[1]).Sub != "a_first" {
				t.Fatalf("Order = %v", a.Order)
			}
			first := sprites.Get(a.Sprites["a_first"])
			if first.Rect != image.Rect(0, 0, 12, 14) || first.Image.Bounds() != first.Rect {
				t.Fatalf("a_first rect = %v / %v", first.Rect, first.Image.Bounds())
			}
			if first.Size != (gfx.Vec2{X: 16, Y: 16}) || first.Offset != (gfx.Vec2{X: 2, Y: 1}) || first.Origin != (gfx.Vec2{X: 0.25, Y: 1}) {
				t.Fatalf("a_first size/offset/origin = %v %v %v", first.Size, first.Offset, first.Origin)
			}
			second := sprites.Get(a.Sprites["b_second"])
			if second.Size != (gfx.Vec2{X: 16, Y: 16}) || second.Offset != (gfx.Vec2{}) || second.Origin != defaultOrigin {
				t.Fatalf("b_second defaults = %v %v %v", second.Size, second.Offset, second.Origin)
			}
		})
	}
}

func TestTexturePackerFrameByKey(t *testing.T) {
	e := newAssetEnv(t, fstest.MapFS{
		"chars/hero.json": textFile(tpArray),
		"chars/hero.png":  pngFile(t, 32, 16, opaque),
	}, 2)
	sc := e.srv.NewScope("s")
	h := LoadSprite(sc, "chars/hero.json#a_first")
	e.settle(sc)
	if !SpriteStoreOf(e.srv).Ready(h) {
		t.Fatalf("err = %v", SpriteStoreOf(e.srv).Err(h))
	}
}

func TestTexturePackerFailures(t *testing.T) {
	png32 := pngFile(t, 32, 16, opaque)
	tests := map[string]struct {
		json string
		want string
	}{
		"bad json":        {`{`, "texture packer json"},
		"no image":        {`{"frames":{"a":{"frame":{"w":1,"h":1}}},"meta":{}}`, `"meta.image" is required`},
		"no frames key":   {`{"meta":{"image":"t.png"}}`, `"frames" is required`},
		"empty frames":    {`{"frames":{},"meta":{"image":"t.png"}}`, "no \"frames\""},
		"frames is wrong": {`{"frames":3,"meta":{"image":"t.png"}}`, "object or an array"},
		"duplicate name":  {`{"frames":[{"filename":"a","frame":{"w":1,"h":1}},{"filename":"a","frame":{"w":1,"h":1}}],"meta":{"image":"t.png"}}`, `duplicate frame name "a"`},
		"unnamed":         {`{"frames":[{"frame":{"w":1,"h":1}}],"meta":{"image":"t.png"}}`, "has no name"},
		"rotated":         {`{"frames":{"a":{"frame":{"w":1,"h":1},"rotated":true}},"meta":{"image":"t.png"}}`, "rotated"},
		"outside texture": {`{"frames":{"a":{"frame":{"x":30,"y":0,"w":8,"h":8}}},"meta":{"image":"t.png"}}`, `frame "a"`},
		"empty rect":      {`{"frames":{"a":{"frame":{"x":0,"y":0,"w":0,"h":8}}},"meta":{"image":"t.png"}}`, "empty or outside"},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			e := newAssetEnv(t, fstest.MapFS{"t.json": textFile(tt.json), "t.png": png32}, 0)
			sc := e.srv.NewScope("s")
			ah := LoadAtlas(sc, "t.json")
			child := LoadSprite(sc, "t.json#a")
			e.settle(sc)

			err := AtlasStoreOf(e.srv).Err(ah)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("err = %v, want it to contain %q", err, tt.want)
			}
			if SpriteStoreOf(e.srv).Err(child) == nil {
				t.Fatal("frame of a failed atlas did not fail")
			}
			// A failed Finish must not leave published frames behind.
			if n := SpriteStoreOf(e.srv).Generic().Len(); n != 1 {
				t.Fatalf("sprite store holds %d slots, want only the reserved one", n)
			}
		})
	}
}
