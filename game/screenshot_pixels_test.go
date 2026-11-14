//go:build ebitentest

package game

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/input"
	"github.com/weakpixel/engine/render"
)

// kageScanlines darkens every second row of window pixels.
const kageScanlines = `//kage:unit pixels

package main

func Fragment(dst vec4, src vec2, color vec4) vec4 {
	if mod(floor(dst.y), 2) == 1 {
		return vec4(0, 0, 0, 1)
	}
	return imageSrc0At(src)
}
`

// shoot queues req and returns a function that runs Updates until its
// result arrived.
func shoot(t *testing.T, e *Engine, req ShotRequest) func() (ShotResult, image.Image) {
	t.Helper()
	var res *ShotResult
	req.Done = func(r ShotResult) { res = &r }
	e.Screenshot(req)
	return func() (ShotResult, image.Image) {
		t.Helper()
		for range 1000 {
			if res != nil {
				break
			}
			e.Update()
		}
		if res == nil {
			t.Fatal("no screenshot result")
		}
		if res.Err != nil {
			t.Fatal(res.Err)
		}
		img, err := png.Decode(bytes.NewReader(res.PNG))
		if err != nil {
			t.Fatal(err)
		}
		if b := img.Bounds(); b.Dx() != res.Width || b.Dy() != res.Height {
			t.Fatalf("PNG is %v, result says %dx%d", b, res.Width, res.Height)
		}
		return *res, img
	}
}

func rgba(img image.Image, x, y int) color.RGBA {
	return color.RGBAModel.Convert(img.At(x, y)).(color.RGBA)
}

// eachPixel fails unless every pixel of img is want.
func eachPixel(t *testing.T, img image.Image, want color.RGBA) {
	t.Helper()
	b := img.Bounds()
	for y := b.Min.Y; y < b.Max.Y; y++ {
		for x := b.Min.X; x < b.Max.X; x++ {
			if got := rgba(img, x, y); got != want {
				t.Fatalf("pixel (%d,%d) = %v, want %v", x, y, got, want)
			}
		}
	}
}

// TestShotGamePixels: the logical frame with its post chain (a Fade) at
// scale 1 and 3, without the stats overlay that the screen shows.
func TestShotGamePixels(t *testing.T) {
	dir := t.TempDir()
	e := New(Config{Width: 64, Height: 32, NoAudio: true, StatsOverlay: true,
		Background: color.RGBA{R: 255, A: 255}, Screenshots: ScreenshotConfig{Dir: dir}})
	defer e.Close()
	e.Update()
	e.Post().Fade = gfx.RGBA(0, 0, 255, 255)
	blue := color.RGBA{B: 255, A: 255}

	one := shoot(t, e, ShotRequest{})
	three := shoot(t, e, ShotRequest{Scale: 3})
	screen := ebiten.NewImage(64, 32)
	e.Draw(screen)
	overlay := false
	for y := range 32 {
		for x := range 64 {
			overlay = overlay || rgba(screen, x, y) != blue
		}
	}
	if !overlay {
		t.Fatal("the stats overlay drew nothing on the screen")
	}

	res, img := one()
	if res.Width != 64 || res.Height != 32 || res.Tick != 1 {
		t.Errorf("result %dx%d tick %d", res.Width, res.Height, res.Tick)
	}
	eachPixel(t, img, blue)
	if filepath.Dir(res.Path) != dir {
		t.Errorf("Path = %q, want a file in %q", res.Path, dir)
	}
	if b, err := os.ReadFile(res.Path); err != nil || !bytes.Equal(b, res.PNG) {
		t.Errorf("file %q: %v", res.Path, err)
	}
	res, img = three()
	if res.Width != 192 || res.Height != 96 {
		t.Errorf("scale 3 result %dx%d", res.Width, res.Height)
	}
	eachPixel(t, img, blue)
}

// TestShotGameScaleNearest: scaling keeps hard pixel edges.
func TestShotGameScaleNearest(t *testing.T) {
	e := New(Config{Width: 4, Height: 2, NoAudio: true, Screenshots: ScreenshotConfig{Dir: t.TempDir()}})
	defer e.Close()
	e.Update()
	// A frame drawn by hand: one green pixel at (1,0).
	screen := ebiten.NewImage(4, 2)
	screen.Set(1, 0, color.RGBA{G: 255, A: 255})
	get := shoot(t, e, ShotRequest{Scale: 2})
	e.captureFrame(screen)
	_, img := get()
	if rgba(img, 2, 0) != (color.RGBA{G: 255, A: 255}) || rgba(img, 3, 1) != (color.RGBA{G: 255, A: 255}) ||
		rgba(img, 4, 0) != (color.RGBA{A: 255}) {
		t.Errorf("scaled pixels: %v %v %v", rgba(img, 2, 0), rgba(img, 3, 1), rgba(img, 4, 0))
	}
}

// fillTool draws white over the whole window in DrawFinal.
type fillTool struct{}

func (fillTool) Init(*Engine)                              {}
func (fillTool) Input(src input.Source) input.Source       { return src }
func (fillTool) Tick(*Engine) TickMode                     { return TickRun }
func (fillTool) DrawFinal(_ *Engine, s ebiten.FinalScreen) { s.Fill(color.White) }

// TestShotWindowPixels: the window capture has the final pass and the
// letterbox bars, not what the Tool draws.
func TestShotWindowPixels(t *testing.T) {
	files := fstest.MapFS{"scan.kage": &fstest.MapFile{Data: []byte(kageScanlines)}}
	e := New(Config{Width: 8, Height: 8, NoAudio: true, FS: files, Tool: fillTool{},
		Background: color.RGBA{R: 255, A: 255}, Screenshots: ScreenshotConfig{Dir: t.TempDir()}})
	defer e.Close()
	e.Update()
	scan, err := render.LoadShaderSync(e.Assets().Global(), "scan.kage")
	if err != nil {
		t.Fatal(err)
	}
	e.Post().Final = &render.Pass{Shader: scan}
	get := shoot(t, e, ShotRequest{Target: ShotWindow})
	game := shoot(t, e, ShotRequest{})

	frame := ebiten.NewImage(8, 8)
	e.Draw(frame)
	window := ebiten.NewImage(20, 16)
	var geoM ebiten.GeoM
	geoM.Scale(2, 2)
	geoM.Translate(2, 0) // 2 px bars left and right
	e.DrawFinalScreen(window, frame, geoM)
	if rgba(window, 10, 0) != (color.RGBA{255, 255, 255, 255}) {
		t.Fatal("the tool did not draw over the window")
	}

	res, img := get()
	if res.Width != 20 || res.Height != 16 {
		t.Fatalf("window capture %dx%d, want 20x16", res.Width, res.Height)
	}
	red, black := color.RGBA{R: 255, A: 255}, color.RGBA{A: 255}
	for _, tt := range []struct {
		x, y int
		want color.RGBA
	}{{10, 0, red}, {10, 1, black}, {10, 2, red}, {0, 0, black}, {1, 4, black}, {19, 0, black}, {2, 0, red}} {
		if got := rgba(img, tt.x, tt.y); got != tt.want {
			t.Errorf("window pixel (%d,%d) = %v, want %v", tt.x, tt.y, got, tt.want)
		}
	}
	_, img = game()
	eachPixel(t, img, red) // no final pass in the game target
}

// TestShotDrawNoAlloc: once captures were taken, a Draw without a request
// allocates no more than before.
func TestShotDrawNoAlloc(t *testing.T) {
	e := New(Config{Width: 16, Height: 16, NoAudio: true, Screenshots: ScreenshotConfig{Dir: t.TempDir()}})
	defer e.Close()
	e.Update()
	screen := ebiten.NewImage(16, 16)
	base := testing.AllocsPerRun(20, func() { e.Draw(screen) })
	get := shoot(t, e, ShotRequest{Scale: 2})
	e.Draw(screen)
	get()
	if n := testing.AllocsPerRun(20, func() { e.Draw(screen) }); n > base {
		t.Errorf("Draw allocates %v after a capture, %v before", n, base)
	}
}
