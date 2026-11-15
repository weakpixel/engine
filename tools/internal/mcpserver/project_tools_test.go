package mcpserver

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestProjectTools(t *testing.T) {
	s := newServer(t, fixture)

	t.Run("project_info", func(t *testing.T) {
		out := mustCall(t, s, "project_info", `{}`)
		if out["gameRunning"] != false || out["godError"] != nil || out["objectTypes"].(float64) < 2 {
			t.Errorf("project_info = %v", out)
		}
		if out["editor"].(map[string]any)["snap"] != 8.0 {
			t.Errorf("editor = %v", out["editor"])
		}
	})

	t.Run("list_components", func(t *testing.T) {
		out := mustCall(t, s, "list_components", `{"group":"demo"}`)
		got, _ := json.Marshal(out["components"])
		if string(got) != `[{"doc":"Health is what a crate can take.","goType":"example.com/demo.Health","key":"demo:Health"}]` {
			t.Errorf("components = %s", got)
		}
		all := mustCall(t, s, "list_components", `{}`)["components"].([]any)
		if len(all) < 5 {
			t.Errorf("only %d components without a filter", len(all))
		}
	})

	t.Run("describe_component", func(t *testing.T) {
		out := mustCall(t, s, "describe_component", `{"key":"demo:Health"}`)
		if init, _ := json.Marshal(out["initialValue"]); string(init) != `{"HP":3,"Kind":0}` {
			t.Errorf("initialValue = %s", init)
		}
		if used, _ := json.Marshal(out["usedBy"]); string(used) != `["demo:Crate"]` {
			t.Errorf("usedBy = %s", used)
		}
		enum := out["schema"].(map[string]any)["properties"].(map[string]any)["Kind"].(map[string]any)["x-enum"]
		if enum == nil {
			t.Error("schema lacks the x-enum")
		}
		_, errText := callTool(t, s, "describe_component", `{"key":"health"}`)
		mustContain(t, errText, "did you mean demo:Health")
		_, errText = callTool(t, s, "describe_component", `{"key":"demo:Nope"}`)
		mustContain(t, errText, `unknown component "demo:Nope"`)
	})

	t.Run("object types", func(t *testing.T) {
		list := mustCall(t, s, "list_object_types", `{"group":"demo"}`)
		if got, _ := json.Marshal(list["objectTypes"]); !strings.Contains(string(got), `"components":["gfx:Transform","demo:Health"]`) {
			t.Errorf("objectTypes = %s", got)
		}
		out := mustCall(t, s, "describe_object_type", `{"key":"demo:Crate"}`)
		comps := out["components"].([]any)
		if len(comps) != 2 || comps[1].(map[string]any)["optional"] != true || comps[0].(map[string]any)["field"] != "Transform" {
			t.Errorf("components = %v", comps)
		}
		_, errText := callTool(t, s, "describe_object_type", `{"key":"demo:crate"}`)
		mustContain(t, errText, "did you mean demo:Crate")
	})

	t.Run("list_assets", func(t *testing.T) {
		out := mustCall(t, s, "list_assets", `{}`)
		got, _ := json.Marshal(out["assets"])
		want := `[{"key":"a/a.level.json","kind":"level"},{"key":"a/bad.level.json","kind":"level"},` +
			`{"key":"global/dot.png","kind":"texture"},{"key":"global/prefabs/crate.prefab.json","kind":"prefab"},` +
			`{"key":"global/prefabs/metal-crate.prefab.json","kind":"prefab"}]`
		if string(got) != want {
			t.Errorf("assets = %s", got)
		}
		out = mustCall(t, s, "list_assets", `{"kind":"prefab","limit":1}`)
		if out["total"] != 2.0 || out["truncated"] != true || len(out["assets"].([]any)) != 1 {
			t.Errorf("filtered = %v", out)
		}
		out = mustCall(t, s, "list_assets", `{"prefix":"a/"}`)
		if out["total"] != 2.0 {
			t.Errorf("prefix = %v", out)
		}
		_, errText := callTool(t, s, "list_assets", `{"kind":"image"}`)
		mustContain(t, errText, `"image" is not one of`)
	})

	t.Run("check_assets", func(t *testing.T) {
		out := mustCall(t, s, "check_assets", `{}`)
		if _, ok := out["findings"].([]any); !ok || out["errors"] == nil {
			t.Errorf("check_assets = %v", out)
		}
	})

	t.Run("list_documents", func(t *testing.T) {
		out := mustCall(t, s, "list_documents", `{"kind":"level"}`)
		got, _ := json.Marshal(out["documents"])
		want := `[{"kind":"level","name":"Level A","objects":3,"path":"a/a.level.json"},{"kind":"level","objects":3,"path":"a/bad.level.json"}]`
		if string(got) != want {
			t.Errorf("documents = %s", got)
		}
	})
}

func TestProjectToolsWithoutAssetRoot(t *testing.T) {
	dir := t.TempDir()
	s := newServer(t, dir)
	for _, name := range []string{"list_assets", "check_assets", "list_documents"} {
		_, errText := callTool(t, s, name, `{}`)
		mustContain(t, errText, "set assets.root")
	}
	_, errText := callTool(t, s, "read_document", `{"path":"a.level.json"}`)
	mustContain(t, errText, "set assets.root")
}

func TestStarterVoice(t *testing.T) {
	for _, c := range []struct {
		want, key string
		starter   bool
	}{
		{"voice", "chirpy", true},
		{"voice", "talk/kip.voice.json", false},
		{"voice", "kip.voice.json", false},
		{"voice", "talk/chirpy", false},
		{"sound", "chirpy", false},
		{"dialogue", "village", false},
	} {
		if got := starterVoice(c.want, c.key); got != c.starter {
			t.Errorf("starterVoice(%q, %q) = %v, want %v", c.want, c.key, got, c.starter)
		}
	}
}

func TestListAssetsSkipsIgnored(t *testing.T) {
	dir := copyFixture(t)
	if err := os.WriteFile(filepath.Join(dir, "assets", ".assetignore"), []byte("bad.level.json\nglobal/prefabs/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	s := newServer(t, dir)
	out := mustCall(t, s, "list_assets", `{}`)
	got, _ := json.Marshal(out["assets"])
	want := `[{"key":"a/a.level.json","kind":"level"},{"key":"global/dot.png","kind":"texture"}]`
	if string(got) != want {
		t.Errorf("assets = %s", got)
	}
	if _, ok := scanAssets(filepath.Join(dir, "assets"))["a/bad.level.json"]; ok {
		t.Error("the change scan reports an ignored file")
	}
}
