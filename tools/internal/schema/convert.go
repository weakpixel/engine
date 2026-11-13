package schema

import (
	"encoding/json"
	"fmt"
	"slices"

	"github.com/weakpixel/engine/god"
)

// jsonSchemaURL is the dialect of the files `weakpixel schema` writes; VSCode
// reads draft-07.
const jsonSchemaURL = "http://json-schema.org/draft-07/schema#"

// toJSONSchema converts a GOD TypeSchema to a JSON Schema (draft-07) value.
// patch drops every "required", for the overrides of a prefab instance, which
// are merge patches.
//
// The mapping follows the TypeSchema rules: an object with Properties allows
// no other keys, nullable becomes a type list with "null", x-enum becomes
// enum with enumDescriptions, x-runtime properties are left out (input may
// not contain them), and a value of unknown shape (x-go-type,
// x-preserve-unknown-fields) accepts anything.
func toJSONSchema(s *god.TypeSchema, patch bool) map[string]any {
	out := map[string]any{}
	if s == nil || s.GoType != "" || s.PreserveUnknownFields || s.Type == "" {
		if s != nil && s.Description != "" {
			out["description"] = s.Description
		}
		return out
	}
	if s.Nullable {
		out["type"] = []string{s.Type, "null"}
	} else {
		out["type"] = s.Type
	}
	if s.Description != "" {
		out["description"] = s.Description
	}
	if s.XAsset != "" {
		out["x-asset"] = s.XAsset
	}
	if s.Format == god.FormatEntity {
		out["x-entity"] = true
	}
	if len(s.Default) > 0 {
		out["default"] = json.RawMessage(s.Default)
	}
	if s.Minimum != nil {
		out["minimum"] = *s.Minimum
	}
	if s.Maximum != nil {
		out["maximum"] = *s.Maximum
	}
	if s.MinItems != nil {
		out["minItems"] = *s.MinItems
	}
	if s.MaxItems != nil {
		out["maxItems"] = *s.MaxItems
	}
	if len(s.XEnum) > 0 {
		values := make([]any, 0, len(s.XEnum)+1)
		docs := make([]string, 0, len(s.XEnum)+1)
		for _, e := range s.XEnum {
			values = append(values, e.Value)
			d := e.Name
			if e.Doc != "" {
				d += ": " + e.Doc
			}
			docs = append(docs, d)
		}
		if s.Nullable {
			values, docs = append(values, nil), append(docs, "null")
		}
		out["enum"], out["enumDescriptions"] = values, docs
	}
	if len(s.Properties) > 0 {
		props := make(map[string]any, len(s.Properties))
		for name, p := range s.Properties {
			if p.XRuntime {
				continue
			}
			props[name] = toJSONSchema(p, patch)
		}
		out["properties"] = props
		if s.AdditionalProperties == nil {
			out["additionalProperties"] = false
		}
		if req := authored(s); len(req) > 0 && !patch {
			out["required"] = req
		}
	}
	if s.AdditionalProperties != nil {
		out["additionalProperties"] = toJSONSchema(s.AdditionalProperties, patch)
	}
	if s.Items != nil {
		out["items"] = toJSONSchema(s.Items, patch)
	}
	return out
}

// authored returns the required properties of s that are not runtime state.
func authored(s *god.TypeSchema) []string {
	var req []string
	for _, name := range s.Required {
		if p := s.Properties[name]; p != nil && !p.XRuntime {
			req = append(req, name)
		}
	}
	slices.Sort(req)
	return req
}

// parseTypeSchema decodes a TypeSchema file.
func parseTypeSchema(data []byte) (*god.TypeSchema, error) {
	var s god.TypeSchema
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, fmt.Errorf("schema: %w", err)
	}
	return &s, nil
}

// document wraps a converted schema as a top-level JSON Schema file.
func document(title, description string, s *god.TypeSchema) map[string]any {
	d := toJSONSchema(s, false)
	d["$schema"] = jsonSchemaURL
	d["title"] = title
	if description != "" {
		d["description"] = description
	}
	return d
}
