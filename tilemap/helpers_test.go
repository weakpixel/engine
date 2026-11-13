package tilemap

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/weakpixel/asset"
	"github.com/weakpixel/engine/render"
)

// pngFile returns a w x h opaque PNG as a MapFS entry.
func pngFile(t testing.TB, w, h int) *fstest.MapFile {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.SetRGBA(x, y, color.RGBA{G: 255, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return &fstest.MapFile{Data: buf.Bytes()}
}

func textFile(s string) *fstest.MapFile { return &fstest.MapFile{Data: []byte(s)} }

// newServer is a server with the render and tilemap loaders registered.
func newServer(t testing.TB, files fstest.MapFS) *asset.Server {
	t.Helper()
	srv := asset.NewServer(asset.Options{FS: files, OnError: func(error) {}})
	t.Cleanup(srv.Close)
	render.RegisterLoaders(srv, render.LoaderOptions{})
	RegisterLoaders(srv)
	return srv
}

func mustContain(t *testing.T, err error, want ...string) {
	t.Helper()
	if err == nil {
		t.Fatalf("got no error, want one containing %q", want)
	}
	for _, w := range want {
		if !strings.Contains(err.Error(), w) {
			t.Fatalf("error %q does not contain %q", err, w)
		}
	}
}

func mustPanic(t *testing.T, want string, f func()) {
	t.Helper()
	defer func() {
		r := recover()
		if r == nil {
			t.Fatalf("expected panic containing %q", want)
		}
		if msg, _ := r.(string); !strings.Contains(msg, want) {
			t.Fatalf("panic %v does not contain %q", r, want)
		}
	}()
	f()
}
