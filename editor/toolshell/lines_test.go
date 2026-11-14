package toolshell

import (
	"math"
	"slices"
	"testing"
	"time"
)

func TestLineGeoMEndpoints(t *testing.T) {
	for _, c := range []struct{ x0, y0, x1, y1 float32 }{
		{10, 10, 40, 10}, {10, 10, 10, 50}, {5, 5, 35, 45}, {40, 10, 10, 10},
	} {
		m := lineGeoM(c.x0, c.y0, c.x1, c.y1, 3)
		// The unit square's left and right edge centers are the line's ends.
		sx, sy := m.Apply(0, 0.5)
		ex, ey := m.Apply(1, 0.5)
		if math.Abs(sx-float64(c.x0)) > 1e-4 || math.Abs(sy-float64(c.y0)) > 1e-4 ||
			math.Abs(ex-float64(c.x1)) > 1e-4 || math.Abs(ey-float64(c.y1)) > 1e-4 {
			t.Errorf("line %v: start (%g, %g), end (%g, %g)", c, sx, sy, ex, ey)
		}
		// The square's width is the line width: its top and bottom edge
		// centers are 3 pixels apart.
		tx, ty := m.Apply(0.5, 0)
		bx, by := m.Apply(0.5, 1)
		if d := math.Hypot(bx-tx, by-ty); math.Abs(d-3) > 1e-4 {
			t.Errorf("line %v: width %g, want 3", c, d)
		}
	}
}

func TestOpenFile(t *testing.T) {
	tr := &fakeTransport{}
	f := newFixture(t, Config{Transport: tr})
	f.tick(time.Millisecond)
	tr.types()
	if err := f.s.OpenFile("levels/a/level-1.level.json"); err != nil {
		t.Fatal(err)
	}
	if len(tr.msgs) != 1 || tr.msgs[0]["type"] != "open" || tr.msgs[0]["path"] != "levels/a/level-1.level.json" {
		t.Errorf("messages = %v, want one open with the key", tr.msgs)
	}
	for _, bad := range []string{"", "../x.json", "/abs.json"} {
		if f.s.OpenFile(bad) == nil {
			t.Errorf("OpenFile(%q) accepted a bad key", bad)
		}
	}
	if got := tr.types(); !slices.Equal(got, []string{"open"}) {
		t.Errorf("bad keys sent %v", got)
	}
}

func TestOpenFileNativeSetsStatus(t *testing.T) {
	f := newFixture(t, Config{})
	f.tick(time.Millisecond)
	if err := f.s.OpenFile("a.level.json"); err != nil {
		t.Fatal(err)
	}
	if got := f.s.fullPanel().Status; got != "Open a.level.json in your editor" {
		t.Errorf("status = %q", got)
	}
}
