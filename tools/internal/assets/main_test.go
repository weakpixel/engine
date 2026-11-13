package assets

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/weakpixel/engine/asset/library"
	"github.com/weakpixel/engine/tools/internal/cli"
)

// goodLib is a library without errors; badLib adds one finding of each
// kind that blocks loading.
var goodLib = map[string]string{
	"global/hero/hero.anim.json":     `{"version": 1, "sheet": "hero.aseprite.json"}`,
	"global/hero/hero.aseprite.json": `{"meta": {"image": "hero.png"}}`,
	"global/hero/hero.png":           "png",
	"forest/forest.tmj":              `{"tilesets": [{"source": "../trees/trees.tsj"}]}`,
	"trees/trees.tsj":                `{"image": "trees.png"}`,
	"trees/trees.png":                "png",
	"readme.txt":                     "hello",
}

var badLib = map[string]string{
	"cave/cave.tmj":        `{"layers": [{"image": "../forest/bg.png"}, {"image": "Rock.png"}, {"image": "gone.png"}]}`,
	"cave/rock.png":        "png",
	"forest/bg.png":        "png",
	"global/ui.sheet.json": `{"image": "../forest/bg.png"}`,
	"title/Logo.png":       "png",
}

func writeFiles(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		p := filepath.Join(dir, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

// result runs f and returns its exit code as cli.Execute maps the error (0, 1,
// or 2 for a usage error), stdout and the text of the error.
func result(f func(stdout io.Writer) error) (int, string, string) {
	var stdout bytes.Buffer
	err := f(&stdout)
	var u *cli.UsageError
	switch {
	case err == nil:
		return exitOK, stdout.String(), ""
	case errors.As(err, &u):
		return exitUsage, stdout.String(), err.Error()
	}
	return exitError, stdout.String(), err.Error()
}

// Exit codes, as cli.Execute maps nil, a plain error and a *cli.UsageError.
const (
	exitOK    = 0
	exitError = 1
	exitUsage = 2
)

func runCheck(opts CheckOptions) (int, string, string) {
	return result(func(w io.Writer) error { return Check(opts, w) })
}

func runList(opts ListOptions) (int, string, string) {
	return result(func(w io.Writer) error { return List(opts, w) })
}

func runCatalog(opts CatalogOptions) (int, string, string) {
	return result(func(w io.Writer) error { return Catalog(opts, w) })
}

func runPack(opts PackOptions) (int, string, string) {
	return result(func(w io.Writer) error { return Pack(opts, w) })
}

func runKeys(opts KeysOptions) (int, string, string) {
	return result(func(w io.Writer) error { return Keys(opts, w) })
}

func TestCheck(t *testing.T) {
	code, out, _ := runCheck(CheckOptions{Root: writeFiles(t, goodLib)})
	if code != exitOK || out != "readme.txt: warn: unused: no known kind and nothing references it (keep source files outside the library)\n0 errors, 1 warnings\n" {
		t.Fatalf("good: code %d\n%s", code, out)
	}
	code, out, _ = runCheck(CheckOptions{Root: writeFiles(t, badLib)})
	want := `cave/cave.tmj: error: reference "Rock.png": case mismatch, the file is cave/rock.png (embed, zip and HTTP are case-sensitive)
cave/cave.tmj: error: reference "gone.png": no such file cave/gone.png
global/ui.sheet.json: error: reference "../forest/bg.png": global/ uses package forest, which would stay mounted for good; move the file to global/
title/Logo.png: warn: name "Logo.png": use lower-case [a-z0-9._-] only
3 errors, 1 warnings
`
	if code != exitError || out != want {
		t.Fatalf("bad: code %d\n%s\nwant\n%s", code, out, want)
	}
}

func TestCheckJSON(t *testing.T) {
	type result struct {
		Findings []struct{ Severity, Path, Msg string }
		Errors   int
		Warnings int
		Fatal    string
	}
	decode := func(out string) result {
		t.Helper()
		var r result
		if err := json.Unmarshal([]byte(out), &r); err != nil {
			t.Fatalf("%v\n%s", err, out)
		}
		return r
	}
	root := writeFiles(t, badLib)
	code, out, _ := runCheck(CheckOptions{Root: root, JSON: true})
	r := decode(out)
	if code != exitError || r.Errors != 3 || r.Warnings != 1 || len(r.Findings) != 4 {
		t.Fatalf("code %d, %+v", code, r)
	}
	if f := r.Findings[1]; f.Severity != "error" || f.Path != "cave/cave.tmj" || !strings.Contains(f.Msg, "no such file") {
		t.Errorf("finding %+v", f)
	}
	code, out, _ = runCheck(CheckOptions{Root: writeFiles(t, goodLib), JSON: true})
	if r := decode(out); code != exitOK || r.Errors != 0 || len(r.Findings) != 1 || r.Findings[0].Severity != "warn" {
		t.Errorf("good: code %d, %+v", code, r)
	}
	// No findings: an empty list, not null.
	code, out, _ = runCheck(CheckOptions{Root: writeFiles(t, map[string]string{"global/a.png": "png"}), JSON: true})
	if code != exitOK || !strings.Contains(out, `"findings": []`) {
		t.Errorf("empty: code %d\n%s", code, out)
	}
	code, out, _ = runCheck(CheckOptions{Root: writeFiles(t, map[string]string{"library.json": "{"}), JSON: true})
	if r := decode(out); code != exitError || r.Fatal == "" {
		t.Errorf("fatal: code %d, %+v", code, r)
	}
}

func TestLs(t *testing.T) {
	code, out, _ := runList(ListOptions{Root: writeFiles(t, goodLib)})
	want := `forest: 1 files, 48 B
  entry forest/forest.tmj
  uses trees
global: 3 files, 79 B
  entry global/hero/hero.anim.json
trees: 2 files, 25 B
  entry trees/trees.tsj
`
	if code != exitOK || out != want {
		t.Fatalf("code %d\n%s\nwant\n%s", code, out, want)
	}
}

func TestCatalog(t *testing.T) {
	root := writeFiles(t, goodLib)
	if code, _, errOut := runCatalog(CatalogOptions{Root: root}); code != exitOK {
		t.Fatalf("code %d: %s", code, errOut)
	}
	data, err := os.ReadFile(filepath.Join(root, library.CatalogFile))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := library.ReadCatalog(data); err != nil {
		t.Fatal(err)
	}
	code, out, _ := runCatalog(CatalogOptions{Root: root, Out: "-"})
	if code != exitOK || !strings.Contains(out, `"name": "trees"`) {
		t.Fatalf("stdout: %d %s", code, out)
	}
	code, _, errOut := runCatalog(CatalogOptions{Root: writeFiles(t, badLib)})
	if code != exitError || !strings.Contains(errOut, "the library has errors") {
		t.Fatalf("bad: %d %s", code, errOut)
	}
}

func TestPack(t *testing.T) {
	root := writeFiles(t, goodLib)
	out := filepath.Join(t.TempDir(), "dist")
	code, stdout, errOut := runPack(PackOptions{Root: root, Out: out})
	if code != exitOK {
		t.Fatalf("code %d: %s", code, errOut)
	}
	if !strings.Contains(stdout, "forest -> packs/forest.") {
		t.Fatalf("stdout: %s", stdout)
	}
	for _, p := range []string{library.CatalogFile, "readme.txt"} {
		if _, err := os.Stat(filepath.Join(out, filepath.FromSlash(p))); err != nil {
			t.Error(err)
		}
	}
	lib, err := library.Open(os.DirFS(out))
	if err != nil {
		t.Fatal(err)
	}
	if g, _ := lib.Package("global"); g.Pack == "" {
		t.Fatal("packed catalog without packs")
	}
	if code, _, errOut := runPack(PackOptions{Root: root, Out: out}); code != exitError || !strings.Contains(errOut, "is not empty") {
		t.Fatalf("second pack into the same dir: %d %s", code, errOut)
	}
}

func TestMissingRoot(t *testing.T) {
	if code, _, errOut := runCheck(CheckOptions{Root: "/does/not/exist"}); code != exitError || !strings.Contains(errOut, "is not a directory") {
		t.Fatalf("missing root: %d %s", code, errOut)
	}
}

// TestPackDefaultOut: pack without an output folder writes build/assets and
// still refuses a folder that is not empty.
func TestPackDefaultOut(t *testing.T) {
	root := writeFiles(t, goodLib)
	t.Chdir(t.TempDir())
	if code, _, errOut := runPack(PackOptions{Root: root}); code != exitOK {
		t.Fatalf("code %d: %s", code, errOut)
	}
	if _, err := os.Stat(filepath.Join("build", "assets", library.CatalogFile)); err != nil {
		t.Error(err)
	}
	if code, _, errOut := runPack(PackOptions{Root: root}); code != exitError || !strings.Contains(errOut, "is not empty") {
		t.Fatalf("second pack: %d %s", code, errOut)
	}
}

func TestKeys(t *testing.T) {
	lib := map[string]string{
		"global/hero/hero.anim.json":     `{"version": 1, "sheet": "hero.aseprite.json"}`,
		"global/hero/hero.aseprite.json": `{"frames": [{"filename": "a", "frame": {"x":0,"y":0,"w":1,"h":1}, "duration": 100}], "meta": {"image": "hero.png", "frameTags": [{"name": "Idle L/R", "from": 0, "to": 0, "direction": "forward"}]}}`,
		"global/hero/hero.png":           "png",
		"world-1/world-1.world":          `{"maps": []}`,
		"readme.txt":                     "hello",
	}
	code, out, errOut := runKeys(KeysOptions{Root: writeFiles(t, lib), Pkg: "game"})
	want := `// Code generated by weakpixel assets keys; DO NOT EDIT.

package game

// Asset library packages (doc/guide/engine/asset-library.md).
const (
	PackageGlobal = "global"
	PackageWorld1 = "world-1"
)

// Asset keys, one per file of a known kind.
const (
	KeyGlobalHeroHeroAnim     = "global/hero/hero.anim.json"
	KeyGlobalHeroHeroAseprite = "global/hero/hero.aseprite.json"
	KeyGlobalHeroHeroPng      = "global/hero/hero.png"
	KeyWorld1World1World      = "world-1/world-1.world"
)

// Clip keys, one per tag of each clip set.
const (
	KeyGlobalHeroHeroAnimIdleLR = "global/hero/hero.anim.json#Idle L/R"
)
`
	if code != exitOK || out != want {
		t.Fatalf("code %d %s\n%s\nwant\n%s", code, errOut, out, want)
	}

	root := writeFiles(t, map[string]string{"global/a-b.png": "png", "global/a_b.png": "png"})
	code, _, errOut = runKeys(KeysOptions{Root: root})
	if code != exitError || !strings.Contains(errOut, `"global/a-b.png" and "global/a_b.png" both make the identifier KeyGlobalABPng`) {
		t.Fatalf("collision: %d %s", code, errOut)
	}
	dst := filepath.Join(t.TempDir(), "zz__gen.assets.go")
	if code, _, errOut := runKeys(KeysOptions{Root: writeFiles(t, lib), Out: dst}); code != exitOK {
		t.Fatalf("-o: %d %s", code, errOut)
	}
	if data, _ := os.ReadFile(dst); !strings.Contains(string(data), "package assets") {
		t.Fatalf("default package: %s", data)
	}
}

// TestRootFromProject: without <root> the library is assets.root of the
// nearest .weakpixel.json.
func TestRootFromProject(t *testing.T) {
	project := writeFiles(t, map[string]string{".weakpixel.json": `{"assets": {"root": "lib"}}`})
	lib := filepath.Join(project, "lib")
	for name, content := range goodLib {
		p := filepath.Join(lib, filepath.FromSlash(name))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	t.Chdir(lib)
	code, out, errOut := runCheck(CheckOptions{})
	if code != exitOK || !strings.HasSuffix(out, "0 errors, 1 warnings\n") {
		t.Fatalf("check: code %d\n%s%s", code, out, errOut)
	}
	if code, out, _ := runCatalog(CatalogOptions{Out: "-"}); code != exitOK || !strings.Contains(out, "global") {
		t.Fatalf("catalog: code %d\n%s", code, out)
	}
	t.Chdir(t.TempDir())
	if code, _, errOut := runCheck(CheckOptions{}); code != exitUsage || !strings.Contains(errOut, "no <root> given") {
		t.Fatalf("no project: code %d %s", code, errOut)
	}
}
