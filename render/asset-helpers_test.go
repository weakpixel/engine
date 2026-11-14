package render

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/weakpixel/asset"
	"github.com/weakpixel/engine/gfx"
)

// pngFile returns a w x h PNG as a MapFS entry. Every pixel is c.
func pngFile(t testing.TB, w, h int, c color.RGBA) *fstest.MapFile {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.SetRGBA(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return &fstest.MapFile{Data: buf.Bytes()}
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

func textFile(s string) *fstest.MapFile { return &fstest.MapFile{Data: []byte(s)} }

// assetEnv is a server with the render loaders registered. Errors reported
// through OnError are collected.
type assetEnv struct {
	t    testing.TB
	srv  *asset.Server
	errs []error
}

func newAssetEnv(t testing.TB, files fstest.MapFS, workers int) *assetEnv {
	t.Helper()
	e := &assetEnv{t: t}
	e.srv = asset.NewServer(asset.Options{
		FS:      files,
		Workers: workers,
		OnError: func(err error) { e.errs = append(e.errs, err) },
	})
	RegisterLoaders(e.srv, LoaderOptions{})
	t.Cleanup(e.srv.Close)
	return e
}

// settle runs Update until sc is done. Update is called on the test
// goroutine, so every Finish (and so every Ebiten call) runs there. It sleeps
// between ticks so that, with workers, a loaded machine (make all runs
// packages in parallel) cannot spin through every tick before Decode ends.
func (e *assetEnv) settle(sc *asset.Scope) {
	e.t.Helper()
	for range 2000 {
		e.srv.Update()
		if sc.Done() {
			return
		}
		time.Sleep(time.Millisecond)
	}
	e.t.Fatal("scope never became Done")
}

// loadSheet loads a grid sheet of n 4x4 red frames ("name.sheet.json" next to
// "name.png") into e's filesystem and returns its sprites in file order. files
// must be the map e was built with.
func (e *assetEnv) loadSheet(files fstest.MapFS, name string, n int) []gfx.SpriteHandle {
	e.t.Helper()
	files[name+".sheet.json"] = textFile(`{"image":"` + name + `.png","frameW":4,"frameH":4}`)
	files[name+".png"] = pngFile(e.t, 4*n, 4, color.RGBA{R: 255, A: 255})
	sc := e.srv.NewScope(name)
	ah := LoadAtlas(sc, name+".sheet.json")
	e.settle(sc)
	atlases := AtlasStoreOf(e.srv)
	if !atlases.Ready(ah) {
		e.t.Fatalf("sheet %s not ready: %v", name, atlases.Err(ah))
	}
	return atlases.Get(ah).Order
}

// testSprites loads n red 4x4 frames of one texture through a real
// asset.Server and returns the env and the sprite handles in order.
func testSprites(t testing.TB, n int) (*assetEnv, []gfx.SpriteHandle) {
	t.Helper()
	files := fstest.MapFS{}
	e := newAssetEnv(t, files, 0)
	return e, e.loadSheet(files, "s", n)
}
