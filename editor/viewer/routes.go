package viewer

import (
	"github.com/weakpixel/engine/editor/animedit"
	"github.com/weakpixel/engine/editor/toolshell"
	"github.com/weakpixel/engine/editor/voiceview"
)

// asepritePreview shows an Aseprite export in the anim tool's read-only
// layout: it plays the export's tags with their own timing. The export has
// no document; the tool reads it through the loaders, so Open also loads it.
type asepritePreview struct {
	*animedit.Tool
}

// Open implements [toolshell.Tool].
func (p *asepritePreview) Open(s *toolshell.Shell, key string) error {
	if err := p.Tool.Open(s, key); err != nil {
		return err
	}
	return p.Tool.Load(nil)
}

func (p *asepritePreview) close(s *toolshell.Shell) { p.Tool.Close(s) }

// voicePreview is the voice tool, installed by [Viewer.Init]; Viewer keeps
// it across files.
type voicePreview struct {
	*voiceview.Tool
}

func (p voicePreview) close(s *toolshell.Shell) { p.Tool.Close(s) }
