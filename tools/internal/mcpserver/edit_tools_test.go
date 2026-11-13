package mcpserver

import (
	"strings"
	"testing"
)

func TestEditComponents(t *testing.T) {
	doc := `"path":"` + levelA + `"`

	t.Run("set_component and set_field on a plain object", func(t *testing.T) {
		dir := copyFixture(t)
		s := newServer(t, dir)
		before := readAsset(t, dir, levelA)
		mustCall(t, s, "set_component", `{`+doc+`,"index":2,"component":"gfx:Transform","value":{"Pos":{"X":3,"Y":4},"PrevPos":{"X":3,"Y":4},"Rot":0,"Scale":{"X":1,"Y":1}}}`)
		// A multi-line value is replaced by a multi-line one in the document's indentation.
		cut := strings.Index(before, `"Pos": {"X": 1, "Y": 2}`)
		got := readAsset(t, dir, levelA)
		if !strings.HasPrefix(got, before[:cut]) || !strings.HasSuffix(got, "\n        }\n      }\n    }\n  ]\n}\n") {
			t.Errorf("text around the value changed:\n%s", got)
		}
		mustContain(t, got, "\"Pos\": {\n            \"X\": 3,\n            \"Y\": 4\n          },")
		mustCall(t, s, "set_field", `{`+doc+`,"index":2,"component":"gfx:Transform","field":["Rot"],"value":1.5}`)
		if after := readAsset(t, dir, levelA); after != strings.Replace(got, `"Rot": 0`, `"Rot": 1.5`, 1) {
			t.Errorf("set_field:\n%s", after)
		}
		mustContain(t, mustFail(t, s, "set_component", `{`+doc+`,"index":2,"component":"demo:Health","value":{"HP":1,"Kind":0}}`), "add_component")
		mustContain(t, mustFail(t, s, "set_field", `{`+doc+`,"id":"left","component":"gfx:Transform"}`), "field")
		mustContain(t, mustFail(t, s, "set_field", `{`+doc+`,"id":"ghost","component":"gfx:Transform","field":["Rot"],"value":1}`), `no object with id "ghost"`)
	})

	t.Run("add_component uses the initial value", func(t *testing.T) {
		dir := copyFixture(t)
		s := newServer(t, dir)
		mustCall(t, s, "add_component", `{`+doc+`,"index":2,"component":"demo:Health"}`)
		got := readAsset(t, dir, levelA)
		mustContain(t, got, `"demo:Health": {
          "HP": 3,
          "Kind": 0
        }`)
		mustContain(t, mustFail(t, s, "add_component", `{`+doc+`,"index":2,"component":"demo:Health"}`), "already has demo:Health")
		mustContain(t, mustFail(t, s, "add_component", `{`+doc+`,"index":2,"component":"demo:health"}`), "did you mean demo:Health")
		// A value is used as given, and checked.
		mustContain(t, mustFail(t, s, "add_component", `{`+doc+`,"id":"left","component":"refs:Follow","value":{"Target":null}}`), "nothing was written")
		// "bare" removed Health with null; adding it writes the value over the null.
		mustCall(t, s, "add_component", `{`+doc+`,"id":"bare","component":"demo:Health"}`)
		mustContain(t, readAsset(t, dir, levelA), `"id": "bare"`)
		if strings.Contains(readAsset(t, dir, levelA), `"demo:Health": null`) {
			t.Error("the null override is still there")
		}
	})

	t.Run("remove_component", func(t *testing.T) {
		dir := copyFixture(t)
		s := newServer(t, dir)
		before := readAsset(t, dir, levelA)
		mustContain(t, mustFail(t, s, "remove_component", `{`+doc+`,"index":2,"component":"gfx:Transform"}`), "required component of demo:Crate")
		mustContain(t, mustFail(t, s, "remove_component", `{`+doc+`,"index":2,"component":"demo:Health"}`), "has no component")
		// A plain object loses the component's entry.
		mustCall(t, s, "add_component", `{`+doc+`,"index":2,"component":"demo:Health"}`)
		mustCall(t, s, "remove_component", `{`+doc+`,"index":2,"component":"demo:Health"}`)
		if got := readAsset(t, dir, levelA); got != before {
			t.Errorf("add then remove did not restore the file:\n%s", got)
		}
		// An instance removes a prefab's component with null ...
		mustCall(t, s, "remove_component", `{`+doc+`,"id":"left","component":"demo:Health"}`)
		mustContain(t, readAsset(t, dir, levelA), `"demo:Health": null`)
		if v := mustCall(t, s, "read_document", `{`+doc+`}`); v["valid"] != true {
			t.Errorf("problems: %v", v["problems"])
		}
		// ... and just deletes a component only the instance added.
		mustCall(t, s, "add_component", `{`+doc+`,"id":"bare","component":"refs:Follow","value":{"Target":null,"Others":[],"Icon":"","Sound":"","Path":[]}}`)
		mustCall(t, s, "remove_component", `{`+doc+`,"id":"bare","component":"refs:Follow"}`)
		if strings.Contains(readAsset(t, dir, levelA), "refs:Follow") {
			t.Error("refs:Follow is still there")
		}
	})

	t.Run("overrides on a prefab instance", func(t *testing.T) {
		dir := copyFixture(t)
		s := newServer(t, dir)
		before := readAsset(t, dir, levelA)
		mustContain(t, mustFail(t, s, "set_override", `{`+doc+`,"index":2,"component":"gfx:Transform","field":["Rot"],"value":1}`), "not a prefab instance")
		mustContain(t, mustFail(t, s, "revert_override", `{`+doc+`,"index":2,"component":"gfx:Transform","field":["Rot"]}`), "not a prefab instance")

		// set_field on an instance is an override at the path.
		mustCall(t, s, "set_field", `{`+doc+`,"id":"left","component":"demo:Health","field":["HP"],"value":9}`)
		mustContain(t, readAsset(t, dir, levelA), `"demo:Health": {
          "HP": 9
        }`)
		got := mustCall(t, s, "get_object", `{`+doc+`,"id":"left","merged":true}`)
		if hp := got["components"].(map[string]any)["demo:Health"].(map[string]any)["HP"]; hp != 9.0 {
			t.Errorf("merged HP = %v", hp)
		}
		// Reverting removes the override and the entries only it held.
		mustCall(t, s, "revert_override", `{`+doc+`,"id":"left","component":"demo:Health","field":["HP"]}`)
		if got := readAsset(t, dir, levelA); got != before {
			t.Errorf("revert did not restore the file:\n%s", got)
		}
		mustContain(t, mustFail(t, s, "revert_override", `{`+doc+`,"id":"left","component":"demo:Health","field":["HP"]}`), "does not override demo:Health")
		mustContain(t, mustFail(t, s, "revert_override", `{`+doc+`,"id":"left","component":"gfx:Transform","field":["Rot"]}`), "gfx:Transform.Pos.X")

		// A whole-component revert, including a null.
		mustCall(t, s, "revert_override", `{`+doc+`,"id":"bare","component":"demo:Health"}`)
		if strings.Contains(readAsset(t, dir, levelA), `"demo:Health": null`) {
			t.Error("the null override was not reverted")
		}
	})

	t.Run("a field inside an array writes the whole array", func(t *testing.T) {
		dir := copyFixture(t)
		s := newServer(t, dir)
		// The prefab holds the array, the instance overrides one element.
		mustCall(t, s, "add_component", `{"path":"global/prefabs/crate.prefab.json","index":0,"component":"refs:Follow","value":{"Target":null,"Others":[],"Icon":"","Sound":"","Path":[1,2,3]}}`)
		mustCall(t, s, "set_field", `{`+doc+`,"id":"bare","component":"refs:Follow","field":["Path",1],"value":20}`)
		got := mustCall(t, s, "get_object", `{`+doc+`,"id":"bare"}`)
		follow := got["components"].(map[string]any)["refs:Follow"].(map[string]any)
		if p := follow["Path"].([]any); len(p) != 3 || p[0] != 1.0 || p[1] != 20.0 || p[2] != 3.0 {
			t.Errorf("override = %v", follow)
		}
		if len(follow) != 1 {
			t.Errorf("only the array should be overridden: %v", follow)
		}
		// Reverting a path inside the array reverts the array.
		mustCall(t, s, "revert_override", `{`+doc+`,"id":"bare","component":"refs:Follow","field":["Path",1]}`)
		if strings.Contains(readAsset(t, dir, levelA), "refs:Follow") {
			t.Error("refs:Follow override is still there")
		}
	})
}
