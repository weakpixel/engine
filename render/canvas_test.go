package render

import (
	"image"
	"testing"

	"github.com/weakpixel/engine/gfx"
)

func TestCanvasesLifecycle(t *testing.T) {
	e, _ := testSprites(t, 1)
	c := NewCanvases(StoresOf(e.srv))
	a, b := c.New(4, 2), c.New(8, 8)
	if a.IsZero() || b.IsZero() || a == b {
		t.Fatalf("New returned %v and %v", a, b)
	}
	if c.Size(a) != image.Pt(4, 2) || c.Size(b) != image.Pt(8, 8) {
		t.Fatalf("sizes %v, %v", c.Size(a), c.Size(b))
	}
	if c.Len() != 2 || c.Bytes() != (4*2+8*8)*4 {
		t.Fatalf("Len %d, Bytes %d", c.Len(), c.Bytes())
	}
	img := c.image(a)
	c.Free(a)
	c.Free(a) // freeing a stale ID does nothing
	c.Free(CanvasID{})
	if c.Size(a) != (image.Point{}) || c.image(a) != nil {
		t.Fatal("a freed canvas still resolves")
	}
	if c.Len() != 1 || c.Bytes() != 8*8*4 {
		t.Fatalf("after Free: Len %d, Bytes %d", c.Len(), c.Bytes())
	}
	// The slot and the image are reused, the old ID stays stale.
	a2 := c.New(4, 2)
	if a2 == a || c.image(a2) != img {
		t.Fatalf("New after Free: %v (was %v), image reused %v", a2, a, c.image(a2) == img)
	}
	if c.image(a) != nil {
		t.Fatal("the stale ID resolves to the new canvas")
	}
	// A spare image of another size is not reused.
	c.Free(a2)
	if c.image(c.New(2, 4)) == img {
		t.Fatal("a 4x2 image was reused for 2x4")
	}
}

func TestCanvasesPanics(t *testing.T) {
	e, _ := testSprites(t, 1)
	c := NewCanvases(StoresOf(e.srv))
	mustPanic(t, "render: canvas size", func() { c.New(0, 4) })
	mustPanic(t, "render: canvas size", func() { c.New(4, -1) })
	mustPanic(t, "render: stale canvas", func() { c.Paint(CanvasID{}, &Queue{}) })
	id := c.New(2, 2)
	c.Free(id)
	mustPanic(t, "render: stale canvas", func() { c.Paint(id, &Queue{}) })
}

// TestDrawCanvasItems checks that a canvas item is one quad, and that a
// stale canvas or a Renderer without canvases draws nothing.
func TestDrawCanvasItems(t *testing.T) {
	e, sprites := testSprites(t, 1)
	st := StoresOf(e.srv)
	c := NewCanvases(st)
	id := c.New(4, 4)
	var paint Queue
	paint.Sprite(0, sprites[0], gfx.Vec2{X: 2, Y: 2}, gfx.Vec2{X: 2, Y: 2})
	c.Paint(id, &paint)
	stale := c.New(4, 4)
	c.Free(stale)

	var q Queue
	q.Push(Item{Kind: KindCanvas, Canvas: id, Scale: gfx.Vec2{X: 1, Y: 1}})
	q.Push(Item{Kind: KindCanvas, Canvas: stale, Scale: gfx.Vec2{X: 1, Y: 1}})
	r := NewRenderer(st)
	dst := newTarget(16, 16)
	r.Draw(dst, &q, &Camera{}, 1)
	if s := r.Stats(); s.Quads != 0 {
		t.Fatalf("without canvases: %d quads, want 0", s.Quads)
	}
	r.SetCanvases(c)
	r.Draw(dst, &q, &Camera{}, 1)
	if s := r.Stats(); s.Quads != 1 || s.Items != 2 {
		t.Fatalf("%d quads of %d items, want 1 of 2", s.Quads, s.Items)
	}
}

func TestQueueSortCanvases(t *testing.T) {
	e, s := testSprites(t, 2)
	var q Queue
	c1, c2 := CanvasID{index: 1}, CanvasID{index: 2}
	q.Push(Item{Kind: KindSprite, Sprite: s[0]})
	q.Push(Item{Kind: KindCanvas, Canvas: c2})
	q.Push(Item{Kind: KindSprite, Sprite: s[1], Layer: -1})
	q.Push(Item{Kind: KindCanvas, Canvas: c1})
	q.Sort(nil, StoresOf(e.srv).Sprites)
	got := []Item{*q.At(0), *q.At(1), *q.At(2), *q.At(3)}
	if got[0].Sprite != s[1] || got[1].Canvas != c2 || got[2].Canvas != c1 || got[3].Sprite != s[0] {
		t.Fatalf("order %+v; want the layer -1 sprite, then the canvases in insertion order, then the sprite", got)
	}
}
