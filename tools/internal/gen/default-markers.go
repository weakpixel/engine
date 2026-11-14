package gen

import (
	"encoding/json"
	"fmt"
	"go/constant"
	"go/token"
	"go/types"
	"math"
	"reflect"
	"regexp"
	"strconv"
	"strings"

	"github.com/weakpixel/engine/god"
	"github.com/weakpixel/engine/internal/marker"
)

// defaultSetting is one value of a +game:default marker: the line it is on,
// the dotted path ("" for the field itself) and the Go expression text.
type defaultSetting struct {
	pos   token.Pos
	path  string
	value string
}

// defaultSettings collects the values of a field's +game:default lines, in
// order. The bare and the path form cannot be mixed, and a path can be set
// only once; both are reported.
func (s *schemaBuilder) defaultSettings(label string, lines []marker.Marker) []defaultSetting {
	settings, issues := marker.PathSettings(lines)
	for _, is := range issues {
		switch is.Kind {
		case marker.PathDuplicate:
			s.errorf(is.Pos, "%s: +%s sets %s more than once", label, markDefault, describePath(is.Path))
		case marker.PathMixedForms:
			s.errorf(is.Pos, "%s: +%s mixes the field form (=value) with the path form (:path=value)", label, markDefault)
		}
	}
	out := make([]defaultSetting, len(settings))
	for i, d := range settings {
		out[i] = defaultSetting{d.Pos, d.Path, d.Value}
	}
	return out
}

func describePath(path string) string {
	if path == "" {
		return "the field"
	}
	return "path " + path
}

// applyDefaults sets the defaults declared by +game:default on field v, whose
// schema prop was just built. Problems are collected in s.diags.
func (s *schemaBuilder) applyDefaults(v *types.Var, prop *god.TypeSchema, lines []marker.Marker) {
	s.pkg, s.pos = v.Pkg(), v.Pos() // the scope the values are evaluated in
	label := s.fieldLabel(v)
	settings := s.defaultSettings(label, lines)
	if len(settings) == 0 {
		return
	}
	if reason := notDescribed(prop); reason != "" {
		s.errorf(lines[0].Pos, "%s: +%s is not allowed on %s", label, markDefault, reason)
		return
	}
	mapEntries := map[*god.TypeSchema]map[string]json.RawMessage{}
	for _, d := range settings {
		s.applySetting(v, label, prop, d, mapEntries)
	}
	for sch, entries := range mapEntries {
		raw, err := json.Marshal(entries)
		if err == nil {
			err = sch.Validate(raw)
		}
		if err != nil {
			s.errorf(lines[0].Pos, "%s: +%s: %s", label, markDefault, trimPath(err))
			continue
		}
		sch.Default = raw
	}
}

// notDescribed names why a schema cannot carry a default, or returns "".
func notDescribed(sch *god.TypeSchema) string {
	switch {
	case sch.Format == god.FormatEntity:
		return "an entity reference (ids belong to one document)"
	case sch.Format == "asset" || sch.XAsset != "":
		return "an asset field (a new component starts without an asset)"
	case sch.XRuntime:
		return "a runtime field (not authored data)"
	case sch.PreserveUnknownFields || sch.GoType != "":
		return "a value whose JSON shape is not described"
	}
	return ""
}

// applySetting resolves one path against the field's type and schema and
// sets the default of the leaf it names. Map keys collect in mapEntries.
func (s *schemaBuilder) applySetting(v *types.Var, label string, prop *god.TypeSchema, d defaultSetting, mapEntries map[*god.TypeSchema]map[string]json.RawMessage) {
	fail := func(format string, args ...any) {
		s.errorf(d.pos, "%s: +%s%s: %s", label, markDefault, pathSuffix(d.path), fmt.Sprintf(format, args...))
	}
	t, sch := unwrapPointer(v.Type()), prop
	var segs []string
	if d.path != "" {
		segs = strings.Split(d.path, ".")
	}
	for i, seg := range segs {
		last := i == len(segs)-1
		switch u := t.Underlying().(type) {
		case *types.Struct:
			f, name, hidden := findJSONField(u, seg)
			child := sch.Properties[name]
			switch {
			case f == nil:
				fail("%s has no field %s", types.TypeString(t, s.qualifier(v)), seg)
				return
			case hidden || child == nil:
				fail("field %s is not part of the schema", seg)
				return
			}
			if reason := notDescribed(child); reason != "" {
				fail("not allowed on %s", reason)
				return
			}
			t, sch = unwrapPointer(f.Type()), child
		case *types.Map:
			if !last {
				fail("a map of structs or maps cannot be set by path (key %s)", seg)
				return
			}
			key, ok := u.Key().Underlying().(*types.Basic)
			if !ok || key.Info()&types.IsInteger != 0 && !isInteger(seg) {
				fail("key %q is not a valid %s", seg, u.Key())
				return
			}
			raw, ok := s.defaultValue(d, label, unwrapPointer(u.Elem()), sch.AdditionalProperties, d.value)
			if ok {
				if mapEntries[sch] == nil {
					mapEntries[sch] = map[string]json.RawMessage{}
				}
				mapEntries[sch][seg] = raw
			}
			return
		default:
			fail("cannot select %s in %s", seg, kindName(t))
			return
		}
	}
	if raw, ok := s.defaultValue(d, label, t, sch, d.value); ok {
		sch.Default = raw
	}
}

func pathSuffix(path string) string {
	if path == "" {
		return ""
	}
	return ":" + path
}

func isInteger(s string) bool {
	_, err := strconv.ParseInt(s, 10, 64)
	return err == nil
}

// kindName describes t for a diagnostic.
func kindName(t types.Type) string {
	switch t.Underlying().(type) {
	case *types.Slice, *types.Array:
		return "a slice or array"
	case *types.Map:
		return "a map"
	case *types.Struct:
		return "a struct"
	}
	return "a scalar"
}

// findJSONField finds the struct field called name (a Go name), looking into
// embedded structs without a json name like encoding/json. jsonName is the
// property name in the schema; hidden reports a json:"-" or unexported field.
func findJSONField(st *types.Struct, name string) (f *types.Var, jsonName string, hidden bool) {
	for i := range st.NumFields() {
		v := st.Field(i)
		tagName, _, _ := strings.Cut(jsonTag(st, i), ",")
		if v.Name() == name {
			jsonName = tagName
			if jsonName == "" {
				jsonName = name
			}
			return v, jsonName, jsonTag(st, i) == "-" || !v.Exported()
		}
		if inner, ok := structOf(v.Type()); ok && v.Anonymous() && tagName == "" {
			if f, jsonName, hidden := findJSONField(inner, name); f != nil {
				return f, jsonName, hidden
			}
		}
	}
	return nil, "", false
}

// defaultValue evaluates the text of one setting against leaf type t and
// returns its JSON, checked against the leaf schema sch. t is a scalar, or a
// slice or array of scalars written as a list.
func (s *schemaBuilder) defaultValue(d defaultSetting, label string, t types.Type, sch *god.TypeSchema, text string) (json.RawMessage, bool) {
	fail := func(format string, args ...any) (json.RawMessage, bool) {
		s.errorf(d.pos, "%s: +%s%s: %s", label, markDefault, pathSuffix(d.path), fmt.Sprintf(format, args...))
		return nil, false
	}
	var raw json.RawMessage
	switch u := t.Underlying().(type) {
	case *types.Basic:
		out, err := s.scalarJSON(t, text)
		if err != nil {
			return fail("%v", err)
		}
		raw = json.RawMessage(out)
	case *types.Slice, *types.Array:
		elem := u.(interface{ Elem() types.Type }).Elem()
		elem = unwrapPointer(elem)
		if _, ok := elem.Underlying().(*types.Basic); !ok || sch.Format == "byte" {
			return fail("a list of %s cannot be written (elements must be scalars)", types.TypeString(elem, func(p *types.Package) string { return p.Name() }))
		}
		items, err := marker.SplitList(text)
		if err != nil {
			return fail("%v", err)
		}
		if arr, ok := u.(*types.Array); ok && int64(len(items)) != arr.Len() {
			return fail("the array has %d elements, the list %d", arr.Len(), len(items))
		}
		outs := make([]string, len(items))
		for i, item := range items {
			if outs[i], err = s.scalarJSON(elem, item); err != nil {
				return fail("element %d: %v", i, err)
			}
		}
		raw = json.RawMessage("[" + strings.Join(outs, ",") + "]")
	default:
		return fail("%s needs its fields set by path (+%s:name=value)", kindName(t), markDefault)
	}
	if err := sch.Validate(raw); err != nil {
		return fail("%s", trimPath(err))
	}
	return raw, true
}

// trimPath drops the "$: " root path from a Validate error.
func trimPath(err error) string {
	return strings.ReplaceAll(err.Error(), "$: ", "")
}

// evalPosition is the "eval:1:3: " prefix types.Eval puts on its errors.
var evalPosition = regexp.MustCompile(`^eval:\d+:\d+: `)

// scalarJSON evaluates text as a Go constant expression in the scope of the
// field being processed and returns its JSON, as the type t would encode it.
func (s *schemaBuilder) scalarJSON(t types.Type, text string) (string, error) {
	tv, err := types.Eval(s.l.fset, s.pkg, s.pos, text)
	if err != nil {
		return "", fmt.Errorf("%s", evalPosition.ReplaceAllString(err.Error(), ""))
	}
	if tv.Value == nil {
		return "", fmt.Errorf("%s is not a constant", text)
	}
	if b, ok := tv.Type.(*types.Basic); !ok || b.Info()&types.IsUntyped == 0 {
		if !types.AssignableTo(tv.Type, t) {
			q := func(p *types.Package) string { return p.Name() }
			return "", fmt.Errorf("%s has type %s, want %s", text, types.TypeString(tv.Type, q), types.TypeString(t, q))
		}
	}
	return constantJSON(tv.Value, t.Underlying().(*types.Basic), text, t)
}

// constantJSON encodes val for basic type b, or reports why it does not fit.
func constantJSON(val constant.Value, b *types.Basic, text string, t types.Type) (string, error) {
	cannot := fmt.Errorf("%s cannot be used as %s", text, t)
	switch info := b.Info(); {
	case info&types.IsBoolean != 0:
		if val.Kind() != constant.Bool {
			return "", cannot
		}
		return strconv.FormatBool(constant.BoolVal(val)), nil
	case info&types.IsString != 0:
		if val.Kind() != constant.String {
			return "", cannot
		}
		out, _ := json.Marshal(constant.StringVal(val))
		return string(out), nil
	case info&types.IsInteger != 0:
		iv := constant.ToInt(val)
		if iv.Kind() != constant.Int {
			return "", cannot
		}
		bits := map[types.BasicKind]int{
			types.Int8: 8, types.Uint8: 8, types.Int16: 16, types.Uint16: 16,
			types.Int32: 32, types.Uint32: 32,
		}[b.Kind()]
		if bits == 0 {
			bits = 64
		}
		var lo, hi constant.Value
		if info&types.IsUnsigned != 0 {
			lo, hi = constant.MakeInt64(0), constant.Shift(constant.MakeInt64(1), token.SHL, uint(bits))
			hi = constant.BinaryOp(hi, token.SUB, constant.MakeInt64(1))
		} else {
			hi = constant.Shift(constant.MakeInt64(1), token.SHL, uint(bits-1))
			lo = constant.UnaryOp(token.SUB, hi, 0)
			hi = constant.BinaryOp(hi, token.SUB, constant.MakeInt64(1))
		}
		if constant.Compare(iv, token.LSS, lo) || constant.Compare(iv, token.GTR, hi) {
			return "", fmt.Errorf("%s overflows %s", text, t)
		}
		return iv.ExactString(), nil
	case info&types.IsFloat != 0:
		fv := constant.ToFloat(val)
		if fv.Kind() != constant.Float && fv.Kind() != constant.Int {
			return "", cannot
		}
		if b.Kind() == types.Float32 {
			f, _ := constant.Float32Val(fv)
			if math.IsInf(float64(f), 0) {
				return "", fmt.Errorf("%s overflows %s", text, t)
			}
			return strconv.FormatFloat(float64(f), 'g', -1, 32), nil
		}
		f, _ := constant.Float64Val(fv)
		if math.IsInf(f, 0) {
			return "", fmt.Errorf("%s overflows %s", text, t)
		}
		return strconv.FormatFloat(f, 'g', -1, 64), nil
	}
	return "", cannot
}

// unwrapPointer unwraps one level of pointer.
func unwrapPointer(t types.Type) types.Type {
	t = types.Unalias(t)
	if ptr, ok := t.(*types.Pointer); ok {
		return types.Unalias(ptr.Elem())
	}
	return t
}

// fieldLabel names field v for diagnostics: pkg.Type.Field of the innermost
// named type being built.
func (s *schemaBuilder) fieldLabel(v *types.Var) string {
	if n := len(s.stack); n > 0 {
		obj := s.stack[n-1].Obj()
		return obj.Pkg().Name() + "." + obj.Name() + "." + v.Name()
	}
	return v.Name()
}

func (s *schemaBuilder) qualifier(v *types.Var) types.Qualifier {
	return types.RelativeTo(v.Pkg())
}

func (s *schemaBuilder) errorf(pos token.Pos, format string, args ...any) {
	s.diags = append(s.diags, diag{Pos: pos, Msg: fmt.Sprintf(format, args...)})
}

// fieldMarkers parses the markers of field v (empty when its syntax was not
// loaded). Marker syntax errors are reported.
func (s *schemaBuilder) fieldMarkers(v *types.Var) marker.Set {
	field := s.l.fields[v.Pos()]
	if field == nil {
		return marker.Set{}
	}
	markers, diags := registry.Collect(field.Doc, marker.DescribesField)
	s.diags = append(s.diags, diags...)
	return markers
}

// fieldDefaults returns the +game:default lines of markers (nil when none).
func fieldDefaults(markers marker.Set) []marker.Marker {
	return markers.Lines(markDefault)
}

// rejectDefault reports a +game:default on field v that cannot take effect.
func (s *schemaBuilder) rejectDefault(v *types.Var, lines []marker.Marker, reason string) {
	if len(lines) > 0 {
		s.errorf(lines[0].Pos, "%s: +%s is not allowed on %s", s.fieldLabel(v), markDefault, reason)
	}
}

// jsonTag is the json struct tag of field i of st.
func jsonTag(st *types.Struct, i int) string {
	return reflect.StructTag(st.Tag(i)).Get("json")
}
