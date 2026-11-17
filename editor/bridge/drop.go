package bridge

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"math"
	"slices"
	"strconv"
	"strings"

	"github.com/weakpixel/ecs"
	"github.com/weakpixel/engine/anim"
	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/god"
)

// formatAsset is the TypeSchema format of asset key fields.
const formatAsset = "asset"

// candidate is an object type a dropped asset can create: the component and
// the JSON path of the asset field the file goes into.
type candidate struct {
	schema    god.Schema
	component string
	field     []string
}

// key is "group:class".
func (c candidate) key() string { return c.schema.Key() }

// candidates returns the registered object types with an asset field of
// kind, in Schemas order (sorted by key). Only required components count:
// the created object gets exactly those.
func candidates(kind string) []candidate {
	var out []candidate
	for s := range god.Schemas() {
		for _, c := range s.Components {
			if c.Optional {
				continue
			}
			if field, ok := assetField(c.Schema, kind); ok {
				out = append(out, candidate{schema: s, component: c.Key(), field: field})
				break
			}
		}
	}
	return out
}

// assetField finds the first property (depth first, sorted names, but a
// property named ID before the others) of s with format "asset" and x-asset
// kind, and returns its path. ID first because it is the main asset of a
// component (gfx:Sprite's image), and other fields of the same kind
// (Sprite.Aux, a shader's second image) sort before it.
func assetField(s *god.TypeSchema, kind string) ([]string, bool) {
	if s == nil {
		return nil, false
	}
	if s.Format == formatAsset && s.XAsset == kind {
		return []string{}, true
	}
	names := make([]string, 0, len(s.Properties))
	for n := range s.Properties {
		names = append(names, n)
	}
	slices.SortFunc(names, func(a, b string) int {
		switch {
		case a == b:
			return 0
		case a == "ID":
			return -1
		case b == "ID":
			return 1
		}
		return strings.Compare(a, b)
	})
	for _, n := range names {
		if p, ok := assetField(s.Properties[n], kind); ok {
			return append([]string{n}, p...), true
		}
	}
	return nil, false
}

// drop creates an object for a file dropped on the canvas: it finds the
// object types that can hold the file, asks the host to choose when there
// are several, and sends "create" with the new object. A dropped prefab
// becomes an instance of it.
func (b *Bridge) drop(m inbound) {
	if b.Level() == nil {
		b.sendError(fmt.Errorf("drop %s: no level is open", m.Path))
		return
	}
	kind := kindOf(m.Path)
	if kind == "prefab" {
		b.createInstance(m.Path, b.WindowToWorld(image.Pt(int(math.Round(m.X)), int(math.Round(m.Y)))))
		return
	}
	if kind == "" {
		b.sendError(fmt.Errorf("drop %s: no object type takes this kind of file", m.Path))
		return
	}
	cands := candidates(kind)
	if m.Class != "" {
		cands = filterKey(cands, m.Class)
	}
	switch len(cands) {
	case 0:
		b.sendError(fmt.Errorf("drop %s: no object type has a %s field", m.Path, kind))
		return
	case 1:
	default:
		opts := make([]string, len(cands))
		for i, c := range cands {
			opts[i] = c.key()
		}
		b.send(chooseMsg{Type: "choose", Path: m.Path, X: m.X, Y: m.Y, Options: opts})
		return
	}
	key, err := b.assetKey(kind, m.Path)
	if err != nil {
		b.sendError(fmt.Errorf("drop %s: %w", m.Path, err))
		return
	}
	at := b.WindowToWorld(image.Pt(int(math.Round(m.X)), int(math.Round(m.Y))))
	obj, err := b.newObject(cands[0], key, at)
	if err != nil {
		b.sendError(fmt.Errorf("drop %s: %w", m.Path, err))
		return
	}
	b.send(createMsg{Type: "create", Object: obj})
}

func filterKey(cs []candidate, key string) []candidate {
	var out []candidate
	for _, c := range cs {
		if c.key() == key {
			out = append(out, c)
		}
	}
	return out
}

// assetKey is the key a dropped file goes into a field of kind with: the
// path itself, except for an animation definition, whose field takes one
// clip: its default clip (or the first one).
func (b *Bridge) assetKey(kind, p string) (string, error) {
	if kind != "clip" {
		return p, nil
	}
	h, err := anim.LoadClipSetSync(b.Level().Scope(), p)
	if err != nil {
		return "", err
	}
	set := b.Engine().Stores().ClipSets().Get(h)
	name := set.Default
	if name == "" && len(set.Names) > 0 {
		name = set.Names[0]
	}
	if name == "" {
		return "", fmt.Errorf("no clips")
	}
	return p + "#" + name, nil
}

// newObject builds the document JSON of a new object of c: every required
// component at its initial value (its declared defaults, else zero), a
// gfx.Transform at pos, key in the asset
// field and the map's default layer (see editor.Session.DefaultLayer) in
// each drawn component. Its id is "<class>-<n>", unused in the level.
func (b *Bridge) newObject(c candidate, key string, pos gfx.Vec2) (json.RawMessage, error) {
	w := b.Engine().World()
	transformID := ecs.RegisterComp[gfx.Transform]()
	comps := map[string]json.RawMessage{}
	for _, comp := range c.schema.Components {
		if comp.Optional {
			continue
		}
		v, err := comp.Decode(comp.Schema.InitialValue())
		if err != nil {
			return nil, fmt.Errorf("%s: %w", comp.Key(), err)
		}
		if comp.RuntimeID == transformID {
			t := v.(gfx.Transform)
			t.Pos, t.PrevPos = pos, pos
			v = t
		}
		raw, err := god.EncodeComponent(w, comp, v, nil)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", comp.Key(), err)
		}
		if comp.Key() == c.component {
			if raw, err = setPath(raw, c.field, key); err != nil {
				return nil, fmt.Errorf("%s: %w", comp.Key(), err)
			}
		}
		comps[comp.Key()] = raw
	}
	if layer, ok := b.DefaultLayer(pos); ok {
		for _, key := range layerKeys {
			if raw, has := comps[key]; has {
				var err error
				if comps[key], err = setLayer(raw, layer); err != nil {
					return nil, fmt.Errorf("%s: %w", key, err)
				}
			}
		}
	}
	return json.Marshal(struct {
		ID         string                     `json:"id"`
		Group      string                     `json:"group"`
		Class      string                     `json:"class"`
		Components map[string]json.RawMessage `json:"components"`
	}{b.freshID(c.schema.Class), c.schema.Group, c.schema.Class, comps})
}

// layerKeys are the components that have a render Layer.
var layerKeys = []string{"gfx:Sprite", "gfx:NineSlice", "gfx:Label", "gfx:Particles"}

// setLayer sets "Layer" in the JSON object raw.
func setLayer(raw json.RawMessage, layer int16) (json.RawMessage, error) {
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(raw, &obj); err != nil {
		return nil, err
	}
	obj["Layer"] = json.RawMessage(strconv.Itoa(int(layer)))
	return json.Marshal(obj)
}

// freshID returns "<class>-<n>" (class lower-cased) with the smallest n >= 1
// that no object of the level uses.
func (b *Bridge) freshID(class string) string {
	used := map[string]bool{}
	for _, id := range b.IDs() {
		used[id] = true
	}
	base := strings.ToLower(class)
	for n := 1; ; n++ {
		if id := fmt.Sprintf("%s-%d", base, n); !used[id] {
			return id
		}
	}
}

// setPath sets the string at path in the JSON object raw.
func setPath(raw json.RawMessage, path []string, value string) (json.RawMessage, error) {
	if len(path) == 0 {
		return json.Marshal(value)
	}
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber() // keep numbers exactly as encoded
	var obj map[string]json.RawMessage
	if err := dec.Decode(&obj); err != nil {
		return nil, err
	}
	inner, err := setPath(obj[path[0]], path[1:], value)
	if err != nil {
		return nil, err
	}
	obj[path[0]] = inner
	return json.Marshal(obj)
}
