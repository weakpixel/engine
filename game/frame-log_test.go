package game

import (
	"image"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/render"
)

func TestFrameLogRows(t *testing.T) {
	path := filepath.Join(t.TempDir(), "frames.csv")
	l, err := newFrameLog(path)
	if err != nil {
		t.Fatal(err)
	}
	// The second row's camera moved 2.4 world units right at zoom 3, with
	// PixelSnap: the world scrolls 7 whole pixels left (2.4*3 = 7.2,
	// rounded).
	screen := image.Pt(640, 360)
	cam := &render.Camera{Zoom: 3, PixelSnap: true}
	now := time.Now()
	l.tick(2 * time.Millisecond)
	l.tick(3 * time.Millisecond)
	l.draw(now, now.Add(-time.Millisecond), 0.5, cam, screen, time.Millisecond)
	cam.PrevPos, cam.Pos = cam.Pos, gfx.Vec2{X: 2.4}
	l.draw(now.Add(16*time.Millisecond), now, 1, cam, screen, time.Millisecond)
	if err := l.close(); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 3 {
		t.Fatalf("got %d lines, want header + 2 rows:\n%s", len(lines), data)
	}
	cols := len(strings.Split(frameLogHeader, ","))
	for i, line := range lines[1:] {
		if n := len(strings.Split(line, ",")); n != cols {
			t.Errorf("row %d has %d columns, want %d", i, n, cols)
		}
	}
	// Two ticks ran before the first Draw, none before the second.
	if f := strings.Split(lines[1], ","); f[3] != "2" {
		t.Errorf("first row ticks = %s, want 2", f[3])
	}
	if f := strings.Split(lines[2], ","); f[3] != "0" {
		t.Errorf("second row ticks = %s, want 0", f[3])
	}
	// origin_x, origin_y, scroll_x, scroll_y are the last four columns.
	tail := func(line string) string {
		f := strings.Split(line, ",")
		return strings.Join(f[len(f)-4:], ",")
	}
	if got, want := tail(lines[1]), "320.000,180.000,0.000,0.000"; got != want {
		t.Errorf("first row origin/scroll = %s, want %s", got, want)
	}
	if got, want := tail(lines[2]), "313.000,180.000,-7.000,0.000"; got != want {
		t.Errorf("second row origin/scroll = %s, want %s", got, want)
	}
}

func TestFrameLogOffIsNoOp(t *testing.T) {
	var l *frameLog
	l.tick(time.Millisecond)
	l.draw(time.Now(), time.Now(), 1, &render.Camera{}, image.Point{}, 0)
	if err := l.close(); err != nil {
		t.Fatal(err)
	}
}
