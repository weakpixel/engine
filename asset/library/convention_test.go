package library

import "testing"

func TestPackageOf(t *testing.T) {
	tests := map[string]string{
		"global/fonts/ui.ttf":    "global",
		"boot/logo.png":          "boot",
		"forest/a.png":           "forest",
		"forest/deep/a.png":      "forest",
		"levels/forest/a.png":    "levels",
		"x.png":                  "",
		"forest":                 "",
		"forest/":                "",
		"packs/forest.1234.zip":  "",
		".git/config":            "",
		"_sources/hero.aseprite": "",
		"library.json":           "",
	}
	for p, want := range tests {
		if got := PackageOf(p); got != want {
			t.Errorf("PackageOf(%q) = %q, want %q", p, got, want)
		}
	}
}

func TestKindOf(t *testing.T) {
	tests := map[string]Kind{
		"a/hero.anim.json":      KindClipSet,
		"a/hero.aseprite.json":  KindClipSet,
		"a/hero.ase.json":       KindClipSet,
		"a/ui.atlas.json":       KindAtlas,
		"a/tiles.sheet.json":    KindAtlas,
		"a/HERO.PNG":            KindTexture,
		"a/x.jpeg":              KindTexture,
		"a/t.tsj":               KindTileset,
		"a/m.tmj":               KindMap,
		"a/w.world":             KindWorld,
		"a/f.ttf":               KindFont,
		"a/f.otf":               KindFont,
		"a/fx.kage":             KindShader,
		"sfx/hit.wav":           KindSound,
		"levels/x/music/a.ogg":  KindMusic,
		"music/a.mp3":           KindMusic,
		"musicbox/a.mp3":        KindSound,
		"input/p.bindings.json": KindBindings,
		"levels/x/x.level.json": KindLevel,
		"a/goblin.prefab.json":  KindPrefab,
		"a/sparks.FX.json":      KindEffect,
		"a/bard.voice.json":     KindVoice,
		"a/elvish.lang.json":    KindLanguage,
		"a/town.dialogue.json":  KindDialogue,
		"a/plain.json":          KindUnknown,
		"a/hero.aseprite":       KindUnknown,
	}
	for p, want := range tests {
		if got := KindOf(p); got != want {
			t.Errorf("KindOf(%q) = %q, want %q", p, got, want)
		}
	}
}
