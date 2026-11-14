// Package sound plays sound effects and music.
//
// It has a data half and a runtime half, like package anim. The data half is
// the asset types [Sound] (a short effect, decoded to PCM on a worker) and
// [Music] (a track kept compressed and streamed while it plays), which
// [RegisterLoaders] makes loadable from .wav, .ogg and .mp3 files.
//
// The runtime half mirrors rendering. Gameplay systems push requests into
// the [Queue] resource ([Queue.Play], [Queue.PlayMusic]); the [AudioSystem]
// drains it once per tick, applies the [Mixer]'s bus volumes and voice
// limits, crossfades music, plays gfx.SoundEmitter components and hands the
// result to a [Backend]. A sound that is not loaded yet is skipped: the
// placeholder of an audio asset is silence.
//
// The Backend is the only part that touches the audio device:
// [NewEbitenBackend] plays through Ebiten's audio package, [NullBackend]
// plays nothing (tests, servers, headless tools). The guide is
// doc/guide/engine/audio.md.
package sound
