package mcpserver

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func TestProject(t *testing.T) {
	p, err := OpenProject(ProjectOptions{Dir: filepath.Join(fixture, "assets", "a")})
	if err != nil {
		t.Fatal(err)
	}
	abs, _ := filepath.Abs(fixture)
	if p.Root != abs {
		t.Errorf("Root = %s, want %s", p.Root, abs)
	}
	if want, _ := filepath.EvalSymlinks(filepath.Join(abs, "assets")); p.AssetRoot != want {
		t.Errorf("AssetRoot = %s, want %s", p.AssetRoot, want)
	}
	if len(p.GODInputs) != 2 || p.GODInputs[0] != filepath.Join(abs, "god") || filepath.Base(p.GODInputs[1]) != "godspecs" {
		t.Errorf("GODInputs = %v, want the specs folder and the engine's godspecs", p.GODInputs)
	}
	if p.Game != abs || !slices.Equal(p.Tags, []string{"editor"}) {
		t.Errorf("Game, Tags = %s, %v", p.Game, p.Tags)
	}
	g := p.GOD()
	if g.err != nil {
		t.Fatal(g.err)
	}
	if g.objByKey["demo:Crate"] == nil || g.compByKey["gfx:Transform"] == nil || g.compByKey["collision:Collider"] == nil {
		t.Error("the merged GOD lacks the fixture's or the engine's entries")
	}
	if p.GOD() != g {
		t.Error("GOD reloaded without a change")
	}
}

func TestProjectReloadsGOD(t *testing.T) {
	dir := copyFixture(t)
	p, err := OpenProject(ProjectOptions{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	if p.GOD().objByKey["demo:Crate"] == nil {
		t.Fatal("no demo:Crate")
	}
	file := filepath.Join(dir, "god", "demo.god.json")
	data, _ := os.ReadFile(file)
	if err := os.WriteFile(file, []byte(strings.ReplaceAll(string(data), `"Crate"`, `"Box"`)), 0o644); err != nil {
		t.Fatal(err)
	}
	if g := p.GOD(); g.objByKey["demo:Box"] == nil {
		t.Errorf("GOD not reloaded after a change: %v", g.err)
	}
}

func TestProjectDefaults(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "game", "global"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "go.mod"), []byte("module example.com/g\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	p, err := OpenProject(ProjectOptions{Dir: filepath.Join(dir, "game"), Tags: []string{"editor", "x"}})
	if err != nil {
		t.Fatal(err)
	}
	if real, _ := filepath.EvalSymlinks(dir); p.Root != dir && p.Root != real {
		t.Errorf("Root = %s, want the module root %s", p.Root, dir)
	}
	if filepath.Base(p.AssetRoot) != "game" {
		t.Errorf("AssetRoot = %s, want the parent of global/", p.AssetRoot)
	}
	if !slices.Equal(p.Tags, []string{"editor", "x"}) {
		t.Errorf("Tags = %v", p.Tags)
	}
	if len(p.Warnings) == 0 || !strings.Contains(p.Warnings[0], "no .weakpixel.json") {
		t.Errorf("Warnings = %v", p.Warnings)
	}
}

func TestProjectPathGuard(t *testing.T) {
	dir := copyFixture(t)
	outside := t.TempDir()
	assets := filepath.Join(dir, "assets")
	if err := os.Symlink(outside, filepath.Join(assets, "escape")); err != nil {
		t.Skipf("no symbolic links: %v", err)
	}
	if err := os.Symlink(filepath.Join(assets, "a"), filepath.Join(assets, "inside")); err != nil {
		t.Fatal(err)
	}
	p, err := OpenProject(ProjectOptions{Dir: dir})
	if err != nil {
		t.Fatal(err)
	}
	ok := []struct{ in, key string }{
		{"a/a.level.json", "a/a.level.json"},
		{"./a/../a/a.level.json", "a/a.level.json"},
		{filepath.Join(p.AssetRoot, "levels", "new.level.json"), "levels/new.level.json"},
		{"inside/a.level.json", "inside/a.level.json"},
		{"levels/missing/dir/x.level.json", "levels/missing/dir/x.level.json"},
	}
	for _, tc := range ok {
		if _, key, err := p.AssetPath(tc.in); err != nil || key != tc.key {
			t.Errorf("AssetPath(%q) = %q, %v; want %q", tc.in, key, err, tc.key)
		}
	}
	bad := []struct{ in, want string }{
		{"../x", "outside the asset root"},
		{"levels/../../x", "outside the asset root"},
		{"/etc/passwd", "outside the asset root"},
		{filepath.Join(dir, "god", "demo.god.json"), "outside the asset root"},
		{"escape/x.level.json", "symbolic link"},
		{"escape", "symbolic link"},
		{"", "empty path"},
		{".", "outside the asset root"},
	}
	for _, tc := range bad {
		_, _, err := p.AssetPath(tc.in)
		if err == nil {
			t.Errorf("AssetPath(%q) passed", tc.in)
			continue
		}
		mustContain(t, err.Error(), tc.want)
	}
}
