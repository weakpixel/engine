package god

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
)

// FormatEntity is the Format of an entity reference: a string property that
// holds the document id of another object (see Refs). The generator emits it,
// nullable, for every field of type ecs.Entity.
const FormatEntity = "entity"

// TypeSchema describes the JSON form of a component or one of its parts. It is
// a subset of OpenAPI v3 / JSON Schema: the same keywords with the same
// meaning, plus the x- extensions below.
//
// Type is one of "object", "array", "string", "integer", "number" and
// "boolean". Structs are objects with Properties (keyed by JSON name) and
// Required; maps are objects with AdditionalProperties. Unknown properties of
// an object with Properties are invalid.
type TypeSchema struct {
	Type     string `json:"type,omitempty"`
	Format   string `json:"format,omitempty"` // int32, int64, float, double, byte, asset, entity
	Nullable bool   `json:"nullable,omitempty"`

	// Description is documentation text (the Go doc comment of a type or
	// field). It has no effect on validation and is left out of generated Go.
	Description string `json:"description,omitempty"`

	Properties           map[string]*TypeSchema `json:"properties,omitempty"`
	Required             []string               `json:"required,omitempty"`
	AdditionalProperties *TypeSchema            `json:"additionalProperties,omitempty"`

	Items    *TypeSchema `json:"items,omitempty"`
	MinItems *int        `json:"minItems,omitempty"`
	MaxItems *int        `json:"maxItems,omitempty"`

	Minimum *float64 `json:"minimum,omitempty"`
	Maximum *float64 `json:"maximum,omitempty"`

	// Default is the JSON value an editor writes when it creates a value of
	// this schema (see InitialValue), declared with +game:default. It is
	// editor seed data: decoding never applies it, so an absent field still
	// decodes to the Go zero value. The generator has already checked it
	// against the schema; Validate does not.
	Default json.RawMessage `json:"default,omitempty"`

	// XEnum lists the named values of an integer type marked +game:enum,
	// sorted by value. When set, Validate accepts only these values. The wire
	// value stays the integer; the names are for tools such as the editor.
	XEnum []EnumValue `json:"x-enum,omitempty"`

	// GoType names a Go type with a custom JSON encoding (e.g. "time.Time"). Its
	// wire shape is not described and not validated.
	GoType string `json:"x-go-type,omitempty"`

	// XAsset is the asset kind ("sprite", "clip", "font", ...) of a string
	// property with Format "asset". The string is an asset key
	// ("chars/hero.aseprite.json#run"); Deserialize and SerializeComponents
	// convert it to and from the runtime handle through the world's
	// AssetResolver. The kind is a free string interpreted by the resolver.
	XAsset string `json:"x-asset,omitempty"`

	// XRuntime marks a property of runtime state (e.g. animation playback
	// time). It is part of the schema for tools and scripts, but it is not
	// authored data: Serialize omits it and Validate rejects it in input.
	XRuntime bool `json:"x-runtime,omitempty"`

	// XEditor holds hints for tools that edit the value, declared with
	// +game:editor. They are like Default: editor data with no effect on
	// validation or decoding, kept in the compiled schema so that an editor
	// built into the game sees them.
	XEditor *EditorHints `json:"x-editor,omitempty"`

	// PreserveUnknownFields marks a value of unknown shape (interfaces,
	// recursive types, ...). It is not validated.
	PreserveUnknownFields bool `json:"x-preserve-unknown-fields,omitempty"`
}

// EditorHints say how an editor shows and edits a property. Every field is
// optional; the zero value means "no hint".
type EditorHints struct {
	// Name is the label shown instead of the Go field name.
	Name string `json:"name,omitempty"`
	// Min, Max and Step bound and step a number (for an array or an object of
	// numbers, each element). They guide the widget (a slider when Min and Max
	// are set); they do not validate. With Unit "deg" they are in degrees.
	Min  *float64 `json:"min,omitempty"`
	Max  *float64 `json:"max,omitempty"`
	Step *float64 `json:"step,omitempty"`
	// Unit labels the value ("s", "px", "px/s"). The unit UnitDegrees means
	// the stored number is in radians and the editor shows degrees.
	Unit string `json:"unit,omitempty"`
	// Order sorts the property among its siblings, lowest first; properties
	// without an Order come after the ones that have it, in declaration order.
	Order *int `json:"order,omitempty"`
	// Color asks for a color picker; the property is an object with R, G, B
	// and A or an array of 3 or 4 numbers.
	Color bool `json:"color,omitempty"`
	// Hidden keeps the property out of the editor's property list. It stays
	// in the file.
	Hidden bool `json:"hidden,omitempty"`
	// ShowIf shows the property only while a condition on a sibling holds.
	ShowIf *ShowIf `json:"showIf,omitempty"`
}

// UnitDegrees is the EditorHints.Unit of a value stored in radians and shown
// in degrees.
const UnitDegrees = "deg"

// ShowIf is a condition on a sibling property of the same struct. Without
// Equals the sibling must be truthy (true, a non-zero number, a non-empty
// string); with it, the sibling must equal the JSON value (an enum is
// compared by its integer). Not negates the result.
type ShowIf struct {
	Field  string          `json:"field"`
	Equals json.RawMessage `json:"equals,omitempty"`
	Not    bool            `json:"not,omitempty"`
}

// EnumValue is one member of an x-enum: a Go constant of the enum type.
type EnumValue struct {
	Value int64  `json:"value"`
	Name  string `json:"name"` // the Go constant name, e.g. "EmitCircle"
	// Doc is documentation text (the constant's doc comment). Like
	// Description it has no effect on validation and is left out of
	// generated Go.
	Doc string `json:"doc,omitempty"`
}

// WithoutDescriptions returns a deep copy of s with every Description (and
// every EnumValue.Doc) removed.
// Documentation is not part of a schema's identity, so the generator strips it
// from the schemas it compiles into Go and RegisterSchema compares schemas
// without it.
func (s *TypeSchema) WithoutDescriptions() *TypeSchema {
	if s == nil {
		return nil
	}
	out := *s
	out.Description = ""
	if s.XEnum != nil {
		out.XEnum = make([]EnumValue, len(s.XEnum))
		for i, e := range s.XEnum {
			out.XEnum[i] = EnumValue{Value: e.Value, Name: e.Name}
		}
	}
	if s.Properties != nil {
		out.Properties = make(map[string]*TypeSchema, len(s.Properties))
		for name, p := range s.Properties {
			out.Properties[name] = p.WithoutDescriptions()
		}
	}
	out.AdditionalProperties = s.AdditionalProperties.WithoutDescriptions()
	out.Items = s.Items.WithoutDescriptions()
	return &out
}

// InitialValue is the JSON an editor writes for a new value of this schema:
// the Default when set, else for an object the initial value of each
// property, else the zero value (arrays get MinItems zero elements, numbers 0,
// booleans false, strings "", and entity references and values of unknown
// shape null). Runtime properties are left out, as they are not authored
// data, and so are pointer properties without a Default: nil is their zero
// value and means "not set" (the editor offers an override), so a new
// component does not override what another source, such as a particle
// effect's file, provides. A nil schema yields null.
func (s *TypeSchema) InitialValue() json.RawMessage {
	data, err := json.Marshal(s.initial())
	if err != nil {
		panic("god: invalid default in schema: " + err.Error())
	}
	return data
}

func (s *TypeSchema) initial() any {
	switch {
	case s == nil:
		return nil
	case len(s.Default) > 0:
		return s.Default
	}
	switch s.Type {
	case "object":
		out := map[string]any{}
		for name, p := range s.Properties {
			if p != nil && p.XRuntime || p.isUnsetPointer() {
				continue
			}
			out[name] = p.initial()
		}
		return out
	case "array":
		out := make([]any, 0)
		if s.MinItems != nil {
			for range *s.MinItems {
				out = append(out, s.Items.initial())
			}
		}
		return out
	case "integer", "number":
		return 0
	case "boolean":
		return false
	case "string":
		if s.Format == FormatEntity {
			return nil
		}
		return ""
	}
	return nil
}

// isUnsetPointer reports whether s is a pointer (nullable, but not an entity
// reference, a byte slice, a slice or a map) without a Default: such a
// property starts absent (nil). Slices and maps are told apart from pointers
// by their shape; a pointer to an array has MaxItems, a slice does not.
func (s *TypeSchema) isUnsetPointer() bool {
	switch {
	case s == nil || !s.Nullable || len(s.Default) > 0:
		return false
	case s.Format == FormatEntity || s.Format == "byte" || s.AdditionalProperties != nil:
		return false
	case s.Type == "array" && s.MaxItems == nil:
		return false
	}
	return true
}

// Ptr returns a pointer to v, for the optional numeric fields of TypeSchema.
//
//go:fix inline
func Ptr[T any](v T) *T { return new(v) }

// MustParseSchema decodes a TypeSchema from its JSON form and panics on error.
func MustParseSchema(data string) *TypeSchema {
	s := new(TypeSchema)
	if err := json.Unmarshal([]byte(data), s); err != nil {
		panic("god: invalid schema: " + err.Error())
	}
	return s
}

// Validate checks that raw conforms to the schema. All violations are
// reported, each with the path of the offending value (e.g. "$.Pos.X").
func (s *TypeSchema) Validate(raw json.RawMessage) error {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return err
	}
	var errs []error
	s.validate("$", v, &errs)
	return errors.Join(errs...)
}

func (s *TypeSchema) validate(path string, v any, errs *[]error) {
	fail := func(format string, args ...any) {
		*errs = append(*errs, fmt.Errorf("%s: %s", path, fmt.Sprintf(format, args...)))
	}
	if s == nil || s.PreserveUnknownFields || s.GoType != "" {
		return
	}
	if s.XAsset != "" && s.Type != "string" {
		fail("x-asset %q on a %s schema (must be string)", s.XAsset, s.Type)
		return
	}
	if v == nil {
		if !s.Nullable {
			fail("null is not allowed")
		}
		return
	}
	switch s.Type {
	case "object":
		m, ok := v.(map[string]any)
		if !ok {
			fail("expected object, got %s", kind(v))
			return
		}
		for _, name := range s.Required {
			if _, ok := m[name]; !ok {
				fail("missing required property %q", name)
			}
		}
		keys := make([]string, 0, len(m))
		for k := range m {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			prop, known := s.Properties[k]
			switch {
			case known && prop != nil && prop.XRuntime:
				*errs = append(*errs, fmt.Errorf("%s.%s: runtime field, not authored data", path, k))
			case known:
				prop.validate(path+"."+k, m[k], errs)
			case s.AdditionalProperties != nil:
				s.AdditionalProperties.validate(path+"."+k, m[k], errs)
			default:
				*errs = append(*errs, fmt.Errorf("%s: unknown property %q", path, k))
			}
		}
	case "array":
		a, ok := v.([]any)
		if !ok {
			fail("expected array, got %s", kind(v))
			return
		}
		if s.MinItems != nil && len(a) < *s.MinItems {
			fail("expected at least %d items, got %d", *s.MinItems, len(a))
		}
		if s.MaxItems != nil && len(a) > *s.MaxItems {
			fail("expected at most %d items, got %d", *s.MaxItems, len(a))
		}
		for i, item := range a {
			s.Items.validate(path+"["+strconv.Itoa(i)+"]", item, errs)
		}
	case "string":
		str, ok := v.(string)
		if !ok {
			fail("expected string, got %s", kind(v))
			return
		}
		switch s.Format {
		case "byte":
			if _, err := base64.StdEncoding.DecodeString(str); err != nil {
				fail("invalid base64: %v", err)
			}
		case FormatEntity:
			if str == "" {
				fail("empty entity reference")
			}
		}
	case "boolean":
		if _, ok := v.(bool); !ok {
			fail("expected boolean, got %s", kind(v))
		}
	case "integer", "number":
		n, ok := v.(json.Number)
		if !ok {
			fail("expected %s, got %s", s.Type, kind(v))
			return
		}
		f, err := strconv.ParseFloat(n.String(), 64)
		if err != nil {
			fail("number %s out of range", n)
			return
		}
		switch {
		case s.Type == "integer" && f != math.Trunc(f):
			fail("expected integer, got %s", n)
		case s.Minimum != nil && f < *s.Minimum:
			fail("%s is less than the minimum %v", n, *s.Minimum)
		case s.Maximum != nil && f > *s.Maximum:
			fail("%s is greater than the maximum %v", n, *s.Maximum)
		case s.XEnum != nil && !s.isEnumValue(f):
			fail("%s is not a value of the enum (%s)", n, s.enumList())
		}
	}
}

// isEnumValue reports whether f is one of the XEnum values.
func (s *TypeSchema) isEnumValue(f float64) bool {
	for _, e := range s.XEnum {
		if float64(e.Value) == f {
			return true
		}
	}
	return false
}

// enumList renders the XEnum members as "0 EmitPoint, 1 EmitCircle" for
// error messages.
func (s *TypeSchema) enumList() string {
	var b strings.Builder
	for i, e := range s.XEnum {
		if i > 0 {
			b.WriteString(", ")
		}
		fmt.Fprintf(&b, "%d %s", e.Value, e.Name)
	}
	return b.String()
}

// kind names the JSON type of a decoded value.
func kind(v any) string {
	switch v.(type) {
	case nil:
		return "null"
	case map[string]any:
		return "object"
	case []any:
		return "array"
	case string:
		return "string"
	case bool:
		return "boolean"
	case json.Number:
		return "number"
	}
	return fmt.Sprintf("%T", v)
}
