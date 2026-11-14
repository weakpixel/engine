package sound

import (
	"bytes"
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2/audio"
)

// The Ebiten backend is checked without a game loop: Ebiten opens the audio
// device only from its update hook, so players stay pending (and report
// Playing) and nothing is heard. That covers construction, the sample rate
// checks and Stop, not the device itself.
func TestEbitenBackend(t *testing.T) {
	ctx := audio.CurrentContext()
	if ctx == nil {
		ctx = audio.NewContext(testRate)
	}
	if ctx.SampleRate() != testRate {
		t.Skipf("audio context already runs at %d Hz", ctx.SampleRate())
	}
	b := NewEbitenBackend(ctx)
	srv := newServer(t)
	snd := SoundStoreOf(srv).Get(mustSound(t, srv, "sfx/hit.wav"))
	mus := MusicStoreOf(srv).Get(mustMusic(t, srv, "music/c.ogg"))

	for _, loop := range []bool{false, true} {
		v, err := b.PlaySound(snd, loop, 1, 0.5)
		if err != nil {
			t.Fatal(err)
		}
		if !v.Playing() {
			t.Errorf("loop=%v: voice not playing (pending) after start", loop)
		}
		v.SetGains(0, 0)
		v.Stop()
		if v.Playing() {
			t.Errorf("loop=%v: voice playing after Stop", loop)
		}
	}
	v, err := b.PlayMusic(mus, 1, 1)
	if err != nil {
		t.Fatal(err)
	}
	v.Stop()

	other := *snd
	other.SampleRate = 22050
	_, err = b.PlaySound(&other, false, 1, 1)
	mustContain(t, err, "22050 Hz")
	otherMusic := *mus
	otherMusic.SampleRate = 44100
	_, err = b.PlayMusic(&otherMusic, 1, 1)
	mustContain(t, err, "44100 Hz")

	for _, f := range []StreamFormat{StreamInt16, StreamFloat32} {
		sv, err := b.PlayStream(bytes.NewReader(make([]byte, 64)), f, 20*time.Millisecond, 1, 1)
		if err != nil {
			t.Fatal(err)
		}
		if sv.Frames() != 0 {
			t.Errorf("format %v: frames read before the device opened", f)
		}
		sv.SetGains(0.5, 0.5)
		sv.Stop()
	}
}
