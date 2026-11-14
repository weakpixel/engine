package speech

import (
	"time"

	"github.com/weakpixel/voice"
	"github.com/weakpixel/voice/synth"

	"github.com/weakpixel/asset"
	"github.com/weakpixel/ecs"
	"github.com/weakpixel/ecs/system"
	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/sound"
)

// SayOptions tune one line of [Speech.Say].
type SayOptions struct {
	// Speed scales the line on top of Speaker.Speed: 2 is twice as fast.
	// 0 means 1.
	Speed float32
	// Instant shows all text at once and plays the motif only.
	Instant bool
	// Voice, when not zero, speaks with this voice instead of the entity's
	// Speaker.Voice. It lets an entity without a Speaker speak.
	Voice gfx.VoiceHandle
}

// patcher is a sink that needs the instruments of an owner's voice before
// its notes: the built-in synth.Stream.
type patcher interface {
	SetVoice(owner voice.Owner, v *voice.Voice, load synth.SampleLoader) error
}

// Speech is the controller of speaking entities: [Install] creates it and
// sets it as a World resource. Use it from the game goroutine only (systems
// and Start hooks), like the World.
//
// Every note it starts ends: Skip, Stop, a new Say, Pause, a frozen root
// scene, the deletion of the entity, the loss of its Speaker and Close
// release the line's notes; Next, a new Play, StopDialogue, PauseDialogue,
// the freeze, the deletion of the box, the loss of its DialogueBox and
// Close release a dialogue's.
type Speech struct {
	w      *ecs.World
	voices VoiceStore
	sink   voice.Sink
	patch  patcher // sink as a patcher, or nil
	stream *sound.Stream
	synth  *synth.Stream
	events *system.Events[Event]
	onErr  func(error)

	owners owners
	lines  []*line              // in the order of the first Say, for determinism
	byEnt  map[ecs.Entity]*line // the same lines by entity

	dialogues DialogueStore
	boxes     []*box              // in the order of the first Play
	byBox     map[ecs.Entity]*box // the same boxes by entity
	boxView   ecs.View1[DialogueBox]

	ran    bool // the StagePost system ran since the last audio-stage check
	closed bool
}

// newSpeech returns a controller that plays on sink with the voices and
// dialogues of srv.
func newSpeech(w *ecs.World, srv *asset.Server, sink voice.Sink, onErr func(error)) *Speech {
	s := &Speech{
		w:         w,
		voices:    VoiceStoreOf(srv),
		dialogues: DialogueStoreOf(srv),
		sink:      sink,
		events:    system.EventsOf[Event](w),
		onErr:     onErr,
		byEnt:     map[ecs.Entity]*line{},
		byBox:     map[ecs.Entity]*box{},
		boxView:   ecs.NewView1[DialogueBox](w),
	}
	s.patch, _ = sink.(patcher)
	return s
}

// Say speaks text on entity e with its Speaker's voice (or opts.Voice),
// replacing the line e is saying (that line gets a Stopped event). The line
// starts at once, or in the speech system's next tick once its voice has
// loaded; then it sends Started. If e has a gfx.Label, the system sets its
// Text to the part said so far every tick until the line ends. Text is
// plain: inline voice markup works in dialogues only. Say after Close does
// nothing. Say allocates (it composes the line); the ticks of a running line
// do not.
//
// Panics with "speech: Say on a dead entity" and with "speech: Say on an
// entity without a Speaker and without SayOptions.Voice".
func (s *Speech) Say(e ecs.Entity, text string, opts SayOptions) {
	if s.closed {
		return
	}
	if !s.w.IsAlive(e) {
		panic("speech: Say on a dead entity")
	}
	comp := ecs.GetCompOrNil[Speaker](s.w, e)
	h := opts.Voice
	if h.IsZero() {
		if comp == nil {
			panic("speech: Say on an entity without a Speaker and without SayOptions.Voice")
		}
		h = comp.Voice
	}
	l := s.lineOf(e)
	l.speaker = comp != nil
	l.text, l.handle, l.opts = text, h, opts
	l.paused = false
	l.waiting = true
	if _, st := s.voices.Lookup(h); !loading(st) {
		s.start(l)
	}
}

// loading reports whether an asset in state st may still become ready.
func loading(st asset.State) bool {
	return st == asset.Queued || st == asset.Decoding || st == asset.Waiting
}

// lineOf returns e's line, creating it (and taking an owner) on the first
// Say.
func (s *Speech) lineOf(e ecs.Entity) *line {
	if l, ok := s.byEnt[e]; ok {
		return l
	}
	l := &line{e: e, owner: s.owners.take()}
	l.sink.to = s.sink
	l.sp = voice.NewSpeaker(&l.sink, l.owner)
	s.lines = append(s.lines, l)
	s.byEnt[e] = l
	return l
}

// start says the waiting line with its voice as it is now: the loaded
// voice, or the silent placeholder of a missing, failed or (for Skip and
// Stop) still loading one.
func (s *Speech) start(l *line) {
	// Get gives the silent placeholder for a zero, missing, failed, stale
	// or still loading voice.
	v := s.voices.Get(l.handle)
	l.waiting = false
	l.sink.mute = v.Silent
	if !l.sink.mute && s.patch != nil && s.owners.needsPatch(l.owner, v.V) {
		if err := s.patch.SetVoice(l.owner, v.V, v.Sample); err != nil {
			s.owners.forget(l.owner)
			s.report(err)
			l.sink.mute = true
		}
	}
	speed := float32(1)
	if c := ecs.GetCompOrNil[Speaker](s.w, l.e); c != nil {
		l.sink.pan = c.Pan
		if c.Speed > 0 {
			speed = c.Speed
		}
	}
	if l.opts.Speed > 0 {
		speed *= l.opts.Speed
	}
	l.sp.Say(v.V, l.text, voice.SayOptions{Language: v.Language, Speed: speed, Instant: l.opts.Instant})
	l.at, l.speed = 0, float64(speed)
	l.showing = true
	l.hold() // a frozen game starts the line held
}

// Skip ends e's line at once: all text is shown, its notes are released and
// it sends Skipped. A line still waiting for its voice starts silently and
// is skipped. Without a line it does nothing.
func (s *Speech) Skip(e ecs.Entity) {
	if l := s.running(e); l != nil {
		l.sp.Skip()
	}
}

// Stop ends e's line where it is (the shown text stays), releases its notes
// and sends Stopped. Without a line it does nothing.
func (s *Speech) Stop(e ecs.Entity) {
	if l := s.running(e); l != nil {
		l.sp.Stop()
	}
}

// running returns e's line, started if it was waiting, or nil.
func (s *Speech) running(e ecs.Entity) *line {
	l, ok := s.byEnt[e]
	if !ok {
		return nil
	}
	if l.waiting {
		s.start(l)
	}
	return l
}

// Pause holds e's line: its time stops and its sounding notes are
// released. Resume continues with the next notes. A waiting line starts
// paused.
func (s *Speech) Pause(e ecs.Entity) {
	if l, ok := s.byEnt[e]; ok {
		l.paused = true
		if !l.waiting {
			l.hold()
		}
	}
}

// Resume continues e's paused line.
func (s *Speech) Resume(e ecs.Entity) {
	if l, ok := s.byEnt[e]; ok {
		l.paused = false
		if !l.waiting {
			l.hold()
		}
	}
}

// Speaking reports whether e has a line that has not ended: playing,
// paused or waiting for its voice.
func (s *Speech) Speaking(e ecs.Entity) bool {
	l, ok := s.byEnt[e]
	return ok && (l.waiting || !l.sp.Done())
}

// Revealed returns how many bytes of e's current line are shown: text[:n]
// is always whole characters. It is 0 without a line and while the line
// waits for its voice.
func (s *Speech) Revealed(e ecs.Entity) int {
	l, ok := s.byEnt[e]
	if !ok || l.waiting {
		return 0
	}
	return l.sp.Revealed()
}

// Stream returns the stream the built-in synthesizer plays on (its
// volume and pan apply on top of the bus), or nil when Config.Sink is set.
func (s *Speech) Stream() *sound.Stream { return s.stream }

// Close stops every line, releases every owner and stops the stream. Later
// calls of Say do nothing, and the system does nothing. Closing twice is
// harmless.
func (s *Speech) Close() {
	if s.closed {
		return
	}
	for _, l := range s.lines {
		l.sp.Close()
		l.sp.Update(0) // collects the Stopped event without advancing
		s.flush(l)
		s.sink.Release(l.owner)
	}
	s.lines = s.lines[:0]
	clear(s.byEnt)
	for _, b := range s.boxes {
		s.closeBox(b)
	}
	s.boxes = s.boxes[:0]
	clear(s.byBox)
	s.closed = true
	if s.stream != nil {
		s.stream.Close()
	}
}

// Run is the speech system (StagePost of the root): it starts the dialogue
// boxes with AutoStart and those whose dialogue finished loading, advances
// them and writes their Labels, starts lines whose voice finished loading, advances every line by the tick, writes the
// reveal into Labels, sends the events and ends the lines of deleted
// entities and of entities that lost their Speaker. A tick of running lines
// allocates nothing.
func (s *Speech) Run(ctx *system.Context) {
	if s.closed {
		return
	}
	s.ran = true
	s.step(ctx.Time.Delta)
}

// step is Run for dt.
func (s *Speech) step(dt time.Duration) {
	s.autoStart()
	s.stepBoxes(dt)
	for i := 0; i < len(s.lines); {
		l := s.lines[i]
		comp := (*Speaker)(nil)
		alive := s.w.IsAlive(l.e)
		if alive {
			comp = ecs.GetCompOrNil[Speaker](s.w, l.e)
		}
		if !alive || l.speaker && comp == nil {
			s.drop(i)
			continue
		}
		if l.frozen {
			l.frozen = false
			if !l.waiting {
				l.hold()
			}
		}
		if comp != nil {
			l.sink.pan = comp.Pan
		}
		if l.waiting {
			if _, st := s.voices.Lookup(l.handle); !loading(st) {
				s.start(l)
			}
		}
		if !l.waiting {
			if !l.sp.Done() && !l.sp.Paused() {
				l.at += time.Duration(float64(dt) * l.speed)
			}
			l.sp.Update(dt)
			s.flush(l)
			s.show(l)
		}
		i++
	}
}

// show writes the reveal of l into the entity's Label while the line runs,
// and once more after it ended. The text is a substring of the line, so
// nothing is allocated.
func (s *Speech) show(l *line) {
	if !l.showing {
		return
	}
	if lb := ecs.GetCompOrNil[gfx.Label](s.w, l.e); lb != nil {
		lb.Text = l.text[:l.sp.Revealed()]
	}
	if l.sp.Done() {
		l.showing = false
	}
}

// flush sends the events of l's Speaker since its previous Update.
func (s *Speech) flush(l *line) {
	for _, ev := range l.sp.Events() {
		s.events.Send(Event{Entity: l.e, Kind: eventKinds[ev.Kind], Line: ev.Line})
	}
}

// drop ends line i (Stopped if it was running) and gives its owner back.
func (s *Speech) drop(i int) {
	l := s.lines[i]
	l.sp.Stop()
	l.sp.Update(0) // collects the Stopped event without advancing
	s.flush(l)
	s.sink.Release(l.owner)
	s.owners.give(l.owner)
	delete(s.byEnt, l.e)
	copy(s.lines[i:], s.lines[i+1:]) // keep the order of the first Say
	s.lines[len(s.lines)-1] = nil
	s.lines = s.lines[:len(s.lines)-1]
}

// checkFrozen runs in the audio stage, every tick: when the speech system
// did not run since the last check (a pause scene froze the root, a Tool
// paused the game), the lines are held so no note hangs. While nobody reads
// the synthesizer's stream (no audio, a browser before the first click),
// the notes queued for it are dropped, so they do not play as a burst once
// reading starts; patches and releases are kept.
func (s *Speech) checkFrozen() {
	if s.closed {
		return
	}
	if !s.ran {
		for _, l := range s.lines {
			if !l.frozen {
				l.frozen = true
				if !l.waiting {
					l.hold()
				}
			}
		}
		s.freezeBoxes()
	}
	s.ran = false
	if s.synth != nil && !s.stream.Reading() {
		s.synth.DropNotes()
	}
}

func (s *Speech) report(err error) {
	if s.onErr != nil {
		s.onErr(err)
	}
}

// audioCheck is the audio-stage runner of a Speech.
type audioCheck struct{ s *Speech }

// Run implements system.Runner.
func (a audioCheck) Run(*system.Context) { a.s.checkFrozen() }
