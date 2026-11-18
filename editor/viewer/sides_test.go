package viewer

import (
	"slices"
	"testing"
	"testing/fstest"

	"golang.org/x/image/font/gofont/goregular"

	"github.com/weakpixel/engine/editor/toolshell"
	testassets "github.com/weakpixel/engine/testdata/assets"
)

// TestPanelSides checks which sections of each view tool are on the left (the
// View section and the lists and toggles you pick content from) and that the
// panels are valid. Every other section is on the right.
func TestPanelSides(t *testing.T) {
	lib := testassets.Lib()
	atlas := fstest.MapFS{"hero.atlas.json": {Data: []byte(hash)}, "hero.png": {Data: pngBytes(t, 32, 16)}}
	texture := fstest.MapFS{"box.png": {Data: pngBytes(t, 32, 16)}}
	font := fstest.MapFS{"ui.ttf": {Data: goregular.TTF}}
	tests := []struct {
		name  string
		files fstest.MapFS
		key   string
		left  []string
		right []string
	}{
		{"texture", texture, "box.png", []string{"view"}, []string{"image"}},
		{"atlas", atlas, "hero.atlas.json", []string{"frames", "view"}, []string{"atlas", "image"}},
		{"font", font, "ui.ttf", []string{"view"}, []string{"font", "sizes"}},
		{"map", lib, "world-1/level-1.tmj", []string{"layers", "objects", "levels", "view"}, []string{"map", "object"}},
		{"tileset", lib, "temple/temple.tsj", []string{"show", "view"}, []string{"tileset", "tile"}},
		{"world", lib, "world-1/world-1.world", []string{"maps", "view"}, []string{"world", "map"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, v, _, _ := openFiles(t, tt.files, tt.key)
			assertSides(t, v.Panel(), tt.left, tt.right)
		})
	}
	t.Run("shader", func(t *testing.T) {
		_, v, _, _ := openShader(t, flashKage)
		if err := v.Panel().Check(); err != nil {
			t.Fatal(err)
		}
		assertSides(t, v.Panel(), []string{"view"}, []string{"shader", "params", "image"})
	})
}

// assertSides checks the section ids on each side, in order.
func assertSides(t *testing.T, p toolshell.Panel, left, right []string) {
	t.Helper()
	if err := p.Check(); err != nil {
		t.Fatalf("panel: %v", err)
	}
	var gotL, gotR []string
	for _, s := range p.Sections {
		if s.Side == toolshell.SideLeft {
			gotL = append(gotL, s.ID)
		} else {
			gotR = append(gotR, s.ID)
		}
	}
	if !slices.Equal(gotL, left) || !slices.Equal(gotR, right) {
		t.Errorf("left %v right %v, want left %v right %v", gotL, gotR, left, right)
	}
}

func TestAtlasFrameList(t *testing.T) {
	_, v, _ := open(t, fstest.MapFS{"hero.atlas.json": {Data: []byte(hash)}, "hero.png": {Data: pngBytes(t, 32, 16)}}, "hero.atlas.json")
	list, ok := v.Panel().Find("atlas.frame")
	if !ok || list.List != maxFrameRows || len(list.Options) != 3 || list.Options[0] != "(none)" || list.Options[2] != "1 a_first" {
		t.Fatalf("frame list = %+v", list)
	}
	if err := v.Set("atlas.frame", []byte("2")); err != nil {
		t.Fatal(err)
	}
	if got := label(t, v, "frame.which"); got != "1 (selected)" {
		t.Errorf("after Set 2 = %q, want frame 1 selected", got)
	}
	// Choosing the selected frame again keeps it, unlike a click.
	if err := v.Set("atlas.frame", []byte("2")); err != nil {
		t.Fatal(err)
	}
	if got := label(t, v, "frame.which"); got != "1 (selected)" {
		t.Errorf("after Set 2 again = %q", got)
	}
	if err := v.Set("atlas.frame", []byte("0")); err != nil {
		t.Fatal(err)
	}
	if got := label(t, v, "frame.which"); got != "click a frame to select it" {
		t.Errorf("after Set 0 = %q, want no selection", got)
	}
	if err := v.Set("atlas.frame", []byte("3")); err == nil {
		t.Error("a frame out of range was accepted")
	}
}

func TestWorldMapList(t *testing.T) {
	_, v, _, rec := openFiles(t, testassets.Lib(), "world-1/world-1.world")
	list, ok := v.Panel().Find("world.map")
	if !ok || list.List != maxMapRows || len(list.Options) != 3 || list.Options[0] != "(none)" {
		t.Fatalf("map list = %+v", list)
	}
	if err := v.Set("world.map", []byte("2")); err != nil {
		t.Fatal(err)
	}
	if got := label(t, v, "map.which"); got != "1 (selected)" {
		t.Errorf("after Set 2 = %q, want map 1 selected", got)
	}
	if got := opens(rec); len(got) != 0 {
		t.Errorf("selecting by list opened %v", got)
	}
	if err := v.Set("world.map", []byte("0")); err != nil {
		t.Fatal(err)
	}
	if got := label(t, v, "map.which"); got == "1 (selected)" || got == "0 (selected)" {
		t.Errorf("after Set 0 = %q, want no selection", got)
	}
	if err := v.Set("world.map", []byte("9")); err == nil {
		t.Error("a map out of range was accepted")
	}
}
