package camera

import (
	"math"

	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/xmath"
)

// Default shake parameters, used where a Shake field is zero.
const (
	DefaultShakeFrequency = 20 // Hz
	DefaultShakeDecay     = 1  // trauma per second
	DefaultShakeExponent  = 2
)

// DefaultShakeOffset is the offset at full shake used when Shake.MaxOffset
// is zero, in world units.
var DefaultShakeOffset = gfx.Vec2{X: 6, Y: 6}

// Noise seeds of the shake axes: one unrelated curve per axis.
const (
	seedShakeX     = 0x5eed01
	seedShakeY     = 0x5eed02
	seedShakeAngle = 0x5eed03
)

// Shake configures the trauma-based screen shake: [Rig.AddTrauma] adds
// trauma in [0, 1], trauma falls by Decay per second, and the camera is
// offset by MaxOffset (and rotated by MaxAngle) times trauma^Exponent times
// smooth noise. Small hits barely move the view, big ones shake hard, and
// several hits add up. Every zero field means its documented default.
type Shake struct {
	// MaxOffset is the offset at full shake, per axis, in world units.
	// Zero means DefaultShakeOffset.
	MaxOffset gfx.Vec2
	// MaxAngle is the rotation at full shake, in radians. 0 means no
	// rotation; a rotating shake turns off render.Camera.PixelSnap while
	// the view is rotated.
	MaxAngle float32
	// Frequency is the noise frequency in Hz: higher shakes faster. Zero
	// means DefaultShakeFrequency.
	Frequency float32
	// Decay is the trauma lost per second; 1 lets full trauma settle in a
	// second. Zero means DefaultShakeDecay.
	Decay float32
	// Exponent shapes the shake: the amplitude is trauma^Exponent. Zero
	// means DefaultShakeExponent.
	Exponent float32
	// InBounds clamps the shaken view to Rig.Bounds, so the shake never
	// shows outside the level but flattens against its edges. By default
	// the shake is added after the clamp and stays visible at an edge. It
	// only matters with a Target and non-zero Bounds.
	InBounds bool
	// Off turns the shake off (an accessibility setting): AddTrauma still
	// counts and trauma still decays, but the camera does not move.
	Off bool
}

// shakeState is the shake's state between ticks. It is kept apart from
// rigState so a snap does not stop a shake.
type shakeState struct {
	trauma float32
	// time is the noise time in seconds; it restarts when trauma is 0.
	time float32
	// offset and angle are what the System applied last tick, so it can
	// take them back when it does not own Camera.Pos (no Target).
	offset gfx.Vec2
	angle  float32
}

// AddTrauma adds t to the shake trauma, which stays in [0, 1]; a negative t
// reduces it. Call it from any system: an explosion adds 0.6, a hit 0.3.
func (r *Rig) AddTrauma(t float32) { r.shake.trauma = min(max(r.shake.trauma+t, 0), 1) }

// Trauma returns the current shake trauma in [0, 1].
func (r *Rig) Trauma() float32 { return r.shake.trauma }

// StopShake sets the trauma to 0; the next System tick puts the camera back
// on its unshaken position.
func (r *Rig) StopShake() { r.shake.trauma = 0 }

// step decays the trauma by dt seconds and returns the shake offset and
// angle for this tick. A dt of zero or less changes nothing and returns
// last tick's values.
func (s *shakeState) step(c Shake, dt float32) (gfx.Vec2, float32) {
	if dt <= 0 {
		return s.offset, s.angle
	}
	s.trauma = max(s.trauma-c.decay()*dt, 0)
	if s.trauma == 0 {
		// Restarting keeps float32 time precise however long the game runs.
		s.time = 0
		return gfx.Vec2{}, 0
	}
	s.time += dt
	if c.Off {
		return gfx.Vec2{}, 0
	}
	amount := float32(math.Pow(float64(s.trauma), float64(c.exponent())))
	x := s.time * c.frequency()
	mo := c.maxOffset()
	off := gfx.Vec2{
		X: mo.X * amount * xmath.Noise1(seedShakeX, x),
		Y: mo.Y * amount * xmath.Noise1(seedShakeY, x),
	}
	var angle float32
	if c.MaxAngle != 0 {
		angle = c.MaxAngle * amount * xmath.Noise1(seedShakeAngle, x)
	}
	return off, angle
}

func (c Shake) maxOffset() gfx.Vec2 {
	if c.MaxOffset == (gfx.Vec2{}) {
		return DefaultShakeOffset
	}
	return c.MaxOffset
}

func (c Shake) frequency() float32 { return orDefault(c.Frequency, DefaultShakeFrequency) }
func (c Shake) decay() float32     { return orDefault(c.Decay, DefaultShakeDecay) }
func (c Shake) exponent() float32  { return orDefault(c.Exponent, DefaultShakeExponent) }
