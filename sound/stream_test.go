package sound

import (
	"bytes"
	"encoding/binary"
	"io"
	"math"
	"testing"
)

func floatFrames(samples ...float32) []byte {
	var b []byte
	for _, s := range samples {
		b = binary.LittleEndian.AppendUint32(b, math.Float32bits(s))
	}
	return b
}

func TestGainReaderFloat32(t *testing.T) {
	src := floatFrames(0.5, -0.5, 1, 1)
	r := newFormatGainReader(bytes.NewReader(src), newGains(0.5, 2), StreamFloat32)
	for _, size := range []int{64, 3, 9} {
		r.src = bytes.NewReader(src)
		var got []byte
		buf := make([]byte, size)
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
		if want := floatFrames(0.25, -1, 0.5, 2); !bytes.Equal(got, want) {
			t.Errorf("buffer %d: got %v, want %v", size, got, want)
		}
	}
}

func TestGainReaderCountsFrames(t *testing.T) {
	for _, size := range []int{64, 3} {
		r := newGainReader(bytes.NewReader(frames(1, 2, 3, 4, 5, 6)), newGains(1, 1))
		if _, err := io.CopyBuffer(io.Discard, struct{ io.Reader }{r}, make([]byte, size)); err != nil {
			t.Fatal(err)
		}
		if got := r.frames.Load(); got != 3 {
			t.Errorf("buffer %d: frames = %d, want 3", size, got)
		}
	}
}

func TestAddStreamStartsWithBusGains(t *testing.T) {
	s := newAudioScene(t)
	s.run()
	s.mixer().SetVolume(BusVoice, 0.5)
	s.mixer().SetMaster(0.5)
	st, err := s.sys.AddStream(StreamSource{Reader: bytes.NewReader(nil), Format: StreamFloat32, Bus: BusVoice, Volume: 0.8})
	if err != nil {
		t.Fatal(err)
	}
	v := s.last()
	if v.format != StreamFloat32 || v.buffer != DefaultStreamBuffer {
		t.Errorf("format %v buffer %v", v.format, v.buffer)
	}
	s.wantGains(v, 0.2, 0.2)

	s.mixer().SetVolume(BusVoice, 1)
	s.run()
	s.wantGains(v, 0.4, 0.4)
	st.SetVolume(1)
	st.SetPan(1)
	s.run()
	s.wantGains(v, 0, 0.5)
	if s.sys.Stats().Streams != 1 || s.sys.Stats().Voices != 0 {
		t.Errorf("stats %+v", s.sys.Stats())
	}
}

func TestStreamsDoNotCountAgainstMaxVoices(t *testing.T) {
	s := newAudioScene(t)
	s.run()
	s.mixer().MaxVoices = 1
	if _, err := s.sys.AddStream(StreamSource{Reader: bytes.NewReader(nil)}); err != nil {
		t.Fatal(err)
	}
	q := s.queue()
	q.Play(PlaySound{Sound: s.sound("sfx/jump.wav")})
	s.run()
	if got := s.sys.Stats().Started; got != 1 {
		t.Errorf("started %d effects, want 1", got)
	}
}

func TestStreamReading(t *testing.T) {
	s := newAudioScene(t)
	s.run()
	st, err := s.sys.AddStream(StreamSource{Reader: bytes.NewReader(nil)})
	if err != nil {
		t.Fatal(err)
	}
	v := s.last()
	if st.Reading() {
		t.Error("reading before any frame was read")
	}
	s.run() // tick is 100ms
	if st.Reading() {
		t.Error("reading without frames")
	}
	v.frames = 10
	s.run()
	if !st.Reading() {
		t.Error("not reading after frames were read")
	}
	s.run() // 100 ms idle
	if !st.Reading() {
		t.Error("reading turned false before the timeout")
	}
	s.run() // 200 ms idle
	if st.Reading() {
		t.Error("still reading after the timeout")
	}
	v.frames = 11
	s.run()
	if !st.Reading() {
		t.Error("reading did not resume")
	}
}

func TestStreamCloseAndEnd(t *testing.T) {
	s := newAudioScene(t)
	s.run()
	a, _ := s.sys.AddStream(StreamSource{Reader: bytes.NewReader(nil)})
	b, _ := s.sys.AddStream(StreamSource{Reader: bytes.NewReader(nil)})
	c, _ := s.sys.AddStream(StreamSource{Reader: bytes.NewReader(nil)})
	a.Close()
	a.Close()
	if !s.b.voices[0].stopped {
		t.Error("Close did not stop the voice")
	}
	s.b.voices[1].finish() // the reader of b ended
	s.run()
	if !s.b.voices[1].stopped || b.Reading() {
		t.Error("ended stream not released")
	}
	if got := s.sys.Stats().Streams; got != 1 {
		t.Errorf("streams = %d, want 1", got)
	}
	s.sys.Close()
	if !s.b.voices[2].stopped || c.Reading() {
		t.Error("AudioSystem.Close left a stream playing")
	}
}

func TestAddStreamReportsBackendError(t *testing.T) {
	s := newAudioScene(t)
	s.run()
	s.b.err = io.ErrClosedPipe
	if st, err := s.sys.AddStream(StreamSource{Reader: bytes.NewReader(nil)}); err == nil || st != nil {
		t.Errorf("got %v, %v", st, err)
	}
}

func TestNullBackendStreamNeverReads(t *testing.T) {
	s := newAudioScene(t)
	s.sys.backend = NullBackend{}
	r := &countingReader{}
	st, err := s.sys.AddStream(StreamSource{Reader: r})
	if err != nil {
		t.Fatal(err)
	}
	for range 10 {
		s.run()
		if st.Reading() {
			t.Fatal("Reading under NullBackend")
		}
	}
	if r.reads != 0 {
		t.Errorf("source read %d times", r.reads)
	}
	st.Close()
}

type countingReader struct{ reads int }

func (c *countingReader) Read([]byte) (int, error) { c.reads++; return 0, io.EOF }
