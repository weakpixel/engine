package game

import (
	"encoding/binary"
	"testing"
	"testing/fstest"

	"github.com/weakpixel/ecs"
	"github.com/weakpixel/ecs/system"
	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/sound"
)

// silentWAV returns a 16-bit stereo WAV of frames silent frames at rate.
func silentWAV(rate, frames int) []byte {
	n := frames * 4
	var b []byte
	b = append(b, "RIFF"...)
	b = binary.LittleEndian.AppendUint32(b, uint32(36+n))
	b = append(b, "WAVEfmt "...)
	b = binary.LittleEndian.AppendUint32(b, 16)
	b = binary.LittleEndian.AppendUint16(b, 1)
	b = binary.LittleEndian.AppendUint16(b, 2)
	b = binary.LittleEndian.AppendUint32(b, uint32(rate))
	b = binary.LittleEndian.AppendUint32(b, uint32(rate*4))
	b = binary.LittleEndian.AppendUint16(b, 4)
	b = binary.LittleEndian.AppendUint16(b, 16)
	b = append(b, "data"...)
	b = binary.LittleEndian.AppendUint32(b, uint32(n))
	return append(b, make([]byte, n)...)
}

func TestAudioWiring(t *testing.T) {
	for _, noAudio := range []bool{true, false} {
		e := New(Config{
			Width: 320, Height: 180, NoAudio: noAudio,
			FS: fstest.MapFS{
				"sfx/hit.wav":     {Data: silentWAV(sound.DefaultSampleRate, 4800)},
				"music/theme.wav": {Data: silentWAV(sound.DefaultSampleRate, 48000)},
			},
		})
		w := e.World()
		if ecs.GetResource[sound.Queue](w) != e.Sound() || ecs.GetResource[sound.Mixer](w) != e.Mixer() ||
			ecs.GetResource[sound.Listener](w) != e.Listener() {
			t.Fatal("audio resources not published")
		}
		hit, err := sound.LoadSoundSync(e.Assets().Global(), "sfx/hit.wav")
		if err != nil {
			t.Fatal(err)
		}
		theme, err := sound.LoadMusicSync(e.Assets().Global(), "music/theme.wav")
		if err != nil {
			t.Fatal(err)
		}
		e.Camera().Pos = gfx.Vec2{X: 10, Y: 20}
		e.Camera().Zoom = 2
		e.Layout(320, 180)
		e.AddSystem(StageSimulate, func(*system.Context) {
			e.Sound().Play(sound.PlaySound{Sound: hit})
		})
		e.Start(func(e *Engine) {
			e.Sound().PlayMusic(theme, 0)
			ecs.NewEntity2(w, gfx.SoundEmitter{Sound: hit, Loop: true}, gfx.Transform{})
		})
		if err := e.Update(); err != nil {
			t.Fatal(err)
		}
		st := e.Audio().Stats()
		if st.Started != 2 || !st.Music {
			t.Errorf("NoAudio=%v: Stats = %+v, want 2 effects and music", noAudio, st)
		}
		if l := e.Listener(); l.Pos != (gfx.Vec2{X: 10, Y: 20}) || l.HalfWidth != 80 {
			t.Errorf("listener = %+v, want camera position, half width 320/2/2", *l)
		}
		e.Close()
	}
}
