package render

import (
	"fmt"
	"image"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/weakpixel/engine/gfx"
)

// CanvasID refers to a canvas of a [Canvases]. It is 8 bytes, comparable and
// pointer-free, so it can live in draw items and components; the zero value
// means "none". Freeing a canvas makes its IDs stale.
type CanvasID struct {
	index uint32 // slot in Canvases.slots; 0 is never used
	gen   uint32 // bumped when the slot is freed; detects stale IDs
}

// IsZero reports whether id is the zero CanvasID.
func (id CanvasID) IsZero() bool { return id.index == 0 }

// Canvases owns offscreen images that code outside this package fills by
// painting a [Queue] into them, and then draws with [KindCanvas] items. It
// is how a system pre-renders something that does not change every frame
// (the static tiles of a map chunk) into one image, so later frames queue
// one item instead of hundreds.
//
// Create it once with [NewCanvases], publish it as a World resource and give
// it to the Renderer with [Renderer.SetCanvases]. Like the Queue, it is used
// on the game goroutine only: Ebiten allows drawing into offscreen images
// in Update as well as in Draw.
type Canvases struct {
	stores Stores
	// painter draws queues into canvases. It is created by the first Paint,
	// because NewRenderer allocates an Ebiten image and NewCanvases may run
	// before the game loop.
	painter *Renderer
	slots   []canvasSlot // slots[0] is unused, so the zero CanvasID is "none"
	free    []uint32
	// spare holds images of freed canvases for New to reuse: a system that
	// scrolls through a map frees and creates canvases of one size all the
	// time, and reusing them avoids allocating Ebiten images (and their GPU
	// memory) every time. At most maxSpare are kept.
	spare []*ebiten.Image
	live  int
	bytes int64
	cam   Camera // Paint's camera, see there
}

type canvasSlot struct {
	img  *ebiten.Image // nil while the slot is free
	size image.Point
	gen  uint32
}

// maxSpare bounds the images Canvases keeps for reuse after Free.
const maxSpare = 16

// NewCanvases returns an empty set of canvases whose Paint resolves sprite
// and font handles through st. It allocates no Ebiten image.
func NewCanvases(st Stores) *Canvases {
	return &Canvases{stores: st, slots: make([]canvasSlot, 1)}
}

// New returns a transparent w x h canvas. It reuses the image of a freed
// canvas of the same size when one is kept, and allocates one otherwise.
//
// Panics with "render: canvas size" when w or h is not positive.
func (c *Canvases) New(w, h int) CanvasID {
	if w <= 0 || h <= 0 {
		panic(fmt.Sprintf("render: canvas size %dx%d", w, h))
	}
	img := c.takeSpare(w, h)
	if img == nil {
		img = ebiten.NewImage(w, h)
	}
	var index uint32
	if n := len(c.free); n > 0 {
		index, c.free = c.free[n-1], c.free[:n-1]
	} else {
		c.slots = append(c.slots, canvasSlot{})
		index = uint32(len(c.slots) - 1)
	}
	s := &c.slots[index]
	s.img, s.size = img, image.Pt(w, h)
	c.live++
	c.bytes += int64(w) * int64(h) * 4
	return CanvasID{index: index, gen: s.gen}
}

// takeSpare returns a cleared spare image of exactly w x h, or nil.
func (c *Canvases) takeSpare(w, h int) *ebiten.Image {
	for i, img := range c.spare {
		if b := img.Bounds(); b.Dx() == w && b.Dy() == h {
			last := len(c.spare) - 1
			c.spare[i], c.spare[last] = c.spare[last], nil
			c.spare = c.spare[:last]
			img.Clear()
			return img
		}
	}
	return nil
}

// Free releases canvas id: its image is kept for reuse by New or
// deallocated, and id and its copies become stale. Freeing the zero or a
// stale ID does nothing.
func (c *Canvases) Free(id CanvasID) {
	s := c.slot(id)
	if s == nil {
		return
	}
	if len(c.spare) < maxSpare {
		c.spare = append(c.spare, s.img)
	} else {
		s.img.Deallocate()
	}
	c.live--
	c.bytes -= int64(s.size.X) * int64(s.size.Y) * 4
	s.img, s.size = nil, image.Point{}
	s.gen++
	c.free = append(c.free, id.index)
}

// Paint clears canvas id and draws q into it, in q's order (Paint does not
// sort it). Items are drawn as by [Renderer.Draw] with an identity camera,
// so world and screen space are both canvas pixels, (0, 0) being the
// canvas's top-left corner; interpolation is off (Pos is used). Sprites that
// are not ready draw their placeholder, so paint once they are ready. Paint
// draws right away; it allocates nothing once its painter exists and its
// buffers have grown.
//
// Panics with "render: stale canvas" when id is zero or stale.
func (c *Canvases) Paint(id CanvasID, q *Queue) {
	s := c.slot(id)
	if s == nil {
		panic(fmt.Sprintf("render: stale canvas %d/%d", id.index, id.gen))
	}
	if c.painter == nil {
		c.painter = NewRenderer(c.stores)
	}
	// A camera centered on the canvas maps world pixels to canvas pixels one
	// to one. It is a field so that passing its address does not allocate.
	center := imageCenter(s.size.X, s.size.Y)
	c.cam = Camera{Pos: center, PrevPos: center}
	s.img.Clear()
	c.painter.Draw(s.img, q, &c.cam, 1)
}

// imageCenter returns the center of a w x h image.
func imageCenter(w, h int) gfx.Vec2 { return gfx.Vec2{X: float32(w) / 2, Y: float32(h) / 2} }

// Size returns the size of canvas id in pixels, or the zero Point for the
// zero or a stale ID.
func (c *Canvases) Size(id CanvasID) image.Point {
	if s := c.slot(id); s != nil {
		return s.size
	}
	return image.Point{}
}

// Len returns the number of canvases that are not freed.
func (c *Canvases) Len() int { return c.live }

// Bytes returns the GPU memory estimate (4 bytes per pixel) of the canvases
// that are not freed, not counting the spare images kept for reuse.
func (c *Canvases) Bytes() int64 { return c.bytes }

// image returns the image of canvas id, or nil for the zero or a stale ID.
func (c *Canvases) image(id CanvasID) *ebiten.Image {
	if s := c.slot(id); s != nil {
		return s.img
	}
	return nil
}

func (c *Canvases) slot(id CanvasID) *canvasSlot {
	if c == nil || id.index == 0 || int(id.index) >= len(c.slots) {
		return nil
	}
	s := &c.slots[id.index]
	if s.gen != id.gen || s.img == nil {
		return nil
	}
	return s
}
