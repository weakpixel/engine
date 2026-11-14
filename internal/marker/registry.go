package marker

import (
	"cmp"
	"errors"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"strings"
)

// Registry holds marker definitions keyed by their full name (`game:object`).
type Registry struct {
	defs   map[string]*Definition
	prefix string   // WithPrefix
	names  []string // defs' names, longest first; nil until needed (see match)
}

// NewRegistry returns an empty registry.
func NewRegistry(opts ...Option) *Registry {
	r := &Registry{defs: map[string]*Definition{}}
	for _, opt := range opts {
		opt(r)
	}
	return r
}

// Define registers name for the given targets (one or more ORed TargetTypes).
// obj is a struct, or pointer to a struct, whose exported fields carry
// `marker:"name[,optional]"` tags; Parse returns values of that struct type.
// The marker may appear once per declaration; see DefineRepeatable.
func (r *Registry) Define(name string, target TargetType, obj any, opts ...DefineOption) error {
	return r.define(name, target, obj, false, opts)
}

// DefineRepeatable is Define for a marker that may appear on several lines of
// one declaration. Collect returns the lines in Set.Lines (see Values); the
// WithLinesCheck option is valid here.
func (r *Registry) DefineRepeatable(name string, target TargetType, obj any, opts ...DefineOption) error {
	return r.define(name, target, obj, true, opts)
}

func (r *Registry) define(name string, target TargetType, obj any, repeat bool, opts []DefineOption) error {
	if !validName(name, true) {
		return fmt.Errorf("invalid marker name %q", name)
	}
	if target == 0 || target&^allTargets != 0 {
		return fmt.Errorf("marker %q: invalid target %d", name, int(target))
	}
	if _, dup := r.defs[name]; dup {
		return fmt.Errorf("marker %q is already defined", name)
	}
	def, err := newDefinition(name, target, obj)
	if err != nil {
		return err
	}
	def.repeat = repeat
	for _, opt := range opts {
		if err := opt(def); err != nil {
			return err
		}
	}
	r.defs[name] = def
	r.names = nil
	return nil
}

// DefinePaths registers name for the given targets as a path marker:
// `+name=value` sets the declaration itself and `+name:a.b=v,c=w` sets values
// by dotted path. The names are free, not a fixed argument struct, and values
// are kept as raw text (see PathArgs) for the caller to interpret. A path
// marker may be repeated on one declaration.
func (r *Registry) DefinePaths(name string, target TargetType) error {
	if !validName(name, true) {
		return fmt.Errorf("invalid marker name %q", name)
	}
	if target == 0 || target&^allTargets != 0 {
		return fmt.Errorf("marker %q: invalid target %d", name, int(target))
	}
	if _, dup := r.defs[name]; dup {
		return fmt.Errorf("marker %q is already defined", name)
	}
	r.defs[name] = &Definition{Name: name, Target: target, typ: reflect.TypeFor[PathArgs](), paths: true}
	r.names = nil
	return nil
}

// Lookup returns the definition registered under name.
func (r *Registry) Lookup(name string) (*Definition, bool) {
	def, ok := r.defs[name]
	return def, ok
}

// match finds the definition with the longest name that prefixes body and ends
// at a boundary (end, whitespace, ':' or '='). rest is the text after the name.
// Names are tried longest first, so the first hit wins and the result does not
// depend on map order.
func (r *Registry) match(body string) (def *Definition, rest string) {
	if r.names == nil {
		r.names = slices.SortedFunc(maps.Keys(r.defs), func(a, b string) int {
			return cmp.Or(len(b)-len(a), strings.Compare(a, b))
		})
	}
	for _, name := range r.names {
		rest, ok := strings.CutPrefix(body, name)
		if ok && (rest == "" || strings.ContainsRune(" \t:=", rune(rest[0]))) {
			return r.defs[name], rest
		}
	}
	return nil, ""
}

var errNotMarker = errors.New("marker must start with +")

// validName reports whether s is a valid arg name or, with path, a
// colon-separated marker name.
func validName(s string, path bool) bool {
	if path {
		for seg := range strings.SplitSeq(s, ":") {
			if !validName(seg, false) {
				return false
			}
		}
		return true
	}
	if s == "" {
		return false
	}
	for _, c := range s {
		if !(c == '_' || c == '-' || c >= '0' && c <= '9' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z') {
			return false
		}
	}
	return true
}
