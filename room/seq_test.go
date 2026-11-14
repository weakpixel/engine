package room

import (
	"github.com/weakpixel/engine/xmath"
	"testing"
)

func TestSeq(t *testing.T) {
	type step struct {
		dt    float32
		ready bool
		want  event
		phase Phase
		prog  float32
	}
	tests := []struct {
		name    string
		out, in float32
		steps   []step
	}{
		{"timing", 0.5, 0.5, []step{
			{0.25, false, evNone, Out, 0.5},
			{0.25, false, evCovered, Wait, 1},
			{0.25, true, evSwap, In, 1},
			{0.25, false, evNone, In, 0.5},
			{0.25, false, evDone, Idle, 0},
		}},
		{"wait holds until ready", 0.1, 0.1, []step{
			{0.1, true, evCovered, Wait, 1},
			{0.1, false, evNone, Wait, 1},
			{0.1, false, evNone, Wait, 1},
			{0, true, evSwap, In, 1},
			{0.1, false, evDone, Idle, 0},
		}},
		{"zero durations", 0, 0, []step{
			{0, false, evCovered, Wait, 1},
			{0, true, evSwap, In, 0},
			{0, false, evDone, Idle, 0},
		}},
		{"negative durations count as zero", -1, -1, []step{
			{1.0 / 60, false, evCovered, Wait, 1},
			{1.0 / 60, true, evSwap, In, 0},
			{1.0 / 60, false, evDone, Idle, 0},
		}},
		{"overshoot clamps", 0.1, 0.1, []step{
			{1, false, evCovered, Wait, 1},
			{1, true, evSwap, In, 1},
			{1, false, evDone, Idle, 0},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var s seq
			s.start(tt.out, tt.in)
			if s.phase != Out || s.progress() != 0 && tt.out > 0 {
				t.Fatalf("after start: phase %v progress %v", s.phase, s.progress())
			}
			for i, st := range tt.steps {
				got := s.tick(st.dt, st.ready)
				if got != st.want || s.phase != st.phase || xmath.Abs(s.progress()-st.prog) > 1e-5 {
					t.Fatalf("step %d: event %d phase %v progress %v, want %d %v %v",
						i, got, s.phase, s.progress(), st.want, st.phase, st.prog)
				}
			}
		})
	}
}
