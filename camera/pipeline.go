package camera

import (
	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/xmath"
)

// step advances the Rig state by one tick of dt seconds and returns the new,
// unclamped follow position. goal is the target's position plus
// Target.Offset, vel the target's velocity. It panics for an unknown Mode.
//
// The goal goes through the look-ahead stage, then the dead-zone stage,
// and the preset's motion moves the follow position toward the result. A
// stage that is off keeps its state neutral (no look offset, the window on
// the goal), so turning it on later starts from rest. A dt of zero or less
// changes nothing.
func (r *Rig) step(goal, vel gfx.Vec2, dt float32) gfx.Vec2 {
	p := r.Mode.preset()
	st := &r.state
	if dt <= 0 {
		return st.follow
	}

	if p.lookAhead || r.LookAhead.On {
		la := &r.LookAhead
		st.look = lookStep(st.look, vel, dt, la.distance(), la.time(), la.minSpeed())
		goal = goal.Add(st.look)
	} else {
		st.look = gfx.Vec2{}
	}

	if p.deadZone || r.DeadZone.On {
		st.window = deadZoneStep(st.window, goal, r.DeadZone.halfSize())
		goal = st.window
	} else {
		st.window = goal
	}

	switch p.motion {
	case motionHard:
		st.follow, st.vel = goal, gfx.Vec2{}
	case motionSmooth:
		st.follow, st.vel = st.follow.Lerp(goal, xmath.ExpFactor(dt, r.Smooth.time())), gfx.Vec2{}
	case motionSpring:
		st.follow = st.follow.Spring(&st.vel, goal, dt, r.Spring.frequency(), r.Spring.damping())
	}
	return st.follow
}
