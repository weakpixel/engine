package gen

import (
	"encoding/json"
	"go/token"
	"go/types"
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/weakpixel/engine/god"
	"github.com/weakpixel/engine/internal/marker"
)

// editorArgs are the arguments of +game:editor, all optional. The numbers are
// pointers: nil means "not set", which differs from 0.
type editorArgs struct {
	Name   string   `marker:"name,optional"`
	Min    *float64 `marker:"min"`
	Max    *float64 `marker:"max"`
	Step   *float64 `marker:"step"`
	Unit   string   `marker:"unit,optional"`
	Order  *int     `marker:"order"`
	Color  bool     `marker:"color,optional"`
	Hidden bool     `marker:"hidden,optional"`
	ShowIf string   `marker:"showIf,optional"`
}

// pendingShowIf is a showIf condition waiting for the sibling properties of
// its struct, which may be declared after the field that uses it.
type pendingShowIf struct {
	prop *god.TypeSchema
	pos  token.Pos
	cond string
	// label is the field label for messages.
	label string
}

// applyEditor sets x-editor on prop, the schema of field v, from its
// +game:editor marker and reports hints that cannot take effect. The showIf
// condition is checked later by checkShowIf, once the siblings are known.
func (s *schemaBuilder) applyEditor(v *types.Var, prop *god.TypeSchema, mv marker.Marker) {
	args := mv.Value.(editorArgs)
	label := s.fieldLabel(v)
	h := &god.EditorHints{Name: args.Name, Unit: args.Unit, Color: args.Color, Hidden: args.Hidden}
	bad := func(format string, a ...any) {
		s.errorf(mv.Pos, "%s: +%s "+format, append([]any{label, markEditor}, a...)...)
	}

	for _, f := range []struct {
		name string
		src  *float64
		dst  **float64
	}{{"min", args.Min, &h.Min}, {"max", args.Max, &h.Max}, {"step", args.Step, &h.Step}} {
		if f.src == nil {
			continue
		}
		if math.IsNaN(*f.src) || math.IsInf(*f.src, 0) {
			bad("%s %v is not a finite number", f.name, *f.src)
			continue
		}
		*f.dst = f.src
	}
	if h.Min != nil && h.Max != nil && *h.Min > *h.Max {
		bad("min %v is above max %v", *h.Min, *h.Max)
	}
	if h.Step != nil && *h.Step <= 0 {
		bad("step must be above 0")
	}
	h.Order = args.Order
	if (h.Min != nil || h.Max != nil || h.Step != nil || h.Unit == god.UnitDegrees) && !numeric(prop) {
		bad("min, max, step and unit=deg need a number, or an array or object of numbers")
	}
	if h.Color && !colorShaped(prop) {
		bad("color needs an integer (0xRRGGBBAA), an object with R, G, B (and A) or an array of 3 or 4 numbers")
	}
	if prop.XRuntime && (h.Hidden || args.ShowIf != "") {
		bad("hidden and showIf are not needed on a runtime field (the editor never shows it)")
	}
	if args.ShowIf != "" {
		s.pending = append(s.pending, pendingShowIf{prop, mv.Pos, args.ShowIf, label})
	}
	prop.XEditor = h
}

// rejectEditor reports a +game:editor on field v that cannot take effect.
func (s *schemaBuilder) rejectEditor(v *types.Var, has bool, mv marker.Marker, reason string) {
	if has {
		s.errorf(mv.Pos, "%s: +%s is not allowed on %s", s.fieldLabel(v), markEditor, reason)
	}
}

// numeric reports whether prop is a number, or an array or object whose
// elements are all numbers.
func numeric(prop *god.TypeSchema) bool {
	switch {
	case prop.Type == "number" || prop.Type == "integer":
		return true
	case prop.Type == "array" && prop.Items != nil:
		return numeric(prop.Items)
	case prop.Type == "object" && len(prop.Properties) > 0 && prop.AdditionalProperties == nil:
		for _, p := range prop.Properties {
			if !numeric(p) {
				return false
			}
		}
		return true
	}
	return false
}

// colorShaped reports whether prop can be edited as a color: an integer
// packed as 0xRRGGBBAA, an object with the number properties R, G, B and
// optionally A, or an array of 3 or 4 numbers.
func colorShaped(prop *god.TypeSchema) bool {
	switch prop.Type {
	case "integer":
		return len(prop.XEnum) == 0
	case "array":
		return prop.Items != nil && numeric(prop.Items) && prop.Items.Type != "array" &&
			prop.MinItems != nil && *prop.MinItems >= 3 && *prop.MinItems <= 4
	case "object":
		for _, k := range []string{"R", "G", "B"} {
			if p := prop.Properties[k]; p == nil || !numeric(p) {
				return false
			}
		}
		return len(prop.Properties) == 3 || len(prop.Properties) == 4 && prop.Properties["A"] != nil
	}
	return false
}

// checkShowIf resolves the showIf conditions collected while building out's
// properties against its siblings and sets their ShowIf hint. A condition
// names a sibling by its JSON name: "Field", "!Field", "Field=Value" or
// "Field!=Value".
func (s *schemaBuilder) checkShowIf(out *god.TypeSchema, pending []pendingShowIf) {
	for _, p := range pending {
		field, value, hasValue := strings.Cut(p.cond, "=")
		not := strings.HasPrefix(field, "!")
		field = strings.TrimPrefix(field, "!")
		if hasValue && strings.HasSuffix(field, "!") { // Field!=Value
			field, not = strings.TrimSuffix(field, "!"), true
		}
		field, value = strings.TrimSpace(field), strings.TrimSpace(value)
		sib := out.Properties[field]
		if sib == nil || sib == p.prop {
			s.errorf(p.pos, "%s: +%s showIf %q: %q is not another property of the same struct (use its JSON name)", p.label, markEditor, p.cond, field)
			continue
		}
		cond := &god.ShowIf{Field: field, Not: not}
		if hasValue {
			eq, err := showIfValue(sib, value)
			if err != "" {
				s.errorf(p.pos, "%s: +%s showIf %q: %s", p.label, markEditor, p.cond, err)
				continue
			}
			cond.Equals = eq
		} else if sib.Type != "boolean" && sib.Type != "integer" && sib.Type != "number" && sib.Type != "string" {
			s.errorf(p.pos, "%s: +%s showIf %q: %q is not a boolean, number or string", p.label, markEditor, p.cond, field)
			continue
		}
		p.prop.XEditor.ShowIf = cond
	}
}

// showIfValue is the JSON value text names for the sibling schema sib: an
// enum constant name or an integer for an integer, true or false for a
// boolean, a number, or a string.
func showIfValue(sib *god.TypeSchema, text string) (json.RawMessage, string) {
	switch {
	case text == "":
		return nil, "missing value after '='"
	case sib.Type == "boolean":
		if text != "true" && text != "false" {
			return nil, "a boolean is compared with true or false"
		}
		return json.RawMessage(text), ""
	case sib.Type == "integer" && len(sib.XEnum) > 0:
		if i := slices.IndexFunc(sib.XEnum, func(e god.EnumValue) bool { return e.Name == text }); i >= 0 {
			return json.RawMessage(strconv.FormatInt(sib.XEnum[i].Value, 10)), ""
		}
		if n, err := strconv.ParseInt(text, 0, 64); err == nil && slices.ContainsFunc(sib.XEnum, func(e god.EnumValue) bool { return e.Value == n }) {
			return json.RawMessage(strconv.FormatInt(n, 10)), ""
		}
		return nil, "\"" + text + "\" is not a constant of the sibling's enum"
	case sib.Type == "integer":
		n, err := strconv.ParseInt(text, 0, 64)
		if err != nil {
			return nil, "\"" + text + "\" is not an integer"
		}
		return json.RawMessage(strconv.FormatInt(n, 10)), ""
	case sib.Type == "number":
		n, err := strconv.ParseFloat(text, 64)
		if err != nil || math.IsNaN(n) || math.IsInf(n, 0) {
			return nil, "\"" + text + "\" is not a number"
		}
		return json.RawMessage(strconv.FormatFloat(n, 'g', -1, 64)), ""
	case sib.Type == "string" && sib.Format == "":
		data, _ := json.Marshal(strings.Trim(text, `"`))
		return data, ""
	}
	return nil, "the sibling is not a boolean, number, enum or string"
}
