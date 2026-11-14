package editor

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/weakpixel/engine/god"
)

// docObject is one object of an editor document: a serialized GOD object
// (doc/guide/ecs/game-object-definition.md) whose "id" is required, or an
// instance of a prefab (Prefab set, Components its overrides) until the
// session expands it.
type docObject struct {
	ID         string                     `json:"id"`
	Prefab     string                     `json:"prefab"`
	Group      string                     `json:"group"`
	Class      string                     `json:"class"`
	Components map[string]json.RawMessage `json:"components"`
	// Children are the overrides of the parts of an instance of a
	// multi-object prefab (see god.PrefabEntry).
	Children map[string]json.RawMessage `json:"children"`
	// Part is the part id of an object an instance expanded to ("" for
	// document entries and instance roots). Set by Session.expand.
	Part string `json:"-"`
}

// parseDocument reads an editor document and checks what can be checked
// without a World: the JSON shape, ids (present and unique), known schemas
// and component sets. The components of a prefab instance are checked once
// it is expanded (Session.expand). Every problem is reported, joined, as
// "objects[<i>]: <problem>". It also returns the document's "name" ("" if none).
func parseDocument(text []byte) ([]docObject, string, error) {
	dec := json.NewDecoder(bytes.NewReader(text))
	dec.DisallowUnknownFields()
	var doc struct {
		Name    string       `json:"name"` // label for editors
		Objects *[]docObject `json:"objects"`
	}
	if err := dec.Decode(&doc); err != nil {
		return nil, "", fmt.Errorf("editor: %w", err)
	}
	if doc.Objects == nil {
		return nil, "", errors.New(`editor: missing "objects"`)
	}
	if _, err := dec.Token(); err != io.EOF {
		return nil, "", errors.New("editor: unexpected data after the JSON document")
	}
	objs := *doc.Objects
	seen := make(map[string]int, len(objs))
	var errs []error
	for i, o := range objs {
		switch first, dup := seen[o.ID]; {
		case o.ID == "":
			errs = append(errs, fmt.Errorf(`objects[%d]: no "id" (the editor needs one on every object)`, i))
		case dup:
			errs = append(errs, fmt.Errorf("objects[%d]: duplicate id %q (first used by objects[%d])", i, o.ID, first))
		default:
			seen[o.ID] = i
		}
		if strings.Contains(o.ID, "/") {
			errs = append(errs, fmt.Errorf(`objects[%d]: id %q contains "/", which is reserved for the parts of prefab instances`, i, o.ID))
		}
		if o.Prefab != "" {
			continue
		}
		if o.Children != nil {
			errs = append(errs, fmt.Errorf(`objects[%d]: "children" needs a "prefab"`, i))
		}
		if err := god.CheckComponents(o.Group, o.Class, o.Components); err != nil {
			errs = append(errs, fmt.Errorf("objects[%d]: %w", i, err))
		}
	}
	return objs, doc.Name, errors.Join(errs...)
}

// compactJSON is raw without insignificant whitespace, so that a reformatted
// but equal component is not applied again. Invalid JSON is returned as is
// (the codec reports it).
func compactJSON(raw json.RawMessage) string {
	var b bytes.Buffer
	if err := json.Compact(&b, raw); err != nil {
		return string(raw)
	}
	return b.String()
}

// hasEntityField reports whether s describes an entity reference anywhere
// (format "entity" on s or a nested property, item or map value). Such a
// component must be decoded again when objects are created or deleted,
// because the entity behind an id may have changed.
func hasEntityField(s *god.TypeSchema) bool {
	if s == nil {
		return false
	}
	if s.Format == god.FormatEntity {
		return true
	}
	for _, p := range s.Properties {
		if hasEntityField(p) {
			return true
		}
	}
	return hasEntityField(s.Items) || hasEntityField(s.AdditionalProperties)
}
