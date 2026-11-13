package library

import (
	"slices"
	"strings"
	"testing"
	"testing/fstest"
)

func TestScanPackages(t *testing.T) {
	cat, _, err := Scan(fixture())
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, p := range cat.Packages {
		names = append(names, p.Name)
	}
	// _sources/ is reserved: not a package and not listed.
	want := []string{"cave", "forest", "global", "misc", "title", "trees"}
	if !slices.Equal(names, want) {
		t.Fatalf("packages %v, want %v", names, want)
	}
	tests := []struct {
		pkg           string
		entries, uses []string
	}{
		// The font comes from library.json; the clip set's sheet and
		// image are its dependencies, not entries.
		{"global", []string{"global/fonts/ui.ttf#16", "global/hero/hero.anim.json", "global/sfx/hit.wav"}, nil},
		// bg.png is the map's; the GOD level is no entry but uses global.
		{"forest", []string{"forest/forest.tmj", "forest/music/theme.ogg"}, []string{"global", "trees"}},
		// A package may use another package's files. rock.png is an entry:
		// the map names it with the wrong case.
		{"cave", []string{"cave/cave.tmj", "cave/rock.png"}, []string{"forest"}},
		{"trees", []string{"trees/trees.tsj"}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.pkg, func(t *testing.T) {
			p, ok := cat.Package(tt.pkg)
			if !ok {
				t.Fatal("missing")
			}
			if !slices.Equal(p.Entries, tt.entries) {
				t.Errorf("entries %v, want %v", p.Entries, tt.entries)
			}
			if !slices.Equal(p.Uses, tt.uses) {
				t.Errorf("uses %v, want %v", p.Uses, tt.uses)
			}
		})
	}
}

func TestScanFindings(t *testing.T) {
	_, findings, err := Scan(fixture())
	if err != nil {
		t.Fatal(err)
	}
	var got []string
	for _, f := range findings {
		got = append(got, f.String())
	}
	want := []string{
		`cave/cave.tmj: error: reference "Rock.png": case mismatch, the file is cave/rock.png`,
		`cave/notes.txt: warn: unused`,
		`misc/readme.json: error: reference "../../outside.png" leaves the library root`,
		`misc/readme.json: warn: unused`,
		`title/Logo Big.png: warn: name "Logo Big.png"`,
		`title/title.sheet.json: error: reference "missing.png": no such file title/missing.png`,
	}
	if len(got) != len(want) {
		t.Fatalf("findings:\n%s\nwant %d", strings.Join(got, "\n"), len(want))
	}
	for i := range want {
		if !strings.HasPrefix(got[i], want[i]) {
			t.Errorf("finding %d = %q, want prefix %q", i, got[i], want[i])
		}
	}
}

// TestScanPackageRules checks the rules between packages: global/ uses only
// boot/, boot/ uses nothing, a fence limits a package's uses, global/ is
// required, and a cycle is a warning.
func TestScanPackageRules(t *testing.T) {
	f := func(s string) *fstest.MapFile { return &fstest.MapFile{Data: []byte(s)} }
	tests := []struct {
		name string
		fsys fstest.MapFS
		want []string // finding prefixes, sorted by path
	}{
		{"global uses a package", fstest.MapFS{
			"global/ui.sheet.json": f(`{"image": "../forest/x.png"}`),
			"forest/x.png":         f("png"),
		}, []string{`global/ui.sheet.json: error: reference "../forest/x.png": global/ uses package forest`}},
		{"global and packages use boot", fstest.MapFS{
			"global/ui.sheet.json": f(`{"image": "../boot/logo.png"}`),
			"forest/a.sheet.json":  f(`{"image": "../boot/logo.png"}`),
			"boot/logo.png":        f("png"),
		}, nil},
		{"boot uses global", fstest.MapFS{
			"boot/load.sheet.json": f(`{"image": "../global/x.png"}`),
			"global/x.png":         f("png"),
		}, []string{`boot/load.sheet.json: error: reference "../global/x.png": boot/ uses package global`}},
		{"fence", fstest.MapFS{
			"library.json": f(`{"packages": {"forest": {"uses": ["trees"]}}}`),
			"global/x.png": f("png"),
			"forest/a.tmj": f(`{"layers": [{"image": "../trees/t.png"}, {"image": "../rocks/r.png"}, {"image": "../global/x.png"}]}`),
			"trees/t.png":  f("png"),
			"rocks/r.png":  f("png"),
		}, []string{`forest/a.tmj: error: reference "../rocks/r.png": package forest uses package rocks, which its uses in library.json do not list`}},
		{"fence settings for unknown packages", fstest.MapFS{
			"library.json": f(`{"packages": {"nope": {"uses": ["gone"]}}}`),
			"global/x.png": f("png"),
		}, []string{`library.json: error: settings for unknown package "nope"`, `library.json: error: package "nope": uses unknown package "gone"`}},
		{"missing global", fstest.MapFS{
			"forest/x.png": f("png"),
		}, []string{`global: error: missing: a library needs a global/ package`}},
		{"cycle", fstest.MapFS{
			"global/x.png":   f("png"),
			"a/a.sheet.json": f(`{"image": "../b/b.png"}`),
			"a/a.png":        f("png"),
			"b/b.png":        f("png"),
			"b/b.sheet.json": f(`{"image": "../a/a.png"}`),
		}, []string{`a: warn: package cycle: a -> b -> a`, `b: warn: package cycle: b -> a -> b`}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, findings, err := Scan(tt.fsys)
			if err != nil {
				t.Fatal(err)
			}
			var got []string
			for _, f := range findings {
				got = append(got, f.String())
			}
			if len(got) != len(tt.want) {
				t.Fatalf("findings:\n%s\nwant %v", strings.Join(got, "\n"), tt.want)
			}
			for i := range tt.want {
				if !strings.HasPrefix(got[i], tt.want[i]) {
					t.Errorf("finding %d = %q, want prefix %q", i, got[i], tt.want[i])
				}
			}
		})
	}
}

func TestScanConfigErrors(t *testing.T) {
	_, _, err := Scan(fstest.MapFS{"library.json": {Data: []byte(`{"bogus": 1}`)}})
	if err == nil || !strings.Contains(err.Error(), "library: library.json") {
		t.Fatalf("err %v", err)
	}
	_, findings, err := Scan(fstest.MapFS{
		"library.json": {Data: []byte(`{"entries": {"none": ["x"], "global": ["global/gone.ttf#8"]}}`)},
		"global/a.png": {Data: []byte("png")},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 2 || !strings.Contains(findings[0].Msg, `entry "global/gone.ttf#8": no such file`) || !strings.Contains(findings[1].Msg, `entries for unknown package "none"`) {
		t.Fatalf("findings %v", findings)
	}
}

func TestCatalogRoundTrip(t *testing.T) {
	cat, _, err := Scan(fixture())
	if err != nil {
		t.Fatal(err)
	}
	back, err := ReadCatalog(cat.Encode())
	if err != nil {
		t.Fatal(err)
	}
	if string(back.Encode()) != string(cat.Encode()) {
		t.Fatalf("round trip differs:\n%s", back.Encode())
	}
}

func TestReadCatalogErrors(t *testing.T) {
	tests := map[string]string{
		`{"version": 2, "packages": []}`:                  "version 2, want 0",
		`{"version": 0, "packages": [{"name": "packs"}]}`: `"packs" is not a package name`,
		`{"version": 0, "packages": [{"name": "a/b"}]}`:   `"a/b" is not a package name`,
		`{"version": 0, "groups": []}`:                    "unknown field",
	}
	for in, want := range tests {
		if _, err := ReadCatalog([]byte(in)); err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("ReadCatalog(%s) = %v, want %q", in, err, want)
		}
	}
}

// TestScanPrefabs checks that a prefab nothing references is an entry of its
// package (code spawns it), and that prefab keys in GOD documents count as
// references, so a level using another package's prefab uses that package.
func TestScanPrefabs(t *testing.T) {
	f := func(s string) *fstest.MapFile { return &fstest.MapFile{Data: []byte(s)} }
	fsys := fstest.MapFS{
		"global/prefabs/coin.prefab.json": f(`{"objects": [{"components": {"gfx:Sprite": {"ID": "global/coin.png"}}}]}`),
		"global/coin.png":                 f("png"),
		"forest/forest.level.json":        f(`{"objects": [{"prefab": "forest/prefabs/tree.prefab.json"}, {"prefab": "global/prefabs/coin.prefab.json"}]}`),
		"forest/prefabs/tree.prefab.json": f(`{"objects": [{"components": {"gfx:Sprite": {"ID": "forest/tree.png"}}}]}`),
		"forest/tree.png":                 f("png"),
		"cave/cave.level.json":            f(`{"objects": [{"prefab": "forest/prefabs/tree.prefab.json"}]}`),
	}
	cat, findings, err := Scan(fsys)
	if err != nil {
		t.Fatal(err)
	}
	if p, _ := cat.Package("global"); !slices.Equal(p.Entries, []string{"global/prefabs/coin.prefab.json"}) {
		t.Errorf("global entries %v", p.Entries)
	}
	if p, _ := cat.Package("forest"); len(p.Entries) != 0 || !slices.Equal(p.Uses, []string{"global"}) {
		t.Errorf("forest entries %v uses %v", p.Entries, p.Uses)
	}
	if p, _ := cat.Package("cave"); !slices.Equal(p.Uses, []string{"forest"}) {
		t.Errorf("cave uses %v", p.Uses)
	}
	if len(findings) != 0 {
		t.Fatalf("findings: %v", findings)
	}
}

// TestScanEffects checks that an effect file is a package entry and that its
// sprite and shader keys count as references, so a level that plays the effect
// uses the package that holds the art.
func TestScanEffects(t *testing.T) {
	f := func(s string) *fstest.MapFile { return &fstest.MapFile{Data: []byte(s)} }
	fsys := fstest.MapFS{
		"global/fx/sparks.fx.json": f(`{"emitters": [{"sprite": "global/fx/spark.png", "shader": "global/fx/glow.kage"}]}`),
		"global/fx/spark.png":      f("png"),
		"global/fx/glow.kage":      f("kage"),
		"cave/cave.level.json":     f(`{"objects": [{"components": {"gfx:Particles": {"Effect": "global/fx/sparks.fx.json"}}}]}`),
	}
	cat, findings, err := Scan(fsys)
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 0 {
		t.Errorf("findings: %v", findings)
	}
	g, _ := cat.Package("global")
	if !slices.Contains(g.Entries, "global/fx/sparks.fx.json") {
		t.Errorf("global entries %v lack the effect", g.Entries)
	}
	if g, _ := cat.Package("cave"); !slices.Equal(g.Uses, []string{"global"}) {
		t.Errorf("cave uses %v", g.Uses)
	}
	var refs []string
	for _, r := range scanRefs("global/fx/sparks.fx.json", fsys["global/fx/sparks.fx.json"].Data) {
		refs = append(refs, r.Path)
	}
	if !slices.Equal(refs, []string{"global/fx/glow.kage", "global/fx/spark.png"}) {
		t.Errorf("effect refs = %v", refs)
	}
}

// TestScanSpeechFiles checks that a voice references its samples and its
// language, a dialogue its voice files, and that a sample a voice uses is
// not loaded as a sound of its own.
func TestScanSpeechFiles(t *testing.T) {
	f := func(s string) *fstest.MapFile { return &fstest.MapFile{Data: []byte(s)} }
	fsys := fstest.MapFS{
		"global/talk/bard.voice.json":    f(`{"language": "elvish", "instruments": {"motif": {"sample": "hum.wav"}}}`),
		"global/talk/hum.wav":            f("wav"),
		"global/talk/elvish.lang.json":   f(`{}`),
		"global/talk/town.dialogue.json": f(`{"speakers": [{"id": "a", "voice": "bard.voice.json"}, {"id": "b", "voice": "chirpy"}]}`),
	}
	cat, findings, err := Scan(fsys)
	if err != nil {
		t.Fatal(err)
	}
	if len(findings) != 0 {
		t.Errorf("findings: %v", findings)
	}
	g, _ := cat.Package("global")
	if want := []string{"global/talk/town.dialogue.json"}; !slices.Equal(g.Entries, want) {
		t.Errorf("entries %v, want %v (the voice, sample and language load through the dialogue)", g.Entries, want)
	}
	var refs []string
	for _, r := range scanRefs("global/talk/bard.voice.json", fsys["global/talk/bard.voice.json"].Data) {
		refs = append(refs, r.Path)
	}
	if want := []string{"global/talk/elvish.lang.json", "global/talk/hum.wav"}; !slices.Equal(refs, want) {
		t.Errorf("voice refs = %v, want %v", refs, want)
	}
}
