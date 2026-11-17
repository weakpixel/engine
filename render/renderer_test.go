package render

import (
	"image"
	"image/color"
	"math"
	"testing"
	"testing/fstest"
	"time"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/weakpixel/engine/gfx"
)

func TestLayoutSize(t *testing.T) {
	tests := []struct {
		name         string
		l            Layout
		outW, outH   int
		wantW, wantH int
	}{
		{"fixed", Layout{Width: 320, Height: 180}, 1920, 1080, 320, 180},
		{"integer 6x", Layout{Width: 320, Height: 180, Integer: true}, 1920, 1080, 320, 180},
		{"integer fills width", Layout{Width: 320, Height: 180, Integer: true}, 2000, 1080, 333, 180},
		{"integer limited by height", Layout{Width: 320, Height: 180, Integer: true}, 1920, 600, 640, 200},
		{"window smaller than logical", Layout{Width: 320, Height: 180, Integer: true}, 200, 100, 200, 100},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			w, h := tc.l.Size(tc.outW, tc.outH)
			if w != tc.wantW || h != tc.wantH {
				t.Fatalf("Size = %dx%d, want %dx%d", w, h, tc.wantW, tc.wantH)
			}
		})
	}
}

func TestGeoM(t *testing.T) {
	apply := func(g ebiten.GeoM, x, y float64) (float64, float64) { return g.Apply(x, y) }
	var g ebiten.GeoM
	// 10x20 image, pivot at the center (shift -5,-10), placed at (100,100), no rotation.
	geoM(&g, gfx.Vec2{X: -5, Y: -10}, false, false, transform{pos: gfx.Vec2{X: 100, Y: 100}, scale: gfx.Vec2{X: 1, Y: 1}})
	if x, y := apply(g, 5, 10); x != 100 || y != 100 {
		t.Fatalf("pivot maps to %v,%v, want 100,100", x, y)
	}
	if x, y := apply(g, 0, 0); x != 95 || y != 90 {
		t.Fatalf("top-left maps to %v,%v, want 95,90", x, y)
	}
	// Flip X mirrors around the pivot.
	geoM(&g, gfx.Vec2{X: -5, Y: -10}, true, false, transform{pos: gfx.Vec2{X: 100, Y: 100}, scale: gfx.Vec2{X: 2, Y: 2}})
	if x, y := apply(g, 0, 0); x != 110 || y != 80 {
		t.Fatalf("flipped, scaled top-left maps to %v,%v, want 110,80", x, y)
	}
	// A quarter turn clockwise on screen: image +x points down.
	geoM(&g, gfx.Vec2{}, false, false, transform{rot: math.Pi / 2, scale: gfx.Vec2{X: 1, Y: 1}})
	if x, y := apply(g, 10, 0); math.Abs(x) > 1e-5 || math.Abs(y-10) > 1e-5 {
		t.Fatalf("rotated +x maps to %v,%v, want 0,10", x, y)
	}
}

func TestSpriteShift(t *testing.T) {
	// A 10x20 frame whose opaque part (6x12) starts at (2,4): trimmed.
	trimmed := Sprite{Size: gfx.Vec2{X: 10, Y: 20}, Offset: gfx.Vec2{X: 2, Y: 4}, Origin: gfx.Vec2{X: 0.5, Y: 0.5}}
	plain := Sprite{Size: gfx.Vec2{X: 10, Y: 20}, Origin: gfx.Vec2{X: 0.5, Y: 0.5}}
	tests := []struct {
		name   string
		sp     Sprite
		origin gfx.Vec2
		want   gfx.Vec2
	}{
		{"zero origin uses the sprite default", plain, gfx.Vec2{}, gfx.Vec2{X: -5, Y: -10}},
		{"explicit origin overrides it", plain, gfx.Vec2{X: 1, Y: 1}, gfx.Vec2{X: -10, Y: -20}},
		{"tiny origin is top-left", plain, gfx.Vec2{X: 1e-9, Y: 1e-9}, gfx.Vec2{X: -1e-8, Y: -2e-8}},
		{"trim moves the image by Offset", trimmed, gfx.Vec2{}, gfx.Vec2{X: -3, Y: -6}},
		{"trim with explicit origin", trimmed, gfx.Vec2{X: 0.2, Y: 0.5}, gfx.Vec2{X: 0, Y: -6}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := spriteShift(&tc.sp, tc.origin)
			if math.Abs(float64(got.X-tc.want.X)) > 1e-6 || math.Abs(float64(got.Y-tc.want.Y)) > 1e-6 {
				t.Fatalf("spriteShift = %v, want %v", got, tc.want)
			}
		})
	}

	// The pivot of the untrimmed frame stays at the entity position: the
	// frame's center (5,10) is image point (3,6) of the trimmed image.
	var g ebiten.GeoM
	geoM(&g, spriteShift(&trimmed, gfx.Vec2{}), false, false, transform{pos: gfx.Vec2{X: 50, Y: 60}, scale: gfx.Vec2{X: 2, Y: 2}})
	if x, y := g.Apply(3, 6); x != 50 || y != 60 {
		t.Fatalf("trimmed pivot maps to %v,%v, want 50,60", x, y)
	}
}

func TestToScreen(t *testing.T) {
	cam := Camera{Pos: gfx.Vec2{X: 50, Y: 50}, Zoom: 2}
	got := transform{pos: gfx.Vec2{X: 60, Y: 50}, scale: gfx.Vec2{X: 1, Y: 1}}.toScreen(&cam, image.Pt(200, 100))
	if got.pos != (gfx.Vec2{X: 120, Y: 50}) || got.scale != (gfx.Vec2{X: 2, Y: 2}) {
		t.Fatalf("toScreen = %+v", got)
	}
}

// newTarget returns an offscreen image to draw on.
func newTarget(w, h int) *ebiten.Image { return ebiten.NewImage(w, h) }

// topLeft is the smallest practical Item.Origin that is not the zero "use the
// sprite default": pixel tests use it to place a sprite by its top-left corner.
var topLeft = gfx.Vec2{X: 1e-6, Y: 1e-6}

// testRenderer returns a Renderer over a real asset server and one 4x4 red
// sprite.
func testRenderer(t testing.TB) (*Renderer, gfx.SpriteHandle) {
	t.Helper()
	e, sprites := testSprites(t, 1)
	return NewRenderer(StoresOf(e.srv)), sprites[0]
}

// TestDrawPlaceholders checks that zero, never-loaded and failed handles draw
// instead of being skipped, each as one draw call, and count in
// Stats.Placeholders while a ready sprite does not.
func TestDrawPlaceholders(t *testing.T) {
	files := fstest.MapFS{}
	e := newAssetEnv(t, files, 0)
	ready := e.loadSheet(files, "s", 1)[0]
	sc := e.srv.NewScope("bad")
	failed := LoadSprite(sc, "missing.png")
	files["late.png"] = pngFile(t, 4, 4, color.RGBA{G: 255, A: 255})
	e.srv.Update()
	loading := LoadSprite(sc, "late.png") // requested, committed by a later Update at the earliest
	r := NewRenderer(StoresOf(e.srv))
	if r.stores.Sprites.Ready(loading) {
		t.Fatal("late.png is ready before any Update")
	}

	var q Queue
	q.Sprite(0, gfx.SpriteHandle{}, gfx.Vec2{}, gfx.Vec2{})
	q.Sprite(0, failed, gfx.Vec2{}, gfx.Vec2{})
	q.Sprite(0, loading, gfx.Vec2{}, gfx.Vec2{})
	q.Sprite(0, ready, gfx.Vec2{}, gfx.Vec2{})
	q.AddCulled(5)
	q.Sort(nil, r.stores.Sprites)
	r.Draw(newTarget(16, 16), &q, &Camera{}, 1)
	// The placeholders share one texture and batch; the ready sprite has its own.
	if st := r.Stats(); st.Items != 4 || st.DrawCalls != 2 || st.Placeholders != 3 || st.Culled != 5 {
		t.Fatalf("Stats = %+v, want 4 items, 2 draw calls, 3 placeholders, 5 culled", st)
	}
}

// TestDrawNoAllocs checks that Draw adds no allocations of its own. Outside
// a running game Ebiten's DrawImage allocates by itself (it queues commands
// that are only flushed at the end of a frame), so the test compares with the
// cost of the same number of bare DrawImage calls instead of with zero.
func TestDrawNoAllocs(t *testing.T) {
	if testing.CoverMode() != "" || raceEnabled {
		t.Skip("instrumentation allocates")
	}
	r, id := testRenderer(t)
	dst := newTarget(256, 256)
	var q Queue
	for i := range 1000 {
		it := q.Sprite(int16(i%3), id, gfx.Vec2{X: float32(i % 200), Y: float32(i % 100)}, gfx.Vec2{X: float32(i%200) - 1, Y: float32(i % 100)})
		it.Rot = 0.3
		it.Color = gfx.RGBA(200, 200, 255, 200)
	}
	q.Push(Item{Kind: KindRect, Scale: gfx.Vec2{X: 1, Y: 1}, Size: gfx.Vec2{X: 20, Y: 10}})
	q.Push(Item{Kind: KindCircle, Scale: gfx.Vec2{X: 1, Y: 1}, Size: gfx.Vec2{X: 20}})
	for i := range 20 { // nine-slice panels, stretched and tiled
		q.Push(Item{Kind: KindNineSlice, Sprite: id, Scale: gfx.Vec2{X: 1, Y: 1}, Size: gfx.Vec2{X: 40, Y: 20},
			Insets: gfx.Insets{Left: 1, Top: 1, Right: 1, Bottom: 1}, Tile: i%2 == 0, Layer: 1})
	}
	q.Sort(nil, r.stores.Sprites)

	src := ebiten.NewImage(4, 4)
	var opts ebiten.DrawImageOptions
	dst.DrawImage(src, &opts)
	perCall := testing.AllocsPerRun(100, func() { dst.DrawImage(src, &opts) })

	for _, cam := range []*Camera{
		{Zoom: 1.5, Rotation: 0.2},
		{Zoom: 1.5, Pos: gfx.Vec2{X: 3.3, Y: 1.7}, PrevPos: gfx.Vec2{X: 2.9, Y: 1.1}, PixelSnap: true},
	} {
		r.Draw(dst, &q, cam, 0.5) // warm up
		calls := r.Stats().DrawCalls
		got := testing.AllocsPerRun(10, func() { r.Draw(dst, &q, cam, 0.5) })
		// slack covers growth of Ebiten's own command queue, which does not
		// scale with the number of items.
		const slack = 16
		if limit := perCall*float64(calls) + slack; got > limit {
			t.Fatalf("camera %+v: allocs per Draw = %v for %d draw calls, want at most %v (Ebiten's own %v per call)", *cam, got, calls, limit, perCall)
		}
	}
}

// shadedRenderer returns a Renderer, one 4x4 red sprite and two shader
// handles: flash (loaded, see kageFlash) and a failed one.
func shadedRenderer(t testing.TB) (r *Renderer, sprite gfx.SpriteHandle, flash, failed gfx.ShaderHandle) {
	t.Helper()
	files := fstest.MapFS{"fx/flash.kage": textFile(kageFlash)}
	e := newAssetEnv(t, files, 0)
	sprite = e.loadSheet(files, "s", 1)[0]
	sc := e.srv.NewScope("fx")
	flash = LoadShader(sc, "fx/flash.kage")
	failed = LoadShader(sc, "fx/missing.kage")
	e.settle(sc)
	return NewRenderer(StoresOf(e.srv)), sprite, flash, failed
}

// kageFlash blends the sprite towards white by Params.x, keeping its alpha
// and the item's tint.
const kageFlash = `//kage:unit pixels

package main

var Params vec4

func Fragment(dst vec4, src vec2, color vec4) vec4 {
	c := imageSrc0At(src)
	return mix(c, vec4(c.a), Params.x) * color
}
`

// TestDrawShaderStats checks which sprites go through their shader: only
// those whose shader is loaded; zero and failed handles draw normally.
func TestDrawShaderStats(t *testing.T) {
	r, sprite, flash, failed := shadedRenderer(t)
	var q Queue
	q.Sprite(0, sprite, gfx.Vec2{}, gfx.Vec2{}).Shader = flash
	q.Sprite(0, sprite, gfx.Vec2{}, gfx.Vec2{}).Shader = failed
	q.Sprite(0, sprite, gfx.Vec2{}, gfx.Vec2{})
	q.Push(Item{Kind: KindRect, Shader: flash, Scale: gfx.Vec2{X: 1, Y: 1}, Size: gfx.Vec2{X: 4, Y: 4}})
	r.Draw(newTarget(16, 16), &q, &Camera{}, 1)
	// The shaded sprite, then one batch of the two unshaded sprites, then
	// the rect: its four edges into the scratch image and that as one more
	// shaded quad.
	if st := r.Stats(); st.Items != 4 || st.DrawCalls != 4 || st.Quads != 2+4 || st.ShadedQuads != 2 || st.ShaderDraws != 2 {
		t.Fatalf("Stats = %+v, want 4 items, 4 draw calls, 6 quads, 2 shaded quads, 2 shader draws", st)
	}
}

// TestDrawShadedBatches checks which consecutive shaded sprites share a draw
// call: those with the same frame, Params, blend mode and shader. Anything
// else, or an unshaded sprite in between, starts a new one.
func TestDrawShadedBatches(t *testing.T) {
	files := fstest.MapFS{"fx/a.kage": textFile(kageFlash), "fx/b.kage": textFile(kageFlash)}
	e := newAssetEnv(t, files, 0)
	frames := e.loadSheet(files, "s", 2)
	sc := e.srv.NewScope("fx")
	sa, sb := LoadShader(sc, "fx/a.kage"), LoadShader(sc, "fx/b.kage")
	e.settle(sc)
	r := NewRenderer(StoresOf(e.srv))
	type sprite struct {
		frame  int
		shader gfx.ShaderHandle
		params float32
		blend  Blend
	}
	for _, tc := range []struct {
		name    string
		sprites []sprite
		calls   int
	}{
		{"equal", []sprite{{0, sa, 1, 0}, {0, sa, 1, 0}, {0, sa, 1, 0}}, 1},
		{"frame", []sprite{{0, sa, 1, 0}, {1, sa, 1, 0}, {1, sa, 1, 0}}, 2},
		{"params", []sprite{{0, sa, 1, 0}, {0, sa, 0.5, 0}, {0, sa, 0.5, 0}}, 2},
		{"blend", []sprite{{0, sa, 1, 0}, {0, sa, 1, BlendAdditive}}, 2},
		{"shader", []sprite{{0, sa, 1, 0}, {0, sb, 1, 0}}, 2},
		{"unshaded between", []sprite{{0, sa, 1, 0}, {0, gfx.ShaderHandle{}, 0, 0}, {0, sa, 1, 0}}, 3},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var q Queue
			shaded := 0
			for _, s := range tc.sprites {
				it := q.Sprite(0, frames[s.frame], gfx.Vec2{}, gfx.Vec2{})
				it.Shader, it.Params[0], it.Blend = s.shader, s.params, s.blend
				if !s.shader.IsZero() {
					shaded++
				}
			}
			// Not sorted: items draw in push order.
			r.Draw(newTarget(16, 16), &q, &Camera{}, 1)
			st := r.Stats()
			if st.DrawCalls != tc.calls || st.ShadedQuads != shaded || st.ShaderDraws != tc.calls-(len(tc.sprites)-shaded) {
				t.Fatalf("Stats = %+v, want %d draw calls, %d shaded quads", st, tc.calls, shaded)
			}
		})
	}
}

// TestDrawShadedNoAllocs is TestDrawNoAllocs for shaded sprites: the
// Renderer adds nothing to what Ebiten's DrawRectShader costs by itself.
func TestDrawShadedNoAllocs(t *testing.T) {
	if testing.CoverMode() != "" || raceEnabled {
		t.Skip("instrumentation allocates")
	}
	r, sprite, flash, _ := shadedRenderer(t)
	dst := newTarget(256, 256)
	var q Queue
	for i := range 1000 {
		it := q.Sprite(int16(i%3), sprite, gfx.Vec2{X: float32(i % 200), Y: float32(i % 100)}, gfx.Vec2{X: float32(i % 200), Y: float32(i % 100)})
		it.Shader = flash
		it.Params = [4]float32{float32(i%10) / 10, 0, 0, 1}
		it.Rot = 0.3
	}
	q.Sort(nil, r.stores.Sprites)
	cam := &Camera{Zoom: 1.5}
	r.Draw(dst, &q, cam, 0.5) // warm up
	if st := r.Stats(); st.ShaderDraws != 1000 {
		t.Fatalf("ShaderDraws = %d, want 1000", st.ShaderDraws)
	}

	src := ebiten.NewImage(4, 4)
	sh := r.stores.Shaders.Get(flash).Shader
	u, params := newUniforms(make([]float32, 1))
	opts := ebiten.DrawRectShaderOptions{Uniforms: u, Images: [4]*ebiten.Image{src}}
	call := func() {
		params[0] += 0.1
		dst.DrawRectShader(4, 4, sh, &opts)
	}
	call()
	perCall := testing.AllocsPerRun(100, call)

	got := testing.AllocsPerRun(10, func() { r.Draw(dst, &q, cam, 0.5) })
	const slack = 16 // see TestDrawNoAllocs
	if limit := perCall*1000 + slack; got > limit {
		t.Fatalf("allocs per Draw = %v for 1000 shader draws, want at most %v (Ebiten's own %v per call)", got, limit, perCall)
	}
}

// TestDrawShadedCanvasNoAllocs checks that shaded canvases cost no more
// than the plain DrawRectShader call they make.
func TestDrawShadedCanvasNoAllocs(t *testing.T) {
	if testing.CoverMode() != "" || raceEnabled {
		t.Skip("instrumentation allocates")
	}
	r, _, flash, _ := shadedRenderer(t)
	c := NewCanvases(r.stores)
	r.SetCanvases(c)
	id := c.New(4, 4)
	dst := newTarget(64, 64)
	var q Queue
	const n = 100
	for i := range n {
		q.Push(Item{Kind: KindCanvas, Canvas: id, Pos: gfx.Vec2{X: float32(i % 50)}, Scale: gfx.Vec2{X: 1, Y: 1},
			Shader: flash, Params: [4]float32{float32(i%10) / 10}})
	}
	cam := &Camera{}
	r.Draw(dst, &q, cam, 1) // warm up
	if st := r.Stats(); st.ShaderDraws != n {
		t.Fatalf("ShaderDraws = %d, want %d", st.ShaderDraws, n)
	}

	src := ebiten.NewImage(4, 4)
	sh := r.stores.Shaders.Get(flash).Shader
	u, params := newUniforms(make([]float32, 1))
	opts := ebiten.DrawRectShaderOptions{Uniforms: u, Images: [4]*ebiten.Image{src}}
	call := func() {
		params[0] += 0.1
		dst.DrawRectShader(4, 4, sh, &opts)
	}
	call()
	perCall := testing.AllocsPerRun(100, call)
	got := testing.AllocsPerRun(10, func() { r.Draw(dst, &q, cam, 1) })
	if limit := perCall*n + 16; got > limit {
		t.Fatalf("allocs per Draw = %v for %d shaded canvases, want at most %v", got, n, limit)
	}
}

// TestSetTimeReachesEveryUniformMap checks that the sprite, group, post and
// final-pass uniform maps share the Time slice SetTime writes, and that
// SetTime allocates nothing.
func TestSetTimeReachesEveryUniformMap(t *testing.T) {
	r, _ := testRenderer(t)
	r.SetTime(1500 * time.Millisecond)
	for name, u := range map[string]map[string]any{
		"sprite": r.sopts.Uniforms, "group": r.gsopts.Uniforms,
		"pass": r.psopts.Uniforms, "final": r.fsopts.Uniforms,
	} {
		got, _ := u[ShaderUniformTime].([]float32)
		if len(got) != 1 || got[0] != 1.5 {
			t.Errorf("%s uniforms Time = %v, want [1.5]", name, u[ShaderUniformTime])
		}
	}
	if n := testing.AllocsPerRun(100, func() { r.SetTime(2 * time.Second) }); n != 0 {
		t.Fatalf("SetTime allocs = %v, want 0", n)
	}
}
