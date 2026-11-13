package xmath

import "testing"

// sink keeps the compiler from dropping the measured calls.
var (
	sinkV Vec2
	sinkF float32
	sinkB bool
	sinkM Mat3
	sinkH Hit
)

func TestAllocs(t *testing.T) {
	a, b := V(3, 4), V(1, 2)
	ra, rb := RectXYWH(0, 0, 10, 10), RectXYWH(5, 5, 10, 10)
	var vel Vec2
	m := TRS(a, 0.3, b)
	rng := NewRand(1)
	tests := []struct {
		name string
		f    func()
	}{
		{"Add", func() { sinkV = a.Add(b) }},
		{"Len", func() { sinkF = a.Len() }},
		{"Norm", func() { sinkV = a.Norm() }},
		{"Rotate", func() { sinkV = a.Rotate(0.5) }},
		{"Damp", func() { sinkV = a.Damp(b, 0.1, 0.016) }},
		{"Spring", func() { sinkV = a.Spring(&vel, b, 0.016, 2, 0.5) }},
		{"Overlaps", func() { sinkB = ra.Overlaps(rb) }},
		{"Penetration", func() { sinkV = ra.Penetration(rb) }},
		{"Mat3.Apply", func() { sinkV = m.Apply(a) }},
		{"Mat3.Mul", func() { sinkM = m.Mul(m) }},
		{"RayRect", func() { sinkH, sinkB = RayRect(a, b, ra) }},
		{"RayCircle", func() { sinkH, sinkB = RayCircle(a, b, Circle{rb.Min, 5}) }},
		{"RaySegment", func() { sinkH, sinkB = RaySegment(a, b, ra.Min, rb.Max) }},
		{"SweepRect", func() { sinkH, sinkB = SweepRect(ra, b, rb) }},
		{"Rand.Float", func() { sinkF = rng.Float() }},
		{"Rand.InCircle", func() { sinkV = rng.InCircle(3) }},
		{"Noise1", func() { sinkF = Noise1(1, 2.5) }},
		{"Noise2", func() { sinkF = Noise2(1, 2.5, 3.5) }},
		{"Wrap", func() { sinkF = Wrap(sinkF, 0, 10) }},
	}
	for _, tt := range tests {
		if n := testing.AllocsPerRun(100, tt.f); n != 0 {
			t.Errorf("%s allocates %v times per call", tt.name, n)
		}
	}
}
