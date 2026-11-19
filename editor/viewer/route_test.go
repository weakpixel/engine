package viewer

import (
	"strings"
	"testing"
	"testing/fstest"

	"github.com/weakpixel/ecs"
	"github.com/weakpixel/engine/editor/toolshell"
	"github.com/weakpixel/engine/game"
	"github.com/weakpixel/engine/gfx"
	testassets "github.com/weakpixel/engine/testdata/assets"
)

const heroExport = `{"frames":{` +
	`"h 0":{"frame":{"x":0,"y":0,"w":4,"h":4},"duration":100},` +
	`"h 1":{"frame":{"x":4,"y":0,"w":4,"h":4},"duration":100}},` +
	`"meta":{"image":"hero.png","frameTags":[{"name":"idle","from":0,"to":1,"direction":"forward"}]}}`

// switcher runs the viewer on first and switches it with the host's open
// message, as the VSCode Preview panel does.
type switcher struct {
	t  *testing.T
	v  *Viewer
	sh *toolshell.Shell
	e  *game.Engine
	tr *recorder
}

func newSwitcher(t *testing.T, files fstest.MapFS, first string) *switcher {
	t.Helper()
	tr := &recorder{}
	v := New().(*Viewer)
	sh := toolshell.New(toolshell.Config{Name: "view", Tool: v, Key: first, FS: files, Transport: tr})
	e := game.New(game.Config{NoAudio: true, Width: 320, Height: 180, FS: files, Tool: sh})
	t.Cleanup(e.Close)
	s := &switcher{t, v, sh, e, tr}
	s.update()
	return s
}

func (s *switcher) update() {
	s.t.Helper()
	if err := s.e.Update(); err != nil {
		s.t.Fatal(err)
	}
}

// open switches to key and returns the errors sent for it.
func (s *switcher) open(key string) []string {
	s.t.Helper()
	s.tr.msgs = nil
	s.sh.Receive([]byte(`{"type":"open","path":"` + key + `"}`))
	s.update()
	var errs []string
	for _, m := range s.tr.msgs {
		if m["type"] == "error" {
			errs = append(errs, m["message"].(string))
		}
	}
	return errs
}

func (s *switcher) sprites() int {
	n := 0
	for range ecs.NewView1[gfx.Sprite](s.e.World()).Each() {
		n++
	}
	return n
}

// TestRoutePreviews switches one viewer through a texture, an Aseprite
// export and a map: each file gets its previewer, and the entities of the
// one before are gone.
func TestRoutePreviews(t *testing.T) {
	files := testassets.Lib()
	files["box.png"] = &fstest.MapFile{Data: pngBytes(t, 64, 32)}
	files["chars/hero.png"] = &fstest.MapFile{Data: pngBytes(t, 8, 4)}
	files["chars/hero.aseprite.json"] = &fstest.MapFile{Data: []byte(heroExport)}
	s := newSwitcher(t, files, "box.png")
	if _, ok := s.v.inner.(*textureView); !ok {
		t.Fatalf("box.png: previewer %T", s.v.inner)
	}

	if errs := s.open("chars/hero.aseprite.json"); len(errs) != 0 {
		t.Fatalf("aseprite export: errors %v", errs)
	}
	if _, ok := s.v.inner.(*asepritePreview); !ok {
		t.Fatalf("aseprite export: previewer %T", s.v.inner)
	}
	if n := s.sprites(); n != 1 {
		t.Errorf("sprites with the export = %d, want its one", n)
	}
	if f, ok := s.v.Panel().Find("clip.sel"); !ok || !strings.Contains(strings.Join(f.Options, ","), "idle") {
		t.Errorf("the export's panel has no idle clip: %+v", f)
	}

	if errs := s.open("world-1/level-1.tmj"); len(errs) != 0 {
		t.Fatalf("map: errors %v", errs)
	}
	if _, ok := s.v.inner.(*mapView); !ok {
		t.Fatalf("map: previewer %T", s.v.inner)
	}

	if errs := s.open("box.png"); len(errs) != 0 {
		t.Fatalf("back to box.png: errors %v", errs)
	}
	if n := s.sprites(); n != 1 {
		t.Errorf("sprites after switching back = %d, want 1: the export's and the map's entities are gone", n)
	}

	if errs := s.open("tiles.sheet.json"); len(errs) == 0 || !strings.Contains(errs[0], "sheet editor") {
		t.Errorf("a sheet in the preview: errors %v, want it sent to its editor", errs)
	}
}

var talkFiles = fstest.MapFS{
	"box.png":                 {Data: nil},
	"talk/kip.voice.json":     {Data: []byte(`{"version": 0, "name": "kip", "instruments": {"vowel": {}}}`)},
	"talk/town.dialogue.json": {Data: []byte(`{"version": 0, "speakers": [{"id": "kip", "voice": "kip.voice.json"}], "lines": [{"speaker": "kip", "text": "Ae tha."}]}`)},
}

// TestRouteSound: a viewer that starts on a voice installs speech and
// previews voices, dialogues and the other kinds; one that starts without
// sound refuses a voice, since its page or window has no sound.
func TestRouteSound(t *testing.T) {
	files := fstest.MapFS{}
	for k, f := range talkFiles {
		files[k] = f
	}
	files["box.png"] = &fstest.MapFile{Data: pngBytes(t, 16, 16)}

	s := newSwitcher(t, files, "talk/kip.voice.json")
	if _, ok := s.v.inner.(voicePreview); !ok || s.v.voice == nil {
		t.Fatalf("voice: previewer %T", s.v.inner)
	}
	if errs := s.open("box.png"); len(errs) != 0 {
		t.Fatalf("box.png after a voice: errors %v", errs)
	}
	if errs := s.open("talk/town.dialogue.json"); len(errs) != 0 {
		t.Fatalf("dialogue: errors %v", errs)
	}
	if _, ok := s.v.inner.(voicePreview); !ok {
		t.Fatalf("dialogue: previewer %T", s.v.inner)
	}

	quiet := newSwitcher(t, files, "box.png")
	if errs := quiet.open("talk/kip.voice.json"); len(errs) == 0 || !strings.Contains(errs[0], "has sound") {
		t.Errorf("a voice in a page without sound: errors %v", errs)
	}
}
