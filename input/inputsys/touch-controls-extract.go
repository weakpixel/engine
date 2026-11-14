package inputsys

import (
	"github.com/weakpixel/ecs"
	"github.com/weakpixel/ecs/system"
	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/input"
	"github.com/weakpixel/engine/render"
)

// TouchLayer is the layer the virtual touch controls are drawn on: above the
// game, below debug shapes.
const TouchLayer int16 = 32766

// TouchControlsExtract draws the touch regions of the player that owns the
// touch screen as screen-space outlines: rectangles and circles, a stick as
// its base plus a knob. It draws nothing while that player's active device
// is not the touch screen, so the controls appear on the first touch and
// disappear when a keyboard or pad is used, unless AlwaysShow is set.
//
// Add it to the render stage after extract.ResetQueue. It needs the
// *render.Queue resource and the *input.Router resource that
// [InputSystem.Init] publishes, so it must be initialized after it.
type TouchControlsExtract struct {
	// Color and PressedColor are the outline colors. Zero means a
	// translucent white and opaque white.
	Color, PressedColor gfx.Color
	// AlwaysShow draws the controls whatever the active device.
	AlwaysShow bool

	queue  *render.Queue
	router *input.Router
}

// NewTouchControlsExtract returns the system.
func NewTouchControlsExtract() *TouchControlsExtract { return &TouchControlsExtract{} }

// Init implements system.Initializer.
func (s *TouchControlsExtract) Init(ctx *system.Context) {
	s.queue = ecs.GetResource[render.Queue](ctx.World)
	s.router = ecs.GetResource[input.Router](ctx.World)
}

// Run implements system.Runner.
func (s *TouchControlsExtract) Run(*system.Context) {
	p := s.touchOwner()
	if p == nil || !s.AlwaysShow && p.ActiveDevice().Class != input.Touch {
		return
	}
	for _, tc := range p.State().Touch() {
		s.draw(&tc)
	}
}

// touchOwner returns the player the touch screen is assigned to: a player
// slot, else the lobby (AssignJoin before anyone joined with touch).
func (s *TouchControlsExtract) touchOwner() *input.Player {
	for i := range s.router.Players() {
		if p := s.router.Player(i); hasDevice(p, input.TouchDevice) {
			return p
		}
	}
	if p := s.router.Lobby(); hasDevice(p, input.TouchDevice) {
		return p
	}
	return nil
}

func hasDevice(p *input.Player, id input.DeviceID) bool {
	for _, d := range p.Devices() {
		if d == id {
			return true
		}
	}
	return false
}

func (s *TouchControlsExtract) draw(tc *input.TouchControl) {
	col := s.Color
	if col == 0 {
		col = gfx.RGBA(255, 255, 255, 96)
	}
	if tc.Pressed {
		col = s.PressedColor
		if col == 0 {
			col = gfx.White
		}
	}
	if tc.Kind == input.TouchStick {
		// A stick is drawn as its base and knob, whatever the shape of the
		// area that catches the touch.
		s.circle(tc.CX, tc.CY, tc.R, col)
		s.circle(tc.KnobX, tc.KnobY, tc.R/2, col)
		return
	}
	if tc.Circle {
		s.circle(tc.CX, tc.CY, tc.R, col)
		return
	}
	pos := gfx.Vec2{X: tc.X0, Y: tc.Y0}
	s.queue.Push(render.Item{
		Kind: render.KindRect, Space: render.ScreenSpace, Layer: TouchLayer,
		Pos: pos, PrevPos: pos, Scale: gfx.Vec2{X: 1, Y: 1},
		Size: gfx.Vec2{X: tc.X1 - tc.X0, Y: tc.Y1 - tc.Y0}, Color: col,
	})
}

func (s *TouchControlsExtract) circle(x, y, r float32, col gfx.Color) {
	c := gfx.Vec2{X: x, Y: y}
	s.queue.Push(render.Item{
		Kind: render.KindCircle, Space: render.ScreenSpace, Layer: TouchLayer,
		Pos: c, PrevPos: c, Scale: gfx.Vec2{X: 1, Y: 1}, Size: gfx.Vec2{X: r}, Color: col,
	})
}
