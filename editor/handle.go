package editor

import (
	"image/color"
	"math"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/xmath"
)

const (
	// handleLen is the length of a move handle's arrows, in window pixels.
	handleLen = 40
	// handleTolerance is how far from an arrow's line a press still grabs it.
	handleTolerance = 6
	// handleThickness is the drawn width of an arrow line.
	handleThickness = 2
	// rotateRadius is the radius of the rotate ring, knobRadius the reach
	// (and size) of its knob, in window pixels.
	rotateRadius = 56
	knobRadius   = 8
	ringDots     = 32
	// scaleOffset is the scale box's distance from the origin on both axes,
	// scaleHalf its half side.
	scaleOffset = 26
	scaleHalf   = 5
)

var (
	handleXColor = color.RGBA{R: 235, G: 70, B: 70, A: 255}
	handleYColor = color.RGBA{R: 80, G: 200, B: 90, A: 255}
	rotateColor  = color.RGBA{R: 90, G: 150, B: 255, A: 255}
	ringColor    = color.RGBA{R: 90, G: 150, B: 255, A: 120}
	scaleColor   = color.RGBA{R: 230, G: 230, B: 230, A: 255}
)

// handleOrigin returns where the move handle of the single selected object
// stands, in window pixels (its Transform.Pos), and false when no handle is
// shown: nothing or several objects are selected, or the object has no
// Transform.
func (s *Session) handleOrigin() (gfx.Vec2, bool) {
	if len(s.selection) != 1 {
		return gfx.Vec2{}, false
	}
	t := s.transformOf(s.selection[0])
	if t == nil {
		return gfx.Vec2{}, false
	}
	cam := s.e.Camera()
	return s.e.LogicalToWindow(cam.WorldToScreen(t.Pos, s.screenSize())), true
}

// gizmo names a handle of the selected object.
type gizmo uint8

const (
	gizmoMoveX gizmo = iota + 1
	gizmoMoveY
	gizmoRotate
	gizmoScale
)

// rotateKnob returns where the rotate knob stands for an object at window
// position o with rotation rot: on the ring, at the angle of its Rot.
func rotateKnob(o gfx.Vec2, rot float32) gfx.Vec2 {
	return o.Add(xmath.FromAngle(rot).Mul(rotateRadius))
}

// scaleBox returns the center of the scale box for an object at o.
func scaleBox(o gfx.Vec2) gfx.Vec2 { return o.Add(gfx.Vec2{X: scaleOffset, Y: scaleOffset}) }

// pickGizmo returns the gizmo of the single selected object under the
// window pixel p, and false when p is on none. The rotate knob wins over the
// scale box, which wins over the move arrows; the X arrow wins over the Y
// arrow where both are in reach (at the origin).
func (s *Session) pickGizmo(p gfx.Vec2) (gizmo, bool) {
	o, ok := s.handleOrigin()
	if !ok {
		return 0, false
	}
	if p.Sub(rotateKnob(o, s.transformOf(s.selection[0]).Rot)).Len() <= knobRadius {
		return gizmoRotate, true
	}
	if b := p.Sub(scaleBox(o)); xmath.Abs(b.X) <= scaleHalf+handleTolerance/2 && xmath.Abs(b.Y) <= scaleHalf+handleTolerance/2 {
		return gizmoScale, true
	}
	dx := xmath.SegmentDist(p, o, o.Add(gfx.Vec2{X: handleLen}))
	dy := xmath.SegmentDist(p, o, o.Add(gfx.Vec2{Y: handleLen}))
	switch {
	case dx <= handleTolerance && dx <= dy:
		return gizmoMoveX, true
	case dy <= handleTolerance:
		return gizmoMoveY, true
	}
	return 0, false
}

// drawHandle draws the gizmos of the selected object, fixed in window
// pixels: the move arrows (a red one to the right for X, a green one down for
// Y), a ring with the rotate knob at the object's rotation, and the scale
// box on the diagonal.
func (s *Session) drawHandle(screen ebiten.FinalScreen) {
	o, ok := s.handleOrigin()
	if !ok {
		return
	}
	s.fillRect(screen, o.X, o.Y-handleThickness/2, handleLen, handleThickness, handleXColor)
	s.fillRect(screen, o.X+handleLen-3, o.Y-3, 6, 6, handleXColor)
	s.fillRect(screen, o.X-handleThickness/2, o.Y, handleThickness, handleLen, handleYColor)
	s.fillRect(screen, o.X-3, o.Y+handleLen-3, 6, 6, handleYColor)
	for i := range ringDots {
		a := float64(i) * 2 * math.Pi / ringDots
		s.fillRect(screen, o.X+float32(math.Cos(a))*rotateRadius-1, o.Y+float32(math.Sin(a))*rotateRadius-1, 2, 2, ringColor)
	}
	k := rotateKnob(o, s.transformOf(s.selection[0]).Rot)
	s.fillRect(screen, k.X-knobRadius/2, k.Y-knobRadius/2, knobRadius, knobRadius, rotateColor)
	b := scaleBox(o)
	s.fillRect(screen, b.X-scaleHalf, b.Y-scaleHalf, 2*scaleHalf, 2*scaleHalf, scaleColor)
}
