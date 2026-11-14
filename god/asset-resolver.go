package god

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"sort"
	"sync"

	"github.com/weakpixel/ecs"
)

// AssetResolver converts between the asset keys of serialized data and the
// runtime handles stored in components. The engine implements it on top of
// its asset server; god knows nothing about handle types.
//
// A schema property with Format "asset" and XAsset kind is a string key
// (e.g. "chars/hero.aseprite.json#run") in JSON and a handle in the Go
// field. The kind ("sprite", "clip", "font", ...) is a free string that only
// the resolver interprets.
//
// Lifetime: the interface has no scope parameter. An implementation loads
// into whatever scope it currently holds, so releasing that scope frees what
// a Deserialize call loaded. Deleting an entity releases nothing.
type AssetResolver interface {
	// ResolveAsset loads (or finds) the asset behind path and returns its
	// handle. The result must be assignable to the Go type of the field.
	ResolveAsset(kind, path string) (any, error)

	// AssetKey returns the key of handle h, which is the (non-zero) value
	// of a field of the given kind. It is the inverse of ResolveAsset.
	AssetKey(kind string, h any) (string, error)
}

// SetAssetResolver makes r the world's AssetResolver, used by Deserialize
// and SerializeComponents for components with asset properties. It is a
// world resource of type AssetResolver (an interface), so other code can
// read it with ecs.GetResourceOrNil[types.AssetResolver].
func SetAssetResolver(world *ecs.World, r AssetResolver) {
	ecs.SetResource(world, &r)
}

// worldAssetResolver returns the world's AssetResolver, or nil.
func worldAssetResolver(world *ecs.World) AssetResolver {
	if p := ecs.GetResourceOrNil[AssetResolver](world); p != nil {
		return *p
	}
	return nil
}

// assetField is a component field that holds an asset handle.
type assetField struct {
	name  string // JSON property name, which is also the Go field name
	kind  string
	index int // field index in the component struct
	typ   reflect.Type
}

type assetFieldsKey struct {
	schema *TypeSchema
	typ    reflect.Type
}

type assetFieldsEntry struct {
	fields []assetField
	err    error
}

// assetFieldsCache maps a component's schema and Go type to its asset
// fields, so reflection over the struct runs once per component.
var assetFieldsCache sync.Map // assetFieldsKey -> assetFieldsEntry

// hasAssetField reports whether s has an asset property. Components without
// one take this cheap check (a small map walk, no allocation) and nothing else.
func hasAssetField(s *TypeSchema) bool {
	for _, p := range s.Properties {
		if p != nil && p.XAsset != "" {
			return true
		}
	}
	return false
}

// assetFieldsOf finds the Go fields behind the asset properties of s. Asset
// fields are tagged `json:"-"` (encoding/json ignores them), so the property
// name is the Go field name.
func assetFieldsOf(s *TypeSchema, t reflect.Type) ([]assetField, error) {
	key := assetFieldsKey{s, t}
	if e, ok := assetFieldsCache.Load(key); ok {
		entry := e.(assetFieldsEntry)
		return entry.fields, entry.err
	}
	fields, err := findAssetFields(s, t)
	assetFieldsCache.Store(key, assetFieldsEntry{fields, err})
	return fields, err
}

func findAssetFields(s *TypeSchema, t reflect.Type) ([]assetField, error) {
	if t.Kind() != reflect.Struct {
		return nil, fmt.Errorf("asset properties need a struct component, got %s", t)
	}
	names := make([]string, 0, len(s.Properties))
	for name, p := range s.Properties {
		if p != nil && p.XAsset != "" {
			names = append(names, name)
		}
	}
	sort.Strings(names)

	var fields []assetField
	var errs []error
	for _, name := range names {
		sf, ok := t.FieldByName(name)
		switch {
		case !ok || len(sf.Index) != 1 || !sf.IsExported():
			errs = append(errs, fmt.Errorf("$.%s: %s has no exported field %s for the asset property", name, t, name))
		case sf.Type.Kind() != reflect.Struct:
			errs = append(errs, fmt.Errorf("$.%s: asset field must be a struct handle, got %s", name, sf.Type))
		default:
			fields = append(fields, assetField{name: name, kind: s.Properties[name].XAsset, index: sf.Index[0], typ: sf.Type})
		}
	}
	return fields, errors.Join(errs...)
}

// decodeAssets resolves the asset properties of raw into the matching fields
// of v, the value Decode produced from raw. Absent and empty keys leave the
// zero handle. It returns v itself when the component has no asset property.
func decodeAssets(resolver AssetResolver, s *TypeSchema, raw json.RawMessage, v any) (any, error) {
	if !hasAssetField(s) {
		return v, nil
	}
	rv := reflect.New(reflect.TypeOf(v)).Elem() // addressable copy; v is not settable
	rv.Set(reflect.ValueOf(v))
	fields, err := assetFieldsOf(s, rv.Type())
	if err != nil {
		return nil, err
	}
	var props map[string]json.RawMessage
	if err := json.Unmarshal(raw, &props); err != nil {
		return nil, err
	}
	var errs []error
	for _, f := range fields {
		data, ok := props[f.name]
		if !ok {
			continue
		}
		var key string
		if err := json.Unmarshal(data, &key); err != nil {
			errs = append(errs, fmt.Errorf("$.%s: %w", f.name, err))
			continue
		}
		if key == "" {
			continue
		}
		if resolver == nil {
			errs = append(errs, fmt.Errorf("$.%s: cannot resolve %s asset %q: no AssetResolver set (see SetAssetResolver)", f.name, f.kind, key))
			continue
		}
		h, err := resolver.ResolveAsset(f.kind, key)
		switch {
		case err != nil:
			errs = append(errs, fmt.Errorf("$.%s: resolve %s asset %q: %w", f.name, f.kind, key, err))
		case h == nil || !reflect.TypeOf(h).AssignableTo(f.typ):
			errs = append(errs, fmt.Errorf("$.%s: resolver returned %T for %s asset %q, want %s", f.name, h, f.kind, key, f.typ))
		default:
			rv.Field(f.index).Set(reflect.ValueOf(h))
		}
	}
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}
	return rv.Interface(), nil
}

// encodeAssets adds the asset keys of component value v to data, the JSON
// that encoding/json produced for v. Zero handles are omitted. It returns
// data itself when the component has no asset property.
func encodeAssets(world *ecs.World, s *TypeSchema, v any, data []byte) ([]byte, error) {
	if !hasAssetField(s) {
		return data, nil
	}
	rv := reflect.Indirect(reflect.ValueOf(v))
	fields, err := assetFieldsOf(s, rv.Type())
	if err != nil {
		return nil, err
	}
	var props map[string]json.RawMessage
	if err := json.Unmarshal(data, &props); err != nil {
		return nil, err
	}
	var errs []error
	for _, f := range fields {
		h := rv.Field(f.index)
		if h.IsZero() {
			continue
		}
		resolver := worldAssetResolver(world)
		if resolver == nil {
			errs = append(errs, fmt.Errorf("$.%s: cannot write %s asset: no AssetResolver set (see SetAssetResolver)", f.name, f.kind))
			continue
		}
		key, err := resolver.AssetKey(f.kind, h.Interface())
		if err != nil {
			errs = append(errs, fmt.Errorf("$.%s: asset key of %s handle: %w", f.name, f.kind, err))
			continue
		}
		props[f.name], _ = json.Marshal(key)
	}
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}
	return json.Marshal(props)
}
