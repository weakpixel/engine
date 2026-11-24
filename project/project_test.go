package project

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoad(t *testing.T) {
	tests := []struct {
		name    string
		file    string // content of root/.weakpixel.json; "" means no file
		start   string // directory below root to load from
		want    Settings
		wantErr []string // substrings of the error
	}{
		{
			name:  "no file",
			start: "a/b",
		},
		{
			name:  "relative paths resolve against the file",
			file:  `{"god": {"specs": "god", "out": "god/project.god.json"}, "godDoc": {"out": "build/doc"}}`,
			start: "a/b",
			want:  Settings{GODSpecs: "god", GODOut: "god/project.god.json", GODDocOut: "build/doc"},
		},
		{
			name:  "missing sections leave paths empty",
			file:  `{"god": {"specs": "specs"}}`,
			start: ".",
			want:  Settings{GODSpecs: "specs"},
		},
		{
			name:    "unknown keys are all reported",
			file:    `{"gods": {}, "god": {"spec": "x", "outt": "y"}, "godDoc": {"out": "d"}}`,
			start:   ".",
			wantErr: []string{`unknown key "god.outt"`, `unknown key "god.spec"`, `unknown key "gods"`},
		},
		{
			name:  "assets and editor sections",
			file:  `{"assets": {"root": "assets", "prefabs": "global/prefabs"}, "editor": {"game": "cmd/game", "tags": ["editor", "dev"], "snap": 8, "screenshots": "shots", "prefab": {"background": "#14121a", "grid": 0}}}`,
			start: "a",
			want: Settings{
				AssetsRoot: "assets", AssetsPrefabs: "global/prefabs",
				EditorGame: "cmd/game", EditorTags: []string{"editor", "dev"}, EditorSnap: 8, EditorScreenshots: "shots",
				EditorPrefabBackground: "#14121a", EditorPrefabGrid: new(float64),
			},
		},
		{
			name:  "editor rates",
			file:  `{"editor": {"rate": {"idle": 5, "hidden": 0}}}`,
			start: ".",
			want:  Settings{EditorRate: EditorRate{Idle: new(5), Hidden: new(0)}},
		},
		{
			name:    "nested unknown keys are reported with their path",
			file:    `{"editor": {"prefab": {"colour": "#fff", "grid": 16}, "gam": "x"}, "assets": {"roots": "a"}}`,
			start:   ".",
			wantErr: []string{`unknown key "assets.roots"`, `unknown key "editor.gam"`, `unknown key "editor.prefab.colour"`},
		},
		{
			name:    "nested section is not an object",
			file:    `{"editor": {"prefab": 3}}`,
			start:   ".",
			wantErr: []string{"editor.prefab: want an object"},
		},
		{
			name:    "bad values are all reported",
			file:    `{"assets": {"prefabs": "../x"}, "editor": {"snap": -1, "prefab": {"background": "red", "grid": -2}, "rate": {"idle": -1, "active": 0, "hidden": -2}}}`,
			start:   ".",
			wantErr: []string{`assets.prefabs: "../x" must be a folder inside assets.root`, "editor.snap: -1 is negative", `editor.prefab.background: "red" is not a #rrggbb colour`, "editor.prefab.grid: -2 is negative", "editor.rate.idle: -1 is below 0", "editor.rate.active: 0 is below 1", "editor.rate.hidden: -2 is below 0"},
		},
		{
			name:    "bad JSON",
			file:    `{"god": `,
			start:   ".",
			wantErr: []string{".weakpixel.json: unexpected EOF"},
		},
		{
			name:    "section is not an object",
			file:    `{"god": "x"}`,
			start:   ".",
			wantErr: []string{"god: want an object"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			if tc.file != "" {
				writeFile(t, filepath.Join(root, FileName), tc.file)
			}
			start := filepath.Join(root, tc.start)
			if err := os.MkdirAll(start, 0o755); err != nil {
				t.Fatal(err)
			}
			s, path, err := Load(start)
			if tc.wantErr != nil {
				if err == nil {
					t.Fatal("expected an error")
				}
				for _, want := range tc.wantErr {
					if !strings.Contains(err.Error(), want) {
						t.Errorf("error %q does not contain %q", err, want)
					}
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if tc.file == "" {
				if s != nil || path != "" {
					t.Fatalf("Load() = %+v, %q; want nil, \"\"", s, path)
				}
				return
			}
			if want := filepath.Join(root, FileName); path != want {
				t.Errorf("path = %q, want %q", path, want)
			}
			abs := func(p string) string {
				if p == "" {
					return ""
				}
				return filepath.Join(root, p)
			}
			want := tc.want
			want.Dir = root
			want.GODSpecs, want.GODOut, want.GODDocOut = abs(want.GODSpecs), abs(want.GODOut), abs(want.GODDocOut)
			want.AssetsRoot, want.EditorGame = abs(want.AssetsRoot), abs(want.EditorGame)
			want.EditorScreenshots = abs(want.EditorScreenshots)
			if !reflect.DeepEqual(*s, want) {
				t.Errorf("Load() = %+v, want %+v", *s, want)
			}
		})
	}
}

// TestLoadStopsAtModuleRoot checks that a nested Go module does not see the
// settings file of an enclosing module.
func TestLoadStopsAtModuleRoot(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, FileName), `{"god": {"specs": "god"}}`)
	writeFile(t, filepath.Join(root, "nested", "go.mod"), "module nested\n")
	if err := os.MkdirAll(filepath.Join(root, "nested", "pkg"), 0o755); err != nil {
		t.Fatal(err)
	}
	s, path, err := Load(filepath.Join(root, "nested", "pkg"))
	if err != nil || s != nil || path != "" {
		t.Errorf("Load = %v, %q, %v; want nil settings, no path, no error", s, path, err)
	}
	writeFile(t, filepath.Join(root, "nested", FileName), `{"god": {"specs": "own"}}`)
	s, _, err = Load(filepath.Join(root, "nested", "pkg"))
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(root, "nested", "own"); s.GODSpecs != want {
		t.Errorf("GODSpecs = %q, want %q", s.GODSpecs, want)
	}
}

// TestLoadNearest checks that the walk stops at the nearest file.
func TestLoadNearest(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, FileName), `{"god": {"specs": "outer"}}`)
	writeFile(t, filepath.Join(root, "sub", FileName), `{"god": {"specs": "inner"}}`)
	if err := os.MkdirAll(filepath.Join(root, "sub", "pkg"), 0o755); err != nil {
		t.Fatal(err)
	}
	s, _, err := Load(filepath.Join(root, "sub", "pkg"))
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(root, "sub", "inner"); s.GODSpecs != want {
		t.Errorf("GODSpecs = %q, want %q", s.GODSpecs, want)
	}
}

// TestDefaultSpecs checks the folder of the GOD files without god.specs:
// build/god below the module root, whatever the directory inside the module.
func TestDefaultSpecs(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "go.mod"), "module m\n")
	if err := os.MkdirAll(filepath.Join(root, "pkg", "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(root, "build", "god")
	for _, dir := range []string{root, filepath.Join(root, "pkg", "sub")} {
		if got := DefaultSpecs(dir); got != want {
			t.Errorf("DefaultSpecs(%s) = %s, want %s", dir, got, want)
		}
	}
	// A settings file without god.specs leaves GODSpecs empty: the default is
	// DefaultSpecs, so a tool can tell "chosen" from "default".
	writeFile(t, filepath.Join(root, FileName), `{"assets": {"root": "assets"}}`)
	s, _, err := Load(root)
	if err != nil || s.GODSpecs != "" {
		t.Errorf("Load = %+v, %v; want GODSpecs empty", s, err)
	}
}
