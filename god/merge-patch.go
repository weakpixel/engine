package god

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
)

// mergePatch applies the override patch to base, the JSON of one component
// (or of a value inside one): when both are JSON objects they merge key by
// key, recursively; anything else in patch replaces base. Unlike RFC 7386,
// null in patch sets null (nullable fields stay writable) and never deletes
// a key. The result is compact JSON with sorted keys.
func mergePatch(base, patch json.RawMessage) (json.RawMessage, error) {
	if !isJSONObject(base) || !isJSONObject(patch) {
		return compactJSON(patch)
	}
	var b, p map[string]json.RawMessage
	if err := json.Unmarshal(base, &b); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(patch, &p); err != nil {
		return nil, err
	}
	for k, pv := range p {
		bv, ok := b[k]
		if !ok {
			b[k] = pv
			continue
		}
		m, err := mergePatch(bv, pv)
		if err != nil {
			return nil, err
		}
		b[k] = m
	}
	return json.Marshal(b)
}

// diffPatch is the inverse of mergePatch: the smallest patch that turns base
// into value, and whether there is any difference. Sub-objects merge, so
// only differing keys are kept; arrays and scalars are compared by value
// (1 equals 1.0) and replaced whole. s, the schema of base and value (nil
// if unknown), supplies the zero JSON of a property value lacks (an
// omitempty field, an asset field holding the zero handle), since a merge
// patch cannot delete a key. A map entry that value lacks cannot be
// expressed and is an error.
func diffPatch(s *TypeSchema, base, value json.RawMessage) (json.RawMessage, bool, error) {
	if !isJSONObject(base) || !isJSONObject(value) {
		equal, err := jsonEqual(base, value)
		if err != nil || equal {
			return nil, false, err
		}
		v, err := compactJSON(value)
		return v, true, err
	}
	var b, v map[string]json.RawMessage
	if err := json.Unmarshal(base, &b); err != nil {
		return nil, false, err
	}
	if err := json.Unmarshal(value, &v); err != nil {
		return nil, false, err
	}
	patch := make(map[string]json.RawMessage)
	var errs []error
	for _, k := range sortedKeys(b) {
		if _, ok := v[k]; ok {
			continue
		}
		ps := propertySchema(s, k)
		if s != nil && s.AdditionalProperties != nil && s.Properties[k] == nil {
			errs = append(errs, fmt.Errorf("$.%s: a map entry of the prefab cannot be removed by an override", k))
			continue
		}
		zero := zeroJSON(ps)
		if equal, err := jsonEqual(b[k], zero); err == nil && !equal {
			patch[k] = zero
		}
	}
	for _, k := range sortedKeys(v) {
		bv, ok := b[k]
		if !ok {
			patch[k] = v[k]
			continue
		}
		d, changed, err := diffPatch(propertySchema(s, k), bv, v[k])
		if err != nil {
			errs = append(errs, prefixPath(k, err))
			continue
		}
		if changed {
			patch[k] = d
		}
	}
	if err := errors.Join(errs...); err != nil {
		return nil, false, err
	}
	if len(patch) == 0 {
		return nil, false, nil
	}
	out, err := json.Marshal(patch)
	return out, true, err
}

// propertySchema returns the schema of property k of object schema s, or
// nil if unknown.
func propertySchema(s *TypeSchema, k string) *TypeSchema {
	if s == nil {
		return nil
	}
	if p := s.Properties[k]; p != nil {
		return p
	}
	return s.AdditionalProperties
}

// zeroJSON is the JSON of a zero Go value of schema s: what a field that
// encoding/json left out (omitempty) or an empty asset key stands for.
func zeroJSON(s *TypeSchema) json.RawMessage {
	switch {
	case s == nil || s.Nullable:
		return json.RawMessage("null")
	case s.Type == "string":
		return json.RawMessage(`""`)
	case s.Type == "number" || s.Type == "integer":
		return json.RawMessage("0")
	case s.Type == "boolean":
		return json.RawMessage("false")
	case s.Type == "array":
		return json.RawMessage("[]")
	}
	return json.RawMessage("null")
}

// prefixPath puts property k in front of a "$..." error path, in each error
// of a joined one.
func prefixPath(k string, err error) error {
	if j, ok := err.(interface{ Unwrap() []error }); ok {
		list := j.Unwrap()
		out := make([]error, len(list))
		for i, e := range list {
			out[i] = prefixPath(k, e)
		}
		return errors.Join(out...)
	}
	msg := err.Error()
	if len(msg) > 0 && msg[0] == '$' {
		return errors.New("$." + k + msg[1:])
	}
	return fmt.Errorf("$.%s: %w", k, err)
}

// jsonEqual compares two JSON values by meaning, not by spelling.
func jsonEqual(a, b json.RawMessage) (bool, error) {
	var x, y any
	if err := json.Unmarshal(a, &x); err != nil {
		return false, err
	}
	if err := json.Unmarshal(b, &y); err != nil {
		return false, err
	}
	return reflect.DeepEqual(x, y), nil
}

func isJSONObject(raw json.RawMessage) bool {
	t := bytes.TrimLeft(raw, " \t\r\n")
	return len(t) > 0 && t[0] == '{'
}

func compactJSON(raw json.RawMessage) (json.RawMessage, error) {
	var buf bytes.Buffer
	if err := json.Compact(&buf, raw); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
