package goddoc

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// writeTree creates the named empty files below dir.
func writeTree(t *testing.T, dir string, files ...string) {
	t.Helper()
	for _, f := range files {
		path := filepath.Join(dir, filepath.FromSlash(f))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("{}"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestCollect(t *testing.T) {
	dir := t.TempDir()
	writeTree(t, dir,
		"zz__gen.god.json",
		"a/zz__gen.god.json",
		"a/b/zz__gen.god.json",
		"mod/go.mod", "mod/c/zz__gen.god.json", // nested module is included
		"vendor/v/zz__gen.god.json",
		"a/testdata/t/zz__gen.god.json",
		".hidden/zz__gen.god.json",
		"_skip/zz__gen.god.json",
		"node_modules/n/zz__gen.god.json",
		"empty/readme.txt",
		"other.txt",
		"god/game.god.json",
	)
	// A merged GOD that weakpixel doc wrote is skipped by walks.
	if err := os.WriteFile(filepath.Join(dir, "god", "project.god.json"), []byte(`{"generator":"weakpixel doc"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)

	tests := []struct {
		name    string
		inputs  []string
		want    []string
		wantErr string
	}{
		{"all", []string{"./..."}, []string{"a/b/zz__gen.god.json", "a/zz__gen.god.json", "god/game.god.json", "mod/c/zz__gen.god.json", "zz__gen.god.json"}, ""},
		{"specs folder", []string{"god"}, []string{"god/game.god.json"}, ""},
		{"explicit merged file is read", []string{"god/project.god.json"}, []string{"god/project.god.json"}, ""},
		{"dir pattern", []string{"a/..."}, []string{"a/b/zz__gen.god.json", "a/zz__gen.god.json"}, ""},
		{"plain directory is recursive", []string{"a"}, []string{"a/b/zz__gen.god.json", "a/zz__gen.god.json"}, ""},
		{"explicit testdata root is walked", []string{"a/testdata/..."}, []string{"a/testdata/t/zz__gen.god.json"}, ""},
		{"explicit hidden root is walked", []string{".hidden"}, []string{".hidden/zz__gen.god.json"}, ""},
		{"explicit file, any name", []string{"other.txt"}, []string{"other.txt"}, ""},
		{"duplicate via two inputs", []string{"a/...", "a/b", "./a/zz__gen.god.json"}, []string{"a/b/zz__gen.god.json", "a/zz__gen.god.json"}, ""},
		{"no match", []string{"empty/..."}, nil, "empty/...: no *.god.json files found"},
		{"missing path", []string{"nope"}, nil, "nope"},
		{"pattern on a file", []string{"other.txt/..."}, nil, "not a directory"},
		{"bad pattern", []string{"a/.../b"}, nil, `"..." is only supported at the end`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := collect(tt.inputs)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			for i := range got {
				got[i] = filepath.ToSlash(filepath.Clean(got[i]))
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("got %v, want %v", got, tt.want)
			}
		})
	}

	t.Run("errors are collected", func(t *testing.T) {
		_, err := collect([]string{"nope", "empty/...", "a/.../b"})
		if err == nil {
			t.Fatal("want error")
		}
		for _, want := range []string{"nope", "empty/...", "a/.../b"} {
			if !strings.Contains(err.Error(), want) {
				t.Errorf("error %q lacks %q", err, want)
			}
		}
	})
}
