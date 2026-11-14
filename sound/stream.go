package sound

import (
	"io"
	"time"
)

// StreamFormat is the PCM layout of a stream source: stereo, interleaved,
// little-endian, at the audio context's sample rate.
type StreamFormat uint8

// Stream formats.
const (
	// StreamInt16 is 16-bit signed integer samples (4 bytes a frame), the
	// layout of every sound the package decodes.
	StreamInt16 StreamFormat = iota
	// StreamFloat32 is 32-bit float samples in [-1, 1] (8 bytes a frame).
	StreamFloat32
)

// DefaultStreamBuffer is the player buffer of a stream whose
// [StreamSource.Buffer] is 0: small enough for dialogue blips to follow the
// text, large enough to survive a slow frame.
const DefaultStreamBuffer = 50 * time.Millisecond

// StreamReadTimeout is how long a stream may go without the audio side
// reading from it before [Stream.Reading] turns false. A browser keeps its
// audio context suspended until the first click, and a window that lost
// focus may pause its player.
const StreamReadTimeout = 200 * time.Millisecond

// StreamSource describes a stream for [AudioSystem.AddStream].
type StreamSource struct {
	// Reader produces the PCM. It is read on the audio goroutine, so it
	// must be safe against the game goroutine, must not block for long and
	// should not allocate. It ends the stream by returning an error.
	Reader io.Reader
	// Format is the layout Reader produces.
	Format StreamFormat
	// Bus is the bus whose volume applies (the zero Bus is BusSFX).
	Bus Bus
	// Volume is the stream's own volume factor; 0 means 1.
	Volume float32
	// Buffer is the player buffer size; 0 means [DefaultStreamBuffer].
	Buffer time.Duration
}

// Stream is a playing stream source, started by [AudioSystem.AddStream].
// Use it from the game goroutine only. Streams do not count against
// [Mixer.MaxVoices].
type Stream struct {
	v      StreamVoice
	bus    Bus
	volume float32
	pan    float32

	// left and right are the gains last handed to the voice.
	left, right float32

	// frames is the voice's counter at the last check; idle is the time
	// since it last changed.
	frames  uint64
	idle    time.Duration
	reading bool
	closed  bool
}

// SetVolume changes the stream's own volume factor (0 is silent; unlike in
// [StreamSource], 0 does not mean 1). It applies from the next tick.
func (s *Stream) SetVolume(v float32) { s.volume = max(v, 0) }

// SetPan changes the stereo position, -1 (left) to 1 (right). It applies
// from the next tick.
func (s *Stream) SetPan(pan float32) { s.pan = pan }

// Reading reports whether the audio side is pulling samples from the source:
// true once frames were read and until [StreamReadTimeout] passes without a
// read. It is false right after AddStream, under [NullBackend], while a
// browser has not started its audio context and after Close. It is updated
// by [AudioSystem.Run], once per tick.
//
// A source that queues commands for the next read (a synthesizer) uses it to
// drop what nobody listened to instead of playing it as a burst later.
func (s *Stream) Reading() bool { return s.reading && !s.closed }

// Close stops the stream and releases its player. The stream must not be
// used afterwards; closing twice is harmless.
func (s *Stream) Close() {
	if s.closed {
		return
	}
	s.closed = true
	s.reading = false
	s.v.Stop()
}

// observe updates Reading from the voice's frame counter after dt.
func (s *Stream) observe(dt time.Duration) {
	if f := s.v.Frames(); f != s.frames {
		s.frames, s.idle, s.reading = f, 0, true
		return
	}
	s.idle += dt
	if s.idle >= StreamReadTimeout {
		s.reading = false
	}
}
