package render

import "testing"

func TestAppendStats(t *testing.T) {
	st := Stats{Items: 1200, DrawCalls: 12, Quads: 1190, ShaderDraws: 3, Groups: 1, PostPasses: 2, Placeholders: 2, Culled: 340}
	got := string(appendStats(nil, st, 59.94, 60))
	want := "FPS 59.9  TPS 60.0\n" +
		"items 1200  draws 12  quads 1190\n" +
		"shaded 3  groups 1  post 2\n" +
		"culled 340  placeholders 2"
	if got != want {
		t.Fatalf("appendStats =\n%s\nwant\n%s", got, want)
	}
}

func TestTextSize(t *testing.T) {
	tests := []struct {
		s          string
		cols, rows int
	}{
		{"", 0, 0},
		{"abc", 3, 1},
		{"ab\nabcd\nx", 4, 3},
		{"ab\n", 2, 1},
	}
	for _, tc := range tests {
		if c, r := textSize([]byte(tc.s)); c != tc.cols || r != tc.rows {
			t.Errorf("textSize(%q) = %d, %d, want %d, %d", tc.s, c, r, tc.cols, tc.rows)
		}
	}
}

// TestStatsOverlayDraw checks that the zero overlay draws and reuses its
// buffer and panel image across frames.
func TestStatsOverlayDraw(t *testing.T) {
	dst := newTarget(320, 180)
	var o StatsOverlay
	o.Draw(dst, Stats{Items: 1})
	bg, buf := o.bg, &o.buf[:1][0]
	o.Draw(dst, Stats{Items: 2})
	if o.bg != bg || &o.buf[:1][0] != buf {
		t.Fatal("overlay reallocated its panel image or text buffer")
	}
}
