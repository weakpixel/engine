package animedit

import (
	"errors"
	"strings"
	"testing"

	"github.com/weakpixel/engine/input"
)

// An Aseprite export opens read-only: its tags play, no editing section or
// tool is offered, and every edit is refused without touching the document.
func TestAsepriteExportOpensReadOnly(t *testing.T) {
	r := newRigKey(t, "chars/hero.aseprite.json", heroSheetJSON)
	if !r.tool.st.ReadOnly || !r.tool.st.Ready() {
		t.Fatalf("ReadOnly = %v, Ready = %v", r.tool.st.ReadOnly, r.tool.st.Ready())
	}
	p := r.tool.Panel()
	if err := p.Check(); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"clip.sel", "play", "frame", "flip", "export.mode", "export.clips"} {
		if _, ok := p.Find(id); !ok {
			t.Errorf("no field %q", id)
		}
	}
	for _, id := range []string{"tool", "file.default", "clip.loop", "box.sel"} {
		if _, ok := p.Find(id); ok {
			t.Errorf("read-only panel has the editing field %q", id)
		}
	}
	if f, _ := p.Find("export.clips"); string(f.Value) != `"2"` {
		t.Errorf("clips = %s, want 2 (idle, run)", f.Value)
	}

	// Playing works: the second tag is selected and steps.
	r.set("clip.sel", 1)
	if got := r.tool.st.ClipName(); got != "run" {
		t.Errorf("clip = %q", got)
	}
	r.set("frame", 2)
	if r.tool.st.Play.Frame != 2 {
		t.Errorf("frame = %d", r.tool.st.Play.Frame)
	}
	if len(r.of("edit")) != 0 {
		t.Errorf("a read-only view sent an edit: %v", r.of("edit"))
	}
}

func TestReadOnlyRefusesEdits(t *testing.T) {
	r := newRigKey(t, "chars/hero.aseprite.json", heroSheetJSON)
	st := r.tool.st
	for name, err := range map[string]error{
		"pivot": st.Set("file.haspivot", []byte("true")),
		"loop":  st.Set("clip.loop", []byte("1")),
		"body":  st.Set("file.clearbody", nil),
		"clip":  st.Set("clip.add", nil),
	} {
		if !errors.Is(err, ErrReadOnly) {
			t.Errorf("%s: err = %v, want ErrReadOnly", name, err)
		}
	}
	// A key press that would delete or nudge does not even report an error.
	r.src.Frame.Keys.Set(input.KeyDelete)
	r.update()
	if msgs := r.of("error"); len(msgs) != 0 {
		t.Errorf("Delete in a read-only view reported %v", msgs)
	}
}

func TestCheckAcceptsDefinitionAndExport(t *testing.T) {
	tool := New().(*Tool)
	for _, key := range []string{"a.anim.json", "chars/hero.aseprite.json"} {
		if err := tool.Check(key); err != nil {
			t.Errorf("Check(%q) = %v", key, err)
		}
	}
	err := tool.Check("a.json")
	if err == nil || !strings.Contains(err.Error(), ".aseprite.json") {
		t.Errorf("Check(a.json) = %v", err)
	}
}
