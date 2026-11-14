package newgame

import (
	"bytes"
	"crypto/sha256"
	"flag"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "rewrite testdata/golden.txt")

// repoRoot is the engine checkout this package is in.
func repoRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.Abs("../../..")
	if err != nil {
		t.Fatal(err)
	}
	return root
}

// TestResolve: defaults come from the directory, and every problem of the
// options is reported at once.
func TestResolve(t *testing.T) {
	root := repoRoot(t)
	o, err := resolve(Options{Dir: "games/My-Game", Replace: root})
	if err != nil {
		t.Fatal(err)
	}
	if o.Module != "My-Game" || o.Name != "my_game" || o.title != "My-Game" || o.Dir != filepath.Join("games", "My-Game") {
		t.Errorf("defaults: %+v", o)
	}
	if o.Replace != root || o.version != replaceVersion {
		t.Errorf("replace %q, version %q", o.Replace, o.version)
	}
	o, err = resolve(Options{Dir: "x", Replace: root, Module: "example.com/me/shooter", Name: "shooter"})
	if err != nil {
		t.Fatal(err)
	}
	if o.Module != "example.com/me/shooter" || o.Name != "shooter" {
		t.Errorf("flags: %+v", o)
	}

	for _, tc := range []struct {
		name string
		opts Options
		want []string
	}{
		{"all at once", Options{Dir: "x", Replace: root, Module: "a b", Name: "Bad"},
			[]string{`module path "a b": bad element "a b"`, `group "Bad": want [a-z][a-z0-9_]*`}},
		{"derived group", Options{Dir: "1st", Replace: root}, []string{`group "1st"`}},
		{"engine group", Options{Dir: "gfx", Replace: root}, []string{`group "gfx" is the engine's own`}},
		{"module dots", Options{Dir: "x", Replace: root, Module: "a/../b"}, []string{`bad element ".."`}},
		{"replace without go.mod", Options{Dir: "x", Replace: t.TempDir()}, []string{"no go.mod there"}},
		// A test binary has no engine version.
		{"no version", Options{Dir: "x"}, []string{"the engine version is unknown"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			_, err := resolve(tc.opts)
			if err == nil {
				t.Fatal("no error")
			}
			for _, w := range tc.want {
				if !strings.Contains(err.Error(), w) {
					t.Errorf("error %q lacks %q", err, w)
				}
			}
		})
	}
}

// goldenOptions is the project the golden file shows.
var goldenOptions = Options{
	Module:       "example.com/mygame",
	Name:         "mygame",
	title:        "mygame",
	version:      "v0.1.2",
	ecsVersion:   "v0.0.1",
	assetVersion: "v0.0.1",
}

// TestGolden compares the rendered project with testdata/golden.txt: every
// file as "-- path --" and its text, binary files as their SHA-256.
// go test -run TestGolden -update rewrites it.
func TestGolden(t *testing.T) {
	files, err := render(goldenOptions)
	if err != nil {
		t.Fatal(err)
	}
	var b strings.Builder
	for _, name := range slices.Sorted(maps.Keys(files)) {
		data := files[name]
		if strings.HasSuffix(name, ".png") {
			data = fmt.Appendf(nil, "sha256 %x\n", sha256.Sum256(data))
		}
		fmt.Fprintf(&b, "-- %s --\n%s", name, data)
		if len(data) > 0 && data[len(data)-1] != '\n' {
			b.WriteString("\n")
		}
	}
	const golden = "testdata/golden.txt"
	if *update {
		if err := os.WriteFile(golden, []byte(b.String()), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	want, err := os.ReadFile(golden)
	if err != nil {
		t.Fatal(err)
	}
	if got := b.String(); got != string(want) {
		t.Errorf("the rendered project differs from %s (go test -run TestGolden -update rewrites it):\n%s", golden, firstDiff(got, string(want)))
	}
}

// firstDiff shows the first line where got and want differ.
func firstDiff(got, want string) string {
	g, w := strings.Split(got, "\n"), strings.Split(want, "\n")
	for i := range max(len(g), len(w)) {
		var gl, wl string
		if i < len(g) {
			gl = g[i]
		}
		if i < len(w) {
			wl = w[i]
		}
		if gl != wl {
			return fmt.Sprintf("line %d:\n got  %q\n want %q", i+1, gl, wl)
		}
	}
	return ""
}

// TestReplaceLine: --replace adds a replace directive to go.mod.
func TestReplaceLine(t *testing.T) {
	o := goldenOptions
	o.Replace = "/src/weakpixel"
	files, err := render(o)
	if err != nil {
		t.Fatal(err)
	}
	if want := "\n\nreplace github.com/weakpixel/engine => /src/weakpixel\n"; !strings.HasSuffix(string(files["go.mod"]), want) {
		t.Errorf("go.mod:\n%s", files["go.mod"])
	}
}

// TestIndexHTML: the page of the browser build is the engine's web shell
// (web/index.html) with the game's title.
func TestIndexHTML(t *testing.T) {
	shell, err := os.ReadFile(filepath.Join(repoRoot(t), "web/index.html"))
	if err != nil {
		t.Fatal(err)
	}
	files, err := render(goldenOptions)
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Replace(string(shell), "<title>Weakpixel game</title>", "<title>mygame</title>", 1)
	if got := string(files["web/index.html"]); got != want {
		t.Errorf("web/index.html.tmpl is not web/index.html with the title replaced:\n%s", firstDiff(got, want))
	}
}

// TestNeverOverwrites: a second run into the same directory fails, names
// every existing file and changes nothing; a non-empty directory needs
// --force, which still writes only new files.
func TestNeverOverwrites(t *testing.T) {
	root := repoRoot(t)
	dir := filepath.Join(t.TempDir(), "mygame")
	run := func(dir string, force bool) error {
		return Run(Options{Dir: dir, Replace: root, NoRun: true, Force: force}, &bytes.Buffer{}, &bytes.Buffer{})
	}
	if err := run(dir, false); err != nil {
		t.Fatalf("first run: %v", err)
	}
	before := snapshot(t, dir)
	if len(before) < 10 {
		t.Fatalf("only %d files written", len(before))
	}
	err := run(dir, false)
	if err == nil {
		t.Fatal("second run succeeded")
	}
	for name := range before {
		if !strings.Contains(err.Error(), "\n  "+name) {
			t.Errorf("second run does not name %s:\n%v", name, err)
		}
	}
	if after := snapshot(t, dir); !maps.Equal(before, after) {
		t.Error("the second run changed files")
	}

	other := filepath.Join(t.TempDir(), "other")
	if err := os.MkdirAll(other, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(other, "notes.txt"), []byte("mine"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := run(other, false); err == nil || !strings.Contains(err.Error(), "is not empty; --force") {
		t.Errorf("non-empty dir: %v", err)
	}
	if err := run(other, true); err != nil {
		t.Errorf("--force: %v", err)
	}
	if data, _ := os.ReadFile(filepath.Join(other, "notes.txt")); string(data) != "mine" {
		t.Error("--force changed an existing file")
	}
}

// snapshot is every file under dir with its contents.
func snapshot(t *testing.T, dir string) map[string]string {
	t.Helper()
	files := map[string]string{}
	err := filepath.WalkDir(dir, func(p string, de os.DirEntry, err error) error {
		if err != nil || de.IsDir() {
			return err
		}
		if de.Name() == ".git" {
			return filepath.SkipDir
		}
		data, err := os.ReadFile(p)
		rel, _ := filepath.Rel(dir, p)
		files[filepath.ToSlash(rel)] = string(data)
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

// TestNewProject creates a project against this checkout, with the
// post-steps, and checks what a new user does with it: regenerate without a
// diff, vet, test, build the editor and both browser builds, and run the
// asset and doc tools with no flags. Skipped under -short.
func TestNewProject(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping module builds in -short mode")
	}
	goBin, err := exec.LookPath("go")
	if err != nil {
		t.Skip("go not found on PATH")
	}
	dir := filepath.Join(t.TempDir(), "mygame")
	var stdout, stderr bytes.Buffer
	if err := Run(Options{Dir: dir, Replace: repoRoot(t), NoGit: true}, &stdout, &stderr); err != nil {
		t.Fatalf("%v\n%s%s", err, stdout.String(), stderr.String())
	}
	for _, name := range []string{"go.sum", "game/zz__gen.go", "build/god/mygame.god.json"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("the post-steps did not write %s: %v", name, err)
		}
	}
	generated := snapshot(t, dir)

	for _, c := range []struct {
		env  []string
		args []string
	}{
		{nil, []string{"generate", "./..."}},
		{nil, []string{"vet", "./..."}},
		{nil, []string{"test", "./..."}},
		{nil, []string{"test", "-tags", "editor", "./..."}},
		{[]string{"GOOS=js", "GOARCH=wasm"}, []string{"build", "./..."}},
		{[]string{"GOOS=js", "GOARCH=wasm"}, []string{"build", "-tags", "webassets", "./..."}},
		{[]string{"GOOS=js", "GOARCH=wasm"}, []string{"build", "-tags", "webassets,webdev", "./..."}},
		{[]string{"GOOS=js", "GOARCH=wasm"}, []string{"build", "-tags", "editor", "./..."}},
		{nil, []string{"tool", "wpix", "assets", "check"}},
		{nil, []string{"run", ".", "-pack", filepath.Join(t.TempDir(), "packs")}},
	} {
		cmd := exec.Command(goBin, c.args...)
		cmd.Dir = dir
		cmd.Env = append(os.Environ(), c.env...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("%v go %v: %v\n%s", c.env, c.args, err, out)
		}
	}
	if after := snapshot(t, dir); !maps.Equal(generated, after) {
		for name := range after {
			if generated[name] != after[name] {
				t.Errorf("%s changed after go generate (or a later step)", name)
			}
		}
	}

	// wpix doc reads build/god/ and writes god.out from .weakpixel.json.
	cmd := exec.Command(goBin, "tool", "wpix", "doc", "--format", "god")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("go tool wpix doc --format god: %v\n%s", err, out)
	}
	if _, err := os.Stat(filepath.Join(dir, "build/god/project.god.json")); err != nil {
		t.Error(err)
	}
}
