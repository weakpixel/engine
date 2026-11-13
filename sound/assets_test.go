package sound

import (
	"testing"
	"time"
)

func TestFormatOf(t *testing.T) {
	tests := []struct {
		name string
		want Format
	}{
		{"a.wav", FormatWAV},
		{"dir/A.WAV", FormatWAV},
		{"b.ogg", FormatOgg},
		{"c.mp3", FormatMP3},
		{"d.flac", FormatUnknown},
		{"noext", FormatUnknown},
	}
	for _, tt := range tests {
		if got := FormatOf(tt.name); got != tt.want {
			t.Errorf("FormatOf(%q) = %v, want %v", tt.name, got, tt.want)
		}
	}
}

func TestLoadSound(t *testing.T) {
	srv := newServer(t)
	store := SoundStoreOf(srv)
	tests := []struct {
		key        string
		wantFrames int // -1: only check that there is audio
	}{
		{"sfx/hit.wav", 480},
		{"sfx/jump.wav", 4800}, // mono is decoded to stereo
		{"sfx/empty.wav", 0},
		{"sfx/jump.ogg", -1},
	}
	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			h, err := LoadSoundSync(srv.Global(), tt.key)
			if err != nil {
				t.Fatal(err)
			}
			s := store.Get(h)
			if s.SampleRate != testRate {
				t.Errorf("SampleRate = %d, want %d", s.SampleRate, testRate)
			}
			if len(s.PCM)%bytesPerFrame != 0 {
				t.Errorf("PCM length %d is not whole frames", len(s.PCM))
			}
			frames := len(s.PCM) / bytesPerFrame
			switch {
			case tt.wantFrames >= 0 && frames != tt.wantFrames:
				t.Errorf("frames = %d, want %d", frames, tt.wantFrames)
			case tt.wantFrames < 0 && frames == 0:
				t.Error("no audio decoded")
			}
		})
	}
}

func TestLoadSoundResamples(t *testing.T) {
	srv := newServer(t)
	h, err := LoadSoundSync(srv.Global(), "sfx/slow.wav") // 100ms at 22.05kHz
	if err != nil {
		t.Fatal(err)
	}
	if d := SoundStoreOf(srv).Get(h).Duration(); d < 99*time.Millisecond || d > 101*time.Millisecond {
		t.Errorf("Duration = %v, want about 100ms", d)
	}
}

// Sounds report their decoded PCM to the asset stats, music its file bytes.
func TestAssetSizes(t *testing.T) {
	srv := newServer(t)
	if _, err := LoadSoundSync(srv.Global(), "sfx/hit.wav"); err != nil {
		t.Fatal(err)
	}
	m, err := LoadMusicSync(srv.Global(), "music/a.wav")
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]int64{"sound.Sound": 480 * bytesPerFrame, "sound.Music": int64(len(MusicStoreOf(srv).Get(m).Data))}
	for _, ts := range srv.Stats().Types {
		if w, ok := want[ts.Name]; ok && ts.Bytes != w {
			t.Errorf("%s bytes = %d, want %d", ts.Name, ts.Bytes, w)
		}
	}
}

func TestLoadMusic(t *testing.T) {
	srv := newServer(t)
	tests := []struct {
		key    string
		format Format
		min    time.Duration
	}{
		{"music/a.wav", FormatWAV, time.Second},
		{"music/c.ogg", FormatOgg, 10 * time.Second},
		{"music/d.mp3", FormatMP3, 10 * time.Second},
	}
	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			h, err := LoadMusicSync(srv.Global(), tt.key)
			if err != nil {
				t.Fatal(err)
			}
			m := MusicStoreOf(srv).Get(h)
			if m.Format != tt.format || m.SampleRate != testRate || len(m.Data) == 0 {
				t.Errorf("got format %v, rate %d, %d bytes", m.Format, m.SampleRate, len(m.Data))
			}
			if m.Duration < tt.min {
				t.Errorf("Duration = %v, want at least %v", m.Duration, tt.min)
			}
		})
	}
}

func TestLoadFailures(t *testing.T) {
	srv := newServer(t)
	tests := []struct {
		name string
		load func() error
		want string
	}{
		{"bad wav", func() error { _, err := LoadSoundSync(srv.Global(), "sfx/bad.wav"); return err }, "sfx/bad.wav"},
		{"unsupported", func() error { _, err := LoadSoundSync(srv.Global(), "sfx/hit.flac"); return err }, "unsupported audio format"},
		{"missing", func() error { _, err := LoadSoundSync(srv.Global(), "sfx/none.wav"); return err }, "sfx/none.wav"},
		{"bad music", func() error { _, err := LoadMusicSync(srv.Global(), "music/bad.ogg"); return err }, "music/bad.ogg"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) { mustContain(t, tt.load(), tt.want) })
	}
}

func TestPlaceholdersAreSilent(t *testing.T) {
	srv := newServer(t)
	s := SoundStoreOf(srv).Get(LoadSound(srv.Global(), "sfx/hit.wav")) // not loaded yet
	if len(s.PCM) != 0 {
		t.Errorf("placeholder sound has %d bytes", len(s.PCM))
	}
	m := MusicStoreOf(srv).Get(LoadMusic(srv.Global(), "music/a.wav"))
	if len(m.Data) != 0 {
		t.Errorf("placeholder music has %d bytes", len(m.Data))
	}
}
