package editorapp

import (
	"slices"
	"strings"
	"testing"

	"github.com/weakpixel/engine/editor/filekind"
)

// toolKinds is filekind.All without the stage kinds.
func toolKinds() []filekind.Info {
	return slices.DeleteFunc(slices.Clone(filekind.All), func(i filekind.Info) bool { return i.Stage })
}

// TestKindsCoverFilekind: one entry per file kind that has a tool, in the
// same order, with a tool; an edited kind has its own editor, every previewed kind the view
// router.
func TestKindsCoverFilekind(t *testing.T) {
	want := toolKinds()
	if len(Kinds) != len(want) {
		t.Fatalf("Kinds has %d entries, filekind.All %d without stage kinds", len(Kinds), len(want))
	}
	for i, k := range Kinds {
		if k.Kind != want[i].Kind {
			t.Errorf("Kinds[%d] = %s, want %s", i, k.Kind, want[i].Kind)
		}
		if k.New == nil || k.Summary == "" {
			t.Errorf("%s: no tool or summary", k.Kind)
		}
		if k.Edits == (k.Tool == "view") {
			t.Errorf("%s: Edits %v with tool %q", k.Kind, k.Edits, k.Tool)
		}
		if k.Starter != nil && !k.Edits {
			t.Errorf("%s: a previewed kind has a starter", k.Kind)
		}
		if (k.Flag == "") != (k.FlagValue == "") || (k.Flag == "") != (k.Noun == "") {
			t.Errorf("%s: flag %q, value %q", k.Kind, k.Flag, k.FlagValue)
		}
	}
}

func TestKindOf(t *testing.T) {
	for name, tool := range map[string]string{
		"a.png": "view", "a.sheet.json": "sheet", "a.anim.json": "anim", "a.fx.json": "fx",
		"a.aseprite.json": "view", "a.voice.json": "view", "a.TMJ": "view",
	} {
		if k, err := KindOf(name); err != nil || k.Tool != tool {
			t.Errorf("KindOf(%q) = %q, %v; want %q", name, k.Tool, err, tool)
		}
	}
	// A prefab has no tool: it opens in the editor's stage.
	if _, err := KindOf("a.prefab.json"); err == nil || !strings.Contains(err.Error(), "opens in VSCode or Weakpixel Studio") {
		t.Errorf("KindOf(a.prefab.json) = %v", err)
	}
	if _, err := KindOf("notes.txt"); err == nil || !strings.Contains(err.Error(), "supported: ") {
		t.Errorf("KindOf(notes.txt) = %v", err)
	}
}

func TestMissingError(t *testing.T) {
	tests := []struct{ file, want string }{
		{"art/tiles.sheet.json", "tiles.sheet.json does not exist: give --image <texture> to start a new sheet"},
		{"hero.anim.json", "hero.anim.json does not exist: give --sheet <export> to start a new definition"},
		{"a.png", "a.png does not exist"},
	}
	for _, tt := range tests {
		k, _ := KindOf(tt.file)
		if got := k.MissingError(tt.file).Error(); got != tt.want {
			t.Errorf("MissingError(%s) = %q, want %q", tt.file, got, tt.want)
		}
	}
}

// TestTable is the table the VSCode extension's file lists (toolTarget.ts,
// openers.ts) are compared with by hand: a change here is a change there.
func TestTable(t *testing.T) {
	const want = `.sheet.json edit
.anim.json edit
.fx.json edit
.prefab.json edit stage
.png .jpg .jpeg preview
.atlas.json preview
.ttf .otf preview
.kage preview
.tmj preview
.tsj preview
.world preview
.aseprite.json .ase.json preview
.voice.json preview sound
.dialogue.json preview sound
`
	if got := Table(); got != want {
		t.Errorf("Table() =\n%s\nwant\n%s", got, want)
	}
}

// TestUntitled: only a particle effect can be untitled (VSCode's new
// effect), and its text is the effect the fx tool starts from.
func TestUntitled(t *testing.T) {
	for _, k := range Kinds {
		if got, want := k.Untitled != nil, k.Kind == filekind.Fx; got != want {
			t.Errorf("%s: Untitled set = %v, want %v", k.Kind, got, want)
		}
	}
	k, err := KindOf("untitled.fx.json")
	if err != nil {
		t.Fatal(err)
	}
	if len(k.Untitled()) == 0 {
		t.Error("the untitled effect is empty")
	}
}
