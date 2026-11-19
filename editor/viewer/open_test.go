package viewer

import (
	"encoding/json"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/weakpixel/ecs"
	"github.com/weakpixel/engine/editor/toolshell"
	"github.com/weakpixel/engine/game"
	"github.com/weakpixel/engine/gfx"
)

// tilesAtlas is a two-frame atlas of tiles.png (32x16).
const tilesAtlas = `{"frames": {"a": {"frame": {"x":0,"y":0,"w":16,"h":16}}, "b": {"frame": {"x":16,"y":0,"w":16,"h":16}}}, "meta": {"image": "tiles.png"}}`

// recorder keeps the messages the shell sends to the host.
type recorder struct{ msgs []map[string]any }

func (r *recorder) Send(msg []byte) {
	var m map[string]any
	if err := json.Unmarshal(msg, &m); err != nil {
		panic(err)
	}
	r.msgs = append(r.msgs, m)
}

// The host's open message switches the viewer to another file in the same
// game: one sprite entity at a time, the old texture unloaded, ready sent
// for the new file, and a file that fails leaves an error, not the old file.
func TestViewerOpenSwitchesFile(t *testing.T) {
	files := fstest.MapFS{
		"box.png":          {Data: pngBytes(t, 64, 32)},
		"tiles.png":        {Data: pngBytes(t, 32, 16)},
		"tiles.atlas.json": {Data: []byte(tilesAtlas)},
	}
	tr := &recorder{}
	v := New().(*Viewer)
	sh := toolshell.New(toolshell.Config{Name: "view", Tool: v, Key: "box.png", FS: files, Transport: tr})
	e := game.New(game.Config{NoAudio: true, Width: 320, Height: 180, FS: files, Tool: sh})
	defer e.Close()
	update := func() {
		t.Helper()
		if err := e.Update(); err != nil {
			t.Fatal(err)
		}
	}
	sprites := func() int {
		n := 0
		for range ecs.NewView1[gfx.Sprite](e.World()).Each() {
			n++
		}
		return n
	}
	// open sends an open message and returns the ready it was answered with
	// and the errors before it.
	open := func(key string) (ready map[string]any, errs []string) {
		t.Helper()
		tr.msgs = nil
		sh.Receive([]byte(`{"type":"open","path":"` + key + `"}`))
		update()
		for _, m := range tr.msgs {
			switch m["type"] {
			case "ready":
				ready = m
			case "error":
				errs = append(errs, m["message"].(string))
			}
		}
		if ready == nil {
			t.Fatalf("open %s: messages %v, want a ready", key, tr.msgs)
		}
		return ready, errs
	}

	update()
	box := v.inner.(*textureView).tex.Sprite
	e.Camera().Zoom = 3 // the user zoomed; the next file is framed anyway

	ready, errs := open("tiles.atlas.json")
	if ready["path"] != "tiles.atlas.json" || ready["tool"] != "view" || len(errs) != 0 {
		t.Errorf("ready = %v, errors %v", ready, errs)
	}
	if _, ok := v.inner.(*atlasView); !ok {
		t.Fatalf("previewer = %T, want the atlas one", v.inner)
	}
	if n := sprites(); n != 1 {
		t.Errorf("sprite entities = %d, want 1", n)
	}
	if cam := e.Camera(); cam.Pos.X != 16 || cam.Pos.Y != 8 {
		t.Errorf("camera = %+v, want the atlas framed", *cam)
	}
	update() // the asset update unloads what only the old file held
	if e.Stores().Sprites().Ready(box) {
		t.Error("box.png is still loaded after switching")
	}

	ready, errs = open("missing.png")
	if ready["path"] != "missing.png" || len(errs) == 0 {
		t.Errorf("open of a missing file: ready %v, errors %v", ready, errs)
	}
	if n := sprites(); n != 0 {
		t.Errorf("sprite entities after a failed open = %d, want 0", n)
	}
	if _, errs = open("notes.txt"); len(errs) == 0 || !strings.Contains(errs[0], "not a file wpix-editor opens") {
		t.Errorf("open of an unsupported file: errors %v", errs)
	}

	ready, errs = open("box.png")
	if len(errs) != 0 || label(t, v, "image.size") != "64 × 32 px" {
		t.Errorf("reopen box.png: errors %v, size %q", errs, label(t, v, "image.size"))
	}
	if p, _ := ready["panel"].(map[string]any); p["title"] != "box.png" || p["status"] != nil {
		t.Errorf("ready panel = %v", ready["panel"])
	}
}
