package xmath

import "math"

// Rad converts degrees to radians, e.g. t.Rot = Rad(45) for a value typed in
// the editor.
func Rad(deg float32) float32 { return float32(float64(deg) * math.Pi / 180) }

// Deg converts radians to degrees, e.g. to show a turret angle in a debug
// label.
func Deg(rad float32) float32 { return float32(float64(rad) * 180 / math.Pi) }

// WrapAngle returns a wrapped into (-Pi, Pi], e.g. to keep an accumulating
// rotation small.
func WrapAngle(a float32) float32 {
	r := float32(math.Mod(float64(a), float64(Tau)))
	if r > Pi {
		r -= Tau
	} else if r <= -Pi {
		r += Tau
	}
	return r
}

// AngleDiff returns the shortest signed turn from one angle to another, in
// (-Pi, Pi], e.g. if Abs(AngleDiff(rot, aim)) < 0.1 { fire() }.
func AngleDiff(from, to float32) float32 { return WrapAngle(to - from) }

// LerpAngle interpolates from a to b along the shortest arc, e.g. a ship
// turning smoothly across the ±Pi seam.
func LerpAngle(a, b, t float32) float32 { return a + AngleDiff(a, b)*t }

// ApproachAngle turns a toward target by at most step along the shortest
// arc, e.g. rot = ApproachAngle(rot, aim, turnSpeed*dt).
func ApproachAngle(a, target, step float32) float32 {
	d := AngleDiff(a, target)
	if Abs(d) <= max(step, 0) {
		return a + d
	}
	return a + Sign(d)*step
}

// DampAngle is [Damp] along the shortest arc, e.g. a camera that rotates with
// the player.
func DampAngle(a, target, halfLife, dt float32) float32 {
	return a + AngleDiff(a, target)*dampFactor(halfLife, dt)
}
