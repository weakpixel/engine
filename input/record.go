package input

import (
	"encoding/gob"
	"io"
	"slices"
	"time"
)

// Recorder keeps the raw input of every tick, for replays, bug reports and
// simulation tests. Attach it with [Router.Record]; play it back with
// [NewReplaySource]. Because every interaction is counted in ticks, a replay
// reproduces the same State sequence.
//
// Ticks whose frame equals the previous one and carry no device changes are
// stored as a repeat count, so idle stretches cost nothing. A changed tick
// appends one entry (a few hundred bytes), which allocates while recording.
type Recorder struct {
	entries []recEntry
	ticks   int
	// MaxTicks caps the recording: when it is exceeded the oldest entries
	// are dropped. 0 means no limit.
	MaxTicks int
}

type recEntry struct {
	Frame        Frame
	Repeat       int // further ticks with the same frame
	Connected    []DeviceInfo
	Disconnected []DeviceID
}

// Ticks returns the number of recorded ticks.
func (r *Recorder) Ticks() int { return r.ticks }

func (r *Recorder) add(f *Frame, ch *DeviceChanges) {
	r.ticks++
	changed := len(ch.Connected) > 0 || len(ch.Disconnected) > 0
	if n := len(r.entries); n > 0 && !changed && r.entries[n-1].Frame == *f {
		r.entries[n-1].Repeat++
	} else {
		r.entries = append(r.entries, recEntry{
			Frame:        *f,
			Connected:    slices.Clone(ch.Connected),
			Disconnected: slices.Clone(ch.Disconnected),
		})
	}
	for r.MaxTicks > 0 && r.ticks > r.MaxTicks && len(r.entries) > 1 {
		r.ticks -= 1 + r.entries[0].Repeat
		r.entries = slices.Delete(r.entries, 0, 1)
	}
}

// WriteTo writes the recording with encoding/gob.
func (r *Recorder) WriteTo(w io.Writer) (int64, error) {
	cw := &countWriter{w: w}
	err := gob.NewEncoder(cw).Encode(r.entries)
	return cw.n, err
}

// ReadRecording reads a recording written by [Recorder.WriteTo].
func ReadRecording(rd io.Reader) (*Recorder, error) {
	var es []recEntry
	if err := gob.NewDecoder(rd).Decode(&es); err != nil {
		return nil, err
	}
	rec := &Recorder{entries: es}
	for _, e := range es {
		rec.ticks += 1 + e.Repeat
	}
	return rec, nil
}

type countWriter struct {
	w io.Writer
	n int64
}

func (c *countWriter) Write(p []byte) (int, error) {
	n, err := c.w.Write(p)
	c.n += int64(n)
	return n, err
}

// ReplaySource plays a [Recorder] back as a [Source]. After the last tick it
// keeps returning the last frame with every control released.
type ReplaySource struct {
	rec    *Recorder
	entry  int
	repeat int
	// Names maps keys to layout labels for KeyName; nil uses Key.String.
	Names map[Key]string
}

// NewReplaySource returns a source that replays rec from its first tick.
func NewReplaySource(rec *Recorder) *ReplaySource { return &ReplaySource{rec: rec} }

// Done reports whether every recorded tick has been played.
func (s *ReplaySource) Done() bool { return s.entry >= len(s.rec.entries) }

// Poll implements [Source].
func (s *ReplaySource) Poll(f *Frame, ch *DeviceChanges) {
	if s.Done() {
		if n := len(s.rec.entries); n > 0 {
			*f = s.rec.entries[n-1].Frame
			blur(f)
		}
		return
	}
	e := &s.rec.entries[s.entry]
	*f = e.Frame
	if s.repeat == 0 {
		ch.Connected = append(ch.Connected, e.Connected...)
		ch.Disconnected = append(ch.Disconnected, e.Disconnected...)
	}
	if s.repeat < e.Repeat {
		s.repeat++
	} else {
		s.entry++
		s.repeat = 0
	}
}

// KeyName implements [Source].
func (s *ReplaySource) KeyName(k Key) string {
	if n, ok := s.Names[k]; ok {
		return n
	}
	return k.String()
}

// Vibrate implements [Source]; a replay does not rumble.
func (s *ReplaySource) Vibrate(DeviceID, float32, float32, time.Duration) {}
