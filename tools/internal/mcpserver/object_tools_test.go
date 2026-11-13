package mcpserver

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"
)

// objectOf returns the object with id in the level as the document holds it.
func objectOf(t *testing.T, s any, dir, key, id string) map[string]any {
	t.Helper()
	var doc struct {
		Objects []map[string]any `json:"objects"`
	}
	if err := json.Unmarshal([]byte(readAsset(t, dir, key)), &doc); err != nil {
		t.Fatal(err)
	}
	for _, o := range doc.Objects {
		if o["id"] == id {
			return o
		}
	}
	t.Fatalf("no object %q in %s", id, key)
	return nil
}

func TestObjects(t *testing.T) {
	doc := `"path":"` + levelA + `"`

	t.Run("place_object writes what the editor's create writes", func(t *testing.T) {
		dir := copyFixture(t)
		s := newServer(t, dir)
		mustCall(t, s, "create_document", `{"path":"levels/new/new.level.json","name":"New"}`)
		key := "levels/new/new.level.json"
		// The same sequence as bridge.TestCreateGolden: a Visual exists, then
		// one more Visual and an AnimatedVisual are created at (30, 40).
		mustCall(t, s, "place_object", `{"path":"`+key+`","class":"gfx:Visual","x":0,"y":0}`)
		a := mustCall(t, s, "place_object", `{"path":"`+key+`","class":"gfx:Visual","x":30,"y":40}`)
		b := mustCall(t, s, "place_object", `{"path":"`+key+`","class":"gfx:AnimatedVisual","x":30,"y":40}`)
		data, err := os.ReadFile("../../../editor/bridge/testdata/create-golden.json")
		if err != nil {
			t.Fatal(err)
		}
		var golden []map[string]any
		if err := json.Unmarshal(data, &golden); err != nil {
			t.Fatal(err)
		}
		for i, id := range []string{a["id"].(string), b["id"].(string)} {
			if got := objectOf(t, nil, dir, key, id); !reflect.DeepEqual(got, golden[i]) {
				gj, _ := json.MarshalIndent(got, "", " ")
				wj, _ := json.MarshalIndent(golden[i], "", " ")
				t.Errorf("object %s =\n%s\nthe editor creates\n%s", id, gj, wj)
			}
		}
		if v := mustCall(t, s, "read_document", `{"path":"`+key+`"}`); v["valid"] != true {
			t.Errorf("problems: %v", v["problems"])
		}
	})

	t.Run("ids, layers and the appended text", func(t *testing.T) {
		dir := copyFixture(t)
		s := newServer(t, dir)
		before := readAsset(t, dir, levelA)
		out := mustCall(t, s, "place_object", `{`+doc+`,"class":"demo:Crate","x":5,"y":6.5}`)
		if out["id"] != "crate-1" {
			t.Errorf("id = %v", out["id"])
		}
		if got := readAsset(t, dir, levelA); !strings.HasPrefix(got, strings.TrimSuffix(before, "\n    }\n  ]\n}\n")+"\n    },\n    {\n") {
			t.Errorf("text before the new object changed:\n%s", got)
		}
		if mustCall(t, s, "place_object", `{`+doc+`,"class":"demo:Crate","x":0,"y":0}`)["id"] != "crate-2" {
			t.Error("second id is not crate-2")
		}
		mustCall(t, s, "place_object", `{`+doc+`,"class":"demo:Crate","x":0,"y":0,"id":"boss"}`)
		mustContain(t, mustFail(t, s, "place_object", `{`+doc+`,"class":"demo:Crate","x":0,"y":0,"id":"boss"}`), `id "boss" is already used`)
		mustContain(t, mustFail(t, s, "place_object", `{`+doc+`,"class":"demo:crate","x":0,"y":0}`), "did you mean demo:Crate")
		mustContain(t, mustFail(t, s, "place_object", `{`+doc+`,"x":0,"y":0}`), "class")
		mustContain(t, mustFail(t, s, "place_object", `{`+doc+`,"class":"demo:Crate","prefab":"global/prefabs/crate.prefab.json","x":0,"y":0}`), "not both")
		mustContain(t, mustFail(t, s, "place_object", `{`+doc+`,"class":"demo:Crate"}`), `missing required "x"`)

		// An explicit layer goes into the drawn components.
		mustCall(t, s, "place_object", `{`+doc+`,"class":"gfx:Visual","x":1,"y":2,"layer":7,"id":"v"}`)
		sprite := objectOf(t, nil, dir, levelA, "v")["components"].(map[string]any)["gfx:Sprite"].(map[string]any)
		if sprite["Layer"] != 7.0 {
			t.Errorf("sprite = %v", sprite)
		}
	})

	t.Run("a prefab instance overrides only the position", func(t *testing.T) {
		dir := copyFixture(t)
		s := newServer(t, dir)
		out := mustCall(t, s, "place_object", `{`+doc+`,"prefab":"global/prefabs/crate.prefab.json","x":12,"y":0}`)
		if out["id"] != "crate-1" {
			t.Errorf("id = %v", out["id"])
		}
		obj := objectOf(t, nil, dir, levelA, "crate-1")
		want := `{"components":{"gfx:Transform":{"Pos":{"X":12},"PrevPos":{"X":12}}},"id":"crate-1","prefab":"global/prefabs/crate.prefab.json"}`
		if got, _ := json.Marshal(obj); string(got) != want {
			t.Errorf("instance = %s\nwant %s", got, want)
		}
		// A position equal to the prefab's writes no override.
		mustCall(t, s, "place_object", `{`+doc+`,"prefab":"global/prefabs/crate.prefab.json","x":0,"y":0,"id":"home"}`)
		if got, _ := json.Marshal(objectOf(t, nil, dir, levelA, "home")); string(got) != `{"components":{},"id":"home","prefab":"global/prefabs/crate.prefab.json"}` {
			t.Errorf("instance = %s", got)
		}
		mustContain(t, mustFail(t, s, "place_object", `{`+doc+`,"prefab":"global/prefabs/none.prefab.json","x":0,"y":0}`), "does not exist")
		mustContain(t, mustFail(t, s, "place_object", `{`+doc+`,"prefab":"global/dot.png","x":0,"y":0}`), "not a *.prefab.json")
		// A prefab holds one object.
		mustContain(t, mustFail(t, s, "place_object", `{"path":"global/prefabs/crate.prefab.json","class":"demo:Crate","x":0,"y":0}`), "exactly one object")
	})

	t.Run("remove_object refuses a referenced id", func(t *testing.T) {
		dir := copyFixture(t)
		s := newServer(t, dir)
		before := readAsset(t, dir, levelA)
		mustCall(t, s, "add_component", `{`+doc+`,"id":"bare","component":"refs:Follow","value":{"Target":"left","Others":[null,"left"],"Icon":"","Sound":"","Path":[]}}`)
		msg := mustFail(t, s, "remove_object", `{`+doc+`,"id":"left"}`)
		mustContain(t, msg, `object "bare" component refs:Follow $.Target`)
		mustContain(t, msg, `$.Others[1]`)
		mustCall(t, s, "remove_object", `{`+doc+`,"id":"bare"}`)
		mustCall(t, s, "remove_object", `{`+doc+`,"id":"left"}`)
		mustCall(t, s, "remove_object", `{`+doc+`,"index":0}`)
		if got := readAsset(t, dir, levelA); got != "{\n  \"name\": \"Level A\",\n  \"objects\": []\n}\n" {
			t.Errorf("file = %q (was %q)", got, before)
		}
		mustContain(t, mustFail(t, s, "remove_object", `{`+doc+`,"id":"left"}`), `no object with id "left"`)
	})
}
