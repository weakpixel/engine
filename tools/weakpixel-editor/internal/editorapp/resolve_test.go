package editorapp

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func write(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestResolve(t *testing.T) {
	tmp := t.TempDir()
	// A module with a settings file: assets live under assets/.
	write(t, filepath.Join(tmp, "game", "go.mod"), "module x\n")
	write(t, filepath.Join(tmp, "game", ".weakpixel.json"), `{"assets":{"root":"assets"}}`)
	write(t, filepath.Join(tmp, "game", "assets", "art", "hero.png"), "x")
	write(t, filepath.Join(tmp, "game", "notes.png"), "x")
	// A folder without go.mod or settings.
	write(t, filepath.Join(tmp, "loose", "a.png"), "x")

	tests := []struct {
		name, file, wantRoot, wantKey, wantErr string
	}{
		{"settings file", "game/assets/art/hero.png", "game/assets", "art/hero.png", ""},
		{"no settings file", "loose/a.png", "loose", "a.png", ""},
		{"outside the root", "game/notes.png", "", "", "outside the asset root"},
		{"missing file", "loose/nope.png", "", "", "no such file"},
		{"a folder", "loose", "", "", "is a folder"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, root, key, err := Resolve(filepath.Join(tmp, tt.file), false)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("err = %v, want %q", err, tt.wantErr)
				}
				if tt.name == "outside the root" && !strings.Contains(err.Error(), "notes.png") {
					t.Errorf("err = %v, want it to name the file and the root", err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if want := filepath.Join(tmp, tt.wantRoot); root != want || key != tt.wantKey {
				t.Errorf("root, key = %s, %s; want %s, %s", root, key, want, tt.wantKey)
			}
		})
	}
}

func TestResolveMissingFile(t *testing.T) {
	tmp := t.TempDir()
	write(t, filepath.Join(tmp, "chars", "hero.json"), "{}")
	_, root, key, err := Resolve(filepath.Join(tmp, "chars", "hero.anim.json"), true)
	if err != nil || root != filepath.Join(tmp, "chars") || key != "hero.anim.json" {
		t.Errorf("resolve missing = %s, %s, %v", root, key, err)
	}
	if _, _, _, err := Resolve(filepath.Join(tmp, "chars", "hero.anim.json"), false); err == nil {
		t.Error("a missing file without allowMissing resolved")
	}
	_, _, _, err = Resolve(filepath.Join(tmp, "nodir", "x.anim.json"), true)
	if err == nil || !strings.Contains(err.Error(), "folder") {
		t.Errorf("missing folder: %v", err)
	}
}
