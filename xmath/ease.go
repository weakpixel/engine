package xmath

import "math"

// EaseFunc maps a time fraction t in 0..1 to a progress fraction. Back and
// Elastic curves overshoot in between (below 0 or above 1). Every curve here
// is exactly 0 at 0 and exactly 1 at 1; inputs outside 0..1 are not clamped.
// The formulas are Robert Penner's. In curves start slowly, Out curves end
// slowly, InOut curves do both.
type EaseFunc func(t float32) float32

// Reverse returns the curve played backwards, 1 - f(1-t), e.g. an Out curve
// from a custom In curve.
func Reverse(f EaseFunc) EaseFunc {
	return func(t float32) float32 { return 1 - f(1-t) }
}

// Mirror returns an InOut curve from an In curve: f on the first half and its
// reverse on the second, e.g. a custom symmetric fade.
func Mirror(f EaseFunc) EaseFunc {
	return func(t float32) float32 { return inOut(f, t) }
}

// EaseLinear returns t.
func EaseLinear(t float32) float32 { return t }

// EaseInQuad accelerates, e.g. an item that starts slowly as it is pulled to the player.
func EaseInQuad(t float32) float32 { return t * t }

// EaseOutQuad decelerates, e.g. a menu sliding in.
func EaseOutQuad(t float32) float32 { return 1 - (1-t)*(1-t) }

// EaseInOutQuad does both, e.g. a camera pan between two rooms.
func EaseInOutQuad(t float32) float32 { return inOut(EaseInQuad, t) }

// EaseInCubic is a steeper [EaseInQuad].
func EaseInCubic(t float32) float32 { return t * t * t }

// EaseOutCubic is a steeper [EaseOutQuad].
func EaseOutCubic(t float32) float32 { u := 1 - t; return 1 - u*u*u }

// EaseInOutCubic is a steeper [EaseInOutQuad].
func EaseInOutCubic(t float32) float32 { return inOut(EaseInCubic, t) }

// EaseInSine is a gentle acceleration along a quarter cosine.
func EaseInSine(t float32) float32 { return 1 - Cos(t*HalfPi) }

// EaseOutSine is a gentle deceleration along a quarter sine.
func EaseOutSine(t float32) float32 { return 1 - EaseInSine(1-t) }

// EaseInOutSine is both, e.g. a room transition fade.
func EaseInOutSine(t float32) float32 { return inOut(EaseInSine, t) }

// EaseInExpo accelerates exponentially.
func EaseInExpo(t float32) float32 {
	if t <= 0 {
		return 0
	}
	return float32(math.Pow(2, 10*float64(t)-10))
}

// EaseOutExpo decelerates exponentially.
func EaseOutExpo(t float32) float32 { return 1 - EaseInExpo(1-t) }

// EaseInOutExpo does both.
func EaseInOutExpo(t float32) float32 { return inOut(EaseInExpo, t) }

// backOvershoot is Penner's default: about 10% overshoot.
const backOvershoot = 1.70158

// EaseInBack pulls back below 0 before accelerating, e.g. a wind-up.
func EaseInBack(t float32) float32 { return t * t * ((backOvershoot+1)*t - backOvershoot) }

// EaseOutBack overshoots 1 then settles, e.g. a popup that "pops".
func EaseOutBack(t float32) float32 { return 1 - EaseInBack(1-t) }

// EaseInOutBack does both.
func EaseInOutBack(t float32) float32 { return inOut(EaseInBack, t) }

// EaseInElastic is a springy wind-up.
func EaseInElastic(t float32) float32 {
	if t <= 0 || t >= 1 {
		return t
	}
	const c = 2 * math.Pi / 3
	return float32(-math.Pow(2, 10*float64(t)-10) * math.Sin((float64(t)*10-10.75)*c))
}

// EaseOutElastic is a springy overshoot, e.g. a damage number that wobbles in.
func EaseOutElastic(t float32) float32 { return 1 - EaseInElastic(1-t) }

// EaseInBounce bounces at the start.
func EaseInBounce(t float32) float32 { return 1 - EaseOutBounce(1-t) }

// EaseOutBounce bounces at the end, e.g. a dropped coin landing.
func EaseOutBounce(t float32) float32 {
	const n, d = 7.5625, 2.75
	switch {
	case t < 1/d:
		return n * t * t
	case t < 2/d:
		t -= 1.5 / d
		return n*t*t + 0.75
	case t < 2.5/d:
		t -= 2.25 / d
		return n*t*t + 0.9375
	default:
		t -= 2.625 / d
		return n*t*t + 0.984375
	}
}

// inOut runs the In curve on the first half and its mirror on the second.
func inOut(in func(float32) float32, t float32) float32 {
	if t < 0.5 {
		return in(2*t) / 2
	}
	return 1 - in(2-2*t)/2
}
