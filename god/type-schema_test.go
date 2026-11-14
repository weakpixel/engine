package god

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestTypeSchemaValidate(t *testing.T) {
	schema := MustParseSchema(`{
		"type": "object",
		"required": ["id", "pos"],
		"properties": {
			"id": {"type": "integer", "format": "int32", "minimum": 0, "maximum": 255},
			"pos": {"type": "object", "nullable": true, "required": ["x"], "properties": {"x": {"type": "number"}}},
			"tags": {"type": "array", "nullable": true, "items": {"type": "string"}, "minItems": 0, "maxItems": 2},
			"attrs": {"type": "object", "additionalProperties": {"type": "boolean"}},
			"blob": {"type": "string", "format": "byte"},
			"when": {"x-go-type": "time.Time"},
			"any": {"x-preserve-unknown-fields": true},
			"sprite": {"type": "string", "format": "asset", "x-asset": "sprite"},
			"time": {"type": "number", "x-runtime": true},
			"ref": {"type": "string", "format": "entity", "nullable": true},
			"must": {"type": "string", "format": "entity"},
			"bad": {"type": "integer", "x-asset": "sprite"},
			"shape": {"type": "integer", "format": "int32", "x-enum": [{"value": 0, "name": "EmitPoint"}, {"value": 1, "name": "EmitCircle"}, {"value": 2, "name": "EmitRect"}]},
			"shapes": {"type": "array", "items": {"type": "integer", "format": "int32", "x-enum": [{"value": 0, "name": "EmitPoint"}, {"value": 1, "name": "EmitCircle"}, {"value": 2, "name": "EmitRect"}]}},
			"byName": {"type": "object", "additionalProperties": {"type": "integer", "format": "int32", "x-enum": [{"value": 0, "name": "EmitPoint"}, {"value": 1, "name": "EmitCircle"}, {"value": 2, "name": "EmitRect"}]}},
			"maybe": {"type": "integer", "nullable": true, "x-enum": [{"value": 0, "name": "A"}]}
		}
	}`)
	for name, tc := range map[string]struct{ json, want string }{
		"valid":           {`{"id": 7, "pos": {"x": 1.5}, "tags": ["a"], "attrs": {"k": true}, "blob": "aGk=", "when": 5, "any": [1]}`, ""},
		"null pos":        {`{"id": 7, "pos": null, "tags": null}`, ""},
		"integral float":  {`{"id": 7.0, "pos": null}`, ""},
		"not integer":     {`{"id": 7.5, "pos": null}`, "$.id: expected integer, got 7.5"},
		"above max":       {`{"id": 256, "pos": null}`, "$.id: 256 is greater than the maximum 255"},
		"below min":       {`{"id": -1, "pos": null}`, "$.id: -1 is less than the minimum 0"},
		"null id":         {`{"id": null, "pos": null}`, "$.id: null is not allowed"},
		"missing":         {`{"pos": null}`, `$: missing required property "id"`},
		"unknown":         {`{"id": 1, "pos": null, "zzz": 1}`, `$: unknown property "zzz"`},
		"nested":          {`{"id": 1, "pos": {"x": "a"}}`, "$.pos.x: expected number, got string"},
		"item":            {`{"id": 1, "pos": null, "tags": ["a", 2]}`, "$.tags[1]: expected string, got number"},
		"too many":        {`{"id": 1, "pos": null, "tags": ["a", "b", "c"]}`, "$.tags: expected at most 2 items, got 3"},
		"map value":       {`{"id": 1, "pos": null, "attrs": {"k": 1}}`, "$.attrs.k: expected boolean, got number"},
		"bad base64":      {`{"id": 1, "pos": null, "blob": "!!"}`, "$.blob: invalid base64"},
		"asset":           {`{"id": 1, "pos": null, "sprite": "a/b.png#x"}`, ""},
		"asset not str":   {`{"id": 1, "pos": null, "sprite": 3}`, "$.sprite: expected string, got number"},
		"runtime":         {`{"id": 1, "pos": null, "time": 1}`, "$.time: runtime field"},
		"x-asset on int":  {`{"id": 1, "pos": null, "bad": 1}`, "$.bad: x-asset"},
		"entity ref":      {`{"id": 1, "pos": null, "ref": "hero", "must": "x"}`, ""},
		"entity null":     {`{"id": 1, "pos": null, "ref": null}`, ""},
		"entity empty":    {`{"id": 1, "pos": null, "ref": ""}`, "$.ref: empty entity reference"},
		"entity number":   {`{"id": 1, "pos": null, "ref": 3}`, "$.ref: expected string, got number"},
		"entity no null":  {`{"id": 1, "pos": null, "must": null}`, "$.must: null is not allowed"},
		"enum member":     {`{"id": 1, "pos": null, "shape": 2, "shapes": [0, 1], "byName": {"a": 1}, "maybe": null}`, ""},
		"enum not member": {`{"id": 1, "pos": null, "shape": 7}`, "$.shape: 7 is not a value of the enum (0 EmitPoint, 1 EmitCircle, 2 EmitRect)"},
		"enum in slice":   {`{"id": 1, "pos": null, "shapes": [0, 3]}`, "$.shapes[1]: 3 is not a value of the enum"},
		"enum in map":     {`{"id": 1, "pos": null, "byName": {"a": -1}}`, "$.byName.a: -1 is not a value of the enum"},
		"not object":      {`[]`, "$: expected object, got array"},
		"not json":        {`{`, "unexpected EOF"},
	} {
		t.Run(name, func(t *testing.T) {
			err := schema.Validate([]byte(tc.json))
			switch {
			case tc.want == "" && err != nil:
				t.Fatalf("unexpected error: %v", err)
			case tc.want != "" && (err == nil || !strings.Contains(err.Error(), tc.want)):
				t.Fatalf("got %v, want error containing %q", err, tc.want)
			}
		})
	}
}

func TestTypeSchemaEditorHints(t *testing.T) {
	const src = `{"type":"object","properties":{"Angle":{"type":"number","format":"float","x-editor":{"name":"Heading","min":0,"max":360,"step":5,"unit":"deg","order":0,"showIf":{"field":"On","equals":true,"not":true}}},"On":{"type":"boolean"}}}`
	s := MustParseSchema(src)
	h := s.Properties["Angle"].XEditor
	if h == nil || h.Name != "Heading" || *h.Min != 0 || *h.Max != 360 || *h.Step != 5 || h.Unit != UnitDegrees || h.Order == nil || *h.Order != 0 {
		t.Fatalf("hints = %+v", h)
	}
	if h.ShowIf == nil || h.ShowIf.Field != "On" || string(h.ShowIf.Equals) != "true" || !h.ShowIf.Not {
		t.Fatalf("showIf = %+v", h.ShowIf)
	}
	// Hints guide the editor only: validation and the initial value ignore them.
	if err := s.Validate([]byte(`{"Angle":720,"On":false}`)); err != nil {
		t.Errorf("Validate rejected a value outside the hinted range: %v", err)
	}
	out, err := json.Marshal(s)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(out), `"showIf":{"field":"On","equals":true,"not":true}`) {
		t.Errorf("marshalled schema lost the hints: %s", out)
	}
}
