//go:build ebitentest

package toolshell

import (
	"bytes"
	"encoding/base64"
	"image/png"
	"os"
	"testing"
	"time"

	"github.com/hajimehoshi/ebiten/v2"
)

// shotsFor ticks and draws until n screenshot answers arrived.
func shotsFor(t *testing.T, f *fixture, tr *fakeTransport, n int) []map[string]any {
	t.Helper()
	var got []map[string]any
	f.e.Draw(ebiten.NewImage(320, 180))
	for range 1000 {
		f.tick(time.Millisecond)
		got = append(got, tr.answers()...)
		if len(got) >= n {
			return got
		}
		time.Sleep(time.Millisecond) // the PNG is encoded on another goroutine
	}
	t.Fatalf("got %d screenshot answers, want %d", len(got), n)
	return nil
}

// TestScreenshotAnswers: a request is answered with its id and the content
// frame; the Screenshot button sends an unasked answer (id "") and no file
// is written while a host is connected.
func TestScreenshotAnswers(t *testing.T) {
	t.Chdir(t.TempDir())
	tr := &fakeTransport{}
	f := newFixture(t, Config{Transport: tr})
	f.tick(time.Millisecond)
	f.send(`{"type":"screenshot","id":"s1","scale":2}`)
	f.tick(time.Millisecond)
	got := shotsFor(t, f, tr, 1)
	if got[0]["id"] != "s1" || got[0]["width"] != float64(640) || got[0]["height"] != float64(360) {
		t.Fatalf("answer %v", got[0])
	}
	b := decodeShot(t, got[0]["png"])
	if _, err := png.Decode(bytes.NewReader(b)); err != nil {
		t.Fatal(err)
	}

	if _, err := f.s.SetView("view.shot", nil); err != nil {
		t.Fatal(err)
	}
	got = shotsFor(t, f, tr, 1)
	if got[0]["id"] != "" || got[0]["error"] != nil {
		t.Fatalf("button answer %v", got[0])
	}
	if _, err := os.Stat("screenshots"); err == nil {
		t.Error("a file was written under a host")
	}
}

func decodeShot(t *testing.T, v any) []byte {
	t.Helper()
	b, err := base64.StdEncoding.DecodeString(v.(string))
	if err != nil {
		t.Fatal(err)
	}
	return b
}
