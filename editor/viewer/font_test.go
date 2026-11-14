package viewer

import (
	"slices"
	"strings"
	"testing"
	"testing/fstest"

	"golang.org/x/image/font/gofont/goregular"
)

func fontFiles() fstest.MapFS {
	return fstest.MapFS{
		"fonts/ui.ttf":          {Data: goregular.TTF},
		"levels/a.level.json":   {Data: []byte(`{"Label": {"Font": "fonts/ui.ttf#16"}, "L2": {"Font": "fonts/ui.ttf#24"}, "o": "fonts/other.ttf#99"}`)},
		"prefabs/b.prefab.json": {Data: []byte(`{"Font": "fonts/ui.ttf#16", "x": "fonts/ui.ttf#9.5"}`)},
		"notes.txt":             {Data: []byte("fonts/ui.ttf#77")},
	}
}

func TestFontSizesFromLevelsAndPrefabs(t *testing.T) {
	files := fontFiles()
	got := fontSizes(files, projectFiles(files, nil), "fonts/ui.ttf")
	if want := []float64{9.5, 16, 24}; !slices.Equal(got, want) {
		t.Errorf("sizes = %v, want %v (other fonts and other file kinds do not count)", got, want)
	}
	// The page build gets a list instead of walking the filesystem.
	got = fontSizes(files, projectFiles(files, []string{"levels/a.level.json", "notes.txt"}), "fonts/ui.ttf")
	if want := []float64{16, 24}; !slices.Equal(got, want) {
		t.Errorf("sizes from a file list = %v, want %v", got, want)
	}
	if got := fontSizes(nil, nil, "fonts/ui.ttf"); got != nil {
		t.Errorf("no filesystem: %v", got)
	}
}

func TestFontViewShowsUsedSizesAndAnExtra(t *testing.T) {
	files := fontFiles()
	e, v, _ := open(t, files, "fonts/ui.ttf")
	for _, id := range []string{"font.size.9.5", "font.size.16", "font.size.24"} {
		if _, ok := v.Panel().Find(id); !ok {
			t.Errorf("no row %s", id)
		}
	}
	if got := label(t, v, "font.source"); !strings.Contains(got, "used in") {
		t.Errorf("source = %q", got)
	}
	if err := v.Set("font.extra", []byte("40")); err != nil {
		t.Fatal(err)
	}
	e.Update()
	if _, ok := v.Panel().Find("font.size.40"); !ok {
		t.Error("the extra size 40 has no row")
	}
	if err := v.Set("font.sample", []byte(`"Hello"`)); err != nil {
		t.Fatal(err)
	}
	if got := v.inner.(*fontView).sample; got != "Hello" {
		t.Errorf("sample = %q", got)
	}
	for _, bad := range []string{"-3", `"x"`} {
		if err := v.Set("font.extra", []byte(bad)); err == nil {
			t.Errorf("extra size %s was accepted", bad)
		}
	}
}

func TestFontViewDefaultSizes(t *testing.T) {
	_, v, _ := open(t, fstest.MapFS{"ui.ttf": {Data: goregular.TTF}}, "ui.ttf")
	for _, id := range []string{"font.size.12", "font.size.16", "font.size.24", "font.size.32"} {
		if _, ok := v.Panel().Find(id); !ok {
			t.Errorf("no default row %s", id)
		}
	}
	if got := label(t, v, "font.source"); !strings.Contains(got, "default") {
		t.Errorf("source = %q", got)
	}
}

func TestFontViewBrokenFileReportsAnError(t *testing.T) {
	src := fstest.MapFS{"ui.ttf": {Data: []byte("not a font")}}
	_, v, _ := open(t, src, "ui.ttf")
	if rows := v.inner.(*fontView).rows; len(rows) != 0 {
		t.Errorf("rows = %d for a file that is no font", len(rows))
	}
}

func TestLayoutRows(t *testing.T) {
	tops, total := layoutRows([]float32{20, 30, 10}, 5)
	if want := []float32{0, 25, 60}; !slices.Equal(tops, want) || total != 70 {
		t.Errorf("tops = %v total = %v, want %v 70", tops, total, want)
	}
	if tops, total := layoutRows(nil, 5); len(tops) != 0 || total != 0 {
		t.Errorf("no rows: %v %v", tops, total)
	}
}
