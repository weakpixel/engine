package framelog

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/weakpixel/engine/tools/internal/cli"
)

// testdata/frames.csv is 40 frames of a steady scroll of 8 px per frame at
// 16.7 ms, with a late/early pair (frames 15 and 16, one tick each), a
// stall with three ticks (25) followed by a frame with none (26), and a GC
// during frame 33.
func TestReportFindsHitches(t *testing.T) {
	var out bytes.Buffer
	if err := Run(Options{File: "testdata/frames.csv"}, &out); err != nil {
		t.Fatal(err)
	}
	got := out.String()
	for _, want := range []string{
		"frames: 40 over 0.7 s, median draw gap 16.70 ms",
		"ticks: 0:1 1:38 3:1",
		"hitch frame 15 at 276 ms: gap 26.0 (prev 16.7, next 7.3) ticks 1 alpha 0.000 scroll 8.00: late",
		"hitch frame 16 at 284 ms: gap 7.3 (prev 26.0, next 16.7) ticks 1 alpha 0.000 scroll 8.00: early",
		"hitch frame 25 at 465 ms: gap 47.3 (prev 16.7, next 0.8) ticks 3 alpha 0.000 scroll 24.00: 3 ticks, late, scroll 24.00, around 8.00",
		"hitch frame 26 at 466 ms: gap 0.8 (prev 47.3, next 16.7) ticks 0 alpha 0.000 scroll 0.00: 0 ticks, early, scroll 0.00, around 8.00",
		"hitch frame 33 at 582 ms: gap 16.7 (prev 16.7, next 16.7) ticks 1 alpha 0.000 scroll 8.00: gc",
		"hitches: 5 (",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("output lacks %q:\n%s", want, got)
		}
	}
	if n := strings.Count(got, "hitch frame"); n != 5 {
		t.Errorf("%d hitch lines, want 5:\n%s", n, got)
	}
}

func TestSkipDropsRows(t *testing.T) {
	var out bytes.Buffer
	if err := Run(Options{File: "testdata/frames.csv", Skip: 30}, &out); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); !strings.Contains(got, "frames: 10 ") || !strings.Contains(got, "hitches: 1 (") {
		t.Errorf("after --skip 30, want 10 frames and only the GC hitch:\n%s", got)
	}
}

// TestOldLog reads a log written before the scroll columns existed.
func TestOldLog(t *testing.T) {
	path := filepath.Join(t.TempDir(), "old.csv")
	data := "frame,t_ms,draw_gap_ms,ticks,since_update_ms,alpha,update_ms,draw_ms,gc\n" +
		"0,0,0,1,0,0,0,0,1\n1,16.7,16.7,1,0,0,0,0,1\n2,33.4,16.7,2,0,0,0,0,1\n"
	if err := os.WriteFile(path, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := Run(Options{File: path, Skip: 1}, &out); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); !strings.Contains(got, "scroll check skipped") || !strings.Contains(got, "hitch frame 2 at 33 ms: gap 16.7 (prev 16.7, next -) ticks 2 alpha 0.000: 2 ticks\n") {
		t.Errorf("old log:\n%s", got)
	}
}

func TestErrors(t *testing.T) {
	dir := t.TempDir()
	bad := filepath.Join(dir, "bad.csv")
	if err := os.WriteFile(bad, []byte("frame,t_ms,draw_gap_ms,ticks,alpha,gc\n0,x,0,1,0,y\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	short := filepath.Join(dir, "short.csv")
	if err := os.WriteFile(short, []byte("frame,t_ms,draw_gap_ms,ticks,alpha,gc\n0,0\n1,0,0,1,0,0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	noCols := filepath.Join(dir, "nocols.csv")
	if err := os.WriteFile(noCols, []byte("frame,t_ms\n0,0\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name  string
		opts  Options
		usage bool
		want  []string
	}{
		{"negative skip", Options{File: "testdata/frames.csv", Skip: -1}, true, []string{"--skip"}},
		{"missing file", Options{File: filepath.Join(dir, "none.csv")}, false, []string{"none.csv"}},
		{"bad cells", Options{File: bad}, false, []string{"line 2: t_ms", "line 2: gc"}},
		{"short middle line", Options{File: short}, false, []string{"line 2: 2 fields, want 6"}},
		{"missing columns", Options{File: noCols}, false, []string{`"draw_gap_ms"`, `"gc"`}},
		{"skip everything", Options{File: "testdata/frames.csv", Skip: 60}, false, []string{"nothing left after --skip 60"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			err := Run(tc.opts, &bytes.Buffer{})
			if err == nil {
				t.Fatal("no error")
			}
			var u *cli.UsageError
			if errors.As(err, &u) != tc.usage {
				t.Errorf("usage error = %v, want %v (%v)", !tc.usage, tc.usage, err)
			}
			for _, w := range tc.want {
				if !strings.Contains(err.Error(), w) {
					t.Errorf("error lacks %q: %v", w, err)
				}
			}
		})
	}
}

// TestTruncatedLastLine reads a log whose game was killed mid-row: the cut-off
// last line is dropped with a note.
func TestTruncatedLastLine(t *testing.T) {
	data, err := os.ReadFile("testdata/frames.csv")
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "cut.csv")
	if err := os.WriteFile(path, append(data, "40,700.0,16.7"...), 0o644); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := Run(Options{File: path, Skip: 0}, &out); err != nil {
		t.Fatal(err)
	}
	if got := out.String(); !strings.Contains(got, "last line is incomplete") || !strings.Contains(got, "frames: 40 ") {
		t.Errorf("truncated log:\n%s", got)
	}
}
