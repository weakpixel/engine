package tilemap

import (
	"math"
	"testing"

	"github.com/weakpixel/engine/tiled"
)

// TestOrient maps the corners of a square centered on the origin through
// Tiled's definition (transpose, then H, then V) and through the item's
// mirror-then-rotate transform (the order of render's geoM); both must agree
// for all eight flag combinations, with and without the hexagonal bit.
func TestOrient(t *testing.T) {
	corners := [4][2]float64{{-1, -1}, {1, -1}, {1, 1}, {-1, 1}}
	tiledMap := func(f tiled.Flip, p [2]float64) [2]float64 {
		x, y := p[0], p[1]
		if f&tiled.FlipD != 0 {
			x, y = y, x
		}
		if f&tiled.FlipH != 0 {
			x = -x
		}
		if f&tiled.FlipV != 0 {
			y = -y
		}
		return [2]float64{x, y}
	}
	itemMap := func(rot float32, fx, fy bool, p [2]float64) [2]float64 {
		x, y := p[0], p[1]
		if fx {
			x = -x
		}
		if fy {
			y = -y
		}
		s, c := math.Sincos(float64(rot))
		return [2]float64{x*c - y*s, x*s + y*c}
	}
	for _, hex := range []tiled.Flip{0, tiled.FlipHex} {
		for f := range tiled.Flip(8) {
			rot, fx, fy := orient(f | hex)
			if fx && fy {
				t.Errorf("flags %03b: both mirrors set", f)
			}
			for _, p := range corners {
				want, got := tiledMap(f, p), itemMap(rot, fx, fy, p)
				if math.Abs(want[0]-got[0]) > 1e-6 || math.Abs(want[1]-got[1]) > 1e-6 {
					t.Errorf("flags %03b: corner %v -> %v, want %v (rot %v, flip %v %v)", f, p, got, want, rot, fx, fy)
				}
			}
		}
	}
}
