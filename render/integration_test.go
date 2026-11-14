package render

import (
	"testing"
	"testing/fstest"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/weakpixel/asset"
	"github.com/weakpixel/engine/gfx"
)

// A level-sized load: atlas + frames + font sizes + shader + whole-image
// sprite, with real workers, loaded through one scope.
func TestLevelLoadWithWorkers(t *testing.T) {
	files := fstest.MapFS{
		"gfx/hero.json":     textFile(tpArray),
		"gfx/hero.png":      pngFile(t, 32, 16, opaque),
		"gfx/fx.sheet.json": textFile(`{"image":"fx.png","frameW":4,"frameH":4}`),
		"gfx/fx.png":        pngFile(t, 16, 16, opaque),
		"ui/cursor.png":     pngFile(t, 8, 8, opaque),
		"fonts/ui.ttf":      {Data: goregular.TTF},
		"fx/tint.kage":      textFile(kageOK),
	}
	e := newAssetEnv(t, files, 4)
	level := e.srv.NewScope("level")
	hero := LoadAtlas(level, "gfx/hero.json")
	idle := LoadSprite(level, "gfx/hero.json#a_first")
	fx := LoadSprite(level, "gfx/fx.sheet.json#15")
	cursor := LoadSprite(level, "ui/cursor.png")
	f12 := LoadFont(level, "fonts/ui.ttf#12")
	f20 := LoadFont(level, "fonts/ui.ttf#20")
	sh := asset.Load[Shader](level, "fx/tint.kage")

	if ready, total := level.Progress(); ready != 0 || total == 0 {
		t.Fatalf("Progress before load = %d/%d", ready, total)
	}
	e.settle(level)
	if err := level.Err(); err != nil {
		t.Fatalf("level errors: %v", err)
	}

	ready, total := level.Progress()
	// hero atlas+texture, fx atlas+texture+frame 15 (children counted via the
	// atlas), cursor sprite+texture, font source+2 fonts, shader; the extra
	// hero frame is the same atlas.
	if ready != total {
		t.Fatalf("Progress = %d/%d", ready, total)
	}
	for name, ok := range map[string]bool{
		"hero":   AtlasStoreOf(e.srv).Ready(hero),
		"idle":   SpriteStoreOf(e.srv).Ready(idle),
		"fx":     SpriteStoreOf(e.srv).Ready(fx),
		"cursor": SpriteStoreOf(e.srv).Ready(cursor),
		"f12":    FontStoreOf(e.srv).Ready(f12),
		"f20":    FontStoreOf(e.srv).Ready(f20),
		"shader": asset.StoreOf[Shader](e.srv).Ready(sh),
	} {
		if !ok {
			t.Errorf("%s not ready", name)
		}
	}
	if len(e.errs) != 0 {
		t.Fatalf("OnError: %v", e.errs)
	}

	// Releasing the level frees everything.
	level.Release()
	e.srv.Update()
	if st := e.srv.Stats(); st.States[asset.Ready]+st.States[asset.Failed]+st.States[asset.Waiting] != 0 {
		t.Fatalf("assets left after release: %v", st.States)
	}
}

func TestProgressCountsTextureAtlasAndFrames(t *testing.T) {
	e := newAssetEnv(t, fstest.MapFS{
		"s.sheet.json": textFile(`{"image":"t.png","frameW":4,"frameH":4}`),
		"t.png":        pngFile(t, 8, 4, opaque),
	}, 0)
	sc := e.srv.NewScope("s")
	LoadSprite(sc, "s.sheet.json#0")
	LoadSprite(sc, "s.sheet.json#1")
	if ready, total := sc.Progress(); ready != 0 || total != 3 {
		t.Fatalf("before: %d/%d, want 0/3 (2 reserved frames + their atlas)", ready, total)
	}
	e.settle(sc)
	// Frames, atlas and the texture the atlas depends on.
	if ready, total := sc.Progress(); ready != 4 || total != 4 {
		t.Fatalf("after: %d/%d, want 4/4", ready, total)
	}
}

func TestLoadSyncAtlasWithWorkers(t *testing.T) {
	e := newAssetEnv(t, fstest.MapFS{
		"s.sheet.json": textFile(`{"image":"t.png","frameW":4,"frameH":4}`),
		"t.png":        pngFile(t, 8, 8, opaque),
	}, 3)
	h, err := LoadAtlasSync(e.srv.Global(), "s.sheet.json")
	if err != nil {
		t.Fatal(err)
	}
	if got := len(AtlasStoreOf(e.srv).Get(h).Order); got != 4 {
		t.Fatalf("frames = %d, want 4", got)
	}
}

func TestSpriteGetDoesNotAllocate(t *testing.T) {
	if testing.CoverMode() != "" || raceEnabled {
		t.Skip("instrumentation allocates")
	}
	e := newAssetEnv(t, fstest.MapFS{"c.png": pngFile(t, 4, 4, opaque)}, 0)
	sc := e.srv.NewScope("s")
	h := LoadSprite(sc, "c.png")
	missing := LoadSprite(sc, "missing.png")
	e.settle(sc)
	sprites := SpriteStoreOf(e.srv)

	for name, hd := range map[string]gfx.SpriteHandle{"ready": h, "failed": missing, "zero": {}} {
		if n := testing.AllocsPerRun(100, func() { _ = sprites.Get(hd).Image }); n != 0 {
			t.Errorf("Get(%s) allocates %v per call", name, n)
		}
	}
}

func BenchmarkSpriteGet(b *testing.B) {
	srv := asset.NewServer(asset.Options{FS: fstest.MapFS{"c.png": pngFile(b, 4, 4, opaque)}})
	defer srv.Close()
	RegisterLoaders(srv, LoaderOptions{})
	h, err := LoadSpriteSync(srv.Global(), "c.png")
	if err != nil {
		b.Fatal(err)
	}
	// The facade must cost nothing over the generic store it wraps.
	b.Run("facade", func(b *testing.B) {
		sprites := SpriteStoreOf(srv)
		b.ReportAllocs()
		for b.Loop() {
			_ = sprites.Get(h).Image
		}
	})
	b.Run("generic", func(b *testing.B) {
		sprites, gh := asset.StoreOf[Sprite](srv), asset.HandleOf[Sprite](h.Ref)
		b.ReportAllocs()
		for b.Loop() {
			_ = sprites.Get(gh).Image
		}
	})
}
