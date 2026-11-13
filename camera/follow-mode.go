package camera

import "fmt"

// FollowMode selects how the camera follows its target. Each mode is a
// preset of a motion and the goal stages it turns on (see the package
// documentation and doc/guide/engine/camera.md):
//
//	mode       look-ahead  dead zone  motion
//	default    -           -          hard
//	smooth     -           -          smooth
//	deadzone   -           on         hard
//	spring     -           -          spring
//	lookahead  on          -          smooth
//
// The zero value "" means FollowDefault. Values are plain strings so they
// read well in config files, Lua and logs.
type FollowMode string

// The follow modes.
const (
	// FollowDefault puts the camera on the target every tick.
	FollowDefault FollowMode = "default"
	// FollowSmooth eases toward the target, independent of the tick rate
	// (Rig.Smooth).
	FollowSmooth FollowMode = "smooth"
	// FollowDeadZone leaves the camera still while the target moves inside
	// a rectangle around the view center, and pushes it along when the
	// target reaches an edge (Rig.DeadZone).
	FollowDeadZone FollowMode = "deadzone"
	// FollowSpring follows with a damped spring, which can overshoot when
	// under-damped (Rig.Spring).
	FollowSpring FollowMode = "spring"
	// FollowLookAhead eases toward a point ahead of the target in its
	// direction of motion (Rig.LookAhead, Rig.Smooth).
	FollowLookAhead FollowMode = "lookahead"
)

// String returns the mode name, "default" for the zero value.
func (m FollowMode) String() string {
	if m == "" {
		return string(FollowDefault)
	}
	return string(m)
}

// Check panics with `camera: unknown follow mode "x"` unless m is one of
// the FollowMode constants or the zero value.
func (m FollowMode) Check() { m.preset() }

// motion is how the follow position moves toward the shaped goal.
type motion uint8

const (
	motionHard motion = iota
	motionSmooth
	motionSpring
)

// preset is the pipeline a FollowMode selects.
type preset struct {
	motion    motion
	deadZone  bool
	lookAhead bool
}

// preset resolves m. It panics for an unknown mode; the message is only
// formatted on that path, so the steady state does not allocate.
func (m FollowMode) preset() preset {
	switch m {
	case "", FollowDefault:
		return preset{motion: motionHard}
	case FollowSmooth:
		return preset{motion: motionSmooth}
	case FollowDeadZone:
		return preset{motion: motionHard, deadZone: true}
	case FollowSpring:
		return preset{motion: motionSpring}
	case FollowLookAhead:
		return preset{motion: motionSmooth, lookAhead: true}
	}
	panic(fmt.Sprintf("camera: unknown follow mode %q", string(m)))
}
