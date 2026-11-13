package xmath

import "math"

// Hash32 is a well-mixed integer hash (the lowbias32 function by Chris
// Wellons): cheap, and neighboring inputs give unrelated outputs, e.g. a
// per-tile random variant: Hash32(uint32(x*73856093 ^ y)).
func Hash32(x uint32) uint32 {
	x ^= x >> 16
	x *= 0x7feb352d
	x ^= x >> 15
	x *= 0x846ca68b
	x ^= x >> 16
	return x
}

// HashFloat returns Hash32(x) mapped to [0, 1), e.g. a stable random grass
// tint per cell.
func HashFloat(x uint32) float32 { return float32(Hash32(x)>>8) / (1 << 24) }

// Noise1 is smooth 1D gradient (Perlin) noise in [-1, 1]: continuous in x,
// zero at every integer x, and the same for the same seed and x on every run.
// Each seed gives an unrelated curve, e.g. camera shake with one seed per
// axis, or a flickering torch.
func Noise1(seed uint32, x float32) float32 {
	fl := float32(math.Floor(float64(x)))
	i := int32(fl)
	f := x - fl
	g0 := gradient1(seed, i)
	g1 := gradient1(seed, i+1)
	n := g0*f + fade(f)*(g1*(f-1)-g0*f)
	// A 1D gradient noise with slopes in [-1, 1] stays inside [-0.5, 0.5].
	return min(max(2*n, -1), 1)
}

// Noise2 is smooth 2D gradient (Perlin) noise in [-1, 1]: continuous in x and
// y, zero at every integer point, and the same for the same seed on every run,
// e.g. clouds or the height of procedural terrain. Different seeds give
// unrelated fields.
func Noise2(seed uint32, x, y float32) float32 {
	fx, fy := float32(math.Floor(float64(x))), float32(math.Floor(float64(y)))
	ix, iy := int32(fx), int32(fy)
	dx, dy := x-fx, y-fy
	// Dot of each corner's gradient with the offset from that corner.
	n00 := gradient2(seed, ix, iy).Dot(Vec2{dx, dy})
	n10 := gradient2(seed, ix+1, iy).Dot(Vec2{dx - 1, dy})
	n01 := gradient2(seed, ix, iy+1).Dot(Vec2{dx, dy - 1})
	n11 := gradient2(seed, ix+1, iy+1).Dot(Vec2{dx - 1, dy - 1})
	u, v := fade(dx), fade(dy)
	n := Lerp(Lerp(n00, n10, u), Lerp(n01, n11, u), v)
	// Unit gradients keep n inside [-√2/2, √2/2].
	return min(max(n*math.Sqrt2, -1), 1)
}

// fade is the quintic ease of Perlin noise: the curve and its first two
// derivatives are continuous at the lattice points, so the noise has no kinks.
func fade(f float32) float32 { return f * f * f * (f*(f*6-15) + 10) }

// gradient1 returns the slope at lattice point i, in [-1, 1].
func gradient1(seed uint32, i int32) float32 {
	return float32(Hash32(seed*0x9e3779b9^uint32(i)))/float32(math.MaxUint32)*2 - 1
}

// gradient2 returns the unit gradient at lattice point (x, y).
func gradient2(seed uint32, x, y int32) Vec2 {
	h := Hash32(Hash32(seed*0x9e3779b9^uint32(x)) ^ uint32(y))
	return FromAngle(float32(h>>8) / (1 << 24) * Tau)
}
