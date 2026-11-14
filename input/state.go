package input

import (
	"math"
)

const (
	flagDown uint8 = 1 << iota
	flagPressed
	flagReleased
)

type actionState struct {
	flags    uint8
	held     uint32
	strength float32
	x, y     float32
	progress float32
	// pressedAt is the tick (State.now) of the last press, 0 for none or
	// after ConsumePress.
	pressedAt uint64
}

// State is one player's action values for the current tick. It is written
// by [Router.Update] and read-only to game code; the pointer from
// [Player.State] is stable for the life of the Router, so systems cache it.
//
// Queries are an index and a mask: 0 allocs. Asking for an action of the
// wrong kind panics with `input: action "jump" is Button, not Vec2`; an ID
// that was never declared panics with "input: unknown action". Actions the
// bindings never mention read as zero.
type State struct {
	acts    *Actions
	a       []actionState // index = Action; a[0] is unused
	pointer Pointer
	touch   []TouchControl
	// now numbers the committed ticks from 1, so pressedAt 0 means "no
	// press" without a separate flag.
	now uint64
}

func (s *State) init(acts *Actions) {
	s.acts = acts
	s.a = make([]actionState, acts.Len()+1)
}

func (s *State) get(id Action, want Kind) *actionState {
	if id == 0 || int(id) >= len(s.a) {
		panicf("unknown action %d", id)
	}
	if k := s.acts.list[id-1].kind; k != want {
		panicf("action %q is %s, not %s", s.acts.list[id-1].name, k, want)
	}
	return &s.a[id]
}

// Down reports whether the Button action a is down.
func (s *State) Down(a Action) bool { return s.get(a, Button).flags&flagDown != 0 }

// JustPressed reports whether a went down this tick.
func (s *State) JustPressed(a Action) bool { return s.get(a, Button).flags&flagPressed != 0 }

// JustReleased reports whether a went up this tick. It also fires when a
// mapping that held a down was blocked or popped.
func (s *State) JustReleased(a Action) bool { return s.get(a, Button).flags&flagReleased != 0 }

// HeldTicks returns for how many ticks a has been down, counting this one;
// 0 while it is up.
func (s *State) HeldTicks(a Action) uint32 { return s.get(a, Button).held }

// PressedWithin reports whether a went down in the last ticks ticks,
// counting this one, and that press was not consumed with
// [Player.ConsumePress]. PressedWithin(a, 1) equals JustPressed(a); 0 ticks
// is always false. Use it for input buffering: a Jump pressed a few ticks
// before landing still jumps.
func (s *State) PressedWithin(a Action, ticks uint32) bool {
	st := s.get(a, Button)
	return st.pressedAt != 0 && s.now-st.pressedAt < uint64(ticks)
}

// Strength returns how far the strongest binding of a is pressed, 0..1:
// 1 for digital controls, the trigger or stick value for analog ones.
func (s *State) Strength(a Action) float32 { return s.get(a, Button).strength }

// Progress returns how far a hold interaction of a is, 0..1, for a fill ring.
func (s *State) Progress(a Action) float32 { return s.get(a, Button).progress }

// Axis returns the value of the Axis action a.
func (s *State) Axis(a Action) float32 { return s.get(a, Axis).x }

// Vec2 returns the value of the Vec2 action a. Y is down positive.
func (s *State) Vec2(a Action) (x, y float32) {
	st := s.get(a, Vec2)
	return st.x, st.y
}

// Pointer returns the pointer of this player: the mouse cursor when the
// player has the mouse, else the first touch.
func (s *State) Pointer() Pointer { return s.pointer }

// Touch returns the live geometry of the touch regions of this player's
// active mappings, for drawing virtual controls. Empty unless the player
// has the touch screen. Valid until the next Update.
func (s *State) Touch() []TouchControl { return s.touch }

// Pointer is a screen position in logical pixels.
type Pointer struct {
	X, Y float32
	// Valid is false when the player has neither mouse nor touch.
	Valid bool
	// Touch is set when the position comes from a touch.
	Touch bool
}

// accum collects one tick of binding values for one action.
type accum struct {
	down     bool
	strength float32
	x, y     float32
	progress float32
}

// commit turns this tick's accumulated values into the State, computing
// edges against the previous tick. Edges come from comparing ticks here, not
// from the source, so fake sources, replays and blocked mappings share one
// code path.
func (s *State) commit(acc []accum) {
	s.now++
	for id := 1; id < len(s.a); id++ {
		st, ac := &s.a[id], &acc[id]
		was := st.flags&flagDown != 0
		st.flags = 0
		if ac.down {
			st.flags |= flagDown
			st.held++
			if !was {
				st.flags |= flagPressed
				st.pressedAt = s.now
			}
		} else {
			st.held = 0
			if was {
				st.flags |= flagReleased
			}
		}
		st.strength, st.x, st.y, st.progress = ac.strength, ac.x, ac.y, ac.progress
	}
}

// merge folds o into s for [Router.Any]: OR of buttons, largest magnitude
// of axes and vectors.
func (ac *accum) merge(o *actionState, kind Kind) {
	switch kind {
	case Button:
		if o.flags&flagDown != 0 {
			ac.down = true
		}
		ac.strength = max(ac.strength, o.strength)
		ac.progress = max(ac.progress, o.progress)
	case Axis:
		if abs(o.x) > abs(ac.x) {
			ac.x = o.x
		}
	case Vec2:
		if o.x*o.x+o.y*o.y > ac.x*ac.x+ac.y*ac.y {
			ac.x, ac.y = o.x, o.y
		}
	}
}

// clearPresses forgets every buffered press.
func (s *State) clearPresses() {
	for i := range s.a {
		s.a[i].pressedAt = 0
	}
}

func len32(x, y float32) float32 { return float32(math.Sqrt(float64(x*x + y*y))) }

// abs returns |x|. It is local so that the package imports only the
// standard library (math.Abs is float64 only).
func abs(x float32) float32 {
	if x < 0 {
		return -x
	}
	return x
}
