package testassets_test

import (
	"testing"

	"github.com/weakpixel/asset"
	"github.com/weakpixel/engine/anim"
	"github.com/weakpixel/engine/asset/library"
	"github.com/weakpixel/engine/render"
	"github.com/weakpixel/engine/tilemap"
	testassets "github.com/weakpixel/engine/testdata/assets"
)

// maxBytes keeps the library small: tests should not grow it into a demo.
const maxBytes = 64 << 10

func TestLibStaysSmall(t *testing.T) {
	total := 0
	for _, f := range testassets.Lib() {
		total += len(f.Data)
	}
	if total > maxBytes {
		t.Fatalf("the library is %d bytes, over %d", total, maxBytes)
	}
}

// TestLibIsValid: every reference resolves, and the world, both maps and the
// animation load with the engine's loaders.
func TestLibIsValid(t *testing.T) {
	_, findings, err := library.Scan(testassets.Lib())
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range findings {
		if f.Severity == library.Error {
			t.Errorf("%v", f)
		}
	}

	srv := asset.NewServer(asset.Options{FS: testassets.Lib()})
	defer srv.Close()
	render.RegisterLoaders(srv, render.LoaderOptions{})
	tilemap.RegisterLoaders(srv)
	anim.RegisterLoaders(srv)
	if _, err := tilemap.LoadWorldSync(srv.Global(), "world-1/world-1.world"); err != nil {
		t.Error(err)
	}
	if _, err := anim.LoadClipSetSync(srv.Global(), "global/hero/hero.anim.json"); err != nil {
		t.Error(err)
	}
}
