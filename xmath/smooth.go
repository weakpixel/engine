package xmath

import "math"

// ExpFactor returns 1 - e^(-dt/tau), the share of the remaining gap that
// exponential smoothing with time constant tau closes in dt seconds. Two
// steps of dt close the same share as one step of 2*dt, so
// pos = Lerp(pos, goal, ExpFactor(dt, tau)) is the frame-rate independent
// replacement for "lerp by 0.1". A dt of 0 or less gives 0, a tau of 0 or
// less gives 1.
func ExpFactor(dt, tau float32) float32 {
	switch {
	case dt <= 0:
		return 0
	case tau <= 0:
		return 1
	}
	return 1 - float32(math.Exp(-float64(dt)/float64(tau)))
}

// dampFactor is the share of the gap closed in dt by a halving time of
// halfLife: 1 - 2^(-dt/halfLife).
func dampFactor(halfLife, dt float32) float32 {
	switch {
	case dt <= 0:
		return 0
	case halfLife <= 0:
		return 1
	}
	return 1 - float32(math.Exp2(-float64(dt)/float64(halfLife)))
}

// Damp moves x toward target, halving the remaining gap every halfLife
// seconds, whatever the frame rate, e.g. zoom = Damp(zoom, targetZoom, 0.15, dt).
// A halfLife of 0 or less snaps to target; a dt of 0 or less leaves x alone.
func Damp(x, target, halfLife, dt float32) float32 {
	return Lerp(x, target, dampFactor(halfLife, dt))
}

// DampFactor returns (1-d)^dt: the factor that keeps (1-d) of a value per
// second over dt seconds, e.g. vel = vel.Mul(DampFactor(airDrag, dt)). A d of
// 0 or less returns 1 (no loss), a d of 1 or more returns 0.
func DampFactor(d, dt float32) float32 {
	switch {
	case d <= 0:
		return 1
	case d >= 1:
		return 0
	}
	return float32(math.Pow(1-float64(d), float64(dt)))
}

// SmoothDamp moves x toward target like a critically damped spring that
// reaches it in about smoothTime seconds (Unity's SmoothDamp), keeping its
// speed in *vel between calls, e.g. a UI bar that eases to the new HP. A
// maxSpeed of 0 or less means no limit. A dt of 0 or less changes nothing.
func SmoothDamp(x, target float32, vel *float32, smoothTime, maxSpeed, dt float32) float32 {
	if dt <= 0 {
		return x
	}
	smoothTime = max(smoothTime, 0.0001)
	omega := 2 / smoothTime
	k := omega * dt
	// Padé approximation of e^(-k), cheap and stable for any k >= 0.
	e := 1 / (1 + k + 0.48*k*k + 0.235*k*k*k)
	change := x - target
	if maxSpeed > 0 {
		limit := maxSpeed * smoothTime
		change = Clamp(change, -limit, limit)
	}
	t := x - change
	tmp := (*vel + omega*change) * dt
	*vel = (*vel - omega*tmp) * e
	out := t + (change+tmp)*e
	if (target-x > 0) == (out > target) { // overshot: land on the target
		out = target
		*vel = (out - target) / dt
	}
	return out
}

// Decay returns x·e^(-rate·dt), fading x toward 0, e.g.
// shakeTrauma = Decay(shakeTrauma, 2, dt).
func Decay(x, rate, dt float32) float32 {
	return x * float32(math.Exp(-float64(rate)*float64(dt)))
}

// criticalEpsilon is how close the damping ratio must be to 1 to use the
// critically damped solution; the other two divide by a value that tends to
// zero there.
const criticalEpsilon = 1e-4

// SpringStep advances a damped spring pulling x (moving at speed v) toward
// target by dt seconds, with natural frequency freq (Hz) and damping ratio
// zeta, and returns the new position and speed. It uses the exact solution of
// x” + 2ζωx' + ω²x = 0 for a target that is constant during dt, so it is
// stable for any dt and two steps of dt equal one step of 2*dt. A zeta below
// 1 overshoots, 1 is critical, above 1 creeps in. A dt of 0 or less changes
// nothing. E.g. a squash-and-stretch scale that wobbles back to 1.
func SpringStep(x, v, target, dt, freq, zeta float32) (float32, float32) {
	if dt <= 0 {
		return x, v
	}
	omega := 2 * math.Pi * float64(freq)
	p, s := springAxis(float64(x-target), float64(v), float64(dt), omega, float64(zeta))
	return target + float32(p), float32(s)
}

// springAxis returns the displacement and velocity after t for a spring
// starting at displacement x0 with velocity v0. It is computed in float64
// so the exponentials keep their precision.
func springAxis(x0, v0, t, omega, zeta float64) (x, v float64) {
	switch {
	case math.Abs(zeta-1) < criticalEpsilon:
		// Critically damped: x = (A + B t) e^(-ωt).
		e := math.Exp(-omega * t)
		b := v0 + omega*x0
		return (x0 + b*t) * e, (b - omega*(x0+b*t)) * e
	case zeta < 1:
		// Under-damped: x = e^(-ζωt) (A cos ωd t + B sin ωd t).
		wd := omega * math.Sqrt(1-zeta*zeta)
		e := math.Exp(-zeta * omega * t)
		s, c := math.Sincos(wd * t)
		b := (v0 + zeta*omega*x0) / wd
		return e * (x0*c + b*s), e * (v0*c - (x0*wd+zeta*omega*b)*s)
	default:
		// Over-damped: x = c1 e^(r1 t) + c2 e^(r2 t).
		d := omega * math.Sqrt(zeta*zeta-1)
		r1, r2 := -zeta*omega+d, -zeta*omega-d
		c2 := (v0 - r1*x0) / (r2 - r1)
		c1 := x0 - c2
		e1, e2 := math.Exp(r1*t), math.Exp(r2*t)
		return c1*e1 + c2*e2, c1*r1*e1 + c2*r2*e2
	}
}
