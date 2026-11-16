package deps

import (
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

var (
	// docLink matches the target of a markdown link: [text](target).
	docLink = regexp.MustCompile(`\]\(([^)\s]+)\)`)
	// docCodeSpan matches inline code, which may hold text that looks like
	// a link (`m[k](x)`).
	docCodeSpan = regexp.MustCompile("`[^`]*`")
	// docURLScheme matches a target that leaves the repository.
	docURLScheme = regexp.MustCompile(`^[a-z][a-z0-9+.-]*:`)
	// docSlugDrop matches what GitHub removes from a heading to build its
	// anchor: everything but letters, digits, underscores, hyphens, spaces.
	docSlugDrop = regexp.MustCompile(`[^\p{L}\p{N}_\- ]`)
	// docInlineLink matches a link inside a heading; its text stays.
	docInlineLink = regexp.MustCompile(`\[([^\]]*)\]\([^)]*\)`)
)

// headingAnchor returns the anchor GitHub gives a heading line's text:
// lower case, links reduced to their text, punctuation dropped, spaces
// turned into hyphens.
func headingAnchor(text string) string {
	s := strings.ToLower(strings.TrimSpace(text))
	s = docInlineLink.ReplaceAllString(s, "$1")
	s = docSlugDrop.ReplaceAllString(s, "")
	return strings.ReplaceAll(s, " ", "-")
}

// proseLines returns the lines of a markdown page with the lines of fenced
// code blocks blanked, so that line numbers stay.
func proseLines(data []byte) []string {
	lines := strings.Split(string(data), "\n")
	fenced := false
	for i, line := range lines {
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			fenced = !fenced
			lines[i] = ""
		} else if fenced {
			lines[i] = ""
		}
	}
	return lines
}

// pageAnchors returns the heading anchors of the markdown page at p. A
// repeated heading gets the suffixes -1, -2 like on GitHub.
func pageAnchors(t *testing.T, p string) map[string]bool {
	t.Helper()
	data, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	anchors := map[string]bool{}
	seen := map[string]int{}
	for _, line := range proseLines(data) {
		if !strings.HasPrefix(line, "#") {
			continue
		}
		a := headingAnchor(strings.TrimLeft(line, "#"))
		if n := seen[a]; n > 0 {
			anchors[a+"-"+strconv.Itoa(n)] = true
		}
		seen[a]++
		anchors[a] = true
	}
	return anchors
}

func TestHeadingAnchor(t *testing.T) {
	tests := []struct{ heading, want string }{
		{" Not built yet", "not-built-yet"},
		{" Tooling: `wpix anim`", "tooling-wpix-anim"},
		{" Final pass (window resolution)", "final-pass-window-resolution"},
		{" The [guide](guide/README.md) pages", "the-guide-pages"},
		{" Hot-reload of `*.fx.json`", "hot-reload-of-fxjson"},
	}
	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			if got := headingAnchor(tt.heading); got != tt.want {
				t.Errorf("headingAnchor(%q) = %q, want %q", tt.heading, got, tt.want)
			}
		})
	}
}

// TestDocLinksResolve checks every relative link of README.md and the pages
// under doc/: the file must exist, and for a markdown file the #anchor must
// be one of its headings. Links with a URL scheme (other repositories, the
// web) are not checked, because tests have no network.
func TestDocLinksResolve(t *testing.T) {
	repo, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	pages := []string{filepath.Join(repo, "README.md")}
	err = filepath.WalkDir(filepath.Join(repo, "doc"), func(p string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() && path.Ext(p) == ".md" {
			pages = append(pages, p)
		}
		return err
	})
	if err != nil {
		t.Fatal(err)
	}
	anchors := map[string]map[string]bool{}
	for _, p := range pages {
		rel := filepath.ToSlash(strings.TrimPrefix(p, repo+string(filepath.Separator)))
		data, err := os.ReadFile(p)
		if err != nil {
			t.Fatal(err)
		}
		for i, line := range proseLines(data) {
			line = docCodeSpan.ReplaceAllString(line, "")
			for _, m := range docLink.FindAllStringSubmatch(line, -1) {
				target := m[1]
				if docURLScheme.MatchString(target) {
					continue
				}
				file, anchor, _ := strings.Cut(target, "#")
				dest := p
				if file != "" {
					dest = filepath.Join(filepath.Dir(p), filepath.FromSlash(file))
				}
				if _, err := os.Stat(dest); err != nil {
					t.Errorf("%s:%d: link to a missing file: %s", rel, i+1, target)
					continue
				}
				if anchor == "" || path.Ext(dest) != ".md" {
					continue
				}
				if anchors[dest] == nil {
					anchors[dest] = pageAnchors(t, dest)
				}
				if !anchors[dest][anchor] {
					t.Errorf("%s:%d: link to a missing heading: %s", rel, i+1, target)
				}
			}
		}
	}
}
