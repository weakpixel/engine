package bridge

import (
	"fmt"
	"strings"

	"github.com/weakpixel/engine/god"
)

// create answers "create" with a new object of the "group:class" m.Class:
// its required components at their zero values and its Transform at the
// center of the view; or, with m.Prefab, an instance of that prefab there.
func (b *Bridge) create(m inbound) {
	if m.Prefab != "" {
		b.createInstance(m.Prefab, b.Engine().Camera().Pos)
		return
	}
	if b.Level() == nil {
		b.sendError(fmt.Errorf("create %s: no level is open", m.Class))
		return
	}
	group, class, _ := strings.Cut(m.Class, ":")
	s, ok := god.LookupSchema(group, class)
	if !ok {
		b.sendError(fmt.Errorf("create: unknown object type %q", m.Class))
		return
	}
	obj, err := b.newObject(candidate{schema: s}, "", b.Engine().Camera().Pos)
	if err != nil {
		b.sendError(fmt.Errorf("create %s: %w", m.Class, err))
		return
	}
	b.send(createMsg{Type: "create", Object: obj})
}

// assetField answers "assetField" (a file dropped on an inspector field)
// with the key to write into the field: the file's kind must be the
// field's asset kind, and an animation definition becomes its default
// clip, as for drops on the canvas.
func (b *Bridge) assetField(m inbound) {
	key, err := b.fieldKey(m)
	if err != nil {
		b.sendError(fmt.Errorf("drop %s on %s: %w", m.Path, strings.Join(m.Field, "."), err))
		return
	}
	b.send(fieldMsg{Type: "field", ID: m.ID, Field: m.Field, Value: key})
}

func (b *Bridge) fieldKey(m inbound) (string, error) {
	if b.Level() == nil {
		return "", fmt.Errorf("no level is open")
	}
	field, err := b.fieldSchema(m.ID, m.Field)
	if err != nil {
		return "", err
	}
	if field.Format != formatAsset {
		return "", fmt.Errorf("not an asset field")
	}
	if kind := kindOf(m.Path); kind != field.XAsset {
		if kind == "" {
			kind = "unknown"
		}
		return "", fmt.Errorf("the field takes a %s, the file is a %s", field.XAsset, kind)
	}
	return b.assetKey(field.XAsset, m.Path)
}

// fieldSchema is the TypeSchema at path (component key, then property
// names) of object id.
func (b *Bridge) fieldSchema(id string, path []string) (*god.TypeSchema, error) {
	if len(path) == 0 {
		return nil, fmt.Errorf("no field")
	}
	for _, o := range b.Level().Objects() {
		if o.ID != id {
			continue
		}
		s, ok := god.LookupSchema(o.Group, o.Class)
		if !ok {
			return nil, fmt.Errorf("unknown object type %s:%s", o.Group, o.Class)
		}
		if c, ok := god.ComponentOf(s, path[0]); ok {
			ts := c.Schema
			for _, p := range path[1:] {
				if ts == nil || ts.Properties[p] == nil {
					return nil, fmt.Errorf("%s has no field %s", c.Key(), strings.Join(path[1:], "."))
				}
				ts = ts.Properties[p]
			}
			return ts, nil
		}
		return nil, fmt.Errorf("%s is not a registered component", path[0])
	}
	return nil, fmt.Errorf("no object with id %q", id)
}
