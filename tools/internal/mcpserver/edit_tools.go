package mcpserver

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"

	"github.com/weakpixel/engine/god"
	"github.com/weakpixel/engine/jsonedit"
	"github.com/weakpixel/engine/tools/internal/mcp"
)

// editable are the annotations of a tool that writes a document: not
// destructive in the MCP sense (it changes one value; git undoes it).
var editable = mcp.Annotations{}

// destructive are the annotations of a tool that deletes data.
var destructive = mcp.Annotations{Destructive: true}

// objArgs select a document and the object in it.
type objArgs struct {
	Path   string `json:"path"`
	ID     string `json:"id"`
	Index  *int   `json:"index"`
	Expect string `json:"expect"`
}

// objProps are the properties of objArgs, plus extra.
func objProps(extra map[string]*mcp.Schema) map[string]*mcp.Schema {
	props := map[string]*mcp.Schema{
		"path":   mcp.String("asset key of the document (e.g. forest/forest.level.json)"),
		"id":     mcp.String("the object's id"),
		"index":  mcp.Integer("the object's position in objects, for objects without an id", 0, 1<<30),
		"expect": mcp.String("sha from read_document; the edit is refused if the file changed since"),
	}
	for k, v := range extra {
		props[k] = v
	}
	return props
}

var (
	componentArg = mcp.String("component key (group:Name, e.g. gfx:Transform)")
	fieldArg     = &mcp.Schema{Type: "array", Items: mcp.Any("a member name (string) or an array index (integer)"),
		Description: `path of the field inside the component, e.g. ["Pos","X"] or ["Frames",2,"Dur"]`}
	valueArg = mcp.Any("the JSON value; it must match the component's schema (describe_component)")
)

// locate returns the index and the object selected by a.
func locate(doc *document, a objArgs) (int, docObject, error) {
	i, err := findObject(doc, a.ID, a.Index)
	if err != nil {
		return 0, docObject{}, err
	}
	return i, doc.Objects[i], nil
}

// isNull reports whether raw is the JSON null.
func isNull(raw json.RawMessage) bool { return string(bytes.TrimSpace(raw)) == "null" }

// current returns the value of comp as the object has it: as written for a
// plain object, merged over its prefab for an instance.
func (t *tools) current(info docInfo, obj docObject, comp string) (json.RawMessage, bool, error) {
	if obj.Prefab == "" {
		v, ok := obj.Components[comp]
		return v, ok && !isNull(v), nil
	}
	exp, err := t.expandObject(obj, info.chain())
	if err != nil {
		return nil, false, err
	}
	v, ok := exp.components[comp]
	return v, ok, nil
}

// objectType is the group:class key of obj, from its prefab for an instance.
func (t *tools) objectType(info docInfo, obj docObject) (string, error) {
	if obj.Prefab == "" {
		return obj.classKey(), nil
	}
	exp, err := t.expandObject(obj, info.chain())
	if err != nil {
		return "", err
	}
	return god.SchemaMeta{Group: exp.group, Class: exp.class}.Key(), nil
}

// needRaw is the value of a "value" argument, or an error when missing.
func needRaw(v json.RawMessage) (json.RawMessage, error) {
	if len(bytes.TrimSpace(v)) == 0 {
		return nil, errors.New(`"value" is required`)
	}
	return v, nil
}

// componentPath is the jsonedit path of comp (and field below it) in object i.
func componentPath(i int, comp string, field []any) []any {
	return append([]any{"objects", i, "components", comp}, field...)
}

// setAt writes value at comp.field of object i. On a plain object the
// component must exist; on a prefab instance the write is an override at
// that path, and, as the extension does, a field inside an array writes the
// whole (merged) array, since an override replaces arrays.
func (t *tools) setAt(text []byte, info docInfo, i int, obj docObject, comp string, field []any, value json.RawMessage) ([]byte, error) {
	cur, ok, err := t.current(info, obj, comp)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("object has no component %s (add_component adds it)", comp)
	}
	if obj.Prefab != "" {
		if k := slices.IndexFunc(field, func(s any) bool { _, isInt := s.(int); return isInt }); k >= 0 {
			merged, err := decodeJSON(cur)
			if err != nil {
				return nil, err
			}
			arr, ok := getIn(merged, field[:k])
			if !ok {
				return nil, fmt.Errorf("%s has no field %s", comp, describePath(field[:k]))
			}
			v, err := decodeJSON(value)
			if err != nil {
				return nil, err
			}
			if arr, err = setIn(arr, field[k:], v); err != nil {
				return nil, fmt.Errorf("%s.%s: %w", comp, describePath(field), err)
			}
			whole, err := marshalJSON(arr)
			if err != nil {
				return nil, err
			}
			return jsonedit.Set(text, whole, componentPath(i, comp, field[:k])...)
		}
	}
	return jsonedit.Set(text, value, componentPath(i, comp, field)...)
}

// describePath renders a field path: Pos.X, Frames[2].Dur.
func describePath(path []any) string {
	s := ""
	for _, step := range path {
		switch v := step.(type) {
		case string:
			if s != "" {
				s += "."
			}
			s += v
		case int:
			s += fmt.Sprintf("[%d]", v)
		}
	}
	return s
}

type setValueArgs struct {
	objArgs
	Component string          `json:"component"`
	Field     []any           `json:"field"`
	Value     json.RawMessage `json:"value"`
}

// setValue is the body of set_component, set_field and set_override.
func (t *tools) setValue(a setValueArgs, needField, instanceOnly bool) (any, error) {
	value, err := needRaw(a.Value)
	if err != nil {
		return nil, err
	}
	field, err := fieldPath(a.Field)
	if err != nil {
		return nil, err
	}
	if needField && len(field) == 0 {
		return nil, errors.New(`"field" is required (set_component writes a whole component)`)
	}
	return t.editDocument(a.Path, a.Expect, func(text []byte, doc *document, info docInfo) ([]byte, error) {
		i, obj, err := locate(doc, a.objArgs)
		if err != nil {
			return nil, err
		}
		if instanceOnly && obj.Prefab == "" {
			return nil, fmt.Errorf("object %s is not a prefab instance (set_field and set_component edit plain objects)", obj.label(i))
		}
		out, err := t.setAt(text, info, i, obj, a.Component, field, value)
		if err != nil {
			return nil, fmt.Errorf("object %s: %w", obj.label(i), err)
		}
		return out, nil
	})
}

func (t *tools) setComponent(_ context.Context, a setValueArgs) (any, error) {
	return t.setValue(a, false, false)
}

func (t *tools) setField(_ context.Context, a setValueArgs) (any, error) {
	return t.setValue(a, true, false)
}

func (t *tools) setOverride(_ context.Context, a setValueArgs) (any, error) {
	return t.setValue(a, false, true)
}

type addComponentArgs struct {
	objArgs
	Component string          `json:"component"`
	Value     json.RawMessage `json:"value"`
}

func (t *tools) addComponent(_ context.Context, a addComponentArgs) (any, error) {
	g, err := t.god()
	if err != nil {
		return nil, err
	}
	c := g.compByKey[a.Component]
	if c == nil {
		var keys []string
		for _, c := range g.components {
			keys = append(keys, c.Key())
		}
		return nil, unknownKey("component", a.Component, keys)
	}
	value := a.Value
	if len(bytes.TrimSpace(value)) == 0 {
		value = c.Schema.InitialValue()
	}
	return t.editDocument(a.Path, a.Expect, func(text []byte, doc *document, info docInfo) ([]byte, error) {
		i, obj, err := locate(doc, a.objArgs)
		if err != nil {
			return nil, err
		}
		if _, has, err := t.current(info, obj, a.Component); err != nil {
			return nil, err
		} else if has {
			return nil, fmt.Errorf("object %s already has %s (set_component or set_field changes it)", obj.label(i), a.Component)
		}
		return jsonedit.Set(text, value, componentPath(i, a.Component, nil)...)
	})
}

type removeComponentArgs struct {
	objArgs
	Component string `json:"component"`
}

func (t *tools) removeComponent(_ context.Context, a removeComponentArgs) (any, error) {
	g, err := t.god()
	if err != nil {
		return nil, err
	}
	return t.editDocument(a.Path, a.Expect, func(text []byte, doc *document, info docInfo) ([]byte, error) {
		i, obj, err := locate(doc, a.objArgs)
		if err != nil {
			return nil, err
		}
		typ, err := t.objectType(info, obj)
		if err != nil {
			return nil, err
		}
		if s := g.objByKey[typ]; s != nil {
			for _, c := range s.Components {
				if c.Key() == a.Component && !c.Optional {
					return nil, fmt.Errorf("%s is a required component of %s and cannot be removed", a.Component, typ)
				}
			}
		}
		if _, has, err := t.current(info, obj, a.Component); err != nil {
			return nil, err
		} else if !has {
			return nil, fmt.Errorf("object %s has no component %s", obj.label(i), a.Component)
		}
		path := componentPath(i, a.Component, nil)
		if obj.Prefab == "" {
			return jsonedit.Delete(text, path...)
		}
		// An instance removes what its prefab has by overriding it with null;
		// a component only the instance adds is just deleted.
		base, err := t.expandPrefab(obj.Prefab, info.chain())
		if err != nil {
			return nil, err
		}
		if _, inPrefab := base.components[a.Component]; inPrefab {
			return jsonedit.Set(text, []byte("null"), path...)
		}
		return jsonedit.Delete(text, path...)
	})
}

type revertOverrideArgs struct {
	objArgs
	Component string `json:"component"`
	Field     []any  `json:"field"`
}

func (t *tools) revertOverride(_ context.Context, a revertOverrideArgs) (any, error) {
	field, err := fieldPath(a.Field)
	if err != nil {
		return nil, err
	}
	return t.editDocument(a.Path, a.Expect, func(text []byte, doc *document, info docInfo) ([]byte, error) {
		i, obj, err := locate(doc, a.objArgs)
		if err != nil {
			return nil, err
		}
		if obj.Prefab == "" {
			return nil, fmt.Errorf("object %s is not a prefab instance: it has no overrides", obj.label(i))
		}
		// An array is overridden as a whole, so a path inside one reverts the array.
		if k := slices.IndexFunc(field, func(s any) bool { _, isInt := s.(int); return isInt }); k >= 0 {
			field = field[:k]
		}
		raw, ok := obj.Components[a.Component]
		if !ok {
			return nil, fmt.Errorf("object %s does not override %s (overrides: %v)", obj.label(i), a.Component, overriddenPaths(obj.Components))
		}
		own, err := decodeJSON(raw)
		if err != nil {
			return nil, err
		}
		if _, ok := getIn(own, field); !ok {
			return nil, fmt.Errorf("object %s does not override %s%s (overrides: %v)", obj.label(i), a.Component, dotted(field), overriddenPaths(obj.Components))
		}
		// Remove the override and every object above it that only held it,
		// up to the component itself (as the extension does).
		cut := field
		for len(cut) > 0 {
			parent, _ := getIn(own, cut[:len(cut)-1])
			m, isMap := parent.(map[string]any)
			if !isMap || len(m) != 1 {
				break
			}
			cut = cut[:len(cut)-1]
		}
		if len(cut) == 0 {
			return jsonedit.Delete(text, componentPath(i, a.Component, nil)...)
		}
		return jsonedit.Delete(text, componentPath(i, a.Component, cut)...)
	})
}

// dotted renders a field path with a leading dot, "" for none.
func dotted(path []any) string {
	if len(path) == 0 {
		return ""
	}
	return "." + describePath(path)
}

func (t *tools) editTools() []*mcp.Tool {
	return []*mcp.Tool{
		tool("set_component", "Set a component",
			"Replaces the whole value of a component the object has, in the document's file, changing only that value's bytes. "+
				"On a prefab instance the value is an override, merged over the prefab's component (null removes it; use remove_component). "+
				"The value must pass the component's schema and asset keys and entity ids must exist; otherwise nothing is written.",
			mcp.Object(objProps(map[string]*mcp.Schema{"component": componentArg, "value": valueArg}), "path", "component", "value"),
			editable, t.setComponent),
		tool("set_field", "Set a field of a component",
			"Sets one field of a component (field is a path such as [\"Pos\",\"X\"]), changing only that value in the file. On a prefab "+
				"instance it writes an override at that path; a field inside an array writes the whole array, since overrides replace arrays. "+
				"Nothing is written if the result is not valid.",
			mcp.Object(objProps(map[string]*mcp.Schema{"component": componentArg, "field": fieldArg, "value": valueArg}), "path", "component", "field", "value"),
			editable, t.setField),
		tool("add_component", "Add a component",
			"Adds a component the object does not have, with the given value or, without one, the component's initial value "+
				"(its declared defaults, else zero values). Refuses a component the object already has. Writes the document's file.",
			mcp.Object(objProps(map[string]*mcp.Schema{"component": componentArg, "value": valueArg}), "path", "component"),
			editable, t.addComponent),
		tool("remove_component", "Remove a component",
			"Removes a component from an object in the document's file. Required components of the object type are refused. On a prefab "+
				"instance a component the prefab has is removed by writing null; one only the instance added is deleted.",
			mcp.Object(objProps(map[string]*mcp.Schema{"component": componentArg}), "path", "component"),
			destructive, t.removeComponent),
		tool("set_override", "Override a prefab instance",
			"Writes an override on a prefab instance: field is the path inside the component (omit it to write a whole-component override). "+
				"Only instances are accepted. Same checks and file change as set_field.",
			mcp.Object(objProps(map[string]*mcp.Schema{"component": componentArg, "field": fieldArg, "value": valueArg}), "path", "component", "value"),
			editable, t.setOverride),
		tool("revert_override", "Revert an override",
			"Removes the override of a field (or of the whole component, without field) from a prefab instance so the prefab's value shows "+
				"again; objects above that held only this override go too. Writes the document's file.",
			mcp.Object(objProps(map[string]*mcp.Schema{"component": componentArg, "field": fieldArg}), "path", "component"),
			destructive, t.revertOverride),
	}
}
