package speech

import (
	"time"

	"github.com/weakpixel/voice"

	"github.com/weakpixel/ecs"
	"github.com/weakpixel/engine/gfx"
)

// line is the runtime state of one speaking entity: its owner, the
// library's Speaker and the text being said. It lives from the entity's
// first Say until the entity is deleted or loses its Speaker.
type line struct {
	e     ecs.Entity
	owner voice.Owner
	sink  panSink
	sp    *voice.Speaker // plays on &sink, so a line never moves (lines are pointers)

	text string
	// speaker is set when the entity had a Speaker at Say: losing it ends
	// the line. A Say with SayOptions.Voice on an entity without one keeps
	// the line until the entity is deleted.
	speaker bool

	// waiting is set while the voice of the line is still loading; the
	// line starts with handle and opts once it is ready.
	waiting bool
	handle  gfx.VoiceHandle
	opts    SayOptions

	// showing is set while the system writes the reveal into the entity's
	// Label: from the start of a line to the tick after it ended.
	showing bool
	// paused is the game's Pause; frozen is set while the speech system did
	// not run (a pause scene above the root, a Tool's pause). Either holds
	// the library's Speaker, which releases the sounding notes.
	paused, frozen bool

	// at is how far the line has played in line time (the timeline's
	// clock, after Speed); speed is the line's Speed. LinePosition reads it.
	at    time.Duration
	speed float64
}

// panSink adds a speaker's pan to every note and drops the notes of a
// silent voice. Release and NoteOff always pass, so nothing can hang.
type panSink struct {
	to   voice.Sink
	pan  float32
	mute bool
}

// NoteOn implements voice.Sink.
func (s *panSink) NoteOn(owner voice.Owner, inst voice.Instrument, note, velocity, pan float32) {
	if s.mute {
		return
	}
	s.to.NoteOn(owner, inst, note, velocity, min(max(pan+s.pan, -1), 1))
}

// NoteOff implements voice.Sink.
func (s *panSink) NoteOff(owner voice.Owner, inst voice.Instrument, note float32) {
	s.to.NoteOff(owner, inst, note)
}

// Release implements voice.Sink.
func (s *panSink) Release(owner voice.Owner) { s.to.Release(owner) }

// hold pauses or resumes the library's Speaker from the game's pause and
// the freeze together.
func (l *line) hold() {
	if l.paused || l.frozen {
		l.sp.Pause()
	} else {
		l.sp.Resume()
	}
}
