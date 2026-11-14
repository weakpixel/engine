package mcpserver

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/weakpixel/engine/editor/bridge"
	"github.com/weakpixel/engine/editor/hostlink"
	"github.com/weakpixel/engine/tools/internal/mcp"
)

// TestProtocolConstants: the names and the version this package repeats are
// the bridge's.
func TestProtocolConstants(t *testing.T) {
	for _, c := range [][2]string{
		{envBridge, bridge.EnvBridge}, {envToken, bridge.EnvBridgeToken}, {envStage, hostlink.EnvStage},
		{envPath, hostlink.EnvPath}, {envHeadless, hostlink.EnvHeadless},
	} {
		if c[0] != c[1] {
			t.Errorf("environment variable %q, bridge has %q", c[0], c[1])
		}
	}
	if protocolVersion != bridge.Version {
		t.Errorf("protocolVersion %d, bridge.Version %d", protocolVersion, bridge.Version)
	}
	// The transport variables must not clash with the screenshot ones.
	for _, v := range []string{envBridge, envToken, envStage, envPath, envHeadless} {
		if strings.HasPrefix(v, "WEAKPIXEL_SCREENSHOT_") {
			t.Errorf("%s clashes with the screenshot variables", v)
		}
	}
}

// fakeGameProject is a copy of the test project with the stand-in game as
// its editor package (folder game/).
func fakeGameProject(t *testing.T) (dir string, s *mcp.Server, tl *tools) {
	t.Helper()
	dir = copyFixture(t)
	src, err := os.ReadFile("testdata/fakegame/main.go")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "game"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "game", "main.go"), src, 0o644); err != nil {
		t.Fatal(err)
	}
	s, _, tl, err = build(Options{ProjectOptions: ProjectOptions{Dir: dir, Game: "game", Tags: []string{"editor"}}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(tl.game.shutdown)
	return dir, s, tl
}

// TestGameProcess runs the stand-in game as a child process: build into
// build/, connect with the token, log capture, rejected documents, stop.
func TestGameProcess(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a program")
	}
	dir, s, tl := fakeGameProject(t)

	st := mustCall(t, s, "game_status", `{}`)
	if st["running"] != false {
		t.Fatalf("status before start: %v", st)
	}
	mustFail(t, s, "step", `{}`) // no game

	out := mustCall(t, s, "game_start", `{"path":"`+levelA+`","headless":true}`)
	if out["running"] != true || out["path"] != levelA || out["stage"] != "level" || out["headless"] != true {
		t.Fatalf("game_start = %v", out)
	}
	if _, err := os.Stat(filepath.Join(dir, "build", "weakpixel-mcp", "game")); err != nil {
		t.Errorf("the binary is not under build/weakpixel-mcp/: %v", err)
	}
	if matches, _ := filepath.Glob(filepath.Join(dir, "game", "game*")); len(matches) > 0 {
		t.Errorf("a binary was written next to the source: %v", matches)
	}
	mustFail(t, s, "game_start", `{"path":"`+levelA+`"}`) // already running

	logs := mustCall(t, s, "read_logs", `{}`)
	text := mustJSONText(logs)
	for _, want := range []string{"fakegame stdout hello", "fakegame stderr hello", `stage=level path=` + levelA, `headless=\"1\"`} {
		mustContain(t, text, want)
	}
	next := logs["next"].(float64)
	if again := mustCall(t, s, "read_logs", `{"since":`+jsonNumber(next)+`}`); len(again["lines"].([]any)) != 0 {
		t.Errorf("read_logs with the cursor returned old lines: %v", again)
	}

	// An edit goes through the game; a document the game refuses is rolled back.
	before := readAsset(t, dir, levelA)
	placed := mustCall(t, s, "place_object", `{"path":"`+levelA+`","class":"demo:Crate","x":5,"y":6}`)
	if placed["changed"] != true {
		t.Fatalf("place_object = %v", placed)
	}
	live := mustCall(t, s, "list_live_objects", `{}`)
	if len(live["objects"].([]any)) != 1 {
		t.Errorf("list_live_objects = %v", live)
	}
	afterPlace := readAsset(t, dir, levelA)
	msg := mustFail(t, s, "write_asset_document", `{"path":"`+levelA+`","text":"{\"name\":\"REJECT\",\"objects\":[]}"}`)
	mustContain(t, msg, "rejected by the test game")
	if got := readAsset(t, dir, levelA); got != afterPlace {
		t.Errorf("a refused document was not rolled back:\n%s", got)
	}
	if before == afterPlace {
		t.Error("place_object changed nothing")
	}

	if shot, _ := callRaw(t, s, "screenshot", `{}`); shot != nil && !shot.IsError {
		t.Error("screenshot of a headless game was not refused")
	}

	stopped := mustCall(t, s, "game_stop", `{}`)
	if stopped["stopped"] != true {
		t.Fatalf("game_stop = %v", stopped)
	}
	st = mustCall(t, s, "game_status", `{}`)
	if st["running"] != false || st["last_exit"] == nil {
		t.Errorf("status after stop: %v", st)
	}
	if tl.game.current() != nil {
		t.Error("the game still counts as running")
	}
	end := mustJSONText(mustCall(t, s, "read_logs", `{}`))
	mustContain(t, end, "fakegame: connection closed") // the game saw the close and ended by itself
}

// TestGameBuildError: a game that does not build gives the compiler output,
// and a running game stays.
func TestGameBuildError(t *testing.T) {
	if testing.Short() {
		t.Skip("builds a program")
	}
	dir, s, _ := fakeGameProject(t)
	mustCall(t, s, "game_start", `{"path":"`+levelA+`","headless":true}`)
	if err := os.WriteFile(filepath.Join(dir, "game", "broken.go"), []byte("//go:build editor\n\npackage main\n\nvar x int = \"no\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	msg := mustFail(t, s, "game_rebuild", `{}`)
	mustContain(t, msg, "did not build")
	mustContain(t, msg, "broken.go")
	if st := mustCall(t, s, "game_status", `{}`); st["running"] != true {
		t.Errorf("the old game did not keep running: %v", st)
	}
	os.Remove(filepath.Join(dir, "game", "broken.go"))
	out := mustCall(t, s, "game_rebuild", `{}`)
	if out["running"] != true || out["path"] != levelA {
		t.Errorf("game_rebuild = %v", out)
	}
	mustCall(t, s, "game_stop", `{}`)
}

// TestGameStartWithoutPackage names the missing setting.
func TestGameStartWithoutPackage(t *testing.T) {
	dir := copyFixture(t)
	s, _, _, err := build(Options{ProjectOptions: ProjectOptions{Dir: dir, Game: "nope"}})
	if err != nil {
		t.Fatal(err)
	}
	mustContain(t, mustFail(t, s, "game_start", `{"path":"`+levelA+`"}`), "not a folder")
	mustContain(t, mustFail(t, s, "game_start", `{}`), `missing required`)
}
