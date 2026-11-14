package mcpserver

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/weakpixel/engine/tools/internal/mcp"
)

// attachClient makes cl the running game of tl, on the document key, as
// game_start would. It starts the window-edit loop.
func attachClient(t *testing.T, tl *tools, cl *client, key string, headless bool) *gameSession {
	t.Helper()
	abs, _, err := tl.p.AssetPath(key)
	if err != nil {
		t.Fatal(err)
	}
	text, err := os.ReadFile(abs)
	if err != nil {
		t.Fatal(err)
	}
	cl.mu.Lock()
	cl.ready, cl.path, cl.docText = true, key, text
	cl.mu.Unlock()
	s := &gameSession{cl: cl, pid: 4242, stage: stageOf(key), headless: headless, started: time.Now(), exited: make(chan struct{})}
	ctx, cancel := context.WithCancel(context.Background())
	go tl.viewportLoop(ctx, s)
	tl.game.mu.Lock()
	tl.game.cur = s
	tl.game.mu.Unlock()
	t.Cleanup(func() {
		cancel()
		tl.game.mu.Lock()
		tl.game.cur = nil
		tl.game.mu.Unlock()
	})
	return s
}

// fakeProject is a copied test project with a scripted game on level A.
func fakeProject(t *testing.T) (dir string, s *mcp.Server, tl *tools, f *fakeGame) {
	t.Helper()
	dir = copyFixture(t)
	s, _, tl, err := build(Options{ProjectOptions: ProjectOptions{Dir: dir}})
	if err != nil {
		t.Fatal(err)
	}
	f = &fakeGame{t: t}
	f.cl = newClient(f, tl.game.logs)
	attachClient(t, tl, f.cl, levelA, false)
	return dir, s, tl, f
}

// documents returns the texts of the "document" messages sent so far.
func (f *fakeGame) documents() []string {
	var out []string
	for _, m := range f.sent {
		if m["type"] == "document" {
			out = append(out, m["text"].(string))
		}
	}
	return out
}

// TestDocWriteGame: an edit reaches the game as a document; one the game
// refuses is rolled back and the game's error comes back.
func TestDocWriteGame(t *testing.T) {
	dir, s, _, f := fakeProject(t)
	before := readAsset(t, dir, levelA)

	mustCall(t, s, "place_object", `{"path":"`+levelA+`","class":"demo:Crate","x":1,"y":2,"id":"fine"}`)
	after := readAsset(t, dir, levelA)
	if docs := f.documents(); len(docs) != 1 || docs[0] != after {
		t.Fatalf("the game got %d documents, the last equal to the file: %v", len(docs), len(docs) == 1 && docs[0] == after)
	}

	f.reply = func(m map[string]any) []any {
		if m["type"] == "document" && strings.Contains(m["text"].(string), "bad-one") {
			return []any{map[string]any{"type": "applied", "error": `object "bad-one": component demo:Health: HP must not be negative`}}
		}
		return nil
	}
	// The scripted game answers both: the stock "applied" first, then the error.
	f.cl.mu.Lock()
	f.cl.mu.Unlock()
	msg := mustFail(t, s, "place_object", `{"path":"`+levelA+`","class":"demo:Crate","x":1,"y":2,"id":"bad-one"}`)
	mustContain(t, msg, `object "bad-one"`)
	mustContain(t, msg, "HP must not be negative")
	if got := readAsset(t, dir, levelA); got != after {
		t.Errorf("the refused edit was not rolled back:\n%s", got)
	}
	docs := f.documents()
	if last := docs[len(docs)-1]; last != after {
		t.Error("the game was not given the old document back")
	}
	if before == after {
		t.Error("the first edit changed nothing")
	}
}

// TestDocWriteOtherFile: a file that is not the open document is only
// written; the watcher tells the game.
func TestDocWriteOtherFile(t *testing.T) {
	_, s, _, f := fakeProject(t)
	mustCall(t, s, "set_field", `{"path":"global/prefabs/crate.prefab.json","id":"crate","component":"demo:Health","field":["HP"],"value":9}`)
	if docs := f.documents(); len(docs) != 0 {
		t.Errorf("the game got a document for another file: %v", docs)
	}
}

// TestViewportEvents: edits made in the game window are written to the file
// with the same splices as the tools, and the game gets the document.
func TestViewportEvents(t *testing.T) {
	dir, _, _, f := fakeProject(t)
	waitFor := func(what string, cond func() bool) {
		t.Helper()
		for i := 0; i < 500; i++ {
			if cond() {
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
		logs, _, _ := f.cl.logs.read(0, 50)
		t.Fatalf("timed out waiting for %s; log: %+v", what, logs)
	}
	flat := func() string { return strings.Join(strings.Fields(readAsset(t, dir, levelA)), "") }
	f.cl.handle([]byte(`{"type":"edit","id":"left","component":"gfx:Transform","json":{"Pos":{"X":99,"Y":20}},"override":true}`))
	waitFor("the edit", func() bool { return strings.Contains(flat(), `"X":99`) })

	f.cl.handle([]byte(`{"type":"create","object":{"id":"dropped","group":"demo","class":"Crate","components":{"gfx:Transform":{"Pos":{"X":3,"Y":4},"PrevPos":{"X":3,"Y":4},"Rot":0,"Scale":{"X":1,"Y":1}},"demo:Health":{"HP":1,"Kind":0}}}}`))
	waitFor("the object", func() bool { return strings.Contains(readAsset(t, dir, levelA), `"dropped"`) })

	// An override reverted in the window ({}) is removed from the file.
	f.cl.handle([]byte(`{"type":"edit","id":"left","component":"gfx:Transform","json":{},"override":true}`))
	waitFor("the revert", func() bool { return !strings.Contains(flat(), `"X":99`) })

	final := readAsset(t, dir, levelA)
	waitFor("the documents", func() bool {
		f.cl.opMu.Lock()
		defer f.cl.opMu.Unlock()
		docs := f.documents()
		return len(docs) >= 3 && docs[len(docs)-1] == final
	})
	if !strings.Contains(final, `"Level A"`) || !strings.Contains(final, `"bare"`) {
		t.Errorf("the rest of the file changed:\n%s", final)
	}
}

// TestWatchAssets: a changed asset goes to the game as assetsChanged; the
// open document, changed by someone else, goes as a document.
func TestWatchAssets(t *testing.T) {
	dir, _, tl, f := fakeProject(t)
	s := tl.game.current()
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	wg.Add(1)
	go func() { defer wg.Done(); tl.watchAssets(ctx, s, 10*time.Millisecond) }()
	defer func() { cancel(); wg.Wait() }()
	time.Sleep(50 * time.Millisecond)

	png := filepath.Join(dir, "assets", "global", "dot.png")
	data, _ := os.ReadFile(png)
	os.WriteFile(png, append(data, 0), 0o644)
	other := []byte(`{"name":"Edited elsewhere","objects":[]}`)
	os.WriteFile(filepath.Join(dir, "assets", filepath.FromSlash(levelA)), other, 0o644)

	var gotPNG, gotDoc bool
	for i := 0; i < 500 && !(gotPNG && gotDoc); i++ {
		time.Sleep(10 * time.Millisecond)
		f.cl.opMu.Lock()
		for _, m := range f.sent {
			if m["type"] == "assetsChanged" {
				if ps, _ := m["paths"].([]any); len(ps) == 1 && ps[0] == "global/dot.png" {
					gotPNG = true
				}
				for _, p := range m["paths"].([]any) {
					if p == levelA {
						t.Errorf("the open document was sent as an asset change: %v", m)
					}
				}
			}
		}
		for _, d := range f.documents() {
			if d == string(other) {
				gotDoc = true
			}
		}
		f.cl.opMu.Unlock()
	}
	if !gotPNG || !gotDoc {
		t.Errorf("assetsChanged sent: %v, changed document sent: %v", gotPNG, gotDoc)
	}
}

func TestChangedKeys(t *testing.T) {
	now := time.Now()
	prev := map[string]stamp{"a": {now, 1}, "b": {now, 1}, "c": {now, 1}}
	next := map[string]stamp{"a": {now, 1}, "b": {now.Add(time.Second), 1}, "d": {now, 2}}
	got := changedKeys(prev, next)
	if strings.Join(got, ",") != "b,c,d" {
		t.Errorf("changed %v, want b,c,d", got)
	}
}

// TestGameToolsInproc drives the tools against a real bridge on a test
// engine.
func TestGameToolsInproc(t *testing.T) {
	dir := copyFixture(t)
	s, _, tl, err := build(Options{ProjectOptions: ProjectOptions{Dir: dir}})
	if err != nil {
		t.Fatal(err)
	}
	p := newInproc(t)
	if err := p.cl.waitReady(context.Background(), 5*time.Second, 0); err != nil {
		t.Fatal(err)
	}
	if err := p.cl.apply(context.Background(), "document", "", levelText(visualObject("a", 1, 2), visualObject("b", 30, 40))); err != nil {
		t.Fatal(err)
	}
	attachClient(t, tl, p.cl, levelA, false)

	st := mustCall(t, s, "game_status", `{}`)
	if st["running"] != true || st["playing"] != false || st["objects"] != float64(2) {
		t.Errorf("game_status = %v", st)
	}
	live := mustCall(t, s, "list_live_objects", `{}`)
	if len(live["objects"].([]any)) != 2 {
		t.Errorf("list_live_objects = %v", live)
	}
	sel := mustCall(t, s, "select_objects", `{"ids":["b","zzz"],"reveal":true}`)
	if len(sel["selection"].([]any)) != 1 || sel["note"] == nil {
		t.Errorf("select_objects = %v", sel)
	}
	cam := mustCall(t, s, "set_camera", `{"x":5,"y":6,"zoom":2}`)["camera"].(map[string]any)
	if cam["x"] != float64(5) || cam["zoom"] != float64(2) {
		t.Errorf("set_camera = %v", cam)
	}
	if got := mustCall(t, s, "get_camera", `{}`)["camera"].(map[string]any); got["y"] != float64(6) {
		t.Errorf("get_camera = %v", got)
	}
	mustCall(t, s, "frame_view", `{}`)

	before := p.runs.Load()
	if out := mustCall(t, s, "step", `{"count":4}`); out["ticks"] != float64(4) || p.runs.Load()-before != 4 {
		t.Errorf("step = %v, ran %d", out, p.runs.Load()-before)
	}
	mustFail(t, s, "step", `{"count":601}`)
	if out := mustCall(t, s, "play", `{}`); out["playing"] != true {
		t.Errorf("play = %v", out)
	}
	mustContain(t, mustFail(t, s, "step", `{}`), "playing")
	mustCall(t, s, "stop", `{}`)
	if out := mustCall(t, s, "stop", `{}`); !strings.Contains(out["note"].(string), "not playing") {
		t.Errorf("stop in edit mode = %v", out)
	}

	layers := mustCall(t, s, "list_map_layers", `{}`)
	if len(layers["layers"].([]any)) != 0 {
		t.Errorf("list_map_layers = %v", layers)
	}
	mustContain(t, mustFail(t, s, "set_map_layer_hidden", `{"map":"x.tmj","path":"l","hidden":true}`), "no map layer")
	set := mustCall(t, s, "editor_settings", `{"snap":8,"colliders":true}`)
	if set["snap"] != float64(8) || set["colliders"] != true {
		t.Errorf("editor_settings = %v", set)
	}

	// A prefab cannot replace a level in a level stage.
	mustContain(t, mustFail(t, s, "open_document", `{"path":"global/prefabs/crate.prefab.json"}`), "stage")
	mustFail(t, s, "reload_assets", `{"paths":[]}`)
	mustCall(t, s, "reload_assets", `{"paths":["global/dot.png"]}`)

	logs := mustCall(t, s, "read_logs", `{}`)
	if logs["next"] == nil {
		t.Errorf("read_logs = %v", logs)
	}
}

// TestScreenshotTool: the tool returns an image and the numbers, refuses a
// headless game, and has the hotkey capture for last.
func TestScreenshotTool(t *testing.T) {
	_, s, tl, f := fakeProject(t)
	f.reply = func(m map[string]any) []any {
		if m["type"] != "screenshot" {
			return nil
		}
		return []any{map[string]any{"type": "screenshot", "id": m["id"], "png": []byte("PNGDATA"), "width": 320, "height": 180, "tick": 9}}
	}
	res, _ := callRaw(t, s, "screenshot", `{"target":"window"}`)
	if res.IsError || len(res.Content) != 2 || res.Content[1].Type != "image" || res.Content[1].MimeType != "image/png" {
		t.Fatalf("screenshot result %+v", res)
	}
	mustContain(t, res.Content[0].Text, `"width":320`)
	mustContain(t, res.Content[0].Text, `"tick":9`)

	res, _ = callRaw(t, s, "screenshot", `{"last":true}`)
	if !res.IsError || !strings.Contains(res.Content[0].Text, "not taken a capture") {
		t.Errorf("last without a capture: %+v", res)
	}
	f.cl.handle([]byte(`{"type":"screenshot","id":"","png":"aG90a2V5","width":4,"height":3,"tick":2}`))
	res, _ = callRaw(t, s, "screenshot", `{"last":true}`)
	if res.IsError || !strings.Contains(res.Content[0].Text, `"width":4`) {
		t.Errorf("last = %+v", res)
	}

	tl.game.current().headless = true
	res, _ = callRaw(t, s, "screenshot", `{}`)
	if !res.IsError || !strings.Contains(res.Content[0].Text, "without a window") {
		t.Errorf("headless screenshot: %+v", res)
	}
	for _, m := range f.sent {
		if m["type"] == "screenshot" && m["target"] == "window" {
			return
		}
	}
	t.Error("no screenshot request reached the game")
}
