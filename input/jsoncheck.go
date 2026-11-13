package input

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strconv"
	"strings"
)

// checkJSON walks raw against the Go type t and reports every shape problem
// (unknown property, wrong JSON type, null where not allowed) with a GOD-style
// path ("$.mappings.menu.block: expected boolean, got string"). It follows
// the encoding/json rules for field names. json.Unmarshal stops at the first
// problem; checking first is what lets a bad file report all of them.
func checkJSON(raw json.RawMessage, t reflect.Type, path string, errs *[]error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return
	}
	if string(raw) == "null" {
		switch t.Kind() {
		case reflect.Pointer, reflect.Slice, reflect.Map:
			return
		}
		*errs = append(*errs, fmt.Errorf("%s: null is not allowed", path))
		return
	}
	got := jsonKind(raw)
	switch t.Kind() {
	case reflect.Pointer:
		checkJSON(raw, t.Elem(), path, errs)
	case reflect.Struct:
		if got != "object" {
			*errs = append(*errs, fmt.Errorf("%s: expected object, got %s", path, got))
			return
		}
		var m map[string]json.RawMessage
		if err := json.Unmarshal(raw, &m); err != nil {
			*errs = append(*errs, fmt.Errorf("%s: %v", path, err))
			return
		}
		fields := jsonFields(t)
		for _, k := range sortedKeys(m) {
			f, ok := fields[k]
			if !ok {
				*errs = append(*errs, fmt.Errorf("%s.%s: unknown property %q", path, k, k))
				continue
			}
			checkJSON(m[k], f.Type, path+"."+k, errs)
		}
	case reflect.Map:
		if got != "object" {
			*errs = append(*errs, fmt.Errorf("%s: expected object, got %s", path, got))
			return
		}
		var m map[string]json.RawMessage
		if err := json.Unmarshal(raw, &m); err != nil {
			*errs = append(*errs, fmt.Errorf("%s: %v", path, err))
			return
		}
		for _, k := range sortedKeys(m) {
			checkJSON(m[k], t.Elem(), path+"."+k, errs)
		}
	case reflect.Slice, reflect.Array:
		if got != "array" {
			*errs = append(*errs, fmt.Errorf("%s: expected array, got %s", path, got))
			return
		}
		var items []json.RawMessage
		if err := json.Unmarshal(raw, &items); err != nil {
			*errs = append(*errs, fmt.Errorf("%s: %v", path, err))
			return
		}
		for i, it := range items {
			checkJSON(it, t.Elem(), path+"["+strconv.Itoa(i)+"]", errs)
		}
	case reflect.String:
		if got != "string" {
			*errs = append(*errs, fmt.Errorf("%s: expected string, got %s", path, got))
		}
	case reflect.Bool:
		if got != "boolean" {
			*errs = append(*errs, fmt.Errorf("%s: expected boolean, got %s", path, got))
		}
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64,
		reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64:
		if got != "number" {
			*errs = append(*errs, fmt.Errorf("%s: expected integer, got %s", path, got))
			return
		}
		if _, err := strconv.ParseInt(string(raw), 10, 64); err != nil {
			*errs = append(*errs, fmt.Errorf("%s: expected integer, got %s", path, raw))
		}
	case reflect.Float32, reflect.Float64:
		if got != "number" {
			*errs = append(*errs, fmt.Errorf("%s: expected number, got %s", path, got))
		}
	}
}

func jsonKind(raw json.RawMessage) string {
	switch raw[0] {
	case '{':
		return "object"
	case '[':
		return "array"
	case '"':
		return "string"
	case 't', 'f':
		return "boolean"
	case 'n':
		return "null"
	}
	return "number"
}

// jsonFields maps the JSON names of t's exported fields to the fields.
func jsonFields(t reflect.Type) map[string]reflect.StructField {
	m := map[string]reflect.StructField{}
	for f := range t.Fields() {
		if !f.IsExported() {
			continue
		}
		name := f.Name
		if tag, ok := f.Tag.Lookup("json"); ok {
			n, _, _ := strings.Cut(tag, ",")
			if n == "-" {
				continue
			}
			if n != "" {
				name = n
			}
		}
		m[name] = f
	}
	return m
}
