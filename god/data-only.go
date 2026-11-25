package god

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"slices"
	"strings"
	"sync"
)

// dataOnly holds the component types and object classes that a GOD file
// names but no package of the running program registers. Nothing here has
// an ECS component type, so storage, views and decoding never see them.
var dataOnly = struct {
	sync.RWMutex
	components map[string]ComponentSchema
	// classes maps a class key to its data-only components (Optional set as
	// the GOD file says). The registered components of the class are in the
	// schema registry.
	classes map[string][]Component
}{components: map[string]ComponentSchema{}, classes: map[string][]Component{}}

// godFile is the part of a GOD file RegisterDataOnly reads.
type godFile struct {
	Version    int               `json:"version"`
	Components []ComponentSchema `json:"components"`
	Objects    []Schema          `json:"objects"`
}

// RegisterDataOnly reads one GOD file (version 0) and records every
// component type and object class it names that the program has not
// registered, as data only: [CheckComponents] accepts such a component key
// and checks its value against the schema of the file, and
// [DataOnlyComponent] returns the schema. Nothing is decoded or put into a
// world: [ObjectComponents], Bind and Serialize skip them as they skip any
// unregistered key.
//
// An editor that opens a prefab without the game's code calls it for each
// GOD file of the project; a game never does. A registered component type or
// object class always wins: an entry of the file for one is ignored, so a
// stale engine entry is harmless. A class that is not registered is
// registered with its registered components only, so [ClassTag] and
// [LookupSchema] work for it; its data-only components are checked from the
// record. Registering the same file again is a no-op; a data-only component
// of an earlier file is replaced by the same key of a later one.
//
// It returns an error, and records nothing, for input that is not a GOD
// file, has another version, or has a component without a group, name or
// schema; all problems are reported together. It is safe for concurrent use.
func RegisterDataOnly(r io.Reader) error {
	var f godFile
	if err := json.NewDecoder(r).Decode(&f); err != nil {
		return fmt.Errorf("god: data-only file: %w", err)
	}
	if f.Version != 0 {
		return fmt.Errorf("god: data-only file: unsupported version %d", f.Version)
	}
	var errs []error
	for i, c := range f.Components {
		switch {
		case c.Group == "" || c.Name == "":
			errs = append(errs, fmt.Errorf("god: data-only file: components[%d] has no group or name", i))
		case c.Schema == nil:
			errs = append(errs, fmt.Errorf("god: data-only file: component %s has no schema", c.Key()))
		}
	}
	if err := errors.Join(errs...); err != nil {
		return err
	}

	dataOnly.Lock()
	defer dataOnly.Unlock()
	for _, c := range f.Components {
		if _, ok := LookupComponent(c.Group, c.Name); ok {
			continue // registered wins
		}
		dataOnly.components[c.Key()] = c
	}
	for _, s := range f.Objects {
		errs = append(errs, registerDataOnlyClass(s))
	}
	return errors.Join(errs...)
}

// registerDataOnlyClass records an object class of a GOD file whose key is
// not registered by the program. The caller holds dataOnly's lock.
func registerDataOnlyClass(s Schema) error {
	key := s.Key()
	var registered, data []Component
	for _, c := range s.Components {
		rc, ok := registeredComponent(c.Key())
		if ok {
			rc.Optional, rc.Readonly, rc.Field, rc.EmbeddedVia = c.Optional, c.Readonly, c.Field, c.EmbeddedVia
			registered = append(registered, rc)
			continue
		}
		if _, ok := dataOnly.components[c.Key()]; !ok {
			return fmt.Errorf("god: data-only class %s: component %s is in no GOD file", key, c.Key())
		}
		data = append(data, c)
	}
	if prev, ok := dataOnly.classes[key]; ok {
		if !slices.EqualFunc(prev, data, func(a, b Component) bool { return a.Key() == b.Key() && a.Optional == b.Optional }) {
			return fmt.Errorf("god: data-only class %s is already registered with a different definition", key)
		}
		return nil
	}
	if _, ok := LookupSchema(s.Group, s.Class); ok {
		return nil // registered wins
	}
	dataOnly.classes[key] = data
	RegisterSchema(Schema{SchemaMeta: s.SchemaMeta, Doc: s.Doc, GoType: s.GoType, Components: registered})
	return nil
}

// DataOnlyComponent returns the schema of the data-only component type with
// key "group:name" (see [RegisterDataOnly]). It reports false for a
// registered component type, which [LookupComponent] returns, and for an
// unknown key. Decode and RuntimeID of the result are unset.
func DataOnlyComponent(key string) (ComponentSchema, bool) {
	if _, ok := registeredComponent(key); ok {
		return ComponentSchema{}, false
	}
	dataOnly.RLock()
	defer dataOnly.RUnlock()
	c, ok := dataOnly.components[key]
	return c, ok
}

// DataOnlyComponents returns the data-only component types, sorted by key.
func DataOnlyComponents() []ComponentSchema {
	dataOnly.RLock()
	defer dataOnly.RUnlock()
	var out []ComponentSchema
	for _, k := range slices.Sorted(maps.Keys(dataOnly.components)) {
		if _, ok := registeredComponent(k); !ok {
			out = append(out, dataOnly.components[k])
		}
	}
	return out
}

// checkDataOnly reports the problems of the data-only part of an object of
// class key: a missing required data-only component and a value that does
// not conform to its schema. known lists the keys of the class, covered
// elsewhere; others returns whether k is accepted as an extra data-only key.
func checkDataOnly(key string, raw map[string]json.RawMessage, known map[string]bool) (errs []error, accepted map[string]bool) {
	dataOnly.RLock()
	defer dataOnly.RUnlock()
	for _, c := range dataOnly.classes[key] {
		known[c.Key()] = true
		if _, ok := raw[c.Key()]; !ok && !c.Optional {
			errs = append(errs, fmt.Errorf("object %s: missing required component %s", key, c.Key()))
		}
	}
	accepted = map[string]bool{}
	for k, v := range raw {
		if _, ok := registeredComponent(k); ok {
			continue
		}
		c, ok := dataOnly.components[k]
		if !ok {
			continue
		}
		accepted[k] = true
		if bytesIsNull(v) {
			continue
		}
		if err := c.Schema.Validate(v); err != nil {
			errs = append(errs, fmt.Errorf("object %s: component %s: %w", key, k, err))
		}
	}
	return errs, accepted
}

func bytesIsNull(v json.RawMessage) bool { return strings.TrimSpace(string(v)) == "null" }
