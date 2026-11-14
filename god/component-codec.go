package god

import (
	"encoding/json"
	"fmt"

	"github.com/weakpixel/ecs"
)

// Refs maps between the entities of a World and the object ids of a
// document, for component fields of type ecs.Entity (schema format
// "entity"). [DecodeComponent] calls RefEntity for every non-null id it
// reads, [EncodeComponent] calls RefID for every non-zero Entity it writes.
type Refs interface {
	// RefID returns the document id of e, for encoding.
	RefID(e ecs.Entity) (string, error)
	// RefEntity returns the entity of document id, for decoding.
	RefEntity(id string) (ecs.Entity, error)
}

// ObjectRefs returns the Refs of a set of objects: an object's id is its
// [Object.ID]. RefEntity fails for an id no object has, RefID for an entity
// that is not among objs or whose object has no id. The objects are read
// when ObjectRefs is called.
func ObjectRefs(objs []Object) Refs {
	r := objectRefs{ids: make(map[ecs.Entity]string, len(objs)), entities: make(map[string]ecs.Entity, len(objs))}
	for _, o := range objs {
		r.ids[o.Entity] = o.ID
		if o.ID != "" {
			r.entities[o.ID] = o.Entity
		}
	}
	return r
}

type objectRefs struct {
	ids      map[ecs.Entity]string
	entities map[string]ecs.Entity
}

func (r objectRefs) RefID(e ecs.Entity) (string, error) {
	id, ok := r.ids[e]
	switch {
	case !ok:
		return "", fmt.Errorf("entity %v is not one of the objects", e)
	case id == "":
		return "", fmt.Errorf("entity %v has no object id", e)
	}
	return id, nil
}

func (r objectRefs) RefEntity(id string) (ecs.Entity, error) {
	e, ok := r.entities[id]
	if !ok {
		return ecs.Entity{}, fmt.Errorf("unknown object id %q", id)
	}
	return e, nil
}

// DecodeComponent turns the JSON of one component into a value ready for
// World.SetComp or NewEntity: it validates raw against c.Schema, resolves
// entity references through refs, decodes it with c.Decode, and resolves
// asset keys through the world's AssetResolver. A nil refs is fine for
// components without references; a non-null reference is then an error
// ("no Refs for entity reference"). Errors carry $-paths and are all
// reported. Deserialize and Lua use it; so can tools that edit one
// component at a time.
func DecodeComponent(world *ecs.World, c Component, raw json.RawMessage, refs Refs) (any, error) {
	return decodeComponent(worldAssetResolver(world), c, raw, refs)
}

// decodeComponent is DecodeComponent with an explicit AssetResolver (nil
// fails on any non-empty asset key).
func decodeComponent(assets AssetResolver, c Component, raw json.RawMessage, refs Refs) (any, error) {
	if err := c.Schema.Validate(raw); err != nil {
		return nil, err
	}
	data := []byte(raw)
	if hasEntityRef(c.Schema) {
		var err error
		if data, err = refsToEntities(c.Schema, data, refs); err != nil {
			return nil, err
		}
	}
	v, err := c.Decode(data)
	if err != nil {
		return nil, err
	}
	// Asset properties are json:"-" in Go, so their keys are read from the
	// original JSON.
	return decodeAssets(assets, c.Schema, raw, v)
}

// EncodeComponent is the inverse of [DecodeComponent]: the JSON of component
// value v (a T or *T of c's type), with asset handles written as keys and
// entity references as ids from refs. A nil refs fails on any non-zero
// reference. Runtime fields are left out like in [Serialize].
func EncodeComponent(world *ecs.World, c Component, v any, refs Refs) (json.RawMessage, error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	if data, err = encodeAssets(world, c.Schema, v, data); err != nil {
		return nil, err
	}
	if hasEntityRef(c.Schema) {
		if data, err = entitiesToRefs(c.Schema, data, refs); err != nil {
			return nil, err
		}
	}
	return data, nil
}
