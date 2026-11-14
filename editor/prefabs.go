package editor

import (
	"encoding/json"
	"errors"
	"fmt"

	"github.com/weakpixel/asset"
	"github.com/weakpixel/engine/god"
	"github.com/weakpixel/engine/prefab"
)

// errPending is returned by apply when a prefab the document uses is still
// loading; the session applies the document again once it is ready.
var errPending = errors.New("editor: waiting for prefabs to load")

// prefabStamp is what the session last saw of a prefab, to notice a reload
// (Version) or a retry after a failure (state).
type prefabStamp struct {
	version uint32
	state   asset.State
}

// expand turns the prefab instances of docs into plain objects: the
// prefab's object type and its components with the instance's overrides
// merged over them, checked against the schema. An instance of a
// multi-object prefab becomes several objects (the root, then its parts
// with ids `<id>/<part>` and Part set). The prefabs are loaded into the
// level's scope, which holds them until the level is unloaded. It returns
// errPending if one of them is still loading and no other problem was
// found.
func (s *Session) expand(docs []docObject) ([]docObject, error) {
	var errs []error
	pending := false
	out := make([]docObject, 0, len(docs))
	for i, d := range docs {
		if d.Prefab == "" {
			out = append(out, d)
			continue
		}
		p, err := s.prefab(d.Prefab)
		if errors.Is(err, errPending) {
			pending = true
			continue
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("objects[%d] (%s): prefab %q: %w", i, d.ID, d.Prefab, err))
			continue
		}
		objs, err := p.ExpandInstance(god.PrefabEntry{ID: d.ID, Group: d.Group, Class: d.Class, Components: d.Components, Children: d.Children})
		for _, o := range objs {
			if err == nil {
				err = god.CheckComponents(o.Group, o.Class, o.Components)
			}
			out = append(out, docObject{ID: o.ID, Prefab: d.Prefab, Group: o.Group, Class: o.Class, Components: o.Components, Part: o.Part})
		}
		if err != nil {
			errs = append(errs, fmt.Errorf("objects[%d] (%s): %w", i, d.ID, err))
		}
	}
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}
	if pending {
		return nil, errPending
	}
	return out, nil
}

// prefab returns the prefab key, loading it into the level's scope on first
// use, and records its stamp. It returns errPending while it loads.
func (s *Session) prefab(key string) (*god.Prefab, error) {
	h, ok := s.prefabs[key]
	if !ok {
		var err error
		if h, err = loadPrefab(s.level.Scope(), key); err != nil {
			return nil, err
		}
		s.prefabs[key] = h
	}
	store := s.e.Stores().Prefabs()
	v, state := store.Lookup(h)
	s.prefabSeen[key] = prefabStamp{version: store.Version(h), state: state}
	switch state {
	case asset.Ready:
		return v.Types(), nil
	case asset.Failed:
		return nil, store.Err(h)
	}
	return nil, errPending
}

// loadPrefab is prefab.Load with a bad key (document data) returned as an
// error instead of a panic.
func loadPrefab(sc *asset.Scope, key string) (h prefab.Handle, err error) {
	if sc == nil {
		return h, errors.New("the level has no asset scope")
	}
	defer func() {
		if r := recover(); r != nil {
			msg, ok := r.(string)
			if !ok {
				panic(r)
			}
			err = errors.New(msg)
		}
	}()
	return prefab.Load(sc, key), nil
}

// prefabsChanged reports whether a prefab the level uses was reloaded, or
// finished loading or failing, since the session last looked at it.
func (s *Session) prefabsChanged() bool {
	store := s.e.Stores().Prefabs()
	for key, h := range s.prefabs {
		_, state := store.Lookup(h)
		if (prefabStamp{version: store.Version(h), state: state}) != s.prefabSeen[key] {
			return true
		}
	}
	return false
}

// updatePrefabs runs at the start of every edit tick: it applies a document
// that waited for its prefabs once they are ready, and applies the current
// document again when a prefab it uses changed (its file was saved), so
// every instance shows the new prefab. The outcome is an [Applied] event.
func (s *Session) updatePrefabs() {
	if s.level == nil {
		return
	}
	text := s.pending
	if text == nil {
		if s.doc == nil || !s.prefabsChanged() {
			return
		}
		text = s.doc
	}
	err := s.apply(text, false)
	if errors.Is(err, errPending) {
		s.pending = text
		return
	}
	s.applied(err)
	s.emit(Applied{Err: err})
}

// applied updates the pending state after a document was applied (err nil)
// or rejected: a newer document replaces one that waited, and a level opened
// with a waiting document is framed once one is applied.
func (s *Session) applied(err error) {
	s.pending = nil
	if err == nil && s.pendingOpen {
		s.pendingOpen = false
		s.frameLevel = !s.camPinned
	}
}

// override returns the override of the component with key of the instance
// o (a root, or one part of an instance) whose whole component is raw: only
// what differs from the prefab, {} for nothing.
func (s *Session) override(o *object, key string, raw []byte) ([]byte, error) {
	h, ok := s.prefabs[o.prefab]
	if !ok {
		return nil, fmt.Errorf("prefab %q is not loaded", o.prefab)
	}
	p := s.e.Stores().Prefabs().Get(h).Types()
	if p == nil {
		return nil, fmt.Errorf("prefab %q is not ready", o.prefab)
	}
	ov, err := p.PartOverrides(o.root, o.part, map[string]json.RawMessage{key: raw})
	if err != nil {
		return nil, err
	}
	if v, ok := ov[key]; ok {
		return v, nil
	}
	return []byte("{}"), nil
}
