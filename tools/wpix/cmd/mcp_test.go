package cmd

import (
	"bufio"
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/weakpixel/engine/tools/internal/cli"
)

// runMCP runs `wpix mcp` on the script's lines and returns the answers
// by id.
func runMCP(t *testing.T, script ...string) map[string]map[string]any {
	t.Helper()
	root := newRootCmd()
	root.SetArgs([]string{"mcp", "--project", "../../internal/mcpserver/testdata/project"})
	var stdout, stderr bytes.Buffer
	root.SetIn(strings.NewReader(strings.Join(script, "\n") + "\n"))
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	if code := cli.Execute(root, &stderr); code != 0 {
		t.Fatalf("exit code %d: %s", code, stderr.String())
	}
	answers := map[string]map[string]any{}
	sc := bufio.NewScanner(&stdout)
	sc.Buffer(nil, 1<<24)
	for sc.Scan() {
		var m map[string]any
		if err := json.Unmarshal(sc.Bytes(), &m); err != nil {
			t.Fatalf("stdout carries a line that is not JSON-RPC: %q", sc.Text())
		}
		answers[string(mustJSON(m["id"]))] = m
	}
	return answers
}

func mustJSON(v any) []byte {
	data, _ := json.Marshal(v)
	return data
}

// toolCount is how many tools milestones 1 to 4 register.
const toolCount = 43

func TestMCPModern(t *testing.T) {
	const meta = `"_meta":{"io.modelcontextprotocol/protocolVersion":"2026-07-28","io.modelcontextprotocol/clientInfo":{"name":"test","version":"1"},"io.modelcontextprotocol/clientCapabilities":{}}`
	answers := runMCP(t,
		`{"jsonrpc":"2.0","id":1,"method":"server/discover","params":{`+meta+`}}`,
		`{"jsonrpc":"2.0","id":2,"method":"tools/list","params":{`+meta+`}}`,
		`{"jsonrpc":"2.0","id":3,"method":"tools/call","params":{"name":"list_components","arguments":{"group":"demo"},`+meta+`}}`,
	)
	discover := answers["1"]["result"].(map[string]any)
	if discover["supportedVersions"].([]any)[0] != "2026-07-28" {
		t.Errorf("discover = %v", discover)
	}
	tools := answers["2"]["result"].(map[string]any)["tools"].([]any)
	if len(tools) != toolCount || tools[0].(map[string]any)["name"] != "project_info" {
		t.Errorf("tools/list has %d tools, first %v", len(tools), tools[0])
	}
	call := answers["3"]["result"].(map[string]any)
	if call["isError"] != false || !strings.Contains(string(mustJSON(call["structuredContent"])), `"key":"demo:Health"`) {
		t.Errorf("list_components = %v", call)
	}
}

func TestMCPLegacy(t *testing.T) {
	answers := runMCP(t,
		`{"jsonrpc":"2.0","id":"init","method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"test","version":"1"}}}`,
		`{"jsonrpc":"2.0","method":"notifications/initialized"}`,
		`{"jsonrpc":"2.0","id":"list","method":"tools/list"}`,
		`{"jsonrpc":"2.0","id":"call","method":"tools/call","params":{"name":"list_components","arguments":{}}}`,
	)
	if v := answers[`"init"`]["result"].(map[string]any)["protocolVersion"]; v != "2025-11-25" {
		t.Errorf("protocolVersion = %v", v)
	}
	if n := len(answers[`"list"`]["result"].(map[string]any)["tools"].([]any)); n != toolCount {
		t.Errorf("tools/list has %d tools", n)
	}
	if !strings.Contains(string(mustJSON(answers[`"call"`]["result"])), "gfx:Transform") {
		t.Errorf("list_components = %v", answers[`"call"`])
	}
}

func TestMCPUsage(t *testing.T) {
	var stdout, stderr bytes.Buffer
	if code := Execute([]string{"mcp", "extra"}, &stdout, &stderr); code != 2 {
		t.Errorf("exit code %d, want 2: %s", code, stderr.String())
	}
}
