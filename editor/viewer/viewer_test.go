package viewer

import (
	"bytes"
	"image"
	"image/png"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/weakpixel/ecs"
	"github.com/weakpixel/engine/editor/toolshell"
	"github.com/weakpixel/engine/game"
	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/input"
	"github.com/weakpixel/engine/render"
)

func TestViewerOpensFramesAndReadsPixel(t *testing.T) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 64, 32))); err != nil {
		t.Fatal(err)
	}
	src := &input.ManualSource{}
	v := New()
	sh := toolshell.New(toolshell.Config{Name: "view", Tool: v, Key: "art/box.png"})
	e := game.New(game.Config{
		NoAudio: true, Width: 320, Height: 180, FS: fstest.MapFS{"art/box.png": {Data: buf.Bytes()}},
		Tool: sh, Input: game.InputConfig{Source: src},
	})
	defer e.Close()

	if err := e.Update(); err != nil {
		t.Fatal(err)
	}
	// 64x32 fits 320x180 at 5 (width), with the 0.9 margin: 4.5, centered.
	if cam := e.Camera(); cam.Pos.X != 32 || cam.Pos.Y != 16 || cam.Zoom != 4.5 {
		t.Errorf("camera = %+v, want centered on the texture at zoom 4.5", *cam)
	}
	if got := label(t, v, "image.size"); got != "64 × 32 px" {
		t.Errorf("size = %q", got)
	}

	// The screen center is the texture center: pixel (32, 16), cell (2, 1).
	src.Frame.Mouse.X, src.Frame.Mouse.Y = 160, 90
	if err := e.Update(); err != nil {
		t.Fatal(err)
	}
	if got := label(t, v, "image.pixel"); got != "32, 16" {
		t.Errorf("pixel = %q, want 32, 16", got)
	}
	if got := label(t, v, "image.cell"); got != "2, 1" {
		t.Errorf("cell = %q, want 2, 1", got)
	}
	src.Frame.Mouse.X, src.Frame.Mouse.Y = 2, 2
	if err := e.Update(); err != nil {
		t.Fatal(err)
	}
	if got := label(t, v, "image.pixel"); got != "outside" {
		t.Errorf("pixel far outside = %q, want outside", got)
	}
}

// The engine's screen size is the configured one until Ebiten's first
// Layout; the content must be framed again when the real size arrives, until
// the user moves the camera.
func TestViewerRefitsOnResize(t *testing.T) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 64, 32))); err != nil {
		t.Fatal(err)
	}
	src := &input.ManualSource{}
	sh := toolshell.New(toolshell.Config{Name: "view", Tool: New(), Key: "box.png"})
	e := game.New(game.Config{
		NoAudio: true, Width: 16384, Height: 16384, IntegerScale: true,
		FS: fstest.MapFS{"box.png": {Data: buf.Bytes()}}, Tool: sh, Input: game.InputConfig{Source: src},
	})
	defer e.Close()
	e.Layout(0, 0) // a webview reports an empty window before its first layout
	e.Update()
	if z := e.Camera().Zoom; z != 0 && z != 1 {
		t.Fatalf("zoom = %v with no screen, want it left alone", z)
	}
	e.Layout(640, 360) // the window's real size, as Ebiten reports it
	e.Update()
	if z := e.Camera().Zoom; z != 9 {
		t.Fatalf("zoom = %v, want 9 (640 / 64 * 0.9)", z)
	}
	e.Layout(320, 180)
	e.Update()
	if z := e.Camera().Zoom; z != 4.5 {
		t.Errorf("zoom after a resize = %v, want 4.5", z)
	}
	src.Frame.Mouse.WheelY = 1
	e.Update()
	src.Frame.Mouse.WheelY = 0
	for range 60 { // the wheel zoom eases; let it settle
		e.Update()
	}
	zoomed := e.Camera().Zoom
	e.Layout(640, 360)
	e.Update()
	if z := e.Camera().Zoom; z != zoomed {
		t.Errorf("zoom after the user zoomed and the window resized = %v, want it kept (%v)", z, zoomed)
	}
}

func TestViewerCheck(t *testing.T) {
	v := New().(*Viewer)
	for _, key := range []string{"a.png", "dir/B.JPG", "c.jpeg"} {
		if err := v.Check(key); err != nil {
			t.Errorf("Check(%q) = %v", key, err)
		}
	}
	for _, key := range []string{"dir/B.ATLAS.JSON", "fonts/ui.ttf", "x.OTF", "hero.aseprite.json", "kip.voice.json", "intro.dialogue.json"} {
		if err := v.Check(key); err != nil {
			t.Errorf("Check(%q) = %v", key, err)
		}
	}
	err := v.Check("levels/a.level.json")
	if err == nil || !strings.Contains(err.Error(), "view: a.level.json is not a file wpix-editor opens") || !strings.Contains(err.Error(), ".atlas.json") {
		t.Errorf("Check(json) = %v", err)
	}
	// An edited kind opens in its editor, never in the preview.
	for _, key := range []string{"tiles.sheet.json", "hero.anim.json", "spark.fx.json"} {
		if err := v.Check(key); err == nil || !strings.Contains(err.Error(), "editor, not in the preview") {
			t.Errorf("Check(%q) = %v, want it refused", key, err)
		}
	}
}

func label(t *testing.T, v toolshell.Tool, id string) string {
	t.Helper()
	f, ok := v.Panel().Find(id)
	if !ok {
		t.Fatalf("no field %q", id)
	}
	return strings.Trim(string(f.Value), `"`)
}

// The engine culls a sprite by its position alone: with the texture's
// top-left corner off screen the texture must still be queued.
func TestViewerStaysVisibleWhenPartlyOffscreen(t *testing.T) {
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 64, 32))); err != nil {
		t.Fatal(err)
	}
	v := New().(*Viewer)
	sh := toolshell.New(toolshell.Config{Name: "view", Tool: v, Key: "box.png"})
	e := game.New(game.Config{
		NoAudio: true, Width: 320, Height: 180,
		FS: fstest.MapFS{"box.png": {Data: buf.Bytes()}}, Tool: sh,
	})
	defer e.Close()
	e.Update()
	// Pan far right and zoom in: the texture's top-left corner is far off screen.
	e.Camera().Pos = gfx.Vec2{X: 60, Y: 16}
	e.Camera().Zoom = 20
	e.Update()
	q := ecs.GetResource[render.Queue](e.World())
	if q.Culled() != 0 {
		t.Errorf("culled = %d, want the texture queued", q.Culled())
	}
	tv := v.inner.(*textureView)
	tr := ecs.GetCompOrNil[gfx.Transform](e.World(), tv.tex.Entity())
	sp := ecs.GetCompOrNil[gfx.Sprite](e.World(), tv.tex.Entity())
	// Whatever the pivot, the texture's corner stays at the origin.
	if x, y := tr.Pos.X-sp.Origin.X*64, tr.Pos.Y-sp.Origin.Y*32; x < -0.01 || x > 0.01 || y < -0.01 || y > 0.01 {
		t.Errorf("texture corner at (%v, %v), want (0, 0)", x, y)
	}
}
