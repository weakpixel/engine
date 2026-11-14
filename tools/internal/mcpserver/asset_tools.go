package mcpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path"
	"strings"
	"unicode/utf8"

	"github.com/weakpixel/engine/anim/animfile"
	"github.com/weakpixel/engine/god"
	"github.com/weakpixel/engine/particles/fxfile"
	"github.com/weakpixel/engine/sheetfile"
	"github.com/weakpixel/engine/tools/internal/mcp"
)

// maxAssetText bounds the text write_asset_document accepts.
const maxAssetText = 4 << 20

// schemaAt returns the schema of the field at path below s.
func schemaAt(s *god.TypeSchema, path []any) (*god.TypeSchema, error) {
	for i, step := range path {
		if s == nil {
			return nil, fmt.Errorf("no field %s", describePath(path[:i+1]))
		}
		switch v := step.(type) {
		case string:
			next := s.Properties[v]
			if next == nil {
				next = s.AdditionalProperties
			}
			s = next
		case int:
			s = s.Items
		}
	}
	if s == nil {
		return nil, fmt.Errorf("no field %s", describePath(path))
	}
	return s, nil
}

type setAssetFieldArgs struct {
	objArgs
	Component string `json:"component"`
	Field     []any  `json:"field"`
	Asset     string `json:"asset"`
}

func (t *tools) setAssetField(_ context.Context, a setAssetFieldArgs) (any, error) {
	g, err := t.god()
	if err != nil {
		return nil, err
	}
	c := g.compByKey[a.Component]
	if c == nil {
		return nil, fmt.Errorf("unknown component %q", a.Component)
	}
	field, err := fieldPath(a.Field)
	if err != nil {
		return nil, err
	}
	if len(field) == 0 {
		return nil, errors.New(`"field" is required`)
	}
	fs, err := schemaAt(c.Schema, field)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", a.Component, err)
	}
	if fs.Format != "asset" {
		return nil, fmt.Errorf("%s.%s is not an asset field (describe_component marks them format asset)", a.Component, describePath(field))
	}
	if a.Asset != "" {
		if msg := t.checkAssetKey(fs.XAsset, a.Asset); msg != "" {
			return nil, fmt.Errorf("%s.%s: %s", a.Component, describePath(field), msg)
		}
	}
	value, _ := json.Marshal(a.Asset)
	return t.editDocument(a.Path, a.Expect, func(text []byte, doc *document, info docInfo) ([]byte, error) {
		i, obj, err := locate(doc, a.objArgs)
		if err != nil {
			return nil, err
		}
		out, err := t.setAt(text, info, i, obj, a.Component, field, value)
		if err != nil {
			return nil, fmt.Errorf("object %s: %w", obj.label(i), err)
		}
		return out, nil
	})
}

type createDocumentArgs struct {
	Path  string `json:"path"`
	Name  string `json:"name"`
	Class string `json:"class"`
}

func (t *tools) createDocument(_ context.Context, a createDocumentArgs) (any, error) {
	t.docMu.Lock()
	defer t.docMu.Unlock()
	abs, key, err := t.p.AssetPath(a.Path)
	if err != nil {
		return nil, err
	}
	kind, err := documentKind(key)
	if err != nil {
		return nil, err
	}
	if _, err := os.Lstat(abs); err == nil {
		return nil, fmt.Errorf("%s already exists (create_document never overwrites; edit it with the edit tools)", key)
	}
	doc := struct {
		Name    string            `json:"name,omitempty"`
		Objects []json.RawMessage `json:"objects"`
	}{Name: a.Name, Objects: []json.RawMessage{}}
	switch {
	case kind == "prefab":
		// A prefab holds exactly one object, so it starts with one.
		if a.Class == "" {
			return nil, errors.New(`a prefab holds exactly one object: give "class", the object type (e.g. demo:Crate)`)
		}
		g, err := t.god()
		if err != nil {
			return nil, err
		}
		s := g.objByKey[a.Class]
		if s == nil {
			var keys []string
			for _, o := range g.objects {
				keys = append(keys, o.Key())
			}
			return nil, unknownKey("object type", a.Class, keys)
		}
		obj, err := t.newClassObject(g, s, strings.ToLower(s.Class), 0, 0, nil)
		if err != nil {
			return nil, err
		}
		doc.Objects = append(doc.Objects, obj)
		if doc.Name == "" {
			doc.Name = strings.TrimSuffix(path.Base(key), ".prefab.json")
		}
	case a.Class != "":
		return nil, errors.New(`"class" is for prefabs; place_object adds objects to a level`)
	}
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, err
	}
	data = append(data, '\n')
	if _, problems := t.checkDocument(key, data); len(problems) > 0 {
		return nil, fmt.Errorf("the new document would have problems, so nothing was written:\n- %s", strings.Join(problems, "\n- "))
	}
	if err := t.writeDocument(abs, key, nil, data); err != nil {
		return nil, err
	}
	return map[string]any{"path": key, "kind": kind, "sha": sha(data), "text": string(data)}, nil
}

type writeAssetArgs struct {
	Path   string `json:"path"`
	Text   string `json:"text"`
	Expect string `json:"expect"`
}

// writableKinds names what write_asset_document accepts.
const writableKinds = "*.level.json, *.prefab.json, *.sheet.json, *.anim.json and *.fx.json"

// checkAssetText decodes and validates text as the asset file key, by its
// suffix; it returns what is wrong, one problem per line.
func (t *tools) checkAssetText(key string, text []byte) error {
	lower := strings.ToLower(key)
	switch {
	case strings.HasSuffix(lower, ".level.json"), strings.HasSuffix(lower, ".prefab.json"):
		if _, problems := t.checkDocument(key, text); len(problems) > 0 {
			return errors.New(strings.Join(problems, "\n"))
		}
		return nil
	case strings.HasSuffix(lower, ".sheet.json"):
		_, err := sheetfile.Decode(text)
		return err
	case strings.HasSuffix(lower, ".anim.json"):
		def, err := animfile.Decode(text)
		if err != nil {
			return err
		}
		sheetAbs, sheetKey, err := t.p.AssetPath(path.Join(path.Dir(key), def.Sheet))
		if err != nil {
			return fmt.Errorf("sheet %q: %v", def.Sheet, err)
		}
		data, err := os.ReadFile(sheetAbs)
		if err != nil {
			return fmt.Errorf("sheet %q (%s) cannot be read: %v", def.Sheet, sheetKey, err)
		}
		sheet, err := animfile.DecodeSheet(data)
		if err != nil {
			return fmt.Errorf("sheet %s: %v", sheetKey, err)
		}
		var errs []string
		if _, findings := animfile.Diagnose(def, sheet); findings != nil {
			for _, f := range findings {
				if f.Level == animfile.LevelError {
					errs = append(errs, f.Msg)
				}
			}
		}
		if len(errs) > 0 {
			return errors.New(strings.Join(errs, "\n"))
		}
		return nil
	case strings.HasSuffix(lower, ".fx.json"):
		return t.checkEffectText(text)
	}
	return fmt.Errorf("%s cannot be written by this tool: it writes %s", key, writableKinds)
}

// checkEffectText decodes a particle effect with the engine's decoder and
// checks that every emitter's sprite and shader key names a file of that
// kind in the asset root, so a typo is refused before the write instead of
// drawing the placeholder in the game.
func (t *tools) checkEffectText(text []byte) error {
	spec, err := fxfile.Decode(text)
	if err != nil {
		return err
	}
	var errs []string
	for i, em := range spec.Emitters {
		for _, ref := range []struct{ field, kind, key string }{{"sprite", "sprite", em.Sprite}, {"shader", "shader", em.Shader}} {
			if ref.key == "" {
				continue
			}
			if msg := t.checkAssetKey(ref.kind, ref.key); msg != "" {
				errs = append(errs, fmt.Sprintf("emitters[%d].%s: %s", i, ref.field, msg))
			}
		}
	}
	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "\n"))
	}
	return nil
}

func (t *tools) writeAssetDocument(_ context.Context, a writeAssetArgs) (any, error) {
	t.docMu.Lock()
	defer t.docMu.Unlock()
	abs, key, err := t.p.AssetPath(a.Path)
	if err != nil {
		return nil, err
	}
	if len(a.Text) > maxAssetText || !utf8.ValidString(a.Text) {
		return nil, fmt.Errorf("text must be UTF-8 and at most %d bytes", maxAssetText)
	}
	text := []byte(a.Text)
	if err := t.checkAssetText(key, text); err != nil {
		return nil, fmt.Errorf("%s is not valid, so nothing was written:\n%v", key, err)
	}
	old, err := os.ReadFile(abs)
	created := errors.Is(err, os.ErrNotExist)
	if err != nil && !created {
		return nil, err
	}
	if !created {
		if err := checkExpect(key, a.Expect, old); err != nil {
			return nil, err
		}
	} else if a.Expect != "" {
		return nil, fmt.Errorf("%s does not exist, but expect was given", key)
	}
	out := map[string]any{"path": key, "created": created, "changed": created || string(old) != a.Text, "sha": sha(text)}
	if out["changed"] == true {
		var prev []byte
		if !created {
			prev = old
		}
		if err := t.writeDocument(abs, key, prev, text); err != nil {
			return nil, err
		}
		if !created {
			out["diff"] = lineDiff(old, text)
		}
	}
	return out, nil
}

func (t *tools) assetEditTools() []*mcp.Tool {
	return []*mcp.Tool{
		tool("set_asset_field", "Set an asset field",
			"Sets an asset field of a component (a sprite, clip, sound, font, shader, effect or prefab key). The key must exist in the "+
				"asset root and be a file of the kind the field takes (describe_component shows the field's x-asset). Changes only that "+
				"value in the document's file; on a prefab instance it is an override.",
			mcp.Object(objProps(map[string]*mcp.Schema{
				"component": componentArg, "field": fieldArg,
				"asset": mcp.String("asset key, e.g. chars/hero.aseprite.json#run; \"\" clears the field"),
			}), "path", "component", "field", "asset"),
			editable, t.setAssetField),
		tool("create_document", "Create a level or prefab",
			"Creates a new, valid *.level.json (no objects) or *.prefab.json (one object of the given class) under the asset root. "+
				"Refuses an existing file; missing folders are created.",
			mcp.Object(map[string]*mcp.Schema{
				"path":  mcp.String("asset key of the new file, ending in .level.json or .prefab.json"),
				"name":  mcp.String("the document's name (levels: optional; prefabs: default the file name)"),
				"class": mcp.String("prefabs only: the object type of the prefab's object, e.g. demo:Crate"),
			}, "path"),
			editable, t.createDocument),
		tool("write_asset_document", "Write an asset document",
			"Replaces the full text of a "+writableKinds+" file under the asset root (creating it if missing) after decoding and "+
				"validating it with the engine's own rules; invalid text is refused and nothing is written. expect is the sha of the "+
				"text you read; the write is refused if the file changed since. No other file kinds and no binary files.",
			mcp.Object(map[string]*mcp.Schema{
				"path":   mcp.String("asset key of the file"),
				"text":   mcp.String("the complete new text"),
				"expect": mcp.String("sha of the text you based this on (read_document returns it); omit for a new file"),
			}, "path", "text"),
			destructive, t.writeAssetDocument),
	}
}
