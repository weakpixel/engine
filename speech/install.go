package speech

import (
	"fmt"
	"log"
	"time"

	"github.com/weakpixel/voice"
	"github.com/weakpixel/voice/synth"

	"github.com/weakpixel/ecs"
	"github.com/weakpixel/engine/game"
	"github.com/weakpixel/engine/sound"
)

// DefaultVoices is the polyphony of the synthesizer when Config.Voices is 0:
// enough for a few speakers at once, cheap enough for a browser (the synth
// renders on the audio pull, which in js/wasm shares the main thread).
const DefaultVoices = 24

// Config configures what [Install] adds. The zero value works: the built-in
// synthesizer with 24 voices on sound.BusVoice, a 50 ms buffer and the
// library's default reverb; errors are logged.
type Config struct {
	// Bus is the mixer bus the voices play on. The zero value (BusSFX)
	// means sound.BusVoice: speech never plays on the effects bus.
	Bus sound.Bus
	// Volume is the stream's own volume factor; 0 means 1.
	Volume float32
	// Voices is the polyphony of the synthesizer; 0 means DefaultVoices.
	Voices int
	// Buffer is the player buffer; 0 means sound.DefaultStreamBuffer.
	Buffer time.Duration
	// Reverb is the synthesizer's reverb; nil means synth.DefaultReverb.
	Reverb *synth.Reverb
	// Sink, when set, receives the notes instead of the built-in
	// synthesizer, and no stream is started (a recording sink in tests,
	// another synthesizer). If it has the synth.Stream method SetVoice, it
	// gets every speaker's instruments before its notes.
	Sink voice.Sink
	// OnError receives what goes wrong while speaking (a voice whose
	// instruments the synthesizer rejects, a stream that cannot start);
	// nil logs it. Asset errors go to the asset server's OnError.
	OnError func(error)
}

// Install adds speech to e and returns its controller, which is also set
// as a World resource (ecs.GetResource[speech.Speech]). It registers the
// asset loaders ([RegisterLoaders]; do not call it as well) and the
// library's loaders of the kinds when e has a library, starts the
// synthesizer at e's sample rate on the mixer, and adds the speech system to
// StagePost of the root scene and a check to the audio stage. Call it once,
// after game.New and before the first Update.
//
// Under game.Config.NoAudio (servers, tests, the level editor) nothing reads
// the synthesizer: text, reveal and events behave the same, silently, and
// the notes are dropped every tick. A pause scene above the root or a
// Tool's pause holds every line and releases its notes; the lines continue
// when the root runs again.
//
// Voices in the library's global/ package are not loaded at boot (the boot
// starts in game.New, before Install); they load when first used.
func Install(e *game.Engine, cfg Config) *Speech {
	srv := e.Assets()
	RegisterLoaders(srv)
	if lib := e.Library(); lib != nil {
		SetLibraryLoaders(lib)
	}
	onErr := cfg.OnError
	if onErr == nil {
		onErr = func(err error) { log.Printf("speech: %v", err) }
	}
	w := e.World()
	sink := cfg.Sink
	var syn *synth.Stream
	if sink == nil {
		var err error
		syn, err = newSynth(e.SampleRate(), cfg)
		if err != nil {
			panic("speech: " + err.Error()) // bad Config: a programmer error
		}
		sink = syn
	}
	s := newSpeech(w, srv, sink, onErr)
	if syn != nil {
		bus := cfg.Bus
		if bus == sound.BusSFX {
			bus = sound.BusVoice
		}
		st, err := e.Audio().AddStream(sound.StreamSource{
			Reader: syn, Format: sound.StreamFloat32, Bus: bus, Volume: cfg.Volume, Buffer: cfg.Buffer,
		})
		if err != nil {
			// Speech still works without sound; the notes are dropped.
			onErr(fmt.Errorf("stream: %w", err))
		} else {
			s.synth, s.stream = syn, st
		}
	}
	ecs.SetResource(w, s)
	e.AddRunner(game.StagePost, s)
	e.AddAudioRunner(audioCheck{s})
	return s
}

// newSynth creates the built-in synthesizer for cfg.
//
// It reports an invalid Voices or Reverb.
func newSynth(rate int, cfg Config) (*synth.Stream, error) {
	voices := cfg.Voices
	if voices == 0 {
		voices = DefaultVoices
	}
	st, err := synth.NewStream(synth.StreamConfig{SampleRate: rate, Format: synth.FormatFloat32, Voices: voices})
	if err != nil {
		return nil, err
	}
	if cfg.Reverb != nil {
		if err := st.SetReverb(*cfg.Reverb); err != nil {
			return nil, err
		}
	}
	return st, nil
}
