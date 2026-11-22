package mcpserver

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/weakpixel/engine/tools/internal/newgame"
)

// TestIntegration creates a game the way `wpix new` does, in a temp
// dir with a replace onto this checkout, builds it with the editor tag, runs
// it headless through the tools and drives it: place an object, step, play,
// pause, input, inspect, stop, read the logs. Nothing is written into the
// checkout.
func TestIntegration(t *testing.T) {
	if testing.Short() {
		t.Skip("creates, builds and runs a game")
	}
	root, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(t.TempDir(), "mcpgame")
	var log bytes.Buffer
	if err := newgame.Run(newgame.Options{Dir: dir, Replace: root, NoGit: true}, &log, &log); err != nil {
		t.Fatalf("wpix new: %v\n%s", err, log.String())
	}
	s, p, tl, err := build(Options{ProjectOptions: ProjectOptions{Dir: dir, Game: ".", Tags: []string{"editor"}}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tl.game.shutdown)
	// The project resolves symlinks (/var -> /private/var on macOS).
	want, err := filepath.EvalSymlinks(filepath.Join(dir, "assets"))
	if err != nil {
		t.Fatal(err)
	}
	if p.AssetRoot != want {
		t.Fatalf("asset root %q, want %q", p.AssetRoot, want)
	}

	const level = "start/mcp-integration.level.json"
	mustCall(t, s, "create_document", `{"path":"`+level+`","name":"MCP integration"}`)

	out := mustCall(t, s, "game_start", `{"path":"`+level+`","headless":true}`)
	if out["running"] != true || out["stage"] != "level" {
		t.Fatalf("game_start = %v", out)
	}

	placed := mustCall(t, s, "place_object", `{"path":"`+level+`","class":"mcpgame:Player","x":40,"y":50}`)
	id, _ := placed["id"].(string)
	if id == "" || placed["changed"] != true {
		t.Fatalf("place_object = %v", placed)
	}
	// The game gets the new document asynchronously: wait for the object.
	var live map[string]any
	for range 500 {
		if live = mustCall(t, s, "list_live_objects", `{}`); strings.Contains(mustJSONText(live), `"`+id+`"`) {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !strings.Contains(mustJSONText(live), `"`+id+`"`) {
		t.Fatalf("the game does not have the placed object %s: %v", id, live)
	}

	mustCall(t, s, "step", `{"count":30}`)
	mustCall(t, s, "play", `{}`)
	if st := mustCall(t, s, "game_status", `{}`); st["playing"] != true || len(st["scenes"].([]any)) == 0 {
		t.Errorf("game_status after play: %v", st)
	}
	mustCall(t, s, "pause", `{}`)
	mustCall(t, s, "send_input", `{"keys":["D"],"ticks":5}`)
	mustCall(t, s, "step", `{"count":5}`)
	ents := mustCall(t, s, "inspect_entities", `{"all":true,"components":["gfx:Transform"]}`)
	if !strings.Contains(mustJSONText(ents), `"id":"`+id+`"`) {
		t.Errorf("inspect_entities all lacks the placed object %s: %v", id, ents)
	}
	mustCall(t, s, "stop", `{}`)
	if live := mustCall(t, s, "list_live_objects", `{}`); !strings.Contains(mustJSONText(live), `"`+id+`"`) {
		t.Errorf("stop lost the object %s: %v", id, live)
	}

	// A value the game rejects (a field of the wrong type passes no offline check).
	mustFail(t, s, "set_field", `{"path":"`+level+`","id":"`+id+`","component":"gfx:Transform","field":["Pos","X"],"value":"left"}`)

	logs := mustCall(t, s, "read_logs", `{}`)
	if logs["lines"] == nil {
		t.Errorf("read_logs = %v", logs)
	}
	if shot, _ := callRaw(t, s, "screenshot", `{}`); shot == nil || !shot.IsError {
		t.Error("a headless game took a screenshot")
	}
	if st := mustCall(t, s, "game_stop", `{}`); st["stopped"] != true {
		t.Errorf("game_stop = %v", st)
	}
}
