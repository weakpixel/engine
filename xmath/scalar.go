package xmath

import "math"

// Mathematical constants as float32.
const (
	// Pi is half a turn. Tau is Pi*2, one full turn: spin += Tau * dt turns
	// once per second. HalfPi is a quarter turn.
	Pi     float32 = math.Pi
	Tau    float32 = 2 * math.Pi
	HalfPi float32 = math.Pi / 2
)

// Epsilon is the default tolerance of the *Ok functions, e.g. [Vec2.NormOk].
const Epsilon float32 = 1e-6

// Number is the set of signed numeric types the generic helpers accept.
type Number interface {
	~int | ~int8 | ~int16 | ~int32 | ~int64 | ~float32 | ~float64
}

// Sqrt returns the float32 square root of x, e.g. speed := Sqrt(vx*vx + vy*vy).
func Sqrt(x float32) float32 { return float32(math.Sqrt(float64(x))) }

// Sin returns the float32 sine of x radians, e.g. bob := Sin(time*4) * 2 for a hovering pickup.
func Sin(x float32) float32 { return float32(math.Sin(float64(x))) }

// Cos returns the float32 cosine of x radians, e.g. x := Cos(a) * orbitRadius for a circling drone.
func Cos(x float32) float32 { return float32(math.Cos(float64(x))) }

// Sincos returns the sine and cosine of x radians, e.g. to build the rotation of a spinning saw blade.
func Sincos(x float32) (s, c float32) {
	s64, c64 := math.Sincos(float64(x))
	return float32(s64), float32(c64)
}

// Atan2 returns the angle of the point (x, y) in (-Pi, Pi], e.g. aim := Atan2(dy, dx) to turn a turret to the mouse.
func Atan2(y, x float32) float32 { return float32(math.Atan2(float64(y), float64(x))) }

// Pow returns x to the power y, e.g. falloff := Pow(1-d/r, 2) for explosion damage.
func Pow(x, y float32) float32 { return float32(math.Pow(float64(x), float64(y))) }

// Exp returns e to the power x, e.g. for decay curves in custom smoothing.
func Exp(x float32) float32 { return float32(math.Exp(float64(x))) }

// Hypot returns Sqrt(x*x + y*y) without intermediate overflow, e.g. the
// distance from two deltas.
func Hypot(x, y float32) float32 { return float32(math.Hypot(float64(x), float64(y))) }

// Floor rounds x down, e.g. tileX := Floor(px / 16).
func Floor(x float32) float32 { return float32(math.Floor(float64(x))) }

// Ceil rounds x up, e.g. pages := Ceil(items / perPage).
func Ceil(x float32) float32 { return float32(math.Ceil(float64(x))) }

// Round rounds x to the nearest integer, halves away from zero, e.g. to
// pixel-snap a sprite: Round(pos.X).
func Round(x float32) float32 { return float32(math.Round(float64(x))) }

// Trunc drops the fraction of x, e.g. whole seconds shown on a timer.
func Trunc(x float32) float32 { return float32(math.Trunc(float64(x))) }

// Mod returns the remainder of x/y that is never negative for y > 0 (Euclidean),
// e.g. scroll := Mod(offset, bgWidth). A y of 0 returns 0.
func Mod(x, y float32) float32 {
	if y == 0 {
		return 0
	}
	r := float32(math.Mod(float64(x), float64(y)))
	if r < 0 && y > 0 || r > 0 && y < 0 {
		r += y
	}
	return r
}

// FloorDiv divides a by b rounding down, so FloorDiv(-1, 16) is -1, e.g.
// chunk := FloorDiv(cellX, 32). A b of 0 panics like the / operator.
func FloorDiv(a, b int) int {
	q := a / b
	if a%b != 0 && (a < 0) != (b < 0) {
		q--
	}
	return q
}

// ModInt returns the remainder of a/b with the sign of b (Euclidean for b > 0),
// e.g. frame := ModInt(i, len(frames)) also for negative i. A b of 0 panics
// like the % operator.
func ModInt(a, b int) int {
	r := a % b
	if r != 0 && (r < 0) != (b < 0) {
		r += b
	}
	return r
}

// Abs returns |x|, e.g. if Abs(vel.X) < 1 { anim = idle }.
func Abs[T Number](x T) T {
	if x < 0 {
		return -x
	}
	return x
}

// Sign returns -1, 0 or 1 by the sign of x, e.g. facing := Sign(input.X).
func Sign[T Number](x T) T {
	switch {
	case x > 0:
		return 1
	case x < 0:
		return -1
	}
	return 0
}

// SignEps is Sign with a dead zone: it returns 0 when |x| <= eps, e.g. to
// ignore stick drift: SignEps(axis, 0.2).
func SignEps(x, eps float32) float32 {
	switch {
	case x > eps:
		return 1
	case x < -eps:
		return -1
	}
	return 0
}

// Sq returns x*x, e.g. to compare distances without Sqrt: d2 < Sq(radius).
func Sq(x float32) float32 { return x * x }

// NearlyEqual reports whether |a-b| <= eps, e.g. to snap a tween's end value
// in tests or logic.
func NearlyEqual(a, b, eps float32) bool { return Abs(a-b) <= eps }

// IsFinite reports whether x is neither NaN nor an infinity, e.g. to guard a
// physics step against a bad division.
func IsFinite(x float32) bool {
	return !math.IsNaN(float64(x)) && !math.IsInf(float64(x), 0)
}
