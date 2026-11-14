package god

import (
	"bytes"
	"cmp"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/weakpixel/ecs"
)

// maxPrefabDepth bounds a chain of variants and nested prefabs (a prefab
// based on a prefab, or holding an instance of one ...).
const maxPrefabDepth = 16

// Prefab is a parsed, validated prefab: pre-defined Game Objects whose
// component values are filled in, stored as a GOD document (`*.prefab.json`)
// of one or more objects. The first object is the root. Documents place it
// with an object entry `{"id": ..., "prefab": "<key>", "components":
// {<overrides>}, "children": {<part>: <overrides>}}` and game code spawns
// copies with [Prefab.Bind] and [BoundPrefab.Spawn].
//
// A prefab with several objects is spawned as a group: the other objects
// are its parts, and entity references between the objects are remapped
// to each copy. This package does not know what ties the parts together;
// the engine requires every part to be attached to the root through
// `hierarchy:Parent`.
//
// A variant is a prefab whose first object is itself an instance of another
// prefab (its "base"); its objects are the base's, merged with its own,
// followed by its other objects. An object after the first may be an
// instance of another prefab too (a nested prefab); its objects join the
// list as parts with qualified ids (`horse`, `horse/saddle`).
//
// A Prefab is immutable and safe for concurrent use.
type Prefab struct {
	key  string
	name string
	// parts are the objects, the root first, with nested prefabs and
	// bases flattened in.
	parts []prefabPart
	// index maps the id of each part that has one to its position.
	index map[string]int
	// base is the key of the prefab the root is an instance of, or "".
	base string
	// direct are the keys of the prefabs this one is built from (its base
	// and the prefabs it nests); deps are those and, transitively, theirs,
	// for cycle checks. depth is the longest chain of such prefabs.
	direct []string
	deps   []string
	depth  int
}

// prefabPart is one object of a prefab.
type prefabPart struct {
	// id is the object's id in the prefab ("" for the root of a prefab
	// with one object that has none). Entity references in the prefab's
	// components use these ids.
	id     string
	schema Schema
	// comps are the components the object has (class and extra ones, see
	// ObjectComponents), plus the class's optional ones it lacks.
	comps []Component
	// components holds the expanded, validated JSON of each component the
	// object has, compact.
	components map[string]json.RawMessage
	// refComps are the components the object has whose schema contains
	// entity references.
	refComps []Component
}

// PrefabSource finds prefabs by key for [Deserialize], [Serialize] and
// [ParsePrefab] (the base of a variant). The engine implements it on top of
// its asset server; [PrefabMap] is a fixed set.
type PrefabSource interface {
	// Prefab returns the prefab stored under key, or an error if there is
	// none or it is not available (not loaded yet, failed).
	Prefab(key string) (*Prefab, error)
}

// PrefabMap is a PrefabSource over a fixed set of prefabs, keyed by
// [Prefab.Key].
type PrefabMap map[string]*Prefab

// Prefab returns m[key], or an error if there is none.
func (m PrefabMap) Prefab(key string) (*Prefab, error) {
	if p, ok := m[key]; ok {
		return p, nil
	}
	return nil, errors.New("unknown prefab")
}

// Add parses data as the prefab key with m as its PrefabSource and adds it,
// for tests and games that keep prefabs in memory. Add bases first.
func (m PrefabMap) Add(key string, data []byte) error {
	p, err := ParsePrefab(key, data, m)
	if err != nil {
		return err
	}
	m[key] = p
	return nil
}

// SetPrefabSource makes src the world's PrefabSource, used by Deserialize
// for object entries with a "prefab" and by Serialize for objects with
// [Object.Prefab]. It is a world resource of type PrefabSource (an
// interface).
func SetPrefabSource(world *ecs.World, src PrefabSource) {
	ecs.SetResource(world, &src)
}

// worldPrefabSource returns the world's PrefabSource, or nil.
func worldPrefabSource(world *ecs.World) PrefabSource {
	if p := ecs.GetResourceOrNil[PrefabSource](world); p != nil {
		return *p
	}
	return nil
}

// ParsePrefab parses data, the content of the prefab file key: a GOD
// document with one or more objects. A prefab with several objects needs an
// "id" on each of them, and ids must not contain "/". The first object may
// name a base prefab ("prefab"), and any other may be an instance of a prefab
// to nest, which src must return; a missing src is then an error. Every
// component is validated against its object's schema after merging, and
// every entity reference must name an object of the prefab, so a Prefab
// always holds complete, valid objects. Errors start with `prefab "<key>": `
// and are all reported.
func ParsePrefab(key string, data []byte, src PrefabSource) (*Prefab, error) {
	p, err := parsePrefab(key, data, src)
	if err != nil {
		return nil, fmt.Errorf("prefab %q: %w", key, err)
	}
	return p, nil
}

func parsePrefab(key string, data []byte, src PrefabSource) (*Prefab, error) {
	insts, name, err := readDocument(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	if len(insts) == 0 {
		return nil, errors.New("no object")
	}
	multi := len(insts) > 1
	label := func(i int) string {
		if multi {
			return fmt.Sprintf("objects[%d]: ", i)
		}
		return ""
	}
	p := &Prefab{key: key, name: name, depth: 1}

	// Flatten: plain objects as they are, instances expanded into their
	// objects (the first object's base keeps its part ids, a nested
	// instance's get the instance's id as prefix).
	var flat []ExpandedObject
	var origin []int       // index of the document entry of each flat object
	var inherited []string // key of the base a flat object comes from, or ""
	var errs []error
	for i, in := range insts {
		if err := checkAuthoredID(in.ID); err != nil {
			errs = append(errs, fmt.Errorf("%s%w", label(i), err))
		}
		if in.Prefab == "" {
			// (The ids of instances are checked after expansion: a
			// variant's root may take its base's.)
			if multi && in.ID == "" {
				errs = append(errs, fmt.Errorf(`%sa prefab with several objects needs an "id" on each`, label(i)))
			}
			if in.Children != nil {
				errs = append(errs, fmt.Errorf(`%s"children" needs a "prefab"`, label(i)))
			}
			flat = append(flat, ExpandedObject{ID: in.ID, Group: in.Group, Class: in.Class, Components: in.Components})
			origin, inherited = append(origin, i), append(inherited, "")
			continue
		}
		if i > 0 && in.ID == "" {
			errs = append(errs, fmt.Errorf(`%sa prefab with several objects needs an "id" on each`, label(i)))
			continue
		}
		dep, err := p.dependOn(src, in.Prefab, i == 0)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s%w", label(i), err))
			continue
		}
		rootID, prefix := in.ID, in.ID+"/"
		if i == 0 {
			// A variant keeps the base's part ids and, unless it names
			// itself, the root's.
			rootID, prefix = cmp.Or(in.ID, dep.parts[0].id), ""
		}
		objs, err := dep.expandEntry(in.entry(), rootID, prefix)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s%w", label(i), err))
			continue
		}
		for j, o := range objs {
			from := ""
			if i == 0 && j > 0 {
				from = dep.key
			}
			flat = append(flat, o)
			origin, inherited = append(origin, i), append(inherited, from)
		}
	}
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}

	// Ids: unique, and present when there are several objects.
	ids := make(map[string]int, len(flat))
	for j, o := range flat {
		if o.ID == "" {
			if len(flat) > 1 {
				errs = append(errs, fmt.Errorf(`%sa prefab with several objects needs an "id" on each`, label(origin[j])))
			}
			continue
		}
		if first, dup := ids[o.ID]; dup {
			if inherited[first] != "" && inherited[j] == "" {
				errs = append(errs, fmt.Errorf("%sduplicate id %q (also in base %q)", label(origin[j]), o.ID, inherited[first]))
			} else {
				errs = append(errs, fmt.Errorf("%sduplicate id %q (first used by objects[%d])", label(origin[j]), o.ID, origin[first]))
			}
			continue
		}
		ids[o.ID] = j
	}

	// Components: validated against the schema, references against the ids.
	parts := make([]prefabPart, len(flat))
	for j, o := range flat {
		part, err := newPrefabPart(o, ids)
		if err != nil {
			if len(flat) > 1 {
				err = fmt.Errorf("object %q: %w", o.ID, err)
			}
			errs = append(errs, err)
			continue
		}
		parts[j] = part
	}
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}
	p.parts, p.index = parts, ids
	return p, nil
}

// newPrefabPart validates o and builds its part. ids are the ids of all
// objects of the prefab.
func newPrefabPart(o ExpandedObject, ids map[string]int) (prefabPart, error) {
	s, err := checkInstance(instance{ID: o.ID, Group: o.Group, Class: o.Class, Components: o.Components})
	if err != nil {
		return prefabPart{}, err
	}
	comps := make(map[string]json.RawMessage, len(o.Components))
	for name, raw := range o.Components {
		if comps[name], err = compactJSON(raw); err != nil {
			return prefabPart{}, err
		}
	}
	part := prefabPart{id: o.ID, schema: s, comps: ObjectComponents(s, maps.Keys(comps)), components: comps}
	var errs []error
	for _, c := range part.comps {
		raw, ok := comps[c.Key()]
		if !ok || !hasEntityRef(c.Schema) {
			continue
		}
		part.refComps = append(part.refComps, c)
		_, err := mapRefs(c.Schema, raw, func(_ string, v any) (any, error) {
			if id, ok := v.(string); ok {
				if _, known := ids[id]; !known {
					return nil, fmt.Errorf("unknown object id %q", id)
				}
			}
			return v, nil
		})
		if err != nil {
			errs = append(errs, fmt.Errorf("component %s: %w", c.Key(), err))
		}
	}
	return part, errors.Join(errs...)
}

// checkAuthoredID rejects an id that uses the "/" reserved for the parts of
// prefab instances.
func checkAuthoredID(id string) error {
	if strings.Contains(id, "/") {
		return fmt.Errorf(`id %q contains "/", which is reserved for the parts of prefab instances`, id)
	}
	return nil
}

// dependOn looks up the prefab dep that p is built from (its base when
// base is true, else a nested prefab), checks for cycles and depth and
// records the dependency.
func (p *Prefab) dependOn(src PrefabSource, key string, base bool) (*Prefab, error) {
	dep, err := lookupPrefab(src, key)
	if err != nil {
		return nil, err
	}
	if dep.key == p.key || slices.Contains(dep.deps, p.key) {
		path := []string{p.key}
		path = append(path, cyclePath(src, dep, p.key, nil)...)
		return nil, fmt.Errorf("prefab cycle: %s", strings.Join(path, " -> "))
	}
	if dep.depth >= maxPrefabDepth {
		return nil, fmt.Errorf("variant chain deeper than %d", maxPrefabDepth)
	}
	if base {
		p.base = dep.key
	}
	p.depth = max(p.depth, dep.depth+1)
	for _, k := range append([]string{dep.key}, dep.deps...) {
		if !slices.Contains(p.deps, k) {
			p.deps = append(p.deps, k)
		}
	}
	if !slices.Contains(p.direct, dep.key) {
		p.direct = append(p.direct, dep.key)
	}
	return dep, nil
}

// cyclePath returns the keys on a path from from to target through direct
// dependencies, both ends included, for the error message of a cycle.
func cyclePath(src PrefabSource, from *Prefab, target string, seen []string) []string {
	path := []string{from.key}
	if slices.Contains(from.direct, target) {
		return append(path, target)
	}
	seen = append(seen, from.key)
	for _, k := range from.direct {
		if slices.Contains(seen, k) {
			continue
		}
		next, err := src.Prefab(k)
		if err != nil {
			continue
		}
		if rest := cyclePath(src, next, target, seen); rest != nil {
			return append(path, rest...)
		}
	}
	return nil
}

// lookupPrefab asks src for key, with the error naming the prefab.
func lookupPrefab(src PrefabSource, key string) (*Prefab, error) {
	if src == nil {
		return nil, fmt.Errorf("prefab %q: no PrefabSource set (see SetPrefabSource)", key)
	}
	p, err := src.Prefab(key)
	if err != nil {
		return nil, fmt.Errorf("prefab %q: %w", key, err)
	}
	return p, nil
}

// Key returns the key the prefab was parsed as (its file path).
func (p *Prefab) Key() string { return p.key }

// Name returns the document's optional top-level "name", a human-readable
// label for editors, or "". It labels the file: a variant does not inherit
// its base's name, and nothing looks prefabs up by it.
func (p *Prefab) Name() string { return p.name }

// ID returns the "id" of the prefab's root object, or "". Entity references
// inside the prefab point at the root by it. A variant that names no id of
// its own has its base's.
func (p *Prefab) ID() string { return p.parts[0].id }

// Meta returns the object type of the prefab's root.
func (p *Prefab) Meta() SchemaMeta { return p.parts[0].schema.SchemaMeta }

// Base returns the key of the prefab this one is a variant of, or "".
func (p *Prefab) Base() string { return p.base }

// Parts returns the ids of the prefab's objects in spawn order, the root
// first: the order of the entities [BoundPrefab.SpawnAll] returns. A
// part of a nested prefab is `<instance id>/<part id>`. The ids of a
// prefab with one object may be [""].
func (p *Prefab) Parts() []string {
	ids := make([]string, len(p.parts))
	for i, part := range p.parts {
		ids[i] = part.id
	}
	return ids
}

// PartIndex returns the position of the object with id in [Prefab.Parts],
// or -1 if there is none.
func (p *Prefab) PartIndex(id string) int {
	if i, ok := p.index[id]; ok {
		return i
	}
	if id == "" && p.parts[0].id == "" {
		return 0
	}
	return -1
}

// PartComponent is [Prefab.Component] for the object with id (see
// [Prefab.Parts]). It returns false for an unknown id or a component the
// object lacks.
func (p *Prefab) PartComponent(id, key string) (json.RawMessage, bool) {
	i := p.PartIndex(id)
	if i < 0 {
		return nil, false
	}
	raw, ok := p.parts[i].components[key]
	return raw, ok
}

// Component returns the expanded JSON of the root's component with key
// "group:name" and whether the root has it. The result must not be
// modified.
func (p *Prefab) Component(key string) (json.RawMessage, bool) {
	raw, ok := p.parts[0].components[key]
	return raw, ok
}

// DocumentName returns the optional top-level "name" of a GOD document (a
// level or a prefab file), a human-readable label for editors, or "" when
// there is none. Like [PrefabKeys] it reads only the JSON shape: invalid JSON
// and a name that is not a string also give "" ([Deserialize] reports real
// errors). [Deserialize] and [ParsePrefab] accept the key; names are not
// unique and nothing looks documents up by them.
func DocumentName(data []byte) string {
	var doc struct {
		Name json.RawMessage `json:"name"`
	}
	if json.Unmarshal(data, &doc) != nil {
		return ""
	}
	var name string
	if json.Unmarshal(doc.Name, &name) != nil {
		return ""
	}
	return name
}

// PrefabKeys returns the distinct "prefab" keys of the objects of a GOD
// document (a level or a prefab file), in document order. Loaders use it to
// load a document's prefabs before deserializing it. Only the JSON shape is
// read; nothing is validated.
func PrefabKeys(data []byte) ([]string, error) {
	var doc struct {
		Objects []struct {
			Prefab string `json:"prefab"`
		} `json:"objects"`
	}
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, err
	}
	var keys []string
	for _, o := range doc.Objects {
		if o.Prefab != "" && !slices.Contains(keys, o.Prefab) {
			keys = append(keys, o.Prefab)
		}
	}
	return keys, nil
}
