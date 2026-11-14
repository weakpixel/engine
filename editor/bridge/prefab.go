package bridge

import (
	"encoding/json"
	"fmt"
	"path"
	"strings"

	"github.com/weakpixel/engine/editor"
	"github.com/weakpixel/engine/gfx"
	"github.com/weakpixel/engine/god"
	"github.com/weakpixel/engine/prefab"
)

// createInstance sends "create" with a new instance of the prefab key at
// pos: {"id", "prefab", "components"} where the components override only
// the gfx:Transform position (none if the prefab has no Transform) and the
// Layer of its drawn components when the map's default layer differs. Its id is
// "<prefab name>-<n>", unused in the level.
func (b *Bridge) createInstance(key string, pos gfx.Vec2) {
	obj, err := b.newInstance(key, pos)
	if err != nil {
		b.sendError(fmt.Errorf("create an instance of %s: %w", key, err))
		return
	}
	b.send(createMsg{Type: "create", Object: obj})
}

func (b *Bridge) newInstance(key string, pos gfx.Vec2) (json.RawMessage, error) {
	if b.Level() == nil {
		return nil, fmt.Errorf("no level is open")
	}
	h, err := loadPrefab(b, key)
	if err != nil {
		return nil, err
	}
	p := b.Engine().Stores().Prefabs().Get(h).Types()
	patches := map[string]json.RawMessage{}
	if _, ok := p.Component(editor.TransformKey); ok && hasTransform(p.Meta().Group, p.Meta().Class) {
		at, err := json.Marshal(gfx.Vec2{X: pos.X, Y: pos.Y})
		if err != nil {
			return nil, err
		}
		patches[editor.TransformKey] = json.RawMessage(fmt.Sprintf(`{"Pos":%s,"PrevPos":%s}`, at, at))
	}
	if layer, ok := b.DefaultLayer(pos); ok {
		for _, key := range layerKeys {
			if _, has := p.Component(key); has {
				patches[key] = json.RawMessage(fmt.Sprintf(`{"Layer":%d}`, layer))
			}
		}
	}
	// Only what differs from the prefab is written: the Transform position
	// and a Layer that is not the prefab's own.
	comps := map[string]json.RawMessage{}
	if len(patches) > 0 {
		merged, err := p.Expand("", "", patches)
		if err != nil {
			return nil, err
		}
		ov, err := p.Overrides(merged)
		if err != nil {
			return nil, err
		}
		for key := range patches {
			if o, ok := ov[key]; ok {
				comps[key] = o
			}
		}
	}
	return json.Marshal(struct {
		ID         string                     `json:"id"`
		Prefab     string                     `json:"prefab"`
		Components map[string]json.RawMessage `json:"components"`
	}{b.freshID(prefabName(key)), key, comps})
}

// loadPrefab loads the prefab key into the level's scope and waits for it;
// a bad key is an error, not a panic (it comes from the host).
func loadPrefab(b *Bridge, key string) (h prefab.Handle, err error) {
	defer func() {
		if r := recover(); r != nil {
			msg, ok := r.(string)
			if !ok {
				panic(r)
			}
			err = fmt.Errorf("%s", msg)
		}
	}()
	return prefab.LoadSync(b.Level().Scope(), key)
}

// hasTransform reports whether the object type group:class has the
// gfx.Transform component (key [editor.TransformKey]).
func hasTransform(group, class string) bool {
	s, ok := god.LookupSchema(group, class)
	if !ok {
		return false
	}
	for _, c := range s.Components {
		if c.Key() == editor.TransformKey {
			return true
		}
	}
	return false
}

// prefabName is the file name of a prefab key without its suffix, as ids
// and labels use it: "global/prefabs/villager.prefab.json" is "villager".
func prefabName(key string) string {
	return strings.TrimSuffix(path.Base(key), prefab.Suffix)
}
