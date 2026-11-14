//go:build ebitentest

package tilemap

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"testing"
	"testing/fstest"
	"time"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/render"
	testassets "github.com/weakpixel/engine/testdata/assets"
)

// patternFile is a w x h PNG whose every pixel differs from its neighbors,
// so a tile drawn from the wrong texel, place or orientation shows.
func patternFile(t testing.TB, w, h int) *fstest.MapFile {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.SetRGBA(x, y, color.RGBA{R: uint8(x * 37), G: uint8(y * 23), B: uint8((x ^ y) * 11), A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return &fstest.MapFile{Data: buf.Bytes()}
}

// drawFrame runs the extract on a fresh env, sorts the queue and draws it.
// It returns the pixels and the number of canvas items drawn.
func drawFrame(t testing.TB, e *xenv, size image.Point) ([]byte, int) {
	t.Helper()
	e.warmUp()
	e.run(16 * time.Millisecond)
	st := render.StoresOf(e.srv)
	e.queue.Sort(nil, st.Sprites)
	canvases := 0
	for i := range e.queue.Len() {
		if e.queue.At(i).Kind == render.KindCanvas {
			canvases++
		}
	}
	r := render.NewRenderer(st)
	r.SetCanvases(e.canvases)
	dst := ebiten.NewImage(size.X, size.Y)
	r.Draw(dst, e.queue, e.cam, 1)
	pix := make([]byte, 4*size.X*size.Y)
	dst.ReadPixels(pix)
	return pix, canvases
}

// TestChunkedMatchesTiles draws scenes tile by tile and chunked and
// compares every pixel: pre-rendering must not change the picture.
func TestChunkedMatchesTiles(t *testing.T) {
	lib := testassets.Lib()
	lib["temple/temple.png"] = patternFile(t, 64, 32)
	lib["temple/colliders.png"] = patternFile(t, 48, 16)
	small := tilesetFiles(t)
	small["t.png"] = patternFile(t, 16, 16)
	small["tall.png"] = patternFile(t, 8, 24)
	// The flips of plainMap and the tall tile (overhang 16 px) of the
	// second layer, which reaches into the chunk padding.
	small["flips.tmj"] = textFile(`{"orientation":"orthogonal","width":4,"height":3,"tilewidth":8,"tileheight":8,
		"tilesets":[{"firstgid":1,"source":"t.tsj"},{"firstgid":5,"source":"tall.tsj"}],
		"layers":[` + plainLayer("") + `,{"type":"tilelayer","name":"tall","width":4,"height":3,"data":[0,0,0,0, 0,0,0,0, 0,5,0,5]}]}`)

	tests := []struct {
		name   string
		files  fstest.MapFS
		key    string
		screen image.Point
		zoom   float32
		look   gfx.Vec2
	}{
		{"level zoom 3", lib, "world-1/level-2.tmj", image.Pt(160, 120), 3, gfx.Vec2{X: 30, Y: 28}},
		{"level sub-pixel camera", lib, "world-1/level-2.tmj", image.Pt(96, 64), 2, gfx.Vec2{X: 30.25, Y: 20.5}},
		{"level 1 whole", lib, "world-1/level-1.tmj", image.Pt(96, 64), 1, gfx.Vec2{X: 48, Y: 32}},
		{"flips and overhang", small, "flips.tmj", image.Pt(64, 64), 2, gfx.Vec2{X: 16, Y: 12}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			env := func(chunked bool) *xenv {
				e := newXEnvFS(t, tc.files, tc.screen, tc.zoom)
				e.spawn(tc.key, SpawnOptions{Chunked: chunked})
				e.look(tc.look)
				return e
			}
			want, _ := drawFrame(t, env(false), tc.screen)
			got, canvases := drawFrame(t, env(true), tc.screen)
			if canvases == 0 {
				t.Fatal("the chunked frame drew no canvas")
			}
			diff, first := 0, -1
			for i := 0; i < len(want); i += 4 {
				if !bytes.Equal(want[i:i+4], got[i:i+4]) {
					if first < 0 {
						first = i / 4
					}
					diff++
				}
			}
			if diff > 0 {
				x, y := first%tc.screen.X, first/tc.screen.X
				t.Fatalf("%d pixels differ; first at %d,%d: tiles %v, chunked %v",
					diff, x, y, want[4*first:4*first+4], got[4*first:4*first+4])
			}
		})
	}
}
