package filekind

import (
	"strings"
	"testing"
)

func TestOf(t *testing.T) {
	tests := []struct {
		name string
		want Kind
		err  string
	}{
		{"a.png", Texture, ""},
		{"dir/A.PNG", Texture, ""},
		{"a.jpeg", Texture, ""},
		{"a.sheet.json", Sheet, ""},
		{"a.Sheet.json", "", "not a file wpix-editor opens"},
		{"a.anim.json", Anim, ""},
		{"a.ANIM.JSON", "", "not a file"},
		{"a.fx.json", Fx, ""},
		{"hero.prefab.json", Prefab, ""},
		{"hero.Prefab.json", "", "not a file"},
		{"a.atlas.json", Atlas, ""},
		{"a.Atlas.JSON", Atlas, ""},
		{"a.otf", Font, ""},
		{"a.kage", Shader, ""},
		{"a.tmj", Map, ""},
		{"a.tsj", Tileset, ""},
		{"a.world", World, ""},
		{"hero.aseprite.json", Aseprite, ""},
		{"hero.ase.json", Aseprite, ""},
		{"kip.voice.json", Voice, ""},
		{"intro.Dialogue.json", Dialogue, ""},
		{"a.json", "", "supported: .sheet.json"},
		{"view", "", "view is not a file"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Of(tt.name)
			if tt.err != "" {
				if err == nil || !strings.Contains(err.Error(), tt.err) {
					t.Fatalf("Of(%q) = %v, %v; want error %q", tt.name, got.Kind, err, tt.err)
				}
				return
			}
			if err != nil || got.Kind != tt.want {
				t.Fatalf("Of(%q) = %v, %v; want %v", tt.name, got.Kind, err, tt.want)
			}
		})
	}
}

// TestOneKindPerSuffix checks that no suffix belongs to two kinds and that
// every suffix is lower case, so a name has one kind.
func TestOneKindPerSuffix(t *testing.T) {
	seen := map[string]Kind{}
	for _, i := range All {
		for _, s := range i.Suffixes {
			if s != strings.ToLower(s) {
				t.Errorf("%s: suffix %q is not lower case", i.Kind, s)
			}
			if k, ok := seen[s]; ok {
				t.Errorf("suffix %q is in %s and %s", s, k, i.Kind)
			}
			seen[s] = i.Kind
			if got, err := Of("x" + s); err != nil || got.Kind != i.Kind {
				t.Errorf("Of(x%s) = %v, %v; want %s", s, got.Kind, err, i.Kind)
			}
		}
	}
}

func TestSoundOnlyForSpeech(t *testing.T) {
	for _, i := range All {
		if want := i.Kind == Voice || i.Kind == Dialogue; i.Sound != want {
			t.Errorf("%s: Sound = %v, want %v", i.Kind, i.Sound, want)
		}
	}
}
