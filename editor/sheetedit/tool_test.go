package sheetedit

import (
	"bytes"
	"encoding/json"
	"image"
	"image/color"
	"image/png"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/weakpixel/engine/editor/toolshell"
	"github.com/weakpixel/engine/editor/viewkit"
	"github.com/weakpixel/engine/game"
	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/input"
	"github.com/weakpixel/engine/sheetfile"
)

const sheetText = `{"image":"hero.png","frameW":16,"frameH":16}`

func pngBytes(t testing.TB, w, h int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := range h {
		for x := range w {
			img.SetRGBA(x, y, color.RGBA{R: 255, A: 255})
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

// rig runs the tool on a real engine and a host document, the way the
// VSCode build does, with a fake transport.
type rig struct {
	t    *testing.T
	e    *game.Engine
	sh   *toolshell.Shell
	tool *Tool
	src  *input.ManualSource
	sent [][]byte
}

func (r *rig) Send(msg []byte) { r.sent = append(r.sent, append([]byte(nil), msg...)) }

func (r *rig) of(typ string) []map[string]any {
	var out []map[string]any
	for _, m := range r.sent {
		var v map[string]any
		if err := json.Unmarshal(m, &v); err != nil {
			r.t.Fatal(err)
		}
		if v["type"] == typ {
			out = append(out, v)
		}
	}
	return out
}

func newRig(t *testing.T, text string, files fstest.MapFS) *rig {
	t.Helper()
	r := &rig{t: t, src: &input.ManualSource{}}
	r.tool = New().(*Tool)
	r.sh = toolshell.New(toolshell.Config{
		Name: "sheet", Tool: r.tool, Key: "gfx/hero.sheet.json", FS: files,
		Transport: r, Document: toolshell.NewHostDocument(r),
	})
	r.e = game.New(game.Config{
		NoAudio: true, Width: 640, Height: 360, FS: files,
		Tool: r.sh, Input: game.InputConfig{Source: r.src},
	})
	t.Cleanup(r.e.Close)
	r.update()
	r.send(map[string]any{"type": "document", "text": text})
	return r
}

func (r *rig) update() {
	r.t.Helper()
	if err := r.e.Update(); err != nil {
		r.t.Fatal(err)
	}
}

func (r *rig) send(m map[string]any) {
	r.t.Helper()
	msg, _ := json.Marshal(m)
	r.sh.Receive(msg)
	r.update()
}

func (r *rig) set(id string, v any) { r.send(map[string]any{"type": "set", "id": id, "value": v}) }

// hover puts the pointer on an image pixel.
func (r *rig) hover(x, y float32) {
	w := r.sh.WindowRect(gfx.Rect{Min: gfx.Vec2{X: x, Y: y}, Max: gfx.Vec2{X: x, Y: y}})
	r.src.Frame.Mouse.X, r.src.Frame.Mouse.Y = w.Min.X, w.Min.Y
	r.update()
}

func (r *rig) field(id string) toolshell.Field {
	r.t.Helper()
	f, ok := r.tool.Panel().Find(id)
	if !ok {
		r.t.Fatalf("no field %q", id)
	}
	return f
}

func heroFS(t testing.TB) fstest.MapFS {
	return fstest.MapFS{
		"gfx/hero.sheet.json": {Data: []byte(sheetText)},
		"gfx/hero.png":        {Data: pngBytes(t, 64, 32)},
	}
}

func TestToolShowsTheGridAndWritesOneEditPerChange(t *testing.T) {
	r := newRig(t, sheetText, heroFS(t))
	ready := r.of("ready")
	if len(ready) != 1 || ready[0]["tool"] != "sheet" || ready[0]["document"] != true {
		t.Fatalf("ready = %v", ready)
	}
	if err := r.tool.Panel().Check(); err != nil {
		t.Fatal(err)
	}
	var left, right []string
	for _, sec := range r.tool.Panel().Sections {
		if sec.Side == toolshell.SideLeft {
			left = append(left, sec.ID)
		} else {
			right = append(right, sec.ID)
		}
	}
	if !slices.Equal(left, []string{"frames", "info", "view"}) || !slices.Equal(right, []string{"sheet"}) {
		t.Errorf("sections: left %v right %v, want left [frames info view] right [sheet]", left, right)
	}
	if got := r.field("info.count").Value; string(got) != `"8"` {
		t.Fatalf("frames = %s", got)
	}
	if len(r.of("edit")) != 0 {
		t.Fatal("opening sent an edit")
	}

	r.set(IDFrameW, 8)
	edits := r.of("edit")
	if len(edits) != 1 {
		t.Fatalf("%d edits, want 1", len(edits))
	}
	f, err := sheetfile.Decode([]byte(edits[0]["text"].(string)))
	if err != nil || f.FrameW != 8 || f.Image != "hero.png" {
		t.Fatalf("edit = %q, %v", edits[0]["text"], err)
	}
	if got := r.field("info.count").Value; string(got) != `"16"` {
		t.Errorf("frames after the edit = %s", got)
	}
	// The host echoes our own edit back: nothing else is sent.
	r.send(map[string]any{"type": "document", "text": edits[0]["text"]})
	if len(r.of("edit")) != 1 {
		t.Error("the echo of an edit sent another one")
	}
}

func TestToolHoverShowsTheFrameIndex(t *testing.T) {
	r := newRig(t, sheetText, heroFS(t))
	r.hover(20, 20) // second column, second row of a 4 x 2 grid
	if got := r.field("info.hover").Value; !strings.HasPrefix(string(got), `"5 (hover)`) {
		t.Errorf("hover = %s", got)
	}
	r.hover(200, 20)
	if got := r.field("info.hover").Value; string(got) != `"click a frame to select it"` {
		t.Errorf("hover off the image = %s", got)
	}
}

// TestToolFrameListAndSelection: the Frames section lists the grid's frames
// by index; a click selects like the list does, a click on nothing clears
// it, the list's 0 clears it, and an edit that drops the frame drops the
// selection. None of it edits the document.
func TestToolFrameListAndSelection(t *testing.T) {
	r := newRig(t, sheetText, heroFS(t))
	list := r.field(IDFrame)
	if len(list.Options) != 9 || list.Options[0] != "(none)" || list.Options[6] != "5" {
		t.Fatalf("frame list = %v, want (none) and 8 frames", list.Options)
	}
	r.hover(20, 20)
	r.src.Frame.Mouse.Buttons = 1
	r.update()
	r.src.Frame.Mouse.Buttons = 0
	r.hover(200, 20) // off the image: the selection stays
	if got := r.field("info.hover").Value; !strings.HasPrefix(string(got), `"5 (selected)`) {
		t.Errorf("after the click = %s", got)
	}
	if got := r.field(IDFrame).Value; string(got) != "6" {
		t.Errorf("list value = %s, want 6 (frame 5 after (none))", got)
	}
	r.src.Frame.Mouse.Buttons = 1
	r.update()
	r.src.Frame.Mouse.Buttons = 0
	r.update()
	if r.st().Frames.Sel != -1 {
		t.Errorf("a click on nothing kept frame %d", r.st().Frames.Sel)
	}
	r.set(IDFrame, 8)
	if r.st().Frames.Sel != 7 {
		t.Errorf("list 8 selected %d, want 7", r.st().Frames.Sel)
	}
	r.set(IDLabels, 3)
	if r.st().Frames.Mode != viewkit.LabelNone {
		t.Errorf("label mode = %d", r.st().Frames.Mode)
	}
	if len(r.of("edit")) != 0 {
		t.Error("selecting or the label mode edited the document")
	}
	r.set(IDFrameW, 32) // 2 x 2 frames: frame 7 is gone
	if r.st().Frames.Sel != -1 || len(r.st().Frames.List) != 4 {
		t.Errorf("after the edit: sel %d, %d frames", r.st().Frames.Sel, len(r.st().Frames.List))
	}
	r.set(IDFrame, 0)
	if r.st().Frames.Sel != -1 {
		t.Error("0 did not clear the selection")
	}
	r.set(IDFrame, 9)
	if len(r.of("error")) == 0 {
		t.Error("a frame out of range was accepted")
	}
}

// TestToolTooManyFramesToList: above MaxListed the list is empty and a note
// says why; the count and the hover still work.
func TestToolTooManyFramesToList(t *testing.T) {
	r := newRig(t, `{"image":"hero.png","frameW":1,"frameH":1}`, heroFS(t)) // 64 x 32 = 2048
	if n := len(r.st().Frames.List); n != 2048 {
		t.Fatalf("%d frames listed", n)
	}
	files := heroFS(t) // a 128 x 64 image: 8192 frames
	files["gfx/hero.png"].Data = pngBytes(t, 128, 64)
	r2 := newRig(t, `{"image":"hero.png","frameW":1,"frameH":1}`, files)
	if n := len(r2.st().Frames.List); n != 0 {
		t.Errorf("%d frames listed above MaxListed", n)
	}
	if _, ok := r2.tool.Panel().Find("sheet.unlisted"); !ok {
		t.Error("no note for an unlisted grid")
	}
	r2.hover(3, 2)
	if got := r2.field("info.hover").Value; !strings.HasPrefix(string(got), `"259 (hover)`) {
		t.Errorf("hover = %s", got)
	}
}

func TestToolKeepsABrokenDocument(t *testing.T) {
	r := newRig(t, sheetText, heroFS(t))
	r.send(map[string]any{"type": "document", "text": `{"image":`})
	if r.st().Stale == "" {
		t.Fatal("not marked stale")
	}
	r.set(IDFrameW, 8)
	if len(r.of("edit")) != 0 || len(r.of("error")) == 0 {
		t.Errorf("edits %d, errors %d", len(r.of("edit")), len(r.of("error")))
	}
	if r.st().File == nil || r.st().File.FrameW != 16 {
		t.Error("the last good sheet is gone")
	}
}

func (r *rig) st() *State { return r.tool.st }

func TestToolMissingImageIsAFinding(t *testing.T) {
	files := fstest.MapFS{"gfx/hero.sheet.json": {Data: []byte(sheetText)}}
	r := newRig(t, sheetText, files)
	if _, ok := r.tool.Panel().Find("problem.0"); !ok {
		t.Errorf("no finding for a missing image: %v", r.st().ImageErr)
	}
}

func TestNewSheet(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "gfx"), 0o755); err != nil {
		t.Fatal(err)
	}
	img := filepath.Join(dir, "gfx", "hero.png")
	if err := os.WriteFile(img, pngBytes(t, 40, 8), 0o644); err != nil {
		t.Fatal(err)
	}
	text, err := NewSheet(img, filepath.Join(dir, "gfx", "hero.sheet.json"))
	if err != nil {
		t.Fatal(err)
	}
	f, err := sheetfile.Decode(text)
	if err != nil || f.Image != "hero.png" || f.FrameW != 16 || f.FrameH != 8 {
		t.Errorf("starter %q: %v", text, err)
	}
	if _, err := NewSheet(filepath.Join(dir, "none.png"), filepath.Join(dir, "none.sheet.json")); err == nil {
		t.Error("a missing image gave a starter")
	}
}
