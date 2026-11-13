package xmath

import (
	"slices"
	"testing"
)

func TestRandDeterministic(t *testing.T) {
	seq := func(r *Rand) []float32 {
		var s []float32
		for range 8 {
			s = append(s, r.Float(), r.Range(-5, 5), float32(r.IntRange(1, 6)), r.Angle(), r.Gauss(0, 1))
		}
		return s
	}
	a, b := seq(NewRand(42)), seq(NewRand(42))
	if !slices.Equal(a, b) {
		t.Error("same seed gave different sequences")
	}
	if slices.Equal(a, seq(NewRand(43))) {
		t.Error("different seeds gave the same sequence")
	}
	r := NewRand(42)
	seq(r)
	r.Seed(42)
	if !slices.Equal(a, seq(r)) {
		t.Error("Seed did not restart the sequence")
	}
}

func TestRandRanges(t *testing.T) {
	r := NewRand(7)
	var sumF, sumG, signs float32
	counts := map[int]int{}
	const n = 20000
	for range n {
		f := r.Float()
		if f < 0 || f >= 1 {
			t.Fatalf("Float = %v", f)
		}
		sumF += f
		if v := r.Range(2, 4); v < 2 || v >= 4 {
			t.Fatalf("Range = %v", v)
		}
		counts[r.IntRange(-2, 3)]++
		if a := r.Angle(); a < 0 || a >= Tau {
			t.Fatalf("Angle = %v", a)
		}
		if s := r.Spread(10, 2); s < 8 || s > 12 {
			t.Fatalf("Spread = %v", s)
		}
		sumG += r.Gauss(5, 2)
		signs += r.Sign()
		if l := r.UnitVec().Len(); !near(l, 1, 1e-5) {
			t.Fatalf("UnitVec length %v", l)
		}
		if l := r.OnCircle(3).Len(); !near(l, 3, 1e-4) {
			t.Fatalf("OnCircle length %v", l)
		}
		if p := r.InCircle(3); p.Len() > 3+1e-5 {
			t.Fatalf("InCircle %v outside", p)
		}
		if p := r.InRect(RectXYWH(10, 20, 4, 6)); !RectXYWH(10, 20, 4, 6).Contains(p) {
			t.Fatalf("InRect %v outside", p)
		}
	}
	if m := sumF / n; !near(m, 0.5, 0.02) {
		t.Errorf("mean of Float = %v", m)
	}
	if m := sumG / n; !near(m, 5, 0.1) {
		t.Errorf("mean of Gauss = %v", m)
	}
	if Abs(signs) > n/20 {
		t.Errorf("Sign is lopsided: sum %v", signs)
	}
	for v := -2; v <= 3; v++ {
		if c := counts[v]; c < n/6-n/30 || c > n/6+n/30 {
			t.Errorf("IntRange value %d came %d of %d times", v, c, n)
		}
	}
	if len(counts) != 6 {
		t.Errorf("IntRange produced %d distinct values, want 6", len(counts))
	}
}

func TestRandInCircleUniform(t *testing.T) {
	r := NewRand(3)
	inner := 0
	const n = 20000
	for range n {
		if r.InCircle(2).Len() < 1 { // half the radius holds a quarter of the area
			inner++
		}
	}
	if f := float32(inner) / n; !near(f, 0.25, 0.02) {
		t.Errorf("share inside half the radius = %v, want 0.25", f)
	}
}

func TestRandChoices(t *testing.T) {
	r := NewRand(9)
	if r.IntRange(5, 5) != 5 || r.IntRange(5, 1) != 5 {
		t.Error("IntRange with hi <= lo")
	}
	for range 100 {
		if r.Chance(0) || !r.Chance(1) {
			t.Fatal("Chance(0) or Chance(1) wrong")
		}
	}
	hits := 0
	for range 10000 {
		if r.Chance(0.25) {
			hits++
		}
	}
	if hits < 2300 || hits > 2700 {
		t.Errorf("Chance(0.25) hit %d of 10000", hits)
	}

	counts := make([]int, 3)
	for range 10000 {
		counts[r.Weighted([]float32{70, 25, 5})]++
	}
	if counts[0] < 6700 || counts[0] > 7300 || counts[2] < 350 || counts[2] > 650 {
		t.Errorf("Weighted counts %v", counts)
	}
	if got := r.Weighted([]float32{0, -1}); got != -1 {
		t.Errorf("Weighted of no positive weight = %d, want -1", got)
	}
	if got := r.Weighted([]float32{0, 3, 0}); got != 1 {
		t.Errorf("Weighted with one positive weight = %d, want 1", got)
	}

	s := []string{"a", "b", "c"}
	seen := map[string]bool{}
	for range 100 {
		seen[Pick(r, s)] = true
	}
	if len(seen) != 3 {
		t.Errorf("Pick saw %v", seen)
	}
	if Pick(r, []int(nil)) != 0 {
		t.Error("Pick of nothing")
	}

	d := []int{1, 2, 3, 4, 5, 6, 7, 8}
	Shuffle(r, d)
	sorted := slices.Clone(d)
	slices.Sort(sorted)
	if !slices.Equal(sorted, []int{1, 2, 3, 4, 5, 6, 7, 8}) {
		t.Errorf("Shuffle lost elements: %v", d)
	}
	if slices.IsSorted(d) {
		t.Errorf("Shuffle left %v in order", d)
	}
}
