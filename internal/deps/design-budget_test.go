package deps

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Design pages are read whole by people and agents, so each holds one topic
// and stays small; one sentence per line keeps search hits and diffs short.
const (
	designPageMaxBytes = 16 << 10
	designLineMaxChars = 600
)

// TestDesignPageBudget checks every page of doc/design: at most
// designPageMaxBytes, and no prose line over designLineMaxChars (table rows
// and fenced code are exempt). A topic that outgrows its page gets a page of
// its own, listed in doc/design/README.md.
func TestDesignPageBudget(t *testing.T) {
	pages, err := filepath.Glob(filepath.Join("..", "..", "doc", "design", "*.md"))
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) == 0 {
		t.Fatal("no design pages found")
	}
	for _, p := range pages {
		data, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		name := filepath.Base(p)
		if len(data) > designPageMaxBytes {
			t.Errorf("doc/design/%s is %d bytes, over %d: split the topic into its own page", name, len(data), designPageMaxBytes)
		}
		for i, line := range proseLines(data) {
			if strings.HasPrefix(strings.TrimSpace(line), "|") {
				continue
			}
			if n := len([]rune(line)); n > designLineMaxChars {
				t.Errorf("doc/design/%s:%d: line of %d characters, over %d: put one sentence per line", name, i+1, n, designLineMaxChars)
			}
		}
	}
}
