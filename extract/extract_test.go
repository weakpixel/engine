package extract_test

import (
	"bytes"
	"image"
	"image/png"
	"slices"
	"testing"
	"testing/fstest"
	"time"

	"github.com/weakpixel/asset"
	"github.com/weakpixel/ecs"
	"github.com/weakpixel/ecs/system"
	"github.com/weakpixel/engine/collision"
	"github.com/weakpixel/engine/extract"
	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/hierarchy"
	"github.com/weakpixel/engine/render"
)

// scene is a World with the resources and the schedule of a game's render
// stage.
type scene struct {
	w     *ecs.World
	sched system.Schedule
	queue *render.Queue
	cam   *render.Camera
	sprs  *extract.SpriteExtract
	move  func(*system.Context)
	srv   *asset.Server
	files fstest.MapFS
	// sprites are nine 4x4 frames of one texture, loaded before the first
	// tick.
	sprites []gfx.SpriteHandle
}

// sheetFiles returns a grid sheet of n 4x4 frames and its PNG.
func sheetFiles(n int) fstest.MapFS {
	img := image.NewRGBA(image.Rect(0, 0, 4*n, 4))
	for i := range img.Pix {
		img.Pix[i] = 255
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		panic(err)
	}
	return fstest.MapFS{
		"s.sheet.json": {Data: []byte(`{"image":"s.png","frameW":4,"frameH":4}`)},
		"s.png":        {Data: buf.Bytes()},
	}
}

// h returns the component handle of the n-th test sprite (1-based, so that the
// zero handle stays "none").
func (s *scene) h(n int) gfx.SpriteHandle { return s.sprites[n-1] }

func newScene(t testing.TB) *scene {
	s := &scene{w: ecs.NewWorld(), queue: &render.Queue{}, cam: &render.Camera{Zoom: 1}, files: sheetFiles(9)}
	s.srv = asset.NewServer(asset.Options{FS: s.files})
	t.Cleanup(s.srv.Close)
	render.RegisterLoaders(s.srv, render.LoaderOptions{})
	ecs.SetResource(s.w, s.srv)
	stores := render.StoresOf(s.srv)
	ecs.SetResource(s.w, &stores)
	atlas, err := render.LoadAtlasSync(s.srv.Global(), "s.sheet.json")
	if err != nil {
		t.Fatal(err)
	}
	s.sprites = render.AtlasStoreOf(s.srv).Get(atlas).Order
	ecs.SetResource(s.w, s.queue)
	ecs.SetResource(s.w, s.cam)
	ecs.SetResource(s.w, &extract.Screen{Size: image.Pt(100, 100)})
	layers := &render.LayerConfig{}
	layers.Set(1, render.LayerOptions{YSort: true})
	ecs.SetResource(s.w, layers)

	s.sprs = extract.NewSpriteExtract(s.w)
	s.sched.AddStage("assets")
	s.sched.AddRunner(extract.NewAssetSystem(s.w))
	s.sched.AddStage("pre", extract.NewSnapshotTransforms(s.w).Run)
	s.sched.AddStage("simulate", func(ctx *system.Context) {
		if s.move != nil {
			s.move(ctx)
		}
	})
	s.sched.AddStage("hierarchy", hierarchy.NewSystem(s.w).Run)
	s.sched.AddStage("render-prepare", extract.ResetQueue)
	s.sched.AddRunner(s.sprs, extract.NewNineSliceExtract(s.w), extract.NewScreenSpriteExtract(s.w), extract.NewScreenNineSliceExtract(s.w), extract.NewLabelExtract(s.w), extract.NewDebugExtract(s.w))
	s.sched.AddRunner(extract.NewSortQueue(s.w))
	return s
}

func (s *scene) tick() { s.sched.Run(s.w, time.Second/60) }

func (s *scene) spawn(pos gfx.Vec2, sp gfx.Sprite) ecs.Entity {
	return ecs.NewEntity2(s.w, gfx.Transform{Pos: pos, PrevPos: pos}, sp)
}

// Camera at the origin with zoom 1 on a 100x100 screen sees -50..50.

func TestSpriteExtractCullsAndHides(t *testing.T) {
	s := newScene(t)
	s.spawn(gfx.Vec2{X: 0, Y: 0}, gfx.Sprite{ID: s.h(1)})
	s.spawn(gfx.Vec2{X: 49, Y: 49}, gfx.Sprite{ID: s.h(2)})
	s.spawn(gfx.Vec2{X: 500, Y: 0}, gfx.Sprite{ID: s.h(3)})             // culled
	s.spawn(gfx.Vec2{X: 1, Y: 1}, gfx.Sprite{ID: s.h(4), Hidden: true}) // hidden
	s.tick()

	got := map[gfx.SpriteHandle]bool{}
	for i := range s.queue.Len() {
		got[s.queue.At(i).Sprite] = true
	}
	if len(got) != 2 || !got[s.sprites[0]] || !got[s.sprites[1]] {
		t.Fatalf("queued sprites = %v, want 1 and 2", got)
	}
	// Hidden sprites are not culled: only the one outside the view counts.
	if n := s.queue.Culled(); n != 1 {
		t.Fatalf("Culled = %d, want 1", n)
	}
	s.tick() // ResetQueue clears the count, so it does not add up over ticks
	if n := s.queue.Culled(); n != 1 {
		t.Fatalf("Culled after a second tick = %d, want 1", n)
	}
}

// TestSpriteExtractCullsDrawnView checks that a sprite seen only at the
// camera's PrevPos is still queued: the Renderer blends the camera between
// the two positions.
func TestSpriteExtractCullsDrawnView(t *testing.T) {
	s := newScene(t)
	s.spawn(gfx.Vec2{X: -60, Y: 0}, gfx.Sprite{ID: s.h(1)}) // visible at PrevPos only
	s.cam.PrevPos, s.cam.Pos = gfx.Vec2{X: -20}, gfx.Vec2{X: 20}
	s.tick()
	if s.queue.Len() != 1 || s.queue.Culled() != 0 {
		t.Fatalf("queued %d, culled %d; want the sprite queued", s.queue.Len(), s.queue.Culled())
	}
	s.cam.PrevPos = s.cam.Pos
	s.tick()
	if s.queue.Len() != 0 || s.queue.Culled() != 1 {
		t.Fatalf("camera at rest: queued %d, culled %d; want the sprite culled", s.queue.Len(), s.queue.Culled())
	}
}

func TestSpriteExtractMargin(t *testing.T) {
	s := newScene(t)
	s.spawn(gfx.Vec2{X: 60, Y: 0}, gfx.Sprite{ID: s.h(1)})
	s.tick()
	if s.queue.Len() != 0 {
		t.Fatal("sprite outside the view was queued without a margin")
	}
	s.sprs.Margin = 20
	s.tick()
	if s.queue.Len() != 1 {
		t.Fatal("sprite inside the margin was culled")
	}
}

func TestSpriteItemFields(t *testing.T) {
	s := newScene(t)
	// The shader file does not exist: the handle is still forwarded, the
	// renderer draws a failed shader as no shader.
	fx := render.LoadShader(s.srv.Global(), "fx/flash.kage")
	params := [4]float32{0.5, 0, 0, 1}
	e := ecs.NewEntity2(s.w,
		gfx.Transform{Pos: gfx.Vec2{X: 5, Y: 6}, PrevPos: gfx.Vec2{X: 4, Y: 6}, Rot: 0.5},
		gfx.Sprite{ID: s.h(9), Layer: 2, Origin: gfx.Vec2{X: 0.5, Y: 1}, FlipX: true, Tint: gfx.White, Shader: fx, Params: params})
	_ = e
	s.tick()
	it := s.queue.At(0)
	// The snapshot stage copied Pos to PrevPos, so there is no movement
	// to interpolate.
	want := render.Item{
		Kind: render.KindSprite, Layer: 2, SortY: 6, Sprite: s.sprites[8],
		Pos: gfx.Vec2{X: 5, Y: 6}, PrevPos: gfx.Vec2{X: 5, Y: 6}, Rot: 0.5,
		Scale: gfx.Vec2{X: 1, Y: 1}, Origin: gfx.Vec2{X: 0.5, Y: 1}, FlipX: true, Color: gfx.White,
		Shader: fx, Params: params,
	}
	if *it != want {
		t.Fatalf("item = %+v\nwant   %+v", *it, want)
	}
}

func TestSnapshotKeepsPreviousPosition(t *testing.T) {
	s := newScene(t)
	e := s.spawn(gfx.Vec2{X: 0, Y: 0}, gfx.Sprite{ID: s.h(1)})
	s.move = func(ctx *system.Context) { ecs.GetComp[gfx.Transform](s.w, e).Pos.X += 10 }
	s.tick()
	s.tick()
	tr := ecs.GetComp[gfx.Transform](s.w, e)
	if tr.Pos.X != 20 || tr.PrevPos.X != 10 {
		t.Fatalf("Pos/PrevPos = %v/%v, want 20/10", tr.Pos.X, tr.PrevPos.X)
	}
	it := s.queue.At(0)
	if it.Pos.X != 20 || it.PrevPos.X != 10 {
		t.Fatalf("item Pos/PrevPos = %v/%v, want 20/10", it.Pos.X, it.PrevPos.X)
	}
}

func TestQueueOrderByLayerAndY(t *testing.T) {
	s := newScene(t)
	s.spawn(gfx.Vec2{X: 0, Y: 30}, gfx.Sprite{ID: s.h(1), Layer: 1})
	s.spawn(gfx.Vec2{X: 0, Y: 10}, gfx.Sprite{ID: s.h(2), Layer: 1})
	s.spawn(gfx.Vec2{X: 0, Y: 20}, gfx.Sprite{ID: s.h(3), Layer: 0})
	s.spawn(gfx.Vec2{X: 0, Y: 5}, gfx.Sprite{ID: s.h(4), Layer: 0})
	s.tick()
	var got []gfx.SpriteHandle
	for i := range s.queue.Len() {
		got = append(got, s.queue.At(i).Sprite)
	}
	// Layer 0 is not Y-sorted (insertion order, which the archetype keeps
	// in spawn order); layer 1 is sorted by Y.
	want := []gfx.SpriteHandle{s.sprites[2], s.sprites[3], s.sprites[1], s.sprites[0]}
	if len(got) != 4 || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] || got[3] != want[3] {
		t.Fatalf("order = %v, want %v", got, want)
	}
}

func TestOtherExtractors(t *testing.T) {
	s := newScene(t)
	ecs.NewEntity2(s.w, gfx.Transform{Pos: gfx.Vec2{X: 8, Y: 9}}, gfx.ScreenSprite{Sprite: gfx.Sprite{ID: s.h(5), Layer: 3}})
	fx := render.LoadShader(s.srv.Global(), "fx/outline.kage")
	ecs.NewEntity2(s.w, gfx.Transform{Pos: gfx.Vec2{X: 1, Y: 2}}, gfx.Label{Text: "hi", Layer: 4, Shader: fx, Params: [4]float32{1, 2}})
	ecs.NewEntity2(s.w, gfx.Transform{Pos: gfx.Vec2{X: 0, Y: 0}}, gfx.DebugShape{Kind: gfx.ShapeCircle, Size: gfx.Vec2{X: 4}})
	ecs.NewEntity2(s.w, gfx.Transform{Pos: gfx.Vec2{X: 900, Y: 0}}, gfx.DebugShape{}) // culled
	s.tick()

	byKind := map[render.Kind]render.Item{}
	for i := range s.queue.Len() {
		byKind[s.queue.At(i).Kind] = *s.queue.At(i)
	}
	if len(byKind) != 3 {
		t.Fatalf("kinds queued = %v, want sprite, text, circle", byKind)
	}
	if n := s.queue.Culled(); n != 1 {
		t.Fatalf("Culled = %d, want 1 (the far debug shape)", n)
	}
	if it := byKind[render.KindSprite]; it.Space != render.ScreenSpace || it.Sprite != s.sprites[4] || it.Pos.X != 8 {
		t.Fatalf("screen sprite item = %+v", it)
	}
	if it := byKind[render.KindText]; it.Text != "hi" || it.Space != render.ScreenSpace || it.Layer != 4 || it.Shader != fx || it.Params != [4]float32{1, 2} {
		t.Fatalf("label item = %+v", it)
	}
	if it := byKind[render.KindCircle]; it.Space != render.WorldSpace || it.Layer != extract.DebugLayer || it.Size.X != 4 {
		t.Fatalf("debug item = %+v", it)
	}
}

func TestDebugExtractColliders(t *testing.T) {
	s := newScene(t)
	sys := collision.NewSystem(s.w, collision.Config{})
	s.w.NewEntity(gfx.Transform{Pos: gfx.Vec2{X: 10, Y: 5}}, collision.Collider{Offset: gfx.Vec2{X: -4, Y: -13}, Size: gfx.Vec2{X: 8, Y: 13}, Layer: collision.Layer(1)})
	s.w.NewEntity(gfx.Transform{Pos: gfx.Vec2{X: -10}}, collision.Collider{Shape: collision.ShapeCircle, Size: gfx.Vec2{X: 3}, Static: true})
	s.w.NewEntity(gfx.Transform{Pos: gfx.Vec2{X: 900}}, collision.Collider{Size: gfx.Vec2{X: 8, Y: 8}}) // culled
	sys.Run(nil)
	s.tick()

	byKind := map[render.Kind]render.Item{}
	for i := range s.queue.Len() {
		byKind[s.queue.At(i).Kind] = *s.queue.At(i)
	}
	r, c := byKind[render.KindRect], byKind[render.KindCircle]
	if len(byKind) != 2 || s.queue.Culled() != 1 {
		t.Fatalf("queued %v, culled %d; want a rect and a circle, one culled", byKind, s.queue.Culled())
	}
	if r.Pos != (gfx.Vec2{X: 6, Y: -8}) || r.Size != (gfx.Vec2{X: 8, Y: 13}) || r.Layer != extract.DebugLayer || r.Color != collision.ColliderColor(collision.Layer(1)) {
		t.Errorf("rect item = %+v", r)
	}
	if c.Pos != (gfx.Vec2{X: -10}) || c.Size.X != 3 || c.Color != collision.ColliderColor(collision.DefaultLayer) {
		t.Errorf("circle item = %+v", c)
	}
}

func TestTickNoAllocs(t *testing.T) {
	if testing.CoverMode() != "" || raceEnabled {
		t.Skip("instrumentation allocates")
	}
	s := newScene(t)
	for i := range 1000 {
		s.spawn(gfx.Vec2{X: float32(i%90) - 45, Y: float32(i % 70)}, gfx.Sprite{ID: s.h(i%5 + 1), Layer: int16(i % 3)})
	}
	s.tick() // warm up: Init, queue capacity, sort buffers
	if n := testing.AllocsPerRun(20, s.tick); n != 0 {
		t.Fatalf("allocs per tick = %v, want 0", n)
	}
}

// TestZeroHandleDraws checks that a sprite with no asset is still queued, so
// the renderer can draw its placeholder.
func TestZeroHandleDraws(t *testing.T) {
	s := newScene(t)
	s.spawn(gfx.Vec2{}, gfx.Sprite{})
	s.tick()
	if s.queue.Len() != 1 || !s.queue.At(0).Sprite.IsZero() {
		t.Fatalf("queue = %d items, want one with the zero handle", s.queue.Len())
	}
}

// TestAssetSystem checks that the system advances the server, so that a
// loading asset becomes Ready after some ticks, and publishes the events.
func TestAssetSystem(t *testing.T) {
	s := newScene(t)
	s.files["extra.png"] = s.files["s.png"]
	h := render.LoadSprite(s.srv.Global(), "extra.png")
	sprites := render.StoresOf(s.srv).Sprites
	if sprites.Ready(h) {
		t.Fatal("asset is ready before any tick")
	}
	for range 100 {
		s.tick()
		if sprites.Ready(h) {
			break
		}
	}
	if !sprites.Ready(h) {
		t.Fatalf("asset not ready after ticks: %v", sprites.Err(h))
	}
	ev := ecs.GetResourceOrNil[asset.Events](s.w)
	if ev == nil {
		t.Fatal("*asset.Events resource was not published")
	}
}

// TestSpriteBlendReachesItem checks that the sprite's Blend is copied to the
// queued item.
func TestSpriteBlendReachesItem(t *testing.T) {
	s := newScene(t)
	s.spawn(gfx.Vec2{}, gfx.Sprite{ID: s.h(1), Blend: gfx.BlendLighten})
	s.tick()
	if s.queue.Len() != 1 || s.queue.At(0).Blend != render.BlendLighten {
		t.Fatalf("item blend = %v, want BlendLighten", s.queue.At(0).Blend)
	}
}

func TestSortQueueBatchesTextures(t *testing.T) {
	s := newScene(t)
	s.spawn(gfx.Vec2{}, gfx.Sprite{})           // placeholder
	s.spawn(gfx.Vec2{}, gfx.Sprite{ID: s.h(1)}) // real texture
	s.tick()
	if s.queue.Len() != 2 || !s.queue.At(0).Sprite.IsZero() {
		t.Fatal("placeholder did not sort before the real texture")
	}
}

// TestGfxBlendMatchesRender guards the gfx.Blend mirror: extract systems
// convert with render.Blend(b), so every constant must equal its render
// counterpart.
func TestGfxBlendMatchesRender(t *testing.T) {
	for _, tc := range []struct {
		name string
		gfx  gfx.Blend
		want render.Blend
	}{
		{"Normal", gfx.BlendNormal, render.BlendNormal},
		{"Additive", gfx.BlendAdditive, render.BlendAdditive},
		{"Multiply", gfx.BlendMultiply, render.BlendMultiply},
		{"Screen", gfx.BlendScreen, render.BlendScreen},
		{"Subtract", gfx.BlendSubtract, render.BlendSubtract},
		{"Lighten", gfx.BlendLighten, render.BlendLighten},
		{"Erase", gfx.BlendErase, render.BlendErase},
	} {
		if got := render.Blend(tc.gfx); got != tc.want {
			t.Errorf("gfx.Blend%s = %d, render.Blend%s = %d", tc.name, got, tc.name, tc.want)
		}
	}
}

func TestNineSliceItemFields(t *testing.T) {
	s := newScene(t)
	fx := render.LoadShader(s.srv.Global(), "fx/outline.kage")
	ecs.NewEntity2(s.w,
		gfx.Transform{Pos: gfx.Vec2{X: 5, Y: 6}, Rot: 0.5},
		gfx.NineSlice{ID: s.h(3), Insets: gfx.Insets{Left: 1, Top: 2, Right: 3, Bottom: 4}, Size: gfx.Vec2{X: 20, Y: 10},
			Layer: 2, Origin: gfx.Vec2{X: 0.5, Y: 0.5}, Tint: gfx.White, Blend: gfx.BlendLighten, Tile: true,
			Shader: fx, Params: [4]float32{0.5}})
	ecs.NewEntity2(s.w, gfx.Transform{Pos: gfx.Vec2{X: 1, Y: 1}}, gfx.NineSlice{ID: s.h(1), Hidden: true})
	ecs.NewEntity2(s.w, gfx.Transform{Pos: gfx.Vec2{X: 7, Y: 8}}, gfx.ScreenNineSlice{NineSlice: gfx.NineSlice{ID: s.h(2), Size: gfx.Vec2{X: 9, Y: 9}}})
	s.tick()
	if s.queue.Len() != 2 {
		t.Fatalf("queued %d items, want the panel and the screen panel", s.queue.Len())
	}
	want := render.Item{
		Kind: render.KindNineSlice, Layer: 2, SortY: 6, Sprite: s.h(3),
		Pos: gfx.Vec2{X: 5, Y: 6}, PrevPos: gfx.Vec2{X: 5, Y: 6}, Rot: 0.5, Scale: gfx.Vec2{X: 1, Y: 1},
		Origin: gfx.Vec2{X: 0.5, Y: 0.5}, Size: gfx.Vec2{X: 20, Y: 10}, Insets: gfx.Insets{Left: 1, Top: 2, Right: 3, Bottom: 4},
		Tile: true, Color: gfx.White, Blend: render.BlendLighten,
		Shader: fx, Params: [4]float32{0.5},
	}
	var world, screen *render.Item
	for i := range s.queue.Len() {
		if it := s.queue.At(i); it.Space == render.WorldSpace {
			world = it
		} else {
			screen = it
		}
	}
	if world == nil || *world != want {
		t.Fatalf("world item = %+v\nwant       %+v", world, want)
	}
	if screen == nil || screen.Kind != render.KindNineSlice || screen.Sprite != s.h(2) || screen.Pos.X != 7 || screen.Size.X != 9 {
		t.Fatalf("screen item = %+v", screen)
	}
}

// TestNineSliceExtractCulls checks that culling keeps a panel whose pivot is
// off-screen but whose body reaches into the view, and drops a far one.
func TestNineSliceExtractCulls(t *testing.T) {
	s := newScene(t)
	// The view is -50..50. This panel's pivot is at x=-70, its body 40 wide.
	ecs.NewEntity2(s.w, gfx.Transform{Pos: gfx.Vec2{X: -70}}, gfx.NineSlice{ID: s.h(1), Size: gfx.Vec2{X: 40, Y: 10}})
	ecs.NewEntity2(s.w, gfx.Transform{Pos: gfx.Vec2{X: 500}}, gfx.NineSlice{ID: s.h(2), Size: gfx.Vec2{X: 40, Y: 10}})
	s.tick()
	if s.queue.Len() != 1 || s.queue.At(0).Sprite != s.h(1) || s.queue.Culled() != 1 {
		t.Fatalf("queued %d, culled %d; want the near panel queued and the far one culled", s.queue.Len(), s.queue.Culled())
	}
}

func TestNineSliceTickNoAllocs(t *testing.T) {
	if testing.CoverMode() != "" || raceEnabled {
		t.Skip("instrumentation allocates")
	}
	s := newScene(t)
	for i := range 200 {
		ecs.NewEntity2(s.w, gfx.Transform{Pos: gfx.Vec2{X: float32(i%90) - 45, Y: float32(i % 70)}},
			gfx.NineSlice{ID: s.h(i%5 + 1), Size: gfx.Vec2{X: 20, Y: 10}})
	}
	s.tick() // warm up
	if n := testing.AllocsPerRun(20, s.tick); n != 0 {
		t.Fatalf("allocs per tick = %v, want 0", n)
	}
}

// order returns the sprite handles of the queue in draw order.
func (s *scene) order() []gfx.SpriteHandle {
	out := make([]gfx.SpriteHandle, s.queue.Len())
	for i := range out {
		out[i] = s.queue.At(i).Sprite
	}
	return out
}

func TestChildSortsNextToItsRootInYSortedLayer(t *testing.T) {
	for _, tt := range []struct {
		name string
		side hierarchy.YSortSide
		want []int // sprite numbers in draw order
	}{
		// root 1 at Y=10, child 2 under it (but far below at Y offset 30),
		// actor 3 just above the root (10.01), actor 4 just below (9.99).
		{"in front", hierarchy.YSortInFront, []int{4, 1, 2, 3}},
		{"behind", hierarchy.YSortBehind, []int{4, 2, 1, 3}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			s := newScene(t)
			root := s.spawn(gfx.Vec2{Y: 10}, gfx.Sprite{ID: s.h(1), Layer: 1})
			s.w.NewEntity(gfx.Transform{}, gfx.Sprite{ID: s.h(2)},
				hierarchy.Parent{Entity: root, Local: hierarchy.Local{Pos: gfx.Vec2{Y: 30}, YSortSide: tt.side}})
			s.spawn(gfx.Vec2{Y: 10.01}, gfx.Sprite{ID: s.h(3), Layer: 1})
			s.spawn(gfx.Vec2{Y: 9.99}, gfx.Sprite{ID: s.h(4), Layer: 1})
			s.tick()
			got := s.order()
			want := make([]gfx.SpriteHandle, len(tt.want))
			for i, n := range tt.want {
				want[i] = s.h(n)
			}
			if !slices.Equal(got, want) {
				t.Fatalf("draw order %v, want %v", got, want)
			}
		})
	}
}

func TestNineSliceChildUsesRootY(t *testing.T) {
	s := newScene(t)
	root := s.spawn(gfx.Vec2{Y: 10}, gfx.Sprite{ID: s.h(1), Layer: 1})
	s.w.NewEntity(gfx.Transform{}, gfx.NineSlice{ID: s.h(2), Size: gfx.Vec2{X: 9, Y: 9}},
		hierarchy.Parent{Entity: root, Local: hierarchy.Local{Pos: gfx.Vec2{Y: 20}, YSortSide: hierarchy.YSortBehind}})
	s.tick()
	for i := range s.queue.Len() {
		if it := s.queue.At(i); it.Kind == render.KindNineSlice {
			if want := float32(10 - 1.0/1024); it.SortY != want {
				t.Fatalf("SortY = %v, want %v", it.SortY, want)
			}
			return
		}
	}
	t.Fatal("no nine-slice queued")
}
