package speech

import (
	"os"
	"slices"
	"testing"
	"time"

	"github.com/weakpixel/voice"
	"github.com/weakpixel/voice/dialogue"
	"github.com/weakpixel/voice/voicetest"

	"github.com/weakpixel/asset"
	"github.com/weakpixel/ecs"
	"github.com/weakpixel/engine/gfx"
)

const tick = time.Second / 60

// village is the library example's dialogue: four starter voices, inline
// voices ({wren}...{/}) and impressions ({laugh~}...{/}).
const village = "talk/village.dialogue.json"

func dialogueRig(t *testing.T) *rig {
	t.Helper()
	return dialogueRigOpts(t, asset.Options{})
}

func dialogueRigOpts(t *testing.T, opts asset.Options) *rig {
	t.Helper()
	fsys := speechFiles()
	data, err := os.ReadFile("testdata/village.dialogue.json")
	if err != nil {
		t.Fatal(err)
	}
	fsys[village] = mapFile(string(data))
	opts.FS = fsys
	return newRig(t, opts)
}

func (r *rig) dialogue(key string) gfx.DialogueHandle {
	r.t.Helper()
	h, err := LoadDialogueSync(r.srv.Global(), key)
	if err != nil {
		r.t.Fatal(err)
	}
	return h
}

// box is a dialogue box of dialogue h with a Label and a name Label.
func (r *rig) box(h gfx.DialogueHandle, auto bool) (box, name ecs.Entity) {
	name = r.w.NewEntity(gfx.Label{})
	box = r.w.NewEntity(DialogueBox{Dialogue: h, Name: name, AutoAdvance: auto}, gfx.Label{})
	return box, name
}

// boxEvents returns the dialogue events read since the last call, as
// kind and line index.
func (r *rig) boxEvents() []Event {
	var out []Event
	for ev := range r.reader.Read() {
		out = append(out, Event{Kind: ev.Kind, Line: ev.Line})
	}
	return out
}

// untilPhase ticks until box b's player is in phase p.
func (r *rig) untilPhase(b ecs.Entity, p dialogue.Phase) {
	r.t.Helper()
	for range int(time.Minute / tick) {
		if r.s.byBox[b].p.Phase() == p {
			return
		}
		r.tick(tick)
	}
	r.t.Fatalf("the box never reached %v", p)
}

func TestDialogueLinesAdvance(t *testing.T) {
	r := dialogueRig(t)
	b, name := r.box(r.dialogue(village), false)
	r.s.Play(b, 0)
	r.tick(tick)
	ln, sp, ok := r.s.Line(b)
	if !ok || sp.ID != "wren" || ln.Text != "Hello there, traveler! Are you lost?" {
		t.Fatalf("Line = %q %q %v", ln.Text, sp.ID, ok)
	}
	if got := label(r.w, name); got != "Wren" {
		t.Errorf("name label %q", got)
	}
	if got := label(r.w, b); len(got) == 0 || len(got) == len(ln.Text) {
		t.Errorf("label %q after one tick, want part of the line", got)
	}
	r.untilPhase(b, dialogue.Holding)
	if got := label(r.w, b); got != ln.Text {
		t.Errorf("label %q, want the whole line", got)
	}
	r.s.Next(b) // go on to Bram
	r.tick(tick)
	if _, sp, _ := r.s.Line(b); sp.ID != "bram" || label(r.w, name) != "Bram" {
		t.Errorf("after Next: speaker %q, name %q", sp.ID, label(r.w, name))
	}
	r.s.Next(b) // skip the typing
	r.tick(tick)
	if ln, _, _ := r.s.Line(b); label(r.w, b) != ln.Text {
		t.Errorf("Next did not show the rest of the line: %q", label(r.w, b))
	}
	want := []Event{{Kind: LineStarted}, {Kind: LineDone}, {Kind: LineStarted, Line: 1}, {Kind: LineDone, Line: 1}}
	if got := r.boxEvents(); !slices.Equal(got, want) {
		t.Errorf("events %v, want %v", got, want)
	}
	r.noneSounding()
}

// TestDialogueSpansSwitchVoices checks that a span speaks with the other
// speaker's owner: Kip's line 8 has {wren}Follow the river{/}.
func TestDialogueSpansSwitchVoices(t *testing.T) {
	r := dialogueRig(t)
	b, _ := r.box(r.dialogue(village), false)
	r.s.Play(b, 8)
	r.tick(tick)
	bx := r.s.byBox[b]
	if len(bx.sink.owners) != 4 || r.s.owners.inUse() != 4 {
		t.Fatalf("owners %v, in use %d, want one per speaker", bx.sink.owners, r.s.owners.inUse())
	}
	r.untilPhase(b, dialogue.Holding)
	kip, wren := bx.sink.owners[2], bx.sink.owners[0]
	on := map[voice.Owner]int{}
	for _, c := range r.sink.Calls() {
		if c.Kind == voicetest.NoteOn {
			on[c.Owner]++
		}
	}
	if on[kip] == 0 || on[wren] == 0 || len(on) != 2 {
		t.Errorf("notes by owner %v, want Kip's %d and Wren's %d", on, kip, wren)
	}
	r.noneSounding()
}

func TestDialogueFinished(t *testing.T) {
	r := dialogueRig(t)
	b, name := r.box(r.dialogue(village), true)
	r.s.Play(b, 12)
	for range int(time.Minute / tick) {
		if !r.s.Playing(b) {
			break
		}
		r.tick(tick)
	}
	r.tick(tick)
	if r.s.Playing(b) || label(r.w, b) != "" || label(r.w, name) != "" {
		t.Fatalf("after the end: playing %v, labels %q %q", r.s.Playing(b), label(r.w, b), label(r.w, name))
	}
	evs := r.boxEvents()
	if len(evs) != 5 || evs[4] != (Event{Kind: DialogueFinished, Line: 13}) {
		t.Errorf("events %v, want two lines and DialogueFinished of line 13", evs)
	}
	if _, _, ok := r.s.Line(b); ok {
		t.Error("Line after the end")
	}
	r.s.Play(b, 0) // plays again
	r.tick(tick)
	if !r.s.Playing(b) {
		t.Error("Play after the end did not play")
	}
}

func TestDialogueBoxDeletedMidLine(t *testing.T) {
	r := dialogueRig(t)
	b, _ := r.box(r.dialogue(village), false)
	r.s.Play(b, 0)
	for range 20 {
		r.tick(tick)
	}
	if len(r.sink.Calls()) == 0 {
		t.Fatal("no notes before the deletion")
	}
	r.w.DeleteEntity(b)
	r.tick(tick)
	r.noneSounding()
	if r.s.owners.inUse() != 0 || len(r.s.boxes) != 0 || r.s.Playing(b) {
		t.Errorf("owners %d, boxes %d after the deletion", r.s.owners.inUse(), len(r.s.boxes))
	}
	r.s.Next(b) // a dead box is ignored
}

func TestDialogueStopAndLoseComponent(t *testing.T) {
	r := dialogueRig(t)
	b, _ := r.box(r.dialogue(village), false)
	r.s.Play(b, 0)
	for range 20 {
		r.tick(tick)
	}
	r.s.StopDialogue(b)
	r.tick(tick)
	r.noneSounding()
	if r.s.Playing(b) || label(r.w, b) != "" {
		t.Errorf("after StopDialogue: playing %v, label %q", r.s.Playing(b), label(r.w, b))
	}
	r.boxEvents()
	r.s.Play(b, 0)
	for range 20 {
		r.tick(tick)
	}
	ecs.RemoveComp[DialogueBox](r.w, b)
	r.tick(tick)
	r.noneSounding()
	if r.s.owners.inUse() != 0 {
		t.Errorf("%d owners after the box lost its DialogueBox", r.s.owners.inUse())
	}
}

func TestPauseDialogue(t *testing.T) {
	r := dialogueRig(t)
	b, _ := r.box(r.dialogue(village), false)
	r.s.Play(b, 0)
	for range 20 {
		r.tick(tick)
	}
	r.s.PauseDialogue(b)
	r.tick(tick)
	n := len(label(r.w, b))
	calls := len(r.sink.Calls())
	for range 60 {
		r.tick(tick)
	}
	if len(label(r.w, b)) != n || len(r.sink.Calls()) != calls {
		t.Fatalf("the paused dialogue moved: %d to %d bytes, %d to %d calls", n, len(label(r.w, b)), calls, len(r.sink.Calls()))
	}
	r.noneSounding()
	r.s.ResumeDialogue(b)
	r.untilPhase(b, dialogue.Holding)
	if ln, _, _ := r.s.Line(b); label(r.w, b) != ln.Text {
		t.Errorf("after ResumeDialogue: %q", label(r.w, b))
	}
}

func TestDialogueWaitsAndFails(t *testing.T) {
	r := dialogueRigOpts(t, asset.Options{Workers: 1})
	h := LoadDialogue(r.srv.Global(), village) // still loading
	b, _ := r.box(h, false)
	r.s.Play(b, 0)
	r.s.PauseDialogue(b)
	if !r.s.Playing(b) {
		t.Fatal("a loading dialogue does not play")
	}
	if _, _, ok := r.s.Line(b); ok {
		t.Error("Line while loading")
	}
	for i := 0; !DialogueStoreOf(r.srv).Ready(h); i++ {
		if i == 1000 {
			t.Fatal("the dialogue never loaded")
		}
		time.Sleep(time.Millisecond)
		r.srv.Update()
	}
	r.tick(tick)
	if bx := r.s.byBox[b]; bx.p == nil || !bx.p.Paused() {
		t.Fatal("the loaded dialogue did not start paused")
	}

	nh := LoadDialogue(r.srv.Global(), "talk/none.dialogue.json")
	none, _ := r.box(nh, false)
	for _, st := DialogueStoreOf(r.srv).Lookup(nh); loading(st); _, st = DialogueStoreOf(r.srv).Lookup(nh) {
		time.Sleep(time.Millisecond)
		r.srv.Update()
	}
	r.boxEvents()
	r.s.Play(none, 0)
	r.tick(tick)
	if got := r.boxEvents(); !slices.Equal(got, []Event{{Kind: DialogueFinished}}) {
		t.Errorf("a missing dialogue sent %v", got)
	}

	bad, _ := r.box(r.dialogue(village), false)
	r.s.Play(bad, 99)
	if len(r.errs) != 1 || r.s.Playing(bad) {
		t.Errorf("a start outside the lines: errors %v, playing %v", r.errs, r.s.Playing(bad))
	}
	mustPanic(t, "speech: Play on an entity without a DialogueBox", func() { r.s.Play(r.w.NewEntity(gfx.Label{}), 0) })
}

func TestDialogueAutoStartAndClose(t *testing.T) {
	r := dialogueRig(t)
	b := r.w.NewEntity(DialogueBox{Dialogue: r.dialogue(village), AutoStart: true}, gfx.Label{})
	r.tick(tick)
	if !r.s.Playing(b) || label(r.w, b) == "" {
		t.Fatal("AutoStart did not play")
	}
	r.s.StopDialogue(b)
	r.tick(tick)
	if r.s.Playing(b) {
		t.Error("AutoStart played again after StopDialogue")
	}
	r.s.Play(b, 0)
	for range 10 {
		r.tick(tick)
	}
	r.s.Close()
	r.noneSounding()
	if r.s.owners.inUse() != 0 || r.s.Playing(b) {
		t.Error("Close kept the box")
	}
}
