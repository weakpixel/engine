package god

import (
	"sync"

	"github.com/weakpixel/ecs"
)

// PrefabInstance is a component that records which prefab an entity was
// made from, and which of its objects it is. [Deserialize] adds it to the
// entities of object entries with a "prefab", and [BoundPrefab.Spawn] to
// every entity it spawns. Like [ObjectClass] it is in no object schema, so
// [Serialize] never writes it as a component; it writes the entity as an
// instance of the prefab instead.
//
// The value is two indexes into process-wide tables of prefab keys and part
// ids: 8 bytes, no pointers, meaningless outside the running process. The
// zero value is no prefab.
type PrefabInstance struct {
	idx  uint32 // index into prefabKeys + 1; 0 means none
	part uint32 // index into prefabParts + 1; 0 means the root
}

// prefabInstanceID is PrefabInstance's component ID.
var prefabInstanceID = ecs.RegisterComp[PrefabInstance]()

// interner numbers strings for the life of the process. It only grows, so a
// number stays valid, like a registered class.
type interner struct {
	sync.RWMutex
	names []string
	index map[string]uint32
}

// id returns the number of name, starting at 1, interning it on first use.
func (t *interner) id(name string) uint32 {
	t.RLock()
	i, ok := t.index[name]
	t.RUnlock()
	if ok {
		return i
	}
	t.Lock()
	defer t.Unlock()
	if i, ok := t.index[name]; ok {
		return i
	}
	t.names = append(t.names, name)
	i = uint32(len(t.names))
	t.index[name] = i
	return i
}

// name returns the string numbered id (not 0).
func (t *interner) name(id uint32) string {
	t.RLock()
	defer t.RUnlock()
	return t.names[id-1]
}

var (
	prefabKeys  = interner{index: map[string]uint32{}}
	prefabParts = interner{index: map[string]uint32{}}
)

// prefabTag returns the PrefabInstance of the root of key.
func prefabTag(key string) PrefabInstance { return prefabPartTag(key, "") }

// prefabPartTag returns the PrefabInstance of the object part of key; the
// root is part "".
func prefabPartTag(key, part string) PrefabInstance {
	tag := PrefabInstance{idx: prefabKeys.id(key)}
	if part != "" {
		tag.part = prefabParts.id(part)
	}
	return tag
}

// PrefabTag returns the PrefabInstance component that marks an entity as
// the root of an instance of the prefab key, for code that creates
// instances without [Deserialize] or [BoundPrefab.Spawn] (the level
// editor). Keys are interned for the life of the process.
func PrefabTag(key string) PrefabInstance { return prefabTag(key) }

// PrefabPartTag is [PrefabTag] for an object other than the root: part is
// its id within the prefab ("sword", "horse/saddle"). An empty part is the
// root.
func PrefabPartTag(key, part string) PrefabInstance { return prefabPartTag(key, part) }

// Key returns the prefab key p stands for, or "" for the zero value.
func (p PrefabInstance) Key() string {
	if p.idx == 0 {
		return ""
	}
	return prefabKeys.name(p.idx)
}

// Part returns the id of the object within the prefab that p stands for,
// or "" for the root.
func (p PrefabInstance) Part() string {
	if p.part == 0 {
		return ""
	}
	return prefabParts.name(p.part)
}

// PrefabOf returns the key of the prefab e was made from, and false if e
// has no [PrefabInstance] tag. A part of a multi-object instance reports
// its instance's prefab.
//
// Panics with "ecs: invalid or stale entity: <e>" if e is stale.
func PrefabOf(world *ecs.World, e ecs.Entity) (string, bool) {
	key, _, ok := PrefabPartOf(world, e)
	return key, ok
}

// PrefabPartOf is [PrefabOf] that also returns which object of the prefab e
// is: "" for the root, else the part's id (`sword`, `horse/saddle`).
//
// Panics with "ecs: invalid or stale entity: <e>" if e is stale.
func PrefabPartOf(world *ecs.World, e ecs.Entity) (key, part string, ok bool) {
	tag, ok := world.GetCompOrNil(e, prefabInstanceID).(*PrefabInstance)
	if !ok || tag.idx == 0 {
		return "", "", false
	}
	return tag.Key(), tag.Part(), true
}
