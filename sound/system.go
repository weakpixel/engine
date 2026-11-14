package sound

import (
	"time"

	"github.com/weakpixel/asset"
	"github.com/weakpixel/ecs"
	"github.com/weakpixel/ecs/system"
	"github.com/weakpixel/engine/gfx"
)

// AudioSystem turns the tick's requests into playing voices. Each Run it:
//
//  1. releases voices that finished;
//  2. plays gfx.SoundEmitter components (see below);
//  3. drains the [Queue]: ready sounds start, within the [Mixer]'s limits;
//  4. switches and crossfades music;
//  5. applies volumes (request or emitter, bus, master, fade) and pan
//     (request, or the [Listener] for positional sounds) to every voice.
//
// Emitters: a one-shot emitter starts once its sound is Ready, then sets
// Done (and Voice); the sound plays to its end even if the entity is deleted
// right away. A looping emitter keeps one voice for as long as the entity has
// the component and Done is unset; its pan and volume follow the entity and
// the component every tick, and the voice stops when the entity is deleted
// or the component removed. An emitter whose sound failed to load sets Done.
//
// Run it at the end of the tick, after gameplay has pushed its requests. It
// needs the *asset.Server resource with [RegisterLoaders] run on it; it uses
// the *Queue and *Mixer resources, creating them in Init if they are not
// set, and the optional *Listener resource. Run does not allocate in steady
// state; starting a voice allocates in the Backend.
type AudioSystem struct {
	// OnError receives errors from the Backend (for example a sample rate
	// mismatch). The request or emitter that caused it is dropped. nil
	// ignores them.
	OnError func(error)

	backend  Backend
	world    *ecs.World
	emitters ecs.View1[gfx.SoundEmitter]
	queue    *Queue
	mixer    *Mixer
	sounds   SoundStore
	music    MusicStore

	voices  []voice
	streams []*Stream
	nextID  uint32
	counts  []soundCount

	track   track   // current music; nil voice when none
	fading  []track // music fading out after a switch or stop
	pending musicRequest

	stats Stats
}

// Stats describes the AudioSystem after its last Run.
type Stats struct {
	// Voices is the number of effect voices playing (music not included).
	Voices int
	// Started is the number of effect voices started in the last Run.
	Started int
	// Dropped is the number of requests and emitter starts refused by the
	// Mixer's limits in the last Run.
	Dropped int
	// Streams is the number of stream sources playing.
	Streams int
	// Music reports whether a track is playing or fading in.
	Music bool
}

// voice is one playing effect.
type voice struct {
	id     uint32
	v      Voice
	bus    Bus
	volume float32 // request or emitter volume, resolved
	pan    float32
	atten  float32 // distance attenuation, 1 for non-positional sounds
	// owned marks the voice of a looping emitter: it lives only while its
	// emitter is seen each tick.
	owned bool
	seen  bool
	// left and right are the gains last handed to the Backend, so
	// unchanged voices are not touched.
	left, right float32
}

// track is a music voice and its fade.
type track struct {
	v     Voice
	h     gfx.MusicHandle
	level float32 // fade level, 0 to 1
	rate  float32 // level change per second; negative fades out
	gain  float32 // last gain handed to the Backend
}

type soundCount struct {
	h gfx.SoundHandle
	n int
}

// NewAudioSystem returns the system for w, playing on b. A nil b means
// [NullBackend].
func NewAudioSystem(w *ecs.World, b Backend) *AudioSystem {
	if b == nil {
		b = NullBackend{}
	}
	return &AudioSystem{backend: b, world: w, emitters: ecs.NewView1[gfx.SoundEmitter](w)}
}

// Init implements system.Initializer. It caches the stores and the Queue
// and Mixer resources, creating the latter two if they are not set.
func (s *AudioSystem) Init(ctx *system.Context) {
	srv := ecs.GetResource[asset.Server](ctx.World)
	s.sounds, s.music = SoundStoreOf(srv), MusicStoreOf(srv)
	if s.queue = ecs.GetResourceOrNil[Queue](ctx.World); s.queue == nil {
		s.queue = &Queue{}
		ecs.SetResource(ctx.World, s.queue)
	}
	if s.mixer = ecs.GetResourceOrNil[Mixer](ctx.World); s.mixer == nil {
		s.mixer = NewMixer()
		ecs.SetResource(ctx.World, s.mixer)
	}
}

// Run implements system.Runner.
func (s *AudioSystem) Run(ctx *system.Context) {
	s.stats.Started, s.stats.Dropped = 0, 0
	// Read every tick, not cached in Init: the game may set the Listener
	// later, and a lookup is a map read without allocation.
	l := ecs.GetResourceOrNil[Listener](s.world)

	s.reap()
	s.runEmitters(l)
	s.drainQueue(l)
	s.runMusic(ctx.Time.Delta)
	s.applyGains()
	s.runStreams(ctx.Time.Delta)

	s.stats.Voices = len(s.voices)
	s.stats.Streams = len(s.streams)
	s.stats.Music = s.track.v != nil
}

// Stats returns what the last Run did.
func (s *AudioSystem) Stats() Stats { return s.stats }

// Close stops every voice and track. Call it when the game ends; the system
// can run again afterwards.
func (s *AudioSystem) Close() {
	for i := range s.voices {
		s.voices[i].v.Stop()
	}
	clear(s.voices)
	s.voices = s.voices[:0]
	for _, st := range s.streams {
		st.Close()
	}
	clear(s.streams)
	s.streams = s.streams[:0]
	s.stopTrack(&s.track)
	for i := range s.fading {
		s.stopTrack(&s.fading[i])
	}
	s.fading = s.fading[:0]
	s.pending = musicRequest{}
}

// reap releases the voices that ended by themselves.
func (s *AudioSystem) reap() {
	for i := 0; i < len(s.voices); {
		if s.voices[i].v.Playing() {
			i++
			continue
		}
		s.voices[i].v.Stop()
		s.removeVoice(i)
	}
}

// removeVoice swap-removes voice i; voice order carries no meaning.
func (s *AudioSystem) removeVoice(i int) {
	last := len(s.voices) - 1
	s.voices[i] = s.voices[last]
	s.voices[last] = voice{} // drop the Voice reference for the GC
	s.voices = s.voices[:last]
}

func (s *AudioSystem) findVoice(id uint32) int {
	for i := range s.voices {
		if s.voices[i].id == id {
			return i
		}
	}
	return -1
}

func (s *AudioSystem) runEmitters(l *Listener) {
	for i := range s.voices {
		s.voices[i].seen = false
	}
	for e, em := range s.emitters.Each() {
		if em.Done {
			continue
		}
		pan, atten := float32(0), float32(1)
		if t := ecs.GetCompOrNil[gfx.Transform](s.world, e); t != nil {
			pan, atten = l.place(t.Pos)
		}
		if em.Voice != 0 && em.Loop {
			if i := s.findVoice(em.Voice); i >= 0 {
				v := &s.voices[i]
				v.seen, v.pan, v.atten = true, pan, atten
				v.volume, v.bus = volumeOf(em.Volume), Bus(em.Bus)
				continue
			}
			// The voice is gone (stopped from outside or by a backend);
			// start a new one below.
			em.Voice = 0
		}
		if em.Sound.IsZero() {
			continue
		}
		snd, state := s.sounds.Lookup(em.Sound)
		switch state {
		case asset.Ready:
		case asset.Failed, asset.Missing:
			em.Done = true
			continue
		default: // still loading: try again next tick
			continue
		}
		if len(snd.PCM) == 0 {
			em.Done = !em.Loop
			continue
		}
		if len(s.voices) >= s.mixer.maxVoices() {
			s.stats.Dropped++
			continue
		}
		id := s.start(snd, em.Loop, Bus(em.Bus), volumeOf(em.Volume), pan, atten)
		if id == 0 {
			em.Done = true
			continue
		}
		em.Voice = id
		if em.Loop {
			v := &s.voices[len(s.voices)-1]
			v.owned, v.seen = true, true
		} else {
			em.Done = true
		}
	}
	// A looping voice whose emitter was not seen lost its entity or
	// component (or was marked Done): stop it.
	for i := 0; i < len(s.voices); {
		if v := &s.voices[i]; v.owned && !v.seen {
			v.v.Stop()
			s.removeVoice(i)
			continue
		}
		i++
	}
}

func (s *AudioSystem) drainQueue(l *Listener) {
	s.counts = s.counts[:0]
	limit := s.mixer.sameSoundPerTick()
	for _, p := range s.queue.sounds {
		snd, state := s.sounds.Lookup(p.Sound)
		if state != asset.Ready || len(snd.PCM) == 0 {
			continue
		}
		if s.countSound(p.Sound) > limit || len(s.voices) >= s.mixer.maxVoices() {
			s.stats.Dropped++
			continue
		}
		pan, atten := p.Pan, float32(1)
		if p.Positional {
			pan, atten = l.place(p.At)
		}
		s.start(snd, false, p.Bus, volumeOf(p.Volume), pan, atten)
	}
	clear(s.queue.sounds) // drop handles so a long-gone burst holds nothing
	s.queue.sounds = s.queue.sounds[:0]
}

// countSound counts one more start of h in this tick and returns the new
// count. A linear scan: a tick has few distinct sounds.
func (s *AudioSystem) countSound(h gfx.SoundHandle) int {
	for i := range s.counts {
		if s.counts[i].h == h {
			s.counts[i].n++
			return s.counts[i].n
		}
	}
	s.counts = append(s.counts, soundCount{h: h, n: 1})
	return 1
}

// start starts an effect voice and returns its id, or 0 if the Backend
// failed.
func (s *AudioSystem) start(snd *Sound, loop bool, bus Bus, volume, pan, atten float32) uint32 {
	left, right := s.voiceGains(bus, volume, pan, atten)
	v, err := s.backend.PlaySound(snd, loop, left, right)
	if err != nil {
		s.report(err)
		return 0
	}
	s.nextID++
	if s.nextID == 0 { // 0 means "no voice" in SoundEmitter.Voice
		s.nextID = 1
	}
	s.voices = append(s.voices, voice{
		id: s.nextID, v: v, bus: bus, volume: volume, pan: pan, atten: atten,
		left: left, right: right,
	})
	s.stats.Started++
	return s.nextID
}

func (s *AudioSystem) voiceGains(bus Bus, volume, pan, atten float32) (left, right float32) {
	g := s.mixer.gain(bus) * volume * atten
	left, right = panGains(pan)
	return left * g, right * g
}

func (s *AudioSystem) runMusic(dt time.Duration) {
	if s.queue.music.set {
		s.pending = s.queue.music
		s.queue.music = musicRequest{}
	}
	if s.pending.set {
		s.switchMusic()
	}

	step := float32(dt.Seconds())
	if s.track.v != nil && s.track.rate > 0 {
		s.track.level = min(1, s.track.level+s.track.rate*step)
	}
	for i := 0; i < len(s.fading); {
		t := &s.fading[i]
		t.level += t.rate * step
		if t.level > 0 {
			i++
			continue
		}
		s.stopTrack(t)
		last := len(s.fading) - 1
		s.fading[i] = s.fading[last]
		s.fading[last] = track{}
		s.fading = s.fading[:last]
	}
}

// switchMusic applies the pending request, or leaves it pending while its
// track loads.
func (s *AudioSystem) switchMusic() {
	req := s.pending
	if req.track.IsZero() {
		s.fadeOutTrack(req.fade)
		s.pending = musicRequest{}
		return
	}
	if req.track == s.track.h && s.track.v != nil {
		s.pending = musicRequest{} // already playing: keep it going
		return
	}
	m, state := s.music.Lookup(req.track)
	switch state {
	case asset.Ready:
	case asset.Failed, asset.Missing:
		s.fadeOutTrack(req.fade)
		s.pending = musicRequest{}
		return
	default:
		return // loading: the current music keeps playing meanwhile
	}
	s.pending = musicRequest{}
	if m.Duration <= 0 { // the placeholder, or a file without a frame
		s.fadeOutTrack(req.fade)
		return
	}
	level, rate := float32(1), float32(0)
	if req.fade > 0 {
		level, rate = 0, float32(1/req.fade.Seconds())
	}
	gain := s.mixer.gain(BusMusic) * level
	v, err := s.backend.PlayMusic(m, gain, gain)
	if err != nil {
		s.report(err) // keep the current music rather than fall silent
		return
	}
	s.fadeOutTrack(req.fade)
	s.track = track{v: v, h: req.track, level: level, rate: rate, gain: gain}
}

// fadeOutTrack moves the current track to the fading list, or stops it at
// once for a zero fade.
func (s *AudioSystem) fadeOutTrack(fade time.Duration) {
	if s.track.v == nil {
		return
	}
	if fade <= 0 {
		s.stopTrack(&s.track)
		return
	}
	t := s.track
	t.rate = -float32(1 / fade.Seconds())
	s.fading = append(s.fading, t)
	s.track = track{}
}

func (s *AudioSystem) stopTrack(t *track) {
	if t.v != nil {
		t.v.Stop()
	}
	*t = track{}
}

func (s *AudioSystem) applyGains() {
	for i := range s.voices {
		v := &s.voices[i]
		left, right := s.voiceGains(v.bus, v.volume, v.pan, v.atten)
		if left != v.left || right != v.right {
			v.left, v.right = left, right
			v.v.SetGains(left, right)
		}
	}
	music := s.mixer.gain(BusMusic)
	s.applyTrackGain(&s.track, music)
	for i := range s.fading {
		s.applyTrackGain(&s.fading[i], music)
	}
}

func (s *AudioSystem) applyTrackGain(t *track, music float32) {
	if t.v == nil {
		return
	}
	if g := music * t.level; g != t.gain {
		t.gain = g
		t.v.SetGains(g, g)
	}
}

// AddStream starts reading src.Reader on the backend and returns the running
// stream, or the backend's error. The stream plays until [Stream.Close],
// until its reader ends, or until [AudioSystem.Close]. Its volume is the
// bus's × master × the stream's own, applied every tick like a voice's.
func (s *AudioSystem) AddStream(src StreamSource) (*Stream, error) {
	st := &Stream{bus: src.Bus, volume: volumeOf(src.Volume)}
	buffer := src.Buffer
	if buffer <= 0 {
		buffer = DefaultStreamBuffer
	}
	// Before Init there is no Mixer yet: start at the stream's own volume;
	// the first Run applies the buses.
	st.left, st.right = st.volume, st.volume
	if s.mixer != nil {
		st.left, st.right = s.voiceGains(st.bus, st.volume, 0, 1)
	}
	v, err := s.backend.PlayStream(src.Reader, src.Format, buffer, st.left, st.right)
	if err != nil {
		return nil, err
	}
	st.v = v
	s.streams = append(s.streams, st)
	return st, nil
}

// runStreams drops closed and ended streams, updates Reading and applies the
// gains of the rest.
func (s *AudioSystem) runStreams(dt time.Duration) {
	for i := 0; i < len(s.streams); {
		st := s.streams[i]
		if !st.closed && !st.v.Playing() { // the reader ended
			st.Close()
		}
		if st.closed {
			last := len(s.streams) - 1
			s.streams[i] = s.streams[last]
			s.streams[last] = nil
			s.streams = s.streams[:last]
			continue
		}
		st.observe(dt)
		left, right := s.voiceGains(st.bus, st.volume, st.pan, 1)
		if left != st.left || right != st.right {
			st.left, st.right = left, right
			st.v.SetGains(left, right)
		}
		i++
	}
}

func (s *AudioSystem) report(err error) {
	if s.OnError != nil {
		s.OnError(err)
	}
}
