package collision

import (
	"math/rand/v2"
	"slices"
	"testing"

	"github.com/weakpixel/engine/gfx"
)

func randomBoxes(r *rand.Rand, n int) []gfx.Rect {
	boxes := make([]gfx.Rect, n)
	for i := range boxes {
		x, y := r.Float32()*400-200, r.Float32()*400-200
		w, h := r.Float32()*40, r.Float32()*40
		if i%10 == 0 { // some boxes much larger than a cell
			w, h = w*8, h*5
		}
		if i%17 == 0 { // and some of zero size
			w = 0
		}
		boxes[i] = rect(x, y, x+w, y+h)
	}
	return boxes
}

func sortedPairs(ps []Pair) []Pair {
	for i, p := range ps {
		if p.A > p.B {
			ps[i] = Pair{p.B, p.A}
		}
	}
	slices.SortFunc(ps, func(a, b Pair) int {
		if a.A != b.A {
			return int(a.A - b.A)
		}
		return int(a.B - b.B)
	})
	return ps
}

func TestGridMatchesBruteForce(t *testing.T) {
	r := rand.New(rand.NewPCG(1, 2))
	boxes := randomBoxes(r, 300)
	var want []Pair
	for i := range boxes {
		for j := i + 1; j < len(boxes); j++ {
			if boxes[i].Overlaps(boxes[j]) {
				want = append(want, Pair{Proxy(i), Proxy(j)})
			}
		}
	}
	queries := []gfx.Rect{rect(-50, -50, 50, 50), rect(10, 10, 10, 10), rect(-300, -300, 300, 300), rect(0, 0, 64, 64)}
	for _, cell := range []float32{8, 32, 64, 500} {
		g := NewGrid(cell)
		// Fill twice to check that Reset leaves nothing behind.
		for range 2 {
			g.Reset()
			for i, b := range boxes {
				g.Insert(Proxy(i), b, AllLayers, AllLayers)
			}
		}
		got := sortedPairs(g.Pairs(nil))
		if !slices.Equal(got, want) {
			t.Errorf("cell %v: %d pairs, want %d", cell, len(got), len(want))
		}
		for _, q := range queries {
			var wantQ []Proxy
			for i, b := range boxes {
				if intersects(b, q) {
					wantQ = append(wantQ, Proxy(i))
				}
			}
			gotQ := g.Query(q, nil)
			slices.Sort(gotQ)
			if !slices.Equal(gotQ, wantQ) {
				t.Errorf("cell %v query %v: %v, want %v", cell, q, gotQ, wantQ)
			}
		}
	}
}

func TestGridAllocs(t *testing.T) {
	boxes := randomBoxes(rand.New(rand.NewPCG(3, 4)), 500)
	g := NewGrid(32)
	var pairs []Pair
	var hits []Proxy
	run := func() {
		g.Reset()
		for i, b := range boxes {
			g.Insert(Proxy(i), b, AllLayers, AllLayers)
		}
		pairs = g.Pairs(pairs[:0])
		hits = g.Query(rect(-40, -40, 40, 40), hits[:0])
	}
	run()
	if a := testing.AllocsPerRun(20, run); a != 0 {
		t.Errorf("allocs per rebuild = %v, want 0", a)
	}
}

// TestGridModes checks the dense counting-sort mode and the sorted fallback
// against brute force: a compact scene is dense, one far-away box makes the
// occupied rectangle too big for it.
func TestGridModes(t *testing.T) {
	tests := []struct {
		name  string
		far   bool
		dense bool
	}{{"compact", false, true}, {"far-away box", true, false}}
	queries := []gfx.Rect{
		rect(-50, -50, 50, 50), rect(10, 10, 10, 10), rect(-300, -300, 300, 300),
		rect(5000, 5000, 5100, 5100), rect(-9000, 0, -8000, 10), rect(150, -9000, 160, 9000),
		rect(999990, 999990, 1000020, 1000020),
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			boxes := randomBoxes(rand.New(rand.NewPCG(9, 10)), 300)
			if tt.far {
				boxes = append(boxes, rect(1e6, 1e6, 1e6+10, 1e6+10))
			}
			var want []Pair
			for i := range boxes {
				for j := i + 1; j < len(boxes); j++ {
					if boxes[i].Overlaps(boxes[j]) {
						want = append(want, Pair{Proxy(i), Proxy(j)})
					}
				}
			}
			g := NewGrid(32)
			fill := func() {
				g.Reset()
				for i, b := range boxes {
					g.Insert(Proxy(i), b, AllLayers, AllLayers)
				}
			}
			fill()
			got := sortedPairs(g.Pairs(nil))
			if g.dense != tt.dense {
				t.Errorf("dense = %v, want %v", g.dense, tt.dense)
			}
			if !slices.Equal(got, want) {
				t.Errorf("%d pairs, want %d", len(got), len(want))
			}
			for _, q := range queries {
				var wantQ []Proxy
				for i, b := range boxes {
					if intersects(b, q) {
						wantQ = append(wantQ, Proxy(i))
					}
				}
				gotQ := g.Query(q, nil)
				slices.Sort(gotQ)
				if !slices.Equal(gotQ, wantQ) {
					t.Errorf("query %v: %v, want %v", q, gotQ, wantQ)
				}
			}
			var pairs []Pair
			run := func() { fill(); pairs = g.Pairs(pairs[:0]) }
			run()
			if a := testing.AllocsPerRun(20, run); a != 0 {
				t.Errorf("allocs per rebuild = %v, want 0", a)
			}
		})
	}
}

func TestGridPairsMasks(t *testing.T) {
	bullet, enemy := LayersOf(lBullet), LayersOf(lEnemy)
	tests := []struct {
		name           string
		l1, m1, l2, m2 Layers
		want           int
	}{
		{"bullets ignore each other", bullet, enemy, bullet, enemy, 0},
		{"bullet and enemy", bullet, enemy, enemy, bullet, 1},
		{"one side detects", bullet, enemy, enemy, 0, 1},
		{"other side detects", bullet, 0, enemy, bullet, 1},
		{"nobody detects", bullet, 0, enemy, 0, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			g := NewGrid(32)
			g.Insert(0, rect(0, 0, 10, 10), tt.l1, tt.m1)
			g.Insert(1, rect(5, 5, 15, 15), tt.l2, tt.m2)
			if got := len(g.Pairs(nil)); got != tt.want {
				t.Errorf("%d pairs, want %d", got, tt.want)
			}
			if got := len(g.Query(rect(6, 6, 7, 7), nil)); got != 2 {
				t.Errorf("Query found %d proxies, want 2: it ignores layers", got)
			}
		})
	}
}
