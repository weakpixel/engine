package camera

import (
	"time"

	"github.com/weakpixel/engine/gfx"
)

// Default parameters, used where a Rig field is zero.
const (
	DefaultSmoothTime      = 150 * time.Millisecond
	DefaultSpringFrequency = 2 // Hz
	DefaultSpringDamping   = 1 // critical
	DefaultLookDistance    = 48
	DefaultLookTime        = 300 * time.Millisecond
	DefaultLookMinSpeed    = 1 // world units per second
)

// DefaultDeadZone is the dead-zone half size used when DeadZone.HalfSize is
// zero, in world units.
var DefaultDeadZone = gfx.Vec2{X: 32, Y: 24}

// Rig configures the follow camera and holds its state between ticks. The
// System reads it as the world resource *Rig (game.Engine owns one, see
// Engine.CameraRig); change its fields at any time from any system or tool.
// Every zero field means its documented default, so the zero Rig is a hard
// follow without bounds.
//
// The Rig keeps its own, unclamped follow position: Bounds only shapes the
// camera's Pos, and the next tick continues from the follow position, not
// from Camera.Pos. So offsets that later camera-stage systems add to Pos (a
// shake) do not feed back, and a system that overrides Pos does so for one
// tick only. The built-in shake (Shake, AddTrauma) works the same way.
type Rig struct {
	// Mode is the follow preset; "" means FollowDefault. An unknown mode
	// panics in System.Run.
	Mode FollowMode
	// Smooth is the easing of the smooth motion (modes smooth, lookahead).
	Smooth Smooth
	// DeadZone configures the dead-zone stage (mode deadzone, or On).
	DeadZone DeadZone
	// Spring is the spring motion (mode spring).
	Spring Spring
	// LookAhead configures the look-ahead stage (mode lookahead, or On).
	LookAhead LookAhead
	// Bounds is the world rectangle the view must stay inside. The zero
	// Rect means unbounded. On an axis where Bounds is smaller than the
	// view, the camera is centered on Bounds. Rotation is ignored.
	Bounds gfx.Rect
	// TeleportDistance snaps the camera (see Snap) when the goal is further
	// than this from the follow position, so a teleported target does not
	// make the camera sweep across the level. 0 turns it off.
	TeleportDistance float32
	// Shake configures the screen shake that AddTrauma starts.
	Shake Shake

	state rigState
	shake shakeState
}

// Smooth configures exponential easing: each tick the follow position
// covers 1 - exp(-dt/Time) of the remaining distance, the same total motion
// at any tick rate.
type Smooth struct {
	// Time is the time constant: after Time the camera has covered 63% of
	// a jump. Zero means DefaultSmoothTime.
	Time time.Duration
}

// DeadZone configures the dead-zone stage: a window that moves only when
// the goal leaves it, by just enough to keep the goal on its edge.
type DeadZone struct {
	// On adds the stage to modes other than deadzone.
	On bool
	// HalfSize is half the window size in world units (it does not scale
	// with Zoom). Zero means DefaultDeadZone.
	HalfSize gfx.Vec2
}

// Spring configures a damped spring, solved in closed form so it is stable
// at any tick rate.
type Spring struct {
	// Frequency is the natural frequency in Hz; higher is stiffer. Zero
	// means DefaultSpringFrequency.
	Frequency float32
	// Damping is the damping ratio: 1 is critical (fastest without
	// overshoot), below 1 overshoots, above 1 is sluggish. Zero means
	// DefaultSpringDamping.
	Damping float32
}

// LookAhead configures the look-ahead stage: the goal moves ahead of the
// target in its direction of motion.
type LookAhead struct {
	// On adds the stage to modes other than lookahead.
	On bool
	// Distance is how far ahead, in world units. Zero means
	// DefaultLookDistance.
	Distance float32
	// Time is the easing time constant of the look offset, toward the
	// motion direction and back to zero. Zero means DefaultLookTime.
	Time time.Duration
	// MinSpeed is the target speed, in world units per second, below which
	// the look offset returns to zero. Zero means DefaultLookMinSpeed.
	MinSpeed float32
}

// rigState is what the pipeline carries between ticks.
type rigState struct {
	// follow is the unclamped camera position.
	follow gfx.Vec2
	// window is the dead-zone window center.
	window gfx.Vec2
	// look is the look-ahead offset.
	look gfx.Vec2
	// vel is the spring velocity.
	vel gfx.Vec2
	// snap is set by Snap and cleared by the next tick with a target.
	snap bool
	// hadTarget reports whether the last tick had a target; a tick with a
	// target after one without snaps.
	hadTarget bool
}

// Snap makes the next tick with a target jump straight to it: the follow
// position and the dead-zone window move to the goal, the spring velocity
// and look offset reset, and Camera.PrevPos is set to Pos so the frame does
// not sweep. Call it after a teleport or a level load. The System also snaps
// on its own the first tick it has a target after a tick without one, and
// when TeleportDistance is exceeded.
func (r *Rig) Snap() { r.state.snap = true }

// reset puts the state on goal, as a snap does.
func (r *Rig) reset(goal gfx.Vec2) {
	r.state = rigState{follow: goal, window: goal, hadTarget: true}
}

func (s Smooth) time() float32 { return seconds(s.Time, DefaultSmoothTime) }

func (d DeadZone) halfSize() gfx.Vec2 {
	if d.HalfSize == (gfx.Vec2{}) {
		return DefaultDeadZone
	}
	return gfx.Vec2{X: max(d.HalfSize.X, 0), Y: max(d.HalfSize.Y, 0)}
}

func (s Spring) frequency() float32 { return orDefault(s.Frequency, DefaultSpringFrequency) }
func (s Spring) damping() float32   { return orDefault(s.Damping, DefaultSpringDamping) }

func (l LookAhead) distance() float32 { return orDefault(l.Distance, DefaultLookDistance) }
func (l LookAhead) time() float32     { return seconds(l.Time, DefaultLookTime) }
func (l LookAhead) minSpeed() float32 { return orDefault(l.MinSpeed, DefaultLookMinSpeed) }

// orDefault returns v, or def when v is zero or negative.
func orDefault(v, def float32) float32 {
	if v <= 0 {
		return def
	}
	return v
}

// seconds returns d in seconds, or def when d is zero or negative.
func seconds(d, def time.Duration) float32 {
	if d <= 0 {
		d = def
	}
	return float32(d.Seconds())
}
