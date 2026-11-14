package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"path"
	"slices"
	"strconv"
	"strings"

	"github.com/weakpixel/engine/asset/library"
	"github.com/weakpixel/engine/god"
	"github.com/weakpixel/engine/internal/jsonedit"
	"github.com/weakpixel/engine/tools/internal/mcp"
)

// transformKey is the component that holds an object's position
// (editor.TransformKey).
const transformKey = "gfx:Transform"

// layerKeys are the components that have a render Layer, as in the editor's
// bridge.
var layerKeys = []string{"gfx:Sprite", "gfx:NineSlice", "gfx:Label", "gfx:Particles"}

// freshID returns "<base>-<n>" (base lower-cased) with the smallest n >= 1
// that no id in used has, as the editor's bridge does for a new object.
func freshID(base string, used map[string]bool) string {
	base = strings.ToLower(base)
	for n := 1; ; n++ {
		if id := fmt.Sprintf("%s-%d", base, n); !used[id] {
			return id
		}
	}
}

// docIDs is the set of ids of the document's objects.
func docIDs(doc *document) map[string]bool {
	used := map[string]bool{}
	for _, o := range doc.Objects {
		if o.ID != "" {
			used[o.ID] = true
		}
	}
	return used
}

// number is the JSON form of a coordinate.
func number(f float64) json.Number { return json.Number(strconv.FormatFloat(f, 'g', -1, 64)) }

// point is a position as the document writes it.
func point(x, y float64) map[string]any { return map[string]any{"X": number(x), "Y": number(y)} }

// newClassObject builds the document JSON of a new object of the GOD type s:
// every required component at its initial value, the Transform at (x, y)
// and, when layer is set, that Layer in each drawn component. It matches
// what the editor's bridge writes for a created object.
func (t *tools) newClassObject(g *godData, s *god.Schema, id string, x, y float64, layer *int) (json.RawMessage, error) {
	comps := map[string]json.RawMessage{}
	for _, c := range s.Components {
		if c.Optional {
			continue
		}
		comp := g.compByKey[c.Key()]
		if comp == nil {
			return nil, fmt.Errorf("component %s of %s is not in the GOD", c.Key(), s.Key())
		}
		raw := initialValue(comp.Schema)
		if c.Key() == transformKey || (layer != nil && slices.Contains(layerKeys, c.Key())) {
			v, err := decodeJSON(raw)
			if err != nil {
				return nil, err
			}
			if c.Key() == transformKey {
				v, err = setIn(v, []any{"Pos"}, point(x, y))
				if err == nil {
					v, err = setIn(v, []any{"PrevPos"}, point(x, y))
				}
			} else {
				v, err = setIn(v, []any{"Layer"}, json.Number(strconv.Itoa(*layer)))
			}
			if err != nil {
				return nil, fmt.Errorf("%s: %w", c.Key(), err)
			}
			if raw, err = marshalJSON(v); err != nil {
				return nil, err
			}
		}
		comps[c.Key()] = raw
	}
	return marshalJSON(struct {
		ID         string                     `json:"id"`
		Group      string                     `json:"group"`
		Class      string                     `json:"class"`
		Components map[string]json.RawMessage `json:"components"`
	}{id, s.Group, s.Class, comps})
}

// newInstance builds the document JSON of an instance of the prefab key at
// (x, y): it overrides only what differs from the prefab, the Transform
// position and an explicit Layer.
func (t *tools) newInstance(key, id string, x, y float64, layer *int, chain []string) (json.RawMessage, error) {
	exp, err := t.expandPrefab(key, chain)
	if err != nil {
		return nil, err
	}
	patches := map[string]any{}
	if _, ok := exp.components[transformKey]; ok {
		patches[transformKey] = map[string]any{"Pos": point(x, y), "PrevPos": point(x, y)}
	}
	if layer != nil {
		for _, k := range layerKeys {
			if _, ok := exp.components[k]; ok {
				patches[k] = map[string]any{"Layer": json.Number(strconv.Itoa(*layer))}
			}
		}
	}
	comps := map[string]json.RawMessage{}
	for _, k := range slices.Sorted(maps.Keys(patches)) {
		base, err := decodeJSON(exp.components[k])
		if err != nil {
			return nil, fmt.Errorf("%s: %w", k, err)
		}
		if diff, differs := diffPatch(base, patches[k]); differs {
			if comps[k], err = marshalJSON(diff); err != nil {
				return nil, err
			}
		}
	}
	return marshalJSON(struct {
		ID         string                     `json:"id"`
		Prefab     string                     `json:"prefab"`
		Components map[string]json.RawMessage `json:"components"`
	}{id, key, comps})
}

// diffPatch returns the part of patch that differs from base, and whether
// there is any.
func diffPatch(base, patch any) (any, bool) {
	pm, ok := patch.(map[string]any)
	bm, ok2 := base.(map[string]any)
	if !ok || !ok2 {
		return patch, !sameJSON(base, patch)
	}
	out := map[string]any{}
	for k, pv := range pm {
		if bv, has := bm[k]; has {
			if d, differs := diffPatch(bv, pv); differs {
				out[k] = d
			}
		} else {
			out[k] = pv
		}
	}
	return out, len(out) > 0
}

// sameJSON compares two decoded JSON values, numbers by value.
func sameJSON(a, b any) bool {
	an, aok := a.(json.Number)
	bn, bok := b.(json.Number)
	if aok && bok {
		x, err1 := an.Float64()
		y, err2 := bn.Float64()
		return err1 == nil && err2 == nil && x == y
	}
	ab, err1 := marshalJSON(a)
	bb, err2 := marshalJSON(b)
	return err1 == nil && err2 == nil && string(ab) == string(bb)
}

type placeArgs struct {
	Path   string   `json:"path"`
	Class  string   `json:"class"`
	Prefab string   `json:"prefab"`
	X      *float64 `json:"x"`
	Y      *float64 `json:"y"`
	ID     string   `json:"id"`
	Layer  *int     `json:"layer"`
	Expect string   `json:"expect"`
}

func (t *tools) placeObject(_ context.Context, a placeArgs) (any, error) {
	if (a.Class == "") == (a.Prefab == "") {
		return nil, errors.New(`give "class" (an object type such as demo:Crate) or "prefab" (a *.prefab.json key), not both`)
	}
	if a.X == nil || a.Y == nil {
		return nil, errors.New(`"x" and "y" are required`)
	}
	g, err := t.god()
	if err != nil {
		return nil, err
	}
	var s *god.Schema
	if a.Class != "" {
		if s = g.objByKey[a.Class]; s == nil {
			var keys []string
			for _, o := range g.objects {
				keys = append(keys, o.Key())
			}
			return nil, unknownKey("object type", a.Class, keys)
		}
	}
	var newID string
	out, err := t.editDocument(a.Path, a.Expect, func(text []byte, doc *document, info docInfo) ([]byte, error) {
		used := docIDs(doc)
		id := a.ID
		switch {
		case id != "" && used[id]:
			return nil, fmt.Errorf("id %q is already used in the document", id)
		case id == "" && s != nil:
			id = freshID(s.Class, used)
		case id == "":
			id = freshID(strings.TrimSuffix(path.Base(a.Prefab), ".prefab.json"), used)
		}
		newID = id
		var obj json.RawMessage
		var err error
		if s != nil {
			obj, err = t.newClassObject(g, s, id, *a.X, *a.Y, a.Layer)
		} else {
			if library.KindOf(a.Prefab) != library.KindPrefab {
				return nil, fmt.Errorf("prefab %q is not a *.prefab.json key", a.Prefab)
			}
			obj, err = t.newInstance(a.Prefab, id, *a.X, *a.Y, a.Layer, info.chain())
		}
		if err != nil {
			return nil, err
		}
		return jsonedit.Insert(text, obj, -1, "objects")
	})
	if err != nil {
		return nil, err
	}
	out["id"] = newID
	return out, nil
}

type removeObjectArgs struct{ objArgs }

func (t *tools) removeObject(_ context.Context, a removeObjectArgs) (any, error) {
	g, err := t.god()
	if err != nil {
		return nil, err
	}
	return t.editDocument(a.Path, a.Expect, func(text []byte, doc *document, info docInfo) ([]byte, error) {
		i, obj, err := locate(doc, a.objArgs)
		if err != nil {
			return nil, err
		}
		if obj.ID != "" {
			if refs := referencesTo(g, doc, i); len(refs) > 0 {
				return nil, fmt.Errorf("object %s is referenced by %s; change those references first", obj.label(i), strings.Join(refs, ", "))
			}
		}
		return jsonedit.Delete(text, "objects", i)
	})
}

// referencesTo lists where other objects of doc refer to the id of object i,
// as `object "x" component c $.Field`.
func referencesTo(g *godData, doc *document, i int) []string {
	id := doc.Objects[i].ID
	var refs []string
	for j, o := range doc.Objects {
		if j == i {
			continue
		}
		for _, k := range slices.Sorted(maps.Keys(o.Components)) {
			c := g.compByKey[k]
			if c == nil || c.Schema == nil {
				continue
			}
			v, err := decodeJSON(o.Components[k])
			if err != nil {
				continue
			}
			walkSchema(c.Schema, "$", v, func(p string, s *god.TypeSchema, v any) {
				if str, _ := v.(string); s.Format == god.FormatEntity && str == id {
					refs = append(refs, fmt.Sprintf("object %s component %s %s", o.label(j), k, p))
				}
			})
		}
	}
	return refs
}

func (t *tools) objectEditTools() []*mcp.Tool {
	return []*mcp.Tool{
		tool("place_object", "Place an object",
			"Adds an object at the end of the document's objects: an object type (class, group:Class) with every required component at "+
				"its initial value and gfx:Transform at (x, y), or an instance of a prefab that overrides only the position. The id is "+
				"<class or prefab name>-<n> unless given. layer, when given, is written into the drawn components (the editor's default "+
				"layer from a Tiled backdrop is not applied here). Writes the document's file.",
			mcp.Object(map[string]*mcp.Schema{
				"path":   mcp.String("asset key of the document"),
				"class":  mcp.String("object type key, e.g. demo:Crate (list_object_types)"),
				"prefab": mcp.String("asset key of a prefab to instantiate (list_assets kind prefab)"),
				"x":      mcp.Number("world x of the object"),
				"y":      mcp.Number("world y of the object"),
				"id":     mcp.String("id for the new object; default <name>-<n>"),
				"layer":  mcp.Integer("render layer for the drawn components", -32768, 32767),
				"expect": mcp.String("sha from read_document; the edit is refused if the file changed since"),
			}, "path", "x", "y"),
			editable, t.placeObject),
		tool("remove_object", "Remove an object",
			"Deletes an object from the document's file. Refused, naming the referrers, while another object refers to its id.",
			mcp.Object(objProps(nil), "path"),
			destructive, t.removeObject),
	}
}

// initialValue is the JSON the editor writes for a new component: the
// schema's initial value without the optional properties that are at their
// zero value (a Go field with omitempty or omitzero is not written by the
// game's encoder, so the editor's objects do not have them either; a
// required property is always kept).
func initialValue(s *god.TypeSchema) json.RawMessage {
	raw := s.InitialValue()
	if s == nil {
		return raw
	}
	v, err := decodeJSON(raw)
	if err != nil {
		return raw
	}
	out, err := marshalJSON(dropZero(s, v))
	if err != nil {
		return raw
	}
	return out
}

// dropZero removes the optional properties of v that are zero, recursively.
func dropZero(s *god.TypeSchema, v any) any {
	m, ok := v.(map[string]any)
	if !ok || s == nil {
		return v
	}
	for k, sub := range m {
		p := s.Properties[k]
		if p != nil {
			m[k] = dropZero(p, sub)
		}
		if !slices.Contains(s.Required, k) && isZero(m[k]) {
			delete(m, k)
		}
	}
	return m
}

// isZero reports whether v is a zero value: null, "", 0, false, or a
// container whose members are all zero.
func isZero(v any) bool {
	switch x := v.(type) {
	case nil:
		return true
	case string:
		return x == ""
	case bool:
		return !x
	case json.Number:
		f, err := x.Float64()
		return err == nil && f == 0
	case []any:
		return !slices.ContainsFunc(x, func(e any) bool { return !isZero(e) })
	case map[string]any:
		for _, e := range x {
			if !isZero(e) {
				return false
			}
		}
		return true
	}
	return false
}
