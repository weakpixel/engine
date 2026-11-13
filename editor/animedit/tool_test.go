package animedit

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/weakpixel/engine/anim/animfile"
	"github.com/weakpixel/engine/editor/toolshell"
	"github.com/weakpixel/engine/game"
	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/input"
)

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

func newRig(t *testing.T, def string) *rig { return newRigKey(t, "chars/hero.anim.json", def) }

// newRigKey is newRig for the file at asset key, whose text is def.
func newRigKey(t *testing.T, key, def string) *rig {
	t.Helper()
	r := &rig{t: t, src: &input.ManualSource{}}
	tool := New().(*Tool)
	r.tool = tool
	r.sh = toolshell.New(toolshell.Config{
		Name: "anim", Tool: tool, Key: key, FS: heroFS(t),
		Transport: r, Document: toolshell.NewHostDocument(r),
	})
	r.e = game.New(game.Config{
		NoAudio: true, Width: 640, Height: 360, FS: heroFS(t),
		Tool: r.sh, Input: game.InputConfig{Source: r.src},
	})
	t.Cleanup(r.e.Close)
	r.update()
	r.document(def)
	return r
}

func (r *rig) update() {
	r.t.Helper()
	if err := r.e.Update(); err != nil {
		r.t.Fatal(err)
	}
}

func (r *rig) document(text string) {
	r.t.Helper()
	msg, _ := json.Marshal(map[string]any{"type": "document", "text": text})
	r.sh.Receive(msg)
	r.update()
}

func (r *rig) set(id string, v any) {
	r.t.Helper()
	msg, _ := json.Marshal(map[string]any{"type": "set", "id": id, "value": v})
	r.sh.Receive(msg)
	r.update()
}

// mouse puts the pointer on a world point (file space, not flipped) and
// presses or releases the left button.
func (r *rig) mouse(x, y float32, down bool) {
	r.t.Helper()
	w := r.sh.WindowRect(gfx.Rect{Min: gfx.Vec2{X: x, Y: y}, Max: gfx.Vec2{X: x, Y: y}})
	r.src.Frame.Mouse.X, r.src.Frame.Mouse.Y = w.Min.X, w.Min.Y
	r.src.Frame.Mouse.Buttons = 0
	if down {
		r.src.Frame.Mouse.Buttons = 1
	}
	r.update()
}

func TestToolOpensAndDrawsABoxWithOneEdit(t *testing.T) {
	r := newRig(t, heroDef)
	ready := r.of("ready")
	if len(ready) != 1 || ready[0]["tool"] != "anim" || ready[0]["document"] != true {
		t.Fatalf("ready = %v", ready)
	}
	panel := r.tool.Panel()
	if err := panel.Check(); err != nil {
		t.Fatal(err)
	}
	if f, _ := panel.Find("clip.sel"); len(f.Options) != 2 {
		t.Fatalf("clip options = %v", f.Options)
	}
	if got := len(r.of("edit")); got != 0 {
		t.Fatalf("opening sent %d edits", got)
	}

	r.set("clip.sel", 1)
	r.set("frame", 2)
	r.set("tool", int(ToolHurt))
	r.mouse(-3, -6, true)
	r.mouse(-2, -5, true)
	if got := len(r.of("edit")); got != 0 {
		t.Fatalf("a drag in progress sent %d edits", got)
	}
	r.mouse(-1, -4, false)

	edits := r.of("edit")
	if len(edits) != 1 {
		t.Fatalf("drawing sent %d edits, want 1", len(edits))
	}
	f, err := animfile.Decode([]byte(edits[0]["text"].(string)))
	if err != nil {
		t.Fatal(err)
	}
	shapes := clipEntry(f, "run").Shapes
	got := shapes[len(shapes)-1]
	if got.Kind != KindHurt || got.Frames == nil || got.Frames.From != 2 || got.Rect != (animfile.Rect{X: -3, Y: -6, W: 2, H: 2}) {
		t.Errorf("new shape = %+v", got)
	}
	if _, findings := animfile.Diagnose(f, heroSheet(t)); hasError(findings) {
		t.Errorf("the written file has errors: %v", findings)
	}
	// The echo of our own edit is not loaded again: nothing else is sent.
	r.document(edits[0]["text"].(string))
	if len(r.of("edit")) != 1 {
		t.Error("the echo caused another edit")
	}
}

func TestToolInvalidDocumentIsKeptAndNeverWritten(t *testing.T) {
	r := newRig(t, heroDef)
	r.document(`{"version": 1, "sheet": `)
	if p := r.tool.Panel(); !strings.Contains(p.Status, "document has errors") {
		t.Errorf("status = %q", p.Status)
	}
	if f, ok := r.tool.Panel().Find("clip.sel"); !ok || len(f.Options) != 2 {
		t.Error("the last good version is not shown")
	}
	r.set("tool", int(ToolHit))
	r.mouse(-3, -6, true)
	r.mouse(-1, -4, false)
	r.set("clip.fps", 30)
	if got := len(r.of("edit")); got != 0 {
		t.Errorf("a broken document received %d edits", got)
	}
	r.document(heroDef)
	if p := r.tool.Panel(); strings.Contains(p.Status, "document has errors") {
		t.Errorf("a valid document left the mark: %q", p.Status)
	}
	r.set("clip.fps", 30)
	if got := len(r.of("edit")); got != 1 {
		t.Errorf("after the fix: %d edits, want 1", got)
	}
}

func TestToolNoSheet(t *testing.T) {
	r := newRig(t, strings.Replace(heroDef, `"hero.aseprite.json"`, `"missing.json"`, 1))
	p := r.tool.Panel()
	if err := p.Check(); err != nil {
		t.Fatal(err)
	}
	if _, ok := p.Find("problem.0"); !ok {
		t.Error("no problem shown for the missing sheet")
	}
	if _, ok := p.Find("clip.sel"); ok {
		t.Error("clip controls shown without a sheet")
	}
}

func TestToolCheck(t *testing.T) {
	tool := &Tool{}
	if err := tool.Check("a/b.anim.json"); err != nil {
		t.Error(err)
	}
	if err := tool.Check("a/b.json"); err == nil || !strings.Contains(err.Error(), ".anim.json") {
		t.Errorf("Check(b.json) = %v", err)
	}
}

// TestPlayKey: Enter toggles playback; Space is the pan key and does not.
func TestPlayKey(t *testing.T) {
	r := newRig(t, heroDef)
	for _, tc := range []struct {
		key     input.Key
		playing bool
	}{{input.KeySpace, r.tool.st.Play.Playing}, {input.KeyEnter, !r.tool.st.Play.Playing}} {
		r.src.Frame.Keys.Set(tc.key)
		r.update()
		r.src.Frame.Keys.Clear(tc.key)
		r.update()
		if got := r.tool.st.Play.Playing; got != tc.playing {
			t.Fatalf("after key %v: playing = %v, want %v", tc.key, got, tc.playing)
		}
	}
}
