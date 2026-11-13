package mcpserver

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"os"
	"slices"
	"strings"

	"github.com/weakpixel/engine/asset/library"
	"github.com/weakpixel/engine/god"
	"github.com/weakpixel/engine/tools/internal/mcp"
)

// maxPrefabDepth bounds a chain of prefab variants, as god does.
const maxPrefabDepth = 16

// document is a GOD document (a level or a prefab) as it is in the file.
type document struct {
	Name    string      `json:"name,omitempty"`
	Objects []docObject `json:"objects"`
}

// docObject is one entry of a document's "objects".
type docObject struct {
	ID         string                     `json:"id,omitempty"`
	Prefab     string                     `json:"prefab,omitempty"`
	Group      string                     `json:"group,omitempty"`
	Class      string                     `json:"class,omitempty"`
	Components map[string]json.RawMessage `json:"components,omitempty"`
}

// label names an object in messages: its id, else its index.
func (o docObject) label(i int) string {
	if o.ID != "" {
		return fmt.Sprintf("%q", o.ID)
	}
	return fmt.Sprintf("#%d", i)
}

// classKey is the object's group:class, "" when it gives none.
func (o docObject) classKey() string {
	if o.Group == "" && o.Class == "" {
		return ""
	}
	return god.SchemaMeta{Group: o.Group, Class: o.Class}.Key()
}

// parseDocument decodes data with the rules of god: only "name" and
// "objects" at the top, known keys in each object, nothing after the
// document. On a later error the partly decoded document is returned too.
func parseDocument(data []byte) (*document, error) {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	var doc struct {
		Name    string       `json:"name"`
		Objects *[]docObject `json:"objects"`
	}
	if err := dec.Decode(&doc); err != nil {
		return nil, fmt.Errorf("invalid document: %v", err)
	}
	if doc.Objects == nil {
		return &document{Name: doc.Name}, errors.New(`invalid document: missing "objects"`)
	}
	out := &document{Name: doc.Name, Objects: *doc.Objects}
	if _, err := dec.Token(); err != io.EOF {
		return out, errors.New("invalid document: data after the JSON document")
	}
	return out, nil
}

// documentKind returns "level" or "prefab" for a document key.
func documentKind(key string) (string, error) {
	switch library.KindOf(key) {
	case library.KindLevel:
		return "level", nil
	case library.KindPrefab:
		return "prefab", nil
	}
	return "", errNotDocument
}

// sha is the version of a document's text that edit tools compare.
func sha(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

// expanded is an object after its prefab chain is merged in.
type expanded struct {
	group, class string
	components   map[string]json.RawMessage
}

// expandPrefab returns the expanded object of the prefab file key. chain
// holds the prefabs being expanded, for cycle checks.
func (t *tools) expandPrefab(key string, chain []string) (*expanded, error) {
	if slices.Contains(chain, key) {
		return nil, fmt.Errorf("prefab cycle: %s -> %s", strings.Join(chain, " -> "), key)
	}
	if len(chain) >= maxPrefabDepth {
		return nil, fmt.Errorf("variant chain deeper than %d", maxPrefabDepth)
	}
	if library.KindOf(key) != library.KindPrefab {
		return nil, fmt.Errorf("prefab %q: not a *.prefab.json key", key)
	}
	abs, _, err := t.p.AssetPath(key)
	if err != nil {
		return nil, fmt.Errorf("prefab %q: %v", key, err)
	}
	data, err := os.ReadFile(abs)
	if errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("prefab %q does not exist", key)
	} else if err != nil {
		return nil, fmt.Errorf("prefab %q: %v", key, err)
	}
	doc, err := parseDocument(data)
	if err != nil {
		return nil, fmt.Errorf("prefab %q: %v", key, err)
	}
	if len(doc.Objects) != 1 {
		return nil, fmt.Errorf("prefab %q: has %d objects, want exactly one", key, len(doc.Objects))
	}
	return t.expandObject(doc.Objects[0], append(chain, key))
}

// expandObject merges obj over its prefab (if it names one). An object
// without a prefab is returned as it is.
func (t *tools) expandObject(obj docObject, chain []string) (*expanded, error) {
	if obj.Prefab == "" {
		return &expanded{obj.Group, obj.Class, obj.Components}, nil
	}
	base, err := t.expandPrefab(obj.Prefab, chain)
	if err != nil {
		return nil, err
	}
	baseKey := god.SchemaMeta{Group: base.group, Class: base.class}.Key()
	if k := obj.classKey(); k != "" && k != baseKey {
		return nil, fmt.Errorf("object %s does not match prefab %q, which is %s", k, obj.Prefab, baseKey)
	}
	comps, err := expandComponents(base.components, obj.Components)
	if err != nil {
		return nil, err
	}
	return &expanded{base.group, base.class, comps}, nil
}

// expandComponents merges overrides over base, as god does: null
// removes a component, other values merge with mergePatch.
func expandComponents(base, overrides map[string]json.RawMessage) (map[string]json.RawMessage, error) {
	out := maps.Clone(base)
	if out == nil {
		out = map[string]json.RawMessage{}
	}
	var errs []error
	for _, k := range slices.Sorted(maps.Keys(overrides)) {
		patch := overrides[k]
		if string(bytes.TrimSpace(patch)) == "null" {
			delete(out, k)
			continue
		}
		b, ok := out[k]
		if !ok {
			out[k] = patch
			continue
		}
		m, err := mergeJSON(b, patch)
		if err != nil {
			errs = append(errs, fmt.Errorf("component %s: %v", k, err))
			continue
		}
		out[k] = m
	}
	return out, errors.Join(errs...)
}

// mergeJSON applies the override patch to base with mergePatch.
func mergeJSON(base, patch json.RawMessage) (json.RawMessage, error) {
	b, err := decodeJSON(base)
	if err != nil {
		return nil, err
	}
	p, err := decodeJSON(patch)
	if err != nil {
		return nil, err
	}
	return json.Marshal(mergePatch(b, p))
}

// decodeJSON decodes raw into any, keeping numbers exact.
func decodeJSON(raw json.RawMessage) (any, error) {
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var v any
	err := dec.Decode(&v)
	return v, err
}

// mergePatch is the prefab override merge of god (the shared cases
// are in god/testdata/merge-patch.json): two objects merge key by
// key, recursively; anything else in patch replaces base, and null sets
// null instead of deleting a key (unlike RFC 7386).
func mergePatch(base, patch any) any {
	b, ok1 := base.(map[string]any)
	p, ok2 := patch.(map[string]any)
	if !ok1 || !ok2 {
		return patch
	}
	out := maps.Clone(b)
	for k, pv := range p {
		if bv, ok := out[k]; ok {
			out[k] = mergePatch(bv, pv)
		} else {
			out[k] = pv
		}
	}
	return out
}

// checkDocument runs the offline checks on a document: it parses, ids are
// unique, prefab instances expand, every object type and component key is
// in the GOD, required components are there and every value passes its
// schema, entity references name objects of the document and asset keys name
// files of the right kind. A prefab holds exactly one object. It returns every problem,
// each naming the object, and the parsed document (nil if it does not parse).
func (t *tools) checkDocument(key string, data []byte) (*document, []string) {
	doc, err := parseDocument(data)
	if err != nil {
		return doc, []string{err.Error()}
	}
	var problems []string
	add := func(format string, args ...any) { problems = append(problems, fmt.Sprintf(format, args...)) }
	if kind, _ := documentKind(key); kind == "prefab" && len(doc.Objects) != 1 {
		add("a prefab holds exactly one object, this one has %d", len(doc.Objects))
	}
	g := t.p.GOD()
	if g.err != nil {
		add("the GOD files could not be read, so objects were not checked: %v", g.err)
	}
	seen := map[string]int{}
	ids := map[string]bool{}
	for _, obj := range doc.Objects {
		if obj.ID != "" {
			ids[obj.ID] = true
		}
	}
	for i, obj := range doc.Objects {
		at := "object " + obj.label(i)
		if obj.ID != "" {
			if j, dup := seen[obj.ID]; dup {
				add("%s: id used again (first by object #%d)", at, j)
			}
			seen[obj.ID] = i
		}
		if obj.Prefab == "" && (obj.Group == "" || obj.Class == "") {
			add(`%s: needs "group" and "class", or a "prefab"`, at)
			continue
		}
		var chain []string
		if kind, _ := documentKind(key); kind == "prefab" {
			chain = []string{key}
		}
		exp, err := t.expandObject(obj, chain)
		if err != nil {
			add("%s: %v", at, err)
			continue
		}
		if g.err != nil {
			continue
		}
		for _, p := range checkObject(g, exp) {
			add("%s: %s", at, p)
		}
		for _, p := range t.checkReferences(g, obj, ids) {
			add("%s: %s", at, p)
		}
	}
	return doc, problems
}

// checkObject checks an expanded object against the GOD.
func checkObject(g *godData, exp *expanded) []string {
	key := god.SchemaMeta{Group: exp.group, Class: exp.class}.Key()
	s := g.objByKey[key]
	if s == nil {
		return []string{fmt.Sprintf("unknown object type %s", key)}
	}
	var problems []string
	for _, c := range s.Components {
		if _, ok := exp.components[c.Key()]; !ok && !c.Optional {
			problems = append(problems, fmt.Sprintf("%s: missing required component %s", key, c.Key()))
		}
	}
	for _, k := range slices.Sorted(maps.Keys(exp.components)) {
		c := g.compByKey[k]
		if c == nil {
			problems = append(problems, fmt.Sprintf("component %s is not in the GOD", k))
			continue
		}
		if c.Schema == nil {
			continue
		}
		if err := c.Schema.Validate(exp.components[k]); err != nil {
			for _, line := range strings.Split(err.Error(), "\n") {
				problems = append(problems, fmt.Sprintf("component %s: %s", k, line))
			}
		}
	}
	return problems
}

func (t *tools) documentTools() []*mcp.Tool {
	pathArg := mcp.String("asset key of the document, relative to the asset root (e.g. forest/forest.level.json)")
	return []*mcp.Tool{
		tool("read_document", "Read a document",
			"Reads a level (*.level.json) or prefab (*.prefab.json): its text, a sha of the text (edit tools take it to refuse "+
				"stale edits), its objects (id, object type, prefab) and every problem the offline checks find (unknown "+
				"components, missing required ones, values that do not match their schema, prefabs that do not expand). Changes nothing.",
			mcp.Object(map[string]*mcp.Schema{
				"path":         pathArg,
				"include_text": mcp.Boolean("return the text too (default true)"),
			}, "path"), readOnly, t.readDocument),
		tool("get_object", "Get an object",
			"Returns one object of a document by id (or by index for objects without an id). With merged, an instance of a "+
				"prefab is returned with the prefab's components merged in, and overridden lists the fields the instance sets "+
				"itself (component.Field.Sub). Changes nothing.",
			mcp.Object(map[string]*mcp.Schema{
				"path":   pathArg,
				"id":     mcp.String("the object's id"),
				"index":  mcp.Integer("the object's position in objects, for objects without an id", 0, 1<<30),
				"merged": mcp.Boolean("merge a prefab instance over its prefab (default false: the components as written)"),
			}, "path"), readOnly, t.getObject),
	}
}

// readDoc reads the document at key.
func (t *tools) readDoc(key string) (clean, kind string, data []byte, err error) {
	abs, clean, err := t.p.AssetPath(key)
	if err != nil {
		return "", "", nil, err
	}
	if kind, err = documentKind(clean); err != nil {
		return "", "", nil, err
	}
	data, err = os.ReadFile(abs)
	if errors.Is(err, os.ErrNotExist) {
		return "", "", nil, fmt.Errorf("%s does not exist (list_documents lists the documents)", clean)
	}
	return clean, kind, data, err
}

type readDocumentArgs struct {
	Path        string `json:"path"`
	IncludeText *bool  `json:"include_text"`
}

func (t *tools) readDocument(_ context.Context, a readDocumentArgs) (any, error) {
	key, kind, data, err := t.readDoc(a.Path)
	if err != nil {
		return nil, err
	}
	doc, problems := t.checkDocument(key, data)
	type entry struct {
		Index  int    `json:"index"`
		ID     string `json:"id,omitempty"`
		Type   string `json:"type,omitempty"`
		Prefab string `json:"prefab,omitempty"`
	}
	objects := []entry{}
	out := map[string]any{"path": key, "kind": kind, "sha": sha(data), "valid": len(problems) == 0, "problems": nonNil(problems)}
	if doc != nil {
		out["name"] = doc.Name
		for i, o := range doc.Objects {
			e := entry{Index: i, ID: o.ID, Type: o.classKey(), Prefab: o.Prefab}
			if e.Type == "" && o.Prefab != "" {
				if exp, err := t.expandObject(o, nil); err == nil {
					e.Type = god.SchemaMeta{Group: exp.group, Class: exp.class}.Key()
				}
			}
			objects = append(objects, e)
		}
	}
	out["objects"] = objects
	if a.IncludeText == nil || *a.IncludeText {
		out["text"] = string(data)
	}
	return out, nil
}

type getObjectArgs struct {
	Path   string `json:"path"`
	ID     string `json:"id"`
	Index  *int   `json:"index"`
	Merged bool   `json:"merged"`
}

func (t *tools) getObject(_ context.Context, a getObjectArgs) (any, error) {
	key, _, data, err := t.readDoc(a.Path)
	if err != nil {
		return nil, err
	}
	doc, err := parseDocument(data)
	if err != nil {
		return nil, err
	}
	i, err := findObject(doc, a.ID, a.Index)
	if err != nil {
		return nil, fmt.Errorf("%s: %v", key, err)
	}
	obj := doc.Objects[i]
	out := map[string]any{"path": key, "index": i, "id": obj.ID, "type": obj.classKey(), "prefab": obj.Prefab}
	comps := obj.Components
	if comps == nil {
		comps = map[string]json.RawMessage{}
	}
	if a.Merged && obj.Prefab != "" {
		exp, err := t.expandObject(obj, nil)
		if err != nil {
			return nil, fmt.Errorf("object %s: %v", obj.label(i), err)
		}
		out["type"] = god.SchemaMeta{Group: exp.group, Class: exp.class}.Key()
		out["overridden"] = overriddenPaths(obj.Components)
		comps = exp.components
	}
	out["components"] = comps
	return out, nil
}

// findObject returns the index of the object with id, or index.
func findObject(doc *document, id string, index *int) (int, error) {
	switch {
	case id != "" && index != nil:
		return 0, errors.New("give id or index, not both")
	case id != "":
		for i, o := range doc.Objects {
			if o.ID == id {
				return i, nil
			}
		}
		return 0, fmt.Errorf("no object with id %q", id)
	case index != nil:
		if *index >= len(doc.Objects) {
			return 0, fmt.Errorf("index %d is out of range: the document has %d objects", *index, len(doc.Objects))
		}
		return *index, nil
	}
	return 0, errors.New("give the object's id or index")
}

// overriddenPaths lists the leaves of an instance's overrides as
// "component.Field.Sub", sorted; a removed component is "component (removed)".
func overriddenPaths(overrides map[string]json.RawMessage) []string {
	paths := []string{}
	var walk func(prefix string, v any)
	walk = func(prefix string, v any) {
		m, ok := v.(map[string]any)
		if !ok || len(m) == 0 {
			paths = append(paths, prefix)
			return
		}
		for k, sub := range m {
			walk(prefix+"."+k, sub)
		}
	}
	for k, raw := range overrides {
		if string(bytes.TrimSpace(raw)) == "null" {
			paths = append(paths, k+" (removed)")
			continue
		}
		v, err := decodeJSON(raw)
		if err != nil {
			continue
		}
		walk(k, v)
	}
	slices.Sort(paths)
	return paths
}
