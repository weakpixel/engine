package mcpserver

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// q quotes s as a JSON string.
func q(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

func TestAssets(t *testing.T) {
	doc := `"path":"` + levelA + `"`

	t.Run("set_asset_field checks the key against the field", func(t *testing.T) {
		dir := copyFixture(t)
		s := newServer(t, dir)
		mustCall(t, s, "add_component", `{`+doc+`,"id":"left","component":"refs:Follow","value":{"Target":null,"Others":[],"Icon":"","Sound":"","Path":[]}}`)
		set := func(field, asset string) string {
			return `{` + doc + `,"id":"left","component":"refs:Follow","field":["` + field + `"],"asset":` + q(asset) + `}`
		}
		mustCall(t, s, "set_asset_field", set("Icon", "global/dot.png"))
		mustContain(t, readAsset(t, dir, levelA), `"Icon": "global/dot.png"`)
		mustCall(t, s, "set_asset_field", set("Icon", "global/dot.png#frame-2"))
		before := readAsset(t, dir, levelA)
		mustContain(t, mustFail(t, s, "set_asset_field", set("Sound", "global/dot.png")), "is a texture, the field takes a sound")
		mustContain(t, mustFail(t, s, "set_asset_field", set("Icon", "global/none.png")), "does not exist")
		mustContain(t, mustFail(t, s, "set_asset_field", set("Icon", "../../go.mod")), "outside")
		mustContain(t, mustFail(t, s, "set_asset_field", set("Target", "global/dot.png")), "not an asset field")
		mustContain(t, mustFail(t, s, "set_asset_field", set("Nope", "global/dot.png")), "no field")
		if readAsset(t, dir, levelA) != before {
			t.Error("a refused edit changed the file")
		}
		mustCall(t, s, "set_asset_field", set("Icon", ""))
		mustContain(t, readAsset(t, dir, levelA), `"Icon": ""`)
	})

	t.Run("create_document", func(t *testing.T) {
		dir := copyFixture(t)
		s := newServer(t, dir)
		out := mustCall(t, s, "create_document", `{"path":"levels/b/deep/b.level.json","name":"B"}`)
		if out["text"] != "{\n  \"name\": \"B\",\n  \"objects\": []\n}\n" {
			t.Errorf("text = %q", out["text"])
		}
		if readAsset(t, dir, "levels/b/deep/b.level.json") != out["text"] {
			t.Error("file differs from the result")
		}
		mustCall(t, s, "create_document", `{"path":"levels/c.level.json"}`)
		mustContain(t, mustFail(t, s, "create_document", `{"path":"levels/b/deep/b.level.json"}`), "already exists")
		mustContain(t, mustFail(t, s, "create_document", `{"path":"global/prefabs/p.prefab.json"}`), "exactly one object")
		mustContain(t, mustFail(t, s, "create_document", `{"path":"global/prefabs/p.prefab.json","class":"demo:Nope"}`), "unknown object type")
		mustContain(t, mustFail(t, s, "create_document", `{"path":"levels/x.level.json","class":"demo:Crate"}`), "place_object")
		mustContain(t, mustFail(t, s, "create_document", `{"path":"levels/x.png"}`), "not a document")
		mustContain(t, mustFail(t, s, "create_document", `{"path":"../x.level.json"}`), "outside")

		mustCall(t, s, "create_document", `{"path":"global/prefabs/p.prefab.json","class":"demo:Crate"}`)
		doc := mustCall(t, s, "read_document", `{"path":"global/prefabs/p.prefab.json"}`)
		if doc["valid"] != true || doc["name"] != "p" {
			t.Errorf("prefab = %v", doc)
		}
		// The new prefab can be instantiated.
		mustCall(t, s, "place_object", `{`+doc2(levelA)+`,"prefab":"global/prefabs/p.prefab.json","x":1,"y":1}`)
	})

	t.Run("write_asset_document validates by kind", func(t *testing.T) {
		dir := copyFixture(t)
		s := newServer(t, dir)
		write := func(key, text string) string {
			return `{"path":` + q(key) + `,"text":` + q(text) + `}`
		}
		// Levels and prefabs go through the offline checks.
		level := "{\n  \"objects\": []\n}\n"
		out := mustCall(t, s, "write_asset_document", write("levels/n.level.json", level))
		if out["created"] != true || readAsset(t, dir, "levels/n.level.json") != level {
			t.Errorf("result = %v", out)
		}
		before := readAsset(t, dir, levelA)
		msg := mustFail(t, s, "write_asset_document", write(levelA, `{"objects":[{"id":"x","group":"demo","class":"Crate"}]}`))
		mustContain(t, msg, "nothing was written")
		mustContain(t, msg, "missing required component gfx:Transform")
		if readAsset(t, dir, levelA) != before {
			t.Error("a refused write changed the file")
		}
		mustContain(t, mustFail(t, s, "write_asset_document", write("levels/x.level.json", "{")), "invalid")
		// A stale sha is refused, the current one accepted.
		stale := `{"path":"levels/n.level.json","text":` + q(level) + `,"expect":"00"}`
		mustContain(t, mustFail(t, s, "write_asset_document", stale), "changed since it was read")
		cur := `{"path":"levels/n.level.json","text":` + q("{\n  \"name\": \"N\",\n  \"objects\": []\n}\n") + `,"expect":"` + sha([]byte(level)) + `"}`
		if out := mustCall(t, s, "write_asset_document", cur); out["changed"] != true || !strings.Contains(out["diff"].(string), `+   "name": "N",`) {
			t.Errorf("result = %v", out)
		}

		// Sheets and animation definitions use the engine's decoders.
		mustCall(t, s, "write_asset_document", write("global/s.sheet.json", `{"image":"dot.png","frameW":4,"frameH":4}`))
		mustContain(t, mustFail(t, s, "write_asset_document", write("global/bad.sheet.json", `{"image":"dot.png","frameW":0,"frameH":4}`)), "frameW and frameH must be positive")
		sheet := `{"frames":[{"filename":"a 0","frame":{"x":0,"y":0,"w":4,"h":4},"rotated":false,"trimmed":false,` +
			`"spriteSourceSize":{"x":0,"y":0,"w":4,"h":4},"sourceSize":{"w":4,"h":4},"duration":100}],` +
			`"meta":{"size":{"w":4,"h":4},"frameTags":[{"name":"Idle","from":0,"to":0,"direction":"forward"}]}}`
		if err := os.WriteFile(filepath.Join(dir, "assets", "global", "a.aseprite.json"), []byte(sheet), 0o644); err != nil {
			t.Fatal(err)
		}
		mustCall(t, s, "write_asset_document", write("global/a.anim.json", `{"version":1,"sheet":"a.aseprite.json","clips":{"Idle":{"loop":true,"fps":8}}}`))
		mustContain(t, mustFail(t, s, "write_asset_document", write("global/b.anim.json", `{"version":1,"sheet":"a.aseprite.json","clips":{"Nope":{"fps":8}}}`)), "Nope")
		mustContain(t, mustFail(t, s, "write_asset_document", write("global/c.anim.json", `{"version":1,"sheet":"missing.aseprite.json"}`)), "cannot be read")

		// Particle effects use the engine's decoder and check their sprite and shader keys.
		if err := os.WriteFile(filepath.Join(dir, "assets", "global", "glow.kage"), []byte("//kage:unit pixels\npackage main\n"), 0o644); err != nil {
			t.Fatal(err)
		}
		fx := "{\"emitters\":[{\"sprite\":\"global/dot.png\",\"shader\":\"global/glow.kage\",\"rate\":10}]}"
		out = mustCall(t, s, "write_asset_document", write("global/fx/sparks.fx.json", fx))
		if out["created"] != true || out["sha"] != sha([]byte(fx)) || readAsset(t, dir, "global/fx/sparks.fx.json") != fx {
			t.Errorf("result = %v", out)
		}
		fx2 := strings.Replace(fx, `"rate":10`, `"rate":20`, 1)
		mustContain(t, mustFail(t, s, "write_asset_document", `{"path":"global/fx/sparks.fx.json","text":`+q(fx2)+`,"expect":"00"}`), "changed since it was read")
		mustCall(t, s, "write_asset_document", `{"path":"global/fx/sparks.fx.json","text":`+q(fx2)+`,"expect":"`+sha([]byte(fx))+`"}`)
		msg = mustFail(t, s, "write_asset_document", write("global/x.fx.json", `{"emitters":[{"blend":"glow"},{"colour":"#fff"}]}`))
		mustContain(t, msg, "emitters[0].blend")
		mustContain(t, msg, "emitters[1]")
		mustContain(t, msg, "colour")
		mustContain(t, mustFail(t, s, "write_asset_document", write("global/x.fx.json", `{"emitters":[{"sprite":"global/none.png"}]}`)), "emitters[0].sprite: asset \"global/none.png\" does not exist")
		msg = mustFail(t, s, "write_asset_document", write("global/x.fx.json", `{"emitters":[{"sprite":"global/glow.kage","shader":"global/dot.png"}]}`))
		mustContain(t, msg, "emitters[0].sprite: asset \"global/glow.kage\" is a shader")
		mustContain(t, msg, "emitters[0].shader: asset \"global/dot.png\" is a texture")

		// Nothing else is written.
		mustContain(t, mustFail(t, s, "write_asset_document", write("global/x.png", "png")), "cannot be written")
		mustContain(t, mustFail(t, s, "write_asset_document", write("../x.level.json", level)), "outside")
		mustContain(t, mustFail(t, s, "write_asset_document", write("notes.txt", "x")), "cannot be written")
		for _, f := range []string{"global/x.png", "global/x.fx.json", "global/c.anim.json", "global/bad.sheet.json"} {
			if _, err := os.Stat(filepath.Join(dir, "assets", filepath.FromSlash(f))); err == nil {
				t.Errorf("%s was written", f)
			}
		}
	})
}

// doc2 is the "path" argument for key.
func doc2(key string) string { return `"path":"` + key + `"` }
