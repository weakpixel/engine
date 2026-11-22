// Package viewer is the "view" tool of wpix-editor: it previews every file
// kind of package filekind that is not edited, with the engine's own
// loaders. A texture is shown on a grid with pan and zoom; an atlas
// (*.atlas.json) shows its frames over the texture, and a font (*.ttf, *.otf) shows sample text at the sizes the
// project uses. A Tiled map (*.tmj) shows its layers and objects, a tileset
// (*.tsj) its tiles with ids, collision shapes and animations, and a world
// (*.world) its maps with the levels that use them. A shader (*.kage) is drawn
// on a texture with a control per +shader:param marker. An Aseprite export
// (*.aseprite.json, *.ase.json) plays its clips in the anim tool's read-only
// layout, and a voice or a dialogue is spoken by the voice tool, with sound.
package viewer

import (
	"encoding/json"
	"fmt"
	"path"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/weakpixel/asset"
	"github.com/weakpixel/engine/editor/animedit"
	"github.com/weakpixel/engine/editor/filekind"
	"github.com/weakpixel/engine/editor/toolshell"
	"github.com/weakpixel/engine/editor/voiceview"
)

// kindOf returns the kind of the file at asset key, which must be one the
// viewer previews: a kind of [filekind.All] that is not edited.
func kindOf(key string) (filekind.Kind, error) {
	info, err := filekind.Of(key)
	if err != nil {
		return "", fmt.Errorf("view: %w", err)
	}
	if info.Edits {
		return "", fmt.Errorf("view: %s opens in the %s editor, not in the preview", path.Base(key), info.Kind)
	}
	return info.Kind, nil
}

// Viewer is the "view" tool, the preview of every kind of [filekind.All]
// that is not edited: it picks the previewer of its file when it opens, and
// passes everything on to it. It is a [toolshell.Tool] without a document, a
// [toolshell.Closer] (the host can switch it to another file) and a
// [toolshell.Initializer]: when its first file has sound (a voice or a
// dialogue) it installs package speech, and only then can it show those
// kinds. A page or window that starts without sound reloads to show them.
type Viewer struct {
	inner toolshell.Tool
	// voice is the voice and dialogue previewer, set by Init when the first
	// file has sound. It is kept across files: speech is installed once.
	voice *voiceview.Tool
	// scope holds the assets of the file shown, so switching files unloads
	// the previous ones instead of keeping them in the global scope.
	scope *asset.Scope
}

// The shell finds these interfaces of the viewer with type assertions, and
// the viewer those of its previewers: each previewer that spawns entities is
// a closer, and each that moves things every tick a Syncer.
var (
	_ toolshell.Initializer = (*Viewer)(nil)
	_ toolshell.Closer      = (*Viewer)(nil)
	_ toolshell.Syncer      = (*Viewer)(nil)
	_ toolshell.Checker     = (*Viewer)(nil)

	_ closer           = (*textureView)(nil)
	_ toolshell.Syncer = (*textureView)(nil)
	_ closer           = (*atlasView)(nil)
	_ toolshell.Syncer = (*atlasView)(nil)
	_ closer           = (*shaderView)(nil)
	_ toolshell.Syncer = (*shaderView)(nil)
	_ closer           = (*mapView)(nil)
	_ closer           = (*tilesetView)(nil)
	_ toolshell.Syncer = (*tilesetView)(nil)
	_ closer           = (*worldView)(nil)
	_ closer           = (*asepritePreview)(nil)
	_ toolshell.Syncer = (*asepritePreview)(nil)
	_ closer           = voicePreview{}
	_ toolshell.Syncer = voicePreview{}
	_ toolshell.Tool   = (*fontView)(nil)
)

// closer is a previewer that spawned entities.
type closer interface {
	close(s *toolshell.Shell)
}

// New returns a viewer.
func New() toolshell.Tool { return &Viewer{inner: idle{}} }

// idle is the previewer before Open and after a file that cannot be shown.
type idle struct{}

func (idle) Open(*toolshell.Shell, string) error       { return nil }
func (idle) Update(*toolshell.Shell)                   {}
func (idle) Draw(*toolshell.Shell, ebiten.FinalScreen) {}
func (idle) Panel() toolshell.Panel                    { return toolshell.Panel{Title: "view"} }
func (idle) Set(id string, _ json.RawMessage) error    { return fmt.Errorf("view: unknown widget %q", id) }

// Check reports whether the viewer can show the file at asset key. The
// command calls it before opening a window.
func (*Viewer) Check(key string) error {
	_, err := kindOf(key)
	return err
}

// Init implements [toolshell.Initializer]: a first file with sound installs
// the voice previewer, which installs package speech.
func (v *Viewer) Init(s *toolshell.Shell) {
	if info, err := filekind.Of(s.Key()); err == nil && info.Sound && v.voice == nil {
		v.voice = voiceview.New().(*voiceview.Tool)
		v.voice.Init(s)
	}
}

// Open implements [toolshell.Tool].
func (v *Viewer) Open(s *toolshell.Shell, key string) error {
	k, err := kindOf(key)
	if err != nil {
		return err
	}
	if v.scope == nil {
		v.scope = s.Engine().Assets().NewScope("view")
	}
	switch k {
	case filekind.Voice, filekind.Dialogue:
		if v.voice == nil {
			return fmt.Errorf("view: %s has sound: it shows in a page or window that starts with it", path.Base(key))
		}
		v.inner = voicePreview{v.voice}
	case filekind.Aseprite:
		v.inner = &asepritePreview{Tool: animedit.NewPreview(v.scope)}
	case filekind.Atlas:
		v.inner = &atlasView{textureView: textureView{scope: v.scope}}
	case filekind.Font:
		v.inner = &fontView{scope: v.scope}
	case filekind.Map:
		v.inner = &mapView{tiledBase: tiledBase{scope: v.scope}}
	case filekind.Tileset:
		v.inner = &tilesetView{tiledBase: tiledBase{scope: v.scope}}
	case filekind.World:
		v.inner = &worldView{tiledBase: tiledBase{scope: v.scope}}
	case filekind.Shader:
		v.inner = &shaderView{textureView: textureView{scope: v.scope}}
	default:
		v.inner = &textureView{scope: v.scope}
	}
	return v.inner.Open(s, key)
}

// Close implements [toolshell.Closer]: it removes the previewer's entities
// and releases the file's assets; they unload at the engine's next asset
// update unless the next file holds them again.
func (v *Viewer) Close(s *toolshell.Shell) {
	if c, ok := v.inner.(closer); ok {
		c.close(s)
	}
	v.inner = idle{}
	if v.scope != nil {
		v.scope.Release()
	}
}

// Update implements [toolshell.Tool].
func (v *Viewer) Update(s *toolshell.Shell) { v.inner.Update(s) }

// Sync implements [toolshell.Syncer] for the previewers that need it.
func (v *Viewer) Sync(s *toolshell.Shell) {
	if sy, ok := v.inner.(toolshell.Syncer); ok {
		sy.Sync(s)
	}
}

// Draw implements [toolshell.Tool].
func (v *Viewer) Draw(s *toolshell.Shell, screen ebiten.FinalScreen) { v.inner.Draw(s, screen) }

// Panel implements [toolshell.Tool].
func (v *Viewer) Panel() toolshell.Panel { return v.inner.Panel() }

// Set implements [toolshell.Tool].
func (v *Viewer) Set(id string, val json.RawMessage) error { return v.inner.Set(id, val) }
