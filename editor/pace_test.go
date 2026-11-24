package editor

import (
	"strings"
	"testing"
	"time"
)

// fakeRate is the Ebitengine tick rate as a pacer test sees it.
type fakeRate struct {
	tps  int
	sets int
}

func (f *fakeRate) get() int  { return f.tps }
func (f *fakeRate) set(n int) { f.tps, f.sets = n, f.sets+1 }

// testPacer returns a pacer on a fake rate (the game's 60) and a clock the
// test moves.
func testPacer() (*pacer, *fakeRate, *time.Time) {
	r := &fakeRate{tps: 60}
	now := time.Unix(1000, 0)
	return newPacer(r.get, r.set, func() time.Time { return now }), r, &now
}

func TestPacerRates(t *testing.T) {
	type step struct {
		after  time.Duration // clock moves before the tick
		mode   paceMode
		active bool
		hidden *bool // setHidden before the tick
		want   int
	}
	yes, no := true, false
	tests := []struct {
		name  string
		steps []step
	}{
		{"idle edit runs at the idle rate", []step{{want: 10}, {after: time.Second, want: 10}}},
		{"activity raises the rate until ActiveFor passed", []step{
			{want: 10},
			{after: 100 * time.Millisecond, active: true, want: 60},
			{after: ActiveFor - time.Millisecond, want: 60},
			{after: 2 * time.Millisecond, want: 10},
		}},
		{"play runs at the game's rate and stop returns to idle", []step{
			{want: 10},
			{after: time.Second, mode: pacePlay, want: 60},
			{after: time.Second, mode: pacePlay, active: true, want: 60},
			{after: time.Second, mode: paceEdit, want: 10},
		}},
		{"hidden uses the hidden rate, also in play", []step{
			{hidden: &yes, want: 0},
			{after: time.Second, mode: pacePlay, want: 0},
			{after: time.Second, mode: pacePlay, hidden: &no, want: 60},
		}},
		{"shown again is active", []step{
			{hidden: &yes, want: 0},
			{after: time.Second, hidden: &no, want: 60},
			{after: ActiveFor + time.Millisecond, want: 10},
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			p, r, now := testPacer()
			for i, st := range tc.steps {
				*now = now.Add(st.after)
				if st.hidden != nil {
					p.setHidden(*st.hidden)
				}
				p.tick(st.mode, st.active)
				if r.tps != st.want {
					t.Fatalf("step %d: rate %d, want %d", i, r.tps, st.want)
				}
			}
		})
	}
}

func TestPacerKeepsTheGamesRate(t *testing.T) {
	p, r, now := testPacer()
	r.tps = 30 // Config.TPS of the game
	p.tick(paceEdit, false)
	*now = now.Add(time.Second)
	p.tick(pacePlay, false)
	if r.tps != 30 {
		t.Fatalf("play rate %d, want the game's 30", r.tps)
	}
	p.wake()
	if r.tps != 30 {
		t.Fatalf("wake during play changed the rate to %d", r.tps)
	}
}

func TestPacerWake(t *testing.T) {
	p, r, now := testPacer()
	p.begin()
	p.tick(paceEdit, false)
	if r.tps != 10 {
		t.Fatalf("idle rate %d", r.tps)
	}
	p.wake()
	if r.tps != 60 {
		t.Fatalf("wake: rate %d, want 60 at once", r.tps)
	}
	*now = now.Add(time.Millisecond)
	p.begin()
	p.tick(paceEdit, false)
	if r.tps != 10 {
		t.Fatalf("the tick after wake: rate %d, want idle 10 again (a wake is no activity)", r.tps)
	}
	p.begin()
	p.tick(paceEdit, true)
	sets := r.sets
	p.wake() // already active: nothing to raise
	if r.sets != sets {
		t.Error("wake set the rate while it was active already")
	}

	// Hidden: a wake runs a tick, which goes back to the hidden rate.
	p.setHidden(true)
	p.wake()
	if r.tps != 60 {
		t.Fatalf("wake while hidden: rate %d, want 60", r.tps)
	}
	p.begin()
	p.tick(paceEdit, false)
	if r.tps != 0 {
		t.Fatalf("tick while hidden: rate %d, want 0", r.tps)
	}
}

func TestPacerSetRates(t *testing.T) {
	p, r, _ := testPacer()
	p.tick(paceEdit, false)
	p.setRates(Rates{Idle: 20, Active: 30, Hidden: 2})
	if r.tps != 20 {
		t.Fatalf("new idle rate %d, want 20", r.tps)
	}
	p.setHidden(true)
	if r.tps != 2 {
		t.Fatalf("hidden rate %d, want 2", r.tps)
	}
}

func TestPacerDrawFrame(t *testing.T) {
	same := func() bool { return false }
	differs := func() bool { return true }
	p, _, _ := testPacer()
	if p.drawFrame(differs) {
		t.Error("drew before any tick")
	}
	p.tick(paceEdit, false)
	if p.drawFrame(same) {
		t.Error("edit mode: drew a tick that changed nothing")
	}
	p.tick(paceEdit, false)
	if !p.drawFrame(differs) || p.drawFrame(differs) {
		t.Error("edit mode: want exactly one frame per tick that changed the picture")
	}
	p.tick(paceEdit, true)
	if !p.drawFrame(same) {
		t.Error("edit mode: an active tick was not drawn")
	}
	p.tick(paceEdit, false)
	p.invalid = true // Session.Invalidate
	if !p.drawFrame(same) {
		t.Error("edit mode: an invalidated tick was not drawn")
	}
	p.tick(pacePlay, false)
	if !p.drawFrame(same) || !p.drawFrame(same) {
		t.Error("play: want every frame")
	}
	p.setHidden(true)
	p.tick(pacePlay, false)
	if p.drawFrame(differs) {
		t.Error("hidden: drew a frame")
	}
	if p.draws != 5 {
		t.Errorf("draws %d, want 5", p.draws)
	}
}

func TestRatesCheck(t *testing.T) {
	for _, tc := range []struct {
		r  Rates
		ok bool
	}{
		{DefaultRates, true},
		{Rates{Idle: 60, Active: 60}, true},
		{Rates{Idle: -1, Active: 60}, false},
		{Rates{Idle: 10, Active: 0}, false},
		{Rates{Idle: 10, Active: 60, Hidden: -1}, false},
	} {
		err := tc.r.Check()
		if (err == nil) != tc.ok {
			t.Errorf("Check(%+v) = %v", tc.r, err)
		}
		if err != nil && !strings.Contains(err.Error(), "editor: rates") {
			t.Errorf("error %q lacks the editor: rates prefix", err)
		}
	}
}

// TestPacerWakeDuringTick: a wake that comes after a tick began is not
// undone by the slower rate that tick sets.
func TestPacerWakeDuringTick(t *testing.T) {
	p, r, now := testPacer()
	p.begin()
	p.tick(paceEdit, false)
	*now = now.Add(time.Second)
	p.begin()
	p.wake() // on another goroutine, while the tick runs
	p.tick(paceEdit, false)
	if r.tps != 60 {
		t.Fatalf("rate %d after a wake during the tick, want 60", r.tps)
	}
	p.begin()
	p.tick(paceEdit, false)
	if r.tps != 10 {
		t.Fatalf("the next tick: rate %d, want idle 10", r.tps)
	}
}
