package xmath

import "testing"

func TestAngles(t *testing.T) {
	checkF(t, "Rad(180)", Rad(180), Pi)
	checkF(t, "Deg(Pi)", Deg(Pi), 180)
	checkF(t, "WrapAngle(3Pi/2)", WrapAngle(3*HalfPi), -HalfPi)
	checkF(t, "WrapAngle(-5Pi/2)", WrapAngle(-5*HalfPi), -HalfPi)
	if got := WrapAngle(Pi); got != Pi {
		t.Errorf("WrapAngle(Pi) = %v, want Pi", got)
	}
	if got := WrapAngle(-Pi); got != Pi {
		t.Errorf("WrapAngle(-Pi) = %v, want Pi (range is (-Pi, Pi])", got)
	}
	// Across the ±Pi seam the short way is 0.2, not -2π+0.2.
	checkF(t, "AngleDiff across seam", AngleDiff(Pi-0.1, -Pi+0.1), 0.2)
	checkF(t, "AngleDiff back", AngleDiff(-Pi+0.1, Pi-0.1), -0.2)
	checkF(t, "LerpAngle across seam", WrapAngle(LerpAngle(Pi-0.1, -Pi+0.1, 0.5)), Pi)
	checkF(t, "ApproachAngle step", ApproachAngle(0, 1, 0.25), 0.25)
	checkF(t, "ApproachAngle arrives", ApproachAngle(0, 0.1, 0.25), 0.1)
	checkF(t, "ApproachAngle short way", WrapAngle(ApproachAngle(Pi-0.1, -Pi+0.1, 0.05)), Pi-0.05)
	checkF(t, "DampAngle half-life", DampAngle(0, 1, 1, 1), 0.5)
	checkF(t, "DampAngle across seam", WrapAngle(DampAngle(Pi-0.1, -Pi+0.1, 1, 1)), Pi)
}
