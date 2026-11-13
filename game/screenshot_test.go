package game

import (
	"bytes"
	"errors"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
)

func TestShotScale(t *testing.T) {
	for _, tt := range []struct {
		req     ShotRequest
		want    int
		wantErr string
	}{
		{ShotRequest{}, 1, ""},
		{ShotRequest{Scale: 3}, 3, ""},
		{ShotRequest{Scale: MaxShotScale}, MaxShotScale, ""},
		{ShotRequest{Scale: MaxShotScale + 1}, 0, "scale 9 out of range 1..8"},
		{ShotRequest{Scale: -1}, 0, "scale -1 out of range"},
		{ShotRequest{Target: ShotWindow, Scale: 99}, 1, ""},
		{ShotRequest{Target: 7}, 0, "unknown target ShotTarget(7)"},
	} {
		got, err := shotScale(tt.req)
		if tt.wantErr != "" {
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Errorf("shotScale(%+v) err = %v, want %q", tt.req, err, tt.wantErr)
			}
			continue
		}
		if err != nil || got != tt.want {
			t.Errorf("shotScale(%+v) = %d, %v, want %d", tt.req, got, err, tt.want)
		}
	}
}

func TestShotSize(t *testing.T) {
	if err := checkShotSize(4096, 10, 4096); err != nil {
		t.Errorf("4096 wide: %v", err)
	}
	if err := checkShotSize(10, 4097, 4096); err == nil || !strings.Contains(err.Error(), "10x4097 exceeds the maximum image size 4096") {
		t.Errorf("4097 high: %v", err)
	}
	if err := checkShotSize(0, 10, 4096); err == nil {
		t.Error("empty frame accepted")
	}
}

func TestShotTargetString(t *testing.T) {
	if ShotGame.String() != "game" || ShotWindow.String() != "window" {
		t.Errorf("names: %v %v", ShotGame, ShotWindow)
	}
}

func TestShotNames(t *testing.T) {
	at := time.Date(2026, 10, 31, 14, 5, 9, 0, time.Local)
	if got, want := shotName(at, 120), "screenshot-20261031-140509-120.png"; got != want {
		t.Errorf("shotName = %q, want %q", got, want)
	}
	for n, want := range map[int]string{0: "a.png", 1: "a.png", 2: "a-2.png", 3: "a-3.png"} {
		if got := shotCandidate("a.png", n); got != want {
			t.Errorf("shotCandidate(%d) = %q, want %q", n, got, want)
		}
	}
}

func TestShotEncodeAlpha(t *testing.T) {
	// One premultiplied pixel: half-transparent red.
	pix := []byte{128, 0, 0, 128}
	for _, keep := range []bool{false, true} {
		data, err := encodeShot(pix, 1, 1, keep)
		if err != nil {
			t.Fatal(err)
		}
		img, err := png.Decode(bytes.NewReader(data))
		if err != nil {
			t.Fatal(err)
		}
		got := color.NRGBAModel.Convert(img.At(0, 0)).(color.NRGBA)
		want := color.NRGBA{R: 128, A: 255} // over black
		if keep {
			want = color.NRGBA{R: 255, A: 128} // straight alpha
		}
		if got != want {
			t.Errorf("keepAlpha=%v: pixel %v, want %v", keep, got, want)
		}
	}
	if pix[3] != 128 {
		t.Error("encodeShot modified its input")
	}
	if _, err := encodeShot(pix, 2, 1, false); err == nil {
		t.Error("short pixel buffer accepted")
	}
}

func TestShotSinkFile(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "shots")
	at := time.Date(2026, 1, 2, 3, 4, 5, 0, time.Local)
	var paths []string
	for range 3 {
		p, err := defaultShotSink(dir, ShotRequest{}, at, 7, []byte("png"))
		if err != nil {
			t.Fatal(err)
		}
		paths = append(paths, filepath.Base(p))
	}
	want := []string{"screenshot-20260102-030405-7.png", "screenshot-20260102-030405-7-2.png", "screenshot-20260102-030405-7-3.png"}
	if strings.Join(paths, " ") != strings.Join(want, " ") {
		t.Errorf("names = %v, want %v", paths, want)
	}

	path := filepath.Join(t.TempDir(), "sub", "x.png")
	for _, data := range []string{"one", "two"} {
		got, err := defaultShotSink(dir, ShotRequest{Path: path}, at, 7, []byte(data))
		if err != nil || got != path {
			t.Fatalf("Path sink = %q, %v", got, err)
		}
	}
	if b, _ := os.ReadFile(path); string(b) != "two" {
		t.Errorf("Path file = %q, want overwritten", b)
	}
}

func TestShotDeliverCustomSink(t *testing.T) {
	sinkErr := errors.New("host gone")
	var sunk int
	e := New(Config{NoAudio: true, Screenshots: ScreenshotConfig{Sink: func(r ShotResult) error {
		sunk++
		return sinkErr
	}}})
	defer e.Close()
	if e.cfg.Screenshots.Dir != "screenshots" {
		t.Errorf("default Dir = %q", e.cfg.Screenshots.Dir)
	}
	var got []ShotResult
	done := func(r ShotResult) { got = append(got, r) }
	e.finishShot(ShotRequest{Done: done}, ShotResult{Err: errors.New("bad")})
	e.finishShot(ShotRequest{Done: done}, ShotResult{PNG: []byte("x")})
	e.Update()
	if len(got) != 2 || got[0].Err.Error() != "bad" || !errors.Is(got[1].Err, sinkErr) || sunk != 1 {
		t.Fatalf("results %+v, sink calls %d", got, sunk)
	}
	if n := e.shots.running.Load(); n != 0 {
		t.Errorf("running = %d after delivery", n)
	}
}

func TestShotQueueNoAllocWithoutRequests(t *testing.T) {
	e := New(Config{NoAudio: true})
	defer e.Close()
	e.Update()
	if n := testing.AllocsPerRun(20, func() { e.deliverShots() }); n != 0 {
		t.Errorf("deliverShots without captures allocates %v", n)
	}
}

func TestShotWaitsForStartup(t *testing.T) {
	e, _, _ := newStartupEngine(t, Config{})
	e.Update()
	if e.startup == nil {
		t.Fatal("startup screen not up")
	}
	e.Screenshot(ShotRequest{})
	e.Draw(ebiten.NewImage(8, 8))
	if len(e.shots.queue) != 1 {
		t.Fatalf("queue = %d while the startup screen is up, want the request kept", len(e.shots.queue))
	}
}
