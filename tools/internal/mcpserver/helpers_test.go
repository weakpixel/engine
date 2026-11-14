package mcpserver

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/weakpixel/engine/tools/internal/mcp"
)

// fixture is the test project under testdata/project.
const fixture = "testdata/project"

// copyFixture copies the test project into a temporary folder, for tests
// that change files.
func copyFixture(t *testing.T) string {
	t.Helper()
	dst := t.TempDir()
	err := filepath.Walk(fixture, func(p string, fi os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(fixture, p)
		target := filepath.Join(dst, rel)
		if fi.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(p)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
	// A go.mod that uses this checkout, so the engine's GOD files are found
	// as in a game.
	engine, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatal(err)
	}
	mod := "module example.com/demo\n\ngo 1.27\n\nrequire github.com/weakpixel/engine v0.0.0\n\nreplace github.com/weakpixel/engine => " + engine + "\n"
	if err := os.WriteFile(filepath.Join(dst, "go.mod"), []byte(mod), 0o644); err != nil {
		t.Fatal(err)
	}
	return dst
}

// newServer opens the project in dir and returns its server.
func newServer(t *testing.T, dir string) *mcp.Server {
	t.Helper()
	s, _, err := New(Options{ProjectOptions: ProjectOptions{Dir: dir}})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

// callTool runs the tool's handler with args and returns the structured
// result, or the text of a tool error.
func callTool(t *testing.T, s *mcp.Server, name, args string) (map[string]any, string) {
	t.Helper()
	for _, tl := range s.Tools() {
		if tl.Name != name {
			continue
		}
		var v any
		if err := json.Unmarshal([]byte(args), &v); err != nil {
			t.Fatalf("bad test arguments: %v", err)
		}
		if err := tl.Input.Validate(v); err != nil {
			return nil, "invalid arguments: " + err.Error()
		}
		r, err := tl.Handler(context.Background(), json.RawMessage(args))
		if err != nil {
			return nil, err.Error()
		}
		if r.IsError {
			return nil, r.Content[0].Text
		}
		// Round-trip through JSON, as a client sees it.
		var out map[string]any
		if err := json.Unmarshal([]byte(r.Content[0].Text), &out); err != nil {
			t.Fatalf("%s: result is not a JSON object: %v", name, err)
		}
		return out, ""
	}
	t.Fatalf("no tool %s", name)
	return nil, ""
}

// mustCall is callTool for a call that must succeed.
func mustCall(t *testing.T, s *mcp.Server, name, args string) map[string]any {
	t.Helper()
	out, errText := callTool(t, s, name, args)
	if errText != "" {
		t.Fatalf("%s %s: %s", name, args, errText)
	}
	return out
}

func mustContain(t *testing.T, got, want string) {
	t.Helper()
	if !strings.Contains(got, want) {
		t.Errorf("got %q, want it to contain %q", got, want)
	}
}

// levelA is the asset key of the fixture level the edit tests change.
const levelA = "a/a.level.json"

// readAsset returns the text of the asset key in the copied project dir.
func readAsset(t *testing.T, dir, key string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, "assets", filepath.FromSlash(key)))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// mustFail calls a tool that must be refused and returns the message.
func mustFail(t *testing.T, s *mcp.Server, name, args string) string {
	t.Helper()
	out, errText := callTool(t, s, name, args)
	if errText == "" {
		t.Fatalf("%s %s succeeded (%v), want an error", name, args, out)
	}
	return errText
}

// mustJSONText renders a tool result as JSON text, for substring checks.
func mustJSONText(v any) string {
	data, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return string(data)
}

func jsonNumber(f float64) string { return strconv.FormatFloat(f, 'f', -1, 64) }

// callRaw runs a tool's handler and returns its raw result (images
// included); the error of the handler becomes an error result.
func callRaw(t *testing.T, s *mcp.Server, name, args string) (*mcp.Result, error) {
	t.Helper()
	for _, tl := range s.Tools() {
		if tl.Name != name {
			continue
		}
		r, err := tl.Handler(context.Background(), json.RawMessage(args))
		if err != nil {
			return mcp.ErrorResult("%s", err.Error()), nil
		}
		return r, nil
	}
	t.Fatalf("no tool %s", name)
	return nil, nil
}
