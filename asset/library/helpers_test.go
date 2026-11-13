package library

import (
	"io/fs"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/weakpixel/asset"
)

func mustPanic(t *testing.T, want string, f func()) {
	t.Helper()
	defer func() {
		r := recover()
		if r == nil {
			t.Fatalf("expected panic containing %q", want)
		}
		if msg, _ := r.(string); !strings.Contains(msg, want) {
			t.Fatalf("panic %v does not contain %q", r, want)
		}
	}()
	f()
}

// text is the asset type of the tests: a file's contents.
type text struct{ S string }

type textLoader struct{}

func (textLoader) Decode(_ *asset.DecodeContext, data []byte) (string, error) {
	return string(data), nil
}
func (textLoader) Finish(_ *asset.FinishContext, s string) (text, error) { return text{S: s}, nil }
func (textLoader) Unload(*text)                                          {}

// noList hides ReadDir, like an HTTP file system.
type noList struct{ fsys fs.FS }

func (n noList) Open(name string) (fs.File, error) {
	f, err := n.fsys.Open(name)
	if err != nil {
		return nil, err
	}
	return plainFile{f}, nil
}

// plainFile hides ReadDir on directories, so fs.ReadDir fails.
type plainFile struct{ fs.File }

// fixture is a small library of packages, with one finding of each kind
// that Scan reports for files.
func fixture() fstest.MapFS {
	f := func(s string) *fstest.MapFile { return &fstest.MapFile{Data: []byte(s)} }
	return fstest.MapFS{
		"library.json":                   f(`{"entries": {"global": ["global/fonts/ui.ttf#16"]}}`),
		"global/fonts/ui.ttf":            f("font"),
		"global/hero/hero.anim.json":     f(`{"version": 1, "sheet": "hero.aseprite.json"}`),
		"global/hero/hero.aseprite.json": f(`{"meta": {"image": "hero.png"}}`),
		"global/hero/hero.png":           f("png"),
		"global/sfx/hit.wav":             f("wav"),
		"forest/forest.tmj":              f(`{"tilesets": [{"firstgid": 1, "source": "../trees/trees.tsj"}], "layers": [{"image": "bg.png"}]}`),
		"forest/bg.png":                  f("png"),
		"forest/forest.level.json":       f(`{"objects": [{"components": {"gfx:Animator": {"Clip": "global/hero/hero.anim.json#idle"}}}]}`),
		"forest/music/theme.ogg":         f("ogg"),
		"cave/cave.tmj":                  f(`{"layers": [{"image": "../forest/bg.png"}, {"image": "Rock.png"}]}`),
		"cave/rock.png":                  f("png"),
		"cave/notes.txt":                 f("todo"),
		"trees/trees.tsj":                f(`{"image": "trees.png"}`),
		"trees/trees.png":                f("png"),
		"title/title.sheet.json":         f(`{"image": "missing.png"}`),
		"title/Logo Big.png":             f("png"),
		"misc/readme.json":               f(`{"image": "../../outside.png"}`),
		"_sources/hero.aseprite":         f("source"),
	}
}
