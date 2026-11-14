package toolshell

import (
	"bytes"
	"image"
	"image/png"
	"io/fs"
	"testing"
	"testing/fstest"

	"github.com/weakpixel/engine/game"
	"github.com/weakpixel/engine/render"
)

func TestOverlayFS(t *testing.T) {
	base := fstest.MapFS{"a.txt": {Data: []byte("base")}}
	o := NewOverlayFS(base)
	o.Set("fx-builtin/x.bin", []byte("mem"))
	for name, want := range map[string]string{"a.txt": "base", "fx-builtin/x.bin": "mem"} {
		got, err := fs.ReadFile(o, name)
		if err != nil || string(got) != want {
			t.Errorf("ReadFile(%s) = %q, %v; want %q", name, got, err, want)
		}
	}
	if fi, err := fs.Stat(o, "fx-builtin/x.bin"); err != nil || fi.Size() != 3 || fi.Name() != "x.bin" {
		t.Errorf("Stat = %v, %v", fi, err)
	}
	if _, err := fs.ReadFile(o, "missing"); err == nil {
		t.Error("missing file read")
	}
	if _, err := fs.ReadFile(NewOverlayFS(nil), "a.txt"); err == nil {
		t.Error("nil base served a file")
	}
}

// TestEngineLoadsOverlayAsset settles the built-in sprite question: the
// engine's asset loading accepts a file that only the overlay holds.
func TestEngineLoadsOverlayAsset(t *testing.T) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 8, 8))); err != nil {
		t.Fatal(err)
	}
	o := NewOverlayFS(fstest.MapFS{})
	o.Set("fx-builtin/dot.png", buf.Bytes())
	e := game.New(game.Config{NoAudio: true, Width: 64, Height: 64, FS: o})
	defer e.Close()
	if _, err := render.LoadSpriteSync(e.Assets().Global(), "fx-builtin/dot.png"); err != nil {
		t.Fatal(err)
	}
}
