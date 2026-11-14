package render

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/hajimehoshi/ebiten/v2/text/v2"
	"golang.org/x/image/font/gofont/goregular"

	"github.com/weakpixel/asset"
	"github.com/weakpixel/engine/gfx"
)

func TestFontSizesShareOneSource(t *testing.T) {
	for _, workers := range []int{0, 3} {
		e := newAssetEnv(t, fstest.MapFS{"fonts/ui.ttf": {Data: goregular.TTF}}, workers)
		sc := e.srv.NewScope("s")
		small := LoadFont(sc, "fonts/ui.ttf#12")
		big := LoadFont(sc, "fonts/ui.ttf#24")
		e.settle(sc)

		fonts, sources := FontStoreOf(e.srv), asset.StoreOf[FontSource](e.srv)
		if !fonts.Ready(small) || !fonts.Ready(big) {
			t.Fatalf("workers=%d: not ready: %v / %v", workers, fonts.Err(small), fonts.Err(big))
		}
		if sources.Len() != 1 {
			t.Fatalf("workers=%d: %d font sources, want 1 shared", workers, sources.Len())
		}
		s, b := fonts.Get(small), fonts.Get(big)
		if s.Face.(*text.GoTextFace).Size != 12 || b.Face.(*text.GoTextFace).Size != 24 {
			t.Fatalf("workers=%d: sizes %v %v", workers, s.Face.(*text.GoTextFace).Size, b.Face.(*text.GoTextFace).Size)
		}
		if s.LineHeight <= 0 || b.LineHeight <= s.LineHeight {
			t.Fatalf("workers=%d: line heights %v %v", workers, s.LineHeight, b.LineHeight)
		}
		wS, _ := text.Measure("Hello", s.Face, 0)
		wB, _ := text.Measure("Hello", b.Face, 0)
		if wS <= 0 || wB <= wS {
			t.Fatalf("workers=%d: measured widths %v %v", workers, wS, wB)
		}
	}
}

func TestFontSourceDirect(t *testing.T) {
	e := newAssetEnv(t, fstest.MapFS{"f.ttf": {Data: goregular.TTF}}, 0)
	sc := e.srv.NewScope("s")
	h, err := asset.LoadSync[FontSource](sc, "f.ttf")
	if err != nil || asset.StoreOf[FontSource](e.srv).Get(h).Source == nil {
		t.Fatalf("LoadSync = %v", err)
	}
}

func TestFontFailures(t *testing.T) {
	e := newAssetEnv(t, fstest.MapFS{
		"f.ttf":   {Data: goregular.TTF},
		"bad.ttf": textFile("not a font"),
	}, 0)
	sc := e.srv.NewScope("s")
	fonts := FontStoreOf(e.srv)
	tests := map[string]string{
		"f.ttf":         "needs a size",
		"f.ttf#abc":     "positive number",
		"f.ttf#0":       "positive number",
		"f.ttf#-3":      "positive number",
		"f.ttf#NaN":     "positive number",
		"bad.ttf#12":    "dependency",
		"missing.ttf#8": "file does not exist",
	}
	for key, want := range tests {
		t.Run(key, func(t *testing.T) {
			h := LoadFont(sc, key)
			e.settle(sc)
			if err := fonts.Err(h); err == nil || !strings.Contains(err.Error(), want) {
				t.Fatalf("err = %v, want it to contain %q", err, want)
			}
			if fonts.Get(h).Face == nil {
				t.Fatal("failed font did not resolve to the placeholder")
			}
		})
	}
}

func TestFontPlaceholder(t *testing.T) {
	e := newAssetEnv(t, nil, 0)
	p := FontStoreOf(e.srv).Get(gfx.FontHandle{})
	if p.Face == nil || p.LineHeight <= 0 {
		t.Fatalf("placeholder font = %+v", p)
	}
}
