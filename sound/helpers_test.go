package sound

import (
	"encoding/binary"
	"io"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	raudio "github.com/hajimehoshi/ebiten/v2/examples/resources/audio"

	"github.com/weakpixel/asset"
	"github.com/weakpixel/engine/gfx"
)

const testRate = 48000

// wavFile returns a 16-bit PCM WAV file at rate with frames frames of
// channels channels, every sample set to value.
func wavFile(rate, channels, frames int, value int16) []byte {
	data := make([]byte, frames*channels*2)
	for i := 0; i < len(data); i += 2 {
		binary.LittleEndian.PutUint16(data[i:], uint16(value))
	}
	var b []byte
	b = append(b, "RIFF"...)
	b = binary.LittleEndian.AppendUint32(b, uint32(36+len(data)))
	b = append(b, "WAVEfmt "...)
	b = binary.LittleEndian.AppendUint32(b, 16)
	b = binary.LittleEndian.AppendUint16(b, 1) // PCM
	b = binary.LittleEndian.AppendUint16(b, uint16(channels))
	b = binary.LittleEndian.AppendUint32(b, uint32(rate))
	b = binary.LittleEndian.AppendUint32(b, uint32(rate*channels*2))
	b = binary.LittleEndian.AppendUint16(b, uint16(channels*2))
	b = binary.LittleEndian.AppendUint16(b, 16)
	b = append(b, "data"...)
	b = binary.LittleEndian.AppendUint32(b, uint32(len(data)))
	return append(b, data...)
}

// testFiles is the file system of every test server.
func testFiles() fstest.MapFS {
	return fstest.MapFS{
		"sfx/hit.wav":     {Data: wavFile(testRate, 2, 480, 1000)}, // 10ms
		"sfx/jump.wav":    {Data: wavFile(testRate, 1, 4800, -2000)},
		"sfx/slow.wav":    {Data: wavFile(22050, 2, 2205, 1000)}, // 100ms at 22.05kHz
		"sfx/empty.wav":   {Data: wavFile(testRate, 2, 0, 0)},
		"sfx/jump.ogg":    {Data: raudio.Jump_ogg},
		"sfx/bad.wav":     {Data: []byte("not a wave file")},
		"sfx/hit.flac":    {Data: []byte("fLaC")},
		"music/a.wav":     {Data: wavFile(testRate, 2, testRate, 500)}, // 1s
		"music/b.wav":     {Data: wavFile(testRate, 2, testRate/2, 500)},
		"music/c.ogg":     {Data: raudio.Ragtime_ogg},
		"music/d.mp3":     {Data: raudio.Ragtime_mp3},
		"music/bad.ogg":   {Data: []byte("OggS but not really")},
		"music/empty.wav": {Data: wavFile(testRate, 2, 0, 0)},
	}
}

// newServer returns a server over testFiles with the sound loaders, decoding
// inline (no workers) so tests are deterministic.
func newServer(t testing.TB) *asset.Server {
	t.Helper()
	// Failures are expected in some tests; they are checked through the
	// stores, not the log.
	srv := asset.NewServer(asset.Options{FS: testFiles(), OnError: func(error) {}})
	RegisterLoaders(srv, testRate)
	t.Cleanup(srv.Close)
	return srv
}

func mustSound(t testing.TB, srv *asset.Server, key string) gfx.SoundHandle {
	t.Helper()
	h, err := LoadSoundSync(srv.Global(), key)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func mustMusic(t testing.TB, srv *asset.Server, key string) gfx.MusicHandle {
	t.Helper()
	h, err := LoadMusicSync(srv.Global(), key)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func mustContain(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil || !strings.Contains(err.Error(), want) {
		t.Fatalf("error %v does not contain %q", err, want)
	}
}

// fakeBackend records the voices it starts.
type fakeBackend struct {
	voices []*fakeVoice
	err    error // returned by the next Play call, then cleared
}

type fakeVoice struct {
	sound       *Sound
	music       *Music
	stream      io.Reader
	format      StreamFormat
	buffer      time.Duration
	frames      uint64
	loop        bool
	left, right float32
	setCalls    int
	playing     bool
	stopped     bool
}

func (b *fakeBackend) PlaySound(s *Sound, loop bool, left, right float32) (Voice, error) {
	return b.add(&fakeVoice{sound: s, loop: loop, left: left, right: right, playing: true})
}

func (b *fakeBackend) PlayMusic(m *Music, left, right float32) (Voice, error) {
	return b.add(&fakeVoice{music: m, loop: true, left: left, right: right, playing: true})
}

func (b *fakeBackend) PlayStream(src io.Reader, f StreamFormat, buffer time.Duration, left, right float32) (StreamVoice, error) {
	v := &fakeVoice{stream: src, format: f, buffer: buffer, left: left, right: right, playing: true}
	if _, err := b.add(v); err != nil {
		return nil, err
	}
	return v, nil
}

func (b *fakeBackend) add(v *fakeVoice) (Voice, error) {
	if err := b.err; err != nil {
		b.err = nil
		return nil, err
	}
	b.voices = append(b.voices, v)
	return v, nil
}

// playing returns the voices that are neither finished nor stopped.
func (b *fakeBackend) playing() []*fakeVoice {
	var out []*fakeVoice
	for _, v := range b.voices {
		if v.playing && !v.stopped {
			out = append(out, v)
		}
	}
	return out
}

func (v *fakeVoice) SetGains(left, right float32) { v.left, v.right = left, right; v.setCalls++ }
func (v *fakeVoice) Playing() bool                { return v.playing && !v.stopped }
func (v *fakeVoice) Stop() {
	if v.stopped {
		panic("fakeVoice stopped twice")
	}
	v.stopped = true
}

func (v *fakeVoice) Frames() uint64 { return v.frames }

// finish ends a one-shot voice as if it reached its end.
func (v *fakeVoice) finish() { v.playing = false }
