package library

import (
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
)

// ignoreOf builds an Ignore from ignore files by folder, failing on bad lines.
func ignoreOf(t *testing.T, files map[string]string) *Ignore {
	t.Helper()
	ig := &Ignore{}
	for dir, text := range files {
		rules, errs := parseIgnore(dir, []byte(text))
		if len(errs) > 0 {
			t.Fatalf("parse %s: %v", dir, errs)
		}
		ig.add(dir, rules)
	}
	return ig
}

// ignoreCases follow the examples of git's gitignore documentation. Each
// path is a file unless it ends with "/". The same cases run against git
// itself in TestIgnoreMatchesGit.
var ignoreCases = []struct {
	name    string
	files   map[string]string // ignore file contents by folder
	ignored []string
	kept    []string
}{
	{"comments and blank lines", map[string]string{"": "# a.png\n\n   \n"},
		nil, []string{"a.png", "#a.png"}},
	{"escaped hash and bang", map[string]string{"": "\\#a.png\n\\!b.png\n"},
		[]string{"#a.png", "!b.png"}, []string{"a.png", "b.png"}},
	{"trailing spaces", map[string]string{"": "a.png   \nb.png\\ \n"},
		[]string{"a.png", "b.png "}, []string{"b.png"}},
	{"CRLF", map[string]string{"": "a.png\r\nb.png\r\n"},
		[]string{"a.png", "b.png"}, nil},
	{"floating name at any depth", map[string]string{"": "*.psd\n"},
		[]string{"x.psd", "forest/raw/x.psd"}, []string{"x.png", "forest/x.psd.png"}},
	{"star does not cross slash", map[string]string{"": "forest/*.png\n"},
		[]string{"forest/a.png"}, []string{"forest/tiles/a.png", "temple/forest/a.png"}},
	{"leading slash anchors", map[string]string{"": "/notes.md\n"},
		[]string{"notes.md"}, []string{"forest/notes.md"}},
	{"middle slash anchors", map[string]string{"": "raw/x.png\n"},
		[]string{"raw/x.png"}, []string{"forest/raw/x.png"}},
	{"question mark and class", map[string]string{"": "a?.png\nb[0-9].png\nc[!x].png\n"},
		[]string{"a1.png", "b5.png", "cy.png"}, []string{"a12.png", "bx.png", "cx.png"}},
	{"dir only", map[string]string{"": "drafts/\n"},
		[]string{"drafts/a.png", "forest/drafts/b/c.png"}, []string{"forest/drafts.png"}},
	{"name matches a folder too", map[string]string{"": "drafts\n"},
		[]string{"drafts", "forest/drafts/a.png"}, []string{"forest/drafts.png"}},
	{"negation, last rule wins", map[string]string{"": "*.png\n!keep.png\n"},
		[]string{"a.png", "forest/b.png"}, []string{"keep.png", "forest/keep.png"}},
	{"negation before rule loses", map[string]string{"": "!keep.png\n*.png\n"},
		[]string{"keep.png"}, nil},
	{"no re-include below an ignored folder", map[string]string{"": "raw/\n!raw/keep.png\n"},
		[]string{"raw/keep.png", "raw/a.png"}, nil},
	{"re-include with folder/* pattern", map[string]string{"": "/raw/*\n!/raw/keep.png\n"},
		[]string{"raw/a.png"}, []string{"raw/keep.png"}},
	{"leading double star", map[string]string{"": "**/tmp\n"},
		[]string{"tmp", "forest/tmp", "forest/a/tmp/x.png"}, []string{"forest/tmpx"}},
	{"double star with slash", map[string]string{"": "**/raw/x.png\n"},
		[]string{"raw/x.png", "a/b/raw/x.png"}, []string{"raw/y.png"}},
	{"trailing double star", map[string]string{"": "raw/**\n"},
		[]string{"raw/x.png", "raw/a/b.png"}, []string{"forest/raw/x.png"}},
	{"middle double star", map[string]string{"": "a/**/b.png\n"},
		[]string{"a/b.png", "a/x/b.png", "a/x/y/b.png"}, []string{"c/a/b.png"}},
	{"nested file applies to its folder", map[string]string{"forest": "*.png\n"},
		[]string{"forest/a.png", "forest/tiles/a.png"}, []string{"a.png", "temple/a.png"}},
	{"nested anchored is relative to its folder", map[string]string{"forest": "/a.png\n"},
		[]string{"forest/a.png"}, []string{"a.png", "forest/tiles/a.png"}},
	{"deeper file overrides", map[string]string{"": "*.png\n", "forest": "!keep.png\n"},
		[]string{"a.png", "forest/a.png"}, []string{"forest/keep.png"}},
	{"case-sensitive", map[string]string{"": "*.PSD\n"},
		[]string{"a.PSD"}, []string{"a.psd"}},
}

func TestIgnoreMatch(t *testing.T) {
	for _, tc := range ignoreCases {
		t.Run(tc.name, func(t *testing.T) {
			ig := ignoreOf(t, tc.files)
			for _, p := range tc.ignored {
				if ok, _ := ig.Match(strings.TrimSuffix(p, "/"), strings.HasSuffix(p, "/")); !ok {
					t.Errorf("%s: not ignored", p)
				}
			}
			for _, p := range tc.kept {
				if ok, by := ig.Match(strings.TrimSuffix(p, "/"), strings.HasSuffix(p, "/")); ok {
					t.Errorf("%s: ignored by %s", p, by)
				}
			}
		})
	}
}

func TestIgnoreDirOnlyNeedsDir(t *testing.T) {
	ig := ignoreOf(t, map[string]string{"": "drafts/\n"})
	if ok, _ := ig.Match("drafts", false); ok {
		t.Error("a file named drafts is ignored by drafts/")
	}
	if ok, _ := ig.Match("drafts", true); !ok {
		t.Error("the folder drafts is not ignored")
	}
}

func TestIgnoreMatchNamesRule(t *testing.T) {
	ig := ignoreOf(t, map[string]string{"": "*.png\n", "forest": "# raw art\n\nraw/\n!keep.png\n"})
	for _, tc := range []struct{ p, by string }{
		{"a.png", ".assetignore:1"},
		{"forest/raw/x.psd", "forest/.assetignore:3"},
		{"forest/keep.png", "forest/.assetignore:4"},
	} {
		if _, by := ig.Match(tc.p, false); by != tc.by {
			t.Errorf("%s: by %q, want %q", tc.p, by, tc.by)
		}
	}
}

func TestIgnoreNil(t *testing.T) {
	var ig *Ignore
	if ok, _ := ig.Match("a.png", false); ok || !ig.Empty() || ig.Files() != nil {
		t.Fatal("nil Ignore ignores something")
	}
}

func TestParseIgnoreReportsEveryBadLine(t *testing.T) {
	rules, errs := parseIgnore("forest", []byte("a[.png\nok.png\n!\nb[.png\n"))
	if len(rules) != 1 || rules[0].segs[0] != "ok.png" {
		t.Fatalf("rules = %+v, want only ok.png", rules)
	}
	var got []string
	for _, e := range errs {
		got = append(got, e.Error())
	}
	want := []string{
		`forest/.assetignore:1: bad pattern "a[.png"`,
		`forest/.assetignore:3: empty pattern`,
		`forest/.assetignore:4: bad pattern "b[.png"`,
	}
	if !slices.Equal(got, want) {
		t.Fatalf("errors:\n%s\nwant:\n%s", strings.Join(got, "\n"), strings.Join(want, "\n"))
	}
}

func TestReadIgnoreSkipsIgnoredFolders(t *testing.T) {
	fsys := fstest.MapFS{
		".assetignore":          {Data: []byte("raw/\n")},
		"raw/.assetignore":      {Data: []byte("bad[\n")},
		"forest/.assetignore":   {Data: []byte("*.psd\n")},
		"forest/a.psd":          {},
		"global/x.png":          {},
		"_scratch/.assetignore": {Data: []byte("bad[\n")},
	}
	ig, err := ReadIgnore(fsys)
	if err != nil {
		t.Fatalf("ReadIgnore: %v (an ignore file in an ignored or reserved folder was read)", err)
	}
	if got := ig.Files(); !slices.Equal(got, []string{".assetignore", "forest/.assetignore"}) {
		t.Fatalf("Files = %v", got)
	}
}

func TestMatchAllocs(t *testing.T) {
	ig := ignoreOf(t, map[string]string{"": "*.psd\n/raw/**\n", "forest": "drafts/\n!keep.png\n"})
	n := testing.AllocsPerRun(100, func() { ig.Match("forest/tiles/deep/trees.png", false) })
	if n != 0 {
		t.Fatalf("Match allocates %v times, want 0", n)
	}
}

// TestIgnoreMatchesGit runs the cases through git check-ignore, so the
// table is checked against git itself. It is skipped without git.
func TestIgnoreMatchesGit(t *testing.T) {
	git, err := exec.LookPath("git")
	if err != nil {
		t.Skip("git not found")
	}
	for _, tc := range ignoreCases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			run := func(args ...string) (string, error) {
				cmd := exec.Command(git, args...)
				cmd.Dir = dir
				cmd.Env = append(os.Environ(), "GIT_CONFIG_GLOBAL=/dev/null", "GIT_CONFIG_NOSYSTEM=1")
				out, err := cmd.CombinedOutput()
				return string(out), err
			}
			if out, err := run("init", "-q"); err != nil {
				t.Fatalf("git init: %v %s", err, out)
			}
			for d, text := range tc.files {
				p := filepath.Join(dir, filepath.FromSlash(d), ".gitignore")
				if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(p, []byte(text), 0o644); err != nil {
					t.Fatal(err)
				}
			}
			check := func(p string, want bool) {
				// check-ignore tells a folder by a trailing slash, and
				// needs the path to exist to see it as one.
				isDir := strings.HasSuffix(p, "/")
				full := filepath.Join(dir, filepath.FromSlash(strings.TrimSuffix(p, "/")))
				if isDir {
					_ = os.MkdirAll(full, 0o755)
				} else {
					_ = os.MkdirAll(filepath.Dir(full), 0o755)
					_ = os.WriteFile(full, nil, 0o644)
				}
				_, err := run("check-ignore", "-q", "--no-index", "--", p)
				got := err == nil
				if got != want {
					t.Errorf("git check-ignore %q = %v, the table says %v", p, got, want)
				}
			}
			for _, p := range tc.ignored {
				check(p, true)
			}
			for _, p := range tc.kept {
				check(p, false)
			}
		})
	}
}
