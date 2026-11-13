package speech

import (
	"time"

	"github.com/weakpixel/voice"
	"github.com/weakpixel/voice/dialogue"

	"github.com/weakpixel/ecs"
	"github.com/weakpixel/engine/gfx"
)

// box is the runtime state of one dialogue box: its player and the owners
// of its speakers. It lives from the box's first Play (or AutoStart) until
// the entity is deleted or loses its DialogueBox.
type box struct {
	e    ecs.Entity
	sink ownerSink
	// p is the running player; nil before the first start and after a
	// start that found no dialogue.
	p *dialogue.Player

	// waiting is set while the dialogue of a Play is still loading; the
	// box starts with handle and from once it is ready.
	waiting bool
	handle  gfx.DialogueHandle
	from    int

	// showing is set while the system writes the line into the Labels:
	// from the start to the tick after the dialogue finished or stopped.
	showing bool
	// paused is the game's PauseDialogue; frozen is set while the speech
	// system did not run. Either holds the player, which releases the
	// sounding notes.
	paused, frozen bool
}

// ownerSink maps the owners a dialogue.Player uses (speaker i plays as
// owner i + 1) to owners of the Speech's pool, so boxes and speaking
// entities never share an owner. It drops the notes of a speaker whose
// voice the synthesizer rejected; Release and NoteOff always pass.
type ownerSink struct {
	to     voice.Sink
	owners []voice.Owner // by speaker index
	mute   []bool        // by speaker index
}

// owner maps the player's owner o to the pool's.
func (s *ownerSink) owner(o voice.Owner) (voice.Owner, bool) {
	i := int(o) - 1
	if i < 0 || i >= len(s.owners) {
		return 0, false
	}
	return s.owners[i], !s.mute[i]
}

// NoteOn implements voice.Sink.
func (s *ownerSink) NoteOn(o voice.Owner, inst voice.Instrument, note, velocity, pan float32) {
	if id, ok := s.owner(o); ok {
		s.to.NoteOn(id, inst, note, velocity, pan)
	}
}

// NoteOff implements voice.Sink.
func (s *ownerSink) NoteOff(o voice.Owner, inst voice.Instrument, note float32) {
	if i := int(o) - 1; i >= 0 && i < len(s.owners) {
		s.to.NoteOff(s.owners[i], inst, note)
	}
}

// Release implements voice.Sink.
func (s *ownerSink) Release(o voice.Owner) {
	if i := int(o) - 1; i >= 0 && i < len(s.owners) {
		s.to.Release(s.owners[i])
	}
}

// hold pauses or resumes the player from the game's pause and the freeze
// together.
func (b *box) hold() {
	if b.p == nil {
		return
	}
	if b.paused || b.frozen {
		b.p.Pause()
	} else {
		b.p.Resume()
	}
}

// Play plays the DialogueBox of entity e from line start (0 is the first),
// replacing what the box played (its notes are released, no event). It
// starts at once, or in the speech system's next tick once the dialogue has
// loaded. The box's gfx.Label shows the current line as it types out, and
// the Label of DialogueBox.Name the speaker's name; both are set to "" when
// the dialogue finishes or stops. A zero, missing or failed dialogue, or a
// start outside the dialogue's lines (reported to Config.OnError), plays
// nothing and sends DialogueFinished. Play after Close does nothing. Play
// allocates; the ticks of a running dialogue do not.
//
// Panics with "speech: Play on a dead entity" and with "speech: Play on an
// entity without a DialogueBox".
func (s *Speech) Play(e ecs.Entity, start int) {
	if s.closed {
		return
	}
	if !s.w.IsAlive(e) {
		panic("speech: Play on a dead entity")
	}
	comp := ecs.GetCompOrNil[DialogueBox](s.w, e)
	if comp == nil {
		panic("speech: Play on an entity without a DialogueBox")
	}
	b := s.boxOf(e)
	if b.p != nil {
		b.p.Close()
		b.p = nil
	}
	b.handle, b.from = comp.Dialogue, start
	b.paused = false
	b.waiting = true
	if _, st := s.dialogues.Lookup(b.handle); !loading(st) {
		s.startBox(b, comp)
	}
}

// boxOf returns e's box, creating it on the first Play.
func (s *Speech) boxOf(e ecs.Entity) *box {
	if b, ok := s.byBox[e]; ok {
		return b
	}
	b := &box{e: e}
	b.sink.to = s.sink
	s.boxes = append(s.boxes, b)
	s.byBox[e] = b
	return b
}

// startBox starts the waiting box with its dialogue as it is now.
func (s *Speech) startBox(b *box, comp *DialogueBox) {
	b.waiting = false
	d := s.dialogues.Get(b.handle) // the placeholder (nil Script) unless ready
	if d.Script == nil {
		s.events.Send(Event{Entity: b.e, Kind: DialogueFinished})
		return
	}
	s.boxOwners(b, len(d.Script.Speakers))
	p, err := dialogue.NewPlayer(&b.sink, d.Script, d.PlayerVoices(), dialogue.Options{
		Languages:   languagesOf(d),
		AutoAdvance: comp.AutoAdvance,
		Start:       b.from,
	})
	if err != nil {
		s.report(err)
		s.events.Send(Event{Entity: b.e, Kind: DialogueFinished})
		return
	}
	for _, sv := range p.Voices() {
		i := int(sv.Owner) - 1
		b.sink.mute[i] = false
		if s.patch == nil {
			continue
		}
		id := b.sink.owners[i]
		if !s.owners.needsPatch(id, sv.Voice) {
			continue
		}
		ref := d.Script.Speakers[i].Voice
		if err := s.patch.SetVoice(id, sv.Voice, d.Voices[ref].Sample); err != nil {
			s.owners.forget(id)
			s.report(err)
			b.sink.mute[i] = true
		}
	}
	b.p = p
	b.showing = true
	b.hold() // a frozen game starts the dialogue held
}

// boxOwners gives b one pool owner per speaker of its dialogue, taking or
// returning owners when the number changed (another dialogue, a reload).
func (s *Speech) boxOwners(b *box, n int) {
	for len(b.sink.owners) < n {
		b.sink.owners = append(b.sink.owners, s.owners.take())
		b.sink.mute = append(b.sink.mute, false)
	}
	for len(b.sink.owners) > n {
		last := len(b.sink.owners) - 1
		s.sink.Release(b.sink.owners[last])
		s.owners.give(b.sink.owners[last])
		b.sink.owners, b.sink.mute = b.sink.owners[:last], b.sink.mute[:last]
	}
}

// languagesOf resolves the language names of d's voices to the languages
// their files loaded.
func languagesOf(d *Dialogue) func(string) *voice.Language {
	return func(name string) *voice.Language {
		for _, v := range d.Voices {
			if v.V.Language == name {
				return v.Language
			}
		}
		return nil
	}
}

// Next is the game's "advance" input for box e: while a line types out it
// shows the rest at once (the line is done, its notes are released); while
// a finished line holds it goes on to the next line, or finishes the
// dialogue after the last. During a line's delay, while the dialogue
// loads, after the end and without a box it does nothing.
func (s *Speech) Next(e ecs.Entity) {
	if b, ok := s.byBox[e]; ok && b.p != nil && !b.waiting {
		b.p.Next()
	}
}

// StopDialogue ends box e's dialogue where it is: its notes are released,
// its Labels are cleared at the next tick and no event is sent. Play starts
// it again. Without a box it does nothing.
func (s *Speech) StopDialogue(e ecs.Entity) {
	b, ok := s.byBox[e]
	if !ok {
		return
	}
	b.waiting = false
	b.paused = false
	if b.p != nil {
		b.p.Stop()
	}
}

// PauseDialogue holds box e's dialogue: its time stops (the line, its delay
// or its hold) and its sounding notes are released. ResumeDialogue
// continues with the next notes. A dialogue that is still loading starts
// paused. Next still works while paused: it shows the rest of the line or
// goes on to the next one, which then waits for ResumeDialogue.
func (s *Speech) PauseDialogue(e ecs.Entity) {
	if b, ok := s.byBox[e]; ok {
		b.paused = true
		b.hold()
	}
}

// ResumeDialogue continues box e's paused dialogue.
func (s *Speech) ResumeDialogue(e ecs.Entity) {
	if b, ok := s.byBox[e]; ok {
		b.paused = false
		b.hold()
	}
}

// Playing reports whether box e has a dialogue that has not finished:
// speaking, holding, in a delay, paused or waiting for its file.
func (s *Speech) Playing(e ecs.Entity) bool {
	b, ok := s.byBox[e]
	return ok && (b.waiting || b.p != nil && b.p.Phase() != dialogue.Finished)
}

// Line returns box e's current line and its speaker, and whether the box
// is playing one (false before Play, while the dialogue loads and once it
// finished or stopped). Line.Text is the whole line; the box's Label shows
// the part said so far.
func (s *Speech) Line(e ecs.Entity) (dialogue.Line, dialogue.Speaker, bool) {
	b, ok := s.byBox[e]
	if !ok || b.waiting || b.p == nil || b.p.Phase() == dialogue.Finished {
		return dialogue.Line{}, dialogue.Speaker{}, false
	}
	return b.p.Line(), b.p.Speaker(), true
}

// autoStart plays the boxes with AutoStart that have not played yet.
func (s *Speech) autoStart() {
	for e, c := range s.boxView.Each() {
		if c.AutoStart {
			if _, ok := s.byBox[e]; !ok {
				s.Play(e, 0)
			}
		}
	}
}

// stepBoxes is the dialogue part of step.
func (s *Speech) stepBoxes(dt time.Duration) {
	for i := 0; i < len(s.boxes); {
		b := s.boxes[i]
		var comp *DialogueBox
		if s.w.IsAlive(b.e) {
			comp = ecs.GetCompOrNil[DialogueBox](s.w, b.e)
		}
		if comp == nil {
			s.dropBox(i)
			continue
		}
		if b.frozen {
			b.frozen = false
			b.hold()
		}
		if b.waiting {
			if _, st := s.dialogues.Lookup(b.handle); !loading(st) {
				s.startBox(b, comp)
			}
		}
		if b.p != nil && !b.waiting {
			b.p.Update(dt)
			for _, ev := range b.p.Events() {
				s.events.Send(Event{Entity: b.e, Kind: dialogueKinds[ev.Kind], Line: uint64(ev.Line)})
			}
			s.showBox(b, comp)
		}
		i++
	}
}

// showBox writes b's line into the box's Label and the speaker's name into
// the Label of comp.Name while the dialogue runs, and "" into both once
// after it ended. The texts are substrings of the script: nothing is
// allocated.
func (s *Speech) showBox(b *box, comp *DialogueBox) {
	if !b.showing {
		return
	}
	text, name := "", ""
	if b.p.Phase() != dialogue.Finished {
		text, name = b.p.Line().Text[:b.p.Revealed()], b.p.Speaker().Name
	} else {
		b.showing = false
	}
	if lb := ecs.GetCompOrNil[gfx.Label](s.w, b.e); lb != nil {
		lb.Text = text
	}
	if comp.Name != (ecs.Entity{}) && s.w.IsAlive(comp.Name) {
		if lb := ecs.GetCompOrNil[gfx.Label](s.w, comp.Name); lb != nil {
			lb.Text = name
		}
	}
}

// dropBox ends box i and gives its owners back.
func (s *Speech) dropBox(i int) {
	b := s.boxes[i]
	s.closeBox(b)
	delete(s.byBox, b.e)
	copy(s.boxes[i:], s.boxes[i+1:])
	s.boxes[len(s.boxes)-1] = nil
	s.boxes = s.boxes[:len(s.boxes)-1]
}

// closeBox closes b's player and releases and returns its owners.
func (s *Speech) closeBox(b *box) {
	if b.p != nil {
		b.p.Close()
	}
	for _, id := range b.sink.owners {
		s.sink.Release(id)
		s.owners.give(id)
	}
	b.sink.owners = b.sink.owners[:0]
}

// freezeBoxes holds every box (the speech system did not run this tick).
func (s *Speech) freezeBoxes() {
	for _, b := range s.boxes {
		if !b.frozen {
			b.frozen = true
			b.hold()
		}
	}
}
