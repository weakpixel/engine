package library

import (
	"encoding/json"
	"io/fs"
	"path"
	"slices"
	"strings"
)

// ref is one file a file names.
type ref struct {
	// Path is the referenced file, relative to the library root, without
	// "#sub". Empty if Raw leaves the root.
	Path string
	// Raw is the text as the file wrote it.
	Raw string
}

// relRefKeys are the JSON object keys whose string values name files
// relative to the file they are in: Tiled maps, tilesets and worlds
// ("source", "image", "fileName"), grid sheets, TexturePacker and Aseprite
// exports ("image", under "meta" for the last two) and animation definitions
// ("sheet"). These are exactly the dependencies the engine's loaders
// declare, so one generic walk covers every format.
var relRefKeys = map[string]bool{"source": true, "image": true, "fileName": true, "sheet": true}

// jsonFile reports whether p is a JSON document the scanner reads.
func jsonFile(p string) bool {
	lower := strings.ToLower(p)
	for _, ext := range []string{".json", ".tmj", ".tsj", ".world"} {
		if strings.HasSuffix(lower, ext) {
			return true
		}
	}
	return false
}

// scanRefs returns the files that the file at p (contents data) refers to.
// Asset files that are not JSON have none. GOD documents (*.level.json,
// *.prefab.json) and effects (*.fx.json) name assets by key, relative to the root, in any string
// field: every string whose path part has a known kind counts. Voices and
// dialogues name their samples, language and voices relative to themselves
// (speechRef). Other JSON files name files relative to their own directory
// under relRefKeys.
// A file that is not valid JSON has no references; its loader reports it.
func scanRefs(p string, data []byte) []ref {
	if !jsonFile(p) {
		return nil
	}
	var doc any
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil
	}
	var out []ref
	seen := map[string]bool{}
	add := func(raw, joined string) {
		r := ref{Raw: raw}
		if clean := path.Clean(joined); fs.ValidPath(clean) && !strings.HasPrefix(raw, "/") {
			r.Path = clean
		}
		if !seen[r.Raw+"\x00"+r.Path] {
			seen[r.Raw+"\x00"+r.Path] = true
			out = append(out, r)
		}
	}
	if k := KindOf(p); k == KindLevel || k == KindPrefab || k == KindEffect {
		walkStrings(doc, "", func(_, s string) {
			key, _, _ := strings.Cut(s, "#")
			if KindOf(key) != KindUnknown || strings.HasSuffix(strings.ToLower(key), ".json") {
				add(s, key)
			}
		})
		return sortRefs(out)
	}
	dir := path.Dir(p)
	if k := KindOf(p); k == KindVoice || k == KindDialogue {
		walkStrings(doc, "", func(field, s string) {
			if key, ok := speechRef(k, field, s); ok {
				add(s, path.Join(dir, key))
			}
		})
		return sortRefs(out)
	}
	walkStrings(doc, "", func(field, s string) {
		if relRefKeys[field] && s != "" {
			key, _, _ := strings.Cut(s, "#")
			add(s, path.Join(dir, key))
		}
	})
	return sortRefs(out)
}

// sortRefs orders refs by their text: JSON objects decode into maps, whose
// order is random, and findings must be stable.
func sortRefs(refs []ref) []ref {
	slices.SortFunc(refs, func(a, b ref) int { return strings.Compare(a.Raw, b.Raw) })
	return refs
}

// walkStrings calls fn for every string in v with the name of the object
// field holding it ("" inside arrays at the top level).
func walkStrings(v any, field string, fn func(field, s string)) {
	switch v := v.(type) {
	case string:
		fn(field, v)
	case []any:
		for _, e := range v {
			walkStrings(e, field, fn)
		}
	case map[string]any:
		for k, e := range v {
			walkStrings(e, k, fn)
		}
	}
}

// speechRef returns the file, relative to the file it is in, that the field
// of a voice or dialogue file names: a voice's instrument "sample" and its
// "language" (a *.lang.json path, or a name that means <name>.lang.json next
// to the voice, as speech resolves it), and a dialogue speaker's
// "voice" when it is a *.voice.json file (not a starter voice).
func speechRef(k Kind, field, s string) (string, bool) {
	if s == "" {
		return "", false
	}
	switch {
	case k == KindVoice && field == "sample":
		return s, true
	case k == KindVoice && field == "language":
		if !strings.HasSuffix(s, ".lang.json") {
			s += ".lang.json"
		}
		return s, true
	case k == KindDialogue && field == "voice" && strings.HasSuffix(s, ".voice.json"):
		return s, true
	}
	return "", false
}
