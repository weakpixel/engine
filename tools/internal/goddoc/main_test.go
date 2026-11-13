package goddoc

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/weakpixel/engine/tools/internal/cli"
)

// runDoc runs the tool and returns the exit code as cli.Execute maps it (0, 1,
// or 2 for a usage error), stdout and the text of the error.
func runDoc(t *testing.T, opts Options) (code int, stdout, stderr string) {
	t.Helper()
	var out strings.Builder
	err := Run(opts, &out)
	if err == nil {
		return 0, out.String(), ""
	}
	var u *cli.UsageError
	if errors.As(err, &u) {
		return 2, out.String(), err.Error()
	}
	return 1, out.String(), err.Error()
}

func TestRun(t *testing.T) {
	sample := filepath.Join("testdata", "sample.god.json")
	dir := t.TempDir()

	t.Run("html by default extension", func(t *testing.T) {
		out := filepath.Join(dir, "doc.html")
		if code, _, stderr := runDoc(t, Options{Output: out, Inputs: []string{sample}}); code != 0 {
			t.Fatalf("code %d: %s", code, stderr)
		}
		data, _ := os.ReadFile(out)
		if !strings.HasPrefix(string(data), "<!doctype html>") || !strings.Contains(string(data), "<title>Game Objects</title>") {
			t.Errorf("not the HTML page: %.80s", data)
		}
	})

	t.Run("format from .md extension", func(t *testing.T) {
		out := filepath.Join(dir, "doc.md")
		if code, _, stderr := runDoc(t, Options{Output: out, Title: "My Game", Inputs: []string{sample}}); code != 0 {
			t.Fatalf("code %d: %s", code, stderr)
		}
		data, _ := os.ReadFile(out)
		if !strings.HasPrefix(string(data), "# My Game\n") {
			t.Errorf("not Markdown with the title: %.80s", data)
		}
	})

	t.Run("explicit format beats extension", func(t *testing.T) {
		code, stdout, _ := runDoc(t, Options{Format: "md", Output: "-", Inputs: []string{sample}})
		if code != 0 || !strings.HasPrefix(stdout, "# Game Objects") {
			t.Errorf("code %d, stdout %.60q", code, stdout)
		}
	})

	t.Run("stdout", func(t *testing.T) {
		code, stdout, _ := runDoc(t, Options{Output: "-", Inputs: []string{sample}})
		if code != 0 || !strings.HasPrefix(stdout, "<!doctype html>") {
			t.Errorf("code %d, stdout %.60q", code, stdout)
		}
	})

	t.Run("split", func(t *testing.T) {
		out := filepath.Join(dir, "split")
		if code, _, stderr := runDoc(t, Options{Split: true, Output: out, Inputs: []string{sample}}); code != 0 {
			t.Fatalf("code %d: %s", code, stderr)
		}
		for _, name := range []string{"index.md", "components.md", "group-game.md", "group-ext.md"} {
			if _, err := os.Stat(filepath.Join(out, name)); err != nil {
				t.Error(err)
			}
		}
	})

	t.Run("default output name and inputs", func(t *testing.T) {
		work := t.TempDir()
		writeTree(t, work, "a/zz__gen.god.json")
		data, _ := os.ReadFile(sample)
		if err := os.WriteFile(filepath.Join(work, "a", "zz__gen.god.json"), data, 0o644); err != nil {
			t.Fatal(err)
		}
		t.Chdir(work)
		if code, _, stderr := runDoc(t, Options{}); code != 0 {
			t.Fatalf("code %d: %s", code, stderr)
		}
		if _, err := os.Stat("god-doc.html"); err != nil {
			t.Error(err)
		}
		if code, _, stderr := runDoc(t, Options{Format: "md"}); code != 0 {
			t.Fatalf("code %d: %s", code, stderr)
		}
		if _, err := os.Stat("god-doc.md"); err != nil {
			t.Error(err)
		}
	})

	failures := []struct {
		name     string
		opts     Options
		code     int
		contains string
	}{
		{"bad format", Options{Format: "pdf", Inputs: []string{sample}}, 2, `unknown format "pdf"`},
		{"split html", Options{Split: true, Format: "html", Inputs: []string{sample}}, 2, "--split needs --format md"},
		{"split stdout", Options{Split: true, Output: "-", Inputs: []string{sample}}, 2, "not stdout"},
		{"no match", Options{Output: "-", Inputs: []string{filepath.Join(dir, "split") + "/..."}}, 1, "no *.god.json files found"},
		{"missing file", Options{Output: "-", Inputs: []string{"missing.json"}}, 1, "missing.json"},
	}
	for _, tt := range failures {
		t.Run(tt.name, func(t *testing.T) {
			code, stdout, stderr := runDoc(t, tt.opts)
			if code != tt.code || !strings.Contains(stderr, tt.contains) {
				t.Errorf("code %d, stderr %q; want code %d containing %q", code, stderr, tt.code, tt.contains)
			}
			if stdout != "" {
				t.Errorf("stdout not empty on failure: %.60q", stdout)
			}
		})
	}
}

// TestRunProjectSettings checks the defaults from .weakpixel.json: inputs
// from god.specs, documentation into godDoc.out, --format god into god.out,
// and that flags override them.
func TestRunProjectSettings(t *testing.T) {
	sample, err := os.ReadFile(filepath.Join("testdata", "sample.god.json"))
	if err != nil {
		t.Fatal(err)
	}
	work := t.TempDir()
	writeTree(t, work, "god/game.god.json")
	if err := os.WriteFile(filepath.Join(work, "god", "game.god.json"), sample, 0o644); err != nil {
		t.Fatal(err)
	}
	settings := `{"god": {"specs": "god", "out": "god/project.god.json"}, "godDoc": {"out": "out"}}`
	if err := os.WriteFile(filepath.Join(work, ".weakpixel.json"), []byte(settings), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(work, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(filepath.Join(work, "sub")) // the file is found by walking up

	for _, format := range []string{"", "md", "god", ""} {
		if code, _, stderr := runDoc(t, Options{Format: format}); code != 0 {
			t.Fatalf("format %q: code %d: %s", format, code, stderr)
		}
	}
	for _, name := range []string{"out/index.html", "out/god.md", "god/project.god.json"} {
		if _, err := os.Stat(filepath.Join(work, name)); err != nil {
			t.Error(err)
		}
	}
	merged, _ := os.ReadFile(filepath.Join(work, "god", "project.god.json"))
	if !strings.Contains(string(merged), `"generator": "weakpixel doc"`) || !strings.Contains(string(merged), `"name": "Score"`) {
		t.Errorf("merged GOD: %.200s", merged)
	}

	// Flags win over the file.
	if code, _, stderr := runDoc(t, Options{Output: "flag.md", Inputs: []string{filepath.Join(work, "god", "game.god.json")}}); code != 0 {
		t.Fatalf("code %d: %s", code, stderr)
	}
	if _, err := os.Stat("flag.md"); err != nil {
		t.Error(err)
	}
}

func TestRunGodFormatNeedsOutput(t *testing.T) {
	t.Chdir(t.TempDir())
	code, _, stderr := runDoc(t, Options{Format: "god", Inputs: []string{filepath.Join("..", "nope.god.json")}})
	if code != 2 || !strings.Contains(stderr, "--format god needs -o or god.out") {
		t.Fatalf("code %d: %s", code, stderr)
	}
}
