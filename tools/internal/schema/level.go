package schema

import (
	"slices"

	"github.com/weakpixel/engine/god"
)

// documentSchema builds the JSON Schema of a serialized GOD document (a
// level, or with prefab set a prefab file) from the project's components and
// objects (doc/guide/ecs/game-object-definition.md#3-serialized-god).
//
// Every component key is a property of "components"; unknown keys are errors,
// as in god.Deserialize. The value shapes sit in if/else on "prefab", so a
// property's own shape is either the full component or its override. An object of a known class requires that class's
// non-optional components. An object with "prefab" takes the same components
// as overrides, where nothing is required.
func documentSchema(prefab bool, comps []god.ComponentSchema, objects []god.Schema) map[string]any {
	defs := map[string]any{}
	names, full, patch := map[string]any{}, map[string]any{}, map[string]any{}
	for _, c := range comps {
		key := c.Key()
		defs["component."+key] = withDoc(toJSONSchema(c.Schema, false), c.Doc)
		defs["override."+key] = withDoc(toJSONSchema(c.Schema, true), c.Doc)
		names[key] = map[string]any{}
		full[key] = ref("component." + key)
		patch[key] = ref("override." + key)
	}

	var groups, classes []string
	var classRules []any
	for _, o := range objects {
		if !slices.Contains(groups, o.Group) {
			groups = append(groups, o.Group)
		}
		if !slices.Contains(classes, o.Class) {
			classes = append(classes, o.Class)
		}
		var req []string
		for _, c := range o.Components {
			if !c.Optional {
				req = append(req, c.Key())
			}
		}
		if len(req) == 0 {
			continue
		}
		slices.Sort(req)
		classRules = append(classRules, map[string]any{
			"if": map[string]any{
				"required":   []string{"group", "class"},
				"properties": map[string]any{"group": map[string]any{"const": o.Group}, "class": map[string]any{"const": o.Class}},
			},
			"then": map[string]any{"required": []string{"components"}, "properties": map[string]any{"components": map[string]any{"required": req}}},
		})
	}
	slices.Sort(groups)
	slices.Sort(classes)

	identity := func(values []string, desc string) map[string]any {
		s := map[string]any{"type": "string", "description": desc}
		if len(values) > 0 {
			s["enum"] = values
		}
		return s
	}
	rules := []any{
		map[string]any{
			"if":   map[string]any{"required": []string{"prefab"}},
			"then": map[string]any{"properties": map[string]any{"components": map[string]any{"properties": patch}}},
			"else": map[string]any{
				"required":   []string{"group", "class", "components"},
				"properties": map[string]any{"components": map[string]any{"properties": full}},
			},
		},
	}
	// The class rules only apply to an object that is not a prefab instance.
	for _, r := range classRules {
		rules = append(rules, map[string]any{"if": map[string]any{"not": map[string]any{"required": []string{"prefab"}}}, "then": r})
	}
	entry := map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"id":     map[string]any{"type": "string", "description": "Unique in the document; the target of entity references."},
			"prefab": map[string]any{"type": "string", "description": "Key of a prefab; the object is an instance and components holds overrides.", "x-asset": "prefab"},
			"group":  identity(groups, "Object group."),
			"class":  identity(classes, "Object class."),
			"components": map[string]any{
				"type": "object", "additionalProperties": false, "properties": names,
				"description": "Component values by component key.",
			},
		},
		"allOf": rules,
	}
	objectsS := map[string]any{"type": "array", "items": entry}
	title, desc := "weakpixel level (*.level.json)", "A serialized GOD document: the objects of a level."
	if prefab {
		objectsS["minItems"], objectsS["maxItems"] = 1, 1
		title, desc = "weakpixel prefab (*.prefab.json)", "A serialized GOD document with exactly one object."
	}
	return map[string]any{
		"$schema": jsonSchemaURL, "title": title, "description": desc,
		"type": "object", "additionalProperties": false, "required": []string{"objects"},
		"properties": map[string]any{
			"name":    map[string]any{"type": "string", "description": "Label for editors; never used for lookups."},
			"objects": objectsS,
		},
		"definitions": defs,
	}
}

func ref(name string) map[string]any { return map[string]any{"$ref": "#/definitions/" + name} }

// withDoc adds the Go doc comment of a component as its description unless
// the schema has one.
func withDoc(s map[string]any, doc string) map[string]any {
	if _, ok := s["description"]; !ok && doc != "" {
		s["description"] = doc
	}
	return s
}
