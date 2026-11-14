package god

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"iter"
	"maps"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/weakpixel/ecs"
)

// Component is one component of an object schema: a reference to a
// component type (identified by Group and Name, see [ComponentSchema]) plus
// how the object uses it.
//
// In JSON (the GOD and the editor protocol) the identity is one key,
// "component": "group:name"; Schema is left out when nil, as in the object
// entries of a GOD file, where each component schema appears once at the
// top level.
type Component struct {
	// Group and Name identify the component type. Key() ("group:name") is
	// the key of the component in JSON instances.
	Group string `json:"-"`
	Name  string `json:"-"`

	// Field is the Go field name of the component in the object struct. It
	// is documentation (and Go sugar): instances are keyed by Key().
	Field string `json:"field,omitempty"`

	// Optional components may be absent from an instance. They are
	// suggestions for editors, not a limit: an object may also hold any
	// other registered component type (see [ObjectComponents]).
	Optional bool `json:"optional,omitempty"`

	// Readonly marks a value component (copied out on Get). It is always required.
	Readonly bool `json:"readonly,omitempty"`

	// RuntimeID is the runtime ID of the component type. It is only valid for the
	// current running instance of the program and is not persisted.
	RuntimeID ecs.ComponentID `json:"-"`

	// Schema describes the JSON form of the component.
	Schema *TypeSchema `json:"schema,omitempty"`

	// Doc is the doc comment of the object field that declares the component.
	// Documentation only: not compared by RegisterSchema.
	Doc string `json:"doc,omitempty"`

	// EmbeddedVia lists the embedded objects the component is promoted
	// through, outermost first (e.g. ["Common"]); empty when the object
	// declares it directly. Documentation only: not compared by RegisterSchema.
	EmbeddedVia []string `json:"embeddedVia,omitempty"`

	// Decode unmarshals JSON into a value of the component type T (not *T),
	// ready to be stored in a World.
	Decode func(json.RawMessage) (any, error) `json:"-"`
}

// Key returns the component's key "group:name", which names it in JSON
// instances, prefab overrides and error messages.
func (c Component) Key() string { return ComponentKey(c.Group, c.Name) }

// MarshalJSON writes the component with its identity as "component": "group:name".
func (c Component) MarshalJSON() ([]byte, error) {
	type plain Component // drops the methods, so this does not recurse
	return json.Marshal(struct {
		Component string `json:"component"`
		plain
	}{c.Key(), plain(c)})
}

// UnmarshalJSON reads the form MarshalJSON writes.
func (c *Component) UnmarshalJSON(data []byte) error {
	type plain Component
	v := struct {
		Component string `json:"component"`
		*plain
	}{plain: (*plain)(c)}
	if err := json.Unmarshal(data, &v); err != nil {
		return err
	}
	group, name, ok := strings.Cut(v.Component, ":")
	if !ok {
		return fmt.Errorf("component key %q is not group:name", v.Component)
	}
	c.Group, c.Name = group, name
	return nil
}

// Schema describes a game object: the components it is made of.
type Schema struct {
	SchemaMeta

	// Doc is the doc comment of the object type. Documentation only.
	Doc string `json:"doc,omitempty"`

	// GoType is the Go struct of the object, "importpath.Type". Documentation
	// only: RegisterSchema does not compare it.
	GoType string `json:"goType,omitempty"`

	// Components are the object's components in declaration order. A component
	// is required unless Optional is set.
	Components []Component `json:"components"`
}

var registry = struct {
	sync.RWMutex
	schemas map[string]Schema
	// classes holds every registered key in registration order. It is
	// append-only, so an ObjectClass (an index into it) stays valid for the
	// life of the process.
	classes []string
	// classIndex maps a key to its index in classes.
	classIndex map[string]uint32
}{schemas: map[string]Schema{}, classIndex: map[string]uint32{}}

// RegisterSchema registers a game object schema under its key. It panics if a
// component has an empty group or name, a duplicate key, a nil Schema or
// Decode, or is both Readonly and Optional, and if the key is already
// registered with a different definition (registering an identical one again
// is a no-op; Decode is not compared).
func RegisterSchema(s Schema) {
	key := s.Key()
	seen := make(map[string]bool, len(s.Components))
	for _, c := range s.Components {
		ck := c.Key()
		switch {
		case c.Group == "" || c.Name == "":
			panic(fmt.Sprintf("god: schema %s has a component without a group or name", key))
		case seen[ck]:
			panic(fmt.Sprintf("god: schema %s has duplicate component %s", key, ck))
		case c.Schema == nil:
			panic(fmt.Sprintf("god: schema %s component %s has no schema", key, ck))
		case c.Decode == nil:
			panic(fmt.Sprintf("god: schema %s component %s has no decoder", key, ck))
		case c.Readonly && c.Optional:
			panic(fmt.Sprintf("god: schema %s component %s cannot be both readonly and optional", key, ck))
		}
		seen[ck] = true
	}

	registry.Lock()
	defer registry.Unlock()
	if prev, exists := registry.schemas[key]; exists {
		if !sameComponents(prev.Components, s.Components) {
			panic(fmt.Sprintf("god: schema %s is already registered with a different definition", key))
		}
		return
	}
	registry.schemas[key] = s
	registry.classIndex[key] = uint32(len(registry.classes))
	registry.classes = append(registry.classes, key)
}

// sameComponents compares component definitions, ignoring Decode (funcs are
// not comparable) and documentation (Doc, EmbeddedVia, schema descriptions).
func sameComponents(a, b []Component) bool {
	return slices.EqualFunc(a, b, func(x, y Component) bool {
		return x.Group == y.Group && x.Name == y.Name && x.Field == y.Field &&
			x.Optional == y.Optional && x.Readonly == y.Readonly &&
			x.RuntimeID == y.RuntimeID && reflect.DeepEqual(x.Schema.WithoutDescriptions(), y.Schema.WithoutDescriptions())
	})
}

// LookupSchema returns the schema registered under group and class.
func LookupSchema(group, class string) (Schema, bool) {
	registry.RLock()
	defer registry.RUnlock()
	s, ok := registry.schemas[SchemaMeta{Group: group, Class: class}.Key()]
	return s, ok
}

// Schemas returns every registered schema, sorted by key. It iterates a
// snapshot taken when iteration starts, so registering a schema meanwhile
// neither blocks nor shows up in the running iteration.
func Schemas() iter.Seq[Schema] {
	return func(yield func(Schema) bool) {
		registry.RLock()
		list := slices.Collect(maps.Values(registry.schemas))
		registry.RUnlock()
		slices.SortFunc(list, func(a, b Schema) int { return strings.Compare(a.Key(), b.Key()) })
		for _, s := range list {
			if !yield(s) {
				return
			}
		}
	}
}

// Object pairs an object's identity (group:class) with the entity that holds
// its components. Deserialize returns these in document order; passing the
// result straight to Serialize writes back the same document shape.
type Object struct {
	SchemaMeta
	// ID is the object's document id ("id" in the document), unique within
	// one document. It is empty for objects without one. Entity references
	// (component fields of type ecs.Entity) point at objects by ID.
	ID string
	// Prefab is the key of the prefab the object is an instance of, or "".
	// Deserialize fills it for entries with a "prefab", [Objects] from the
	// entity's [PrefabInstance] tag; Serialize then writes only what
	// differs from the prefab.
	Prefab string
	// Part is the id within the prefab of an object that is not the root of
	// its instance: `sword` or `horse/saddle` for an object of a
	// multi-object prefab. It is "" for plain objects and instance roots.
	// Its ID is `<root ID>/<Part>`. Deserialize returns the parts of an
	// instance right after its root, in prefab order; Serialize folds them
	// back into the root's entry as "children".
	Part   string
	Entity ecs.Entity
}

// instance is one object of a Deserialize stream.
type instance struct {
	ID         string                     `json:"id"`
	Prefab     string                     `json:"prefab"`
	Group      string                     `json:"group"`
	Class      string                     `json:"class"`
	Components map[string]json.RawMessage `json:"components"`
	// Children are the overrides of the parts of a prefab instance, see
	// [PrefabEntry.Children].
	Children map[string]json.RawMessage `json:"children"`
}

// Deserialize reads a JSON document of the form
//
//	{"objects": [{"id": "hero", "group": "g", "class": "c", "components": {"g:Comp": {...}}}, ...]}
//
// and creates one entity per object in world, with the object's components
// and its [ObjectClass] tag. "id" is optional and must be unique; component
// fields of type ecs.Entity hold the id of another object of the document
// (or null), forward references included. Each object is checked against
// its registered schema: missing required components and components that
// are not registered are errors; an object may hold any registered
// component type, not only those of its class. It returns the objects in input order. On error no entity of this
// call remains in the world.
//
// An entry with "prefab": "<key>" is an instance of that prefab, found
// through the world's [PrefabSource]: group and class may be left out, and
// its components are overrides merged over the prefab's (see [Prefab]).
// Such entities also get a [PrefabInstance] tag. An instance of a prefab
// with several objects (it needs an "id") also creates the other objects,
// returned right after the root with ids `<id>/<part>` and [Object.Part]
// set; "children" overrides them (see [PrefabEntry]). Authored ids must not
// contain "/".
func Deserialize(world *ecs.World, r io.Reader) ([]Object, error) {
	insts, _, err := readDocument(r)
	if err != nil {
		return nil, err
	}

	// Phase 0: an instance of a prefab becomes plain objects, one for each
	// object of the prefab.
	flat := make([]flatInstance, 0, len(insts))
	var errs []error
	for i, in := range insts {
		if err := checkAuthoredID(in.ID); err != nil {
			errs = append(errs, fmt.Errorf("objects[%d]: %w", i, err))
		}
		if in.Prefab == "" {
			if in.Children != nil {
				errs = append(errs, fmt.Errorf("objects[%d]: \"children\" needs a \"prefab\"", i))
			}
			flat = append(flat, flatInstance{instance: in, origin: i})
			continue
		}
		objs, err := expandInstance(world, in)
		if err != nil {
			errs = append(errs, fmt.Errorf("objects[%d]: %w", i, err))
			continue
		}
		for _, o := range objs {
			flat = append(flat, flatInstance{
				instance: instance{ID: o.ID, Prefab: in.Prefab, Group: o.Group, Class: o.Class, Components: o.Components},
				part:     o.Part, origin: i,
			})
		}
	}

	// Phase 1: check every object and collect the ids, so that references
	// (which may point forward) can be resolved in phase 2.
	schemas := make([]Schema, len(flat))
	seen := make(map[string]int, len(flat))
	for i, f := range flat {
		s, err := checkInstance(f.instance)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", f.label(), err))
		}
		schemas[i] = s
		if f.ID == "" {
			continue
		}
		if first, dup := seen[f.ID]; dup {
			errs = append(errs, fmt.Errorf("%s: duplicate id %q (first used by %s)", f.label(), f.ID, flat[first].label()))
			continue
		}
		seen[f.ID] = i
	}
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}

	// Phase 2: one entity per object (tag only), then decode every object's
	// components with references resolved and set them at once.
	objs := make([]Object, len(flat))
	for i, f := range flat {
		var e ecs.Entity
		if f.Prefab != "" {
			e = world.NewEntity(mustClassTag(schemas[i].SchemaMeta), prefabPartTag(f.Prefab, f.part))
		} else {
			e = world.NewEntity(mustClassTag(schemas[i].SchemaMeta))
		}
		objs[i] = Object{SchemaMeta: schemas[i].SchemaMeta, ID: f.ID, Prefab: f.Prefab, Part: f.part, Entity: e}
	}
	refs := ObjectRefs(objs)
	for i, f := range flat {
		values, err := decodeInstance(world, schemas[i], f.instance, refs)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", f.label(), err))
			continue
		}
		world.SetComp(objs[i].Entity, values...)
	}
	if err := errors.Join(errs...); err != nil {
		for _, o := range objs {
			world.DeleteEntity(o.Entity)
		}
		return nil, err
	}
	return objs, nil
}

// readDocument reads a serialized GOD document: {"name": "...", "objects":
// [...]}, nothing else, and nothing after it. The optional "name" is a label
// for editors; it is returned (or "") and only prefabs keep it.
func readDocument(r io.Reader) (insts []instance, name string, err error) {
	dec := json.NewDecoder(r)
	dec.DisallowUnknownFields()
	var doc struct {
		Name    string      `json:"name"`
		Objects *[]instance `json:"objects"`
	}
	if err := dec.Decode(&doc); err != nil {
		return nil, "", fmt.Errorf("deserialize: %w", err)
	}
	if doc.Objects == nil {
		return nil, "", errors.New("deserialize: missing \"objects\"")
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, "", errors.New("deserialize: unexpected data after the JSON document")
	}
	return *doc.Objects, doc.Name, nil
}

// flatInstance is one object of a Deserialize stream after prefab instances
// are expanded: an instance of a multi-object prefab gives several.
type flatInstance struct {
	instance
	// part is the object's part id within its prefab, "" for roots and plain
	// objects.
	part string
	// origin is the index of the document entry it comes from.
	origin int
}

// label names the object in errors: its entry, and the part for the
// objects an instance expands to.
func (f flatInstance) label() string {
	if f.part != "" {
		return fmt.Sprintf("objects[%d] part %q", f.origin, f.part)
	}
	return fmt.Sprintf("objects[%d]", f.origin)
}

// expandInstance turns an entry with a "prefab" into plain objects through
// the world's PrefabSource.
func expandInstance(world *ecs.World, in instance) ([]ExpandedObject, error) {
	p, err := lookupPrefab(worldPrefabSource(world), in.Prefab)
	if err != nil {
		return nil, err
	}
	return p.ExpandInstance(in.entry())
}

// checkInstance looks up the schema of one object and checks its component
// set and each component's JSON against its TypeSchema.
func checkInstance(obj instance) (Schema, error) {
	s, ok := LookupSchema(obj.Group, obj.Class)
	if !ok {
		return Schema{}, fmt.Errorf("unknown object %s", SchemaMeta{Group: obj.Group, Class: obj.Class}.Key())
	}
	if err := CheckComponents(obj.Group, obj.Class, obj.Components); err != nil {
		return Schema{}, err
	}
	var errs []error
	for _, c := range ObjectComponents(s, maps.Keys(obj.Components)) {
		if raw, ok := obj.Components[c.Key()]; ok {
			if err := c.Schema.Validate(raw); err != nil {
				errs = append(errs, fmt.Errorf("component %s: %w", c.Key(), err))
			}
		}
	}
	return s, errors.Join(errs...)
}

// decodeInstance decodes the components of one checked object.
func decodeInstance(world *ecs.World, s Schema, obj instance, refs Refs) ([]any, error) {
	values := make([]any, 0, len(obj.Components))
	var errs []error
	for _, c := range ObjectComponents(s, maps.Keys(obj.Components)) {
		raw, ok := obj.Components[c.Key()]
		if !ok {
			continue
		}
		v, err := DecodeComponent(world, c, raw, refs)
		if err != nil {
			errs = append(errs, fmt.Errorf("component %s: %w", c.Key(), err))
			continue
		}
		values = append(values, v)
	}
	return values, errors.Join(errs...)
}

// Serialize writes one JSON document to w with one entry per Object, in
// argument order — the inverse of Deserialize. Each object gets the
// components of its class the entity has, plus every other registered
// component type it has; optional components it lacks are left out; a missing required component or an
// unregistered group:class is an error, prefixed with the object's index
// ("objects[i]: ..."). The whole document is built before anything is
// written, so on error nothing reaches w.
//
// Entity references are written as object ids: an object's id is its
// Object.ID, and a referenced object without one gets "_<i>" (its index in
// objs). "id" is written only for objects that have one after that, so a
// document without references has no ids. A reference to an entity that is
// not among objs, and two objects with the same id, are errors.
//
// An Object with a Prefab is written as an instance of that prefab (found
// through the world's [PrefabSource]): "prefab" instead of group and class,
// and as "components" only what differs from the prefab (a removed
// optional component is null); "components" is left out when nothing
// differs. The objects of an instance with [Object.Part] set (their ID is
// `<root ID>/<Part>`) are not entries of their own: they are folded into
// the root's "children", again as what differs from the prefab, and a part
// of the prefab that is not among objs is written as null. A part without
// its root among objs, or with another ID, is an error.
//
// Panics with "ecs: invalid or stale entity: <e>" if any Entity is stale
// (see the ecs panic table).
func Serialize(world *ecs.World, w io.Writer, objs ...Object) error {
	type docObject struct {
		ID         string          `json:"id,omitempty"`
		Prefab     string          `json:"prefab,omitempty"`
		Group      string          `json:"group,omitempty"`
		Class      string          `json:"class,omitempty"`
		Components json.RawMessage `json:"components,omitempty"`
		Children   json.RawMessage `json:"children,omitempty"`
	}
	entries := make([]docObject, len(objs))

	refs := newSerializeRefs(objs)
	parts, errs := groupParts(objs)
	for i, obj := range objs {
		if obj.Part != "" {
			continue
		}
		if err := checkAuthoredID(obj.ID); err != nil {
			errs = append(errs, fmt.Errorf("objects[%d]: %w", i, err))
		}
		if obj.Prefab != "" {
			comps, children, err := encodeInstance(world, objs, i, parts[i], refs)
			if err != nil {
				errs = append(errs, fmt.Errorf("objects[%d]: %w", i, err))
			}
			entries[i] = docObject{Prefab: obj.Prefab, Components: comps, Children: children}
			continue
		}
		comps, err := encodeComponents(world, obj.Group, obj.Class, obj.Entity, refs)
		if err != nil {
			errs = append(errs, fmt.Errorf("objects[%d]: %w", i, err))
		}
		entries[i] = docObject{Group: obj.Group, Class: obj.Class, Components: comps}
	}
	ids := make(map[string]int, len(objs))
	for i := range objs {
		id := refs.id(i)
		if id == "" {
			continue
		}
		if first, dup := ids[id]; dup {
			errs = append(errs, fmt.Errorf("objects[%d]: duplicate id %q (first used by objects[%d])", i, id, first))
		}
		ids[id] = i
		entries[i].ID = id
	}
	if err := errors.Join(errs...); err != nil {
		return err
	}

	doc := struct {
		Objects []docObject `json:"objects"`
	}{Objects: make([]docObject, 0, len(objs))}
	for i, obj := range objs {
		if obj.Part == "" {
			doc.Objects = append(doc.Objects, entries[i])
		}
	}
	data, err := json.Marshal(doc)
	if err != nil {
		return err
	}
	_, err = w.Write(data)
	return err
}

// groupParts finds the root of each object with a Part: the object with
// the same Prefab and the ID the part's ID minus "/<Part>". It returns the
// parts of each root by root index and part id, and the problems found.
func groupParts(objs []Object) (map[int]map[string]int, []error) {
	roots := map[string]int{}
	for i, o := range objs {
		if o.Prefab != "" && o.Part == "" && o.ID != "" {
			roots[o.ID] = i
		}
	}
	parts := map[int]map[string]int{}
	var errs []error
	for i, o := range objs {
		if o.Part == "" {
			continue
		}
		rootID, ok := strings.CutSuffix(o.ID, "/"+o.Part)
		root, found := roots[rootID]
		switch {
		case o.Prefab == "":
			errs = append(errs, fmt.Errorf("objects[%d]: part %q without a prefab", i, o.Part))
		case !ok:
			errs = append(errs, fmt.Errorf("objects[%d]: part %q of prefab %q needs the ID \"<root ID>/%s\", has %q", i, o.Part, o.Prefab, o.Part, o.ID))
		case !found || objs[root].Prefab != o.Prefab:
			errs = append(errs, fmt.Errorf("objects[%d]: part %q of prefab %q has no root object %q", i, o.Part, o.Prefab, rootID))
		default:
			if parts[root] == nil {
				parts[root] = map[string]int{}
			}
			parts[root][o.Part] = i
		}
	}
	return parts, errs
}

// serializeRefs are the Refs of Serialize: Object.ID, or "_<i>" for a
// referenced object without one.
type serializeRefs struct {
	objs       []Object
	index      map[ecs.Entity]int
	referenced []bool
}

func newSerializeRefs(objs []Object) *serializeRefs {
	r := &serializeRefs{objs: objs, index: make(map[ecs.Entity]int, len(objs)), referenced: make([]bool, len(objs))}
	for i, o := range objs {
		if _, dup := r.index[o.Entity]; !dup {
			r.index[o.Entity] = i
		}
	}
	return r
}

// id is the final id of objs[i]: its own, its automatic one, or "".
func (r *serializeRefs) id(i int) string {
	switch {
	case r.objs[i].ID != "":
		return r.objs[i].ID
	case r.referenced[i]:
		return "_" + strconv.Itoa(i)
	}
	return ""
}

func (r *serializeRefs) RefID(e ecs.Entity) (string, error) {
	i, ok := r.index[e]
	if !ok {
		return "", fmt.Errorf("reference to entity %v, which is not one of the serialized objects", e)
	}
	r.referenced[i] = true
	return r.id(i), nil
}

func (r *serializeRefs) RefEntity(string) (ecs.Entity, error) {
	return ecs.Entity{}, errors.New("serializeRefs: decoding is not supported")
}

// SerializeComponents returns the JSON `components` object of e (a map of
// component key "group:name" to its JSON value), as described by the schema registered
// as group:class. Optional components e lacks are left out; a missing
// required component is an error. Use it to embed object data in your own
// file format. It has no document, so a non-zero entity reference is an
// error ("no Refs for entity reference"); use [EncodeComponent] with Refs
// for those.
//
// Panics with "ecs: invalid or stale entity: <e>" if e is stale.
func SerializeComponents(world *ecs.World, group, class string, e ecs.Entity) (json.RawMessage, error) {
	return encodeComponents(world, group, class, e, nil)
}

// encodeInstance writes objs[root], an instance of the prefab it names,
// as the overrides of its "components" and "children": only what differs
// from the prefab, nil if nothing does. parts are the instance's other
// objects, by part id.
func encodeInstance(world *ecs.World, objs []Object, root int, parts map[string]int, refs Refs) (comps, children json.RawMessage, err error) {
	obj := objs[root]
	p, err := lookupPrefab(worldPrefabSource(world), obj.Prefab)
	if err != nil {
		return nil, nil, err
	}
	if meta := p.Meta(); obj.SchemaMeta != (SchemaMeta{}) && obj.SchemaMeta != meta {
		return nil, nil, fmt.Errorf("object %s does not match prefab %q, which is %s", obj.Key(), p.key, meta.Key())
	}
	if len(p.parts) > 1 && obj.ID == "" {
		return nil, nil, fmt.Errorf("instance of multi-object prefab %q needs an ID", p.key)
	}
	// The prefab's objects in the document's ids are what the entities
	// are compared with.
	base, err := p.ExpandInstance(PrefabEntry{ID: obj.ID})
	if err != nil {
		return nil, nil, err
	}
	var errs []error
	for _, name := range sortedKeys(parts) {
		if !slices.ContainsFunc(base, func(o ExpandedObject) bool { return o.Part == name }) {
			errs = append(errs, fmt.Errorf("prefab %q has no object %q", p.key, name))
		}
	}
	if err := errors.Join(errs...); err != nil {
		return nil, nil, err
	}

	over, err := encodeOverrides(world, p.parts[0].schema, base[0], obj.Entity, refs)
	if err != nil {
		return nil, nil, err
	}
	kids := map[string]json.RawMessage{}
	for _, b := range base[1:] {
		i, ok := parts[b.Part]
		if !ok {
			kids[b.Part] = json.RawMessage("null")
			continue
		}
		o, err := encodeOverrides(world, p.parts[p.index[b.Part]].schema, b, objs[i].Entity, refs)
		if err != nil {
			errs = append(errs, fmt.Errorf("part %q: %w", b.Part, err))
		} else if len(o) > 0 {
			kids[b.Part] = o
		}
	}
	if err := errors.Join(errs...); err != nil {
		return nil, nil, err
	}
	if len(over) > 0 {
		comps = over
	}
	if len(kids) > 0 {
		if children, err = json.Marshal(kids); err != nil {
			return nil, nil, err
		}
	}
	return comps, children, nil
}

// encodeOverrides returns the JSON of the components of e that differ from
// those of base, an object of the prefab expanded for the document, or nil.
func encodeOverrides(world *ecs.World, s Schema, base ExpandedObject, e ecs.Entity, refs Refs) (json.RawMessage, error) {
	comps, err := encodeComponentMap(world, base.Group, base.Class, e, refs)
	if err != nil {
		return nil, err
	}
	over, err := diffComponents(s, base.Components, comps)
	if err != nil || len(over) == 0 {
		return nil, err
	}
	return json.Marshal(over)
}

func encodeComponents(world *ecs.World, group, class string, e ecs.Entity, refs Refs) (json.RawMessage, error) {
	out, err := encodeComponentMap(world, group, class, e, refs)
	if err != nil {
		return nil, err
	}
	return json.Marshal(out)
}

// encodeComponentMap encodes every component e has of the object
// group:class, keyed by component key: the class's components, then every
// other registered component type e has (see [ObjectComponents]). Walking
// the registry costs one HasComp per registered type; Serialize is a tool
// path, not a per-tick one.
func encodeComponentMap(world *ecs.World, group, class string, e ecs.Entity, refs Refs) (map[string]json.RawMessage, error) {
	s, ok := LookupSchema(group, class)
	if !ok {
		return nil, fmt.Errorf("unknown object %s", SchemaMeta{Group: group, Class: class}.Key())
	}
	out := make(map[string]json.RawMessage, len(s.Components))
	var errs []error
	encode := func(c Component, v any) {
		data, err := EncodeComponent(world, c, v, refs)
		if err != nil {
			errs = append(errs, fmt.Errorf("component %s: %w", c.Key(), err))
			return
		}
		out[c.Key()] = data
	}
	// done holds the runtime IDs already written, so a registered type the
	// class declares is not written twice.
	done := make(map[ecs.ComponentID]bool, len(s.Components))
	for _, c := range s.Components {
		done[c.RuntimeID] = true
		v := world.GetCompOrNil(e, c.RuntimeID)
		if v == nil {
			if !c.Optional {
				errs = append(errs, fmt.Errorf("object %s: missing required component %s", s.Key(), c.Key()))
			}
			continue
		}
		encode(c, v)
	}
	for rc := range Components() {
		if done[rc.RuntimeID] {
			continue
		}
		if v := world.GetCompOrNil(e, rc.RuntimeID); v != nil {
			done[rc.RuntimeID] = true
			encode(extraComponent(rc), v)
		}
	}
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}
	return out, nil
}

// CheckComponents verifies that raw holds every required component of the
// object registered under group:class, and that every other key names a
// registered component type (see [RegisterComponent]): the class sets the
// minimum of an object, not its limit. All problems are reported, in key
// order.
func CheckComponents(group, class string, raw map[string]json.RawMessage) error {
	key := SchemaMeta{Group: group, Class: class}.Key()
	registry.RLock()
	s, ok := registry.schemas[key]
	registry.RUnlock()
	if !ok {
		return fmt.Errorf("unknown object %s", key)
	}
	var errs []error
	known := make(map[string]bool, len(s.Components))
	for _, c := range s.Components {
		known[c.Key()] = true
		if _, ok := raw[c.Key()]; !ok && !c.Optional {
			errs = append(errs, fmt.Errorf("object %s: missing required component %s", key, c.Key()))
		}
	}
	for _, k := range sortedKeys(raw) {
		if known[k] {
			continue
		}
		if _, ok := registeredComponent(k); !ok {
			errs = append(errs, fmt.Errorf("object %s: component %s is not registered", key, k))
		}
	}
	return errors.Join(errs...)
}

// ComponentOf returns how an object of class s holds the component with
// key "group:name": the class's own entry, or, for any other component type
// in the component registry, an optional Component built from its
// [ComponentSchema]. ok is false for a key that names neither.
func ComponentOf(s Schema, key string) (Component, bool) {
	for _, c := range s.Components {
		if c.Key() == key {
			return c, true
		}
	}
	return registeredComponent(key)
}

// ObjectComponents returns the components of an object of class s that has
// the component keys keys: every component of the class in declaration
// order, then every other key that names a registered component type,
// sorted, as optional components (see [ComponentOf]). Keys that name no
// registered type are left out; [CheckComponents] reports them.
func ObjectComponents(s Schema, keys iter.Seq[string]) []Component {
	out := slices.Clone(s.Components)
	var extra []string
	for k := range keys {
		if !slices.ContainsFunc(s.Components, func(c Component) bool { return c.Key() == k }) {
			extra = append(extra, k)
		}
	}
	slices.Sort(extra)
	for _, k := range slices.Compact(extra) {
		if c, ok := registeredComponent(k); ok {
			out = append(out, c)
		}
	}
	return out
}

// registeredComponent returns the component type registered as key, as an
// optional Component.
func registeredComponent(key string) (Component, bool) {
	group, name, ok := strings.Cut(key, ":")
	if !ok {
		return Component{}, false
	}
	rc, ok := LookupComponent(group, name)
	if !ok {
		return Component{}, false
	}
	return extraComponent(rc), true
}

// extraComponent is a registered component type used by an object whose
// class does not declare it: always optional.
func extraComponent(rc ComponentSchema) Component {
	return Component{Group: rc.Group, Name: rc.Name, Optional: true, RuntimeID: rc.RuntimeID, Schema: rc.Schema, Decode: rc.Decode, Doc: rc.Doc}
}

// DecodeFunc returns a Component.Decode for component type T.
func DecodeFunc[T any]() func(json.RawMessage) (any, error) {
	return func(raw json.RawMessage) (any, error) {
		var v T
		if err := json.Unmarshal(raw, &v); err != nil {
			return nil, err
		}
		return v, nil
	}
}
