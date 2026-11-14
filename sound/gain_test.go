package sound

import (
	"bytes"
	"encoding/binary"
	"io"
	"testing"
	"testing/iotest"
)

func TestPanGains(t *testing.T) {
	tests := []struct {
		pan         float32
		left, right float32
	}{
		{0, 1, 1},
		{-1, 1, 0},
		{1, 0, 1},
		{0.5, 0.5, 1},
		{-0.25, 1, 0.75},
		{-3, 1, 0}, // clamped
		{3, 0, 1},
	}
	for _, tt := range tests {
		l, r := panGains(tt.pan)
		if l != tt.left || r != tt.right {
			t.Errorf("panGains(%v) = %v, %v, want %v, %v", tt.pan, l, r, tt.left, tt.right)
		}
	}
}

func TestGainsRoundTrip(t *testing.T) {
	g := newGains(0.25, 1.5)
	if l, r := g.get(); l != 0.25 || r != 1.5 {
		t.Errorf("get = %v, %v", l, r)
	}
}

// frames encodes (left, right) sample pairs as 16-bit stereo PCM.
func frames(samples ...int16) []byte {
	b := make([]byte, 0, 2*len(samples))
	for _, s := range samples {
		b = binary.LittleEndian.AppendUint16(b, uint16(s))
	}
	return b
}

func TestGainReaderScalesAndClamps(t *testing.T) {
	src := frames(1000, 1000, -1000, 30000, 20000, -20000)
	want := frames(500, 2000, -500, 32767, 10000, -32768)

	tests := []struct {
		name string
		src  func([]byte) io.Reader
		buf  int // caller buffer size
	}{
		{"whole reads", func(b []byte) io.Reader { return bytes.NewReader(b) }, 64},
		{"one byte source", func(b []byte) io.Reader { return iotest.OneByteReader(bytes.NewReader(b)) }, 64},
		{"half-frame source", func(b []byte) io.Reader { return iotest.HalfReader(bytes.NewReader(b)) }, 64},
		{"tiny caller buffer", func(b []byte) io.Reader { return bytes.NewReader(b) }, 3},
		{"odd caller buffer", func(b []byte) io.Reader { return bytes.NewReader(b) }, 7},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newGainReader(tt.src(bytes.Clone(src)), newGains(0.5, 2))
			var got []byte
			buf := make([]byte, tt.buf)
			for {
				n, err := r.Read(buf)
				got = append(got, buf[:n]...)
				if err == io.EOF {
					break
				}
				if err != nil {
					t.Fatal(err)
				}
			}
			if !bytes.Equal(got, want) {
				t.Errorf("got %v, want %v", got, want)
			}
		})
	}
}

func TestGainReaderDropsPartialFrame(t *testing.T) {
	src := append(frames(100, 200), 1, 2, 3) // one frame and 3 stray bytes
	got, err := io.ReadAll(newGainReader(bytes.NewReader(src), newGains(1, 1)))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, frames(100, 200)) {
		t.Errorf("got %v", got)
	}
}

func TestGainReaderFollowsGainChanges(t *testing.T) {
	g := newGains(1, 1)
	r := newGainReader(bytes.NewReader(frames(100, 100, 100, 100)), g)
	buf := make([]byte, 4)
	if _, err := io.ReadFull(r, buf); err != nil {
		t.Fatal(err)
	}
	g.set(0, 0.5)
	if _, err := io.ReadFull(r, buf); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(buf, frames(0, 50)) {
		t.Errorf("second frame = %v, want gains applied", buf)
	}
}
