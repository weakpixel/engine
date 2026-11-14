package god

import (
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"slices"

	"github.com/weakpixel/ecs"
)

// BoundPrefab is a [Prefab] decoded for one World: its component values,
// with asset keys already resolved, ready to be copied into new entities.
// Get one with [Prefab.Bind]. Like the World it is single-goroutine.
type BoundPrefab struct {
	prefab *Prefab
	world  *ecs.World
	assets AssetResolver

	// parts holds the decoded objects of the prefab, the root first.
	parts []boundPart
	// types[i] is the type of parts[0].values[i] for the root's components
	// (not its tags); refTypes[i] the type of parts[0].refs[i].
	types    []reflect.Type
	refTypes []reflect.Type
	// allowed are the Go types of every component of the root and its
	// class, for overrides the prefab lacks. Spawn adds other registered
	// types the first time it sees them, so an override of such a type
	// allocates only once.
	allowed map[reflect.Type]bool
	scratch []any
	skip    []bool
	// ents are the entities of the last spawn, in part order.
	ents []ecs.Entity
}

// boundPart is one decoded object of a prefab.
type boundPart struct {
	// values holds the decoded components without entity references, then
	// the ObjectClass and PrefabInstance tags.
	values []any
	// refs are the components with entity references; they can only be
	// set once all entities of the spawn exist.
	refs []boundRef
}

// boundRef is a component of a part that references other objects of the
// prefab.
type boundRef struct {
	c   Component
	t   reflect.Type
	raw json.RawMessage
	// fixed is set when every reference sits at a fixed position of the Go
	// value (struct fields and arrays, no slices, maps or pointers). The
	// value is then decoded once, with placeholder entities: tmpl holds it
	// and sites say where the placeholders are, and Spawn copies it into
	// scratch and patches the sites. Otherwise Spawn decodes raw again.
	fixed   bool
	tmpl    reflect.Value
	scratch reflect.Value
	sites   []refSite
}

// refSite is the position of an ecs.Entity in a component value that
// references the object part of the prefab.
type refSite struct {
	path []int // field or array indexes from the component value
	part int
}

// placeholderGen marks the placeholder entities Bind decodes references
// to: ID is the part index. A live entity never has this generation.
const placeholderGen = ^uint32(0)

// Bind decodes p's components for world, resolving asset keys with assets
// (nil means the world's AssetResolver, see SetAssetResolver). Handles it
// resolves are held by whatever scope the resolver loads into. Bind once
// and spawn many times.
func (p *Prefab) Bind(world *ecs.World, assets AssetResolver) (*BoundPrefab, error) {
	if assets == nil {
		assets = worldAssetResolver(world)
	}
	b := &BoundPrefab{prefab: p, world: world, assets: assets, parts: make([]boundPart, len(p.parts)), allowed: map[reflect.Type]bool{}}
	var errs []error
	for i, part := range p.parts {
		if err := b.bindPart(i, part); err != nil {
			if len(p.parts) > 1 {
				err = fmt.Errorf("object %q: %w", part.id, err)
			}
			errs = append(errs, err)
		}
	}
	if err := errors.Join(errs...); err != nil {
		return nil, fmt.Errorf("prefab %q: %w", p.key, err)
	}
	for i := range p.parts {
		b.parts[i].values = append(b.parts[i].values, mustClassTag(p.parts[i].schema.SchemaMeta), prefabPartTag(p.key, partName(p, i)))
	}
	b.skip = make([]bool, len(b.parts[0].refs))
	return b, nil
}

// partName is the Part of the object i of p: "" for the root.
func partName(p *Prefab, i int) string {
	if i == 0 {
		return ""
	}
	return p.parts[i].id
}

// bindPart decodes the components of part i.
func (b *BoundPrefab) bindPart(i int, part prefabPart) error {
	bp := &b.parts[i]
	var errs []error
	for _, c := range part.comps {
		zero, err := c.Decode(json.RawMessage("null"))
		if err != nil {
			errs = append(errs, fmt.Errorf("component %s: %w", c.Key(), err))
			continue
		}
		t := reflect.TypeOf(zero)
		if i == 0 {
			b.allowed[t] = true
		}
		raw, ok := part.components[c.Key()]
		if !ok {
			continue
		}
		if hasEntityRef(c.Schema) {
			r, err := b.bindRef(c, t, raw)
			if err != nil {
				errs = append(errs, fmt.Errorf("component %s: %w", c.Key(), err))
				continue
			}
			bp.refs = append(bp.refs, r)
			if i == 0 {
				b.refTypes = append(b.refTypes, t)
			}
			continue
		}
		v, err := decodeComponent(b.assets, c, raw, nil)
		if err != nil {
			errs = append(errs, fmt.Errorf("component %s: %w", c.Key(), err))
			continue
		}
		bp.values = append(bp.values, v)
		if i == 0 {
			b.types = append(b.types, t)
		}
	}
	return errors.Join(errs...)
}

// bindRef decodes a component with entity references once, with
// placeholder entities, and finds out where they sit.
func (b *BoundPrefab) bindRef(c Component, t reflect.Type, raw json.RawMessage) (boundRef, error) {
	r := boundRef{c: c, t: t, raw: raw}
	v, err := decodeComponent(b.assets, c, raw, placeholderRefs{b.prefab.index})
	if err != nil {
		return r, err
	}
	// Count the references in the JSON and the placeholders in the value:
	// equal means every reference is at a fixed position.
	n := 0
	_, err = mapRefs(c.Schema, raw, func(_ string, v any) (any, error) {
		if v != nil {
			n++
		}
		return v, nil
	})
	if err != nil {
		return r, err
	}
	tmpl := reflect.New(t).Elem()
	tmpl.Set(reflect.ValueOf(v))
	r.sites = findPlaceholders(tmpl, nil, len(b.parts), nil)
	if len(r.sites) == n {
		r.fixed, r.tmpl, r.scratch = true, tmpl, reflect.New(t).Elem()
	} else {
		r.sites = nil
	}
	return r, nil
}

// entityType is the reflect type of ecs.Entity.
var entityType = reflect.TypeFor[ecs.Entity]()

// findPlaceholders appends the sites of the placeholder entities in v,
// looking through exported struct fields and arrays.
func findPlaceholders(v reflect.Value, path []int, parts int, sites []refSite) []refSite {
	switch v.Kind() {
	case reflect.Struct:
		if v.Type() == entityType {
			e := v.Interface().(ecs.Entity)
			if e.Generation == placeholderGen && int(e.ID) < parts {
				sites = append(sites, refSite{path: slices.Clone(path), part: int(e.ID)})
			}
			return sites
		}
		for i := range v.NumField() {
			if v.Type().Field(i).IsExported() {
				sites = findPlaceholders(v.Field(i), append(path, i), parts, sites)
			}
		}
	case reflect.Array:
		for i := range v.Len() {
			sites = findPlaceholders(v.Index(i), append(path, i), parts, sites)
		}
	}
	return sites
}

// placeholderRefs resolves the prefab's ids to placeholder entities.
type placeholderRefs struct{ index map[string]int }

func (r placeholderRefs) RefID(ecs.Entity) (string, error) {
	return "", errors.New("placeholderRefs: encoding is not supported")
}

func (r placeholderRefs) RefEntity(id string) (ecs.Entity, error) {
	i, ok := r.index[id]
	if !ok {
		return ecs.Entity{}, fmt.Errorf("unknown object id %q", id)
	}
	return ecs.Entity{ID: ecs.EntityID(i), Generation: placeholderGen}, nil
}

// Prefab returns the prefab b was bound from.
func (b *BoundPrefab) Prefab() *Prefab { return b.prefab }

// Spawn creates one entity per object of the prefab, each with the object's
// components, its [ObjectClass] and its [PrefabInstance] tag, and returns the
// root. Each override is a component value (a T, not *T) that replaces the
// root's value of that type, or adds a component the root lacks (any
// registered component type, see [RegisterComponent]): typically the
// position, which moves a group whose parts follow the root.
//
//	e := goblin.Spawn(gfx.Transform{Pos: at, PrevPos: at, Scale: gfx.Vec2{X: 1, Y: 1}})
//
// No JSON is read: the values were decoded by Bind. Entity references
// between the objects are patched into a copy of the decoded value, which
// costs one allocation per component with references; only a reference
// below a slice, a map or a pointer is decoded from JSON per spawn. To get
// the other entities use [BoundPrefab.SpawnAll].
//
// Panics with "god: Spawn: <T> is not a registered component" for an
// override of a type that is neither in the prefab's class nor in the
// component registry, and with the ecs panics of NewEntity.
func (b *BoundPrefab) Spawn(overrides ...any) ecs.Entity {
	b.spawn(overrides)
	return b.ents[0]
}

// SpawnAll is [BoundPrefab.Spawn] that appends every spawned entity to dst
// in [Prefab.Parts] order, the root first, and returns it. A caller that
// reuses dst allocates nothing for the slice.
func (b *BoundPrefab) SpawnAll(dst []ecs.Entity, overrides ...any) []ecs.Entity {
	b.spawn(overrides)
	return append(dst, b.ents...)
}

// spawn creates the entities into b.ents. All entities are created with
// their reference-free values first, then the components with references
// are set, when every entity they can point at exists.
func (b *BoundPrefab) spawn(overrides []any) {
	root := &b.parts[0]
	// scratch is reused across spawns so that a spawn without entity
	// references allocates nothing here; NewEntity copies the values.
	b.scratch = append(b.scratch[:0], root.values...)
	clear(b.skip)
	for _, o := range overrides {
		t := reflect.TypeOf(o)
		if i := slices.Index(b.types, t); i >= 0 {
			b.scratch[i] = o
			continue
		}
		if i := slices.Index(b.refTypes, t); i >= 0 {
			b.skip[i] = true
		} else if !b.allowed[t] {
			if _, ok := componentByType(t); !ok {
				panic(fmt.Sprintf("god: Spawn: %v is not a registered component", t))
			}
			b.allowed[t] = true
		}
		b.scratch = append(b.scratch, o)
	}
	b.ents = append(b.ents[:0], b.world.NewEntity(b.scratch...))
	clear(b.scratch) // drop the overrides' boxes
	for i := 1; i < len(b.parts); i++ {
		b.ents = append(b.ents, b.world.NewEntity(b.parts[i].values...))
	}
	for i := range b.parts {
		for j := range b.parts[i].refs {
			if i == 0 && b.skip[j] {
				continue
			}
			b.setRef(b.ents[i], &b.parts[i].refs[j])
		}
	}
}

// setRef sets the component r of e, with its references pointing at the
// entities of the current spawn.
func (b *BoundPrefab) setRef(e ecs.Entity, r *boundRef) {
	if !r.fixed {
		v, err := decodeComponent(b.assets, r.c, r.raw, spawnRefs{b.prefab.index, b.ents})
		if err != nil {
			// The JSON was validated by ParsePrefab and refers to nothing
			// but the prefab's own objects.
			panic(fmt.Sprintf("god: Spawn: prefab %q: component %s: %v", b.prefab.key, r.c.Key(), err))
		}
		b.world.SetComp(e, v)
		return
	}
	r.scratch.Set(r.tmpl)
	for _, s := range r.sites {
		f := r.scratch
		for _, i := range s.path {
			if f.Kind() == reflect.Array {
				f = f.Index(i)
			} else {
				f = f.Field(i)
			}
		}
		// Set the two fields, not the struct: boxing an ecs.Entity for
		// Value.Set would allocate.
		target := b.ents[s.part]
		f.Field(0).SetUint(uint64(target.ID))
		f.Field(1).SetUint(uint64(target.Generation))
	}
	// Interface copies the value into a new box: the one allocation.
	b.world.SetComp(e, r.scratch.Interface())
}

// spawnRefs resolves the prefab's ids to the entities being spawned.
type spawnRefs struct {
	index map[string]int
	ents  []ecs.Entity
}

func (r spawnRefs) RefID(e ecs.Entity) (string, error) {
	return "", errors.New("spawnRefs: encoding is not supported")
}

func (r spawnRefs) RefEntity(id string) (ecs.Entity, error) {
	if i, ok := r.index[id]; ok {
		return r.ents[i], nil
	}
	return ecs.Entity{}, fmt.Errorf("unknown object id %q", id)
}
