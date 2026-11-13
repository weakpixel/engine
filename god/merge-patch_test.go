package god

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
)

// TestMergePatch runs the shared cases of testdata/merge-patch.json (the
// VSCode extension runs the same file against its own merge).
func TestMergePatch(t *testing.T) {
	data, err := os.ReadFile("testdata/merge-patch.json")
	if err != nil {
		t.Fatal(err)
	}
	var fixture struct {
		Cases []struct {
			Name              string
			Base, Patch, Want json.RawMessage
		}
	}
	if err := json.Unmarshal(data, &fixture); err != nil {
		t.Fatal(err)
	}
	for _, tc := range fixture.Cases {
		t.Run(tc.Name, func(t *testing.T) {
			got, err := mergePatch(tc.Base, tc.Patch)
			if err != nil {
				t.Fatal(err)
			}
			if eq, _ := jsonEqual(got, tc.Want); !eq {
				t.Fatalf("got %s, want %s", got, tc.Want)
			}
		})
	}
	// mergePatch compacts.
	if got, _ := mergePatch(json.RawMessage(`{"A":1}`), json.RawMessage(`{ "A" : 2 }`)); string(got) != `{"A":2}` {
		t.Errorf("not compacted: %s", got)
	}
}

func TestDiffPatch(t *testing.T) {
	s := &TypeSchema{Type: "object", Properties: map[string]*TypeSchema{
		"P":    {Type: "object", Properties: map[string]*TypeSchema{"X": {Type: "number"}, "Y": {Type: "number"}}},
		"Name": {Type: "string"},
		"Ptr":  {Type: "object", Nullable: true},
		"L":    {Type: "array", Items: &TypeSchema{Type: "integer"}},
		"M":    {Type: "object", AdditionalProperties: &TypeSchema{Type: "integer"}},
	}}
	for _, tc := range []struct {
		name, base, value, want string
		changed                 bool
	}{
		{"equal", `{"P":{"X":1,"Y":2},"Name":"a"}`, `{"Name":"a","P":{"Y":2,"X":1}}`, ``, false},
		{"equal numbers by value", `{"P":{"X":1.0,"Y":2}}`, `{"P":{"X":1,"Y":2}}`, ``, false},
		{"one nested field", `{"P":{"X":1,"Y":2},"Name":"a"}`, `{"P":{"X":1,"Y":7},"Name":"a"}`, `{"P":{"Y":7}}`, true},
		{"array whole", `{"L":[1,2]}`, `{"L":[1,3]}`, `{"L":[1,3]}`, true},
		{"new key", `{"Name":"a"}`, `{"Name":"a","L":[1]}`, `{"L":[1]}`, true},
		{"omitted string is empty", `{"Name":"a"}`, `{}`, `{"Name":""}`, true},
		{"omitted nullable is null", `{"Ptr":{"A":1}}`, `{}`, `{"Ptr":null}`, true},
		{"omitted zero already zero", `{"Name":""}`, `{}`, ``, false},
		{"map entries merge", `{"M":{"a":1,"b":2}}`, `{"M":{"a":1,"b":3}}`, `{"M":{"b":3}}`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, changed, err := diffPatch(s, json.RawMessage(tc.base), json.RawMessage(tc.value))
			if err != nil || changed != tc.changed || string(got) != tc.want {
				t.Fatalf("got %s, %v, %v; want %s, %v", got, changed, err, tc.want, tc.changed)
			}
			if !changed {
				return
			}
			// Merging the diff back gives the value (up to omitted zeros).
			merged, err := mergePatch(json.RawMessage(tc.base), got)
			if err != nil {
				t.Fatal(err)
			}
			again, changed, err := diffPatch(s, merged, json.RawMessage(tc.value))
			if err != nil || changed {
				t.Fatalf("merge(base, diff) = %s differs from value: %s", merged, again)
			}
		})
	}

	_, _, err := diffPatch(s, json.RawMessage(`{"M":{"a":1,"b":2}}`), json.RawMessage(`{"M":{"a":1}}`))
	if err == nil || !strings.Contains(err.Error(), "$.M.b: a map entry of the prefab cannot be removed") {
		t.Fatalf("removed map entry: %v", err)
	}
}
