package mcp

import (
	"errors"
	"fmt"
	"maps"
	"math"
	"slices"
	"strings"
)

// Schema is the subset of JSON Schema 2020-12 the tools' input and output
// schemas use: type, description, properties, required, closed objects,
// enum, minimum, maximum and items. An empty Schema accepts any value.
// Validate checks exactly these keywords, so a schema can never promise
// more than the server checks.
type Schema struct {
	Type        string             `json:"type,omitempty"` // object, array, string, integer, number, boolean
	Description string             `json:"description,omitempty"`
	Properties  map[string]*Schema `json:"properties,omitempty"`
	Required    []string           `json:"required,omitempty"`
	// AdditionalProperties false closes an object with Properties: other
	// keys are invalid. Object builds closed objects.
	AdditionalProperties *bool    `json:"additionalProperties,omitempty"`
	Enum                 []string `json:"enum,omitempty"`
	Minimum              *float64 `json:"minimum,omitempty"`
	Maximum              *float64 `json:"maximum,omitempty"`
	Items                *Schema  `json:"items,omitempty"`
	Default              any      `json:"default,omitempty"`
}

// Object returns a closed object schema with props; required names the
// required properties.
func Object(props map[string]*Schema, required ...string) *Schema {
	if props == nil {
		props = map[string]*Schema{}
	}
	closed := false
	return &Schema{Type: "object", Properties: props, Required: required, AdditionalProperties: &closed}
}

// String returns a string schema.
func String(desc string) *Schema { return &Schema{Type: "string", Description: desc} }

// Enum returns a string schema that accepts only values.
func Enum(desc string, values ...string) *Schema {
	return &Schema{Type: "string", Description: desc, Enum: values}
}

// Boolean returns a boolean schema.
func Boolean(desc string) *Schema { return &Schema{Type: "boolean", Description: desc} }

// Integer returns an integer schema bounded by min and max.
func Integer(desc string, min, max float64) *Schema {
	return &Schema{Type: "integer", Description: desc, Minimum: &min, Maximum: &max}
}

// Number returns a number schema.
func Number(desc string) *Schema { return &Schema{Type: "number", Description: desc} }

// Any returns a schema that accepts any JSON value.
func Any(desc string) *Schema { return &Schema{Description: desc} }

// types are the values of Schema.Type.
var types = []string{"object", "array", "string", "integer", "number", "boolean"}

// Check reports problems of the schema itself: an unknown type, keywords
// that do not fit the type, required names that are not properties.
func (s *Schema) Check() error {
	var errs []error
	s.check("", &errs)
	return errors.Join(errs...)
}

func (s *Schema) check(path string, errs *[]error) {
	at := func(format string, args ...any) {
		*errs = append(*errs, fmt.Errorf("%s: %s", pathOrRoot(path), fmt.Sprintf(format, args...)))
	}
	if s.Type != "" && !slices.Contains(types, s.Type) {
		at("unknown type %q", s.Type)
	}
	if (len(s.Properties) > 0 || len(s.Required) > 0 || s.AdditionalProperties != nil) && s.Type != "object" {
		at("properties on a %q schema", s.Type)
	}
	for _, r := range s.Required {
		if s.Properties[r] == nil {
			at("required %q is not a property", r)
		}
	}
	if len(s.Enum) > 0 && s.Type != "string" {
		at("enum on a %q schema", s.Type)
	}
	if (s.Minimum != nil || s.Maximum != nil) && s.Type != "integer" && s.Type != "number" {
		at("minimum or maximum on a %q schema", s.Type)
	}
	if s.Items != nil && s.Type != "array" {
		at("items on a %q schema", s.Type)
	}
	for _, k := range slices.Sorted(maps.Keys(s.Properties)) {
		s.Properties[k].check(path+"."+k, errs)
	}
	if s.Items != nil {
		s.Items.check(path+"[]", errs)
	}
}

// Validate checks v, a value decoded by encoding/json into any, against s
// and reports every problem with its path.
func (s *Schema) Validate(v any) error {
	var errs []error
	s.validate("", v, &errs)
	return errors.Join(errs...)
}

func pathOrRoot(path string) string {
	if path == "" {
		return "arguments"
	}
	return strings.TrimPrefix(path, ".")
}

func (s *Schema) validate(path string, v any, errs *[]error) {
	fail := func(format string, args ...any) {
		*errs = append(*errs, fmt.Errorf("%s: %s", pathOrRoot(path), fmt.Sprintf(format, args...)))
	}
	switch s.Type {
	case "":
		return
	case "object":
		obj, ok := v.(map[string]any)
		if !ok {
			fail("want an object, got %s", kindOf(v))
			return
		}
		for _, r := range s.Required {
			if _, ok := obj[r]; !ok {
				fail("missing required %q", r)
			}
		}
		for _, k := range slices.Sorted(maps.Keys(obj)) {
			if p := s.Properties[k]; p != nil {
				p.validate(path+"."+k, obj[k], errs)
			} else if s.AdditionalProperties != nil && !*s.AdditionalProperties {
				fail("unknown property %q (known: %s)", k, strings.Join(slices.Sorted(maps.Keys(s.Properties)), ", "))
			}
		}
	case "array":
		arr, ok := v.([]any)
		if !ok {
			fail("want an array, got %s", kindOf(v))
			return
		}
		if s.Items != nil {
			for i, e := range arr {
				s.Items.validate(fmt.Sprintf("%s[%d]", path, i), e, errs)
			}
		}
	case "string":
		str, ok := v.(string)
		if !ok {
			fail("want a string, got %s", kindOf(v))
			return
		}
		if len(s.Enum) > 0 && !slices.Contains(s.Enum, str) {
			fail("%q is not one of %s", str, strings.Join(s.Enum, ", "))
		}
	case "integer", "number":
		f, ok := v.(float64)
		if !ok {
			fail("want a %s, got %s", s.Type, kindOf(v))
			return
		}
		if s.Type == "integer" && f != math.Trunc(f) {
			fail("want an integer, got %v", f)
			return
		}
		if s.Minimum != nil && f < *s.Minimum {
			fail("%v is less than %v", f, *s.Minimum)
		}
		if s.Maximum != nil && f > *s.Maximum {
			fail("%v is more than %v", f, *s.Maximum)
		}
	case "boolean":
		if _, ok := v.(bool); !ok {
			fail("want a boolean, got %s", kindOf(v))
		}
	}
}

// kindOf names the JSON type of v.
func kindOf(v any) string {
	switch v.(type) {
	case nil:
		return "null"
	case map[string]any:
		return "an object"
	case []any:
		return "an array"
	case string:
		return "a string"
	case float64:
		return "a number"
	case bool:
		return "a boolean"
	}
	return fmt.Sprintf("%T", v)
}
