package sound

import (
	"bytes"
	"errors"
	"testing"
	"time"

	"github.com/weakpixel/asset"
	"github.com/weakpixel/ecs"
	"github.com/weakpixel/ecs/system"
	"github.com/weakpixel/engine/gfx"
)

const tick = 100 * time.Millisecond

// audioScene is a world with the test sounds, a fake backend and an
// AudioSystem in a schedule.
type audioScene struct {
	t     testing.TB
	srv   *asset.Server
	w     *ecs.World
	b     *fakeBackend
	sys   *AudioSystem
	sched system.Schedule
	errs  []error
}

func newAudioScene(t testing.TB) *audioScene {
	s := &audioScene{t: t, srv: newServer(t), w: ecs.NewWorld(), b: &fakeBackend{}}
	ecs.SetResource(s.w, s.srv)
	s.sys = NewAudioSystem(s.w, s.b)
	s.sys.OnError = func(err error) { s.errs = append(s.errs, err) }
	s.sched.AddRunner(s.sys)
	return s
}

func (s *audioScene) sound(key string) gfx.SoundHandle { return mustSound(s.t, s.srv, key) }
func (s *audioScene) music(key string) gfx.MusicHandle { return mustMusic(s.t, s.srv, key) }

func (s *audioScene) run()             { s.sched.Run(s.w, tick) }
func (s *audioScene) queue() *Queue    { return ecs.GetResource[Queue](s.w) }
func (s *audioScene) mixer() *Mixer    { return ecs.GetResource[Mixer](s.w) }
func (s *audioScene) last() *fakeVoice { return s.b.voices[len(s.b.voices)-1] }

func near(a, b float32) bool { return a-b < 1e-4 && b-a < 1e-4 }

func (s *audioScene) wantGains(v *fakeVoice, left, right float32) {
	s.t.Helper()
	if !near(v.left, left) || !near(v.right, right) {
		s.t.Errorf("gains = %v, %v, want %v, %v", v.left, v.right, left, right)
	}
}

func TestInitCreatesResources(t *testing.T) {
	s := newAudioScene(t)
	s.run()
	if !ecs.HasResource[Queue](s.w) || !ecs.HasResource[Mixer](s.w) {
		t.Fatal("Queue and Mixer resources not created")
	}
	if s.mixer().Volume(BusSFX) != 1 || s.mixer().Master() != 1 {
		t.Error("default volumes are not 1")
	}
}

func TestQueuePlaysReadySounds(t *testing.T) {
	s := newAudioScene(t)
	s.run()
	hit := s.sound("sfx/hit.wav")
	s.mixer().SetVolume(BusUI, 0.5)
	s.mixer().SetMaster(0.8)
	s.queue().Play(PlaySound{Sound: hit, Volume: 0.5, Pan: 0.5, Bus: BusUI})
	s.run()

	if len(s.b.voices) != 1 {
		t.Fatalf("started %d voices, want 1", len(s.b.voices))
	}
	v := s.last()
	if v.loop || v.sound != SoundStoreOf(s.srv).Get(hit) {
		t.Errorf("voice = %+v", v)
	}
	g := float32(0.5 * 0.5 * 0.8)
	s.wantGains(v, 0.5*g, g)
	if st := s.sys.Stats(); st.Voices != 1 || st.Started != 1 {
		t.Errorf("Stats = %+v", st)
	}
	if s.queue().Len() != 0 {
		t.Error("queue not drained")
	}
}

func TestQueueSkipsUnavailableSounds(t *testing.T) {
	s := newAudioScene(t)
	s.run()
	_, _ = LoadSoundSync(s.srv.Global(), "sfx/bad.wav")
	bad := LoadSound(s.srv.Global(), "sfx/bad.wav")
	empty := s.sound("sfx/empty.wav")
	loading := LoadSound(s.srv.Global(), "sfx/jump.wav") // no Update: still queued
	for _, h := range []gfx.SoundHandle{{}, loading, bad, empty} {
		s.queue().Play(PlaySound{Sound: h})
	}
	s.run()
	if len(s.b.voices) != 0 {
		t.Errorf("started %d voices for unavailable sounds", len(s.b.voices))
	}
	s.run() // a skipped request is not retried
	if len(s.b.voices) != 0 {
		t.Error("skipped request was retried")
	}
}

func TestQueueLimits(t *testing.T) {
	s := newAudioScene(t)
	s.run()
	hit, jump := s.sound("sfx/hit.wav"), s.sound("sfx/jump.wav")
	for range 5 {
		s.queue().Play(PlaySound{Sound: hit})
	}
	s.queue().Play(PlaySound{Sound: jump})
	s.run()
	if st := s.sys.Stats(); st.Started != DefaultSameSoundPerTick+1 || st.Dropped != 5-DefaultSameSoundPerTick {
		t.Errorf("same-sound limit: Stats = %+v", st)
	}

	s.mixer().MaxVoices = 4 // 3 playing
	s.mixer().SameSoundPerTick = 10
	for range 3 {
		s.queue().Play(PlaySound{Sound: jump})
	}
	s.run()
	if st := s.sys.Stats(); st.Started != 1 || st.Dropped != 2 || st.Voices != 4 {
		t.Errorf("voice limit: Stats = %+v", st)
	}
}

func TestFinishedVoicesAreReleased(t *testing.T) {
	s := newAudioScene(t)
	s.run()
	s.queue().Play(PlaySound{Sound: s.sound("sfx/hit.wav")})
	s.run()
	v := s.last()
	v.finish()
	s.run()
	if !v.stopped || s.sys.Stats().Voices != 0 {
		t.Errorf("finished voice: stopped %v, Voices %d", v.stopped, s.sys.Stats().Voices)
	}
}

func TestVolumeChangesReachPlayingVoices(t *testing.T) {
	s := newAudioScene(t)
	s.run()
	s.queue().Play(PlaySound{Sound: s.sound("sfx/hit.wav")})
	s.run()
	v := s.last()
	calls := v.setCalls
	s.run()
	if v.setCalls != calls {
		t.Error("unchanged gains were set again")
	}
	s.mixer().SetVolume(BusSFX, 0.25)
	s.run()
	s.wantGains(v, 0.25, 0.25)
	s.mixer().SetMaster(0)
	s.run()
	s.wantGains(v, 0, 0)
}

func TestVolumeZeroMeansFull(t *testing.T) {
	tests := []struct{ in, want float32 }{{0, 1}, {-1, 0}, {0.3, 0.3}, {2, 2}}
	for _, tt := range tests {
		if got := volumeOf(tt.in); got != tt.want {
			t.Errorf("volumeOf(%v) = %v, want %v", tt.in, got, tt.want)
		}
	}
}

func TestPositionalRequest(t *testing.T) {
	s := newAudioScene(t)
	s.run()
	hit := s.sound("sfx/hit.wav")

	s.queue().Play(PlaySound{Sound: hit, Positional: true, At: gfx.Vec2{X: 50}})
	s.run()
	s.wantGains(s.last(), 1, 1) // no Listener: centered, full volume

	ecs.SetResource(s.w, &Listener{Pos: gfx.Vec2{X: 100}, HalfWidth: 100, Range: 200})
	s.queue().Play(PlaySound{Sound: hit, Positional: true, At: gfx.Vec2{X: 50}, Pan: 1})
	s.run()
	// 50 left of the listener: pan -0.5, attenuation 1 - 50/200.
	s.wantGains(s.last(), 0.75, 0.5*0.75)
}

func TestBackendErrorsAreReported(t *testing.T) {
	s := newAudioScene(t)
	s.run()
	s.b.err = errors.New("device gone")
	s.queue().Play(PlaySound{Sound: s.sound("sfx/hit.wav")})
	s.run()
	if len(s.errs) != 1 || s.sys.Stats().Voices != 0 {
		t.Errorf("errors %v, voices %d", s.errs, s.sys.Stats().Voices)
	}
}

func TestOneShotEmitter(t *testing.T) {
	s := newAudioScene(t)
	s.run()
	hit := s.sound("sfx/hit.wav")
	e := ecs.NewEntity1(s.w, gfx.SoundEmitter{Sound: hit})
	s.run()
	em := ecs.GetComp[gfx.SoundEmitter](s.w, e)
	if len(s.b.voices) != 1 || !em.Done || em.Voice == 0 {
		t.Fatalf("voices %d, emitter %+v", len(s.b.voices), *em)
	}
	v := s.last()
	s.w.DeleteEntity(e)
	s.run()
	if v.stopped || len(s.b.voices) != 1 {
		t.Error("one-shot sound was cut when its entity was deleted, or replayed")
	}
}

func TestEmitterWaitsForLoadingSound(t *testing.T) {
	s := newAudioScene(t)
	s.run()
	h := LoadSound(s.srv.Global(), "sfx/jump.wav")
	e := ecs.NewEntity1(s.w, gfx.SoundEmitter{Sound: h})
	s.run()
	if len(s.b.voices) != 0 || ecs.GetComp[gfx.SoundEmitter](s.w, e).Done {
		t.Fatal("emitter played or gave up before its sound loaded")
	}
	for !SoundStoreOf(s.srv).Ready(h) {
		s.srv.Update()
	}
	s.run()
	if len(s.b.voices) != 1 {
		t.Error("emitter did not start once its sound was ready")
	}
}

func TestEmitterWithFailedSoundIsDone(t *testing.T) {
	s := newAudioScene(t)
	s.run()
	_, _ = LoadSoundSync(s.srv.Global(), "sfx/bad.wav")
	e := ecs.NewEntity1(s.w, gfx.SoundEmitter{Sound: LoadSound(s.srv.Global(), "sfx/bad.wav"), Loop: true})
	s.run()
	if !ecs.GetComp[gfx.SoundEmitter](s.w, e).Done || len(s.b.voices) != 0 {
		t.Error("emitter with a failed sound is not Done")
	}
}

func TestLoopingEmitter(t *testing.T) {
	s := newAudioScene(t)
	ecs.SetResource(s.w, &Listener{HalfWidth: 100})
	s.run()
	jump := s.sound("sfx/jump.wav")
	e := ecs.NewEntity2(s.w, gfx.SoundEmitter{Sound: jump, Loop: true, Volume: 0.5}, gfx.Transform{Pos: gfx.Vec2{X: -100}})
	s.run()
	if len(s.b.voices) != 1 || !s.last().loop {
		t.Fatalf("voices %d", len(s.b.voices))
	}
	v := s.last()
	s.wantGains(v, 0.5, 0)

	// The voice follows the entity and the component.
	ecs.GetComp[gfx.Transform](s.w, e).Pos.X = 50
	ecs.GetComp[gfx.SoundEmitter](s.w, e).Volume = 1
	s.run()
	s.wantGains(v, 0.5, 1)
	if len(s.b.voices) != 1 {
		t.Error("looping emitter started a second voice")
	}

	s.w.DeleteEntity(e)
	s.run()
	if !v.stopped || s.sys.Stats().Voices != 0 {
		t.Error("deleting the entity did not stop its loop")
	}
}

func TestLoopingEmitterStopsWhenRemovedOrDone(t *testing.T) {
	s := newAudioScene(t)
	s.run()
	jump := s.sound("sfx/jump.wav")
	a := ecs.NewEntity1(s.w, gfx.SoundEmitter{Sound: jump, Loop: true})
	b := ecs.NewEntity1(s.w, gfx.SoundEmitter{Sound: jump, Loop: true})
	s.run()
	va, vb := s.b.voices[0], s.b.voices[1]

	ecs.RemoveComp[gfx.SoundEmitter](s.w, a)
	ecs.GetComp[gfx.SoundEmitter](s.w, b).Done = true
	s.run()
	if !va.stopped || !vb.stopped {
		t.Errorf("stopped: removed %v, done %v", va.stopped, vb.stopped)
	}
}

func TestLoopingEmitterRestartsLostVoice(t *testing.T) {
	s := newAudioScene(t)
	s.run()
	ecs.NewEntity1(s.w, gfx.SoundEmitter{Sound: s.sound("sfx/jump.wav"), Loop: true})
	s.run()
	s.last().finish() // e.g. the device dropped it
	s.run()
	if len(s.b.playing()) != 1 || len(s.b.voices) != 2 {
		t.Errorf("voices %d, playing %d", len(s.b.voices), len(s.b.playing()))
	}
}

func TestMusicCut(t *testing.T) {
	s := newAudioScene(t)
	s.run()
	a, b := s.music("music/a.wav"), s.music("music/b.wav")
	s.mixer().SetVolume(BusMusic, 0.5)
	s.queue().PlayMusic(a, 0)
	s.run()
	va := s.last()
	if va.music == nil || !s.sys.Stats().Music {
		t.Fatal("music did not start")
	}
	s.wantGains(va, 0.5, 0.5)

	s.queue().PlayMusic(a, time.Second) // same track: no restart
	s.run()
	if len(s.b.voices) != 1 || va.stopped {
		t.Fatal("requesting the playing track restarted it")
	}

	s.queue().PlayMusic(b, 0)
	s.run()
	if !va.stopped || len(s.b.playing()) != 1 || s.last().music != MusicStoreOf(s.srv).Get(b) {
		t.Error("cut to the next track failed")
	}
}

func TestMusicCrossfade(t *testing.T) {
	s := newAudioScene(t)
	s.run()
	a, b := s.music("music/a.wav"), s.music("music/b.wav")
	s.queue().PlayMusic(a, 0)
	s.run()
	va := s.last()

	s.queue().PlayMusic(b, 4*tick)
	s.run() // the switch tick advances the fade by one step
	vb := s.last()
	s.wantGains(va, 0.75, 0.75)
	s.wantGains(vb, 0.25, 0.25)
	s.run()
	s.run()
	s.wantGains(va, 0.25, 0.25)
	s.wantGains(vb, 0.75, 0.75)
	s.run()
	if !va.stopped {
		t.Error("old track not stopped after the fade")
	}
	s.wantGains(vb, 1, 1)
}

func TestStopMusicFades(t *testing.T) {
	s := newAudioScene(t)
	s.run()
	s.queue().PlayMusic(s.music("music/a.wav"), 0)
	s.run()
	v := s.last()
	s.queue().StopMusic(2 * tick)
	s.run()
	s.wantGains(v, 0.5, 0.5)
	if s.sys.Stats().Music {
		t.Error("Stats.Music is set for fading-out music")
	}
	s.run()
	if !v.stopped {
		t.Error("music not stopped after the fade")
	}
}

func TestMusicWaitsForLoadingTrack(t *testing.T) {
	s := newAudioScene(t)
	s.run()
	s.queue().PlayMusic(s.music("music/a.wav"), 0)
	s.run()
	va := s.last()
	next := LoadMusic(s.srv.Global(), "music/b.wav")
	s.queue().PlayMusic(next, 0)
	s.run()
	if va.stopped || len(s.b.voices) != 1 {
		t.Fatal("current music stopped before the next track loaded")
	}
	for !MusicStoreOf(s.srv).Ready(next) {
		s.srv.Update()
	}
	s.run()
	if !va.stopped || len(s.b.voices) != 2 {
		t.Error("next track did not start once loaded")
	}
}

func TestMusicFailedOrZeroTrackStops(t *testing.T) {
	for _, name := range []string{"zero", "failed", "empty"} {
		t.Run(name, func(t *testing.T) {
			s := newAudioScene(t)
			s.run()
			s.queue().PlayMusic(s.music("music/a.wav"), 0)
			s.run()
			va := s.last()
			var h gfx.MusicHandle
			switch name {
			case "failed":
				_, _ = LoadMusicSync(s.srv.Global(), "music/bad.ogg")
				h = LoadMusic(s.srv.Global(), "music/bad.ogg")
			case "empty":
				h = s.music("music/empty.wav")
			}
			s.queue().PlayMusic(h, 0)
			s.run()
			if !va.stopped || len(s.b.voices) != 1 {
				t.Error("music did not stop")
			}
		})
	}
}

func TestCloseStopsEverything(t *testing.T) {
	s := newAudioScene(t)
	s.run()
	ecs.NewEntity1(s.w, gfx.SoundEmitter{Sound: s.sound("sfx/jump.wav"), Loop: true})
	s.queue().PlayMusic(s.music("music/a.wav"), 0)
	s.run()
	s.queue().PlayMusic(s.music("music/b.wav"), time.Second)
	s.run()
	s.sys.Close()
	if p := s.b.playing(); len(p) != 0 {
		t.Errorf("%d voices still playing after Close", len(p))
	}
}

func TestNullBackend(t *testing.T) {
	var b NullBackend
	once, _ := b.PlaySound(&Sound{}, false, 1, 1)
	loop, _ := b.PlaySound(&Sound{}, true, 1, 1)
	music, _ := b.PlayMusic(&Music{}, 1, 1)
	if once.Playing() || !loop.Playing() || !music.Playing() {
		t.Error("one-shot must end at once, loops and music must play")
	}
	loop.Stop()
	if loop.Playing() {
		t.Error("stopped loop still playing")
	}
}

func TestAudioSystemRunDoesNotAllocate(t *testing.T) {
	s := newAudioScene(t)
	ecs.SetResource(s.w, &Listener{HalfWidth: 100, Range: 500})
	s.run()
	jump := s.sound("sfx/jump.wav")
	for i := range 8 {
		ecs.NewEntity2(s.w, gfx.SoundEmitter{Sound: jump, Loop: true}, gfx.Transform{Pos: gfx.Vec2{X: float32(i * 10)}})
	}
	s.queue().PlayMusic(s.music("music/a.wav"), time.Second)
	if _, err := s.sys.AddStream(StreamSource{Reader: bytes.NewReader(nil), Bus: BusVoice}); err != nil {
		t.Fatal(err)
	}
	s.run()
	s.run()
	ctx := &system.Context{World: s.w, Time: system.Time{Delta: tick}}
	allocs := testing.AllocsPerRun(100, func() {
		s.sys.Run(ctx)
	})
	if allocs != 0 {
		t.Errorf("Run allocates %v times per tick", allocs)
	}
}
