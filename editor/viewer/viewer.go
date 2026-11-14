// Package viewer is the "view" tool of weakpixel-editor: it previews an asset
// with the engine's own loaders. A texture is shown on a grid with pan and
// zoom; an atlas (*.sheet.json, *.atlas.json) shows its frames over the
// texture, and a font (*.ttf, *.otf) shows sample text at the sizes the
// project uses. A Tiled map (*.tmj) shows its layers and objects, a tileset
// (*.tsj) its tiles with ids, collision shapes and animations, and a world
// (*.world) its maps with the levels that use them. A shader (*.kage) is drawn
// on a texture with a control per +shader:param marker. The previewers for
// other kinds come with later milestones.
package viewer

import (
	"encoding/json"
	"fmt"
	"path"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/weakpixel/asset"
	"github.com/weakpixel/engine/editor/toolshell"
)

// kind is what a previewer shows.
type kind int

const (
	kindTexture kind = iota
	kindAtlas
	kindFont
	kindMap
	kindTileset
	kindWorld
	kindShader
)

// supported is the list in the error of a file the viewer cannot show.
const supported = ".png, .jpg, .sheet.json, .atlas.json, .ttf, .otf, .tmj, .tsj, .world, .kage"

// kindOf picks the previewer of the file at asset key.
func kindOf(key string) (kind, error) {
	lower := strings.ToLower(key)
	switch ext := path.Ext(lower); {
	case ext == ".png", ext == ".jpg", ext == ".jpeg":
		return kindTexture, nil
	case strings.HasSuffix(lower, ".sheet.json"), strings.HasSuffix(lower, ".atlas.json"):
		return kindAtlas, nil
	case ext == ".ttf", ext == ".otf":
		return kindFont, nil
	case ext == ".tmj":
		return kindMap, nil
	case ext == ".tsj":
		return kindTileset, nil
	case ext == ".world":
		return kindWorld, nil
	case ext == ".kage":
		return kindShader, nil
	default:
		return 0, fmt.Errorf("view: %s files are not supported (supported: %s)", ext, supported)
	}
}

// Viewer is the "view" tool: it picks the previewer of its file when it
// opens, and passes everything on to it. It is a [toolshell.Tool] without a
// document and a [toolshell.Closer]: the host can switch it to another file.
type Viewer struct {
	inner toolshell.Tool
	// scope holds the assets of the file shown, so switching files unloads
	// the previous ones instead of keeping them in the global scope.
	scope *asset.Scope
}

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
	case kindAtlas:
		v.inner = &atlasView{textureView: textureView{scope: v.scope}}
	case kindFont:
		v.inner = &fontView{scope: v.scope}
	case kindMap:
		v.inner = &mapView{tiledBase: tiledBase{scope: v.scope}}
	case kindTileset:
		v.inner = &tilesetView{tiledBase: tiledBase{scope: v.scope}}
	case kindWorld:
		v.inner = &worldView{tiledBase: tiledBase{scope: v.scope}}
	case kindShader:
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
