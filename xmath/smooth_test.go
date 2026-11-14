package xmath

import "testing"

func TestDamp(t *testing.T) {
	checkF(t, "half-life", Damp(0, 10, 1, 1), 5)
	checkF(t, "dt 0", Damp(3, 10, 1, 0), 3)
	checkF(t, "half-life 0 snaps", Damp(3, 10, 0, 0.016), 10)
	// Two steps of dt equal one step of 2*dt.
	one := Damp(0, 10, 0.3, 0.2)
	two := Damp(Damp(0, 10, 0.3, 0.1), 10, 0.3, 0.1)
	checkF(t, "composition", two, one)
	checkF(t, "ExpFactor composition", 1-(1-ExpFactor(0.1, 0.5))*(1-ExpFactor(0.1, 0.5)), ExpFactor(0.2, 0.5))
	checkF(t, "ExpFactor dt 0", ExpFactor(0, 1), 0)
	checkF(t, "ExpFactor tau 0", ExpFactor(1, 0), 1)
}

func TestDampFactor(t *testing.T) {
	checkF(t, "keeps (1-d) per second", DampFactor(0.75, 1), 0.25)
	checkF(t, "half a second", DampFactor(0.75, 0.5), 0.5)
	checkF(t, "no loss", DampFactor(0, 1), 1)
	checkF(t, "all lost", DampFactor(1, 1), 0)
}

func TestDecay(t *testing.T) {
	checkF(t, "dt 0", Decay(8, 2, 0), 8)
	checkF(t, "composition", Decay(Decay(8, 2, 0.1), 2, 0.1), Decay(8, 2, 0.2))
}

func TestSmoothDamp(t *testing.T) {
	var vel float32
	x := float32(0)
	for range 600 {
		x = SmoothDamp(x, 10, &vel, 0.3, 0, 1.0/60)
		if x > 10 {
			t.Fatalf("overshot the target: %v", x)
		}
	}
	checkF(t, "arrives", x, 10)
	vel = 0
	x = SmoothDamp(0, 10, &vel, 0.3, 5, 1.0/60)
	if x > 5.0/60+1e-4 {
		t.Errorf("first step %v exceeds maxSpeed*dt", x)
	}
	if got := SmoothDamp(1, 10, &vel, 0.3, 0, 0); got != 1 {
		t.Errorf("dt 0 moved to %v", got)
	}
}

func simulateSpring(goal float32, seconds float32, tps int, freq, zeta float32) (pos, peak float32) {
	var vel float32
	dt := 1 / float32(tps)
	for range int(seconds * float32(tps)) {
		pos, vel = SpringStep(pos, vel, goal, dt, freq, zeta)
		peak = max(peak, pos)
	}
	return pos, peak
}

func TestSpringStep(t *testing.T) {
	tests := []struct {
		name      string
		zeta      float32
		overshoot bool
	}{
		{"critical", 1, false},
		{"under-damped", 0.3, true},
		{"over-damped", 2.5, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pos, peak := simulateSpring(100, 10, 60, 2, tt.zeta)
			if !near(pos, 100, 0.01) {
				t.Errorf("after 10 s at %v, want the goal", pos)
			}
			if over := peak > 100.01; over != tt.overshoot {
				t.Errorf("peak %v: overshoot = %v, want %v", peak, over, tt.overshoot)
			}
		})
	}
}

func TestSpringStepZeroDt(t *testing.T) {
	if x, v := SpringStep(1, 3, 9, 0, 2, 1); x != 1 || v != 3 {
		t.Errorf("dt 0 changed the spring: %v %v", x, v)
	}
}

// TestSpringStepLargeDtStable takes huge steps: an exact solution never blows up.
func TestSpringStepLargeDtStable(t *testing.T) {
	for _, zeta := range []float32{0.2, 1, 3} {
		var x, v float32
		for range 20 {
			x, v = SpringStep(x, v, 100, 5, 2, zeta)
		}
		if !near(x, 100, 0.01) {
			t.Errorf("zeta %v: %v after big steps, want the goal", zeta, x)
		}
	}
}

// TestSpringStepTickRateIndependent compares the step response at 30, 60 and
// 120 ticks per second part way through (where it is still moving).
func TestSpringStepTickRateIndependent(t *testing.T) {
	for _, zeta := range []float32{0.4, 1, 2} {
		ref, _ := simulateSpring(100, 0.4, 30, 2, zeta)
		for _, tps := range []int{60, 120} {
			got, _ := simulateSpring(100, 0.4, tps, 2, zeta)
			if !near(got, ref, 0.01) {
				t.Errorf("zeta %v: %d TPS gives %v, 30 TPS %v", zeta, tps, got, ref)
			}
		}
	}
}
