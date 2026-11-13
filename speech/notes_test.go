package speech

import (
	"testing"
	"time"

	"github.com/weakpixel/asset"
)

func TestLineNotesAndPosition(t *testing.T) {
	r := newRig(t, asset.Options{})
	e := r.talker(r.voice("chirpy"))
	if notes := r.s.LineNotes(e, nil); len(notes) != 0 {
		t.Fatalf("notes without a line: %v", notes)
	}
	r.s.Say(e, hello, SayOptions{Speed: 2})
	notes := r.s.LineNotes(e, nil)
	if len(notes) == 0 {
		t.Fatal("no notes")
	}
	kinds := map[NoteKind]bool{}
	for i, n := range notes {
		if n.Off < n.On || i > 0 && n.On < notes[i-1].On {
			t.Fatalf("note %d %+v out of order", i, n)
		}
		kinds[n.Kind] = true
	}
	if !kinds[NoteVowel] || !kinds[NoteMotif] {
		t.Errorf("kinds %v, want vowels and the motif", kinds)
	}
	at, length := r.s.LinePosition(e)
	if at != 0 || length <= 0 {
		t.Fatalf("position at the start %v of %v", at, length)
	}
	dt := time.Second / 60
	r.tick(dt)
	r.tick(dt)
	if at, _ := r.s.LinePosition(e); at != 4*dt { // Speed 2: two ticks are four in line time
		t.Errorf("after two ticks at %v, want %v", at, 4*dt)
	}
	r.s.Pause(e)
	r.tick(dt)
	if at, _ := r.s.LinePosition(e); at != 4*dt {
		t.Errorf("paused line moved to %v", at)
	}
	r.s.Resume(e)
	r.until(e, dt)
	if at, l := r.s.LinePosition(e); at != l {
		t.Errorf("completed line at %v of %v", at, l)
	}
}

func TestStep(t *testing.T) {
	r := newRig(t, asset.Options{})
	e := r.talker(r.voice("chirpy"))
	r.s.Say(e, hello, SayOptions{})
	for range 600 {
		if !r.s.Speaking(e) {
			break
		}
		r.s.Step(time.Second / 60) // no Run: a paused tool
		r.s.checkFrozen()
	}
	if r.s.Speaking(e) || label(r.w, e) != hello {
		t.Errorf("Step did not play the line: speaking %v, label %q", r.s.Speaking(e), label(r.w, e))
	}
	r.noneSounding()
	r.s.Close()
	r.s.Step(time.Second) // harmless after Close
}

func TestNoteKindString(t *testing.T) {
	for k, want := range map[NoteKind]string{NoteVowel: "vowel", NoteMotif: "motif", NotePercussion: "percussion"} {
		if got := k.String(); got != want {
			t.Errorf("%d: %q, want %q", k, got, want)
		}
	}
}
