package mcpserver

import (
	"fmt"
	"maps"
	"os"
	"slices"
	"strconv"
	"strings"

	"github.com/weakpixel/engine/asset/library"
	"github.com/weakpixel/engine/god"
)

// walkSchema calls fn for every value in v (decoded JSON) that s marks as an
// entity reference or an asset key, with its $-path; partial values (an
// override) are fine, fields v does not have are not visited.
func walkSchema(s *god.TypeSchema, path string, v any, fn func(path string, s *god.TypeSchema, v any)) {
	if s == nil || v == nil && s.Format != god.FormatEntity {
		return
	}
	if s.Format == god.FormatEntity || s.Format == "asset" {
		fn(path, s, v)
		return
	}
	switch val := v.(type) {
	case map[string]any:
		for _, k := range slices.Sorted(maps.Keys(val)) {
			sub := s.Properties[k]
			if sub == nil {
				sub = s.AdditionalProperties
			}
			walkSchema(sub, path+"."+k, val[k], fn)
		}
	case []any:
		for i, item := range val {
			walkSchema(s.Items, path+"["+strconv.Itoa(i)+"]", item, fn)
		}
	}
}

// fieldKinds maps the x-asset kind of a field to the kinds of file it takes
// (the resolver's kinds, resolver). A sprite may be an atlas frame or
// a frame of an animation sheet.
var fieldKinds = map[string][]library.Kind{
	"sprite":   {library.KindTexture, library.KindAtlas, library.KindClipSet},
	"clip":     {library.KindClipSet},
	"font":     {library.KindFont},
	"shader":   {library.KindShader},
	"sound":    {library.KindSound},
	"music":    {library.KindMusic},
	"prefab":   {library.KindPrefab},
	"effect":   {library.KindEffect},
	"voice":    {library.KindVoice},
	"dialogue": {library.KindDialogue},
}

// starterVoice reports whether key, the value of a field that takes want, is
// a starter voice name ("chirpy") rather than a file: speech loads a
// voice key without the .voice.json suffix and without a folder as a starter,
// the rule the library's dialogue references follow too. The server does not
// link the voice library, so an unknown name is reported when the level
// loads, not here.
func starterVoice(want, key string) bool {
	return want == "voice" && !strings.Contains(key, "/") && !strings.HasSuffix(strings.ToLower(key), ".json")
}

// kindName names a file kind for messages.
func kindName(k library.Kind) string {
	if k == library.KindUnknown {
		return "file of no known kind"
	}
	return string(k)
}

// checkAssetKey checks the asset key of a field that takes want ("sprite",
// ...): the file must exist in the asset root and be of a kind the field
// takes. A key may end in "#sub" (a clip, a frame or a font size); a voice
// field may name a starter voice (see starterVoice). It returns "" when the
// key is fine.
func (t *tools) checkAssetKey(want, key string) string {
	if starterVoice(want, key) {
		return ""
	}
	file, _, _ := strings.Cut(key, "#")
	abs, clean, err := t.p.AssetPath(file)
	if err != nil {
		return fmt.Sprintf("asset %q: %v", key, err)
	}
	if fi, err := os.Stat(abs); err != nil || fi.IsDir() {
		return fmt.Sprintf("asset %q does not exist in the asset root (list_assets lists the keys)", key)
	}
	kinds, known := fieldKinds[want]
	if !known {
		return ""
	}
	if got := library.KindOf(clean); !slices.Contains(kinds, got) {
		return fmt.Sprintf("asset %q is a %s, the field takes a %s", key, kindName(got), want)
	}
	return ""
}

// checkReferences checks the entity references and asset keys in the
// components of obj as written: a reference names an object of the
// document (ids), an asset key a file of the kind the field takes.
func (t *tools) checkReferences(g *godData, obj docObject, ids map[string]bool) []string {
	var problems []string
	for _, k := range slices.Sorted(maps.Keys(obj.Components)) {
		c := g.compByKey[k]
		if c == nil || c.Schema == nil {
			continue
		}
		v, err := decodeJSON(obj.Components[k])
		if err != nil {
			continue
		}
		walkSchema(c.Schema, "$", v, func(path string, s *god.TypeSchema, v any) {
			str, ok := v.(string)
			if !ok || str == "" {
				return
			}
			var msg string
			if s.Format == god.FormatEntity {
				if !ids[str] {
					msg = fmt.Sprintf("references object %q, which is not in this document", str)
				}
			} else {
				msg = t.checkAssetKey(s.XAsset, str)
			}
			if msg != "" {
				problems = append(problems, fmt.Sprintf("component %s: %s: %s", k, path, msg))
			}
		})
	}
	return problems
}
