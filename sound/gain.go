package sound

import (
	"encoding/binary"
	"io"
	"math"
	"sync/atomic"
)

// gains is a voice's left and right gain, written by the game goroutine
// (AudioSystem) and read by the audio goroutine that pulls samples. Both
// float32s are packed into one atomic word so a reader never sees the left
// gain of one update with the right gain of another, and no lock is taken on
// the audio thread.
type gains struct{ v atomic.Uint64 }

func newGains(left, right float32) *gains {
	g := &gains{}
	g.set(left, right)
	return g
}

func (g *gains) set(left, right float32) {
	g.v.Store(uint64(math.Float32bits(left)) | uint64(math.Float32bits(right))<<32)
}

func (g *gains) get() (left, right float32) {
	v := g.v.Load()
	return math.Float32frombits(uint32(v)), math.Float32frombits(uint32(v >> 32))
}

// panGains returns the left and right gain for pan in [-1, 1] (-1 is hard
// left). The law is linear and keeps the center at full volume on both
// sides: panning only ever attenuates the far side, so a centered sound is
// exactly as loud as an unpanned one.
func panGains(pan float32) (left, right float32) {
	pan = min(max(pan, -1), 1)
	return min(1, 1-pan), min(1, 1+pan)
}

// gainReader scales stereo PCM read from src by the voice's current gains,
// either 16-bit little-endian (4 bytes a frame) or float32 little-endian (8
// bytes a frame). Ebiten's players have a volume but no pan, so every voice
// reads through one of these instead, and the AudioSystem steers it through
// the shared gains.
//
// Reads are kept frame-aligned, because a gain applies to whole samples: a
// short read from src is completed to the next frame, and a caller buffer
// smaller than a frame is served from carry.
//
// frames counts the frames handed out. It is an atomic because the audio
// goroutine reads and the game goroutine looks at it (see [Stream.Reading]).
type gainReader struct {
	src   io.Reader
	gains *gains
	float bool // float32 frames instead of int16
	frame int  // bytes per frame

	carry    [maxFrameBytes]byte
	carryOff int // next byte of carry to hand out; frame when empty

	frames atomic.Uint64
}

// maxFrameBytes is the largest frame a gainReader handles (float32 stereo).
const maxFrameBytes = 8

func newGainReader(src io.Reader, g *gains) *gainReader {
	return newFormatGainReader(src, g, StreamInt16)
}

func newFormatGainReader(src io.Reader, g *gains, f StreamFormat) *gainReader {
	frame := bytesPerFrame
	if f == StreamFloat32 {
		frame = 2 * 4
	}
	return &gainReader{src: src, gains: g, float: f == StreamFloat32, frame: frame, carryOff: frame}
}

func (r *gainReader) Read(p []byte) (int, error) {
	if len(p) == 0 {
		return 0, nil
	}
	if r.carryOff < r.frame {
		n := copy(p, r.carry[r.carryOff:r.frame])
		r.carryOff += n
		return n, nil
	}
	if len(p) < r.frame {
		if _, err := io.ReadFull(r.src, r.carry[:r.frame]); err != nil {
			return 0, eof(err)
		}
		r.apply(r.carry[:r.frame])
		r.frames.Add(1)
		r.carryOff = copy(p, r.carry[:r.frame])
		return r.carryOff, nil
	}
	n, err := r.src.Read(p[:len(p)/r.frame*r.frame])
	if rem := n % r.frame; rem != 0 {
		// The buffer was cut to whole frames, so the rest of this frame
		// fits. If src ends mid-frame, the partial frame is dropped.
		m, ferr := io.ReadFull(r.src, p[n:n+r.frame-rem])
		n += m
		if ferr != nil {
			n -= n % r.frame
			err = eof(ferr)
		}
	}
	r.apply(p[:n])
	r.frames.Add(uint64(n / r.frame))
	return n, err
}

// eof maps the io.ErrUnexpectedEOF of a partial frame to io.EOF: a file that
// ends mid-frame simply ends.
func eof(err error) error {
	if err == io.ErrUnexpectedEOF {
		return io.EOF
	}
	return err
}

// apply scales whole frames of b in place.
func (r *gainReader) apply(b []byte) {
	left, right := r.gains.get()
	if left == 1 && right == 1 {
		return
	}
	if r.float {
		for i := 0; i+r.frame <= len(b); i += r.frame {
			scaleFloat(b[i:i+4], left)
			scaleFloat(b[i+4:i+8], right)
		}
		return
	}
	for i := 0; i+r.frame <= len(b); i += r.frame {
		scaleSample(b[i:i+2], left)
		scaleSample(b[i+2:i+4], right)
	}
}

// scaleFloat multiplies the float32 little-endian sample in b by g. It does
// not clamp: Ebiten's mixer clips at the output.
func scaleFloat(b []byte, g float32) {
	v := math.Float32frombits(binary.LittleEndian.Uint32(b)) * g
	binary.LittleEndian.PutUint32(b, math.Float32bits(v))
}

// scaleSample multiplies the int16 little-endian sample in b by g, clamped.
func scaleSample(b []byte, g float32) {
	s := float32(int16(uint16(b[0]) | uint16(b[1])<<8))
	v := int16(min(max(s*g, math.MinInt16), math.MaxInt16))
	b[0], b[1] = byte(v), byte(uint16(v)>>8)
}
