package mcpserver

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadDocument(t *testing.T) {
	s := newServer(t, fixture)

	t.Run("valid level", func(t *testing.T) {
		out := mustCall(t, s, "read_document", `{"path":"a/a.level.json"}`)
		data, _ := os.ReadFile(filepath.Join(fixture, "assets/a/a.level.json"))
		if out["text"] != string(data) || out["sha"] != sha(data) || out["name"] != "Level A" || out["kind"] != "level" {
			t.Errorf("text, sha, name or kind wrong: %v", out)
		}
		if out["valid"] != true {
			t.Errorf("problems = %v", out["problems"])
		}
		got, _ := json.Marshal(out["objects"])
		want := `[{"id":"left","index":0,"prefab":"global/prefabs/metal-crate.prefab.json","type":"demo:Crate"},` +
			`{"id":"bare","index":1,"prefab":"global/prefabs/crate.prefab.json","type":"demo:Crate"},{"index":2,"type":"demo:Crate"}]`
		if string(got) != want {
			t.Errorf("objects = %s", got)
		}
		out = mustCall(t, s, "read_document", `{"path":"a/a.level.json","include_text":false}`)
		if _, ok := out["text"]; ok {
			t.Error("include_text false still returns the text")
		}
	})

	t.Run("problems", func(t *testing.T) {
		out := mustCall(t, s, "read_document", `{"path":"a/bad.level.json"}`)
		problems := out["problems"].([]any)
		var all []string
		for _, p := range problems {
			all = append(all, p.(string))
		}
		text := strings.Join(all, "\n")
		for _, want := range []string{
			`object "x": demo:Crate: missing required component gfx:Transform`,
			`object "x": component demo:Health: $.HP: -1 is less than the minimum 0`,
			`$.Kind: 7 is not a value of the enum (0 Wood, 1 Metal)`,
			`object "x": component demo:Nope is not in the GOD`,
			`object "x": id used again (first by object #0)`,
			`object "x": unknown object type demo:Barrel`,
			`object "y": prefab "global/prefabs/missing.prefab.json" does not exist`,
		} {
			mustContain(t, text, want)
		}
		if out["valid"] != false {
			t.Error("valid = true")
		}
	})

	t.Run("refused paths", func(t *testing.T) {
		for _, tc := range []struct{ path, want string }{
			{"global/dot.png", "not a document"},
			{"../god/demo.god.json", "outside the asset root"},
			{"levels/none.level.json", "does not exist"},
		} {
			_, errText := callTool(t, s, "read_document", `{"path":"`+tc.path+`"}`)
			mustContain(t, errText, tc.want)
		}
	})
}

func TestReadDocumentChecks(t *testing.T) {
	dir := copyFixture(t)
	write := func(key, text string) {
		t.Helper()
		if err := os.WriteFile(filepath.Join(dir, "assets", filepath.FromSlash(key)), []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	write("global/prefabs/loop-a.prefab.json", `{"objects":[{"prefab":"global/prefabs/loop-b.prefab.json"}]}`)
	write("global/prefabs/loop-b.prefab.json", `{"objects":[{"prefab":"global/prefabs/loop-a.prefab.json"}]}`)
	write("global/prefabs/two.prefab.json", `{"objects":[{"group":"demo","class":"Crate"},{"group":"demo","class":"Crate"}]}`)
	write("a/c.level.json", `{"objects":[{"id":"l","prefab":"global/prefabs/loop-a.prefab.json"},`+
		`{"id":"w","group":"gfx","class":"Visual","prefab":"global/prefabs/crate.prefab.json"},{"id":"n"}]}`)
	write("a/d.level.json", `{"objects":[], "extra": 1}`)
	write("a/e.level.json", `{"objects":[]} {}`)
	s := newServer(t, dir)
	tests := []struct{ path, want string }{
		{"a/c.level.json", "prefab cycle: global/prefabs/loop-a.prefab.json -> global/prefabs/loop-b.prefab.json -> global/prefabs/loop-a.prefab.json"},
		{"a/c.level.json", `object "w": object gfx:Visual does not match prefab "global/prefabs/crate.prefab.json", which is demo:Crate`},
		{"a/c.level.json", `object "n": needs "group" and "class", or a "prefab"`},
		{"global/prefabs/two.prefab.json", "a prefab holds exactly one object, this one has 2"},
		{"a/d.level.json", `unknown field "extra"`},
		{"a/e.level.json", "data after the JSON document"},
	}
	for _, tc := range tests {
		out := mustCall(t, s, "read_document", `{"path":"`+tc.path+`"}`)
		var problems []string
		for _, p := range out["problems"].([]any) {
			problems = append(problems, p.(string))
		}
		mustContain(t, strings.Join(problems, "\n"), tc.want)
	}
}

func TestReadGetObject(t *testing.T) {
	s := newServer(t, fixture)

	out := mustCall(t, s, "get_object", `{"path":"a/a.level.json","id":"left"}`)
	if got, _ := json.Marshal(out["components"]); string(got) != `{"gfx:Transform":{"Pos":{"X":10,"Y":20}}}` {
		t.Errorf("components as written = %s", got)
	}

	out = mustCall(t, s, "get_object", `{"path":"a/a.level.json","id":"left","merged":true}`)
	comps := out["components"].(map[string]any)
	tr := comps["gfx:Transform"].(map[string]any)
	if pos, _ := json.Marshal(tr["Pos"]); string(pos) != `{"X":10,"Y":20}` || tr["Rot"] != 0.0 {
		t.Errorf("merged transform = %v", tr)
	}
	if h, _ := json.Marshal(comps["demo:Health"]); string(h) != `{"HP":3,"Kind":1}` {
		t.Errorf("health through the variant = %s", h)
	}
	if got, _ := json.Marshal(out["overridden"]); string(got) != `["gfx:Transform.Pos.X","gfx:Transform.Pos.Y"]` {
		t.Errorf("overridden = %s", got)
	}
	if out["type"] != "demo:Crate" {
		t.Errorf("type = %v", out["type"])
	}

	out = mustCall(t, s, "get_object", `{"path":"a/a.level.json","id":"bare","merged":true}`)
	if _, ok := out["components"].(map[string]any)["demo:Health"]; ok {
		t.Error("a null override did not remove the component")
	}
	if got, _ := json.Marshal(out["overridden"]); string(got) != `["demo:Health (removed)"]` {
		t.Errorf("overridden = %s", got)
	}

	out = mustCall(t, s, "get_object", `{"path":"a/a.level.json","index":2}`)
	if out["type"] != "demo:Crate" || out["id"] != "" {
		t.Errorf("by index = %v", out)
	}

	for _, tc := range []struct{ args, want string }{
		{`{"path":"a/a.level.json","id":"nope"}`, `no object with id "nope"`},
		{`{"path":"a/a.level.json","index":9}`, "index 9 is out of range: the document has 3 objects"},
		{`{"path":"a/a.level.json"}`, "give the object's id or index"},
		{`{"path":"a/a.level.json","id":"left","index":0}`, "not both"},
	} {
		_, errText := callTool(t, s, "get_object", tc.args)
		mustContain(t, errText, tc.want)
	}
}

// TestMergePatchFixture checks mergePatch against the cases god and
// the VSCode extension share.
func TestMergePatchFixture(t *testing.T) {
	data, err := os.ReadFile("../../../god/testdata/merge-patch.json")
	if err != nil {
		t.Fatal(err)
	}
	var fx struct {
		Cases []struct {
			Name              string
			Base, Patch, Want json.RawMessage
		}
	}
	if err := json.Unmarshal(data, &fx); err != nil {
		t.Fatal(err)
	}
	for _, c := range fx.Cases {
		got, err := mergeJSON(c.Base, c.Patch)
		if err != nil {
			t.Fatalf("%s: %v", c.Name, err)
		}
		g, _ := decodeJSON(got)
		w, _ := decodeJSON(c.Want)
		gs, _ := json.Marshal(g)
		ws, _ := json.Marshal(w)
		if string(gs) != string(ws) {
			t.Errorf("%s: got %s, want %s", c.Name, gs, ws)
		}
	}
}
