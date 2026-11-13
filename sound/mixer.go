package sound

import (
	"math"
	"time"

	"github.com/weakpixel/engine/gfx"
)

// Bus groups voices under one volume, for the options screen's sliders. A
// request names its bus; the zero Bus is BusSFX.
type Bus uint8

// Predefined buses. Buses up to NumBuses-1 are free for the game's own use.
const (
	BusSFX   Bus = iota // effects from gameplay (Queue.Play, SoundEmitter)
	BusMusic            // music (Queue.PlayMusic always uses it)
	BusUI               // interface clicks and confirmations
	BusVoice            // dialogue
)

// NumBuses is the number of buses a Mixer has. A Bus at or above it is
// treated as BusSFX.
const NumBuses = 8

// Default limits of a [Mixer].
const (
	DefaultMaxVoices        = 32
	DefaultSameSoundPerTick = 2
)

// Mixer is the volume and limit configuration the [AudioSystem] applies. It
// is a world resource; the AudioSystem creates one with [NewMixer] if none
// is set. Changes take effect on the next tick, for playing voices too.
//
// Volumes are factors: 0 is silent, 1 is the recorded level. Values above 1
// amplify (and may clip).
type Mixer struct {
	// MaxVoices bounds the effect voices (queue requests and emitters,
	// not music) playing at once. A request that would exceed it is dropped;
	// an emitter retries on the next tick. 0 means DefaultMaxVoices.
	MaxVoices int
	// SameSoundPerTick bounds how often one sound starts from the queue in
	// a single tick, so ten enemies hit at once do not play ten identical
	// sounds at ten times the loudness. 0 means DefaultSameSoundPerTick.
	SameSoundPerTick int

	master float32
	buses  [NumBuses]float32
}

// NewMixer returns a Mixer with every volume at 1 and the default limits.
// The zero Mixer has every volume at 0, which is silence.
func NewMixer() *Mixer {
	m := &Mixer{master: 1}
	for i := range m.buses {
		m.buses[i] = 1
	}
	return m
}

// SetMaster sets the volume that applies to every bus. Negative is 0.
func (m *Mixer) SetMaster(v float32) { m.master = max(v, 0) }

// Master returns the master volume.
func (m *Mixer) Master() float32 { return m.master }

// SetVolume sets the volume of bus b. Negative is 0.
func (m *Mixer) SetVolume(b Bus, v float32) { m.buses[busIndex(b)] = max(v, 0) }

// Volume returns the volume of bus b, not including the master volume.
func (m *Mixer) Volume(b Bus) float32 { return m.buses[busIndex(b)] }

// gain returns the factor for a voice on bus b: bus times master.
func (m *Mixer) gain(b Bus) float32 { return m.buses[busIndex(b)] * m.master }

func (m *Mixer) maxVoices() int {
	if m.MaxVoices <= 0 {
		return DefaultMaxVoices
	}
	return m.MaxVoices
}

func (m *Mixer) sameSoundPerTick() int {
	if m.SameSoundPerTick <= 0 {
		return DefaultSameSoundPerTick
	}
	return m.SameSoundPerTick
}

func busIndex(b Bus) int {
	if int(b) >= NumBuses {
		return int(BusSFX)
	}
	return int(b)
}

// Listener is where the player hears from: positional sounds (a
// SoundEmitter on an entity with a Transform, a [PlaySound] with At set) are
// panned and attenuated by their position relative to it. It is an optional
// world resource; without it positional sounds play centered and unattenuated.
// game.Engine keeps it at the camera.
type Listener struct {
	// Pos is the listener's world position, usually the camera center.
	Pos gfx.Vec2
	// HalfWidth is the horizontal distance at which a sound is panned hard
	// left or right, usually half the visible world width. 0 disables
	// panning.
	HalfWidth float32
	// Range is the distance at which a sound fades to silence, linearly.
	// 0 disables distance attenuation.
	Range float32
}

// place returns the pan and the attenuation for a sound at pos. A nil
// Listener means centered and full volume.
func (l *Listener) place(pos gfx.Vec2) (pan, gain float32) {
	if l == nil {
		return 0, 1
	}
	d := pos.Sub(l.Pos)
	if l.HalfWidth > 0 {
		pan = min(max(d.X/l.HalfWidth, -1), 1)
	}
	gain = 1
	if l.Range > 0 {
		dist := float32(math.Hypot(float64(d.X), float64(d.Y)))
		gain = max(0, 1-dist/l.Range)
	}
	return pan, gain
}

// PlaySound is a request to play a sound once, pushed with [Queue.Play].
type PlaySound struct {
	// Sound is the effect. A zero handle, or a sound that is not loaded
	// (yet), plays nothing: requests are never deferred, because a hit
	// sound a second late is worse than none.
	Sound gfx.SoundHandle
	// Volume scales the sound. Zero means 1, like SoundEmitter.Volume; a
	// negative value is silent.
	Volume float32
	// Pan places the sound from -1 (left) to 1 (right). It is ignored when
	// Positional is set.
	Pan float32
	// Bus is the mixer bus whose volume applies.
	Bus Bus
	// Positional takes the pan and a distance attenuation from At and the
	// Listener instead of Pan.
	Positional bool
	// At is the world position of a positional sound.
	At gfx.Vec2
}

// Queue collects the sound and music requests of a tick. It is a world
// resource: gameplay systems push into it and the [AudioSystem] drains it at
// the end of the tick. Its buffer is reused, so pushing does not allocate in
// steady state. The zero Queue is ready to use.
type Queue struct {
	sounds []PlaySound
	music  musicRequest
}

// musicRequest is the last PlayMusic or StopMusic of a tick.
type musicRequest struct {
	set   bool
	track gfx.MusicHandle // zero means stop
	fade  time.Duration
}

// Play requests a sound for this tick; see [PlaySound].
func (q *Queue) Play(p PlaySound) { q.sounds = append(q.sounds, p) }

// PlayMusic switches the music to track, crossfading over fade (0 cuts). If
// track is still loading, the current music keeps playing until it is
// ready; if track is the music already playing, nothing changes. A zero or
// failed track stops the music. Music loops and plays on BusMusic. The last
// request of a tick wins.
func (q *Queue) PlayMusic(track gfx.MusicHandle, fade time.Duration) {
	q.music = musicRequest{set: true, track: track, fade: max(fade, 0)}
}

// StopMusic fades the music out over fade (0 cuts). The last request of a
// tick wins.
func (q *Queue) StopMusic(fade time.Duration) {
	q.music = musicRequest{set: true, fade: max(fade, 0)}
}

// Len returns the number of sound requests waiting.
func (q *Queue) Len() int { return len(q.sounds) }

// volumeOf resolves the zero-means-1 rule of PlaySound.Volume and
// SoundEmitter.Volume.
func volumeOf(v float32) float32 {
	switch {
	case v == 0:
		return 1
	case v < 0:
		return 0
	}
	return v
}
