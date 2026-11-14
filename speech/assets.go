package speech

import (
	"bytes"
	"errors"
	"fmt"
	"path"
	"slices"
	"strings"

	"github.com/weakpixel/voice"
	"github.com/weakpixel/voice/dialogue"
	"github.com/weakpixel/voice/synth"
	"github.com/weakpixel/voice/wav"

	"github.com/weakpixel/asset"
)

// File suffixes of the speech assets.
const (
	LanguageSuffix = ".lang.json"
	VoiceSuffix    = ".voice.json"
	DialogueSuffix = ".dialogue.json"
)

// Language is a loaded *.lang.json file: how text becomes phonemes.
//
// The placeholder is the library's default language.
type Language struct {
	// L is the language. It is immutable and shared.
	L *voice.Language
}

// Sample is a mono recording a voice's instrument plays, loaded from a 16-bit
// PCM WAV file (mixed to mono at the file's own rate; the synthesizer
// resamples per note). Root and loop points come from the voice file.
//
// It has no handle of its own: it loads only as a dependency of a [Voice].
// The placeholder has a nil S.
type Sample struct {
	// S is the recording. It is shared by every voice that uses the file:
	// never modify it.
	S *synth.Sample
}

// Voice is how one character speaks: a loaded *.voice.json file with its
// language and samples resolved, or a starter voice ([voice.Preset]) when
// the key is a starter name such as "chirpy".
//
// The placeholder (a missing or failed voice) is the "soft" starter with
// Silent set: it reveals text at the default speed and plays no notes.
type Voice struct {
	// V is the voice. It is shared by every speaker that uses the asset:
	// never modify it; copy it to change a field.
	V *voice.Voice
	// Language is the voice's language: the file its language field names,
	// or the default language.
	Language *voice.Language
	// Silent is set on the placeholder.
	Silent bool

	samples map[string]*synth.Sample
}

// Sample returns the recording an instrument names in the voice file
// ("hum.wav"), so (*Voice).Sample can be passed as a [synth.SampleLoader].
// It returns an error for a name the file does not use.
func (v *Voice) Sample(name string) (*synth.Sample, error) {
	if s, ok := v.samples[name]; ok {
		return s, nil
	}
	return nil, fmt.Errorf("speech: voice %q has no sample %q", v.V.Name, name)
}

// Dialogue is a loaded *.dialogue.json file with the voice of every speaker.
//
// The placeholder has a nil Script: a failed dialogue plays nothing.
type Dialogue struct {
	// Script is the dialogue. It is shared: never modify it.
	Script *dialogue.Script
	// Voices holds the voice of every speaker, keyed by the speaker's voice
	// reference ([dialogue.Speaker].Voice), like [dialogue.LoadVoices].
	// Speakers that share a reference share the value.
	Voices map[string]*Voice
}

// PlayerVoices returns the voices keyed by reference, in the form
// [dialogue.NewPlayer] takes. It allocates a new map.
func (d *Dialogue) PlayerVoices() map[string]*voice.Voice {
	m := make(map[string]*voice.Voice, len(d.Voices))
	for ref, v := range d.Voices {
		m[ref] = v.V
	}
	return m
}

// IsStarter reports whether name is a starter voice ("chirpy"): a voice key
// or a dialogue speaker's voice reference that loads [voice.Preset] instead
// of a file.
func IsStarter(name string) bool { return slices.Contains(voice.PresetNames(), name) }

// LanguageKey returns the key of the language a voice file names in its
// language field: "" for the default language, a path relative to the voice for a
// name ending in .lang.json, otherwise <name>.lang.json next to the voice.
// The result is relative to the voice file, as [asset.Dep] takes it.
func LanguageKey(name string) string {
	switch {
	case name == "":
		return ""
	case strings.HasSuffix(name, LanguageSuffix):
		return name
	}
	return name + LanguageSuffix
}

// languageLoader decodes a *.lang.json file on the worker.
type languageLoader struct{}

func (languageLoader) Decode(_ *asset.DecodeContext, data []byte) (*voice.Language, error) {
	return voice.LoadLanguage(bytes.NewReader(data))
}

func (languageLoader) Finish(_ *asset.FinishContext, l *voice.Language) (Language, error) {
	return Language{L: l}, nil
}

func (languageLoader) Unload(*Language) {}

// sampleLoader decodes a WAV file on the worker.
type sampleLoader struct{}

func (sampleLoader) Decode(_ *asset.DecodeContext, data []byte) (*synth.Sample, error) {
	pcm, rate, err := wav.Read(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	return &synth.Sample{Data: pcm, Rate: rate}, nil
}

func (sampleLoader) Finish(_ *asset.FinishContext, s *synth.Sample) (Sample, error) {
	return Sample{S: s}, nil
}

func (sampleLoader) Unload(*Sample) {}

// Size reports the decoded samples to [asset.Stats] and the keep-alive
// budget.
func (sampleLoader) Size(s *Sample) int64 {
	if s.S == nil {
		return 0
	}
	return int64(len(s.S.Data)) * 4
}

// voiceSpec is a decoded voice with its declared dependencies.
type voiceSpec struct {
	v        *voice.Voice
	language asset.DepRef[Language] // zero for the default language
	samples  []sampleDep
}

type sampleDep struct {
	name string
	dep  asset.DepRef[Sample]
}

// voiceLoader decodes a *.voice.json file, or a starter voice by its key.
type voiceLoader struct {
	languages *asset.Store[Language]
	samples   *asset.Store[Sample]
}

// Builtin reports whether the key is a starter voice name: those read no
// file.
func (voiceLoader) Builtin(k asset.Key) bool { return k.Sub == "" && IsStarter(k.Path) }

func (voiceLoader) Decode(ctx *asset.DecodeContext, data []byte) (voiceSpec, error) {
	if data == nil && IsStarter(ctx.Key.Path) {
		v, err := voice.Preset(ctx.Key.Path)
		return voiceSpec{v: v}, err
	}
	v, err := voice.LoadVoice(bytes.NewReader(data))
	if err != nil {
		return voiceSpec{}, err
	}
	spec := voiceSpec{v: v}
	if key := LanguageKey(v.Language); key != "" {
		spec.language = asset.Dep[Language](ctx, key)
	}
	for _, name := range sampleNames(v) {
		spec.samples = append(spec.samples, sampleDep{name: name, dep: asset.Dep[Sample](ctx, name)})
	}
	return spec, nil
}

// sampleNames returns the distinct sample names of v's instruments and
// percussion sounds, in instrument order.
func sampleNames(v *voice.Voice) []string {
	var names []string
	add := func(s voice.InstrumentSpec) {
		if s.Sample != "" && !slices.Contains(names, s.Sample) {
			names = append(names, s.Sample)
		}
	}
	for _, s := range v.Instruments {
		add(s)
	}
	for _, p := range v.Percussion {
		add(p.Spec)
	}
	return names
}

func (l voiceLoader) Finish(ctx *asset.FinishContext, spec voiceSpec) (Voice, error) {
	out := Voice{V: spec.v, Language: voice.DefaultLanguage()}
	var zero asset.DepRef[Language]
	if spec.language != zero {
		out.Language = l.languages.Get(asset.Resolve(ctx, spec.language)).L
	}
	if len(spec.samples) > 0 {
		out.samples = make(map[string]*synth.Sample, len(spec.samples))
		for _, s := range spec.samples {
			out.samples[s.name] = l.samples.Get(asset.Resolve(ctx, s.dep)).S
		}
	}
	return out, nil
}

func (voiceLoader) Unload(*Voice) {}

// dialogueSpec is a decoded dialogue with its declared voice dependencies.
type dialogueSpec struct {
	script *dialogue.Script
	files  []voiceDep        // speaker voices that are files
	starts map[string]*Voice // starter voices, by name
}

type voiceDep struct {
	ref string
	dep asset.DepRef[Voice]
}

// dialogueLoader decodes a *.dialogue.json file. Starter voices are made in
// Decode; voice files are dependencies relative to the dialogue.
type dialogueLoader struct{ voices *asset.Store[Voice] }

func (dialogueLoader) Decode(ctx *asset.DecodeContext, data []byte) (dialogueSpec, error) {
	s, err := dialogue.Load(bytes.NewReader(data))
	if err != nil {
		return dialogueSpec{}, err
	}
	spec := dialogueSpec{script: s, starts: map[string]*Voice{}}
	var errs []error
	for i, sp := range s.Speakers {
		ref := sp.Voice
		if _, ok := spec.starts[ref]; ok || slices.ContainsFunc(spec.files, func(d voiceDep) bool { return d.ref == ref }) {
			continue
		}
		if IsStarter(ref) {
			v, err := voice.Preset(ref)
			if err != nil {
				errs = append(errs, &voice.FieldError{File: "dialogue", Path: fmt.Sprintf("speakers[%d].voice", i), Message: err.Error()})
				continue
			}
			spec.starts[ref] = &Voice{V: v, Language: voice.DefaultLanguage()}
			continue
		}
		spec.files = append(spec.files, voiceDep{ref: ref, dep: asset.Dep[Voice](ctx, path.Clean(ref))})
	}
	return spec, errors.Join(errs...)
}

func (l dialogueLoader) Finish(ctx *asset.FinishContext, spec dialogueSpec) (Dialogue, error) {
	voices := make(map[string]*Voice, len(spec.starts)+len(spec.files))
	for ref, v := range spec.starts {
		voices[ref] = v
	}
	for _, f := range spec.files {
		voices[f.ref] = l.voices.Get(asset.Resolve(ctx, f.dep))
	}
	return Dialogue{Script: spec.script, Voices: voices}, nil
}

func (dialogueLoader) Unload(*Dialogue) {}

// RegisterLoaders registers [Language], [Sample], [Voice] and [Dialogue] and
// their loaders on srv. The assets do not depend on the stream's sample
// rate: samples keep their file's rate and the synthesizer resamples them
// per note.
//
// It makes no audio calls and may run on any goroutine that owns srv. It
// panics like [asset.Register] if a type is already registered.
func RegisterLoaders(srv *asset.Server) {
	languages := asset.Register[Language, *voice.Language](srv, languageLoader{}, Language{L: voice.DefaultLanguage()})
	samples := asset.Register[Sample, *synth.Sample](srv, sampleLoader{}, Sample{})
	voices := asset.Register[Voice, voiceSpec](srv, voiceLoader{languages: languages, samples: samples}, placeholderVoice())
	asset.Register[Dialogue, dialogueSpec](srv, dialogueLoader{voices: voices}, Dialogue{})
}

// placeholderVoice is the silent "soft" starter.
func placeholderVoice() Voice {
	v, err := voice.Preset("soft")
	if err != nil {
		panic("speech: the soft starter voice is missing: " + err.Error())
	}
	return Voice{V: v, Language: voice.DefaultLanguage(), Silent: true}
}
