package sound

import (
	"testing"

	"github.com/weakpixel/ecs"
	"github.com/weakpixel/engine/gfx"
)

// BenchmarkRunEmitters measures one AudioSystem run with 500 looping
// emitters that have a Transform, so every run places each emitter against
// the listener. The voices are started before the timer, with the voice
// limit raised to fit them.
func BenchmarkRunEmitters(b *testing.B) {
	s := newAudioScene(b)
	s.run()
	s.mixer().MaxVoices = 500
	ecs.SetResource(s.w, &Listener{HalfWidth: 100, Range: 400})
	jump := s.sound("sfx/jump.wav")
	for i := range 500 {
		ecs.NewEntity2(s.w,
			gfx.SoundEmitter{Sound: jump, Loop: true},
			gfx.Transform{Pos: gfx.Vec2{X: float32(i - 250)}},
		)
	}
	s.run() // start the voices
	if len(s.b.voices) != 500 {
		b.Fatalf("started %d voices, want 500", len(s.b.voices))
	}
	b.ReportAllocs()
	for b.Loop() {
		s.run()
	}
}
