package xmath

import "math"

// Clamp returns x limited to [lo, hi], e.g. hp = Clamp(hp+heal, 0, maxHP).
// If lo > hi, lo wins.
func Clamp[T Number](x, lo, hi T) T { return max(lo, min(x, hi)) }

// Clamp01 returns x limited to [0, 1], e.g. alpha := Clamp01(timeLeft / fadeTime).
func Clamp01(x float32) float32 { return max(0, min(x, 1)) }

// ClampAbs returns x limited to [-m, m]; an m of 0 or less means no limit.
// E.g. vel.Y = ClampAbs(vel.Y, maxFall).
func ClampAbs(x, m float32) float32 {
	if m <= 0 {
		return x
	}
	return max(-m, min(x, m))
}

// Lerp returns the value t of the way from a to b, extrapolating outside
// 0..1, e.g. width := Lerp(0, barW, hp/maxHP). It is exactly a at t = 0 and
// exactly b at t = 1.
func Lerp(a, b, t float32) float32 { return (1-t)*a + t*b }

// InvLerp returns where v lies between a and b (0 at a, 1 at b), or 0 when
// a == b, e.g. t := InvLerp(minSpeed, maxSpeed, speed).
func InvLerp(a, b, v float32) float32 {
	if a == b {
		return 0
	}
	return (v - a) / (b - a)
}

// Remap maps v from the range [a0, a1] to [b0, b1], e.g.
// pitch := Remap(speed, 0, 300, 0.8, 1.4). An empty source range maps to b0.
func Remap(v, a0, a1, b0, b1 float32) float32 { return Lerp(b0, b1, InvLerp(a0, a1, v)) }

// RemapClamped is [Remap] with the fraction limited to 0..1, e.g. an engine
// volume that never exceeds its maximum.
func RemapClamped(v, a0, a1, b0, b1 float32) float32 {
	return Lerp(b0, b1, Clamp01(InvLerp(a0, a1, v)))
}

// SmoothStep returns the Hermite ramp from 0 at e0 to 1 at e1, e.g.
// fog := SmoothStep(100, 200, dist). Equal edges give a hard step at e0.
func SmoothStep(e0, e1, x float32) float32 {
	if e0 == e1 {
		return Step(e0, x)
	}
	t := Clamp01((x - e0) / (e1 - e0))
	return t * t * (3 - 2*t)
}

// SmootherStep is [SmoothStep] with Perlin's quintic, flat in its first and
// second derivative at both ends, e.g. a softer vignette.
func SmootherStep(e0, e1, x float32) float32 {
	if e0 == e1 {
		return Step(e0, x)
	}
	t := Clamp01((x - e0) / (e1 - e0))
	return t * t * t * (t*(t*6-15) + 10)
}

// Step returns 0 for x below edge and 1 from edge on, e.g.
// onGround := Step(groundY, feetY).
func Step(edge, x float32) float32 {
	if x < edge {
		return 0
	}
	return 1
}

// Approach moves x toward target by at most step without overshooting, e.g.
// vel.X = Approach(vel.X, 0, friction*dt). A negative step is treated as 0.
func Approach[T Number](x, target, step T) T {
	step = max(step, 0)
	switch {
	case x < target:
		return min(x+step, target)
	case x > target:
		return max(x-step, target)
	}
	return x
}

// Wrap returns x wrapped into [lo, hi), e.g. pos.X = Wrap(pos.X, 0, screenW)
// for Asteroids-style screen edges. If hi <= lo it returns lo.
func Wrap[T Number](x, lo, hi T) T {
	size := hi - lo
	if size <= 0 {
		return lo
	}
	half := T(1)
	half /= 2 // 0 for integer types, 0.5 for floats
	if half == 0 {
		r := int64(x-lo) % int64(size)
		if r < 0 {
			r += int64(size)
		}
		return lo + T(r)
	}
	r := math.Mod(float64(x-lo), float64(size))
	if r < 0 {
		r += float64(size)
	}
	if out := lo + T(r); out < hi {
		return out
	}
	return lo // rounding pushed the result onto hi
}

// PingPong bounces t between 0 and length and back, e.g. a patrol:
// x := left + PingPong(time*speed, width). A length of 0 or less returns 0.
func PingPong(t, length float32) float32 {
	if length <= 0 {
		return 0
	}
	return length - Abs(Repeat(t, 2*length)-length)
}

// Snap returns the nearest multiple of step, e.g. an editor grid:
// Snap(mouse.X, 8). A step of 0 returns x.
func Snap(x, step float32) float32 {
	if step == 0 {
		return x
	}
	return Round(x/step) * step
}

// SnapFloor returns the multiple of step at or below x, e.g. the tile under a
// point: SnapFloor(p.X, 16). A step of 0 returns x.
func SnapFloor(x, step float32) float32 {
	if step == 0 {
		return x
	}
	return Floor(x/step) * step
}

// Fract returns x - Floor(x), in [0, 1), e.g. a looping phase for a blinking
// cursor.
func Fract(x float32) float32 { return x - Floor(x) }

// Repeat loops t into [0, length), e.g. a looping animation time. A length
// of 0 or less returns 0.
func Repeat(t, length float32) float32 {
	if length <= 0 {
		return 0
	}
	r := Mod(t, length)
	if r >= length { // float rounding for tiny negative t
		return 0
	}
	return r
}
