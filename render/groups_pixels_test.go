//go:build ebitentest

package render

import (
	"image/color"
	"testing"
	"testing/fstest"

	"github.com/weakpixel/engine/gfx"
)

// kageGray composites a group in gray (Params.x = amount).
const kageGray = `//kage:unit pixels

package main

var Params vec4

func Fragment(dst vec4, src vec2, color vec4) vec4 {
	c := imageSrc0At(src)
	g := dot(c.rgb, vec3(0.299, 0.587, 0.114))
	return mix(c, vec4(g, g, g, c.a), Params.x)
}
`

// kageBelowSwap replaces the group by what was below it, with red and blue
// swapped: it shows that SampleBelow passes the scene below as imageSrc1.
const kageBelowSwap = `//kage:unit pixels

package main

func Fragment(dst vec4, src vec2, color vec4) vec4 {
	return imageSrc1At(src).bgra
}
`

// groupPixels returns a Renderer, a 4x4 red sprite, and the gray and
// below-swap shaders.
func groupPixels(t *testing.T) (r *Renderer, sprite gfx.SpriteHandle, gray, swap gfx.ShaderHandle) {
	t.Helper()
	files := fstest.MapFS{"gray.kage": textFile(kageGray), "swap.kage": textFile(kageBelowSwap)}
	e := newAssetEnv(t, files, 0)
	sprite = e.loadSheet(files, "s", 1)[0]
	sc := e.srv.NewScope("fx")
	gray, swap = LoadShader(sc, "gray.kage"), LoadShader(sc, "swap.kage")
	e.settle(sc)
	return NewRenderer(StoresOf(e.srv)), sprite, gray, swap
}

// put queues the red sprite with its top-left at (x, y) on layer.
func put(q *Queue, sprite gfx.SpriteHandle, layer int16, x, y float32) *Item {
	it := q.Sprite(layer, sprite, gfx.Vec2{X: x, Y: y}, gfx.Vec2{X: x, Y: y})
	it.Space, it.Origin = ScreenSpace, topLeft
	return it
}

func TestDrawGroupShaderOnlyItsLayers(t *testing.T) {
	r, sprite, gray, _ := groupPixels(t)
	var g GroupConfig
	g.Add(GroupOptions{From: 1, To: 1, Shader: gray, Params: [4]float32{1}})
	r.SetGroups(&g)

	dst := newTarget(16, 16)
	var q Queue
	put(&q, sprite, 0, 0, 0)
	put(&q, sprite, 1, 4, 0)
	put(&q, sprite, 2, 8, 0)
	q.Sort(nil, r.stores.Sprites)
	r.Draw(dst, &q, &Camera{}, 1)

	if got := pixel(dst, 1, 1); got != (color.RGBA{R: 255, A: 255}) {
		t.Fatalf("layer 0 = %v, want red", got)
	}
	if got := pixel(dst, 5, 1); got.R != got.G || got.G != got.B || got.R == 0 || got.A != 255 {
		t.Fatalf("layer 1 (grouped) = %v, want gray", got)
	}
	if got := pixel(dst, 9, 1); got != (color.RGBA{R: 255, A: 255}) {
		t.Fatalf("layer 2 = %v, want red", got)
	}
}

// TestDrawGroupOrder checks that a group composites at its place in the
// layer order: over layer 0, under layer 2.
func TestDrawGroupOrder(t *testing.T) {
	r, sprite, gray, _ := groupPixels(t)
	var g GroupConfig
	g.Add(GroupOptions{From: 1, To: 1, Shader: gray, Params: [4]float32{1}})
	r.SetGroups(&g)

	dst := newTarget(16, 16)
	var q Queue
	put(&q, sprite, 0, 0, 0)
	put(&q, sprite, 1, 2, 0)                                    // covers x 2..5, over layer 0
	put(&q, sprite, 2, 4, 0).Color = gfx.RGBA(0, 255, 255, 255) // red * cyan tint = black, over the group
	q.Sort(nil, r.stores.Sprites)
	r.Draw(dst, &q, &Camera{}, 1)

	if got := pixel(dst, 1, 1); got != (color.RGBA{R: 255, A: 255}) {
		t.Fatalf("x=1 = %v, want layer 0 red", got)
	}
	if got := pixel(dst, 3, 1); got.R != got.G || got.R == 0 {
		t.Fatalf("x=3 = %v, want the gray group over layer 0", got)
	}
	if got := pixel(dst, 5, 1); got != (color.RGBA{A: 255}) {
		t.Fatalf("x=5 = %v, want layer 2 (black) over the group", got)
	}
}

func TestDrawGroupWithoutShaderIsPlain(t *testing.T) {
	r, sprite, _, _ := groupPixels(t)
	var g GroupConfig
	g.Add(GroupOptions{From: 0, To: 3})
	r.SetGroups(&g)
	dst := newTarget(16, 16)
	var q Queue
	put(&q, sprite, 1, 0, 0)
	r.Draw(dst, &q, &Camera{}, 1)
	if got := pixel(dst, 1, 1); got != (color.RGBA{R: 255, A: 255}) {
		t.Fatalf("pixel = %v, want red", got)
	}
	if got := pixel(dst, 6, 1); got.A != 0 {
		t.Fatalf("outside the sprite = %v, want transparent", got)
	}
}

// TestDrawLightMap is the light map recipe: a group cleared to the ambient
// color, a light drawn additively into it, composited with BlendMultiply.
func TestDrawLightMap(t *testing.T) {
	r, sprite, _, _ := groupPixels(t)
	var g GroupConfig
	g.Add(GroupOptions{From: 10, To: 10, Blend: BlendMultiply, Clear: color.RGBA{64, 64, 64, 255}})
	r.SetGroups(&g)

	scene := func(light bool) *Queue {
		var q Queue
		put(&q, sprite, 0, 0, 0)
		put(&q, sprite, 0, 8, 0)
		if light {
			put(&q, sprite, 10, 8, 0).Blend = BlendAdditive // a red light over the second sprite
		}
		q.Sort(nil, r.stores.Sprites)
		return &q
	}

	dst := newTarget(16, 16)
	r.Draw(dst, scene(true), &Camera{}, 1)
	if got := pixel(dst, 1, 1); got.R < 60 || got.R > 68 || got.G != 0 || got.A != 255 {
		t.Fatalf("unlit sprite = %v, want red at ambient 64", got)
	}
	if got := pixel(dst, 9, 1); got != (color.RGBA{R: 255, A: 255}) {
		t.Fatalf("lit sprite = %v, want full red", got)
	}

	// No light this frame: the empty light map still applies the ambient.
	dst = newTarget(16, 16)
	r.Draw(dst, scene(false), &Camera{}, 1)
	if got := pixel(dst, 9, 1); got.R < 60 || got.R > 68 {
		t.Fatalf("sprite without light = %v, want ambient 64", got)
	}
	if st := r.Stats(); st.Groups != 1 {
		t.Fatalf("Groups = %d, want the empty light map composited", st.Groups)
	}
}

func TestDrawGroupSampleBelow(t *testing.T) {
	r, sprite, _, swap := groupPixels(t)
	var g GroupConfig
	g.Add(GroupOptions{From: 1, To: 1, Shader: swap, SampleBelow: true})
	r.SetGroups(&g)
	dst := newTarget(16, 16)
	var q Queue
	put(&q, sprite, 0, 0, 0)  // red below
	put(&q, sprite, 1, 10, 0) // something in the group
	q.Sort(nil, r.stores.Sprites)
	r.Draw(dst, &q, &Camera{}, 1)
	if got := pixel(dst, 1, 1); got != (color.RGBA{B: 255, A: 255}) {
		t.Fatalf("pixel = %v, want the red below turned blue", got)
	}
}

// TestDrawGroupResize checks that group images follow the screen size.
func TestDrawGroupResize(t *testing.T) {
	r, sprite, gray, _ := groupPixels(t)
	var g GroupConfig
	g.Add(GroupOptions{From: 0, To: 0, Shader: gray, Params: [4]float32{1}})
	r.SetGroups(&g)
	for _, size := range []int{16, 40} {
		dst := newTarget(size, size)
		var q Queue
		put(&q, sprite, 0, float32(size-4), float32(size-4))
		r.Draw(dst, &q, &Camera{}, 1)
		if got := pixel(dst, size-2, size-2); got.A != 255 || got.R != got.G {
			t.Fatalf("size %d: corner pixel = %v, want gray sprite", size, got)
		}
	}
}
