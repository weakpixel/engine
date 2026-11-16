package mcpserver

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/weakpixel/engine/asset/library"
	"github.com/weakpixel/engine/god"
	"github.com/weakpixel/engine/tools/internal/mcp"
)

// assetKinds are the values of list_assets' kind filter: the library's
// kinds, and "unknown" for files of no known kind.
var assetKinds = []string{
	string(library.KindTexture), string(library.KindAtlas), string(library.KindClipSet), string(library.KindTileset),
	string(library.KindMap), string(library.KindWorld), string(library.KindFont), string(library.KindShader),
	string(library.KindSound), string(library.KindMusic), string(library.KindBindings), string(library.KindLevel),
	string(library.KindPrefab), string(library.KindEffect), string(library.KindVoice), string(library.KindLanguage),
	string(library.KindDialogue), "unknown",
}

// maxAssets bounds list_assets' limit.
const maxAssets = 5000

func (t *tools) projectTools() []*mcp.Tool {
	groupArg := mcp.String("only entries of this group (the part before ':', e.g. gfx)")
	return []*mcp.Tool{
		tool("project_info", "Project info",
			"Describes the project: its root, the asset root, where the GOD (component and object type definitions) is read from, "+
				"the editor package and build tags, whether a game runs, and warnings about missing settings. Changes nothing.",
			mcp.Object(nil), readOnly, t.projectInfo),
		tool("list_components", "List components",
			"Lists the component types of the GOD with key (group:Name), Go type and the first line of their doc. Changes nothing.",
			mcp.Object(map[string]*mcp.Schema{"group": groupArg}), readOnly, t.listComponents),
		tool("describe_component", "Describe a component",
			"Returns one component type: its JSON schema (fields, types, enums, asset kinds, x-editor hints), the initial value an "+
				"editor writes for a new component, its doc and the object types that use it. Changes nothing.",
			mcp.Object(map[string]*mcp.Schema{"key": mcp.String("component key, group:Name (e.g. gfx:Transform)")}, "key"), readOnly, t.describeComponent),
		tool("list_object_types", "List object types",
			"Lists the object types (Game Object classes) of the GOD with key (group:Class), Go type, doc and component keys. Changes nothing.",
			mcp.Object(map[string]*mcp.Schema{"group": groupArg}), readOnly, t.listObjectTypes),
		tool("describe_object_type", "Describe an object type",
			"Returns one object type: each component with its Go field, whether it is optional or readonly, and the embedded "+
				"objects it comes through. Use describe_component for a component's fields. Changes nothing.",
			mcp.Object(map[string]*mcp.Schema{"key": mcp.String("object type key, group:Class (e.g. gfx:Visual)")}, "key"), readOnly, t.describeObjectType),
		tool("list_assets", "List assets",
			"Lists the files of the asset library as asset keys (paths relative to the asset root) with their kind, sorted. Changes nothing.",
			mcp.Object(map[string]*mcp.Schema{
				"kind":   mcp.Enum("only assets of this kind", assetKinds...),
				"prefix": mcp.String("only keys starting with this (e.g. forest/)"),
				"limit":  mcp.Integer("at most this many keys (default 200)", 1, maxAssets),
			}), readOnly, t.listAssets),
		tool("check_assets", "Check assets",
			"Checks the asset library like `wpix assets check`: missing references, case mismatches, oversized textures and "+
				"other findings, each with the file. Changes nothing.",
			mcp.Object(nil), readOnly, t.checkAssets),
		tool("list_documents", "List documents",
			"Lists the levels (*.level.json) and prefabs (*.prefab.json) of the asset library with their name and object count. Changes nothing.",
			mcp.Object(map[string]*mcp.Schema{"kind": mcp.Enum("only levels or only prefabs", "level", "prefab")}), readOnly, t.listDocuments),
	}
}

type noArgs struct{}

func (t *tools) projectInfo(context.Context, noArgs) (any, error) {
	p := t.p
	g := p.GOD()
	info := map[string]any{
		"root":         p.Root,
		"settingsFile": p.SettingsFile,
		"assetRoot":    p.AssetRoot,
		"godInputs":    p.GODInputs,
		"components":   len(g.components),
		"objectTypes":  len(g.objects),
		"editor":       map[string]any{"game": p.Game, "tags": p.Tags},
		"gameRunning":  t.game.current() != nil,
		"warnings":     nonNil(p.Warnings),
	}
	if g.err != nil {
		info["godError"] = g.err.Error()
	}
	if p.Settings != nil {
		info["editor"].(map[string]any)["snap"] = p.Settings.EditorSnap
	}
	return info, nil
}

func nonNil[T any](s []T) []T {
	if s == nil {
		return []T{}
	}
	return s
}

// god returns the merged GOD, or its load error.
func (t *tools) god() (*godData, error) {
	g := t.p.GOD()
	if g.err != nil {
		return nil, fmt.Errorf("the GOD files could not be read (run go generate in the game?): %v", g.err)
	}
	return g, nil
}

// firstLine is the first line of a doc comment.
func firstLine(doc string) string {
	line, _, _ := strings.Cut(doc, "\n")
	return line
}

type groupArgs struct {
	Group string `json:"group"`
}

func (t *tools) listComponents(_ context.Context, a groupArgs) (any, error) {
	g, err := t.god()
	if err != nil {
		return nil, err
	}
	type entry struct {
		Key    string `json:"key"`
		GoType string `json:"goType,omitempty"`
		Doc    string `json:"doc,omitempty"`
	}
	out := []entry{}
	for _, c := range g.components {
		if a.Group == "" || c.Group == a.Group {
			out = append(out, entry{c.Key(), c.GoType, firstLine(c.Doc)})
		}
	}
	return map[string]any{"components": out}, nil
}

type keyArgs struct {
	Key string `json:"key"`
}

// unknownKey is the error for a key the GOD lacks, naming keys with the
// same name in other groups.
func unknownKey(what, key string, keys []string) error {
	_, name, _ := strings.Cut(key, ":")
	if name == "" {
		name = key
	}
	var similar []string
	for _, k := range keys {
		if _, n, _ := strings.Cut(k, ":"); strings.EqualFold(n, name) {
			similar = append(similar, k)
		}
	}
	if len(similar) > 0 {
		return fmt.Errorf("unknown %s %q; did you mean %s?", what, key, strings.Join(similar, ", "))
	}
	return fmt.Errorf("unknown %s %q (keys are group:Name; list them with list_%ss)", what, key, strings.ReplaceAll(what, " ", "_"))
}

func (t *tools) describeComponent(_ context.Context, a keyArgs) (any, error) {
	g, err := t.god()
	if err != nil {
		return nil, err
	}
	c := g.compByKey[a.Key]
	if c == nil {
		var keys []string
		for _, c := range g.components {
			keys = append(keys, c.Key())
		}
		return nil, unknownKey("component", a.Key, keys)
	}
	usedBy := []string{}
	for _, o := range g.objects {
		if slices.ContainsFunc(o.Components, func(oc god.Component) bool { return oc.Key() == a.Key }) {
			usedBy = append(usedBy, o.Key())
		}
	}
	out := map[string]any{"key": c.Key(), "goType": c.GoType, "doc": c.Doc, "schema": c.Schema, "usedBy": usedBy}
	if c.Schema != nil {
		out["initialValue"] = c.Schema.InitialValue()
	}
	return out, nil
}

// objectEntry is an object type in list_object_types.
type objectEntry struct {
	Key        string   `json:"key"`
	GoType     string   `json:"goType,omitempty"`
	Doc        string   `json:"doc,omitempty"`
	Components []string `json:"components"`
}

func (t *tools) listObjectTypes(_ context.Context, a groupArgs) (any, error) {
	g, err := t.god()
	if err != nil {
		return nil, err
	}
	out := []objectEntry{}
	for _, o := range g.objects {
		if a.Group != "" && o.Group != a.Group {
			continue
		}
		e := objectEntry{Key: o.Key(), GoType: o.GoType, Doc: firstLine(o.Doc), Components: []string{}}
		for _, c := range o.Components {
			e.Components = append(e.Components, c.Key())
		}
		out = append(out, e)
	}
	return map[string]any{"objectTypes": out}, nil
}

func (t *tools) describeObjectType(_ context.Context, a keyArgs) (any, error) {
	g, err := t.god()
	if err != nil {
		return nil, err
	}
	o := g.objByKey[a.Key]
	if o == nil {
		var keys []string
		for _, o := range g.objects {
			keys = append(keys, o.Key())
		}
		return nil, unknownKey("object type", a.Key, keys)
	}
	type comp struct {
		Component   string   `json:"component"`
		Field       string   `json:"field,omitempty"`
		Optional    bool     `json:"optional"`
		Readonly    bool     `json:"readonly"`
		EmbeddedVia []string `json:"embeddedVia,omitempty"`
		Doc         string   `json:"doc,omitempty"`
	}
	comps := []comp{}
	for _, c := range o.Components {
		comps = append(comps, comp{c.Key(), c.Field, c.Optional, c.Readonly, c.EmbeddedVia, c.Doc})
	}
	return map[string]any{
		"key": o.Key(), "goType": o.GoType, "doc": o.Doc, "components": comps,
		"note": "Objects may also hold any other component of the GOD; required components must be present.",
	}, nil
}

// assetFile is one file of the asset library.
type assetFile struct {
	Key  string `json:"key"`
	Kind string `json:"kind"`
}

// assetFiles lists every file below the asset root, sorted by key.
// Folders and files starting with "." are skipped, and so are the files
// .assetignore files ignore: they are not part of the game.
func (t *tools) assetFiles() ([]assetFile, error) {
	root := t.p.AssetRoot
	if root == "" {
		return nil, errNoAssetRoot
	}
	ig, _ := library.ReadIgnore(os.DirFS(root)) // bad lines: wpix assets check reports them
	var files []assetFile
	err := filepath.WalkDir(root, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if strings.HasPrefix(d.Name(), ".") && p != root {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(root, p)
		if err != nil {
			return err
		}
		key := filepath.ToSlash(rel)
		if ignored, _ := ig.Match(key, false); ignored {
			return nil
		}
		kind := string(library.KindOf(key))
		if kind == "" {
			kind = "unknown"
		}
		files = append(files, assetFile{key, kind})
		return nil
	})
	slices.SortFunc(files, func(a, b assetFile) int { return strings.Compare(a.Key, b.Key) })
	return files, err
}

type listAssetsArgs struct {
	Kind   string `json:"kind"`
	Prefix string `json:"prefix"`
	Limit  int    `json:"limit"`
}

func (t *tools) listAssets(_ context.Context, a listAssetsArgs) (any, error) {
	files, err := t.assetFiles()
	if err != nil {
		return nil, err
	}
	limit := a.Limit
	if limit == 0 {
		limit = 200
	}
	out := []assetFile{}
	total := 0
	for _, f := range files {
		if (a.Kind != "" && f.Kind != a.Kind) || !strings.HasPrefix(f.Key, a.Prefix) {
			continue
		}
		total++
		if len(out) < limit {
			out = append(out, f)
		}
	}
	return map[string]any{"assets": out, "total": total, "truncated": total > len(out)}, nil
}

func (t *tools) checkAssets(context.Context, noArgs) (any, error) {
	if t.p.AssetRoot == "" {
		return nil, errNoAssetRoot
	}
	_, findings, err := library.Scan(os.DirFS(t.p.AssetRoot))
	if err != nil {
		return nil, fmt.Errorf("the library cannot be read: %v", err)
	}
	type finding struct {
		Severity string `json:"severity"`
		Path     string `json:"path"`
		Msg      string `json:"msg"`
	}
	out := []finding{}
	errs, warns := 0, 0
	for _, f := range findings {
		out = append(out, finding{f.Severity.String(), f.Path, f.Msg})
		if f.Severity == library.Error {
			errs++
		} else {
			warns++
		}
	}
	return map[string]any{"findings": out, "errors": errs, "warnings": warns}, nil
}

type listDocumentsArgs struct {
	Kind string `json:"kind"`
}

func (t *tools) listDocuments(_ context.Context, a listDocumentsArgs) (any, error) {
	files, err := t.assetFiles()
	if err != nil {
		return nil, err
	}
	type entry struct {
		Path    string `json:"path"`
		Kind    string `json:"kind"`
		Name    string `json:"name,omitempty"`
		Objects int    `json:"objects"`
		Error   string `json:"error,omitempty"`
	}
	out := []entry{}
	for _, f := range files {
		if (f.Kind != "level" && f.Kind != "prefab") || (a.Kind != "" && f.Kind != a.Kind) {
			continue
		}
		e := entry{Path: f.Key, Kind: f.Kind}
		data, err := os.ReadFile(filepath.Join(t.p.AssetRoot, filepath.FromSlash(f.Key)))
		if err == nil {
			var doc *document
			doc, err = parseDocument(data)
			if doc != nil {
				e.Name, e.Objects = doc.Name, len(doc.Objects)
			}
		}
		if err != nil {
			e.Error = err.Error()
		}
		out = append(out, e)
	}
	return map[string]any{"documents": out}, nil
}

// errNotDocument is returned for a path that is no level or prefab.
var errNotDocument = errors.New("not a document: the path must end in .level.json or .prefab.json")
