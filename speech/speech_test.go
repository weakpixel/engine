package speech

import (
	"slices"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/weakpixel/voice"
	"github.com/weakpixel/voice/voicetest"

	"github.com/weakpixel/asset"
	"github.com/weakpixel/ecs"
	"github.com/weakpixel/ecs/system"
	"github.com/weakpixel/engine/gfx"
)

const hello = "Hello there, traveler! How are you?"

// rig is a Speech on a recording sink, driven tick by tick without an
// engine.
type rig struct {
	t      *testing.T
	w      *ecs.World
	srv    *asset.Server
	sink   *voicetest.RecordingSink
	s      *Speech
	reader *system.EventReader[Event]
	errs   []error
}

func newRig(t *testing.T, opts asset.Options) *rig {
	t.Helper()
	if opts.FS == nil {
		opts.FS = speechFiles()
	}
	srv, _ := newServer(t, opts.FS.(fstest.MapFS), opts)
	r := &rig{t: t, w: ecs.NewWorld(), srv: srv, sink: voicetest.NewRecordingSink(4096)}
	r.s = newSpeech(r.w, srv, r.sink, func(err error) { r.errs = append(r.errs, err) })
	r.reader = system.NewEventReader[Event](r.w)
	return r
}

// voice loads key synchronously.
func (r *rig) voice(key string) gfx.VoiceHandle {
	r.t.Helper()
	h, err := LoadVoiceSync(r.srv.Global(), key)
	if err != nil {
		r.t.Fatal(err)
	}
	return h
}

// talker is an entity with a Speaker of voice h and a Label.
func (r *rig) talker(h gfx.VoiceHandle) ecs.Entity {
	return r.w.NewEntity(Speaker{Voice: h, Speed: 1}, gfx.Label{})
}

// tick runs one tick of dt: the StagePost system, then the audio check.
func (r *rig) tick(dt time.Duration) {
	r.s.Run(&system.Context{World: r.w, Time: system.Time{Delta: dt}})
	r.s.checkFrozen()
}

// until ticks at dt until e stops speaking, at most a minute of game time.
func (r *rig) until(e ecs.Entity, dt time.Duration) {
	r.t.Helper()
	for range int(time.Minute / dt) {
		if !r.s.Speaking(e) {
			return
		}
		r.tick(dt)
	}
	r.t.Fatal("the line never ended")
}

// events returns the kinds of the events read since the last call.
func (r *rig) events() []EventKind {
	var out []EventKind
	for ev := range r.reader.Read() {
		out = append(out, ev.Kind)
	}
	return out
}

func (r *rig) noneSounding() {
	r.t.Helper()
	for o := range voice.Owner(8) {
		if on := r.sink.Sounding(o); len(on) > 0 {
			r.t.Fatalf("owner %d still has %d notes", o, len(on))
		}
	}
}

func label(w *ecs.World, e ecs.Entity) string { return ecs.GetComp[gfx.Label](w, e).Text }

func TestSayLabelReveal(t *testing.T) {
	r := newRig(t, asset.Options{})
	e := r.talker(r.voice("chirpy"))
	r.s.Say(e, hello, SayOptions{})
	var shown []string
	for r.s.Speaking(e) {
		r.tick(time.Second / 60)
		shown = append(shown, label(r.w, e))
	}
	if got := label(r.w, e); got != hello {
		t.Fatalf("label after the line %q", got)
	}
	for i := 1; i < len(shown); i++ {
		if !strings.HasPrefix(shown[i], shown[i-1]) {
			t.Fatalf("reveal went back: %q then %q", shown[i-1], shown[i])
		}
	}
	if len(slices.Compact(slices.Clone(shown))) < 5 {
		t.Errorf("the text did not type out: %q", shown)
	}
	if got := r.events(); !slices.Equal(got, []EventKind{Started, Completed}) {
		t.Errorf("events %v", got)
	}
	if !slices.ContainsFunc(r.sink.Calls(), func(c voicetest.Call) bool { return c.Kind == voicetest.NoteOn }) {
		t.Error("no note played")
	}
	r.noneSounding()
	// The Label is left alone after the line.
	ecs.GetComp[gfx.Label](r.w, e).Text = "mine"
	r.tick(time.Second / 60)
	if got := label(r.w, e); got != "mine" {
		t.Errorf("label after the end rewritten to %q", got)
	}
}

// TestSayRevealRate checks that the reveal depends on the time, not on the
// tick rate.
func TestSayRevealRate(t *testing.T) {
	reveal := func(hz int) []int {
		r := newRig(t, asset.Options{})
		e := r.talker(r.voice("chirpy"))
		r.s.Say(e, hello, SayOptions{})
		var at []int
		per := hz / 6 // sample every 1/6 s
		for i := 1; r.s.Speaking(e); i++ {
			r.tick(time.Second / time.Duration(hz))
			if i%per == 0 {
				at = append(at, r.s.Revealed(e))
			}
		}
		return at
	}
	slow, fast := reveal(30), reveal(144)
	n := min(len(slow), len(fast))
	if n < 3 || !slices.Equal(slow[:n-1], fast[:n-1]) {
		t.Errorf("30 Hz %v, 144 Hz %v", slow, fast)
	}
}

func TestSkipStopReplaceRelease(t *testing.T) {
	for _, tc := range []struct {
		name string
		end  func(r *rig, e ecs.Entity)
		want []EventKind
		text string
	}{
		{"skip", func(r *rig, e ecs.Entity) { r.s.Skip(e) }, []EventKind{Started, Skipped}, hello},
		{"stop", func(r *rig, e ecs.Entity) { r.s.Stop(e) }, []EventKind{Started, Stopped}, ""},
		{"replace", func(r *rig, e ecs.Entity) { r.s.Say(e, "Bye!", SayOptions{Instant: true}) }, []EventKind{Started, Stopped, Started, Completed}, "Bye!"},
		{"pause", func(r *rig, e ecs.Entity) { r.s.Pause(e) }, []EventKind{Started}, ""},
		{"delete", func(r *rig, e ecs.Entity) { r.w.DeleteEntity(e) }, []EventKind{Started, Stopped}, ""},
		{"lose speaker", func(r *rig, e ecs.Entity) { ecs.RemoveComp[Speaker](r.w, e) }, []EventKind{Started, Stopped}, ""},
		{"close", func(r *rig, e ecs.Entity) { r.s.Close() }, []EventKind{Started, Stopped}, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := newRig(t, asset.Options{})
			e := r.talker(r.voice("chirpy"))
			r.s.Say(e, hello, SayOptions{})
			for range 20 {
				r.tick(time.Second / 60)
			}
			if len(r.sink.Sounding(0)) == 0 {
				t.Fatal("no note sounding mid-line: the test checks nothing")
			}
			mid := label(r.w, e)
			tc.end(r, e)
			r.tick(time.Second / 60)
			if tc.name == "replace" { // the new line's notes end with it
				r.until(e, time.Second/60)
			}
			r.noneSounding()
			if got := r.events(); !slices.Equal(got, tc.want) {
				t.Errorf("events %v, want %v", got, tc.want)
			}
			if r.w.IsAlive(e) && tc.text != "" {
				if got := label(r.w, e); got != tc.text {
					t.Errorf("label %q, want %q", got, tc.text)
				}
			}
			if tc.name == "stop" || tc.name == "pause" {
				if got := label(r.w, e); got != mid || mid == "" {
					t.Errorf("label %q after %s, %q before", got, tc.name, mid)
				}
			}
		})
	}
}

func TestPauseResume(t *testing.T) {
	r := newRig(t, asset.Options{})
	e := r.talker(r.voice("chirpy"))
	r.s.Say(e, hello, SayOptions{})
	r.tick(time.Second / 10)
	r.s.Pause(e)
	n := r.s.Revealed(e)
	for range 30 {
		r.tick(time.Second / 10)
	}
	if r.s.Revealed(e) != n || !r.s.Speaking(e) {
		t.Fatalf("paused line moved: %d to %d", n, r.s.Revealed(e))
	}
	r.noneSounding()
	r.s.Resume(e)
	r.until(e, time.Second/60)
	if label(r.w, e) != hello {
		t.Errorf("label %q", label(r.w, e))
	}
}

// TestFrozen checks that a tick without the speech system (a pause scene,
// a Tool's pause) holds the lines and releases their notes, and that they
// continue afterwards.
func TestFrozen(t *testing.T) {
	r := newRig(t, asset.Options{})
	e := r.talker(r.voice("chirpy"))
	r.s.Say(e, hello, SayOptions{})
	r.tick(time.Second / 10)
	r.s.checkFrozen() // the root did not run this tick
	r.noneSounding()
	n := r.s.Revealed(e)
	r.s.checkFrozen()
	if !r.s.Speaking(e) || r.s.Revealed(e) != n {
		t.Fatal("a frozen line moved or ended")
	}
	r.until(e, time.Second/60)
	if got := r.events(); !slices.Equal(got, []EventKind{Started, Completed}) {
		t.Errorf("events %v", got)
	}
	// Pause, then freeze and thaw: the game's pause wins.
	r.s.Say(e, hello, SayOptions{})
	r.tick(time.Second / 10)
	r.s.Pause(e)
	r.s.checkFrozen()
	n = r.s.Revealed(e)
	r.tick(time.Second / 10)
	if r.s.Revealed(e) != n {
		t.Error("the thaw resumed a line the game paused")
	}
}

func TestOwners(t *testing.T) {
	r := newRig(t, asset.Options{})
	h := r.voice("chirpy")
	a, b := r.talker(h), r.talker(h)
	r.s.Say(a, "Hi.", SayOptions{})
	r.s.Say(b, "Ho.", SayOptions{})
	if la, lb := r.s.byEnt[a], r.s.byEnt[b]; la.owner == lb.owner {
		t.Fatal("two speakers share an owner")
	}
	r.s.Say(a, "Again.", SayOptions{})
	if r.s.owners.inUse() != 2 {
		t.Fatalf("%d owners in use after a second Say, want 2", r.s.owners.inUse())
	}
	old := r.s.byEnt[a].owner
	r.w.DeleteEntity(a)
	r.tick(time.Second / 60)
	if r.s.owners.inUse() != 1 || r.s.Speaking(a) {
		t.Fatalf("%d owners in use after the delete, want 1", r.s.owners.inUse())
	}
	c := r.talker(h)
	r.s.Say(c, "New.", SayOptions{})
	if r.s.byEnt[c].owner != old {
		t.Error("the owner of the deleted entity was not reused")
	}
	// An entity without a Speaker can speak with SayOptions.Voice and keeps
	// its line while it lives.
	d := r.w.NewEntity(gfx.Label{})
	r.s.Say(d, "Hey.", SayOptions{Voice: h})
	r.until(d, time.Second/60)
	if label(r.w, d) != "Hey." {
		t.Errorf("label %q", label(r.w, d))
	}
}

func TestSayPanics(t *testing.T) {
	r := newRig(t, asset.Options{})
	e := r.w.NewEntity(gfx.Label{})
	mustPanic(t, "without a Speaker", func() { r.s.Say(e, "x", SayOptions{}) })
	r.w.DeleteEntity(e)
	mustPanic(t, "dead entity", func() { r.s.Say(e, "x", SayOptions{Voice: r.voice("soft")}) })
}

func mustPanic(t *testing.T, want string, f func()) {
	t.Helper()
	defer func() {
		t.Helper()
		r := recover()
		if s, ok := r.(string); !ok || !strings.Contains(s, "speech: ") || !strings.Contains(s, want) {
			t.Errorf("panic %v, want one containing %q", r, want)
		}
	}()
	f()
}

func TestSilentVoices(t *testing.T) {
	r := newRig(t, asset.Options{})
	e := r.talker(gfx.VoiceHandle{}) // no voice
	f := r.talker(LoadVoice(r.srv.Global(), "talk/none.voice.json"))
	for range 5 {
		r.srv.Update()
	}
	r.s.Say(e, hello, SayOptions{})
	r.s.Say(f, hello, SayOptions{})
	r.until(e, time.Second/60)
	r.until(f, time.Second/60)
	if label(r.w, e) != hello || label(r.w, f) != hello {
		t.Errorf("labels %q, %q", label(r.w, e), label(r.w, f))
	}
	if n := len(r.sink.Calls()); slices.ContainsFunc(r.sink.Calls(), func(c voicetest.Call) bool { return c.Kind == voicetest.NoteOn }) {
		t.Errorf("a silent voice played (%d calls)", n)
	}
}

// TestSayWaitsForVoice checks that a line waits for a voice that is still
// loading instead of playing it silently.
func TestSayWaitsForVoice(t *testing.T) {
	r := newRig(t, asset.Options{Workers: 1})
	e := r.talker(LoadVoice(r.srv.Global(), "talk/bard.voice.json"))
	r.s.Say(e, "Ae tha.", SayOptions{})
	if !r.s.Speaking(e) || r.s.Revealed(e) != 0 {
		t.Fatal("the waiting line does not speak")
	}
	for range 200 {
		r.srv.Update()
		r.tick(time.Second / 60)
		if r.s.Revealed(e) > 0 {
			break
		}
		time.Sleep(time.Millisecond)
	}
	if got := r.events(); len(got) == 0 || got[0] != Started {
		t.Fatalf("events %v", got)
	}
	if !slices.ContainsFunc(r.sink.Calls(), func(c voicetest.Call) bool { return c.Kind == voicetest.NoteOn }) {
		t.Error("the loaded voice did not play")
	}
}

func TestSpeakerPanAndSpeed(t *testing.T) {
	r := newRig(t, asset.Options{})
	h := r.voice("chirpy")
	left := r.w.NewEntity(Speaker{Voice: h, Pan: -2}, gfx.Label{})
	r.s.Say(left, hello, SayOptions{})
	r.until(left, time.Second/60)
	for _, c := range r.sink.Calls() {
		if c.Kind == voicetest.NoteOn && c.Pan != -1 {
			t.Fatalf("note at pan %v, want -1 (clamped)", c.Pan)
		}
	}
	ticks := func(speed, opt float32) int {
		e := r.w.NewEntity(Speaker{Voice: h, Speed: speed}, gfx.Label{})
		r.s.Say(e, hello, SayOptions{Speed: opt})
		n := 0
		for ; r.s.Speaking(e); n++ {
			r.tick(time.Second / 60)
		}
		return n
	}
	normal, fast, both := ticks(0, 0), ticks(2, 0), ticks(2, 2)
	if fast >= normal*3/4 || both >= fast*3/4 {
		t.Errorf("ticks: speed 1 %d, 2 %d, 4 %d", normal, fast, both)
	}
}
