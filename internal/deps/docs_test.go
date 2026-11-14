package deps

import (
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

// docExamples matches what the documentation must not say: a path or link
// into an examples folder, a run command for a demo, or a demo's name.
var docExamples = regexp.MustCompile("examples/|go -C examples|hero-tiled|hero-anim|input-demo|sounddemo|shaders-layer-group|shaders-group-input")

// docsMayNameExamples are the pages that describe who uses the engine: the
// module table of the architecture page.
var docsMayNameExamples = map[string]bool{
	"doc/design/architecture.md": true,
}

// TestDocsDoNotNameExamples keeps the guides independent of any demo: a
// guide that needs a running reference shows a snippet or points to a
// compiled example_test.go of the package.
func TestDocsDoNotNameExamples(t *testing.T) {
	repo, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	var pages []string
	err = filepath.WalkDir(filepath.Join(repo, "doc"), func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && path.Ext(p) == ".md" {
			pages = append(pages, p)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	pages = append(pages, filepath.Join(repo, "README.md"))
	for _, p := range pages {
		rel := filepath.ToSlash(strings.TrimPrefix(p, repo+string(filepath.Separator)))
		if docsMayNameExamples[rel] {
			continue
		}
		data, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		for i, line := range strings.Split(string(data), "\n") {
			if docExamples.MatchString(line) {
				t.Errorf("%s:%d names the examples: %s", rel, i+1, strings.TrimSpace(line))
			}
		}
	}
}
