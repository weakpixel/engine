// Package project loads .weakpixel.json, the project settings file that gives
// the tools (`weakpixel gen`, `weakpixel doc`, later the editor) their defaults. The
// format is described in doc/guide/project-file.md.
package project

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
)

// FileName is the name of the settings file.
const FileName = ".weakpixel.json"

// Settings is the content of a .weakpixel.json. Paths are absolute: Load
// resolves them against the directory of the file. A path that the file
// leaves out is "".
type Settings struct {
	// Dir is the directory holding the settings file (the project root).
	Dir string

	// GODSpecs is the folder go generate writes every package's GOD into
	// ("god.specs"), and the default input of god-doc. "" when the file
	// leaves it out: [DefaultSpecs] is the default then.
	GODSpecs string
	// GODOut is where god-doc -format god writes the merged project GOD
	// ("god.out").
	GODOut string
	// GODDocOut is god-doc's default output directory ("godDoc.out").
	GODDocOut string

	// SchemaOut is the folder `weakpixel schema` writes the JSON schemas
	// into ("schema.out").
	SchemaOut string

	// AssetsRoot is the asset library root ("assets.root").
	AssetsRoot string
	// AssetsPrefabs is the folder of new prefabs under AssetsRoot, a
	// slash-separated relative path ("assets.prefabs").
	AssetsPrefabs string

	// EditorGame is the directory of the package the editor builds
	// ("editor.game").
	EditorGame string
	// EditorTags are the build tags of the editor's wasm build
	// ("editor.tags"); nil when the file leaves them out.
	EditorTags []string
	// EditorSnap is the drag grid in world units ("editor.snap"), 0 for off.
	EditorSnap float64
	// EditorPrefabBackground is the prefab stage's background colour as
	// "#rrggbb" ("editor.prefab.background"), "" for the default.
	EditorPrefabBackground string
	// EditorPrefabGrid is the prefab stage's grid step in world units
	// ("editor.prefab.grid", 0: no grid); nil for the default.
	EditorPrefabGrid *float64
}

// file is the JSON layout of the settings file.
type file struct {
	GOD *struct {
		Specs string `json:"specs"`
		Out   string `json:"out"`
	} `json:"god"`
	GODDoc *struct {
		Out string `json:"out"`
	} `json:"godDoc"`
	Schema *struct {
		Out string `json:"out"`
	} `json:"schema"`
	Assets *struct {
		Root    string `json:"root"`
		Prefabs string `json:"prefabs"`
	} `json:"assets"`
	Editor *struct {
		Game   string   `json:"game"`
		Tags   []string `json:"tags"`
		Snap   float64  `json:"snap"`
		Prefab *struct {
			Background string   `json:"background"`
			Grid       *float64 `json:"grid"`
		} `json:"prefab"`
	} `json:"editor"`
}

// Load finds the settings file by walking up from dir to the first
// directory that has one, and reads it. The walk stops at the root of the Go
// module holding dir (the first directory with a go.mod), so a nested module
// never picks up the settings of an enclosing one and, for example, writes
// its GOD files into another module's specs folder. Without a file it returns
// nil settings, an empty path and no error. Unknown keys and other problems
// with the file are all reported in one error that names the file.
func Load(dir string) (*Settings, string, error) {
	dir, err := filepath.Abs(dir)
	if err != nil {
		return nil, "", err
	}
	for {
		path := filepath.Join(dir, FileName)
		data, err := os.ReadFile(path)
		switch {
		case err == nil:
			s, err := parse(data, dir)
			if err != nil {
				return nil, path, fmt.Errorf("%s: %w", path, err)
			}
			return s, path, nil
		case !errors.Is(err, fs.ErrNotExist):
			return nil, path, err
		}
		parent := filepath.Dir(dir)
		if parent == dir || isModuleRoot(dir) {
			return nil, "", nil
		}
		dir = parent
	}
}

// DefaultGODSpecs is the folder, relative to the module root, that holds the
// GOD files when no file or no "god.specs" key sets one. It is build output
// (ignored by git).
const DefaultGODSpecs = "build/god"

// DefaultSpecs returns the default GOD specs folder for the project around
// dir: build/god below the module holding dir (below dir itself when no
// go.mod is found). Settings.GODSpecs, when set, wins over it.
func DefaultSpecs(dir string) string {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return filepath.FromSlash(DefaultGODSpecs)
	}
	for d := abs; ; {
		if isModuleRoot(d) {
			return filepath.Join(d, filepath.FromSlash(DefaultGODSpecs))
		}
		parent := filepath.Dir(d)
		if parent == d {
			return filepath.Join(abs, filepath.FromSlash(DefaultGODSpecs))
		}
		d = parent
	}
}

// isModuleRoot reports whether dir holds a go.mod.
func isModuleRoot(dir string) bool {
	_, err := os.Stat(filepath.Join(dir, "go.mod"))
	return err == nil
}

// parse decodes a settings file found in dir.
func parse(data []byte, dir string) (*Settings, error) {
	if err := checkKeys(data); err != nil {
		return nil, err
	}
	var f file
	if err := json.Unmarshal(data, &f); err != nil {
		return nil, err
	}
	s := &Settings{Dir: dir}
	abs := func(p string) string {
		if p == "" || filepath.IsAbs(p) {
			return p
		}
		return filepath.Join(dir, filepath.FromSlash(p))
	}
	if f.GOD != nil {
		s.GODSpecs = abs(f.GOD.Specs)
		s.GODOut = abs(f.GOD.Out)
	}
	if f.GODDoc != nil {
		s.GODDocOut = abs(f.GODDoc.Out)
	}
	if f.Schema != nil {
		s.SchemaOut = abs(f.Schema.Out)
	}
	var errs []error
	if a := f.Assets; a != nil {
		s.AssetsRoot = abs(a.Root)
		s.AssetsPrefabs = path.Clean(filepath.ToSlash(a.Prefabs))
		if a.Prefabs == "" {
			s.AssetsPrefabs = ""
		} else if path.IsAbs(s.AssetsPrefabs) || s.AssetsPrefabs == ".." || strings.HasPrefix(s.AssetsPrefabs, "../") {
			errs = append(errs, fmt.Errorf("assets.prefabs: %q must be a folder inside assets.root", a.Prefabs))
		}
	}
	if e := f.Editor; e != nil {
		s.EditorGame = abs(e.Game)
		s.EditorTags = e.Tags
		s.EditorSnap = e.Snap
		if e.Snap < 0 {
			errs = append(errs, fmt.Errorf("editor.snap: %v is negative", e.Snap))
		}
		if p := e.Prefab; p != nil {
			s.EditorPrefabBackground, s.EditorPrefabGrid = p.Background, p.Grid
			if p.Background != "" && !hexColor.MatchString(p.Background) {
				errs = append(errs, fmt.Errorf("editor.prefab.background: %q is not a #rrggbb colour", p.Background))
			}
			if p.Grid != nil && *p.Grid < 0 {
				errs = append(errs, fmt.Errorf("editor.prefab.grid: %v is negative", *p.Grid))
			}
		}
	}
	if err := errors.Join(errs...); err != nil {
		return nil, err
	}
	return s, nil
}

// hexColor is the colour form of editor.prefab.background.
var hexColor = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)

// known lists the allowed keys per object path ("" is the top level). A
// key whose own path is a key of known holds an object.
var known = map[string][]string{
	"":              {"god", "godDoc", "schema", "assets", "editor"},
	"god":           {"specs", "out"},
	"godDoc":        {"out"},
	"schema":        {"out"},
	"assets":        {"root", "prefabs"},
	"editor":        {"game", "tags", "snap", "prefab"},
	"editor.prefab": {"background", "grid"},
}

// checkKeys reports every unknown key of data at once, so a typo never
// passes silently. json.Decoder.DisallowUnknownFields would stop at the
// first one.
func checkKeys(data []byte) error {
	var top map[string]json.RawMessage
	dec := json.NewDecoder(bytes.NewReader(data))
	if err := dec.Decode(&top); err != nil {
		return err
	}
	return errors.Join(checkObject("", top)...)
}

// checkObject checks the keys of the object at path, and those of its
// nested objects.
func checkObject(path string, obj map[string]json.RawMessage) []error {
	var errs []error
	for _, key := range slices.Sorted(maps.Keys(obj)) {
		full := key
		if path != "" {
			full = path + "." + key
		}
		if !slices.Contains(known[path], key) {
			errs = append(errs, fmt.Errorf("unknown key %q", full))
			continue
		}
		if _, isObject := known[full]; !isObject {
			continue
		}
		var sub map[string]json.RawMessage
		if err := json.Unmarshal(obj[key], &sub); err != nil {
			errs = append(errs, fmt.Errorf("%s: want an object", full))
			continue
		}
		errs = append(errs, checkObject(full, sub)...)
	}
	return errs
}

// Keys lists the allowed keys per object path ("" is the top level), for
// tools that describe the file, such as `weakpixel schema`.
func Keys() map[string][]string { return maps.Clone(known) }
