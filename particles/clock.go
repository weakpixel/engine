package particles

// maxSeekSteps caps a seek (ten seconds at 60 ticks per second), so a scrub
// stays cheap.
const maxSeekSteps = 600

// Clock is an optional World resource that editors use to control the
// simulation of every emitter: pause, single steps, time scale and seeking
// to a time. The zero Clock, and a world without one, leave the simulation
// as it is. The engine registers a zero Clock; only tools write it.
type Clock struct {
	// Scale multiplies the tick's delta time; 0 means 1.
	Scale float32
	// Paused stops the simulation. Each count of Step runs one tick while
	// paused.
	Paused bool
	// Step is the number of ticks to run while Paused; the System counts it
	// down.
	Step int
	// Seek is the time in seconds of the pending [Clock.SeekTo].
	Seek float32
	seek bool
}

// SeekTo makes the next tick restart every emitter and simulate sec seconds
// in steps of the tick's delta time (at most 600 steps). The result is deterministic
// for an emitter with a Seed. The simulation then continues from there.
func (c *Clock) SeekTo(sec float32) {
	c.Seek, c.seek = max(sec, 0), true
}

func (c *Clock) scale() float32 {
	if c.Scale == 0 {
		return 1
	}
	return c.Scale
}

// idle reports whether the clock changes nothing.
func (c *Clock) idle() bool {
	return !c.Paused && !c.seek && (c.Scale == 0 || c.Scale == 1)
}
