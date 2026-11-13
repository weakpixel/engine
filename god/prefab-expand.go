package god

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
)

// PrefabEntry is the object entry of a document that names a prefab: the
// parts of an `{"id", "prefab", "components", "children"}` entry that
// [Prefab.ExpandInstance] reads.
type PrefabEntry struct {
	// ID is the entry's "id": the id of the instance's root. The other
	// objects of the instance get `<ID>/<part>`. A prefab with several
	// objects needs one.
	ID string
	// Group and Class may be left out, or must be the prefab's object type.
	Group, Class string
	// Components are overrides of the root's components, merged over the
	// prefab's (see [Prefab.Expand]).
	Components map[string]json.RawMessage
	// Children maps the id of a part of the prefab (not the root) to
	// either an object of component overrides, as in Components, or null,
	// which removes the part from this instance.
	Children map[string]json.RawMessage
}

// ExpandedObject is one plain object of an expanded prefab instance.
type ExpandedObject struct {
	// ID is the object's id in the document: the entry's ID for the root,
	// `<entry ID>/<Part>` for a part.
	ID string
	// Group and Class are the object's type.
	Group, Class string
	// Components are the complete components: the prefab's merged with the
	// overrides. They are not validated against the schema (see
	// [CheckComponents]); values the prefab holds are shared and must not
	// be modified.
	Components map[string]json.RawMessage
	// Part is the id of the object within the prefab ("sword",
	// "horse/saddle"), or "" for the root.
	Part string
}

// entry is in as a PrefabEntry.
func (in instance) entry() PrefabEntry {
	return PrefabEntry{ID: in.ID, Group: in.Group, Class: in.Class, Components: in.Components, Children: in.Children}
}

// ExpandInstance turns an entry that names p as its prefab into plain
// objects, the root first and the other objects in prefab order. The
// prefab's entity references are rewritten into the document's id space
// first (the root's id becomes e.ID, a part's `<id>` becomes
// `<e.ID>/<id>`), then e.Components and e.Children are merged over the
// objects, so override values are written in the document's ids and may
// point at any object of the document.
//
// Errors name the component and the JSON path. They include a Children
// key that is no part of p, a removed part that another object still
// references, and an entry without an ID for a prefab with several
// objects.
func (p *Prefab) ExpandInstance(e PrefabEntry) ([]ExpandedObject, error) {
	if len(p.parts) > 1 && e.ID == "" {
		return nil, fmt.Errorf("instance of multi-object prefab %q needs an \"id\"", p.key)
	}
	return p.expandEntry(e, e.ID, e.ID+"/")
}

// expandEntry is ExpandInstance with the ids spelled out: the root gets
// rootID and the part with prefab id x gets prefix+x. A variant uses the
// empty prefix to keep its base's part ids.
func (p *Prefab) expandEntry(e PrefabEntry, rootID, prefix string) ([]ExpandedObject, error) {
	root := p.parts[0]
	if meta := p.Meta(); (e.Group != "" || e.Class != "") && (e.Group != meta.Group || e.Class != meta.Class) {
		return nil, fmt.Errorf("object %s does not match prefab %q, which is %s", SchemaMeta{Group: e.Group, Class: e.Class}.Key(), p.key, meta.Key())
	}
	patches, removed, err := p.childPatches(e.Children)
	if err != nil {
		return nil, err
	}

	// ids maps the prefab's ids to the target ids; nothing to rewrite when
	// they are the same.
	idFor := func(i int) string {
		if i == 0 {
			return rootID
		}
		return prefix + p.parts[i].id
	}
	var ids map[string]string
	hasRefs := slices.ContainsFunc(p.parts, func(part prefabPart) bool { return len(part.refComps) > 0 })
	if hasRefs && (rootID != root.id || prefix != "") {
		ids = make(map[string]string, len(p.parts))
		for i, part := range p.parts {
			if part.id != "" {
				ids[part.id] = idFor(i)
			}
		}
	}

	var errs []error
	out := make([]ExpandedObject, 0, len(p.parts))
	for i, part := range p.parts {
		if removed[i] {
			continue
		}
		base := part.components
		if ids != nil && len(part.refComps) > 0 {
			base = maps.Clone(base)
			for _, c := range part.refComps {
				raw, err := mapRefs(c.Schema, base[c.Key()], func(_ string, v any) (any, error) {
					id, ok := v.(string)
					if !ok {
						return v, nil
					}
					to, ok := ids[id]
					switch {
					case !ok:
						return nil, fmt.Errorf("unknown object id %q", id)
					case to == "":
						return nil, fmt.Errorf("a reference to %q needs the instance to have an \"id\"", id)
					}
					return to, nil
				})
				if err != nil {
					errs = append(errs, fmt.Errorf("component %s: %w", c.Key(), err))
					continue
				}
				base[c.Key()] = raw
			}
		}
		patch := patches[i]
		if i == 0 {
			patch = e.Components
		}
		comps, err := mergeComponents(base, patch)
		if err != nil {
			errs = append(errs, err)
		}
		o := ExpandedObject{ID: idFor(i), Group: part.schema.Group, Class: part.schema.Class, Components: comps}
		if i > 0 {
			o.Part = part.id
		}
		out = append(out, o)
	}
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}
	if slices.Contains(removed, true) {
		if err := p.checkRemoved(out, removed, idFor); err != nil {
			return nil, err
		}
	}
	return out, nil
}

// childPatches parses the children of an entry: the component overrides of
// each part by position, and which parts are removed.
func (p *Prefab) childPatches(children map[string]json.RawMessage) (patches map[int]map[string]json.RawMessage, removed []bool, err error) {
	removed = make([]bool, len(p.parts))
	var errs []error
	for _, k := range sortedKeys(children) {
		i, ok := p.index[k]
		switch {
		case !ok || k == "":
			errs = append(errs, fmt.Errorf("children: prefab %q has no object %q", p.key, k))
			continue
		case i == 0:
			errs = append(errs, fmt.Errorf("children: %q is the root of prefab %q; override it with \"components\"", k, p.key))
			continue
		}
		raw := bytes.TrimSpace(children[k])
		if string(raw) == "null" {
			removed[i] = true
			continue
		}
		var comps map[string]json.RawMessage
		if !isJSONObject(raw) || json.Unmarshal(raw, &comps) != nil {
			errs = append(errs, fmt.Errorf("children.%s: expected an object of components or null", k))
			continue
		}
		if patches == nil {
			patches = map[int]map[string]json.RawMessage{}
		}
		patches[i] = comps
	}
	return patches, removed, errors.Join(errs...)
}

// checkRemoved reports every remaining object that still references a
// removed part. objs are the remaining objects in part order.
func (p *Prefab) checkRemoved(objs []ExpandedObject, removed []bool, idFor func(int) string) error {
	gone := map[string]string{} // target id -> prefab id
	for i, r := range removed {
		if r {
			gone[idFor(i)] = p.parts[i].id
		}
	}
	if len(gone) == 0 {
		return nil
	}
	var errs []error
	for _, o := range objs {
		s, ok := LookupSchema(o.Group, o.Class)
		if !ok {
			continue
		}
		for _, k := range sortedKeys(o.Components) {
			c, ok := ComponentOf(s, k)
			if !ok || !hasEntityRef(c.Schema) {
				continue
			}
			name := o.Part
			if name == "" {
				name = p.parts[0].id
			}
			_, _ = mapRefs(c.Schema, o.Components[k], func(_ string, v any) (any, error) {
				if id, ok := v.(string); ok && gone[id] != "" {
					errs = append(errs, fmt.Errorf("children: removing %q leaves %q with a reference to it", gone[id], name))
				}
				return v, nil
			})
		}
	}
	return errors.Join(errs...)
}

// Expand returns the components of an instance of p's root: overrides,
// the "components" of a document entry that names p as its "prefab",
// merged over the root's components (objects merge key by key, other
// values replace, an override of null removes an optional component).
// group and class are the entry's: empty, or p's object type. The result
// is not validated against the schema (see [CheckComponents]); the
// prefab's own values are shared and must not be modified. Errors name the
// component and the JSON path.
//
// Expand knows only the root and does not rewrite entity references; for a
// prefab with several objects use [Prefab.ExpandInstance].
func (p *Prefab) Expand(group, class string, overrides map[string]json.RawMessage) (map[string]json.RawMessage, error) {
	if meta := p.Meta(); (group != "" || class != "") && (group != meta.Group || class != meta.Class) {
		return nil, fmt.Errorf("object %s does not match prefab %q, which is %s", SchemaMeta{Group: group, Class: class}.Key(), p.key, meta.Key())
	}
	return mergeComponents(p.parts[0].components, overrides)
}

// Overrides is the inverse of [Prefab.Expand]: the "components" of an
// instance of p whose complete root components are comps, as compact JSON.
// Components and fields equal to the root's are left out, a component of the
// root that comps lacks is null. Only fields can be overridden: an error is
// returned for a value that would need to delete a map entry of p.
func (p *Prefab) Overrides(comps map[string]json.RawMessage) (map[string]json.RawMessage, error) {
	root := p.parts[0]
	return diffComponents(root.schema, root.components, comps)
}

// PartOverrides is [Prefab.Overrides] for one object of an instance whose
// root has the document id rootID: the "components" (part is "", the root)
// or the "children" entry (part is the part's id) that turn the prefab's
// object, expanded for that instance, into comps. Only the keys of comps
// are looked at, and references in comps are in the document's ids. An
// unknown part is an error.
func (p *Prefab) PartOverrides(rootID, part string, comps map[string]json.RawMessage) (map[string]json.RawMessage, error) {
	i := 0
	if part != "" {
		var ok bool
		if i, ok = p.index[part]; !ok || i == 0 {
			return nil, fmt.Errorf("prefab %q has no object %q", p.key, part)
		}
	}
	base, err := p.ExpandInstance(PrefabEntry{ID: rootID})
	if err != nil {
		return nil, err
	}
	for _, b := range base {
		if b.Part == part {
			return diffComponents(p.parts[i].schema, b.Components, comps)
		}
	}
	return nil, fmt.Errorf("prefab %q has no object %q", p.key, part)
}

// mergeComponents merges the overrides patch over the components base (see
// mergePatch): an override of null removes a component. base is not
// modified; the result is a new map.
func mergeComponents(base, patch map[string]json.RawMessage) (map[string]json.RawMessage, error) {
	merged := make(map[string]json.RawMessage, len(base)+len(patch))
	maps.Copy(merged, base)
	var errs []error
	for _, name := range sortedKeys(patch) {
		p := patch[name]
		if string(bytes.TrimSpace(p)) == "null" {
			delete(merged, name)
			continue
		}
		b, ok := merged[name]
		if !ok {
			merged[name] = p
			continue
		}
		m, err := mergePatch(b, p)
		if err != nil {
			errs = append(errs, fmt.Errorf("component %s: %w", name, err))
			continue
		}
		merged[name] = m
	}
	return merged, errors.Join(errs...)
}

// diffComponents is the inverse of mergeComponents for an object of class
// s: the overrides that turn base into comps. Components equal to base's
// are left out, a component base has and comps lacks is null.
func diffComponents(s Schema, base, comps map[string]json.RawMessage) (map[string]json.RawMessage, error) {
	out := make(map[string]json.RawMessage)
	var errs []error
	keys := maps.Clone(base)
	maps.Copy(keys, comps)
	for _, c := range ObjectComponents(s, maps.Keys(keys)) {
		raw, has := comps[c.Key()]
		b, inBase := base[c.Key()]
		switch {
		case !has && inBase:
			out[c.Key()] = json.RawMessage("null")
		case has && !inBase:
			out[c.Key()] = raw
		case has:
			d, changed, err := diffPatch(c.Schema, b, raw)
			if err != nil {
				errs = append(errs, fmt.Errorf("component %s: %w", c.Key(), err))
			} else if changed {
				out[c.Key()] = d
			}
		}
	}
	return out, errors.Join(errs...)
}
