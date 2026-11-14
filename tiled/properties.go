package tiled

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"
)

// Property is one custom property. Value holds a Go value that matches Type:
//
//	"string"  string
//	"file"    string (a path relative to the file that declares it)
//	"color"   Color
//	"int"     int
//	"float"   float64
//	"bool"    bool
//	"object"  int (an object ID; 0 means none)
//	"class"   Properties (the members the file sets)
//
// Members of a class value carry no type in the file, so their Type is empty
// and Value is inferred from the JSON: string, bool, int (an integral
// number), float64 or Properties (a nested class). The getters accept such
// untyped members.
type Property struct {
	Name string
	Type string
	// PropertyType is the name of the custom class or enum, for "class"
	// values and enum-backed "string" or "int" values.
	PropertyType string
	Value        any
}

// Properties is an ordered list of custom properties: file order for
// properties, name order for the members of a class value. Defaults of custom
// types (the propertyTypes of a .tiled-project file) are not applied, because
// the project file is not read: a member the file does not set is absent.
type Properties []Property

// Get returns the property called name.
func (p Properties) Get(name string) (*Property, bool) {
	for i := range p {
		if p[i].Name == name {
			return &p[i], true
		}
	}
	return nil, false
}

// String returns a "string" or "file" property (or an untyped string member).
func (p Properties) String(name string) (string, bool) {
	if pr, ok := p.Get(name); ok && (pr.Type == "string" || pr.Type == "file" || pr.Type == "") {
		v, ok := pr.Value.(string)
		return v, ok
	}
	return "", false
}

// File returns a "file" property (or an untyped string member), a path
// relative to the file that declares it.
func (p Properties) File(name string) (string, bool) {
	if pr, ok := p.Get(name); ok && (pr.Type == "file" || pr.Type == "") {
		v, ok := pr.Value.(string)
		return v, ok
	}
	return "", false
}

// Int returns an "int" property (or an untyped integral member).
func (p Properties) Int(name string) (int, bool) {
	if pr, ok := p.Get(name); ok && (pr.Type == "int" || pr.Type == "") {
		v, ok := pr.Value.(int)
		return v, ok
	}
	return 0, false
}

// Float returns a "float" property. An "int" property (or an untyped number
// member) widens to float64, because Tiled writes a float with an integral
// value, such as 1.0, as 1.
func (p Properties) Float(name string) (float64, bool) {
	pr, ok := p.Get(name)
	if !ok || (pr.Type != "float" && pr.Type != "int" && pr.Type != "") {
		return 0, false
	}
	switch v := pr.Value.(type) {
	case float64:
		return v, true
	case int:
		return float64(v), true
	}
	return 0, false
}

// Bool returns a "bool" property (or an untyped bool member).
func (p Properties) Bool(name string) (bool, bool) {
	if pr, ok := p.Get(name); ok && (pr.Type == "bool" || pr.Type == "") {
		v, ok := pr.Value.(bool)
		return v, ok
	}
	return false, false
}

// Color returns a "color" property, or an untyped member whose string parses
// as a color.
func (p Properties) Color(name string) (Color, bool) {
	pr, ok := p.Get(name)
	if !ok {
		return 0, false
	}
	switch v := pr.Value.(type) {
	case Color:
		return v, pr.Type == "color"
	case string:
		if pr.Type == "" {
			c, err := ParseColor(v)
			return c, err == nil
		}
	}
	return 0, false
}

// Object returns an "object" property: the ID of the referenced object, or 0
// for none.
func (p Properties) Object(name string) (int, bool) {
	if pr, ok := p.Get(name); ok && pr.Type == "object" {
		v, ok := pr.Value.(int)
		return v, ok
	}
	return 0, false
}

// Class returns the members of a "class" property (or of an untyped nested
// class member).
func (p Properties) Class(name string) (Properties, bool) {
	if pr, ok := p.Get(name); ok && (pr.Type == "class" || pr.Type == "") {
		v, ok := pr.Value.(Properties)
		return v, ok
	}
	return nil, false
}

// rawProperty is a property as the file stores it.
type rawProperty struct {
	Name         string          `json:"name"`
	Type         string          `json:"type"`
	PropertyType string          `json:"propertytype"`
	Value        json.RawMessage `json:"value"`
}

// decodeProperties converts the file's properties, collecting one error per
// bad property.
func decodeProperties(raw []rawProperty) (Properties, error) {
	if len(raw) == 0 {
		return nil, nil
	}
	out := make(Properties, 0, len(raw))
	var errs []error
	for _, r := range raw {
		typ := r.Type
		if typ == "" {
			typ = "string" // Tiled's default property type
		}
		v, err := propertyValue(typ, r.Value)
		if err != nil {
			errs = append(errs, fmt.Errorf("property %q: %w", r.Name, err))
			continue
		}
		out = append(out, Property{Name: r.Name, Type: typ, PropertyType: r.PropertyType, Value: v})
	}
	return out, errors.Join(errs...)
}

// propertyValue decodes the JSON value of a property of type typ.
func propertyValue(typ string, raw json.RawMessage) (any, error) {
	if len(raw) == 0 {
		raw = json.RawMessage("null")
	}
	switch typ {
	case "string", "file":
		var s string
		if err := unmarshalNullable(raw, &s); err != nil {
			return nil, fmt.Errorf("%s value: %w", typ, err)
		}
		return s, nil
	case "color":
		var s string
		if err := unmarshalNullable(raw, &s); err != nil {
			return nil, fmt.Errorf("color value: %w", err)
		}
		return ParseColor(s)
	case "int", "object":
		var f float64
		if err := unmarshalNullable(raw, &f); err != nil {
			return nil, fmt.Errorf("%s value: %w", typ, err)
		}
		if f != math.Trunc(f) {
			return nil, fmt.Errorf("%s value %v is not an integer", typ, f)
		}
		return int(f), nil
	case "float":
		var f float64
		if err := unmarshalNullable(raw, &f); err != nil {
			return nil, fmt.Errorf("float value: %w", err)
		}
		return f, nil
	case "bool":
		var b bool
		if err := unmarshalNullable(raw, &b); err != nil {
			return nil, fmt.Errorf("bool value: %w", err)
		}
		return b, nil
	case "class":
		var m map[string]any
		if err := unmarshalNullable(raw, &m); err != nil {
			return nil, fmt.Errorf("class value: %w", err)
		}
		return classMembers(m), nil
	}
	return nil, fmt.Errorf("unknown property type %q", typ)
}

// unmarshalNullable is json.Unmarshal where null leaves v at its zero value.
func unmarshalNullable(raw json.RawMessage, v any) error {
	if bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
		return nil
	}
	return json.Unmarshal(raw, v)
}

// classMembers turns the JSON object of a class value into untyped members,
// sorted by name (JSON objects have no order).
func classMembers(m map[string]any) Properties {
	if len(m) == 0 {
		return Properties{}
	}
	out := make(Properties, 0, len(m))
	for name, v := range m {
		out = append(out, Property{Name: name, Value: memberValue(v)})
	}
	slices.SortFunc(out, func(a, b Property) int { return strings.Compare(a.Name, b.Name) })
	return out
}

// memberValue converts a decoded JSON value of a class member to the Go
// value documented on [Property].
func memberValue(v any) any {
	switch v := v.(type) {
	case float64:
		if v == math.Trunc(v) && math.Abs(v) < 1<<53 {
			return int(v)
		}
		return v
	case map[string]any:
		return classMembers(v)
	}
	return v // string, bool, nil, or an array (not produced by Tiled)
}
