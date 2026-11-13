package sound

import (
	"bytes"
	"fmt"
	"io"
	"time"

	"github.com/hajimehoshi/ebiten/v2/audio"
)

// Backend starts voices on an audio device. The [AudioSystem] is its only
// caller, on the game goroutine. Implement it to play through something
// other than Ebiten, or to observe playback in tests.
type Backend interface {
	// PlaySound starts s, repeated forever if loop is set, with the given
	// gains (see [Voice.SetGains]). s has at least one frame.
	PlaySound(s *Sound, loop bool, left, right float32) (Voice, error)
	// PlayMusic starts streaming m, looping, with the given gains. m has at
	// least one frame.
	PlayMusic(m *Music, left, right float32) (Voice, error)
	// PlayStream starts reading src, which delivers stereo PCM in format at
	// the device's sample rate, with the given gains. buffer is the
	// player's buffer size (0 means the backend's default); a larger buffer
	// is safer against stalls, a smaller one lowers the latency of what the
	// source produces. The stream ends when src returns an error (io.EOF
	// included) or on Stop.
	PlayStream(src io.Reader, format StreamFormat, buffer time.Duration, left, right float32) (StreamVoice, error)
}

// Voice is one playing sound or track.
type Voice interface {
	// SetGains changes the left and right gain (0 is silent, 1 is the
	// recorded level) of the playing voice.
	SetGains(left, right float32)
	// Playing reports whether the voice is still playing. A one-shot voice
	// stops by itself at its end; a looping one only through Stop.
	Playing() bool
	// Stop ends the voice and releases its resources. The Voice must not
	// be used afterwards.
	Stop()
}

// StreamVoice is a [Voice] that plays a stream source.
type StreamVoice interface {
	Voice
	// Frames returns how many stereo frames the audio side has read from
	// the source so far. It may be called from the game goroutine while the
	// audio goroutine reads.
	Frames() uint64
}

// EbitenBackend plays through an Ebiten audio.Context. Each voice is an
// audio.Player that reads its source through a gain stage (Ebiten players
// have no pan), so starting a voice allocates; a playing voice does not.
type EbitenBackend struct {
	ctx *audio.Context
}

// NewEbitenBackend returns a backend that plays on ctx. Sounds and tracks
// must have been loaded with ctx's sample rate ([RegisterLoaders]).
func NewEbitenBackend(ctx *audio.Context) *EbitenBackend {
	return &EbitenBackend{ctx: ctx}
}

// Context returns the audio context the backend plays on.
func (b *EbitenBackend) Context() *audio.Context { return b.ctx }

// PlaySound implements [Backend]. It returns an error if s was decoded at
// another sample rate than the context's.
func (b *EbitenBackend) PlaySound(s *Sound, loop bool, left, right float32) (Voice, error) {
	if s.SampleRate != b.ctx.SampleRate() {
		return nil, fmt.Errorf("sound: sound decoded at %d Hz, audio context runs at %d Hz", s.SampleRate, b.ctx.SampleRate())
	}
	var src io.Reader = bytes.NewReader(s.PCM)
	if loop {
		src = audio.NewInfiniteLoop(bytes.NewReader(s.PCM), int64(len(s.PCM)))
	}
	return b.start(src, left, right)
}

// PlayMusic implements [Backend]. The track is decoded while it plays.
func (b *EbitenBackend) PlayMusic(m *Music, left, right float32) (Voice, error) {
	if m.SampleRate != b.ctx.SampleRate() {
		return nil, fmt.Errorf("sound: music loaded for %d Hz, audio context runs at %d Hz", m.SampleRate, b.ctx.SampleRate())
	}
	s, err := decode(m.Format, m.SampleRate, m.Data)
	if err != nil {
		return nil, err
	}
	return b.start(audio.NewInfiniteLoop(s, s.Length()), left, right)
}

func (b *EbitenBackend) start(src io.Reader, left, right float32) (Voice, error) {
	g := newGains(left, right)
	p, err := b.ctx.NewPlayer(newGainReader(src, g))
	if err != nil {
		return nil, err
	}
	p.Play()
	return &ebitenVoice{player: p, gains: g}, nil
}

// PlayStream implements [Backend]. The source must produce frames at the
// context's sample rate. StreamFloat32 plays through NewPlayerF32.
func (b *EbitenBackend) PlayStream(src io.Reader, format StreamFormat, buffer time.Duration, left, right float32) (StreamVoice, error) {
	g := newGains(left, right)
	r := newFormatGainReader(src, g, format)
	var p *audio.Player
	var err error
	if format == StreamFloat32 {
		p, err = b.ctx.NewPlayerF32(r)
	} else {
		p, err = b.ctx.NewPlayer(r)
	}
	if err != nil {
		return nil, err
	}
	if buffer > 0 {
		p.SetBufferSize(buffer)
	}
	p.Play()
	return &ebitenStream{ebitenVoice: ebitenVoice{player: p, gains: g}, reader: r}, nil
}

type ebitenStream struct {
	ebitenVoice
	reader *gainReader
}

func (s *ebitenStream) Frames() uint64 { return s.reader.frames.Load() }

type ebitenVoice struct {
	player *audio.Player
	gains  *gains
}

func (v *ebitenVoice) SetGains(left, right float32) { v.gains.set(left, right) }

// Playing is also true while Ebiten has not opened the device yet (before
// the game loop's first update), so a sound started at boot is not reaped
// before it was heard.
func (v *ebitenVoice) Playing() bool { return v.player.IsPlaying() }

// Stop pauses the player and stops it reading its source (Player.Close is
// deprecated since Ebiten v2.10). Ebiten releases the device player once
// the stopped, unreachable Player is finalized.
func (v *ebitenVoice) Stop() { v.player.PauseAndStopReading() }

// NullBackend plays nothing. Its one-shot voices end at once and its looping
// voices and tracks last until stopped, so the AudioSystem's bookkeeping
// behaves as with a device. Use it for servers, tools and tests.
type NullBackend struct{}

// PlaySound implements [Backend].
func (NullBackend) PlaySound(_ *Sound, loop bool, _, _ float32) (Voice, error) {
	return &nullVoice{playing: loop}, nil
}

// PlayMusic implements [Backend].
func (NullBackend) PlayMusic(*Music, float32, float32) (Voice, error) {
	return &nullVoice{playing: true}, nil
}

type nullVoice struct{ playing bool }

func (*nullVoice) SetGains(float32, float32) {}
func (v *nullVoice) Playing() bool           { return v.playing }
func (v *nullVoice) Stop()                   { v.playing = false }

// PlayStream implements [Backend]. The source is never read, so a stream on
// the null backend reports no frames and a source that buffers while nobody
// reads must bound itself.
func (NullBackend) PlayStream(io.Reader, StreamFormat, time.Duration, float32, float32) (StreamVoice, error) {
	return &nullStream{nullVoice{playing: true}}, nil
}

type nullStream struct{ nullVoice }

func (*nullStream) Frames() uint64 { return 0 }
