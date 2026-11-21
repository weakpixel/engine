package jsonedit

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

const level = `{
  "name": "L",
  "objects": [
    {
      "id": "a",
      "components": {
        "gfx:Transform": {"Pos": {"X": 1, "Y": 2}, "Rot": 0},
        "demo:Health": {
          "HP": 3
        }
      }
    },
    {"id": "b", "group": "demo"}
  ]
}
`

func TestFind(t *testing.T) {
	for _, tc := range []struct {
		name string
		path []any
		want string
	}{
		{"scalar", []any{"name"}, `"L"`},
		{"nested", []any{"objects", 0, "components", "gfx:Transform", "Pos", "X"}, `1`},
		{"inline object", []any{"objects", 1}, `{"id": "b", "group": "demo"}`},
		{"escaped key", []any{"k\"ey"}, `7`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			doc := level
			if tc.name == "escaped key" {
				doc = `{"k\"ey": 7}`
			}
			sp, err := Find([]byte(doc), tc.path...)
			if err != nil {
				t.Fatal(err)
			}
			if got := doc[sp.Start:sp.End]; got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
	if _, err := Find([]byte(level), "objects", 5); !errors.Is(err, ErrNotFound) {
		t.Errorf("missing index: %v", err)
	}
	if _, err := Find([]byte(level), "nope"); !errors.Is(err, ErrNotFound) {
		t.Errorf("missing key: %v", err)
	}
	if _, err := Find([]byte(`{"a": `), "a"); err == nil {
		t.Error("invalid JSON accepted")
	}
}

func TestReplace(t *testing.T) {
	for _, tc := range []struct {
		name, doc, value, want string
		path                   []any
	}{
		{"scalar keeps the rest", level, `9`, strings.Replace(level, `"X": 1`, `"X": 9`, 1),
			[]any{"objects", 0, "components", "gfx:Transform", "Pos", "X"}},
		{"one-line object stays on one line", level, `{"Pos":{"X":5,"Y":6},"Rot":1}`,
			strings.Replace(level, `{"Pos": {"X": 1, "Y": 2}, "Rot": 0}`, `{"Pos": {"X": 5, "Y": 6}, "Rot": 1}`, 1),
			[]any{"objects", 0, "components", "gfx:Transform"}},
		{"multi-line object keeps its indentation", level, `{"HP":4,"Kind":1}`,
			strings.Replace(level, "{\n          \"HP\": 3\n        }", "{\n          \"HP\": 4,\n          \"Kind\": 1\n        }", 1),
			[]any{"objects", 0, "components", "demo:Health"}},
		{"tabs", "{\n\t\"a\": {\n\t\t\"b\": 1\n\t}\n}\n", `{"c":[1,2]}`,
			"{\n\t\"a\": {\n\t\t\"c\": [\n\t\t\t1,\n\t\t\t2\n\t\t]\n\t}\n}\n", []any{"a"}},
		{"crlf", "{\r\n  \"a\": {\r\n    \"b\": 1\r\n  }\r\n}\r\n", `{"c":2}`,
			"{\r\n  \"a\": {\r\n    \"c\": 2\r\n  }\r\n}\r\n", []any{"a"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Replace([]byte(tc.doc), []byte(tc.value), tc.path...)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tc.want {
				t.Errorf("got:\n%s\nwant:\n%s", got, tc.want)
			}
		})
	}
	if _, err := Replace([]byte(level), []byte(`1`), "nope"); !errors.Is(err, ErrNotFound) {
		t.Errorf("missing path: %v", err)
	}
	if _, err := Replace([]byte(level), []byte(`{`), "name"); err == nil {
		t.Error("invalid value accepted")
	}
}

func TestSet(t *testing.T) {
	for _, tc := range []struct {
		name, doc, value, want string
		path                   []any
	}{
		{"replaces", `{"a": 1}`, `2`, `{"a": 2}`, []any{"a"}},
		{"member into a one-line object", `{"a": 1}`, `[1,2]`, `{"a": 1, "b": [1, 2]}`, []any{"b"}},
		{"member into an empty object", `{}`, `1`, `{"a": 1}`, []any{"a"}},
		{"member into an empty object of a multi-line document", "{\n  \"c\": {}\n}\n", `1`,
			"{\n  \"c\": {\n    \"a\": 1\n  }\n}\n", []any{"c", "a"}},
		{"missing parents are created", `{"a": {}}`, `3`, `{"a": {"b": {"c": 3}}}`, []any{"a", "b", "c"}},
		{"append to an array", `[1, 2]`, `3`, `[1, 2, 3]`, []any{2}},
		{"escaped key", `{}`, `1`, `{"a\"b": 1}`, []any{`a"b`}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Set([]byte(tc.doc), []byte(tc.value), tc.path...)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tc.want {
				t.Errorf("got:\n%s\nwant:\n%s", got, tc.want)
			}
		})
	}
	if _, err := Set([]byte(`[1]`), []byte(`1`), 4); !errors.Is(err, ErrNotFound) {
		t.Errorf("index past the end: %v", err)
	}
	if _, err := Set([]byte(`{"a": 1}`), []byte(`1`), "a", "b"); err == nil {
		t.Error("member of a number accepted")
	}
}

func TestSetMultiLine(t *testing.T) {
	got, err := Set([]byte(level), []byte(`{"Kind":1}`), "objects", 0, "components", "demo:Extra")
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Replace(level, "          \"HP\": 3\n        }\n", "          \"HP\": 3\n        },\n        \"demo:Extra\": {\n          \"Kind\": 1\n        }\n", 1)
	if string(got) != want {
		t.Errorf("got:\n%s\nwant:\n%s", got, want)
	}
	// Only the inserted text differs.
	if !strings.HasPrefix(string(got), level[:strings.Index(level, "        }\n      }")+len("        }")]) {
		t.Error("text before the insertion changed")
	}
}

func TestInsert(t *testing.T) {
	obj := []byte(`{"id":"c","group":"demo"}`)
	got, err := Insert([]byte(level), obj, -1, "objects")
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Replace(level, "    {\"id\": \"b\", \"group\": \"demo\"}\n", "    {\"id\": \"b\", \"group\": \"demo\"},\n    {\n      \"id\": \"c\",\n      \"group\": \"demo\"\n    }\n", 1)
	if string(got) != want {
		t.Errorf("append:\n%s\nwant:\n%s", got, want)
	}
	got, err = Insert([]byte(`[1, 3]`), []byte(`2`), 1)
	if err != nil || string(got) != `[1, 2, 3]` {
		t.Errorf("insert before: %q %v", got, err)
	}
	got, err = Insert([]byte("[\n  1,\n  3\n]\n"), []byte(`2`), 1)
	if err != nil || string(got) != "[\n  1,\n  2,\n  3\n]\n" {
		t.Errorf("insert before, lines: %q %v", got, err)
	}
	got, err = Insert([]byte(`{"objects": []}`), []byte(`{"id":"a"}`), 0, "objects")
	if err != nil || string(got) != `{"objects": [{"id": "a"}]}` {
		t.Errorf("into empty one-line array: %q %v", got, err)
	}
	got, err = Insert([]byte("{\n  \"objects\": []\n}\n"), []byte(`{"id":"a"}`), 0, "objects")
	if err != nil || string(got) != "{\n  \"objects\": [\n    {\n      \"id\": \"a\"\n    }\n  ]\n}\n" {
		t.Errorf("into empty array: %q %v", got, err)
	}
	if _, err := Insert([]byte(level), obj, 0, "name"); err == nil {
		t.Error("insert into a string accepted")
	}
	if _, err := Insert([]byte(level), obj, 9, "objects"); err == nil {
		t.Error("index out of range accepted")
	}
}

func TestDelete(t *testing.T) {
	for _, tc := range []struct {
		name, doc, want string
		path            []any
	}{
		{"first of several, lines", "{\n  \"a\": 1,\n  \"b\": 2\n}\n", "{\n  \"b\": 2\n}\n", []any{"a"}},
		{"middle, lines", "{\n  \"a\": 1,\n  \"b\": 2,\n  \"c\": 3\n}\n", "{\n  \"a\": 1,\n  \"c\": 3\n}\n", []any{"b"}},
		{"last, lines", "{\n  \"a\": 1,\n  \"b\": 2\n}\n", "{\n  \"a\": 1\n}\n", []any{"b"}},
		{"only member", "{\n  \"a\": 1\n}\n", "{}\n", []any{"a"}},
		{"one-line first", `{"a": 1, "b": 2}`, `{"b": 2}`, []any{"a"}},
		{"one-line last", `{"a": 1, "b": 2}`, `{"a": 1}`, []any{"b"}},
		{"array element", `[1, 2, 3]`, `[1, 3]`, []any{1}},
		{"array last", `[1, 2, 3]`, `[1, 2]`, []any{2}},
		{"escaped key", `{"a\"b": 1, "c": 2}`, `{"c": 2}`, []any{`a"b`}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got, err := Delete([]byte(tc.doc), tc.path...)
			if err != nil {
				t.Fatal(err)
			}
			if string(got) != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
	if _, err := Delete([]byte(level), "nope"); !errors.Is(err, ErrNotFound) {
		t.Errorf("missing: %v", err)
	}
	if _, err := Delete([]byte(level)); err == nil {
		t.Error("deleting the document accepted")
	}
}

// Every result is valid JSON that decodes to what the edit says.
func TestResultsAreValid(t *testing.T) {
	doc := []byte(level)
	steps := []func([]byte) ([]byte, error){
		func(d []byte) ([]byte, error) { return Set(d, []byte(`{"A":1}`), "objects", 1, "components", "x:Y") },
		func(d []byte) ([]byte, error) { return Delete(d, "objects", 0, "components", "demo:Health") },
		func(d []byte) ([]byte, error) { return Insert(d, []byte(`{"id":"z"}`), 0, "objects") },
		func(d []byte) ([]byte, error) { return Replace(d, []byte(`"M"`), "name") },
		func(d []byte) ([]byte, error) { return Delete(d, "objects", 1) },
	}
	for i, step := range steps {
		var err error
		if doc, err = step(doc); err != nil {
			t.Fatalf("step %d: %v", i, err)
		}
		if !json.Valid(doc) {
			t.Fatalf("step %d: invalid JSON:\n%s", i, doc)
		}
	}
	var got struct {
		Name    string
		Objects []struct{ ID string }
	}
	if err := json.Unmarshal(doc, &got); err != nil || got.Name != "M" || len(got.Objects) != 2 || got.Objects[0].ID != "z" {
		t.Errorf("final document: %+v %v\n%s", got, err, doc)
	}
}
