package mcpserver

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDocWrite(t *testing.T) {
	t.Run("only the edited value changes", func(t *testing.T) {
		dir := copyFixture(t)
		s := newServer(t, dir)
		before := readAsset(t, dir, levelA)
		out := mustCall(t, s, "set_field", `{"path":"`+levelA+`","id":"left","component":"gfx:Transform","field":["Pos","X"],"value":11}`)
		want := strings.Replace(before, `"Pos": {"X": 10, "Y": 20}`, `"Pos": {"X": 11, "Y": 20}`, 1)
		if got := readAsset(t, dir, levelA); got != want {
			t.Errorf("file =\n%s\nwant\n%s", got, want)
		}
		if out["changed"] != true || out["sha"] != sha([]byte(want)) {
			t.Errorf("result = %v", out)
		}
		mustContain(t, out["diff"].(string), `+           "Pos": {"X": 11, "Y": 20}`)
	})

	t.Run("a stale sha is refused", func(t *testing.T) {
		dir := copyFixture(t)
		s := newServer(t, dir)
		read := mustCall(t, s, "read_document", `{"path":"`+levelA+`"}`)
		mustCall(t, s, "set_field", `{"path":"`+levelA+`","id":"left","component":"gfx:Transform","field":["Pos","X"],"value":1}`)
		after := readAsset(t, dir, levelA)
		msg := mustFail(t, s, "set_field", `{"path":"`+levelA+`","id":"left","component":"gfx:Transform","field":["Pos","X"],"value":2,"expect":"`+read["sha"].(string)+`"}`)
		mustContain(t, msg, "changed since it was read")
		if readAsset(t, dir, levelA) != after {
			t.Error("a refused edit changed the file")
		}
		// With the current sha it goes through.
		cur := mustCall(t, s, "read_document", `{"path":"`+levelA+`","include_text":false}`)
		mustCall(t, s, "set_field", `{"path":"`+levelA+`","id":"left","component":"gfx:Transform","field":["Pos","X"],"value":2,"expect":"`+cur["sha"].(string)+`"}`)
	})

	t.Run("an invalid result is not written", func(t *testing.T) {
		dir := copyFixture(t)
		s := newServer(t, dir)
		mustCall(t, s, "add_component", `{"path":"`+levelA+`","index":2,"component":"demo:Health"}`)
		before := readAsset(t, dir, levelA)
		for args, want := range map[string]string{
			`"field":["HP"],"value":-1`:      "-1 is less than the minimum 0",
			`"field":["Nope"],"value":1`:     "Nope",
			`"field":["HP"],"value":"three"`: "$.HP",
			`"field":["Kind"],"value":7`:     "$.Kind",
		} {
			msg := mustFail(t, s, "set_field", `{"path":"`+levelA+`","index":2,"component":"demo:Health",`+args+`}`)
			mustContain(t, msg, "nothing was written")
			mustContain(t, msg, want)
		}
		if readAsset(t, dir, levelA) != before {
			t.Error("a refused edit changed the file")
		}
		if entries, _ := filepath.Glob(filepath.Join(dir, "assets", "levels", "a", ".*.tmp")); len(entries) != 0 {
			t.Errorf("temporary files left: %v", entries)
		}
	})

	t.Run("an edit that adds no problem is written to a document that has some", func(t *testing.T) {
		dir := copyFixture(t)
		s := newServer(t, dir)
		out := mustCall(t, s, "set_field", `{"path":"a/bad.level.json","index":0,"component":"demo:Health","field":["HP"],"value":1}`)
		problems := out["problems"].([]any)
		for _, p := range problems {
			if strings.Contains(p.(string), "$.HP") {
				t.Errorf("fixed problem still reported: %v", p)
			}
		}
		if len(problems) == 0 {
			t.Error("the other problems of bad.level.json were dropped from the result")
		}
	})

	t.Run("entity references and asset keys are checked", func(t *testing.T) {
		dir := copyFixture(t)
		s := newServer(t, dir)
		add := func(value string) string {
			return mustFail(t, s, "add_component", `{"path":"`+levelA+`","id":"left","component":"refs:Follow","value":`+value+`}`)
		}
		good := `{"Target":"bare","Others":["left",null],"Icon":"global/dot.png","Sound":"","Path":[1,2]}`
		mustContain(t, add(strings.Replace(good, `"bare"`, `"ghost"`, 1)), `$.Target: references object "ghost"`)
		mustContain(t, add(strings.Replace(good, `"left"`, `"ghost"`, 1)), `$.Others[0]: references object "ghost"`)
		mustContain(t, add(strings.Replace(good, `global/dot.png`, `global/none.png`, 1)), `does not exist`)
		mustContain(t, add(strings.Replace(good, `"Sound":""`, `"Sound":"global/dot.png"`, 1)), `is a texture, the field takes a sound`)
		mustCall(t, s, "add_component", `{"path":"`+levelA+`","id":"left","component":"refs:Follow","value":`+good+`}`)
		doc := mustCall(t, s, "read_document", `{"path":"`+levelA+`"}`)
		if doc["valid"] != true {
			t.Errorf("problems: %v", doc["problems"])
		}
	})

	t.Run("the file mode and other files survive", func(t *testing.T) {
		dir := copyFixture(t)
		s := newServer(t, dir)
		f := filepath.Join(dir, "assets", levelA)
		if err := os.Chmod(f, 0o600); err != nil {
			t.Fatal(err)
		}
		mustCall(t, s, "set_field", `{"path":"`+levelA+`","id":"left","component":"gfx:Transform","field":["Pos","X"],"value":5}`)
		if fi, _ := os.Stat(f); fi.Mode().Perm() != 0o600 {
			t.Errorf("mode = %v", fi.Mode())
		}
	})

	t.Run("an edit that changes nothing writes nothing", func(t *testing.T) {
		dir := copyFixture(t)
		s := newServer(t, dir)
		out := mustCall(t, s, "set_field", `{"path":"`+levelA+`","id":"left","component":"gfx:Transform","field":["Pos","X"],"value":10}`)
		if out["changed"] != false {
			t.Errorf("result = %v", out)
		}
	})

	t.Run("paths outside the asset root and other kinds are refused", func(t *testing.T) {
		s := newServer(t, copyFixture(t))
		mustContain(t, mustFail(t, s, "set_field", `{"path":"../x.level.json","index":0,"component":"a:B","field":["x"],"value":1}`), "outside")
		mustContain(t, mustFail(t, s, "set_field", `{"path":"global/dot.png","index":0,"component":"a:B","field":["x"],"value":1}`), "not a document")
	})
}
