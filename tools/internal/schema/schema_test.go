package schema

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/weakpixel/engine/god"
	"github.com/weakpixel/engine/project"
	testassets "github.com/weakpixel/engine/testdata/assets"
	"github.com/weakpixel/engine/tools/internal/goddoc"
)

// fixtureGOD holds two hand-kept GOD files (a game package with objects and
// a level package): the GOD of a project is build output, not in git.
const fixtureGOD = "testdata/god"

func asMap(t *testing.T, v any) map[string]any {
	t.Helper()
	data, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		t.Fatal(err)
	}
	return m
}

func TestToJSONSchema(t *testing.T) {
	in := &god.TypeSchema{
		Type: "object",
		Properties: map[string]*god.TypeSchema{
			"Kind":  {Type: "integer", Nullable: true, XEnum: []god.EnumValue{{Value: 0, Name: "Point", Doc: "one"}, {Value: 1, Name: "Ring"}}},
			"Tmp":   {Type: "number", XRuntime: true},
			"Image": {Type: "string", Format: "asset", XAsset: "sprite"},
			"Any":   {PreserveUnknownFields: true},
			"Inner": {Type: "object", Properties: map[string]*god.TypeSchema{"X": {Type: "number"}}, Required: []string{"X"}},
		},
		Required: []string{"Kind", "Tmp", "Inner"},
	}
	got := asMap(t, toJSONSchema(in, false))
	want := `{
	"type": "object", "additionalProperties": false, "required": ["Inner", "Kind"],
	"properties": {
		"Kind": {"type": ["integer", "null"], "enum": [0, 1, null], "enumDescriptions": ["Point: one", "Ring", "null"]},
		"Image": {"type": "string", "x-asset": "sprite"},
		"Any": {},
		"Inner": {"type": "object", "additionalProperties": false, "required": ["X"], "properties": {"X": {"type": "number"}}}
	}}`
	var wantM map[string]any
	if err := json.Unmarshal([]byte(want), &wantM); err != nil {
		t.Fatal(err)
	}
	if g, w := marshal(t, got), marshal(t, wantM); g != w {
		t.Errorf("got\n%s\nwant\n%s", g, w)
	}
	patch := marshal(t, asMap(t, toJSONSchema(in, true)))
	if strings.Contains(patch, `"required"`) {
		t.Errorf("patch form keeps required:\n%s", patch)
	}
}

func marshal(t *testing.T, v any) string {
	t.Helper()
	data, err := json.MarshalIndent(v, "", " ")
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// TestKindsAcceptSamples checks the fixed shapes against complete files of
// the test library and rejects typical mistakes.
func TestKindsAcceptSamples(t *testing.T) {
	shape := map[string]*god.TypeSchema{}
	for _, k := range kinds() {
		if k.build == nil {
			continue
		}
		s, err := k.build()
		if err != nil {
			t.Fatalf("%s: %v", k.name, err)
		}
		shape[k.name] = s
	}
	lib := testassets.Lib()
	for name, file := range map[string]string{
		"anim":     "global/hero/hero.anim.json",
		"bindings": "global/input/hero.bindings.json",
	} {
		if err := shape[name].Validate(lib[file].Data); err != nil {
			t.Errorf("%s: %v", file, err)
		}
	}
	for _, tc := range []struct{ kind, doc string }{
		{"sheet", `{"image": "a.png", "frameW": 16, "frameH": 16, "margin": 1, "spacing": 2, "pivot": {"x": 0.5, "y": 1}}`},
		{"library", `{"entries": {"global": ["global/fonts/ui.ttf#16"]}}`},
		{"weakpixel", `{"god": {"specs": "god", "out": "o.json"}, "schema": {"out": "s"}, "assets": {"root": "assets"}, "editor": {"tags": ["editor"], "snap": 8, "screenshots": "shots", "prefab": {"grid": 16}}}`},
	} {
		if err := shape[tc.kind].Validate([]byte(tc.doc)); err != nil {
			t.Errorf("%s: %v", tc.kind, err)
		}
	}
	for _, tc := range []struct{ kind, doc string }{
		{"anim", `{"version": 1}`},
		{"anim", `{"version": 2, "sheet": "a.json"}`},
		{"sheet", `{"image": "a.png", "frameW": 0, "frameH": 16}`},
		{"weakpixel", `{"god": {"spec": "god"}}`},
		{"weakpixel", `{"editor": {"snap": -1}}`},
	} {
		if err := shape[tc.kind].Validate([]byte(tc.doc)); err == nil {
			t.Errorf("%s accepts %s", tc.kind, tc.doc)
		}
	}
}

// TestProjectShapeCoversKeys keeps the .weakpixel.json schema in step with
// the keys package project accepts.
func TestProjectShapeCoversKeys(t *testing.T) {
	s, err := projectShape()
	if err != nil {
		t.Fatal(err)
	}
	var walk func(path string, s *god.TypeSchema)
	seen := map[string]bool{}
	walk = func(path string, s *god.TypeSchema) {
		seen[path] = true
		var got []string
		for k, p := range s.Properties {
			got = append(got, k)
			if p.Type == "object" {
				sub := k
				if path != "" {
					sub = path + "." + k
				}
				walk(sub, p)
			}
		}
		slices.Sort(got)
		want := slices.Clone(project.Keys()[path])
		slices.Sort(want)
		if !slices.Equal(got, want) {
			t.Errorf("keys at %q: schema %v, project %v", path, got, want)
		}
	}
	walk("", s)
	for path := range project.Keys() {
		if !seen[path] {
			t.Errorf("project object %q is not in the schema", path)
		}
	}
}

func TestDocumentSchema(t *testing.T) {
	comps, objects, err := goddoc.Load([]string{fixtureGOD})
	if err != nil {
		t.Fatal(err)
	}
	level := asMap(t, documentSchema(false, comps, objects))
	prefab := asMap(t, documentSchema(true, comps, objects))

	defs := level["definitions"].(map[string]any)
	for _, c := range comps {
		for _, prefix := range []string{"component.", "override."} {
			if _, ok := defs[prefix+c.Key()]; !ok {
				t.Errorf("no definition %s%s", prefix, c.Key())
			}
		}
	}
	if strings.Contains(marshal(t, defs["override.game:Weapon"]), `"required"`) {
		t.Error("override definition keeps required")
	}
	if !strings.Contains(marshal(t, defs["component.game:Weapon"]), `"required"`) {
		t.Error("component definition lost required")
	}

	objectsOf := func(doc map[string]any) map[string]any {
		return doc["properties"].(map[string]any)["objects"].(map[string]any)
	}
	if _, ok := objectsOf(level)["maxItems"]; ok {
		t.Error("a level is limited to one object")
	}
	if objectsOf(prefab)["maxItems"] != float64(1) {
		t.Error("a prefab holds exactly one object")
	}
	entry := objectsOf(level)["items"].(map[string]any)
	props := entry["properties"].(map[string]any)
	if got := props["group"].(map[string]any)["enum"].([]any); !slices.Contains(got, any("game")) {
		t.Errorf("group enum %v", got)
	}
	comp := props["components"].(map[string]any)
	if comp["additionalProperties"] != false || len(comp["properties"].(map[string]any)) != len(comps) {
		t.Errorf("components: %v keys, additionalProperties %v", len(comp["properties"].(map[string]any)), comp["additionalProperties"])
	}
	// The Hero class requires its non-optional components, not the optional Weapon.
	rules := marshal(t, entry["allOf"])
	if !strings.Contains(rules, `"game:Hero"`) && !strings.Contains(rules, `"Hero"`) {
		t.Errorf("no rule for class Hero:\n%s", rules)
	}
}

func TestRunWritesFiles(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "out")
	var stdout bytes.Buffer
	if err := Run(Options{Out: dir, Inputs: []string{fixtureGOD}}, &stdout); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"anim", "sheet", "atlas", "bindings", "library", "weakpixel", "level", "prefab"} {
		data, err := os.ReadFile(filepath.Join(dir, name+".schema.json"))
		if err != nil {
			t.Fatal(err)
		}
		var doc map[string]any
		if err := json.Unmarshal(data, &doc); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if doc["$schema"] != jsonSchemaURL || doc["title"] == "" {
			t.Errorf("%s: $schema %v, title %v", name, doc["$schema"], doc["title"])
		}
	}

	// Without GOD files the fixed kinds are still written.
	empty := t.TempDir()
	dir2 := filepath.Join(t.TempDir(), "out")
	stdout.Reset()
	if err := Run(Options{Out: dir2, Inputs: []string{empty}}, &stdout); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir2, "level.schema.json")); err == nil {
		t.Error("level schema written without GOD files")
	}
	if !strings.Contains(stdout.String(), "level, prefab: skipped") {
		t.Errorf("no skip note:\n%s", stdout.String())
	}
	if _, err := os.Stat(filepath.Join(dir2, "anim.schema.json")); err != nil {
		t.Error(err)
	}
}

func TestRunUsesSchemaOut(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, project.FileName), []byte(`{"schema": {"out": "gen/schemas"}}`), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	if err := Run(Options{}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dir, "gen", "schemas", "anim.schema.json")); err != nil {
		t.Error(err)
	}
}
