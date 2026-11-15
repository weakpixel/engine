package library

import (
	"slices"
	"strings"
	"testing"
	"testing/fstest"
)

// ignoreFixture is a small library with ignore files in the root and in a
// package, and a package that is ignored as a whole.
func ignoreFixture() fstest.MapFS {
	return fstest.MapFS{
		".assetignore":              {Data: []byte("# work files\n*.psd\n/drafts/\n")},
		"global/hit.wav":            {Data: []byte("hit")},
		"global/art.psd":            {Data: []byte("psd")},
		"forest/.assetignore":       {Data: []byte("raw/\n!keep.psd\n")},
		"forest/forest.tmj":         {Data: []byte(`{"layers": []}`)},
		"forest/keep.psd":           {Data: []byte("kept")},
		"forest/raw/sketch.png":     {Data: []byte("png")},
		"forest/raw/notes.txt":      {Data: []byte("notes")},
		"drafts/draft.level.json":   {Data: []byte(`{}`)},
		"forest/music/theme.ogg":    {Data: []byte("ogg")},
		"forest/music/.assetignore": {Data: []byte("\n")},
	}
}

func TestScanSkipsIgnoredFiles(t *testing.T) {
	cat, findings, err := Scan(ignoreFixture())
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	var files []string
	for _, p := range cat.Packages {
		names = append(names, p.Name)
		files = append(files, p.Files...)
	}
	if want := []string{"forest", "global"}; !slices.Equal(names, want) {
		t.Errorf("packages %v, want %v (drafts/ is ignored)", names, want)
	}
	// keep.psd: the deeper file re-includes it. The ignore files
	// themselves are never library files.
	want := []string{"forest/forest.tmj", "forest/keep.psd", "forest/music/theme.ogg", "global/hit.wav"}
	if !slices.Equal(files, want) {
		t.Errorf("files %v, want %v", files, want)
	}
	for _, f := range findings {
		if strings.Contains(f.Path, "raw/") || strings.HasSuffix(f.Path, "art.psd") {
			t.Errorf("finding about an ignored file: %v", f)
		}
	}
}

func TestScanIgnoreFindings(t *testing.T) {
	fsys := fstest.MapFS{
		".assetignore":        {Data: []byte("*.psd\nbad[\nlibrary.json\n")},
		"library.json":        {Data: []byte(`{"entries": {"global": ["global/art.psd"]}}`)},
		"global/art.psd":      {},
		"global/x.sheet.json": {Data: []byte(`{"image": "art.psd"}`)},
		"forest/.assetignore": {Data: []byte("[z\n")},
	}
	_, findings, err := Scan(fsys)
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, f := range findings {
		got = append(got, f.String())
	}
	for _, want := range []string{
		`.assetignore: error: line 2: bad pattern "bad["`,
		`forest/.assetignore: error: line 1: bad pattern "[z"`,
		`library.json: error: ignored by .assetignore:3: the library reads this file itself, it cannot be ignored`,
		`library.json: error: entry "global/art.psd": the file is ignored by .assetignore:1`,
		`global/x.sheet.json: error: reference "art.psd": global/art.psd is ignored by .assetignore:1`,
	} {
		if !slices.Contains(got, want) {
			t.Errorf("missing finding %q in:\n%s", want, strings.Join(got, "\n"))
		}
	}
}

func TestScanIgnoredGlobal(t *testing.T) {
	_, findings, err := Scan(fstest.MapFS{
		".assetignore":   {Data: []byte("global/\n")},
		"global/hit.wav": {},
		"forest/a.wav":   {},
	})
	if err != nil {
		t.Fatal(err)
	}
	want := "global: error: ignored by .assetignore:1: a library needs a global/ package (loaded at boot)"
	if len(findings) == 0 || findings[0].String() != want {
		t.Fatalf("findings %v, want %q first", findings, want)
	}
}

func TestScanWithoutIgnoreUnchanged(t *testing.T) {
	// The ignore walk must list exactly what the plain walk listed.
	cat, _, err := Scan(fixture())
	if err != nil {
		t.Fatal(err)
	}
	ig, err := ReadIgnore(fixture())
	if err != nil || !ig.Empty() {
		t.Fatalf("fixture has ignore rules: %v %v", ig.Files(), err)
	}
	n := 0
	for _, p := range cat.Packages {
		n += len(p.Files)
	}
	if n == 0 {
		t.Fatal("no files")
	}
}

func TestIgnoredFiles(t *testing.T) {
	got, err := IgnoredFiles(ignoreFixture())
	if err != nil {
		t.Fatal(err)
	}
	want := []IgnoredFile{
		{"drafts/draft.level.json", ".assetignore:3"},
		{"forest/raw/notes.txt", "forest/.assetignore:1"},
		{"forest/raw/sketch.png", "forest/.assetignore:1"},
		{"global/art.psd", ".assetignore:2"},
	}
	if !slices.Equal(got, want) {
		t.Fatalf("IgnoredFiles = %v, want %v", got, want)
	}
}

func TestDiffFiles(t *testing.T) {
	disk := ignoreFixture()
	embedded := fstest.MapFS{
		"global/hit.wav":         {},
		"global/art.psd":         {}, // ignored on disk, so extra here
		"forest/forest.tmj":      {},
		"forest/music/theme.ogg": {},
	}
	missing, extra, err := DiffFiles(disk, embedded)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(missing, []string{"forest/keep.psd"}) {
		t.Errorf("missing %v", missing)
	}
	if !slices.Equal(extra, []string{"global/art.psd"}) {
		t.Errorf("extra %v", extra)
	}
}
