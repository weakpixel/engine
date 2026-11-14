package editor

import (
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/weakpixel/ecs"
	"github.com/weakpixel/engine/collision"
	"github.com/weakpixel/engine/gfx"
)

const (
	// markerHalf is half the side of the marker of an object without art, in
	// device-independent pixels: the same size as the pick area of a small
	// object.
	markerHalf = minPickPx / 2
	// circleDots is how many dots outline a circle collider.
	circleDots = 48
)

var (
	soundMarkerColor    = color.RGBA{R: 80, G: 230, B: 230, A: 255}
	colliderMarkerColor = color.RGBA{R: 80, G: 160, B: 255, A: 255}
	plainMarkerColor    = color.RGBA{R: 190, G: 190, B: 190, A: 255}
	smallMarkerColor    = color.RGBA{R: 255, G: 255, B: 255, A: 200}
	boxFillColor        = color.RGBA{R: 30, G: 24, B: 5, A: 30}
)

// hasArt reports whether the game draws something for the entity at its
// Transform: a sprite, a panel, a label, a particle effect, a debug shape.
func hasArt(w *ecs.World, e ecs.Entity) bool {
	return ecs.HasComp[gfx.Sprite](w, e) || ecs.HasComp[gfx.NineSlice](w, e) ||
		ecs.HasComp[gfx.DebugShape](w, e) || ecs.HasComp[gfx.Label](w, e) ||
		ecs.HasComp[gfx.Particles](w, e) || ecs.HasComp[gfx.ScreenSprite](w, e) ||
		ecs.HasComp[gfx.ScreenNineSlice](w, e)
}

// drawMarkers draws a small box with a cross at every object that has a
// Transform and no art (a sound emitter, a spawn point, a sensor), so it
// can be seen and picked: cyan for a SoundEmitter, blue for a Collider,
// grey else. An object whose art is smaller on screen than minPickPx gets a
// white box of that size around it instead, which is also the area that
// picks it (see [Session.Pick]).
func (s *Session) drawMarkers(screen ebiten.FinalScreen) {
	w := s.e.World()
	cam, size := s.e.Camera(), s.screenSize()
	scale := float32(deviceScale())
	for _, id := range s.order {
		en := s.objs[id].entity
		if !w.IsAlive(en) {
			continue
		}
		t := ecs.GetCompOrNil[gfx.Transform](w, en)
		if t == nil {
			continue
		}
		if hasArt(w, en) {
			r, ok := s.Bounds(id)
			if !ok {
				continue
			}
			wr := s.windowRect(r)
			if least := minPickPx * scale; wr.Max.X-wr.Min.X < least || wr.Max.Y-wr.Min.Y < least {
				s.strokeRect(screen, atLeast(wr, least), smallMarkerColor)
			}
			continue
		}
		c := plainMarkerColor
		switch {
		case ecs.HasComp[gfx.SoundEmitter](w, en):
			c = soundMarkerColor
		case ecs.HasComp[collision.Collider](w, en):
			c = colliderMarkerColor
		}
		p := s.e.LogicalToWindow(cam.WorldToScreen(t.Pos, size))
		half := float32(math.Round(float64(markerHalf * scale)))
		s.strokeRect(screen, gfx.Rect{Min: p.Sub(gfx.Vec2{X: half, Y: half}), Max: p.Add(gfx.Vec2{X: half, Y: half})}, c)
		s.fillRect(screen, p.X-half+2, p.Y, 2*half-4, 1, c)
		s.fillRect(screen, p.X, p.Y-half+2, 1, 2*half-4, c)
	}
}

// atLeast grows r around its center to at least least on each side.
func atLeast(r gfx.Rect, least float32) gfx.Rect {
	c := gfx.Vec2{X: (r.Min.X + r.Max.X) / 2, Y: (r.Min.Y + r.Max.Y) / 2}
	half := gfx.Vec2{X: max(r.Max.X-r.Min.X, least) / 2, Y: max(r.Max.Y-r.Min.Y, least) / 2}
	return gfx.Rect{Min: c.Sub(half), Max: c.Add(half)}
}

// drawColliders outlines the Collider of every object, colored by its lowest
// layer like the game's debug overlay. The collision Space only holds
// shapes after a run, which edit mode never does, so the shapes are placed
// from the components.
func (s *Session) drawColliders(screen ebiten.FinalScreen) {
	world := s.e.World()
	for _, id := range s.order {
		en := s.objs[id].entity
		if !world.IsAlive(en) {
			continue
		}
		t, c := ecs.GetCompOrNil[gfx.Transform](world, en), ecs.GetCompOrNil[collision.Collider](world, en)
		if t == nil || c == nil {
			continue
		}
		sh, ok := collision.ShapeOf(t, c)
		if !ok {
			continue
		}
		col := toRGBA(collision.ColliderColor(sh.Layer))
		if sh.Kind == collision.ShapeCircle {
			s.drawCircle(screen, sh.Center, sh.Radius, col)
			continue
		}
		wr := s.windowRect(sh.Bounds)
		w, h := wr.Max.X-wr.Min.X, wr.Max.Y-wr.Min.Y
		s.fillRect(screen, wr.Min.X, wr.Min.Y, w, 1, col)
		s.fillRect(screen, wr.Min.X, wr.Max.Y-1, w, 1, col)
		s.fillRect(screen, wr.Min.X, wr.Min.Y, 1, h, col)
		s.fillRect(screen, wr.Max.X-1, wr.Min.Y, 1, h, col)
	}
}

// drawCircle outlines the circle at the world position center with dots.
func (s *Session) drawCircle(screen ebiten.FinalScreen, center gfx.Vec2, radius float32, c color.RGBA) {
	r := s.windowRect(gfx.Rect{Min: center.Sub(gfx.Vec2{X: radius, Y: radius}), Max: center.Add(gfx.Vec2{X: radius, Y: radius})})
	mid := r.Min.Add(r.Max).Mul(0.5)
	rx, ry := (r.Max.X-r.Min.X)/2, (r.Max.Y-r.Min.Y)/2
	for i := range circleDots {
		a := float64(i) * 2 * math.Pi / circleDots
		s.fillRect(screen, mid.X+float32(math.Cos(a))*rx-1, mid.Y+float32(math.Sin(a))*ry-1, 2, 2, c)
	}
}

// drawBox draws the rubber band of a selection box drag.
func (s *Session) drawBox(screen ebiten.FinalScreen) {
	if s.mouse.mode != dragBox {
		return
	}
	wr := s.windowRect(normRect(s.mouse.boxFrom, s.mouse.boxTo))
	w, h := wr.Max.X-wr.Min.X, wr.Max.Y-wr.Min.Y
	s.fillRect(screen, wr.Min.X, wr.Min.Y, w, h, boxFillColor)
	s.fillRect(screen, wr.Min.X, wr.Min.Y, w, 1, selectionColor)
	s.fillRect(screen, wr.Min.X, wr.Max.Y-1, w, 1, selectionColor)
	s.fillRect(screen, wr.Min.X, wr.Min.Y, 1, h, selectionColor)
	s.fillRect(screen, wr.Max.X-1, wr.Min.Y, 1, h, selectionColor)
}

// toRGBA unpacks a gfx.Color (0xRRGGBBAA).
func toRGBA(c gfx.Color) color.RGBA {
	return color.RGBA{R: uint8(c >> 24), G: uint8(c >> 16), B: uint8(c >> 8), A: uint8(c)}
}
