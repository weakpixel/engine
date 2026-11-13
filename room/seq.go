package room

// Phase is the part of a transition that is running.
type Phase uint8

// Transition phases.
const (
	// Idle: no transition runs.
	Idle Phase = iota
	// Out covers the screen: Progress goes from 0 to 1.
	Out
	// Wait keeps the screen covered (Progress 1) while the target room
	// loads.
	Wait
	// In uncovers the screen in the new room: Progress goes from 1 to 0.
	In
)

func (p Phase) String() string {
	switch p {
	case Idle:
		return "idle"
	case Out:
		return "out"
	case Wait:
		return "wait"
	case In:
		return "in"
	}
	return "invalid"
}

// event is what a tick of a seq reports: the edges the director acts on.
type event uint8

const (
	evNone    event = iota
	evCovered       // Out finished: start loading the target room
	evSwap          // Wait finished (the new room is ready): place the travelers
	evDone          // In finished: end the transition
)

// seq is the timing of one transition, free of the engine so it can be
// tested on its own: Out for out seconds, Wait until ready, In for in
// seconds. Each tick reports at most one event, so the director loads in
// one tick and swaps in a later one.
type seq struct {
	phase   Phase
	t       float32 // seconds into the current phase
	out, in float32
}

// start begins a transition. Negative durations count as 0.
func (s *seq) start(out, in float32) {
	*s = seq{phase: Out, out: max(out, 0), in: max(in, 0)}
}

// tick advances the sequence by dt seconds. ready reports whether the
// target room is loaded (or failed for good); it is read only in Wait.
func (s *seq) tick(dt float32, ready bool) event {
	switch s.phase {
	case Out:
		s.t += dt
		if s.t >= s.out {
			s.phase, s.t = Wait, 0
			return evCovered
		}
	case Wait:
		if ready {
			s.phase, s.t = In, 0
			return evSwap
		}
	case In:
		s.t += dt
		if s.t >= s.in {
			s.phase, s.t = Idle, 0
			return evDone
		}
	}
	return evNone
}

// progress is the raw coverage, 0 (open) to 1 (covered).
func (s *seq) progress() float32 {
	switch s.phase {
	case Out:
		if s.out == 0 {
			return 1
		}
		return min(s.t/s.out, 1)
	case Wait:
		return 1
	case In:
		if s.in == 0 {
			return 0
		}
		return 1 - min(s.t/s.in, 1)
	}
	return 0
}
