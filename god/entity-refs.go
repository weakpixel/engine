package god

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"sync"

	"github.com/weakpixel/ecs"
)

// Entity references. A component field of type ecs.Entity has the schema
// {"type":"string","format":"entity","nullable":true}: in a document it is
// the id of another object of the same document, or null for the zero
// Entity. encoding/json knows nothing about ids, so the codec rewrites the
// JSON between Decode/Marshal and the document: ids become
// {"ID":n,"Generation":g} (what encoding/json reads into an ecs.Entity) on the
// way in, and back on the way out. The rewrite is driven by the TypeSchema,
// not by reflection.

// refsCache maps a schema to whether it contains an entity reference, so
// components without one skip the rewrite after a single walk.
var refsCache sync.Map // *TypeSchema -> bool

// hasEntityRef reports whether s or anything below it has Format entity.
func hasEntityRef(s *TypeSchema) bool {
	if s == nil {
		return false
	}
	if v, ok := refsCache.Load(s); ok {
		return v.(bool)
	}
	has := s.Format == FormatEntity || hasEntityRef(s.Items) || hasEntityRef(s.AdditionalProperties)
	for _, p := range s.Properties {
		has = has || hasEntityRef(p)
	}
	refsCache.Store(s, has)
	return has
}

// rewriteRefs replaces every entity reference in v (a value decoded with
// UseNumber) that s describes with fn's result, and reports every failure
// with its $-path.
func rewriteRefs(s *TypeSchema, path string, v any, fn func(path string, v any) (any, error), errs *[]error) any {
	if s == nil || (v == nil && s.Format != FormatEntity) {
		return v
	}
	if s.Format == FormatEntity {
		out, err := fn(path, v)
		if err != nil {
			*errs = append(*errs, fmt.Errorf("%s: %w", path, err))
		}
		return out
	}
	switch val := v.(type) {
	case map[string]any:
		for k, item := range val {
			p := s.Properties[k]
			if p == nil {
				p = s.AdditionalProperties
			}
			if hasEntityRef(p) {
				val[k] = rewriteRefs(p, path+"."+k, item, fn, errs)
			}
		}
	case []any:
		if hasEntityRef(s.Items) {
			for i, item := range val {
				val[i] = rewriteRefs(s.Items, path+"["+strconv.Itoa(i)+"]", item, fn, errs)
			}
		}
	}
	return v
}

// mapRefs decodes data, rewrites its entity references with fn and encodes it
// again. Numbers keep their exact text (UseNumber).
func mapRefs(s *TypeSchema, data []byte, fn func(path string, v any) (any, error)) ([]byte, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var v any
	if err := dec.Decode(&v); err != nil {
		return nil, err
	}
	var errs []error
	v = rewriteRefs(s, "$", v, fn, &errs)
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}
	return json.Marshal(v)
}

// errNoRefs is reported for a non-null reference when no Refs was given.
var errNoRefs = errors.New("no Refs for entity reference")

// refsToEntities turns document ids into the JSON form of ecs.Entity.
func refsToEntities(s *TypeSchema, raw []byte, refs Refs) ([]byte, error) {
	return mapRefs(s, raw, func(_ string, v any) (any, error) {
		if v == nil {
			return nil, nil
		}
		id, ok := v.(string)
		if !ok {
			return nil, fmt.Errorf("expected entity id string, got %s", kind(v))
		}
		if refs == nil {
			return nil, errNoRefs
		}
		e, err := refs.RefEntity(id)
		if err != nil {
			return nil, err
		}
		return map[string]any{"ID": e.ID, "Generation": e.Generation}, nil
	})
}

// entitiesToRefs turns the JSON form of ecs.Entity into document ids; the
// zero Entity becomes null.
func entitiesToRefs(s *TypeSchema, data []byte, refs Refs) ([]byte, error) {
	return mapRefs(s, data, func(_ string, v any) (any, error) {
		if v == nil {
			return nil, nil
		}
		e, err := entityOf(v)
		if err != nil {
			return nil, err
		}
		if e == (ecs.Entity{}) {
			return nil, nil
		}
		if refs == nil {
			return nil, errNoRefs
		}
		return refs.RefID(e)
	})
}

// entityOf reads {"ID":n,"Generation":g} as encoding/json wrote it.
func entityOf(v any) (ecs.Entity, error) {
	m, ok := v.(map[string]any)
	if !ok {
		return ecs.Entity{}, fmt.Errorf("expected an encoded ecs.Entity, got %s", kind(v))
	}
	id, err1 := strconv.ParseUint(fmt.Sprint(m["ID"]), 10, 32)
	gen, err2 := strconv.ParseUint(fmt.Sprint(m["Generation"]), 10, 32)
	if err := errors.Join(err1, err2); err != nil {
		return ecs.Entity{}, fmt.Errorf("malformed ecs.Entity: %w", err)
	}
	return ecs.Entity{ID: ecs.EntityID(id), Generation: uint32(gen)}, nil
}
