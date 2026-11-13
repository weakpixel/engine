package speech

import (
	"time"

	"github.com/weakpixel/voice"

	"github.com/weakpixel/ecs"
)

// NoteKind is what plays a [Note]: one of a voice's instruments.
type NoteKind uint8

// The kinds of a note.
const (
	NoteVowel      NoteKind = iota // a vowel
	NoteSonorant                   // m, n, l, r, ...
	NoteFricative                  // s, f, h, ...
	NotePlosive                    // p, t, k, ...
	NoteMotif                      // the quiet melody under a line
	NotePercussion                 // a percussion sound of a rhythm voice
)

// String returns "vowel", "sonorant", "fricative", "plosive", "motif" or
// "percussion".
func (k NoteKind) String() string {
	if k >= NotePercussion {
		return "percussion"
	}
	return voice.Instrument(k).String()
}

// Note is one note of a line, for showing it (a piano roll). Times are in
// line time, from the start of the line, before the line's Speed.
type Note struct {
	On, Off time.Duration
	// Pitch is in semitones, MIDI numbering (60 is middle C).
	Pitch    float32
	Velocity float32 // 0 to 1
	Kind     NoteKind
}

// Step advances speech by dt, like one tick of the speech system ([Speech.Run]).
// A tool that runs the engine paused (game.TickPause skips StagePost, so
// the system does not run) calls it once per tick so its lines and dialogue
// boxes play; a game never needs it, and calling both in one tick advances
// twice. Step after Close does nothing.
func (s *Speech) Step(dt time.Duration) {
	if s.closed {
		return
	}
	s.ran = true
	s.step(dt)
}

// LineNotes appends the notes of e's line (the running one, or the last one
// that ended) to dst and returns it, sorted by On. Without a line, or while
// it waits for its voice, it returns dst unchanged. It allocates only when
// dst grows.
func (s *Speech) LineNotes(e ecs.Entity, dst []Note) []Note {
	l, ok := s.byEnt[e]
	if !ok || l.waiting {
		return dst
	}
	for _, n := range l.sp.Timeline().Notes {
		k := NoteKind(n.Instrument)
		if n.Instrument >= voice.InstPercussion {
			k = NotePercussion
		}
		dst = append(dst, Note{On: n.On, Off: n.Off, Pitch: n.Note, Velocity: n.Velocity, Kind: k})
	}
	return dst
}

// LinePosition returns how far e's line has played and how long it is, both
// in line time like [Note]: at stays put while the line is paused or held
// and reaches length when the line completes (Skip and Stop leave it where
// they ended the line). Both are 0 without a line and while it waits for its voice.
func (s *Speech) LinePosition(e ecs.Entity) (at, length time.Duration) {
	l, ok := s.byEnt[e]
	if !ok || l.waiting {
		return 0, 0
	}
	length = l.sp.Timeline().Duration
	return min(l.at, length), length
}
