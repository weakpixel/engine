package marker

import (
	"fmt"
	"reflect"
	"strings"
)

// Definition describes one marker: its name, targets and argument struct.
type Definition struct {
	Name   string
	Target TargetType

	typ  reflect.Type
	args []arg

	paths  bool // free-name path arguments (see Registry.DefinePaths)
	repeat bool // struct marker that may repeat (see Registry.DefineRepeatable)

	validate func(any) error             // Validate option; nil when absent
	check    func(lines []Marker) []Diag // WithLinesCheck option; nil when absent
}

// Repeatable reports whether the marker may appear several times on one
// declaration: path markers (Registry.DefinePaths) and markers defined with
// Registry.DefineRepeatable.
func (d *Definition) Repeatable() bool { return d.paths || d.repeat }

// DefineOption configures a definition made by Registry.Define or
// Registry.DefineRepeatable. An option that does not fit the definition makes
// the Define call return an error.
type DefineOption func(*Definition) error

// Validate checks the value of every parsed marker. fn receives the
// definition's struct (T must be exactly that struct type, not a pointer); a
// non-nil error is reported as a Diag at the marker's comment, worded
// `marker +name: <err>`, and the marker is not collected. Report all problems
// of one marker together with errors.Join. fn runs after the arguments parsed,
// so it only sees values the argument types already accepted.
func Validate[T any](fn func(T) error) DefineOption {
	return func(d *Definition) error {
		if d.typ != reflect.TypeFor[T]() {
			return fmt.Errorf("marker %q: Validate takes a func(%s) error, got func(%s) error", d.Name, d.typ, reflect.TypeFor[T]())
		}
		d.validate = func(v any) error { return fn(v.(T)) }
		return nil
	}
}

// WithLinesCheck checks all lines of a repeatable marker together, for rules
// that need the siblings, such as "a slot is used by one marker only". fn runs
// once per Collect, after every line parsed and passed Validate, with only
// those lines in source order; each returned Diag is reported as is. It is an
// error on a marker that is not repeatable.
func WithLinesCheck(fn func(lines []Marker) []Diag) DefineOption {
	return func(d *Definition) error {
		if !d.Repeatable() {
			return fmt.Errorf("marker %q: WithLinesCheck needs a repeatable marker", d.Name)
		}
		d.check = fn
		return nil
	}
}

type arg struct {
	name     string
	index    int
	optional bool
	typ      reflect.Type
}

func newDefinition(name string, target TargetType, obj any) (*Definition, error) {
	typ := reflect.TypeOf(obj)
	if typ != nil && typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	if typ == nil || typ.Kind() != reflect.Struct {
		return nil, fmt.Errorf("marker %q: definition must be a struct or pointer to struct, got %T", name, obj)
	}
	def := &Definition{Name: name, Target: target, typ: typ}
	seen := map[string]bool{}
	for i := range typ.NumField() {
		f := typ.Field(i)
		if !f.IsExported() {
			continue
		}
		a, err := newArg(f)
		if err != nil {
			return nil, fmt.Errorf("marker %q: field %s: %w", name, f.Name, err)
		}
		if seen[a.name] {
			return nil, fmt.Errorf("marker %q: duplicate argument %q", name, a.name)
		}
		seen[a.name] = true
		a.index = i
		def.args = append(def.args, a)
	}
	return def, nil
}

func newArg(f reflect.StructField) (arg, error) {
	tag, ok := f.Tag.Lookup("marker")
	if !ok {
		return arg{}, fmt.Errorf("missing marker tag")
	}
	name, opts, _ := strings.Cut(tag, ",")
	if !validName(name, false) {
		return arg{}, fmt.Errorf("invalid argument name %q in marker tag", name)
	}
	a := arg{name: name, typ: f.Type}
	switch opts {
	case "":
	case "optional":
		a.optional = true
	default:
		return arg{}, fmt.Errorf("unknown marker tag option %q", opts)
	}
	elem := f.Type
	switch elem.Kind() {
	case reflect.Slice:
		elem = elem.Elem()
	case reflect.Pointer:
		// A pointer is nil when the argument is absent, so "not set" differs
		// from the zero value; it is optional by nature.
		if a.optional {
			return arg{}, fmt.Errorf("pointer fields are always optional; drop the optional option")
		}
		a.optional = true
		elem = elem.Elem()
	}
	if !scalarKind(elem.Kind()) {
		return arg{}, fmt.Errorf("unsupported type %s", f.Type)
	}
	return a, nil
}

func scalarKind(k reflect.Kind) bool {
	switch k {
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64,
		reflect.Float32, reflect.Float64,
		reflect.Bool, reflect.String:
		return true
	}
	return false
}

func (d *Definition) arg(name string) *arg {
	for i := range d.args {
		if d.args[i].name == name {
			return &d.args[i]
		}
	}
	return nil
}
