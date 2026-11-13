package speech

import (
	"strings"
	"testing"
	"time"

	"github.com/weakpixel/voice/synth"

	"github.com/weakpixel/asset"
	"github.com/weakpixel/engine/gfx"
)

// TestSpeechTickDoesNotAllocate checks that a tick of running lines and a
// dialogue box (notes on the synthesizer, the Label reveal, the AutoStart
// scan, a freeze and thaw) allocates nothing.
func TestSpeechTickDoesNotAllocate(t *testing.T) {
	fsys := speechFiles()
	long := strings.Repeat(hello+" ", 40)
	fsys["talk/long.dialogue.json"] = mapFile(`{"version": 0, "speakers": [{"id": "a", "voice": "chirpy"}, {"id": "b", "voice": "bard.voice.json"}],
  "lines": [{"speaker": "b", "text": "` + long + `{a}` + long + `{/}"}]}`)
	r := newRig(t, asset.Options{FS: fsys})
	st, err := synth.NewStream(synth.StreamConfig{SampleRate: 48000})
	if err != nil {
		t.Fatal(err)
	}
	r.s.sink, r.s.patch = st, st
	h := r.voice("chirpy")
	a, b := r.talker(h), r.talker(h)
	dh, err := LoadDialogueSync(r.srv.Global(), "talk/long.dialogue.json")
	if err != nil {
		t.Fatal(err)
	}
	name := r.w.NewEntity(gfx.Label{})
	bx := r.w.NewEntity(DialogueBox{Dialogue: dh, Name: name}, gfx.Label{})
	r.s.Play(bx, 0)
	r.s.Say(a, long, SayOptions{})
	r.s.Say(b, long, SayOptions{Speed: 1.5})
	left, right := make([]float32, 800), make([]float32, 800)
	frame := func() {
		r.tick(time.Second / 60)
		st.Render(left, right) // what the audio player would read
	}
	for range 10 {
		frame()
	}
	if n := testing.AllocsPerRun(200, frame); n != 0 {
		t.Errorf("tick: %v allocs, want 0", n)
	}
	if n := testing.AllocsPerRun(50, func() { r.s.checkFrozen(); frame() }); n != 0 {
		t.Errorf("freeze and thaw: %v allocs, want 0", n)
	}
	if !r.s.Speaking(a) || !r.s.Speaking(b) || !r.s.Playing(bx) {
		t.Fatal("the lines ended during the test: make them longer")
	}
}
