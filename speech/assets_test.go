package speech

import (
	"bytes"
	"encoding/binary"
	"errors"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/weakpixel/voice"

	"github.com/weakpixel/asset"
)

// wavFile returns a 16-bit PCM mono WAV with n frames of a ramp.
func wavFile(rate, n int) []byte {
	var b bytes.Buffer
	w := func(v any) { _ = binary.Write(&b, binary.LittleEndian, v) }
	b.WriteString("RIFF")
	w(uint32(36 + 2*n))
	b.WriteString("WAVEfmt ")
	w(uint32(16))
	w(uint16(1))        // PCM
	w(uint16(1))        // mono
	w(uint32(rate))     // sample rate
	w(uint32(rate * 2)) // byte rate
	w(uint16(2))        // block align
	w(uint16(16))       // bits per sample
	b.WriteString("data")
	w(uint32(2 * n))
	for i := range n {
		w(int16(i * 100))
	}
	return b.Bytes()
}

func mapFile(s string) *fstest.MapFile {
	return &fstest.MapFile{Data: []byte(s), ModTime: time.Unix(1, 0)}
}

const elvish = `{"version": 0, "name": "elvish",
  "phonemes": [{"symbol": "a", "category": "vowel"}, {"symbol": "th", "category": "fricative"}],
  "unknown": {"policy": "fallback", "phoneme": "a"}}`

// bardVoice is a voice with a language by name and a sample on its motif.
const bardVoice = `{"version": 0, "name": "bard", "language": "elvish", "baseNote": 60, "speed": 12,
  "instruments": {"vowel": {"wave": "square"}, "motif": {"sample": "hum.wav", "root": 57, "loop": true, "sustain": 1}}}`

func newServer(t *testing.T, fsys fstest.MapFS, opts asset.Options) (*asset.Server, *[]error) {
	t.Helper()
	var errs []error
	opts.FS = fsys
	opts.OnError = func(err error) { errs = append(errs, err) }
	srv := asset.NewServer(opts)
	RegisterLoaders(srv)
	t.Cleanup(srv.Close)
	return srv, &errs
}

func speechFiles() fstest.MapFS {
	return fstest.MapFS{
		"talk/bard.voice.json":    mapFile(bardVoice),
		"talk/elvish.lang.json":   mapFile(elvish),
		"talk/hum.wav":            &fstest.MapFile{Data: wavFile(22050, 64)},
		"talk/plain.voice.json":   mapFile(`{"version": 0, "name": "plain", "language": "../lang/x.lang.json", "instruments": {"vowel": {}}}`),
		"lang/x.lang.json":        mapFile(elvish),
		"talk/town.dialogue.json": mapFile(`{"version": 0, "speakers": [{"id": "wren", "voice": "chirpy"}, {"id": "bard", "voice": "bard.voice.json"}, {"id": "kip", "voice": "chirpy"}], "lines": [{"speaker": "bard", "text": "Ae tha."}]}`),
	}
}

func TestAssetVoiceFile(t *testing.T) {
	srv, _ := newServer(t, speechFiles(), asset.Options{})
	h, err := LoadVoiceSync(srv.Global(), "talk/bard.voice.json")
	if err != nil {
		t.Fatal(err)
	}
	v := VoiceStoreOf(srv).Get(h)
	if v.Silent || v.V.Name != "bard" {
		t.Fatalf("voice %q silent %v", v.V.Name, v.Silent)
	}
	if v.Language == voice.DefaultLanguage() || v.Language == nil {
		t.Error("language not resolved from elvish.lang.json")
	}
	s, err := v.Sample("hum.wav")
	if err != nil || len(s.Data) != 64 || s.Rate != 22050 {
		t.Fatalf("sample: %+v, %v", s, err)
	}
	if _, err := v.Sample("other.wav"); err == nil {
		t.Error("an unknown sample name resolved")
	}
	// Language by a path relative to the voice.
	p, err := LoadVoiceSync(srv.Global(), "talk/plain.voice.json")
	if err != nil {
		t.Fatal(err)
	}
	if l := VoiceStoreOf(srv).Get(p).Language; l == voice.DefaultLanguage() {
		t.Error("language path ../lang/x.lang.json not resolved")
	}
}

func TestAssetStarterVoiceKey(t *testing.T) {
	srv, _ := newServer(t, fstest.MapFS{}, asset.Options{Workers: 2})
	h, err := LoadVoiceSync(srv.Global(), "chirpy")
	if err != nil {
		t.Fatal(err)
	}
	v := VoiceStoreOf(srv).Get(h)
	if v.V.Name != "chirpy" || v.Silent || v.Language != voice.DefaultLanguage() {
		t.Errorf("starter: name %q silent %v", v.V.Name, v.Silent)
	}
	if _, err := LoadVoiceSync(srv.Global(), "chirpyy"); err == nil {
		t.Error("an unknown name that is no file loaded")
	}
}

func TestAssetVoiceErrors(t *testing.T) {
	tests := []struct {
		name, file, want string
		extra            fstest.MapFS
	}{
		{"missing sample", `{"version": 0, "name": "x", "instruments": {"vowel": {"sample": "gone.wav"}}}`, "gone.wav", nil},
		{"bad sample", `{"version": 0, "name": "x", "instruments": {"vowel": {"sample": "bad.wav"}}}`, "wav: ", fstest.MapFS{"v/bad.wav": mapFile("RIFF")}},
		{"missing language", `{"version": 0, "name": "x", "language": "dwarvish", "instruments": {"vowel": {}}}`, "v/dwarvish.lang.json", nil},
		{"bad language", `{"version": 0, "name": "x", "language": "bad", "instruments": {"vowel": {}}}`, "phonemes", fstest.MapFS{"v/bad.lang.json": mapFile(`{"version": 0, "phonemes": [{"symbol": "a", "category": "nope"}]}`)}},
		{"field path", `{"version": 0, "name": "x", "velocity": 2, "instruments": {"vowel": {}}}`, "velocity", nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			fsys := fstest.MapFS{"v/x.voice.json": mapFile(tc.file)}
			for k, f := range tc.extra {
				fsys[k] = f
			}
			srv, _ := newServer(t, fsys, asset.Options{})
			h, err := LoadVoiceSync(srv.Global(), "v/x.voice.json")
			if err == nil || !strings.Contains(err.Error(), tc.want) || !strings.Contains(err.Error(), "v/x.voice.json") {
				t.Fatalf("error %v, want one naming v/x.voice.json and containing %q", err, tc.want)
			}
			if v := VoiceStoreOf(srv).Get(h); !v.Silent || v.V == nil {
				t.Error("a failed voice does not give the silent placeholder")
			}
		})
	}
}

func TestAssetDialogue(t *testing.T) {
	srv, _ := newServer(t, speechFiles(), asset.Options{Workers: 2})
	h, err := LoadDialogueSync(srv.Global(), "talk/town.dialogue.json")
	if err != nil {
		t.Fatal(err)
	}
	d := DialogueStoreOf(srv).Get(h)
	if len(d.Script.Speakers) != 3 || len(d.Voices) != 2 {
		t.Fatalf("speakers %d, voices %d", len(d.Script.Speakers), len(d.Voices))
	}
	if d.Voices["chirpy"].V.Name != "chirpy" || d.Voices["bard.voice.json"].V.Name != "bard" {
		t.Errorf("voices %v", d.Voices)
	}
	if _, err := d.Voices["bard.voice.json"].Sample("hum.wav"); err != nil {
		t.Errorf("the dialogue's file voice has no samples: %v", err)
	}
	if pv := d.PlayerVoices(); len(pv) != 2 || pv["chirpy"] != d.Voices["chirpy"].V {
		t.Errorf("PlayerVoices %v", pv)
	}
}

func TestAssetDialogueErrors(t *testing.T) {
	tests := []struct{ name, file, want string }{
		{"unknown starter", `{"version": 0, "speakers": [{"id": "a", "voice": "chirpy"}, {"id": "b", "voice": "chirpyy"}], "lines": [{"speaker": "a", "text": "Hi."}]}`, "speakers[1].voice"},
		{"missing voice file", `{"version": 0, "speakers": [{"id": "a", "voice": "gone.voice.json"}], "lines": [{"speaker": "a", "text": "Hi."}]}`, "d/gone.voice.json"},
		{"bad line", `{"version": 0, "speakers": [{"id": "a", "voice": "soft"}], "lines": [{"speaker": "b", "text": "Hi."}]}`, "lines[0].speaker"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv, _ := newServer(t, fstest.MapFS{"d/x.dialogue.json": mapFile(tc.file)}, asset.Options{})
			h, err := LoadDialogueSync(srv.Global(), "d/x.dialogue.json")
			if err == nil || !strings.Contains(err.Error(), tc.want) || !strings.Contains(err.Error(), "d/x.dialogue.json") {
				t.Fatalf("error %v, want one naming d/x.dialogue.json and containing %q", err, tc.want)
			}
			if d := DialogueStoreOf(srv).Get(h); d.Script != nil {
				t.Error("a failed dialogue has a script")
			}
			var fe *voice.FieldError
			if tc.name != "missing voice file" && !errors.As(err, &fe) {
				t.Errorf("error %v carries no voice.FieldError", err)
			}
		})
	}
}

func TestAssetVoiceHotReload(t *testing.T) {
	fsys := speechFiles()
	srv, errs := newServer(t, fsys, asset.Options{WatchInterval: time.Nanosecond})
	h, err := LoadVoiceSync(srv.Global(), "talk/bard.voice.json")
	if err != nil {
		t.Fatal(err)
	}
	d, err := LoadDialogueSync(srv.Global(), "talk/town.dialogue.json")
	if err != nil {
		t.Fatal(err)
	}
	voices := VoiceStoreOf(srv)
	before := voices.Version(h)
	fsys["talk/bard.voice.json"] = mapFile(strings.Replace(bardVoice, `"speed": 12`, `"speed": 20`, 1))
	fsys["talk/bard.voice.json"].ModTime = time.Unix(2, 0)
	for range 3 {
		srv.Update()
	}
	if len(*errs) > 0 {
		t.Fatalf("reload errors: %v", *errs)
	}
	if voices.Version(h) == before || voices.Get(h).V.Speed != 20 {
		t.Fatalf("voice not reloaded: version %d, speed %v", voices.Version(h), voices.Get(h).V.Speed)
	}
	if got := DialogueStoreOf(srv).Get(d).Voices["bard.voice.json"].V.Speed; got != 20 {
		t.Errorf("the dialogue still has the old voice (speed %v)", got)
	}
}
