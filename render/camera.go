package render

import (
	"image"
	"math"

	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/xmath"
)

// Camera is a plain value: a CameraSystem writes it during Update and the
// Renderer reads a copy in Draw. Pos is the world point shown at the center
// of the viewport.
//
// All methods take the logical screen size in pixels (the size passed to
// Ebiten's Layout), because the camera itself does not know it.
type Camera struct {
	// Pos is the world position at the viewport center for the current
	// tick, PrevPos the one for the previous tick (see At).
	Pos, PrevPos gfx.Vec2
	// Zoom is the scale from world units to pixels. Zero means 1.
	Zoom float32
	// Rotation rotates the view, in radians, clockwise on screen.
	Rotation float32
	// Viewport is the screen region the camera draws into. The zero
	// rectangle means the whole screen.
	Viewport image.Rectangle
	// PixelSnap rounds the camera position so that the world sits on whole
	// logical pixels: viewport center - Pos*Zoom becomes an integer, which
	// moves the view by at most half a pixel per axis. The Renderer rounds
	// once per frame, after blending PrevPos to Pos, and WorldToScreen,
	// ScreenToWorld and WorldBounds round the same way, so picking and
	// culling match the drawn pixels.
	//
	// Use it for pixel art. Without it, a camera at a sub-pixel position
	// under a non-integer Zoom changes, from frame to frame, which texels of
	// a sprite are drawn one pixel wide and which two, and the art shimmers
	// while the camera moves. It is ignored while Rotation is not 0.
	PixelSnap bool
}

// pixelSnapped returns the camera as PixelSnap draws it: with Pos moved by
// at most half a pixel per axis so that the world-to-screen offset
// (viewport center - Pos*Zoom) is a whole number of pixels, and PixelSnap
// cleared, so the result's methods don't round again. Every world point then
// keeps its sub-pixel phase on screen while the camera moves, so each sprite
// is rasterized the same way in every frame and only shifts as a whole.
// Without PixelSnap, or with a rotation, it returns c unchanged.
func (c Camera) pixelSnapped(screen image.Point) Camera {
	if !c.PixelSnap || c.Rotation != 0 {
		return c
	}
	vp := c.ViewportRect(screen)
	z := float64(c.zoom())
	// float64: Pos*Zoom must round-trip exactly enough that the snapped
	// offset lands on the integer, not a hair beside it.
	snap := func(pos float32, lo, hi int) float32 {
		center := float64(lo+hi) / 2
		off := math.Round(center - float64(pos)*z)
		return float32((center - off) / z)
	}
	c.Pos.X = snap(c.Pos.X, vp.Min.X, vp.Max.X)
	c.Pos.Y = snap(c.Pos.Y, vp.Min.Y, vp.Max.Y)
	c.PixelSnap = false
	return c
}

// zoom returns Zoom with the zero value resolved to 1.
func (c *Camera) zoom() float32 {
	if c.Zoom == 0 {
		return 1
	}
	return c.Zoom
}

// ViewportRect returns the pixel region the camera draws into, given the
// screen size.
func (c *Camera) ViewportRect(screen image.Point) image.Rectangle {
	if c.Viewport.Empty() {
		return image.Rectangle{Max: screen}
	}
	return c.Viewport
}

// At returns a copy whose Pos is blended from PrevPos to Pos by alpha in
// [0,1], for drawing between ticks. The copy's PrevPos equals its Pos.
func (c Camera) At(alpha float32) Camera {
	c.Pos = c.PrevPos.Lerp(c.Pos, alpha)
	c.PrevPos = c.Pos
	return c
}

// WorldToScreen converts a world position to logical screen pixels. With
// PixelSnap it uses the rounded position, as the Renderer draws.
func (c *Camera) WorldToScreen(p gfx.Vec2, screen image.Point) gfx.Vec2 {
	if c.PixelSnap {
		v := c.pixelSnapped(screen)
		return v.WorldToScreen(p, screen)
	}
	vp := c.ViewportRect(screen)
	sin, cos := xmath.Sincos(-c.Rotation)
	d := p.Sub(c.Pos)
	z := c.zoom()
	x := (d.X*cos - d.Y*sin) * z
	y := (d.X*sin + d.Y*cos) * z
	return gfx.Vec2{
		X: x + float32(vp.Min.X+vp.Max.X)/2,
		Y: y + float32(vp.Min.Y+vp.Max.Y)/2,
	}
}

// ScreenToWorld converts logical screen pixels to a world position, for
// mouse picking and aiming. It is the inverse of WorldToScreen, PixelSnap
// included.
func (c *Camera) ScreenToWorld(p gfx.Vec2, screen image.Point) gfx.Vec2 {
	if c.PixelSnap {
		v := c.pixelSnapped(screen)
		return v.ScreenToWorld(p, screen)
	}
	vp := c.ViewportRect(screen)
	z := c.zoom()
	x := (p.X - float32(vp.Min.X+vp.Max.X)/2) / z
	y := (p.Y - float32(vp.Min.Y+vp.Max.Y)/2) / z
	sin, cos := xmath.Sincos(c.Rotation)
	return gfx.Vec2{X: x*cos - y*sin + c.Pos.X, Y: x*sin + y*cos + c.Pos.Y}
}

// WorldBounds returns the axis-aligned world rectangle that contains
// everything the camera can see, for culling. With a rotation it is larger
// than the view (it bounds the rotated rectangle). With PixelSnap it is the
// rounded view's rectangle.
func (c *Camera) WorldBounds(screen image.Point) gfx.Rect {
	if c.PixelSnap {
		v := c.pixelSnapped(screen)
		return v.WorldBounds(screen)
	}
	vp := c.ViewportRect(screen)
	corners := [4]gfx.Vec2{
		{X: float32(vp.Min.X), Y: float32(vp.Min.Y)},
		{X: float32(vp.Max.X), Y: float32(vp.Min.Y)},
		{X: float32(vp.Max.X), Y: float32(vp.Max.Y)},
		{X: float32(vp.Min.X), Y: float32(vp.Max.Y)},
	}
	first := c.ScreenToWorld(corners[0], screen)
	r := gfx.Rect{Min: first, Max: first}
	for _, corner := range corners[1:] {
		w := c.ScreenToWorld(corner, screen)
		r.Min.X, r.Min.Y = min(r.Min.X, w.X), min(r.Min.Y, w.Y)
		r.Max.X, r.Max.Y = max(r.Max.X, w.X), max(r.Max.Y, w.Y)
	}
	return r
}

// DrawBounds returns the world rectangle that frames drawn during the
// current tick can show, for culling in extract systems: the union of
// WorldBounds at PrevPos and at Pos. The Renderer blends the view between
// the two, so culling against Pos alone could drop a sliver at the trailing
// screen edge while the camera moves. PixelSnap needs no extra margin:
// rounding is monotonic, so the rounded blended view lies between the
// rounded views at PrevPos and Pos, which WorldBounds already returns.
func (c *Camera) DrawBounds(screen image.Point) gfx.Rect {
	prev := *c
	prev.Pos = c.PrevPos
	a, b := c.WorldBounds(screen), prev.WorldBounds(screen)
	return gfx.Rect{
		Min: gfx.Vec2{X: min(a.Min.X, b.Min.X), Y: min(a.Min.Y, b.Min.Y)},
		Max: gfx.Vec2{X: max(a.Max.X, b.Max.X), Y: max(a.Max.Y, b.Max.Y)},
	}
}
