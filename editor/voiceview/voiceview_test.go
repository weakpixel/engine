package voiceview

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/weakpixel/engine/editor/toolshell"
	"github.com/weakpixel/engine/game"
	"github.com/weakpixel/engine/input"
)

// TestMain fixes the display scale: the real lookup (ebiten.Monitor)
// initializes the windowing system, which a headless test has no display for.
func TestMain(m *testing.M) {
	deviceScale = func() float64 { return 1 }
	os.Exit(m.Run())
}

var files = fstest.MapFS{
	"talk/kip.voice.json":     {Data: []byte(`{"version": 0, "name": "kip", "instruments": {"vowel": {}}}`)},
	"talk/town.dialogue.json": {Data: []byte(`{"version": 0, "speakers": [{"id": "wren", "name": "Wren", "voice": "chirpy"}, {"id": "kip", "voice": "kip.voice.json"}], "lines": [{"speaker": "wren", "text": "Hello there."}, {"speaker": "kip", "text": "Ae tha."}]}`)},
}

// open runs the tool on key in a headless engine for one tick.
func open(t *testing.T, key string) (*Tool, *game.Engine) {
	t.Helper()
	tool := New().(*Tool)
	sh := toolshell.New(toolshell.Config{Name: "voice", Tool: tool, Key: key})
	e := game.New(game.Config{NoAudio: true, Width: 320, Height: 180, FS: files, Tool: sh, Input: game.InputConfig{Source: &input.ManualSource{}}})
	t.Cleanup(e.Close)
	update(t, e, 1)
	return tool, e
}

func update(t *testing.T, e *game.Engine, n int) {
	t.Helper()
	for range n {
		if err := e.Update(); err != nil {
			t.Fatal(err)
		}
	}
}

func status(t *testing.T, tool *Tool) string {
	t.Helper()
	f, ok := tool.Panel().Find(IDStatus)
	if !ok {
		t.Fatal("no status")
	}
	var s string
	json.Unmarshal(f.Value, &s)
	return s
}

func set(t *testing.T, tool *Tool, id string, v any) {
	t.Helper()
	b, _ := json.Marshal(v)
	if err := tool.Set(id, b); err != nil {
		t.Fatal(err)
	}
}

func TestCheck(t *testing.T) {
	tool := New().(*Tool)
	for key, ok := range map[string]bool{"a/b.voice.json": true, "a/B.Dialogue.JSON": true, "a/x.lang.json": false, "a.png": false} {
		if err := tool.Check(key); (err == nil) != ok {
			t.Errorf("Check(%q) = %v", key, err)
		}
	}
}

// The voice says its line while the shell keeps the game paused: the tool
// steps speech itself.
func TestVoiceSpeaksWhilePaused(t *testing.T) {
	tool, e := open(t, "talk/kip.voice.json")
	if len(tool.notes) == 0 {
		t.Fatal("the line has no notes for the piano roll")
	}
	if got := status(t, tool); !strings.HasPrefix(got, "speaking") {
		t.Errorf("status %q", got)
	}
	update(t, e, 60*30)
	if tool.sp.Speaking(tool.talker) || tool.sp.Revealed(tool.talker) != len(DefaultLine) {
		t.Fatalf("the line did not finish: %q", status(t, tool))
	}
	if got := status(t, tool); !strings.HasPrefix(got, "done") {
		t.Errorf("status after the line %q", got)
	}

	set(t, tool, IDSpeed, 2)
	set(t, tool, IDText, "Ae tha.")
	if !tool.sp.Speaking(tool.talker) || tool.text != "Ae tha." {
		t.Error("a new text did not start a line")
	}
	set(t, tool, IDSkip, nil)
	if tool.sp.Speaking(tool.talker) || tool.sp.Revealed(tool.talker) != len("Ae tha.") {
		t.Error("Skip did not end the line")
	}
	if err := tool.Set(IDNext, nil); err == nil {
		t.Error("a dialogue button on a voice was accepted")
	}
	if err := tool.Set("nope", nil); err == nil {
		t.Error("an unknown widget was accepted")
	}
}

func TestDialoguePlaysAndAdvances(t *testing.T) {
	tool, e := open(t, "talk/town.dialogue.json")
	update(t, e, 2)
	if got := status(t, tool); got != "line 1 of 2, Wren" {
		t.Fatalf("status %q", got)
	}
	set(t, tool, IDNext, nil) // shows the whole line
	set(t, tool, IDNext, nil) // goes on
	update(t, e, 2)
	if got := status(t, tool); got != "line 2 of 2, kip" {
		t.Errorf("status after Next %q", got)
	}
	set(t, tool, IDEnd, nil)
	update(t, e, 1)
	if got := status(t, tool); got != "finished" {
		t.Errorf("status after Stop %q", got)
	}
	set(t, tool, IDNext, nil) // starts again
	update(t, e, 2)
	if got := status(t, tool); got != "line 1 of 2, Wren" {
		t.Errorf("status after a restart %q", got)
	}
}

// Close deletes the entities, so the host can switch files.
func TestCloseAndReopen(t *testing.T) {
	tool, e := open(t, "talk/kip.voice.json")
	sh := toolshell.New(toolshell.Config{Name: "voice", Tool: tool})
	sh.Init(e)
	old := tool.talker
	tool.Close(sh)
	if e.World().IsAlive(old) {
		t.Error("the speaker survived Close")
	}
	if err := tool.Open(sh, "talk/town.dialogue.json"); err != nil {
		t.Fatal(err)
	}
	if tool.kind != kindDialogue || !e.World().IsAlive(tool.box) {
		t.Error("the dialogue did not open")
	}
	if err := tool.Open(sh, "a.png"); err == nil {
		t.Error("a texture was opened")
	}
}

// The View section is the only one on the left and comes first.
func TestPanelSides(t *testing.T) {
	for _, key := range []string{"talk/kip.voice.json", "talk/town.dialogue.json"} {
		tool, _ := open(t, key)
		p := tool.Panel()
		if err := p.Check(); err != nil {
			t.Fatalf("%s: %v", key, err)
		}
		for i, s := range p.Sections {
			if left := s.Side == toolshell.SideLeft; left != (s.ID == "view") || (left && i != 0) {
				t.Errorf("%s: section %q (index %d) side %q", key, s.ID, i, s.Side)
			}
		}
	}
}
