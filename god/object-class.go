package god

import (
	"fmt"
	"iter"

	"github.com/weakpixel/ecs"
)

// ObjectClass is a component that records which registered object type
// (group:class) an entity is. [Deserialize] adds it to every entity it
// creates; code that spawns objects itself can add the value returned by
// [ClassTag]. It is in no object schema, so [Serialize] never writes it.
//
// The value is an index into the process's registered schemas: 4 bytes, no
// pointers. It is only meaningful in the running process; never persist it.
// The zero value is no class.
type ObjectClass struct {
	idx uint32 // index into registry.classes + 1; 0 means none
}

// objectClassID is ObjectClass's component ID, registered once for every
// World.
var objectClassID = ecs.RegisterComp[ObjectClass]()

// ClassTag returns the ObjectClass of the schema registered as group:class,
// and false if there is none.
func ClassTag(group, class string) (ObjectClass, bool) {
	registry.RLock()
	defer registry.RUnlock()
	i, ok := registry.classIndex[SchemaMeta{Group: group, Class: class}.Key()]
	if !ok {
		return ObjectClass{}, false
	}
	return ObjectClass{idx: i + 1}, true
}

// Meta returns the group and class c stands for, and the zero SchemaMeta for
// the zero ObjectClass.
func (c ObjectClass) Meta() SchemaMeta {
	if c.idx == 0 {
		return SchemaMeta{}
	}
	registry.RLock()
	key := registry.classes[c.idx-1]
	s := registry.schemas[key]
	registry.RUnlock()
	return s.SchemaMeta
}

// ClassOf returns the object type of e. An entity with an [ObjectClass]
// component is that class. Otherwise the class is inferred from e's
// components: candidates are the schemas whose required components e all
// has, and the one with the most of its components (required and optional)
// present wins. A tie, or no candidate, returns false, as does an entity
// with no registered schema at all.
//
// Inference costs O(schemas × components) component lookups: fine for tools,
// not for per-tick game code.
//
// Panics with "ecs: invalid or stale entity: <e>" if e is stale.
func ClassOf(world *ecs.World, e ecs.Entity) (SchemaMeta, bool) {
	if tag, ok := world.GetCompOrNil(e, objectClassID).(*ObjectClass); ok && tag.idx != 0 {
		return tag.Meta(), true
	}
	var best SchemaMeta
	bestScore, tie := -1, false
	for s := range Schemas() {
		score, ok := matchScore(world, e, s)
		switch {
		case !ok || score < bestScore:
		case score == bestScore:
			tie = true
		default:
			best, bestScore, tie = s.SchemaMeta, score, false
		}
	}
	if bestScore < 0 || tie {
		return SchemaMeta{}, false
	}
	return best, true
}

// matchScore reports whether e has every required component of s and how
// many of s's components it has.
func matchScore(world *ecs.World, e ecs.Entity, s Schema) (int, bool) {
	if len(s.Components) == 0 {
		return 0, false // matches everything; never a useful answer
	}
	n := 0
	for _, c := range s.Components {
		if world.HasComp(e, c.RuntimeID) {
			n++
		} else if !c.Optional {
			return 0, false
		}
	}
	return n, true
}

// Objects yields every entity of world whose class [ClassOf] resolves, as an
// Object (with [Object.Prefab] and [Object.Part] set from its [PrefabInstance] tag), in query order (archetype, then row; not creation order). Its
// result can go straight to [Serialize]:
//
//	types.Serialize(world, w, slices.Collect(types.Objects(world))...)
//
// Structural changes to world while iterating follow the rules of
// [ecs.Query.Each].
func Objects(world *ecs.World) iter.Seq[Object] {
	return func(yield func(Object) bool) {
		for e := range world.Query().Each() {
			meta, ok := ClassOf(world, e)
			if !ok {
				continue
			}
			key, part, _ := PrefabPartOf(world, e)
			if !yield(Object{SchemaMeta: meta, Prefab: key, Part: part, Entity: e}) {
				return
			}
		}
	}
}

// mustClassTag returns the tag of a registered schema; callers have just
// looked the schema up.
func mustClassTag(meta SchemaMeta) ObjectClass {
	tag, ok := ClassTag(meta.Group, meta.Class)
	if !ok {
		panic(fmt.Sprintf("god: schema %s is not registered", meta.Key()))
	}
	return tag
}
