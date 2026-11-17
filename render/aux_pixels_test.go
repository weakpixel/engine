//go:build ebitentest

package render

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
	"testing/fstest"

	"github.com/weakpixel/engine/gfx"
)

// kageMask multiplies the sprite by the alpha of its Aux image, stretched
// over the sprite, with the mapping the shaders guide shows for a second
// image: imageSrc1At takes imageSrc0Origin plus the position inside
// imageSrc1's region.
const kageMask = `//kage:unit pixels

package main

func Fragment(dst vec4, src vec2, color vec4) vec4 {
	uv := (src - imageSrc0Origin()) / imageSrc0Size()
	m := imageSrc1At(imageSrc0Origin() + uv*imageSrc1Size())
	return imageSrc0At(src) * m.a * color
}
`

// halfMask returns an 8x4 PNG whose left half is opaque white and whose
// right half is transparent. As one image it masks the right half of a
// sprite; as a sheet of two 4x4 frames, frame 0 keeps all of it and frame 1
// none.
func halfMask(t *testing.T) *fstest.MapFile {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 8, 4))
	for y := range 4 {
		for x := range 4 {
			img.SetRGBA(x, y, color.RGBA{255, 255, 255, 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return &fstest.MapFile{Data: buf.Bytes()}
}

// TestDrawShadedAux checks that the shader reads Item.Aux as imageSrc1: an
// image of another size than the sprite, an atlas frame (whose region the
// shader must see, not the whole sheet), and no image for the zero handle
// and a failed sprite, never the placeholder.
func TestDrawShadedAux(t *testing.T) {
	files := fstest.MapFS{
		"mask.png":        halfMask(t),
		"mask.sheet.json": textFile(`{"image":"mask.png","frameW":4,"frameH":4}`),
		"fx/mask.kage":    textFile(kageMask),
	}
	e := newAssetEnv(t, files, 0)
	sprite := e.loadSheet(files, "s", 1)[0]
	sc := e.srv.NewScope("aux")
	whole := LoadSprite(sc, "mask.png")
	ah := LoadAtlas(sc, "mask.sheet.json")
	failed := LoadSprite(sc, "missing.png")
	mask := LoadShader(sc, "fx/mask.kage")
	e.settle(sc)
	frames := AtlasStoreOf(e.srv).Get(ah).Order
	r := NewRenderer(StoresOf(e.srv))

	red := color.RGBA{R: 255, A: 255}
	none := color.RGBA{}
	for _, tc := range []struct {
		name        string
		aux         gfx.SpriteHandle
		left, right color.RGBA
	}{
		{"other size", whole, red, none},
		{"atlas frame opaque", frames[0], red, red},
		{"atlas frame clear", frames[1], none, none},
		{"zero", gfx.SpriteHandle{}, none, none},
		{"failed", failed, none, none},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// The 4x4 sprite at (8,8), scaled to 16x16.
			got := drawItem(t, r, Item{
				Kind: KindSprite, Sprite: sprite, Origin: topLeft,
				Space: ScreenSpace, Pos: gfx.Vec2{X: 8, Y: 8}, PrevPos: gfx.Vec2{X: 8, Y: 8}, Scale: gfx.Vec2{X: 4, Y: 4},
				Shader: mask, Aux: tc.aux,
			})
			if c := got.RGBAAt(11, 15); c != tc.left {
				t.Errorf("left half = %v, want %v", c, tc.left)
			}
			if c := got.RGBAAt(20, 15); c != tc.right {
				t.Errorf("right half = %v, want %v", c, tc.right)
			}
		})
	}
}

// TestDrawShadedAuxOnShape checks that the scratch image path passes Aux
// too: a rect outline through the mask shader with the half mask keeps its
// left half only.
func TestDrawShadedAuxOnShape(t *testing.T) {
	files := fstest.MapFS{"mask.png": halfMask(t), "fx/mask.kage": textFile(kageMask)}
	e := newAssetEnv(t, files, 0)
	sc := e.srv.NewScope("aux")
	whole := LoadSprite(sc, "mask.png")
	mask := LoadShader(sc, "fx/mask.kage")
	e.settle(sc)
	r := NewRenderer(StoresOf(e.srv))
	got := drawItem(t, r, Item{
		Kind: KindRect, Space: ScreenSpace, Pos: gfx.Vec2{X: 10, Y: 10}, PrevPos: gfx.Vec2{X: 10, Y: 10}, Scale: gfx.Vec2{X: 1, Y: 1},
		Size: gfx.Vec2{X: 40, Y: 20}, Shader: mask, Aux: whole,
	})
	// The region is the rect plus the margin, x 6 to 54; the mask's left
	// half covers x 6 to 30.
	if c := got.RGBAAt(15, 10); c.A != 255 {
		t.Errorf("top edge, left part = %v, want opaque", c)
	}
	if c := got.RGBAAt(45, 10); c.A != 0 {
		t.Errorf("top edge, right part = %v, want transparent", c)
	}
}
