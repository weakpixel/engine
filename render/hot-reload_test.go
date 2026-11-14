package render

import (
	"image"
	"image/color"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/weakpixel/asset"
)

// newWatchEnv is newAssetEnv with hot reload polling on every Update.
func newWatchEnv(t *testing.T, files fstest.MapFS) *assetEnv {
	t.Helper()
	e := &assetEnv{t: t}
	e.srv = asset.NewServer(asset.Options{
		FS:            files,
		WatchInterval: time.Nanosecond,
		OnError:       func(err error) { e.errs = append(e.errs, err) },
	})
	RegisterLoaders(e.srv, LoaderOptions{})
	t.Cleanup(e.srv.Close)
	return e
}

// save replaces a file with a newer modtime, as an editor does.
func save(files fstest.MapFS, name string, f *fstest.MapFile, n int) {
	f.ModTime = time.Unix(int64(n), 0)
	files[name] = f
}

func TestHotReloadAtlasTexture(t *testing.T) {
	red := color.RGBA{R: 255, A: 255}
	files := fstest.MapFS{"fx/s.sheet.json": textFile(`{"image":"t.png","frameW":8,"frameH":8}`)}
	save(files, "fx/t.png", pngFile(t, 16, 8, opaque), 1)
	e := newWatchEnv(t, files)
	sc := e.srv.NewScope("s")
	ah := LoadAtlas(sc, "fx/s.sheet.json")
	e.settle(sc)
	atlases, sprites := AtlasStoreOf(e.srv), SpriteStoreOf(e.srv)
	first := atlases.Get(ah).Order[1]
	oldSheet := sprites.Get(first).source()

	// A wider texture: the atlas is re-cut in place on the next Update.
	save(files, "fx/t.png", pngFile(t, 32, 8, red), 2)
	ev := e.srv.Update()
	a := atlases.Get(ah)
	if len(a.Order) != 4 || a.Order[1] != first {
		t.Fatalf("after reload: %d frames, frame 1 same handle = %v", len(a.Order), a.Order[1] == first)
	}
	sp := sprites.Get(first)
	if sp.source() == oldSheet || sp.source().Bounds() != image.Rect(0, 0, 32, 8) {
		t.Fatal("sprite still cut from the old texture")
	}
	if sprites.Generic().Version(asset.HandleOf[Sprite](first.Ref)) != 2 {
		t.Fatal("sprite version not bumped")
	}
	modified := 0
	for e := range ev.All() {
		if e.Kind == asset.EventModified {
			modified++
		}
	}
	if modified != 4 { // texture, atlas, sprites 0 and 1 (2 and 3 are new)
		t.Fatalf("modified events = %d, want 4", modified)
	}

	// A broken PNG keeps the old texture and is reported once.
	save(files, "fx/t.png", textFile("not a png"), 3)
	for range 3 {
		e.srv.Update()
	}
	if len(atlases.Get(ah).Order) != 4 || !sprites.Ready(first) {
		t.Fatal("broken PNG replaced the atlas")
	}
	if len(e.errs) != 1 || !strings.Contains(e.errs[0].Error(), "asset: reload failed, keeping previous version: fx/t.png") {
		t.Fatalf("errors = %v", e.errs)
	}

	// A narrower texture: frames that no longer fit fail, the rest stay.
	save(files, "fx/t.png", pngFile(t, 8, 8, red), 4)
	e.srv.Update()
	if !sprites.Ready(atlases.Get(ah).Order[0]) {
		t.Fatal("frame 0 lost")
	}
	if err := sprites.Err(first); err == nil || !strings.Contains(err.Error(), `no longer has Sprite "1"`) {
		t.Fatalf("removed frame error = %v", err)
	}
}

func TestHotReloadWholeImageSprite(t *testing.T) {
	files := fstest.MapFS{}
	save(files, "ui/cursor.png", pngFile(t, 4, 4, opaque), 1)
	e := newWatchEnv(t, files)
	sc := e.srv.NewScope("s")
	h := LoadSprite(sc, "ui/cursor.png")
	e.settle(sc)
	sprites := SpriteStoreOf(e.srv)

	save(files, "ui/cursor.png", pngFile(t, 6, 3, opaque), 2)
	for range 2 { // the texture reload re-finishes the sprite; the sprite's own reload follows
		e.srv.Update()
	}
	if got := sprites.Get(h).Image.Bounds().Size(); got != image.Pt(6, 3) {
		t.Fatalf("sprite size after reload = %v", got)
	}
	if len(e.errs) != 0 {
		t.Fatal(e.errs)
	}
}

// kageSolid paints the sprite's opaque pixels with the given color.
func kageSolid(rgb string) string {
	return "//kage:unit pixels\n\npackage main\n\nfunc Fragment(dst vec4, src vec2, color vec4) vec4 {\n\ta := imageSrc0At(src).a\n\treturn vec4(" + rgb + ", 1) * a\n}\n"
}

func TestHotReloadShader(t *testing.T) {
	files := fstest.MapFS{}
	save(files, "fx/s.kage", textFile(kageSolid("1, 0, 0")), 1)
	e := newWatchEnv(t, files)
	sc := e.srv.NewScope("s")
	h := LoadShader(sc, "fx/s.kage")
	e.settle(sc)
	shaders := ShaderStoreOf(e.srv)
	first := shaders.Get(h).Shader
	if first == nil {
		t.Fatalf("shader not ready: %v", shaders.Err(h))
	}

	// A new source: same handle, new compiled shader, version and event.
	save(files, "fx/s.kage", textFile(kageSolid("0, 1, 0")), 2)
	ev := e.srv.Update()
	if got := shaders.Get(h).Shader; got == nil || got == first {
		t.Fatalf("shader not swapped: %v", got)
	}
	if v := shaders.Generic().Version(asset.HandleOf[Shader](h.Ref)); v != 2 {
		t.Fatalf("version = %d, want 2", v)
	}
	modified := 0
	for ev := range ev.All() {
		if ev.Kind == asset.EventModified {
			modified++
		}
	}
	if modified != 1 {
		t.Fatalf("modified events = %d, want 1", modified)
	}

	// A save that does not compile, and one that declares Params with the
	// wrong type, keep the working shader; each is reported once.
	kept := shaders.Get(h).Shader
	save(files, "fx/s.kage", textFile("package main\nfunc Fragment( {"), 3)
	for range 3 {
		e.srv.Update()
	}
	save(files, "fx/s.kage", textFile("//kage:unit pixels\n\npackage main\n\nvar Params float\n\nfunc Fragment(dst vec4, src vec2, color vec4) vec4 {\n\treturn color\n}\n"), 4)
	for range 3 {
		e.srv.Update()
	}
	if shaders.Get(h).Shader != kept || !shaders.Ready(h) {
		t.Fatal("a broken save replaced the working shader")
	}
	if len(e.errs) != 2 {
		t.Fatalf("errors = %v, want 2", e.errs)
	}
	for _, err := range e.errs {
		if !strings.Contains(err.Error(), "asset: reload failed, keeping previous version: fx/s.kage") {
			t.Fatalf("error = %v", err)
		}
	}

	// A fixed save loads again.
	save(files, "fx/s.kage", textFile(kageSolid("0, 0, 1")), 5)
	e.srv.Update()
	if shaders.Get(h).Shader == kept {
		t.Fatal("fixed shader not loaded")
	}
}
