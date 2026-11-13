package god

import (
	"encoding/json"
	"fmt"
	"iter"
	"maps"
	"reflect"
	"slices"
	"strings"
	"sync"

	"github.com/weakpixel/ecs"
)

// ComponentSchema describes a component type: its identity and its JSON
// form. Every object that uses the type refers to it by Key(); the GOD lists
// each component type once (its "components").
type ComponentSchema struct {
	// Group and Name identify the component type. Name defaults to the Go
	// type name, Group to the declaring package's group (see the
	// +game:component marker). Neither contains ':'.
	Group string `json:"group"`
	Name  string `json:"name"`

	// Doc is the doc comment of the Go type. Documentation only.
	Doc string `json:"doc,omitempty"`

	// GoType is the Go type, "importpath.Type". Documentation only.
	GoType string `json:"goType,omitempty"`

	// Schema describes the JSON form of the component.
	Schema *TypeSchema `json:"schema"`

	// RuntimeID is the runtime ID of the component type. It is only valid
	// for the current running instance of the program and is not persisted.
	RuntimeID ecs.ComponentID `json:"-"`

	// Decode unmarshals JSON into a value of the component type T (not *T).
	Decode func(json.RawMessage) (any, error) `json:"-"`
}

// ComponentKey returns the key "group:name" of a component type.
func ComponentKey(group, name string) string { return group + ":" + name }

// Key returns the component's key "group:name".
func (c ComponentSchema) Key() string { return ComponentKey(c.Group, c.Name) }

var componentRegistry = struct {
	sync.RWMutex
	byKey map[string]ComponentSchema
}{byKey: map[string]ComponentSchema{}}

// RegisterComponent registers a component type under its key. Generated
// code calls it from init for every component type a package declares or
// uses, so the same type is often registered by several packages:
// registering an identical definition again is a no-op (Doc, schema
// descriptions and Decode are not compared).
//
// Panics with "god: component without a group or name" if Group or Name
// is empty, "god: component \"<key>\": group and name must not contain ':'",
// "god: component \"<key>\" has no schema" or "... has no decoder" for a nil
// Schema or Decode, and "god: component \"<key>\" registered twice with
// different definitions" when the key is taken by a different type or
// schema.
func RegisterComponent(c ComponentSchema) {
	key := c.Key()
	switch {
	case c.Group == "" || c.Name == "":
		panic(fmt.Sprintf("god: component without a group or name (%q)", key))
	case strings.Contains(c.Group, ":") || strings.Contains(c.Name, ":"):
		panic(fmt.Sprintf("god: component %q: group and name must not contain ':'", key))
	case c.Schema == nil:
		panic(fmt.Sprintf("god: component %q has no schema", key))
	case c.Decode == nil:
		panic(fmt.Sprintf("god: component %q has no decoder", key))
	}

	componentRegistry.Lock()
	defer componentRegistry.Unlock()
	if prev, exists := componentRegistry.byKey[key]; exists {
		if prev.GoType != c.GoType || prev.RuntimeID != c.RuntimeID ||
			!reflect.DeepEqual(prev.Schema.WithoutDescriptions(), c.Schema.WithoutDescriptions()) {
			panic(fmt.Sprintf("god: component %q registered twice with different definitions", key))
		}
		return
	}
	componentRegistry.byKey[key] = c
}

// LookupComponent returns the component type registered as group:name.
func LookupComponent(group, name string) (ComponentSchema, bool) {
	componentRegistry.RLock()
	defer componentRegistry.RUnlock()
	c, ok := componentRegistry.byKey[ComponentKey(group, name)]
	return c, ok
}

// Components returns every registered component type, sorted by key, used
// by an object or not. Like [Schemas] it iterates a snapshot.
func Components() iter.Seq[ComponentSchema] {
	return func(yield func(ComponentSchema) bool) {
		componentRegistry.RLock()
		list := slices.Collect(maps.Values(componentRegistry.byKey))
		componentRegistry.RUnlock()
		slices.SortFunc(list, func(a, b ComponentSchema) int { return strings.Compare(a.Key(), b.Key()) })
		for _, c := range list {
			if !yield(c) {
				return
			}
		}
	}
}

// componentByType returns the registered component type whose values have
// Go type t. It decodes null for every registered type, so it is slow; Spawn
// caches what it finds.
func componentByType(t reflect.Type) (ComponentSchema, bool) {
	for c := range Components() {
		if zero, err := c.Decode(json.RawMessage("null")); err == nil && reflect.TypeOf(zero) == t {
			return c, true
		}
	}
	return ComponentSchema{}, false
}
