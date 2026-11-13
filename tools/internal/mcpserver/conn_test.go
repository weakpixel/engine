package mcpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"strings"
	"sync/atomic"
	"testing"
	"testing/fstest"
	"time"

	"github.com/weakpixel/ecs"
	"github.com/weakpixel/ecs/system"
	"github.com/weakpixel/engine/editor"
	"github.com/weakpixel/engine/editor/bridge"
	"github.com/weakpixel/engine/game"
	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/input"
)

// inproc is a real bridge on a test engine, connected to a client without a
// network: the engine ticks on its own goroutine, as a game does.
type inproc struct {
	cl   *client
	b    *bridge.Bridge
	e    *game.Engine
	runs atomic.Int64 // ticks the game's own systems ran
	// move makes the game's system move object "a" right by 1 per tick.
	move  atomic.Bool
	stop  chan struct{}
	done  chan struct{}
	conn  *procConn
	delay time.Duration
}

// procConn is the client's gameConn: it hands messages to the bridge.
type procConn struct{ b *bridge.Bridge }

func (c *procConn) Send(msg []byte) error { c.b.Receive(msg); return nil }
func (c *procConn) Close() error          { return nil }

// toClient is the bridge's Transport: it hands messages to the client.
type toClient struct{ cl *client }

func (t toClient) Send(msg []byte) { t.cl.handle(msg) }

func newInproc(t *testing.T) *inproc {
	t.Helper()
	var buf bytes.Buffer
	if err := png.Encode(&buf, image.NewRGBA(image.Rect(0, 0, 16, 8))); err != nil {
		t.Fatal(err)
	}
	files := fstest.MapFS{"box.png": {Data: buf.Bytes()}}
	p := &inproc{stop: make(chan struct{}), done: make(chan struct{}), conn: &procConn{}}
	p.cl = newClient(p.conn, newLogRing())
	p.b = bridge.New(editor.Config{Path: "test.level.json"}, toClient{p.cl})
	p.conn.b = p.b
	p.e = game.New(game.Config{
		NoAudio: true, Width: 320, Height: 180, Tool: p.b, FS: files,
		Input: game.InputConfig{Source: &input.ManualSource{}},
	})
	p.e.AddSystem(game.StageSimulate, func(*system.Context) {
		p.runs.Add(1)
		if en, ok := p.b.Entity("a"); ok && p.move.Load() && p.e.World().IsAlive(en) {
			ecs.GetComp[gfx.Transform](p.e.World(), en).Pos.X++
		}
	})
	go func() {
		defer close(p.done)
		for {
			select {
			case <-p.stop:
				return
			default:
			}
			if err := p.e.Update(); err != nil {
				return
			}
			time.Sleep(time.Millisecond)
		}
	}()
	t.Cleanup(func() {
		close(p.stop)
		<-p.done
		p.e.Close()
	})
	return p
}

func visualObject(id string, x, y float64) string {
	return fmt.Sprintf(`{"id":%q,"group":"gfx","class":"Visual","components":{`+
		`"gfx:Transform":{"Pos":{"X":%g,"Y":%g},"PrevPos":{"X":%g,"Y":%g},"Rot":0,"Scale":{"X":1,"Y":1}},`+
		`"gfx:Sprite":{"ID":"box.png","Layer":0,"Origin":{"X":0,"Y":0},"FlipX":false,"FlipY":false,"Tint":0,"Blend":0,"Hidden":false}}}`,
		id, x, y, x, y)
}

func levelText(objs ...string) []byte {
	return []byte(`{"objects":[` + strings.Join(objs, ",") + `]}`)
}

func TestConn(t *testing.T) {
	p := newInproc(t)
	ctx := context.Background()
	cl := p.cl
	if err := cl.waitReady(ctx, 5*time.Second, bridge.Version); err != nil {
		t.Fatal(err)
	}
	if err := cl.waitReady(ctx, time.Second, bridge.Version+1); err == nil {
		t.Error("a protocol version mismatch was accepted")
	}

	t.Run("document", func(t *testing.T) {
		if err := cl.apply(ctx, "document", "", levelText(visualObject("a", 1, 2), visualObject("b", 30, 40))); err != nil {
			t.Fatal(err)
		}
		st := cl.state()
		if len(st.Objects) != 2 || st.Objects[0].ID != "a" {
			t.Errorf("objects %+v", st.Objects)
		}
		// A document the game refuses: the error comes back and the old text stays.
		err := cl.apply(ctx, "document", "", []byte(`{"objects":[{"id":"x","group":"nope","class":"Nope","components":{}}]}`))
		if err == nil {
			t.Fatal("a document with an unknown class was applied")
		}
		if len(cl.state().Objects) != 2 {
			t.Errorf("the refused document changed the objects: %+v", cl.state().Objects)
		}
	})

	t.Run("select", func(t *testing.T) {
		sel, err := cl.selectIDs(ctx, []string{"b", "nope"}, false)
		if err != nil || len(sel) != 1 || sel[0] != "b" {
			t.Errorf("selection %v, %v", sel, err)
		}
		if sel, _ := cl.selectIDs(ctx, nil, false); len(sel) != 0 {
			t.Errorf("selection after clearing: %v", sel)
		}
	})

	t.Run("step", func(t *testing.T) {
		before := p.runs.Load()
		if err := cl.step(ctx, 5); err != nil {
			t.Fatal(err)
		}
		if got := p.runs.Load() - before; got != 5 {
			t.Errorf("the game ran %d ticks for step 5, want 5", got)
		}
		// The game stays paused afterwards.
		time.Sleep(30 * time.Millisecond)
		if got := p.runs.Load() - before; got != 5 {
			t.Errorf("the game kept running after step: %d ticks", got)
		}
	})

	t.Run("play", func(t *testing.T) {
		on, err := cl.setPlay(ctx, true)
		if err != nil || !on {
			t.Fatalf("play: %v, %v", on, err)
		}
		if err := cl.step(ctx, 1); err == nil || !strings.Contains(err.Error(), "playing") {
			t.Errorf("step while playing: %v", err)
		}
		before := p.runs.Load()
		deadline := time.Now().Add(5 * time.Second)
		for p.runs.Load() < before+3 && time.Now().Before(deadline) {
			time.Sleep(2 * time.Millisecond)
		}
		if p.runs.Load() < before+3 {
			t.Error("the game does not run in play mode")
		}
		on, err = cl.setPlay(ctx, false)
		if err != nil || on {
			t.Fatalf("stop: %v, %v", on, err)
		}
		if len(cl.state().Objects) != 2 {
			t.Errorf("stop lost the objects: %+v", cl.state().Objects)
		}
	})

	t.Run("camera", func(t *testing.T) {
		cam, err := cl.setCamera(ctx, 12, 34, 2)
		if err != nil || cam != (cameraState{X: 12, Y: 34, Zoom: 2}) {
			t.Errorf("camera %+v, %v", cam, err)
		}
		if _, err := cl.frame(ctx); err != nil {
			t.Error(err)
		}
	})

	t.Run("errors", func(t *testing.T) {
		errs, err := cl.sendSync(ctx, map[string]any{"type": "mapLayerHidden", "map": "no.tmj", "path": "x", "hidden": true})
		if err != nil || len(errs) != 1 || !strings.Contains(errs[0], "no map layer") {
			t.Errorf("errors %v, %v", errs, err)
		}
	})

	t.Run("status", func(t *testing.T) {
		a, err := cl.queryStatus(ctx)
		if err != nil {
			t.Fatal(err)
		}
		b, _ := cl.queryStatus(ctx)
		if b.Tick <= a.Tick || a.Play || a.Scenes == nil {
			t.Errorf("status %+v then %+v", a, b)
		}
		lines, _, _ := cl.logs.read(0, 100)
		for _, l := range lines {
			if strings.Contains(l.Text, "unknown message type") {
				t.Errorf("a round trip reached the log as an error: %v", l)
			}
		}
	})

	t.Run("closed", func(t *testing.T) {
		cl.markClosed()
		if err := cl.step(ctx, 1); err == nil {
			t.Error("a closed client accepted a request")
		}
	})
}

// TestScreenshotPairing: requests and answers pair by id, an answer with an
// empty id is the game's own capture, and an error answer is an error.
func TestScreenshotPairing(t *testing.T) {
	f := &fakeGame{t: t}
	cl := newClient(f, newLogRing())
	f.cl = cl
	f.reply = func(m map[string]any) []any {
		if m["type"] != "screenshot" {
			return nil
		}
		switch m["target"] {
		case "window":
			return []any{map[string]any{"type": "screenshot", "id": m["id"], "error": "no window yet"}}
		}
		return []any{
			map[string]any{"type": "screenshot", "id": "", "png": []byte("hotkey"), "width": 9, "height": 9, "tick": 1}, // F12 meanwhile
			map[string]any{"type": "screenshot", "id": m["id"], "png": []byte("mine"), "width": 3, "height": 2, "tick": 42},
		}
	}
	a, err := cl.screenshot(context.Background(), "game", 2, false)
	if err != nil || string(a.PNG) != "mine" || a.Width != 3 || a.Tick != 42 {
		t.Fatalf("answer %+v, %v", a, err)
	}
	if last := cl.latestShot(); last == nil || string(last.PNG) != "hotkey" {
		t.Errorf("latest capture %+v", last)
	}
	if _, err := cl.screenshot(context.Background(), "window", 1, false); err == nil || !strings.Contains(err.Error(), "no window yet") {
		t.Errorf("error answer: %v", err)
	}
	var sent map[string]any
	for _, m := range f.sent {
		if m["type"] == "screenshot" {
			sent = m
			break
		}
	}
	if sent["target"] != "game" || sent["scale"] != float64(2) || sent["id"] == "" {
		t.Errorf("request %v", sent)
	}
}

// fakeGame is a gameConn that records what the client sends and answers
// through reply, synchronously. It answers "status" with a tick that grows
// by one per answer.
type fakeGame struct {
	t     *testing.T
	cl    *client
	reply func(m map[string]any) []any
	sent  []map[string]any
	tick  int
}

func (f *fakeGame) Send(msg []byte) error {
	var m map[string]any
	if err := json.Unmarshal(msg, &m); err != nil {
		f.t.Errorf("bad message %q: %v", msg, err)
		return nil
	}
	f.sent = append(f.sent, m)
	var answers []any
	switch m["type"] {
	case "status":
		f.tick++
		st := f.cl.state()
		answers = []any{map[string]any{"type": "status", "tick": f.tick, "play": st.Play, "paused": st.Paused, "scenes": []string{"root"}}}
	case "document", "open":
		answers = []any{map[string]any{"type": "applied", "error": ""}}
	}
	if f.reply != nil {
		answers = append(answers, f.reply(m)...)
	}
	for _, a := range answers {
		data, _ := json.Marshal(a)
		f.cl.handle(data)
	}
	return nil
}

func (f *fakeGame) Close() error { return nil }
