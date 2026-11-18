package render

import (
	"image"
	"testing"
	"testing/fstest"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/weakpixel/engine/gfx"
)

// TestDrawBatches checks where batches break: on another texture, another
// blend mode, and around items that are not batched (text, shaded sprites).
// Frames of one atlas and all shapes (the white image) share a batch.
func TestDrawBatches(t *testing.T) {
	files := fstest.MapFS{"fx/flash.kage": textFile(kageFlash)}
	e := newAssetEnv(t, files, 0)
	a := e.loadSheet(files, "a", 3)
	b := e.loadSheet(files, "b", 1)[0]
	sc := e.srv.NewScope("fx")
	flash := LoadShader(sc, "fx/flash.kage")
	e.settle(sc)

	sprite := func(id gfx.SpriteHandle) Item {
		return Item{Kind: KindSprite, Sprite: id, Scale: gfx.Vec2{X: 1, Y: 1}}
	}
	shape := func(k Kind) Item {
		return Item{Kind: k, Scale: gfx.Vec2{X: 1, Y: 1}, Size: gfx.Vec2{X: 8, Y: 8}}
	}
	with := func(it Item, f func(*Item)) Item { f(&it); return it }

	tests := []struct {
		name         string
		items        []Item
		noBatch      bool
		calls, quads int
	}{
		{"frames of one atlas", []Item{sprite(a[0]), sprite(a[1]), sprite(a[2])}, false, 1, 3},
		{"texture change", []Item{sprite(a[0]), sprite(b), sprite(a[1])}, false, 3, 3},
		{"blend change", []Item{
			sprite(a[0]),
			with(sprite(a[1]), func(it *Item) { it.Blend = BlendAdditive }),
			sprite(a[2]),
		}, false, 3, 3},
		{"tint and transform do not break", []Item{
			with(sprite(a[0]), func(it *Item) { it.Color = gfx.RGBA(10, 20, 30, 40) }),
			with(sprite(a[1]), func(it *Item) { it.Rot, it.FlipX = 1, true }),
			with(sprite(a[2]), func(it *Item) { it.Space = ScreenSpace }),
		}, false, 1, 3},
		{"text splits", []Item{sprite(a[0]), {Kind: KindText, Text: "x", Scale: gfx.Vec2{X: 1, Y: 1}}, sprite(a[1])}, false, 3, 2},
		{"shaded sprite splits", []Item{
			sprite(a[0]),
			with(sprite(a[1]), func(it *Item) { it.Shader = flash }),
			sprite(a[2]),
		}, false, 3, 2},
		{"rect and line share the white image", []Item{shape(KindRect), shape(KindLine)}, false, 1, 4 + 1},
		{"circle is a path of its own", []Item{shape(KindRect), shape(KindCircle), shape(KindLine)}, false, 3, 4 + 1},
		{"sprite then shape", []Item{sprite(a[0]), shape(KindLine)}, false, 2, 2},
		{"no batching", []Item{sprite(a[0]), sprite(a[1]), shape(KindRect)}, true, 2 + 4, 0},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := NewRenderer(StoresOf(e.srv))
			r.noBatch = tc.noBatch
			var q Queue
			for _, it := range tc.items {
				q.Push(it)
			}
			r.Draw(newTarget(32, 32), &q, &Camera{}, 1)
			if st := r.Stats(); st.DrawCalls != tc.calls || st.Quads != tc.quads {
				t.Fatalf("Stats = %+v, want %d draw calls, %d quads", st, tc.calls, tc.quads)
			}
			if r.batch.dst != nil || r.batch.src != nil || len(r.batch.verts) != 0 {
				t.Fatal("Draw left a pending batch or kept its images")
			}
		})
	}
}

// TestAddQuadVertices checks the corners, source rectangle and color of a
// queued quad: the GeoM of a 4x2 frame at (8, 4) in its texture, scaled by 2
// and moved to (10, 20).
func TestAddQuadVertices(t *testing.T) {
	r := NewRenderer(Stores{})
	dst, src := newTarget(64, 64), newTarget(16, 16)
	var g ebiten.GeoM
	g.Scale(2, 2)
	g.Translate(10, 20)
	var cs ebiten.ColorScale
	cs.Set(0.25, 0.5, 0.75, 1)
	r.addQuad(dst, src, image.Rect(8, 4, 12, 6), &g, &cs, BlendNormal)

	want := []ebiten.Vertex{
		{DstX: 10, DstY: 20, SrcX: 8, SrcY: 4},
		{DstX: 18, DstY: 20, SrcX: 12, SrcY: 4},
		{DstX: 10, DstY: 24, SrcX: 8, SrcY: 6},
		{DstX: 18, DstY: 24, SrcX: 12, SrcY: 6},
	}
	got := r.batch.verts
	if len(got) != len(want) {
		t.Fatalf("%d vertices, want %d", len(got), len(want))
	}
	for i, w := range want {
		w.ColorR, w.ColorG, w.ColorB, w.ColorA = 0.25, 0.5, 0.75, 1
		if got[i] != w {
			t.Errorf("vertex %d = %+v, want %+v", i, got[i], w)
		}
	}
	r.flush()
	if st := r.Stats(); st.DrawCalls != 1 || st.Quads != 1 {
		t.Fatalf("Stats = %+v, want 1 draw call, 1 quad", st)
	}
	if want := []uint32{0, 1, 2, 1, 3, 2}; len(r.batch.indices) != 6 || [6]uint32(r.batch.indices) != [6]uint32(want) {
		t.Fatalf("indices = %v, want %v", r.batch.indices, want)
	}
}

// TestSnapPixel checks the copy of Ebiten's destination snapping: thirds of
// a pixel, away from pixel centers, the same for negative coordinates.
func TestSnapPixel(t *testing.T) {
	tests := []struct{ in, want float32 }{
		{0, 0},
		{2, 2},
		{2.1, 2},
		{2.25, 2 + 5.0/16},
		{2.5, 2 + 11.0/16},
		{2.9, 3},
		{-0.5, -1 + 11.0/16},
		{-1, -1},
		{-1.9, -2},
	}
	for _, tc := range tests {
		if got := snapPixel(tc.in); got != tc.want {
			t.Errorf("snapPixel(%v) = %v, want %v", tc.in, got, tc.want)
		}
	}
}
