package render

import (
	"image"
	"image/color"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/weakpixel/asset"
)

func TestTextureLoads(t *testing.T) {
	for _, workers := range []int{0, 3} {
		e := newAssetEnv(t, fstest.MapFS{"hero.png": pngFile(t, 16, 8, color.RGBA{R: 255, A: 255})}, workers)
		sc := e.srv.NewScope("s")
		h := asset.Load[Texture](sc, "hero.png")
		e.settle(sc)

		tex := asset.StoreOf[Texture](e.srv)
		if !tex.Ready(h) {
			t.Fatalf("workers=%d: not ready: %v", workers, tex.Err(h))
		}
		got := tex.Get(h)
		if got.Size != (image.Point{X: 16, Y: 8}) || got.Bytes != 16*8*4 || got.Image.Bounds().Size() != got.Size {
			t.Fatalf("workers=%d: texture = %+v", workers, got)
		}
		if b := e.srv.Stats().Bytes; b != 16*8*4 {
			t.Fatalf("workers=%d: Stats.Bytes = %d, want the texture's GPU size", workers, b)
		}
		if got.Image == tex.Get(asset.Handle[Texture]{}).Image {
			t.Fatal("loaded texture shares the placeholder image")
		}
	}
}

func TestTextureFailures(t *testing.T) {
	e := newAssetEnv(t, fstest.MapFS{
		"junk.png":      textFile("not an image"),
		"truncated.png": {Data: pngFile(t, 8, 8, color.RGBA{A: 255}).Data[:40]},
	}, 0)
	sc := e.srv.NewScope("s")
	tex := asset.StoreOf[Texture](e.srv)
	placeholder := tex.Get(asset.Handle[Texture]{}).Image // pointers from Get die at the next Update; images don't

	for _, key := range []string{"junk.png", "missing.png", "truncated.png"} {
		t.Run(key, func(t *testing.T) {
			h := asset.Load[Texture](sc, key)
			e.srv.Update()
			if tex.Err(h) == nil {
				t.Fatalf("%s did not fail", key)
			}
			if tex.Get(h).Image != placeholder {
				t.Fatalf("%s did not resolve to the placeholder", key)
			}
		})
	}
	if len(e.errs) != 3 {
		t.Fatalf("OnError called %d times: %v", len(e.errs), e.errs)
	}
}

func TestTextureUnloadOnRelease(t *testing.T) {
	e := newAssetEnv(t, fstest.MapFS{"a.png": pngFile(t, 4, 4, color.RGBA{A: 255})}, 0)
	sc := e.srv.NewScope("s")
	tex := asset.StoreOf[Texture](e.srv)
	h := asset.Load[Texture](sc, "a.png")
	e.srv.Update()
	img := tex.Get(h).Image

	sc.Release()
	e.srv.Update()
	if tex.Ready(h) || tex.Len() != 0 {
		t.Fatal("texture still loaded after its scope was released")
	}
	_ = img // Deallocate has no observable state headless; the call must not panic
}

func TestRegisterLoadersTwicePanics(t *testing.T) {
	e := newAssetEnv(t, nil, 0)
	mustPanic(t, "asset: type already registered", func() { RegisterLoaders(e.srv, LoaderOptions{}) })
}

func TestTransparentPlaceholder(t *testing.T) {
	srv := asset.NewServer(asset.Options{})
	defer srv.Close()
	RegisterLoaders(srv, LoaderOptions{TransparentPlaceholders: true})
	p := asset.StoreOf[Texture](srv).Get(asset.Handle[Texture]{})
	if p.Size != (image.Point{X: 1, Y: 1}) {
		t.Fatalf("transparent placeholder size = %v", p.Size)
	}
	srv2 := asset.NewServer(asset.Options{})
	defer srv2.Close()
	RegisterLoaders(srv2, LoaderOptions{})
	if p := asset.StoreOf[Texture](srv2).Get(asset.Handle[Texture]{}); p.Size.X < 2 {
		t.Fatalf("checker placeholder size = %v", p.Size)
	}
}

func TestTextureErrorsNameTheFile(t *testing.T) {
	e := newAssetEnv(t, fstest.MapFS{"junk.png": textFile("x")}, 0)
	h := asset.Load[Texture](e.srv.Global(), "junk.png")
	e.srv.Update()
	if err := asset.StoreOf[Texture](e.srv).Err(h); err == nil || !strings.Contains(err.Error(), "junk.png") {
		t.Fatalf("err = %v", err)
	}
}

func TestUploadImageTooBig(t *testing.T) {
	img := image.NewRGBA(image.Rect(0, 0, 20000, 1))
	tooBig := func(image.Image) *ebiten.Image {
		panic("atlas: the image being put on an atlas is too big: width: 20000, height: 1")
	}
	e, err := uploadImage(img, tooBig)
	if e != nil || err == nil {
		t.Fatalf("uploadImage = %v, %v; want nil and an error", e, err)
	}
	if want := "render: texture 20000x1 is larger than this GPU supports"; err.Error() != want {
		t.Errorf("err = %q, want %q", err, want)
	}

	other := func(image.Image) *ebiten.Image { panic("something else") }
	mustPanic(t, "something else", func() { _, _ = uploadImage(img, other) })
}
