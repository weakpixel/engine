package goddoc

import (
	"flag"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strings"
	"testing"
)

var update = flag.Bool("update", false, "update golden files")

func checkGolden(t *testing.T, name, got string) {
	t.Helper()
	path := filepath.Join("testdata", name)
	if *update {
		if err := os.WriteFile(path, []byte(got), 0o644); err != nil {
			t.Fatal(err)
		}
		return
	}
	want, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("%v (run with -update to create it)", err)
	}
	if string(want) != got {
		t.Errorf("%s differs from golden output (run with -update, review the diff):\n%s", name, got)
	}
}

func TestRenderMarkdownGolden(t *testing.T) {
	checkGolden(t, "sample.md.golden", renderMarkdown(loadSample(t)))
}

func TestRenderHTMLGolden(t *testing.T) {
	var b strings.Builder
	if err := renderHTML(&b, loadSample(t)); err != nil {
		t.Fatal(err)
	}
	checkGolden(t, "sample.html.golden", b.String())
}

func TestRenderDeterministic(t *testing.T) {
	var a, b strings.Builder
	for _, w := range []*strings.Builder{&a, &b} {
		if err := renderHTML(w, loadSample(t)); err != nil {
			t.Fatal(err)
		}
	}
	if a.String() != b.String() {
		t.Error("HTML output differs between runs")
	}
	if renderMarkdown(loadSample(t)) != renderMarkdown(loadSample(t)) {
		t.Error("Markdown output differs between runs")
	}
}

func TestSlug(t *testing.T) {
	tests := []struct{ in, want string }{
		{"game:Hero", "gamehero"},
		{"Transform (2)", "transform-2"},
		{"Objects", "objects"},
		{"Group game_ext", "group-game_ext"},
		{"A-B  C", "a-b--c"},
	}
	for _, tt := range tests {
		if got := slug(tt.in); got != tt.want {
			t.Errorf("slug(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
}

var (
	headingRe = regexp.MustCompile(`(?m)^#{1,6} (.+)$`)
	linkRe    = regexp.MustCompile(`\]\(([^)]*)\)`)
)

// checkLinks verifies that every Markdown link points to a heading of the
// file it names.
func checkLinks(t *testing.T, files map[string]string, current string) {
	t.Helper()
	anchors := map[string][]string{}
	for name, content := range files {
		seen := map[string]int{}
		for _, m := range headingRe.FindAllStringSubmatch(content, -1) {
			s := slug(m[1])
			if n := seen[s]; n > 0 {
				s += "-" + string(rune('0'+n))
			}
			seen[s]++
			anchors[name] = append(anchors[name], s)
		}
	}
	links := 0
	for name, content := range files {
		for _, m := range linkRe.FindAllStringSubmatch(content, -1) {
			links++
			file, anchor, _ := strings.Cut(m[1], "#")
			if file == "" {
				file = name
			}
			if !slices.Contains(anchors[file], anchor) {
				t.Errorf("%s: link %q does not resolve", name, m[1])
			}
		}
	}
	if links == 0 {
		t.Error("no links found")
	}
}

func TestMarkdownLinksResolve(t *testing.T) {
	doc := loadSample(t)
	checkLinks(t, map[string]string{"": renderMarkdown(doc)}, "")
	checkLinks(t, renderMarkdownSplit(doc), "")
}

func TestMarkdownContent(t *testing.T) {
	md := renderMarkdown(loadSample(t))
	for _, want := range []string{
		"# Sample\n",
		"| [game:Transform](#gametransform) | `Transform` | embedded (Common.Base) | Where the hero is. |",
		"| [game:Weapon](#gameweapon) | `Weapon` | optional | The weapon the hero carries, if any. |",
		"| [game:Companion](#gamecompanion) | `Companion` | readonly |  |",
		"### game:Transform\n",
		"Go type: `example.com/game.Transform`",
		"_No object uses it yet._",                            // game:Score
		`Horizontal \| position.`,                             // pipe escaped in a cell
		"Health is hit points.\n\nIt never drops below zero.", // multi-line doc stays a paragraph
		"- [game:Boss](#gameboss) — embedded (Common)",
	} {
		if !strings.Contains(md, want) {
			t.Errorf("Markdown lacks %q", want)
		}
	}
	// A multi-line doc must not leak into a table row.
	for line := range strings.Lines(md) {
		if strings.HasPrefix(line, "|") && !strings.HasSuffix(strings.TrimRight(line, "\n"), "|") {
			t.Errorf("broken table row: %q", line)
		}
	}
}

func TestCell(t *testing.T) {
	if got, want := cell("a | b\nc\n\n  d"), `a \| b c d`; got != want {
		t.Errorf("cell = %q, want %q", got, want)
	}
}

func TestMarkdownSplit(t *testing.T) {
	files := renderMarkdownSplit(loadSample(t))
	var names []string
	for n := range files {
		names = append(names, n)
	}
	slices.Sort(names)
	if want := []string{"components.md", "group-ext.md", "group-game.md", "index.md"}; !slices.Equal(names, want) {
		t.Fatalf("files = %v, want %v", names, want)
	}
	if !strings.Contains(files["index.md"], "[Hero](group-game.md#gamehero)") {
		t.Errorf("index lacks a cross-file object link:\n%s", files["index.md"])
	}
	if !strings.Contains(files["group-game.md"], "[game:Transform](components.md#gametransform)") {
		t.Errorf("group file lacks a component link:\n%s", files["group-game.md"])
	}
	if strings.Contains(files["group-game.md"], "ext:Thing") {
		t.Error("group file contains another group's object")
	}
}

func TestHTMLContent(t *testing.T) {
	var b strings.Builder
	if err := renderHTML(&b, loadSample(t)); err != nil {
		t.Fatal(err)
	}
	html := b.String()
	for _, want := range []string{
		`<span class="badge embedded">embedded (Common.Base)</span>`,
		`id="obj-game-Hero"`,
		`id="comp-game-Transform"`,
		`href="#comp-game-Health"`,
		`<em>no object yet</em>`,
		`id="tab-components"`,
		`id="theme"`,
		`:root[data-theme="dark"]`,
		`:root:not([data-theme="light"])`,
		`localStorage.getItem("god-doc-theme")`,
		"Stores the hero&#39;s health.",
	} {
		if !strings.Contains(html, want) {
			t.Errorf("HTML lacks %q", want)
		}
	}
	for _, bad := range []string{"+game:", "http://", "https://"} {
		if strings.Contains(html, bad) {
			t.Errorf("HTML contains %q", bad)
		}
	}
	// Every in-page link has a target.
	ids := map[string]bool{}
	for _, m := range regexp.MustCompile(`id="([^"]+)"`).FindAllStringSubmatch(html, -1) {
		ids[m[1]] = true
	}
	for _, m := range regexp.MustCompile(`href="#([^"]+)"`).FindAllStringSubmatch(html, -1) {
		if !ids[m[1]] {
			t.Errorf("href #%s has no target", m[1])
		}
	}
}
