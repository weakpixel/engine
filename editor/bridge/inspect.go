package bridge

import (
	"encoding/json"
	"fmt"
	"slices"

	"github.com/weakpixel/ecs"
	"github.com/weakpixel/engine/god"
)

// defaultInspectLimit is how many entities "inspect" returns without a
// limit.
const defaultInspectLimit = 100

// entitiesMsg answers "inspect": the entities asked for with their live
// component values. Truncated is set when the limit cut the list; Missing
// lists asked ids that name no object of the level.
type entitiesMsg struct {
	Type      string       `json:"type"` // "entities"
	Entities  []entityInfo `json:"entities"`
	Truncated bool         `json:"truncated"`
	Missing   []string     `json:"missing,omitempty"`
}

// entityInfo is one entity of an "entities" answer. Entity is the entity as
// "<index>.<generation>"; ID is its document id, "" for an entity that is
// not an object of the level (one a system spawned). Components holds the
// value of every registered component type the entity has, encoded as a
// document holds it; Errors the components that could not be encoded.
type entityInfo struct {
	Entity     string                     `json:"entity"`
	ID         string                     `json:"id,omitempty"`
	Components map[string]json.RawMessage `json:"components"`
	Errors     map[string]string          `json:"errors,omitempty"`
}

// entityName is how "entities" names an entity.
func entityName(e ecs.Entity) string { return fmt.Sprintf("%d.%d", e.ID, e.Generation) }

// inspectRefs writes entity references as the document id of a level
// object, and as "#<index>.<generation>" for any other entity, so a live
// value that points at a spawned entity still encodes. It never decodes.
type inspectRefs map[ecs.Entity]string

func (r inspectRefs) RefID(e ecs.Entity) (string, error) {
	if id, ok := r[e]; ok {
		return id, nil
	}
	return "#" + entityName(e), nil
}

func (r inspectRefs) RefEntity(id string) (ecs.Entity, error) {
	return ecs.Entity{}, fmt.Errorf("bridge: inspect does not decode references")
}

// inspect answers an "inspect" message with "entities": the level's objects
// (those in m.IDs, else all in document order), then with m.All every other
// live entity in index order, at most m.Limit, each with its registered
// components (only m.Components when given). Components the types registry
// does not know are not listed: the ecs API has no way to name them.
func (b *Bridge) inspect(m inbound) {
	limit := m.Limit
	if limit <= 0 {
		limit = defaultInspectLimit
	}
	out := entitiesMsg{Type: "entities", Entities: []entityInfo{}}
	if b.Level() == nil {
		b.send(out)
		return
	}
	w := b.Engine().World()
	refs := inspectRefs{}
	for _, id := range b.IDs() {
		if en, ok := b.Entity(id); ok {
			refs[en] = id
		}
	}
	var list []ecs.Entity
	ids := m.IDs
	if len(ids) == 0 {
		ids = b.IDs()
	}
	for _, id := range ids {
		en, ok := b.Entity(id)
		if !ok || !w.IsAlive(en) {
			out.Missing = append(out.Missing, id)
			continue
		}
		list = append(list, en)
	}
	if m.All {
		var rest []ecs.Entity
		for en := range w.Query().Each() {
			if _, ok := refs[en]; !ok {
				rest = append(rest, en)
			}
		}
		slices.SortFunc(rest, func(a, b ecs.Entity) int { return int(a.ID) - int(b.ID) })
		list = append(list, rest...)
	}
	if len(list) > limit {
		list, out.Truncated = list[:limit], true
	}
	comps := inspectComponents(m.Components)
	for _, en := range list {
		out.Entities = append(out.Entities, encodeEntity(w, en, refs, comps))
	}
	b.send(out)
}

// inspectComponents is the registered component types to report: those
// with the keys given, or all.
func inspectComponents(keys []string) []god.ComponentSchema {
	var out []god.ComponentSchema
	for c := range god.Components() {
		if len(keys) == 0 || slices.Contains(keys, c.Key()) {
			out = append(out, c)
		}
	}
	return out
}

// encodeEntity is the entityInfo of en with the components of comps it has.
func encodeEntity(w *ecs.World, en ecs.Entity, refs inspectRefs, comps []god.ComponentSchema) entityInfo {
	info := entityInfo{Entity: entityName(en), ID: refs[en], Components: map[string]json.RawMessage{}}
	for _, cs := range comps {
		if !w.HasComp(en, cs.RuntimeID) {
			continue
		}
		c := god.Component{Group: cs.Group, Name: cs.Name, RuntimeID: cs.RuntimeID, Schema: cs.Schema}
		data, err := god.EncodeComponent(w, c, w.GetComp(en, cs.RuntimeID), refs)
		if err != nil {
			if info.Errors == nil {
				info.Errors = map[string]string{}
			}
			info.Errors[cs.Key()] = err.Error()
			continue
		}
		info.Components[cs.Key()] = data
	}
	return info
}
