// Package filekind is the one list of the file kinds wpix-editor opens: the
// name suffixes of each kind, whether it is edited or previewed and whether
// it has sound. The file name is the only rule, as for the engine's loaders;
// the content of a file is never looked at. It imports the standard library
// only, so the preview router (editor/viewer) and the wpix-editor command
// (which adds a tool to every kind) read the same table.
package filekind

import (
	"fmt"
	"path"
	"strings"
)

// Kind names a file kind.
type Kind string

// The file kinds.
const (
	Sheet    Kind = "sheet"    // a grid sprite sheet, edited
	Anim     Kind = "anim"     // an animation definition, edited
	Fx       Kind = "fx"       // a particle effect, edited
	Texture  Kind = "texture"  // an image
	Atlas    Kind = "atlas"    // a texture atlas with named frames
	Font     Kind = "font"     // a TrueType or OpenType font
	Shader   Kind = "shader"   // a Kage shader
	Map      Kind = "map"      // a Tiled map
	Tileset  Kind = "tileset"  // a Tiled tileset
	World    Kind = "world"    // a Tiled world
	Aseprite Kind = "aseprite" // an Aseprite export, previewed read-only (the format belongs to Aseprite)
	Voice    Kind = "voice"    // a voice of package speech, with sound
	Dialogue Kind = "dialogue" // a dialogue of package speech, with sound
)

// Info is one file kind.
type Info struct {
	Kind Kind
	// Suffixes are the name suffixes of the kind, lower case.
	Suffixes []string
	// CaseSensitive is true for a kind whose suffixes match only in lower
	// case (the engine's own formats); the others match in any case.
	CaseSensitive bool
	// Edits is true for a kind that opens in its editor; the others open in
	// the preview.
	Edits bool
	// Sound is true for a kind whose tool plays sound: its page or window
	// starts with the audio context open.
	Sound bool
}

// All lists every kind once, the edited kinds first.
var All = []Info{
	{Kind: Sheet, Suffixes: []string{".sheet.json"}, CaseSensitive: true, Edits: true},
	{Kind: Anim, Suffixes: []string{".anim.json"}, CaseSensitive: true, Edits: true},
	{Kind: Fx, Suffixes: []string{".fx.json"}, CaseSensitive: true, Edits: true},
	{Kind: Texture, Suffixes: []string{".png", ".jpg", ".jpeg"}},
	{Kind: Atlas, Suffixes: []string{".atlas.json"}},
	{Kind: Font, Suffixes: []string{".ttf", ".otf"}},
	{Kind: Shader, Suffixes: []string{".kage"}},
	{Kind: Map, Suffixes: []string{".tmj"}},
	{Kind: Tileset, Suffixes: []string{".tsj"}},
	{Kind: World, Suffixes: []string{".world"}},
	{Kind: Aseprite, Suffixes: []string{".aseprite.json", ".ase.json"}, CaseSensitive: true},
	{Kind: Voice, Suffixes: []string{".voice.json"}, Sound: true},
	{Kind: Dialogue, Suffixes: []string{".dialogue.json"}, Sound: true},
}

// Matches reports whether the file name or asset key name is of the kind.
func (i Info) Matches(name string) bool {
	if !i.CaseSensitive {
		name = strings.ToLower(name)
	}
	for _, s := range i.Suffixes {
		if strings.HasSuffix(name, s) {
			return true
		}
	}
	return false
}

// Of returns the kind of the file name or asset key name. A name of no
// kind is an error that names the supported suffixes.
func Of(name string) (Info, error) {
	for _, i := range All {
		if i.Matches(name) {
			return i, nil
		}
	}
	return Info{}, fmt.Errorf("%s is not a file wpix-editor opens (supported: %s)", path.Base(name), Supported())
}

// Lookup returns the entry of kind k; ok is false for a kind not in [All].
func Lookup(k Kind) (info Info, ok bool) {
	for _, i := range All {
		if i.Kind == k {
			return i, true
		}
	}
	return Info{}, false
}

// Supported lists every suffix of [All], comma separated, in table order.
func Supported() string {
	var s []string
	for _, i := range All {
		s = append(s, i.Suffixes...)
	}
	return strings.Join(s, ", ")
}
