//go:build ebitentest

package bridge

import (
	"bytes"
	"encoding/base64"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
)

// TestScreenshotRoundTrip: a request is answered with a base64 PNG of the
// scaled frame and the request's id, and no file is written.
func TestScreenshotRoundTrip(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)
	f := newFixture(t)
	f.recv(documentMsg(visual("a", 1, 2)))
	f.recv(`{"type":"screenshot","id":"s1","scale":2}`)
	f.recv(`{"type":"screenshot","id":"s2","scale":99}`)
	f.e.Draw(ebiten.NewImage(320, 180))
	var ok, bad map[string]any
	for range 1000 {
		f.e.Update()
		for _, m := range f.host.take("screenshot") {
			if m["id"] == "s1" {
				ok = m
			} else {
				bad = m
			}
		}
		if ok != nil && bad != nil {
			break
		}
		time.Sleep(time.Millisecond) // the PNG is encoded on another goroutine
	}
	if ok == nil || bad == nil {
		t.Fatalf("missing answers: ok=%v bad=%v ", ok != nil, bad != nil)
	}
	if ok["width"] != float64(640) || ok["height"] != float64(360) || ok["error"] != nil {
		t.Errorf("answer %v", ok)
	}
	// encoding/json decodes the base64 PNG into the same bytes the
	// message struct holds.
	if _, err := png.Decode(bytes.NewReader(decodePNG(t, ok["png"]))); err != nil {
		t.Errorf("png: %v", err)
	}
	if bad["error"] == nil {
		t.Errorf("scale 99 answer %v", bad)
	}
	if files, _ := filepath.Glob(filepath.Join(dir, "*", "*")); len(files) != 0 {
		t.Errorf("files written: %v", files)
	}
	if _, err := os.Stat(filepath.Join(dir, "screenshots")); err == nil {
		t.Error("screenshots folder created")
	}
}

func decodePNG(t *testing.T, v any) []byte {
	t.Helper()
	b, err := base64.StdEncoding.DecodeString(v.(string))
	if err != nil {
		t.Fatal(err)
	}
	return b
}
