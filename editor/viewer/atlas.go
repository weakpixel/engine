package viewer

import (
	"encoding/json"
	"fmt"
	"path"
	"strings"

	"github.com/hajimehoshi/ebiten/v2"

	"github.com/weakpixel/asset"
	"github.com/weakpixel/engine/editor/filekind"
	"github.com/weakpixel/engine/editor/toolshell"
	"github.com/weakpixel/engine/editor/viewkit"
	"github.com/weakpixel/engine/render"
)

// atlasView shows an atlas: the texture it is cut from, with every frame
// outlined and labelled, and the frame under the mouse described in the
// panel. It reads the atlas with the engine's loader, so what it shows is
// what the game gets.
type atlasView struct {
	textureView
	atlas   render.AtlasHandle
	version uint32
	frames  viewkit.Frames
}

// Open implements [toolshell.Tool].
func (v *atlasView) Open(s *toolshell.Shell, key string) error {
	// The atlas loader also reads grid sheets, but a *.sheet.json opens in
	// the sheet editor, which shows its frames itself.
	if k, err := filekind.Of(key); err != nil || k.Kind != filekind.Atlas {
		return fmt.Errorf("view: %s is not an atlas (%s)", path.Base(key), strings.Join(atlasSuffixes(), ", "))
	}
	v.shell, v.key = s, key
	v.frames = viewkit.NewFrames("atlas")
	srv := s.Engine().Assets()
	h, err := render.LoadAtlasSync(v.scope, key)
	if err != nil {
		return fmt.Errorf("view: %w", err)
	}
	v.atlas = h
	atlas := s.Engine().Stores().Atlases().Get(h)
	image := asset.StoreOf[render.Texture](srv).Key(atlas.Texture).Path
	if err := v.openImage(s, image); err != nil {
		return err
	}
	v.key = key // openImage names the panel after the texture
	v.reload()
	return nil
}

// reload reads the frames again and notes the atlas's version.
func (v *atlasView) reload() {
	e := v.shell.Engine()
	if a := e.Stores().Atlases().Get(v.atlas); a != nil {
		v.frames.SetList(viewkit.FramesOf(a, e.Stores().Sprites()))
	}
	v.version = e.Stores().Atlases().Generic().Version(asset.HandleOf[render.Atlas](v.atlas.Ref))
}

// Update implements [toolshell.Tool]: a reloaded atlas, the frame under the
// mouse and a click that selects it.
func (v *atlasView) Update(s *toolshell.Shell) {
	if !v.ready {
		return
	}
	v.textureView.Update(s)
	if ver := s.Engine().Stores().Atlases().Generic().Version(asset.HandleOf[render.Atlas](v.atlas.Ref)); ver != v.version {
		v.reload()
	}
	v.frames.Update(s)
}

// Draw implements [toolshell.Tool]: the texture's border, then the frames.
func (v *atlasView) Draw(s *toolshell.Shell, screen ebiten.FinalScreen) {
	if !v.ready {
		return
	}
	v.textureView.Draw(s, screen)
	v.frames.Draw(s, screen)
}

// Panel implements [toolshell.Tool].
func (v *atlasView) Panel() toolshell.Panel {
	which := "none"
	fields := []toolshell.Field{
		toolshell.Label("atlas.count", "Frames", fmt.Sprint(len(v.frames.List))),
	}
	f, ok := v.frames.Shown()
	detail := func(id, label, text string) toolshell.Field { return toolshell.Label("frame."+id, label, text) }
	if ok {
		which = "hover"
		if v.frames.Sel >= 0 {
			which = "selected"
		}
		name := f.Name
		if name == "" {
			name = "-"
		}
		fields = append(fields,
			detail("which", "Frame", fmt.Sprintf("%d (%s)", f.Index, which)),
			detail("name", "Name", name),
			detail("rect", "Rect", viewkit.RectText(f.Rect)),
			detail("size", "Untrimmed", viewkit.PointText(f.Size)),
			detail("offset", "Trim offset", viewkit.PointText(f.Offset)),
			detail("pivot", "Pivot", viewkit.PointText(f.Pivot)),
		)
	} else {
		fields = append(fields, detail("which", "Frame", "click a frame to select it"))
	}
	return toolshell.Panel{Title: path.Base(v.key), Sections: []toolshell.Section{
		v.frames.Section(),
		{ID: "atlas", Label: "Atlas", Fields: fields},
		v.imageSection(),
		v.shell.ViewSection(),
	}}
}

// Set implements [toolshell.Tool].
func (v *atlasView) Set(id string, val json.RawMessage) error {
	if ok, err := v.frames.Set(id, val); ok {
		return err
	}
	return fmt.Errorf("view: unknown widget %q", id)
}

// atlasSuffixes are the suffixes of an atlas file.
func atlasSuffixes() []string {
	i, _ := filekind.Lookup(filekind.Atlas)
	return i.Suffixes
}
