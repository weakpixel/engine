package viewer

import (
	"bytes"
	"image"
	"image/png"
	"testing"
	"testing/fstest"

	"github.com/weakpixel/engine/editor/toolshell"
	"github.com/weakpixel/engine/editor/viewkit"
	"github.com/weakpixel/engine/game"
	"github.com/weakpixel/engine/input"
)

const hash = `{
  "frames": {
    "b_second": {"frame": {"x":16,"y":0,"w":16,"h":16}},
    "a_first":  {"frame": {"x":0,"y":0,"w":12,"h":14}, "trimmed": true,
                 "spriteSourceSize": {"x":2,"y":1,"w":12,"h":14},
                 "sourceSize": {"w":16,"h":16},
                 "pivot": {"x":0.25,"y":1}}
  },
  "meta": {"image": "hero.png"}
}`

func pngBytes(t *testing.T, w, h int) []byte {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, w, h))); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// open runs the view tool on key in a 320x180 engine and updates it once.
func open(t *testing.T, files fstest.MapFS, key string) (*game.Engine, *Viewer, *input.ManualSource) {
	t.Helper()
	src := &input.ManualSource{}
	v := New().(*Viewer)
	sh := toolshell.New(toolshell.Config{Name: "view", Tool: v, Key: key, FS: files})
	e := game.New(game.Config{NoAudio: true, Width: 320, Height: 180, FS: files, Tool: sh, Input: game.InputConfig{Source: src}})
	t.Cleanup(e.Close)
	if err := e.Update(); err != nil {
		t.Fatal(err)
	}
	if err := v.Panel().Check(); err != nil {
		t.Fatalf("panel: %v", err)
	}
	return e, v, src
}

func TestAtlasFramesInFileOrder(t *testing.T) {
	_, v, _ := open(t, fstest.MapFS{"hero.atlas.json": {Data: []byte(hash)}, "hero.png": {Data: pngBytes(t, 32, 16)}}, "hero.atlas.json")
	a := v.inner.(*atlasView)
	if len(a.frames.List) != 2 {
		t.Fatalf("frames = %d, want 2", len(a.frames.List))
	}
	b, f := a.frames.List[0], a.frames.List[1]
	if b.Index != 0 || b.Name != "b_second" || viewkit.RectText(b.Rect) != "16, 0, 16 × 16" {
		t.Errorf("frame 0 = %+v", b)
	}
	// The second frame is trimmed: 12x14 pixels of a 16x16 frame, pivot (0.25, 1).
	if f.Name != "a_first" || viewkit.RectText(f.Rect) != "0, 0, 12 × 14" || viewkit.PointText(f.Size) != "16, 16" ||
		viewkit.PointText(f.Offset) != "2, 1" || viewkit.PointText(f.Pivot) != "4, 16" {
		t.Errorf("frame 1 = %+v", f)
	}
	if got := label(t, v, "atlas.count"); got != "2" {
		t.Errorf("count = %q", got)
	}
	if got := label(t, v, "image.size"); got != "32 × 16 px" {
		t.Errorf("image size = %q: the panel shows the texture the atlas names", got)
	}
}

// atlasOnly runs the atlas previewer on its own, for a grid sheet: the
// preview router sends a *.sheet.json to the sheet editor, but the atlas
// loader reads both formats.
type atlasOnly struct{ *atlasView }

func (a atlasOnly) Open(s *toolshell.Shell, key string) error {
	a.scope = s.Engine().Assets().NewScope("test")
	return a.atlasView.Open(s, key)
}

func TestAtlasGridSheetHoverAndSelect(t *testing.T) {
	files := fstest.MapFS{
		"tiles.sheet.json": {Data: []byte(`{"image":"tiles.png","frameW":16,"frameH":16}`)},
		"tiles.png":        {Data: pngBytes(t, 64, 32)},
	}
	src := &input.ManualSource{}
	v := atlasOnly{&atlasView{}}
	sh := toolshell.New(toolshell.Config{Name: "view", Tool: v, Key: "tiles.sheet.json", FS: files})
	e := game.New(game.Config{NoAudio: true, Width: 320, Height: 180, FS: files, Tool: sh, Input: game.InputConfig{Source: src}})
	t.Cleanup(e.Close)
	if err := e.Update(); err != nil {
		t.Fatal(err)
	}
	if got := label(t, v, "atlas.count"); got != "8" {
		t.Fatalf("count = %q, want 8 (4 x 2 cells)", got)
	}
	if got := label(t, v, "frame.which"); got != "click a frame to select it" {
		t.Errorf("before the mouse is over a frame: %q", got)
	}
	// The screen center is the texture center, (32, 16): row 1, column 2.
	src.Frame.Mouse.X, src.Frame.Mouse.Y = 160, 90
	e.Update()
	if got := label(t, v, "frame.which"); got != "6 (hover)" {
		t.Errorf("hover = %q, want frame 6", got)
	}
	if got := label(t, v, "frame.name"); got != "6" {
		t.Errorf("grid frames are named by index, got %q", got)
	}
	// A click selects; moving away keeps the selection; a click on nothing
	// clears it.
	src.Frame.Mouse.Buttons = 1
	e.Update()
	src.Frame.Mouse.Buttons = 0
	src.Frame.Mouse.X, src.Frame.Mouse.Y = 2, 2
	e.Update()
	if got := label(t, v, "frame.which"); got != "6 (selected)" {
		t.Errorf("after the click = %q", got)
	}
	src.Frame.Mouse.Buttons = 1
	e.Update()
	if got := label(t, v, "frame.which"); got != "click a frame to select it" {
		t.Errorf("click on nothing = %q, want the selection cleared", got)
	}
}

func TestAtlasLabelMode(t *testing.T) {
	_, v, _ := open(t, fstest.MapFS{"hero.atlas.json": {Data: []byte(hash)}, "hero.png": {Data: pngBytes(t, 32, 16)}}, "hero.atlas.json")
	if err := v.Set("atlas.labels", []byte("2")); err != nil || v.inner.(*atlasView).frames.Mode != viewkit.LabelName {
		t.Errorf("Set labels = %v", err)
	}
	if err := v.Set("atlas.labels", []byte("9")); err == nil {
		t.Error("a label mode out of range was accepted")
	}
}

func TestAtlasWithoutImageFails(t *testing.T) {
	src := &input.ManualSource{}
	v := New().(*Viewer)
	files := fstest.MapFS{"x.atlas.json": {Data: []byte(`{"frames": {"a": {"frame": {"x":0,"y":0,"w":8,"h":8}}}, "meta": {"image": "missing.png"}}`)}}
	sh := toolshell.New(toolshell.Config{Name: "view", Tool: v, Key: "x.atlas.json", FS: files})
	e := game.New(game.Config{NoAudio: true, Width: 320, Height: 180, FS: files, Tool: sh, Input: game.InputConfig{Source: src}})
	defer e.Close()
	if err := e.Update(); err != nil {
		t.Fatal(err) // the error is shown in the panel, it does not stop the tool
	}
	v.Update(sh) // and the tool keeps working without an atlas
	if err := v.Panel().Check(); err != nil {
		t.Errorf("panel after a failed open: %v", err)
	}
}
