package sound

import (
	"bytes"
	"fmt"
	"io"
	"path"
	"strings"
	"time"

	"github.com/hajimehoshi/ebiten/v2/audio/mp3"
	"github.com/hajimehoshi/ebiten/v2/audio/vorbis"
	"github.com/hajimehoshi/ebiten/v2/audio/wav"

	"github.com/weakpixel/asset"
)

// Sound is a short effect, fully decoded on a worker to 16-bit signed
// little-endian stereo PCM at the server's sample rate (see
// [RegisterLoaders]). Playing it decodes nothing.
//
// Load it with [LoadSound]. The placeholder is an empty Sound: it plays
// silence.
type Sound struct {
	// PCM is the decoded audio, 4 bytes per frame (left, right). It is
	// shared by every voice that plays the sound: never modify it.
	PCM []byte
	// SampleRate is the rate PCM was decoded at.
	SampleRate int
}

// Duration returns how long the sound plays.
func (s *Sound) Duration() time.Duration {
	if s.SampleRate <= 0 {
		return 0
	}
	frames := int64(len(s.PCM) / bytesPerFrame)
	return time.Duration(frames) * time.Second / time.Duration(s.SampleRate)
}

// Music is a track that is kept compressed and decoded while it plays, so a
// three-minute song costs its file size, not 30 MB of PCM.
//
// Load it with [LoadMusic]. The placeholder is an empty Music: it plays
// silence.
type Music struct {
	// Data is the file as loaded. It is shared by every voice that plays the
	// track: never modify it.
	Data []byte
	// Format is the file's encoding, from its extension.
	Format Format
	// SampleRate is the rate the track is decoded to when it plays.
	SampleRate int
	// Duration is the track's length, from its header.
	Duration time.Duration
}

// Format is the encoding of an audio file.
type Format uint8

// Audio formats, chosen by file extension (case-insensitive).
const (
	FormatUnknown Format = iota
	FormatWAV            // .wav
	FormatOgg            // .ogg (Ogg Vorbis)
	FormatMP3            // .mp3
)

// String returns "wav", "ogg", "mp3" or "unknown".
func (f Format) String() string {
	switch f {
	case FormatWAV:
		return "wav"
	case FormatOgg:
		return "ogg"
	case FormatMP3:
		return "mp3"
	}
	return "unknown"
}

// FormatOf returns the format for a file name by its extension.
func FormatOf(name string) Format {
	switch strings.ToLower(path.Ext(name)) {
	case ".wav":
		return FormatWAV
	case ".ogg":
		return FormatOgg
	case ".mp3":
		return FormatMP3
	}
	return FormatUnknown
}

// bytesPerFrame is the size of one stereo 16-bit frame, the layout every
// decoder of Ebiten's audio packages produces.
const bytesPerFrame = 4

// stream is the part of the Ebiten decoders' *Stream types the package uses.
type stream interface {
	io.ReadSeeker
	Length() int64
}

// decode opens data as format, resampled to sampleRate. The decoders read
// only the header here; the samples are decoded as the stream is read.
func decode(format Format, sampleRate int, data []byte) (stream, error) {
	r := bytes.NewReader(data)
	switch format {
	case FormatWAV:
		return wav.DecodeWithSampleRate(sampleRate, r)
	case FormatOgg:
		return vorbis.DecodeWithSampleRate(sampleRate, r)
	case FormatMP3:
		return mp3.DecodeWithSampleRate(sampleRate, r)
	}
	return nil, fmt.Errorf("unsupported audio format (want .wav, .ogg or .mp3)")
}

// soundLoader decodes the whole file on the worker: Finish only stores it.
type soundLoader struct{ rate int }

func (l soundLoader) Decode(ctx *asset.DecodeContext, data []byte) (Sound, error) {
	s, err := decode(FormatOf(ctx.Key.Path), l.rate, data)
	if err != nil {
		return Sound{}, err
	}
	pcm, err := io.ReadAll(s)
	if err != nil {
		return Sound{}, err
	}
	// A truncated file can end mid-frame; drop the partial frame so voices
	// can rely on whole frames.
	pcm = pcm[:len(pcm)/bytesPerFrame*bytesPerFrame]
	return Sound{PCM: pcm, SampleRate: l.rate}, nil
}

func (soundLoader) Finish(_ *asset.FinishContext, s Sound) (Sound, error) { return s, nil }

func (soundLoader) Unload(*Sound) {}

// musicLoader checks the header on the worker, so a broken track fails at
// load time instead of when it starts to play, and keeps the bytes.
type musicLoader struct{ rate int }

func (l musicLoader) Decode(ctx *asset.DecodeContext, data []byte) (Music, error) {
	f := FormatOf(ctx.Key.Path)
	s, err := decode(f, l.rate, data)
	if err != nil {
		return Music{}, err
	}
	frames := s.Length() / bytesPerFrame
	return Music{
		Data:       data,
		Format:     f,
		SampleRate: l.rate,
		Duration:   time.Duration(frames) * time.Second / time.Duration(l.rate),
	}, nil
}

func (musicLoader) Finish(_ *asset.FinishContext, m Music) (Music, error) { return m, nil }

func (musicLoader) Unload(*Music) {}

// Size reports the decoded PCM to [asset.Stats] and the keep-alive budget.
func (soundLoader) Size(s *Sound) int64 { return int64(len(s.PCM)) }

// Size reports the file bytes Music keeps to [asset.Stats] and the keep-alive
// budget.
func (musicLoader) Size(m *Music) int64 { return int64(len(m.Data)) }

// DefaultSampleRate is the sample rate [RegisterLoaders] uses for 0, and the
// one game.Engine opens the audio device with.
const DefaultSampleRate = 48000

// RegisterLoaders registers [Sound] and [Music] and their loaders on srv.
// sampleRate is the rate sounds are decoded to; it must match the rate of
// the audio device (the Ebiten audio.Context) they play on. 0 means
// [DefaultSampleRate].
//
// It makes no audio calls, so it needs no audio device and may run on any
// goroutine that owns srv. It panics like [asset.Register] if a type is
// already registered.
func RegisterLoaders(srv *asset.Server, sampleRate int) {
	if sampleRate <= 0 {
		sampleRate = DefaultSampleRate
	}
	asset.Register[Sound, Sound](srv, soundLoader{rate: sampleRate}, Sound{SampleRate: sampleRate})
	asset.Register[Music, Music](srv, musicLoader{rate: sampleRate}, Music{SampleRate: sampleRate})
}
