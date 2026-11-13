//go:build ebitentest

package render

import (
	"image"
	"image/color"
	"testing"
	"testing/fstest"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/weakpixel/engine/gfx"
)

// kageSwapRB swaps red and blue of the frame.
const kageSwapRB = `//kage:unit pixels

package main

func Fragment(dst vec4, src vec2, color vec4) vec4 {
	return imageSrc0At(src).bgra
}
`

// kageSetRed sets the red channel of the frame to Params.x.
const kageSetRed = `//kage:unit pixels

package main

var Params vec4

func Fragment(dst vec4, src vec2, color vec4) vec4 {
	c := imageSrc0At(src)
	c.r = Params.x
	return c
}
`

// kageScanlines darkens every second row of window pixels.
const kageScanlines = `//kage:unit pixels

package main

func Fragment(dst vec4, src vec2, color vec4) vec4 {
	if mod(floor(dst.y), 2) == 1 {
		return vec4(0, 0, 0, 1)
	}
	return imageSrc0At(src)
}
`

// postPixels returns a Renderer, a queue with the red 4x4 sprite at the
// origin, and the swap, set-red and scanline shaders.
func postPixels(t *testing.T) (r *Renderer, q *Queue, swap, setRed, scan gfx.ShaderHandle) {
	t.Helper()
	files := fstest.MapFS{
		"swap.kage": textFile(kageSwapRB), "red.kage": textFile(kageSetRed), "scan.kage": textFile(kageScanlines),
	}
	e := newAssetEnv(t, files, 0)
	sprite := e.loadSheet(files, "s", 1)[0]
	sc := e.srv.NewScope("fx")
	swap, setRed, scan = LoadShader(sc, "swap.kage"), LoadShader(sc, "red.kage"), LoadShader(sc, "scan.kage")
	e.settle(sc)
	r = NewRenderer(StoresOf(e.srv))
	q = &Queue{}
	put(q, sprite, 0, 0, 0)
	q.Sort(nil, r.stores.Sprites)
	return r, q, swap, setRed, scan
}

// drawOnGreen fills a 16x16 target green, as game.Engine fills the
// background, and draws q onto it.
func drawOnGreen(r *Renderer, q *Queue) *ebiten.Image {
	dst := newTarget(16, 16)
	dst.Fill(color.RGBA{G: 255, A: 255})
	r.Draw(dst, q, &Camera{}, 1)
	return dst
}

func TestDrawPostOrder(t *testing.T) {
	r, q, swap, setRed, _ := postPixels(t)
	tests := []struct {
		name   string
		passes []Pass
		want   color.RGBA // the red sprite after the chain
	}{
		{"swap then set red", []Pass{{Shader: swap}, {Shader: setRed}}, color.RGBA{B: 255, A: 255}},
		{"set red then swap", []Pass{{Shader: setRed}, {Shader: swap}}, color.RGBA{A: 255}},
		{"off pass skipped", []Pass{{Shader: swap}, {Shader: setRed, Off: true}}, color.RGBA{B: 255, A: 255}},
		{"three passes", []Pass{{Shader: swap}, {Shader: setRed}, {Shader: swap}}, color.RGBA{R: 255, A: 255}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r.SetPost(&PostChain{Passes: tt.passes})
			dst := drawOnGreen(r, q)
			if got := pixel(dst, 1, 1); got != tt.want {
				t.Fatalf("sprite = %v, want %v", got, tt.want)
			}
			// The background drawn before Draw goes through the chain too:
			// green has no red or blue, and setRed only ever sets red to 0, so
			// every chain keeps it.
			if got := pixel(dst, 10, 10); got != (color.RGBA{G: 255, A: 255}) {
				t.Fatalf("background = %v, want green", got)
			}
		})
	}
}

func TestDrawPostNoPassIsPlain(t *testing.T) {
	r, q, swap, _, _ := postPixels(t)
	r.SetPost(nil)
	want := drawOnGreen(r, q)
	r.SetPost(&PostChain{Passes: []Pass{{Shader: swap, Off: true}, {}}})
	got := drawOnGreen(r, q)
	for y := range 16 {
		for x := range 16 {
			if a, b := pixel(got, x, y), pixel(want, x, y); a != b {
				t.Fatalf("pixel (%d,%d) = %v, want %v as without post", x, y, a, b)
			}
		}
	}
	if r.postImgs[0] != nil {
		t.Fatal("a chain with no running pass allocated an image")
	}
}

func TestDrawPostResize(t *testing.T) {
	r, q, swap, _, _ := postPixels(t)
	r.SetPost(&PostChain{Passes: []Pass{{Shader: swap}, {Shader: swap}}})
	drawOnGreen(r, q)
	small := newTarget(8, 8)
	r.Draw(small, q, &Camera{}, 1)
	if got := pixel(small, 1, 1); got != (color.RGBA{R: 255, A: 255}) {
		t.Fatalf("sprite after resize = %v, want red (swapped twice)", got)
	}
	if s := r.postImgs[1].Bounds().Size(); s.X != 8 || s.Y != 8 {
		t.Fatalf("post image = %v, want 8x8", s)
	}
}

func TestDrawFinalPass(t *testing.T) {
	r, _, _, _, scan := postPixels(t)
	frame := newTarget(8, 8)
	frame.Fill(color.RGBA{R: 255, A: 255})
	var geoM ebiten.GeoM
	geoM.Scale(2, 2)

	window := newTarget(16, 16)
	r.SetPost(&PostChain{Final: &Pass{Shader: scan}})
	r.DrawFinal(window, frame, geoM)
	// Scanlines are one window pixel high, half a frame pixel.
	for y, want := range []color.RGBA{{R: 255, A: 255}, {A: 255}, {R: 255, A: 255}, {A: 255}} {
		if got := pixel(window, 3, y); got != want {
			t.Fatalf("window row %d = %v, want %v", y, got, want)
		}
	}

	window = newTarget(16, 16)
	r.SetPost(&PostChain{})
	r.DrawFinal(window, frame, geoM)
	if got := pixel(window, 3, 1); got != (color.RGBA{R: 255, A: 255}) {
		t.Fatalf("without final pass row 1 = %v, want the frame scaled", got)
	}
}

// TestDrawFinalMatchesDrawRectShader checks that the final pass covers and
// samples exactly what a DrawRectShader call with the same GeoM did, at a
// non-integer window scale with a letterbox offset.
func TestDrawFinalMatchesDrawRectShader(t *testing.T) {
	r, _, _, _, scan := postPixels(t)
	frame := newTarget(10, 6)
	for x := range 10 {
		frame.SubImage(image.Rect(x, 0, x+1, 6)).(*ebiten.Image).Fill(color.RGBA{R: uint8(25 * x), G: 200, B: uint8(255 - 25*x), A: 255})
	}
	var geoM ebiten.GeoM
	geoM.Scale(2.7, 2.7)
	geoM.Translate(1.4, 3.6)

	// Unmanaged images are not on Ebiten's atlas, so dst (in the
	// destination texture's pixels) is the same for both, as on the real
	// window screen.
	window := ebiten.NewImageWithOptions(image.Rect(0, 0, 32, 24), &ebiten.NewImageOptions{Unmanaged: true})
	r.SetPost(&PostChain{Final: &Pass{Shader: scan}})
	r.DrawFinal(window, frame, geoM)

	ref := ebiten.NewImageWithOptions(image.Rect(0, 0, 32, 24), &ebiten.NewImageOptions{Unmanaged: true})
	o := ebiten.DrawRectShaderOptions{GeoM: geoM, Blend: ebiten.BlendCopy, Images: [4]*ebiten.Image{frame}}
	o.Uniforms, _ = newUniforms(make([]float32, 1))
	ref.DrawRectShader(10, 6, r.stores.Shaders.Get(scan).Shader, &o)
	for y := range 24 {
		for x := range 32 {
			if got, want := pixel(window, x, y), pixel(ref, x, y); got != want {
				t.Fatalf("window pixel %d,%d = %v, want %v as with DrawRectShader", x, y, got, want)
			}
		}
	}
}

func TestDrawPostFade(t *testing.T) {
	r, q, swap, _, _ := postPixels(t)
	for _, tt := range []struct {
		name  string
		chain PostChain
		want  color.RGBA // the green background
	}{
		{"none", PostChain{}, color.RGBA{G: 255, A: 255}},
		{"half black", PostChain{Fade: gfx.RGBA(0, 0, 0, 128)}, color.RGBA{G: 127, A: 255}},
		{"full white", PostChain{Fade: gfx.White}, color.RGBA{R: 255, G: 255, B: 255, A: 255}},
		{"after a pass", PostChain{Passes: []Pass{{Shader: swap}}, Fade: gfx.Black}, color.RGBA{A: 255}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			r.SetPost(&tt.chain)
			dst := drawOnGreen(r, q)
			got := pixel(dst, 10, 10)
			if d := int(got.G) - int(tt.want.G); got.R != tt.want.R || got.B != tt.want.B || d < -1 || d > 1 {
				t.Fatalf("background = %v, want %v", got, tt.want)
			}
		})
	}
}

// kageFill paints everything green.
const kageFill = `//kage:unit pixels

package main

func Fragment(dst vec4, src vec2, color vec4) vec4 {
	return vec4(0, 1, 0, 1)
}
`

// kageMixSizes takes red from imageSrc0 and green from imageSrc1, whatever
// the size of imageSrc1.
const kageMixSizes = `//kage:unit pixels

package main

func Fragment(dst vec4, src vec2, color vec4) vec4 {
	c := imageSrc0At(src)
	p := imageSrc0Origin() + (src-imageSrc0Origin())/imageSrc0Size()*imageSrc1Size()
	return vec4(c.r, imageSrc1At(p).g, 0, 1)
}
`

// kageFrameMix takes red from imageSrc1 and blue of imageSrc0 into green.
const kageFrameMix = `//kage:unit pixels

package main

func Fragment(dst vec4, src vec2, color vec4) vec4 {
	return vec4(imageSrc1At(src).r, imageSrc0At(src).b, 0, 1)
}
`

// kageHalfPrev averages the red of the chain and of imageSrc1.
const kageHalfPrev = `//kage:unit pixels

package main

func Fragment(dst vec4, src vec2, color vec4) vec4 {
	return vec4(imageSrc0At(src).r*0.5+imageSrc1At(src).r*0.5, 0, 0, 1)
}
`

// postPixelsWith is postPixels with the shaders of sources added to the
// renderer's asset server (they share one: the renderer is made over it).
func postPixelsWith(t *testing.T, sources map[string]string) (*Renderer, *Queue, map[string]gfx.ShaderHandle) {
	t.Helper()
	files := fstest.MapFS{}
	for name, src := range sources {
		files[name+".kage"] = textFile(src)
	}
	e := newAssetEnv(t, files, 0)
	sprite := e.loadSheet(files, "s", 1)[0]
	sc := e.srv.NewScope("fx")
	out := map[string]gfx.ShaderHandle{}
	for name := range sources {
		out[name] = LoadShader(sc, name+".kage")
	}
	e.settle(sc)
	r := NewRenderer(StoresOf(e.srv))
	q := &Queue{}
	put(q, sprite, 0, 0, 0)
	q.Sort(nil, r.stores.Sprites)
	return r, q, out
}

func TestDrawPostTargetSizes(t *testing.T) {
	r, q, sh := postPixelsWith(t, map[string]string{"fill": kageFill})
	post := &PostChain{}
	half, quarter := post.AddTarget(Target{Scale: 2}), post.AddTarget(Target{Scale: 4})
	post.Passes = []Pass{{Shader: sh["fill"], Out: half}, {Shader: sh["fill"], Out: quarter}}
	r.SetPost(post)
	for _, tt := range []struct{ screen, half, quarter int }{{16, 8, 4}, {10, 5, 3}, {16, 8, 4}} {
		dst := newTarget(tt.screen, tt.screen)
		r.Draw(dst, q, &Camera{}, 1)
		if s := r.ptargets[0].Bounds().Size(); s.X != tt.half || s.Y != tt.half {
			t.Fatalf("screen %d: half target = %v, want %d", tt.screen, s, tt.half)
		}
		if s := r.ptargets[1].Bounds().Size(); s.X != tt.quarter || s.Y != tt.quarter {
			t.Fatalf("screen %d: quarter target = %v, want %d", tt.screen, s, tt.quarter)
		}
		// No pass writes the chain: the frame reaches the screen unchanged.
		if got := pixel(dst, 1, 1); got != (color.RGBA{R: 255, A: 255}) {
			t.Fatalf("screen %d: sprite = %v, want the frame", tt.screen, got)
		}
	}
}

func TestDrawPostMixedSizes(t *testing.T) {
	r, q, sh := postPixelsWith(t, map[string]string{"fill": kageFill, "mix": kageMixSizes})
	post := &PostChain{}
	half := post.AddTarget(Target{Scale: 2})
	post.Passes = []Pass{
		{Shader: sh["fill"], Out: half},
		{Shader: sh["mix"], Extra: [3]TargetID{half}},
	}
	r.SetPost(post)
	dst := drawOnGreen(r, q)
	for _, tt := range []struct {
		x, y int
		want color.RGBA
	}{
		{1, 1, color.RGBA{R: 255, G: 255, A: 255}}, // sprite: red from the chain, green from the half target
		{10, 10, color.RGBA{G: 255, A: 255}},       // background
	} {
		if got := pixel(dst, tt.x, tt.y); got != tt.want {
			t.Fatalf("pixel (%d,%d) = %v, want %v", tt.x, tt.y, got, tt.want)
		}
	}
}

func TestDrawPostFrameInput(t *testing.T) {
	r, q, sh := postPixelsWith(t, map[string]string{"swap": kageSwapRB, "red": kageSetRed, "mix": kageFrameMix})
	for _, tt := range []struct {
		name   string
		passes []Pass
	}{
		{"after two chain passes", []Pass{
			{Shader: sh["swap"]},
			{Shader: sh["red"], Params: [4]float32{0.5}},
			{Shader: sh["mix"], Extra: [3]TargetID{TargetFrame}},
		}},
		{"as the source", []Pass{
			{Shader: sh["swap"]},
			{Shader: sh["red"], Params: [4]float32{0.5}},
			{Shader: sh["mix"], Src: TargetChain, Extra: [3]TargetID{TargetFrame}},
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			r.SetPost(&PostChain{Passes: tt.passes})
			dst := drawOnGreen(r, q)
			// The frame's red is 1 although the second pass wrote 0.5 over
			// the chain; the chain's blue (set by the swap) is 1.
			if got := pixel(dst, 1, 1); got != (color.RGBA{R: 255, G: 255, A: 255}) {
				t.Fatalf("sprite = %v, want the untouched frame's red", got)
			}
		})
	}
}

func TestDrawPostPrevious(t *testing.T) {
	r, q, sh := postPixelsWith(t, map[string]string{"half": kageHalfPrev})
	r.SetPost(&PostChain{Passes: []Pass{{Shader: sh["half"], Extra: [3]TargetID{TargetPrevious}}}})
	var empty Queue
	red := func(dst *ebiten.Image) int { return int(pixel(dst, 1, 1).R) }
	near := func(got, want int) bool { return got >= want-2 && got <= want+2 }

	dst := newTarget(16, 16)
	r.Draw(dst, q, &Camera{}, 1)
	if got := red(dst); got != 255 {
		t.Fatalf("frame 1: red = %d, want 255 (the first frame is its own previous)", got)
	}
	dst = newTarget(16, 16)
	r.Draw(dst, &empty, &Camera{}, 1)
	if got := red(dst); !near(got, 128) {
		t.Fatalf("frame 2: red = %d, want 128", got)
	}
	dst = newTarget(16, 16)
	r.Draw(dst, &empty, &Camera{}, 1)
	if got := red(dst); !near(got, 64) {
		t.Fatalf("frame 3: red = %d, want 64", got)
	}
	// A resize starts again from the current frame.
	small := newTarget(8, 8)
	r.Draw(small, &empty, &Camera{}, 1)
	if got := red(small); got != 0 {
		t.Fatalf("after a resize: red = %d, want 0", got)
	}
}

// kageCopy shows imageSrc0 unchanged.
const kageCopy = `//kage:unit pixels

package main

func Fragment(dst vec4, src vec2, color vec4) vec4 {
	return imageSrc0At(src)
}
`

func TestDrawPostGroup(t *testing.T) {
	r, q, sh := postPixelsWith(t, map[string]string{"copy": kageCopy})
	blue := color.RGBA{B: 255, A: 255}
	green, red := color.RGBA{G: 255, A: 255}, color.RGBA{R: 255, A: 255}
	var g GroupConfig
	id := g.Add(GroupOptions{From: 0, To: 0, NoComposite: true, Clear: blue})
	r.SetGroups(&g)

	t.Run("NoComposite stays off the screen", func(t *testing.T) {
		r.SetPost(nil)
		if got := pixel(drawOnGreen(r, q), 1, 1); got != green {
			t.Fatalf("sprite pixel = %v, want the untouched green", got)
		}
	})
	t.Run("pass sees the group image", func(t *testing.T) {
		r.SetPost(&PostChain{Passes: []Pass{{Shader: sh["copy"], Src: TargetGroup(id)}}})
		dst := drawOnGreen(r, q)
		if got := pixel(dst, 1, 1); got != red {
			t.Fatalf("sprite pixel = %v, want red", got)
		}
		if got := pixel(dst, 12, 12); got != blue {
			t.Fatalf("clear pixel = %v, want the group's Clear", got)
		}
	})
	t.Run("group without items is not stale", func(t *testing.T) {
		empty := &Queue{}
		empty.Sort(nil, r.stores.Sprites)
		dst := newTarget(16, 16)
		r.Draw(dst, empty, &Camera{}, 1)
		if got := pixel(dst, 1, 1); got != blue {
			t.Fatalf("pixel = %v, want Clear, not last frame's sprite", got)
		}
	})
	t.Run("normal group still composites", func(t *testing.T) {
		g.Set(id, GroupOptions{From: 0, To: 0})
		r.SetPost(&PostChain{Passes: []Pass{{Shader: sh["copy"], Extra: [3]TargetID{TargetGroup(id)}}}})
		if got := pixel(drawOnGreen(r, q), 1, 1); got != red {
			t.Fatalf("sprite pixel = %v, want red", got)
		}
	})
}
