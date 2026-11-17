package render

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
	"testing/fstest"

	"github.com/hajimehoshi/ebiten/v2"
	"golang.org/x/image/font/gofont/goregular"

	"github.com/weakpixel/engine/gfx"
)

func TestScratchSide(t *testing.T) {
	tests := []struct{ have, need, want int }{
		{0, 1, 64},
		{0, 64, 64},
		{0, 65, 128},
		{128, 10, 128},
		{128, 300, 512},
	}
	for _, tc := range tests {
		if got := scratchSide(tc.have, tc.need); got != tc.want {
			t.Errorf("scratchSide(%d, %d) = %d, want %d", tc.have, tc.need, got, tc.want)
		}
	}
}

func TestScratchRegion(t *testing.T) {
	const m = 2 * shadeMargin
	tests := []struct {
		name string
		size gfx.Vec2
		want image.Point
		ok   bool
	}{
		{"whole pixels", gfx.Vec2{X: 10, Y: 4}, image.Pt(10+m, 4+m), true},
		{"rounds up", gfx.Vec2{X: 10.2, Y: 3.5}, image.Pt(11+m, 4+m), true},
		{"no width", gfx.Vec2{X: 0, Y: 4}, image.Point{}, false},
		{"negative", gfx.Vec2{X: 4, Y: -1}, image.Point{}, false},
		{"largest", gfx.Vec2{X: maxScratchSide - m, Y: 1}, image.Pt(maxScratchSide, 1+m), true},
		{"too large", gfx.Vec2{X: maxScratchSide, Y: 1}, image.Point{}, false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := scratchRegion(tc.size)
			if got != tc.want || ok != tc.ok {
				t.Fatalf("scratchRegion(%v) = %v, %v, want %v, %v", tc.size, got, ok, tc.want, tc.ok)
			}
		})
	}
}

// TestScratchRegionReuse checks that the scratch image only grows and that
// a size used before gets the same region image back.
func TestScratchRegionReuse(t *testing.T) {
	var s shadeScratch
	a := s.region(image.Pt(20, 10))
	img := s.img
	if a.Bounds() != image.Rect(0, 0, 20, 10) {
		t.Fatalf("region bounds = %v", a.Bounds())
	}
	if b := s.region(image.Pt(30, 30)); s.img != img || b == a {
		t.Fatalf("a smaller region reallocated the image or reused another size's region")
	}
	if s.region(image.Pt(20, 10)) != a {
		t.Fatal("the same size got a new region image")
	}
	s.region(image.Pt(100, 10))
	if s.img == img || s.img.Bounds().Size() != image.Pt(128, 64) {
		t.Fatalf("scratch image = %v after a 100 px region, want a new 128x64 one", s.img.Bounds())
	}
	if len(s.regions) != 1 {
		t.Fatalf("%d cached regions after growing, want only the new one", len(s.regions))
	}
}

// kagePass draws the item as it is: the source pixel times the tint.
const kagePass = `//kage:unit pixels

package main

func Fragment(dst vec4, src vec2, color vec4) vec4 {
	return imageSrc0At(src) * color
}
`

// kageTintFill paints its whole rectangle with the tint, also where the source
// is transparent.
const kageTintFill = `//kage:unit pixels

package main

func Fragment(dst vec4, src vec2, color vec4) vec4 {
	return color
}
`

// kindsEnv is a Renderer with what the multi-quad kinds need: a panel image,
// a font and shaders.
type kindsEnv struct {
	r                          *Renderer
	panel                      gfx.SpriteHandle
	font                       gfx.FontHandle
	pass, fill, flash, missing gfx.ShaderHandle
}

// newKindsEnv loads a 6x6 panel image with a 2 px border (red corners, green
// edges, a blue center, and a white pixel at the first center column so that
// tiling is visible), a 12 px font and the test shaders.
func newKindsEnv(t testing.TB) kindsEnv {
	t.Helper()
	red, green, blue, white := color.RGBA{255, 0, 0, 255}, color.RGBA{0, 255, 0, 255}, color.RGBA{0, 0, 255, 255}, color.RGBA{255, 255, 255, 255}
	img := image.NewRGBA(image.Rect(0, 0, 6, 6))
	for y := range 6 {
		for x := range 6 {
			c := blue
			switch inX, inY := x < 2 || x >= 4, y < 2 || y >= 4; {
			case inX && inY:
				c = red
			case inX || inY:
				c = green
			}
			img.SetRGBA(x, y, c)
		}
	}
	img.SetRGBA(2, 2, white)
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	files := fstest.MapFS{
		"panel.png":     {Data: buf.Bytes()},
		"ui.ttf":        {Data: goregular.TTF},
		"fx/pass.kage":  textFile(kagePass),
		"fx/fill.kage":  textFile(kageTintFill),
		"fx/flash.kage": textFile(kageFlash),
	}
	e := newAssetEnv(t, files, 0)
	sc := e.srv.NewScope("ui")
	env := kindsEnv{
		panel:   LoadSprite(sc, "panel.png"),
		font:    LoadFont(sc, "ui.ttf#12"),
		pass:    LoadShader(sc, "fx/pass.kage"),
		fill:    LoadShader(sc, "fx/fill.kage"),
		flash:   LoadShader(sc, "fx/flash.kage"),
		missing: LoadShader(sc, "fx/missing.kage"),
	}
	e.settle(sc)
	env.r = NewRenderer(StoresOf(e.srv))
	return env
}

// kindItems returns one screen-space item of every kind that is shaded
// through the scratch image, at pos.
func (e kindsEnv) kindItems(pos gfx.Vec2) map[string]Item {
	one := gfx.Vec2{X: 1, Y: 1}
	base := Item{Space: ScreenSpace, Pos: pos, PrevPos: pos, Scale: one}
	with := func(f func(*Item)) Item {
		it := base
		f(&it)
		return it
	}
	return map[string]Item{
		"nine-slice": with(func(it *Item) {
			it.Kind, it.Sprite, it.Size = KindNineSlice, e.panel, gfx.Vec2{X: 20, Y: 14}
			it.Insets = gfx.Insets{Left: 2, Top: 2, Right: 2, Bottom: 2}
		}),
		"text":   with(func(it *Item) { it.Kind, it.Text, it.Font = KindText, "Hi", e.font }),
		"rect":   with(func(it *Item) { it.Kind, it.Size = KindRect, gfx.Vec2{X: 12, Y: 8} }),
		"line":   with(func(it *Item) { it.Kind, it.Size = KindLine, gfx.Vec2{X: 10, Y: -6} }),
		"circle": with(func(it *Item) { it.Kind, it.Size = KindCircle, gfx.Vec2{X: 7} }),
	}
}

// TestDrawShadedKindsStats checks that text, nine-slices and shapes go
// through a loaded shader, one shaded quad and one shader draw each, and
// draw normally with a zero or failed handle.
func TestDrawShadedKindsStats(t *testing.T) {
	e := newKindsEnv(t)
	for name, it := range e.kindItems(gfx.Vec2{X: 20, Y: 20}) {
		t.Run(name, func(t *testing.T) {
			tests := []struct {
				name   string
				shader gfx.ShaderHandle
				shaded int
			}{
				{"loaded", e.pass, 1},
				{"zero", gfx.ShaderHandle{}, 0},
				{"failed", e.missing, 0},
			}
			for _, tc := range tests {
				var q Queue
				it.Shader = tc.shader
				q.Push(it)
				e.r.Draw(newTarget(64, 64), &q, &Camera{}, 1)
				if st := e.r.Stats(); st.ShadedQuads != tc.shaded || st.ShaderDraws != tc.shaded {
					t.Errorf("%s shader: %d shaded quads, %d shader draws, want %d each", tc.name, st.ShadedQuads, st.ShaderDraws, tc.shaded)
				}
			}
		})
	}
}

// TestDrawShadedKindsFallBack checks that an item without area, or too
// large for the scratch image, draws without its shader instead of failing.
func TestDrawShadedKindsFallBack(t *testing.T) {
	e := newKindsEnv(t)
	one := gfx.Vec2{X: 1, Y: 1}
	tests := []struct {
		name string
		it   Item
	}{
		{"empty text", Item{Kind: KindText, Font: e.font, Scale: one}},
		{"huge rect", Item{Kind: KindRect, Size: gfx.Vec2{X: 2 * maxScratchSide, Y: 4}, Scale: one}},
		{"flat rect", Item{Kind: KindRect, Size: gfx.Vec2{X: 8}, Scale: one}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var q Queue
			tc.it.Space, tc.it.Shader = ScreenSpace, e.pass
			q.Push(tc.it)
			e.r.Draw(newTarget(64, 64), &q, &Camera{}, 1)
			if st := e.r.Stats(); st.ShadedQuads != 0 {
				t.Fatalf("Stats = %+v, want no shaded quad", st)
			}
		})
	}
}

// TestDrawShadedKindsNoAllocs checks that a shaded nine-slice, shape or
// text costs no allocations of the Renderer's own in steady state: no more
// than the same item unshaded plus what Ebiten's calls for the scratch
// image (a clear, the item's draw call and a shader draw) cost by
// themselves.
func TestDrawShadedKindsNoAllocs(t *testing.T) {
	if testing.CoverMode() != "" || raceEnabled {
		t.Skip("instrumentation allocates")
	}
	e := newKindsEnv(t)
	dst := newTarget(128, 128)
	cam := &Camera{}

	// What Ebiten allocates for the calls the scratch path adds per item.
	src := ebiten.NewImage(8, 8)
	sub := src.SubImage(image.Rect(0, 0, 4, 4)).(*ebiten.Image)
	sh := e.r.stores.Shaders.Get(e.pass).Shader
	u, params := newUniforms(make([]float32, 1))
	sopts := ebiten.DrawTrianglesShaderOptions{Uniforms: u, Images: [4]*ebiten.Image{sub}}
	verts := make([]ebiten.Vertex, 4)
	indices := []uint32{0, 1, 2, 1, 3, 2}
	var topts ebiten.DrawTrianglesOptions
	white := ebiten.NewImage(1, 1)
	// Per item: clear the region, draw the item's quads into it, shade it
	// onto the target. Measured as one sequence, because switching between
	// the two targets has a cost of its own inside Ebiten.
	item := func() {
		sub.Clear()
		sub.DrawTriangles32(verts, indices, white, &topts)
		params[0] += 0.1
		dst.DrawTrianglesShader32(verts, indices, sh, &sopts)
	}
	item()
	// While a game runs, Ebiten allocates two objects for every command
	// that cannot merge with the one before (measured with a memory
	// profile: all of it is in its command queue), and the sequence above
	// can merge one more than Draw's. So allow three unmerged commands.
	perItem := max(testing.AllocsPerRun(100, item), 3*2)

	const n = 20
	for name, it := range e.kindItems(gfx.Vec2{X: 30, Y: 30}) {
		t.Run(name, func(t *testing.T) {
			measure := func(shader gfx.ShaderHandle) float64 {
				var q Queue
				for i := range n {
					it.Shader, it.Rot = shader, 0.3
					it.Params = [4]float32{float32(i) / n}
					q.Push(it)
				}
				e.r.Draw(dst, &q, cam, 1) // warm up
				return testing.AllocsPerRun(10, func() { e.r.Draw(dst, &q, cam, 1) })
			}
			plain, shaded := measure(gfx.ShaderHandle{}), measure(e.pass)
			const slack = 16 // see TestDrawNoAllocs
			// Unshaded, the items of a kind share draw calls; shaded, each
			// one flushes into the scratch image on its own.
			limit := plain + n*perItem + slack
			if name == "text" {
				// Text is the one kind whose drawing allocates (see
				// Renderer.Draw); measuring it and drawing its glyphs onto
				// another target add up to two more per text inside Ebiten.
				limit += n * 2
			}
			if shaded > limit {
				t.Fatalf("allocs per Draw = %v for %d shaded items, %v unshaded (limit %v, Ebiten's own %v per item)", shaded, n, plain, limit, perItem)
			}
			t.Logf("allocs per Draw: %v shaded, %v unshaded; Ebiten per item %v", shaded, plain, perItem)
		})
	}
}
