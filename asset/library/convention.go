package library

import (
	"path"
	"strings"
)

// Package folder names with a meaning at the library root. Every other top
// folder is a package of its own, except the reserved ones ([PackageOf]).
const (
	// GlobalDir is the required global package: loaded at boot into the
	// global scope and never released.
	GlobalDir = "global"
	// BootDir is the optional boot package: mounted before everything else,
	// blocking, so a startup screen can draw while global/ loads. It may
	// reference no other package.
	BootDir = "boot"
)

// PackageOf returns the package of the file at p (a key's path, without
// "#sub"): its first folder. "forest/tiles/trees.tsj" is in "forest",
// "global/fonts/ui.ttf" in "global". A file at the library root, a file in
// a reserved folder (packs/, or a name starting with "." or "_") and a
// folder name alone return "".
func PackageOf(p string) string {
	first, rest, ok := strings.Cut(p, "/")
	if !ok || first == "" || rest == "" || reservedFolder(first) {
		return ""
	}
	return first
}

// reservedFolder reports whether a top folder is not a package: the packs
// of a packed build, and scratch or source folders starting with "." or "_".
func reservedFolder(name string) bool {
	return name == PacksDir || strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_")
}

// Kind names what an asset file is, derived from its name by [KindOf].
type Kind string

// The kinds the convention knows. KindLevel and KindPrefab are GOD
// documents: the engine spawns levels itself and loads prefabs as assets
// (prefab). KindEffect is a particle effect (particles); like a
// GOD document it names its sprite and shader by key from the asset root.
// KindVoice, KindLanguage and KindDialogue are the speech files
// (speech, which sets their loaders with speech.SetLibraryLoaders).
const (
	KindUnknown  Kind = ""
	KindTexture  Kind = "texture"  // *.png, *.jpg, *.jpeg
	KindAtlas    Kind = "atlas"    // *.sheet.json (grid), *.atlas.json (TexturePacker)
	KindClipSet  Kind = "clipset"  // *.anim.json, *.aseprite.json
	KindTileset  Kind = "tileset"  // *.tsj
	KindMap      Kind = "map"      // *.tmj
	KindWorld    Kind = "world"    // *.world
	KindFont     Kind = "font"     // *.ttf, *.otf (load as "file.ttf#size")
	KindShader   Kind = "shader"   // *.kage
	KindSound    Kind = "sound"    // *.wav, *.ogg, *.mp3 outside a music/ folder
	KindMusic    Kind = "music"    // *.wav, *.ogg, *.mp3 inside a music/ folder
	KindBindings Kind = "bindings" // *.bindings.json
	KindLevel    Kind = "level"    // *.level.json
	KindPrefab   Kind = "prefab"   // *.prefab.json
	KindEffect   Kind = "effect"   // *.fx.json
	KindVoice    Kind = "voice"    // *.voice.json
	KindLanguage Kind = "language" // *.lang.json
	KindDialogue Kind = "dialogue" // *.dialogue.json
)

// suffixKinds is checked in order, so the double suffixes come before
// their plain extensions.
var suffixKinds = []struct {
	suffix string
	kind   Kind
}{
	{".level.json", KindLevel},
	{".prefab.json", KindPrefab},
	{".fx.json", KindEffect},
	{".voice.json", KindVoice},
	{".lang.json", KindLanguage},
	{".dialogue.json", KindDialogue},
	{".bindings.json", KindBindings},
	{".anim.json", KindClipSet},
	{".aseprite.json", KindClipSet},
	{".atlas.json", KindAtlas},
	{".sheet.json", KindAtlas},
	{".tsj", KindTileset},
	{".tmj", KindMap},
	{".world", KindWorld},
	{".png", KindTexture},
	{".jpg", KindTexture},
	{".jpeg", KindTexture},
	{".ttf", KindFont},
	{".otf", KindFont},
	{".kage", KindShader},
}

var audioExts = []string{".wav", ".ogg", ".mp3"}

// KindOf returns the kind of the file at p (a key's path, without "#sub")
// by its name, case-insensitively. Audio is music inside a folder named
// "music" and a sound anywhere else. Other names (a plain *.json, an
// *.aseprite source file) are KindUnknown: they load only as dependencies
// or by explicit key.
func KindOf(p string) Kind {
	lower := strings.ToLower(p)
	for _, sk := range suffixKinds {
		if strings.HasSuffix(lower, sk.suffix) {
			return sk.kind
		}
	}
	for _, ext := range audioExts {
		if strings.HasSuffix(lower, ext) {
			if inFolder(lower, "music") {
				return KindMusic
			}
			return KindSound
		}
	}
	return KindUnknown
}

// inFolder reports whether a directory of p is named dir.
func inFolder(p, dir string) bool {
	for _, seg := range strings.Split(path.Dir(p), "/") {
		if seg == dir {
			return true
		}
	}
	return false
}

// entryKind reports whether a file of kind k is loaded by its group when
// nothing in the group references it. Fonts need a size, and levels are GOD
// documents the engine spawns itself, so they are entries only when
// library.json lists them. A prefab, an effect, a voice or a dialogue nothing
// references is loaded, so code can spawn or play it; a language loads with
// the voices that name it.
func entryKind(k Kind) bool {
	switch k {
	case KindTexture, KindAtlas, KindClipSet, KindTileset, KindMap, KindWorld,
		KindShader, KindSound, KindMusic, KindBindings, KindPrefab, KindEffect,
		KindVoice, KindDialogue:
		return true
	}
	return false
}
