package xmath

import (
	"math"
	"testing"
)

func TestNoise1(t *testing.T) {
	t.Run("range and zero at lattice points", func(t *testing.T) {
		lo, hi := float32(0), float32(0)
		for i := range 20000 {
			x := float32(i)*0.013 - 100
			n := Noise1(7, x)
			if n < -1 || n > 1 {
				t.Fatalf("Noise1(7, %v) = %v, outside [-1, 1]", x, n)
			}
			lo, hi = min(lo, n), max(hi, n)
		}
		if lo > -0.3 || hi < 0.3 {
			t.Errorf("noise spans [%v, %v], want a wider range", lo, hi)
		}
		for _, x := range []float32{-3, 0, 1, 42} {
			if n := Noise1(7, x); n != 0 {
				t.Errorf("Noise1(7, %v) = %v, want 0", x, n)
			}
		}
	})
	t.Run("continuous", func(t *testing.T) {
		const step = 0.001
		for i := range 5000 {
			x := float32(i) * 0.01
			if d := math.Abs(float64(Noise1(3, x+step) - Noise1(3, x))); d > 0.01 {
				t.Fatalf("noise jumps by %v between %v and %v", d, x, x+step)
			}
		}
	})
	t.Run("deterministic and seeded", func(t *testing.T) {
		same := 0
		for i := range 100 {
			x := float32(i)*0.37 + 0.5
			if Noise1(1, x) != Noise1(1, x) {
				t.Fatal("same seed and x gave two values")
			}
			if Noise1(1, x) == Noise1(2, x) {
				same++
			}
		}
		if same > 5 {
			t.Errorf("seeds 1 and 2 agree at %d of 100 points", same)
		}
	})
}

func TestNoise2(t *testing.T) {
	t.Run("range and zero at lattice points", func(t *testing.T) {
		lo, hi := float32(0), float32(0)
		for i := range 200 {
			for j := range 200 {
				x, y := float32(i)*0.071-7, float32(j)*0.053-5
				n := Noise2(5, x, y)
				if n < -1 || n > 1 {
					t.Fatalf("Noise2(5, %v, %v) = %v, outside [-1, 1]", x, y, n)
				}
				lo, hi = min(lo, n), max(hi, n)
			}
		}
		if lo > -0.4 || hi < 0.4 {
			t.Errorf("noise spans [%v, %v], want a wider range", lo, hi)
		}
		for _, p := range []Vec2{{0, 0}, {3, -2}, {-7, 9}} {
			if n := Noise2(5, p.X, p.Y); n != 0 {
				t.Errorf("Noise2 at lattice point %v = %v, want 0", p, n)
			}
		}
	})
	t.Run("continuous", func(t *testing.T) {
		const step = 0.001
		for i := range 100 {
			for j := range 100 {
				x, y := float32(i)*0.093, float32(j)*0.087
				n := Noise2(2, x, y)
				if d := Abs(Noise2(2, x+step, y) - n); d > 0.01 {
					t.Fatalf("noise jumps by %v in x at (%v, %v)", d, x, y)
				}
				if d := Abs(Noise2(2, x, y+step) - n); d > 0.01 {
					t.Fatalf("noise jumps by %v in y at (%v, %v)", d, x, y)
				}
			}
		}
	})
	t.Run("deterministic and seeded", func(t *testing.T) {
		same := 0
		for i := range 100 {
			x, y := float32(i)*0.37+0.5, float32(i)*0.23+0.25
			if Noise2(1, x, y) != Noise2(1, x, y) {
				t.Fatal("same seed and point gave two values")
			}
			if Noise2(1, x, y) == Noise2(2, x, y) {
				same++
			}
		}
		if same > 5 {
			t.Errorf("seeds 1 and 2 agree at %d of 100 points", same)
		}
	})
}

func TestHash(t *testing.T) {
	if Hash32(0) != 0 || Hash32(1) == Hash32(2) || Hash32(1) != Hash32(1) {
		t.Error("Hash32 is not a stable mixer")
	}
	var sum float32
	for i := range uint32(10000) {
		h := HashFloat(i)
		if h < 0 || h >= 1 {
			t.Fatalf("HashFloat(%d) = %v", i, h)
		}
		sum += h
	}
	if m := sum / 10000; !near(m, 0.5, 0.02) {
		t.Errorf("mean of HashFloat = %v", m)
	}
}
