package editor

import (
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"slices"
	"strings"

	"github.com/weakpixel/ecs"
	"github.com/weakpixel/engine/god"
	"github.com/weakpixel/engine/resolver"
)

// idRefs resolves entity references of a document being applied: ids name
// the entities the objects will have once it is applied.
type idRefs map[string]ecs.Entity

func (r idRefs) RefEntity(id string) (ecs.Entity, error) {
	if e, ok := r[id]; ok {
		return e, nil
	}
	return ecs.Entity{}, fmt.Errorf("no object with id %q", id)
}

func (r idRefs) RefID(e ecs.Entity) (string, error) {
	for id, en := range r {
		if en == e {
			return id, nil
		}
	}
	return "", fmt.Errorf("entity %v is not an object of the level", e)
}

// planned is the new state of one object of the document being applied.
type planned struct {
	doc    docObject
	schema god.Schema
	entity ecs.Entity
	create bool // a new entity (new id, or the class changed)
	set    []any
	remove []ecs.ComponentID
	comps  map[string]string
}

// apply makes the level match text (see Session.Apply). With force, every
// component is set even when its JSON did not change (Stop uses it to undo
// what Play changed).
func (s *Session) apply(text []byte, force bool) error {
	docs, _, err := parseDocument(text)
	if err != nil {
		return err
	}
	docs, err = s.expand(docs)
	if err != nil {
		return err
	}
	w := s.e.World()

	// Which objects need a new entity (a new id, or another class or
	// prefab), and which old ones go away. Created
	// entities exist before decoding, so references to them resolve; they
	// are deleted again if the document turns out to be invalid.
	plan := make([]*planned, len(docs))
	refs := make(idRefs, len(docs))
	byID := make(map[string]*planned, len(docs))
	changedSet := false
	for i, d := range docs {
		schema, _ := god.LookupSchema(d.Group, d.Class) // parseDocument checked it
		p := &planned{doc: d, schema: schema, comps: map[string]string{}}
		old := s.objs[d.ID]
		if old == nil || old.schema.SchemaMeta != schema.SchemaMeta || old.prefab != d.Prefab || old.part != d.Part {
			tag, _ := god.ClassTag(d.Group, d.Class)
			if d.Prefab != "" {
				p.entity = w.NewEntity(tag, god.PrefabPartTag(d.Prefab, d.Part))
			} else {
				p.entity = w.NewEntity(tag)
			}
			p.create = true
			changedSet = true
		} else {
			p.entity = old.entity
		}
		plan[i] = p
		refs[d.ID] = p.entity
		byID[d.ID] = p
	}
	for _, id := range s.order {
		if byID[id] == nil {
			changedSet = true
		}
	}

	// Decode every component that changed, in the level's asset scope.
	if r := levelResolver(w); r != nil && s.level.Scope() != nil {
		prev := r.Scope()
		r.SetScope(s.level.Scope())
		defer r.SetScope(prev)
	}
	var errs []error
	for i, p := range plan {
		old := s.objs[p.doc.ID]
		// The class's components and any other registered ones the
		// document or the old state has, so a removed extra component is
		// removed from the entity too.
		keys := slices.Collect(maps.Keys(p.doc.Components))
		if !p.create {
			keys = slices.AppendSeq(keys, maps.Keys(old.comps))
		}
		for _, c := range god.ObjectComponents(p.schema, slices.Values(keys)) {
			raw, present := p.doc.Components[c.Key()]
			if !present {
				if !p.create && w.HasComp(p.entity, c.RuntimeID) {
					p.remove = append(p.remove, c.RuntimeID)
				}
				continue
			}
			compact := compactJSON(raw)
			p.comps[c.Key()] = compact
			if !p.create && !force && old.comps[c.Key()] == compact && !(changedSet && hasEntityField(c.Schema)) {
				continue
			}
			v, err := god.DecodeComponent(w, c, raw, refs)
			if err != nil {
				errs = append(errs, fmt.Errorf("objects[%d] (%s): %s: %w", i, p.doc.ID, c.Key(), err))
				continue
			}
			p.set = append(p.set, v)
		}
	}
	if err := errors.Join(errs...); err != nil {
		for _, p := range plan {
			if p.create {
				w.DeleteEntity(p.entity)
			}
		}
		return err
	}

	// Valid: delete what is gone or replaced, then write the new state.
	for _, id := range s.order {
		old := s.objs[id]
		if old == nil {
			continue // already gone (Stop drops objects the game deleted)
		}
		if p := byID[id]; p == nil || p.create {
			if w.IsAlive(old.entity) {
				w.DeleteEntity(old.entity)
			}
		}
	}
	objs := make(map[string]*object, len(plan))
	order := make([]string, len(plan))
	levelObjs := make([]god.Object, len(plan))
	for i, p := range plan {
		if len(p.remove) > 0 {
			w.RemoveComp(p.entity, p.remove...)
		}
		if len(p.set) > 0 {
			w.SetComp(p.entity, p.set...)
		}
		objs[p.doc.ID] = &object{id: p.doc.ID, prefab: p.doc.Prefab, part: p.doc.Part, root: rootID(p.doc), schema: p.schema, entity: p.entity, comps: p.comps}
		order[i] = p.doc.ID
		levelObjs[i] = god.Object{SchemaMeta: p.schema.SchemaMeta, ID: p.doc.ID, Prefab: p.doc.Prefab, Part: p.doc.Part, Entity: p.entity}
	}
	s.objs, s.order, s.doc = objs, order, slices.Clone(text)
	s.level.SetObjects(levelObjs)

	// Selected objects that are gone drop out of the selection.
	sel := slices.DeleteFunc(slices.Clone(s.selection), func(id string) bool { return objs[id] == nil })
	s.setSelection(sel...)
	return nil
}

// rootID is the id of the instance root of d: its own id, or its id
// without the `/<part>` suffix for a part.
func rootID(d docObject) string {
	if d.Part == "" {
		return d.ID
	}
	return strings.TrimSuffix(d.ID, "/"+d.Part)
}

// encode returns the JSON of the component with key ("group:name") of the
// object with id, as the document holds it.
func (s *Session) encode(id, key string) (json.RawMessage, error) {
	o := s.objs[id]
	refs := make(idRefs, len(s.objs))
	for oid, ob := range s.objs {
		refs[oid] = ob.entity
	}
	w := s.e.World()
	if c, ok := god.ComponentOf(o.schema, key); ok && w.HasComp(o.entity, c.RuntimeID) {
		return god.EncodeComponent(w, c, w.GetComp(o.entity, c.RuntimeID), refs)
	}
	return nil, fmt.Errorf("editor: object %s has no component %s", id, key)
}

// levelResolver returns the engine's asset resolver (game.Engine installs a
// *resolver.Resolver), or nil.
func levelResolver(w *ecs.World) *resolver.Resolver {
	rp := ecs.GetResourceOrNil[god.AssetResolver](w)
	if rp == nil {
		return nil
	}
	r, _ := (*rp).(*resolver.Resolver)
	return r
}
