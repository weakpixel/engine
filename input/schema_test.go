package input_test

import (
	"bytes"
	"encoding/json"
	"flag"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/weakpixel/engine/god"
	"github.com/weakpixel/engine/input"
)

var update = flag.Bool("update", false, "rewrite input/schema/*.schema.json from the Go types")

// schemaOf describes the JSON form of t as an god TypeSchema, the GOD
// schema dialect, so editors and tools that read GOD schemas can check
// bindings and profiles. Fields without omitempty are required.
func schemaOf(t reflect.Type) *god.TypeSchema {
	switch t.Kind() {
	case reflect.Pointer:
		s := schemaOf(t.Elem())
		s.Nullable = true
		return s
	case reflect.Struct:
		s := &god.TypeSchema{Type: "object", Properties: map[string]*god.TypeSchema{}}
		for f := range t.Fields() {
			name, opts, _ := strings.Cut(f.Tag.Get("json"), ",")
			if !f.IsExported() || name == "-" {
				continue
			}
			if name == "" {
				name = f.Name
			}
			s.Properties[name] = schemaOf(f.Type)
			if !strings.Contains(opts, "omitempty") {
				s.Required = append(s.Required, name)
			}
		}
		return s
	case reflect.Map:
		return &god.TypeSchema{Type: "object", AdditionalProperties: schemaOf(t.Elem())}
	case reflect.Slice:
		return &god.TypeSchema{Type: "array", Items: schemaOf(t.Elem())}
	case reflect.String:
		return &god.TypeSchema{Type: "string"}
	case reflect.Bool:
		return &god.TypeSchema{Type: "boolean"}
	case reflect.Float32:
		return &god.TypeSchema{Type: "number", Format: "float"}
	case reflect.Int:
		return &god.TypeSchema{Type: "integer", Format: "int64"}
	}
	panic("schemaOf: unsupported kind " + t.Kind().String())
}

// TestSchemas keeps the committed schemas in sync with the Go file model and
// checks that the shipped files conform to them.
func TestSchemas(t *testing.T) {
	for _, tc := range []struct {
		file    string
		typ     reflect.Type
		samples []string
	}{
		{"schema/bindings.schema.json", reflect.TypeFor[input.BindingsFile](), []string{"testdata/bindings.json"}},
		{"schema/profile.schema.json", reflect.TypeFor[input.Profile](), nil},
	} {
		t.Run(tc.file, func(t *testing.T) {
			want, err := json.MarshalIndent(schemaOf(tc.typ), "", "  ")
			if err != nil {
				t.Fatal(err)
			}
			want = append(want, '\n')
			if *update {
				if err := os.WriteFile(tc.file, want, 0o644); err != nil {
					t.Fatal(err)
				}
			}
			got, err := os.ReadFile(tc.file)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(got, want) {
				t.Fatalf("%s is stale; run go test ./input -run TestSchemas -update", tc.file)
			}
			s := god.MustParseSchema(string(got))
			for _, sample := range tc.samples {
				data, err := os.ReadFile(sample)
				if err != nil {
					t.Fatal(err)
				}
				if err := s.Validate(data); err != nil {
					t.Errorf("%s: %v", sample, err)
				}
			}
		})
	}
}

func TestProfileSchemaAcceptsMarshal(t *testing.T) {
	p := &input.Profile{Version: 1, Settings: input.Settings{InvertY: true}, Overrides: []input.Override{
		{Mapping: "gameplay", Action: "jump", Class: "kbm", Binding: &input.BindingSpec{Control: "key:K"}},
		{Mapping: "gameplay", Action: "move", Class: "kbm", Part: "up", Control: "key:Z"},
		{Mapping: "gameplay", Action: "fire", Class: "gamepad", Slot: 1, Unbind: true},
	}}
	data, err := p.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	s := god.MustParseSchema(string(must(os.ReadFile("schema/profile.schema.json"))))
	if err := s.Validate(data); err != nil {
		t.Fatalf("%v\n%s", err, data)
	}
}

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}
