package editor

import "testing"

// TestSetAnimate: with Animate off the engine's animations are frozen in
// edit mode and run in Play, paused or not; Animate on never freezes them.
func TestSetAnimate(t *testing.T) {
	f := newFixture(t, Config{})
	f.open(`{"objects":[]}`)
	if !f.s.Animate() {
		t.Fatal("Animate is off by default")
	}
	steps := []struct {
		name   string
		do     func()
		frozen bool
	}{
		{"edit, animate on", func() {}, false},
		{"edit, animate off", func() { f.s.SetAnimate(false) }, true},
		{"play", f.s.Play, false},
		{"paused play", func() { f.s.Pause(true) }, false},
		{"stop", f.s.Stop, true},
		{"animate on again", func() { f.s.SetAnimate(true) }, false},
	}
	for _, st := range steps {
		st.do()
		f.update()
		if got := f.e.AnimationsFrozen(); got != st.frozen {
			t.Errorf("%s: frozen = %v, want %v", st.name, got, st.frozen)
		}
	}
}
