package editor

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/weakpixel/engine/god"
)

// editorGOD names the component editor:Stats, which no package registers.
const editorGOD = `{"version":0,"components":[{"group":"editor","name":"Stats","schema":{"type":"object",
"properties":{"HP":{"type":"integer"}},"required":["HP"]}}],"objects":[]}`

// TestDataOnlyDocument: a level object and an instance may hold a component
// only a GOD file names; its value is checked, and a key in no GOD file is
// still an error.
func TestDataOnlyDocument(t *testing.T) {
	if err := god.RegisterDataOnly(strings.NewReader(editorGOD)); err != nil {
		t.Fatal(err)
	}
	files := testFiles(t)
	files["stat.prefab.json"] = &fstest.MapFile{Data: []byte(doc(strings.Replace(visual("stat", 0, 0, 1), `"components":{`, `"components":{"editor:Stats":{"HP":5},`, 1)))}
	f := newFixtureFS(t, Config{}, files)
	f.open(doc(`{"id":"a","prefab":"stat.prefab.json","components":{"editor:Stats":{"HP":9}}}`))
	if err := f.waitApplied(); err != nil {
		t.Fatal(err)
	}
	for text, want := range map[string]string{
		doc(`{"id":"a","prefab":"stat.prefab.json","components":{"editor:Stats":{"HP":"x"}}}`):                    "component editor:Stats",
		doc(`{"id":"a","prefab":"stat.prefab.json","components":{"editor:Stat":{"HP":1}}}`):                       "component editor:Stat is not registered",
		strings.Replace(doc(visual("b", 0, 0, 0)), `"components":{`, `"components":{"editor:Stats":{"HP":1},`, 1): "",
	} {
		err := f.s.Apply([]byte(text))
		if (want == "") != (err == nil) || (want != "" && !strings.Contains(err.Error(), want)) {
			t.Errorf("%s: got %v, want %q", text, err, want)
		}
	}
}
