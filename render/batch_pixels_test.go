//go:build ebitentest

package render

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
	"testing/fstest"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/weakpixel/engine/gfx"
)

// gradientSheet returns a grid sheet PNG of n 4x4 frames in which every
// pixel has its own color, so a quad that samples the wrong frame, or the
// right frame shifted or mirrored, changes the output.
func gradientSheet(t *testing.T, n int) *fstest.MapFile {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 4*n, 4))
	for y := range 4 {
		for x := range 4 * n {
			img.SetRGBA(x, y, color.RGBA{R: uint8(x * 255 / (4 * n)), G: uint8(64 * y), B: uint8(255 - 60*(x%4)), A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return &fstest.MapFile{Data: buf.Bytes()}
}

// TestDrawBatchedMatchesUnbatched draws one scene twice, batched and with
// one DrawImage per quad, and compares every pixel: frames of one atlas,
// half-pixel positions, scale, rotation, flips, tints, blend modes and
// shapes, in world and screen space under a zoomed, rotated camera.
func TestDrawBatchedMatchesUnbatched(t *testing.T) {
	files := fstest.MapFS{
		"g.sheet.json": textFile(`{"image":"g.png","frameW":4,"frameH":4}`),
		"g.png":        gradientSheet(t, 4),
	}
	e := newAssetEnv(t, files, 0)
	sc := e.srv.NewScope("g")
	ah := LoadAtlas(sc, "g.sheet.json")
	e.settle(sc)
	frames := AtlasStoreOf(e.srv).Get(ah).Order
	if len(frames) != 4 {
		t.Fatalf("%d frames, want 4", len(frames))
	}

	var q Queue
	for i := range 40 {
		x, y := float32(i%8)*7.5, float32(i/8)*9.25
		it := q.Sprite(int16(i%2), frames[i%4], gfx.Vec2{X: x, Y: y}, gfx.Vec2{X: x - 1.5, Y: y})
		switch i % 5 {
		case 1:
			it.Scale = gfx.Vec2{X: 2, Y: 1.5}
		case 2:
			it.Rot = 0.7
		case 3:
			it.FlipX, it.FlipY = true, i%2 == 0
		case 4:
			it.Color = gfx.RGBA(255, 128, 64, 200)
		}
		if i%7 == 0 {
			it.Blend = BlendAdditive
		}
		if i%11 == 0 {
			it.Space = ScreenSpace
		}
	}
	q.Push(Item{Kind: KindRect, Pos: gfx.Vec2{X: 5, Y: 5}, PrevPos: gfx.Vec2{X: 5, Y: 5}, Scale: gfx.Vec2{X: 1, Y: 1}, Size: gfx.Vec2{X: 20, Y: 12}, Layer: 1})
	q.Push(Item{Kind: KindCircle, Pos: gfx.Vec2{X: 30, Y: 30}, PrevPos: gfx.Vec2{X: 30, Y: 30}, Scale: gfx.Vec2{X: 1, Y: 1}, Size: gfx.Vec2{X: 9}, Layer: 1,
		Color: gfx.RGBA(0, 255, 0, 255)})
	q.Sort(nil, StoresOf(e.srv).Sprites)
	cam := &Camera{Pos: gfx.Vec2{X: 30, Y: 20}, PrevPos: gfx.Vec2{X: 30, Y: 20}, Zoom: 1.25, Rotation: 0.1}

	draw := func(noBatch bool) (*ebiten.Image, Stats) {
		r := NewRenderer(StoresOf(e.srv))
		r.noBatch = noBatch
		dst := newTarget(64, 64)
		r.Draw(dst, &q, cam, 0.5)
		return dst, r.Stats()
	}
	batched, bst := draw(false)
	plain, pst := draw(true)
	// The circle is a mesh of its own in both: one draw call, no quad.
	if bst.DrawCalls >= pst.DrawCalls || bst.Quads != pst.DrawCalls-1 {
		t.Fatalf("batched %+v, unbatched %+v: want fewer draw calls, one quad per unbatched call besides the circle", bst, pst)
	}
	diffs := 0
	for y := range 64 {
		for x := range 64 {
			if b, p := pixel(batched, x, y), pixel(plain, x, y); b != p {
				if diffs < 5 {
					t.Errorf("pixel %d,%d = %v batched, %v unbatched", x, y, b, p)
				}
				diffs++
			}
		}
	}
	if diffs > 0 {
		t.Fatalf("%d pixels differ", diffs)
	}
}

// kageNeighbor mixes each texel with its right neighbor, so a quad whose
// imageSrc0 is not exactly the frame (a wrong region, or the whole sheet)
// shows at the frame's right edge.
const kageNeighbor = `//kage:unit pixels

package main

var Params vec4

func Fragment(dst vec4, src vec2, color vec4) vec4 {
	return mix(imageSrc0At(src), imageSrc0At(src+vec2(1, 0)), Params.x) * color
}
`

// kageFlashTexels is kageFlash in Kage's default texel unit.
const kageFlashTexels = `package main

var Params vec4

func Fragment(dst vec4, src vec2, color vec4) vec4 {
	c := imageSrc0At(src)
	return mix(c, vec4(c.a), Params.x) * color
}
`

// TestDrawShadedMatchesDrawRectShader draws shaded sprites and canvases
// through the Renderer's shaded batches and, as the reference, with one
// DrawRectShader call per item built from the same GeoM, color and blend
// (what the Renderer did before it batched shaded quads), and compares every
// pixel: atlas frames, half-pixel positions, scale, rotation, flips, tints
// with alpha, every blend mode, a canvas, a shader that reads neighbor
// texels and one in texel units.
func TestDrawShadedMatchesDrawRectShader(t *testing.T) {
	files := fstest.MapFS{
		"g.sheet.json":     textFile(`{"image":"g.png","frameW":4,"frameH":4}`),
		"g.png":            gradientSheet(t, 4),
		"fx/flash.kage":    textFile(kageFlash),
		"fx/neighbor.kage": textFile(kageNeighbor),
		"fx/texels.kage":   textFile(kageFlashTexels),
	}
	e := newAssetEnv(t, files, 0)
	sc := e.srv.NewScope("g")
	ah := LoadAtlas(sc, "g.sheet.json")
	shaders := []gfx.ShaderHandle{LoadShader(sc, "fx/flash.kage"), LoadShader(sc, "fx/neighbor.kage"), LoadShader(sc, "fx/texels.kage")}
	e.settle(sc)
	frames := AtlasStoreOf(e.srv).Get(ah).Order
	st := StoresOf(e.srv)
	canvases := NewCanvases(st)
	cv := canvases.New(5, 3)
	canvases.image(cv).Fill(color.RGBA{R: 40, G: 200, B: 90, A: 255})

	var q Queue
	for i := range 84 {
		x, y := float32(i%9)*6.5+2, float32(i/9)*6.25+2
		var it *Item
		if i%13 == 5 {
			q.Push(Item{Kind: KindCanvas, Canvas: cv, Pos: gfx.Vec2{X: x, Y: y}, Scale: gfx.Vec2{X: 1, Y: 1}})
			it = q.At(q.Len() - 1)
		} else {
			it = q.Sprite(0, frames[i/6%4], gfx.Vec2{X: x, Y: y}, gfx.Vec2{X: x, Y: y})
		}
		it.PrevPos = it.Pos
		it.Space = ScreenSpace
		// Runs of six sprites share frame, shader, Params and blend.
		it.Shader = shaders[i/6%3]
		it.Params = [4]float32{float32(i/18%3) / 2}
		it.Blend = Blend(i / 12 % 7)
		switch i % 5 {
		case 1:
			it.Scale = gfx.Vec2{X: 2, Y: 1.5}
		case 2:
			it.Rot = 0.7
		case 3:
			it.FlipX, it.FlipY = true, i%2 == 0
		case 4:
			it.Color = gfx.RGBA(255, 128, 64, 200)
		}
	}

	background := func() *ebiten.Image {
		dst := newTarget(64, 64)
		dst.Fill(color.RGBA{R: 90, G: 60, B: 160, A: 200})
		return dst
	}
	r := NewRenderer(st)
	r.SetCanvases(canvases)
	batched := background()
	r.Draw(batched, &q, &Camera{}, 1)
	if s := r.Stats(); s.ShadedQuads != q.Len() || s.ShaderDraws >= q.Len() {
		t.Fatalf("Stats = %+v, want %d shaded quads in fewer draw calls", s, q.Len())
	}

	ref := background()
	var o ebiten.DrawRectShaderOptions
	o.Uniforms, _ = newUniforms(make([]float32, 1))
	for i := range q.Len() {
		it := q.At(i)
		img := canvases.image(it.Canvas)
		var shift gfx.Vec2
		if it.Kind == KindSprite {
			sp := st.Sprites.Get(it.Sprite)
			img, shift = sp.Image, spriteShift(sp, it.Origin)
		}
		geoM(&o.GeoM, shift, it.FlipX, it.FlipY, transform{pos: it.Pos, rot: it.Rot, scale: it.Scale})
		cr, cg, cb, ca := it.Color.Floats()
		o.ColorScale.Reset()
		o.ColorScale.Scale(cr*ca, cg*ca, cb*ca, ca)
		o.Blend = ebitenBlend(it.Blend)
		o.Images[0] = img
		copy(o.Uniforms[ShaderUniformParams].([]float32), it.Params[:])
		b := img.Bounds()
		ref.DrawRectShader(b.Dx(), b.Dy(), st.Shaders.Get(it.Shader).Shader, &o)
	}

	diffs := 0
	for y := range 64 {
		for x := range 64 {
			if b, p := pixel(batched, x, y), pixel(ref, x, y); b != p {
				if diffs < 5 {
					t.Errorf("pixel %d,%d = %v batched, %v with DrawRectShader", x, y, b, p)
				}
				diffs++
			}
		}
	}
	if diffs > 0 {
		t.Fatalf("%d pixels differ", diffs)
	}
}
